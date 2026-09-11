package streaming

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/genai"

	"buddy/server/internal/chat"
	"buddy/server/internal/domain"
	platformVertex "buddy/server/internal/platform/vertex"
	"buddy/server/internal/rag"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // Allow same-origin / non-browser requests
		}
		allowedRaw := os.Getenv("CORS_ALLOWED_ORIGINS")
		if allowedRaw == "" {
			return true // Dev: no restriction if env not set
		}
		for _, allowed := range strings.Split(allowedRaw, ",") {
			if strings.TrimSpace(allowed) == origin {
				return true
			}
		}
		return false
	},
	ReadBufferSize:  65536,
	WriteBufferSize: 65536,
}

// ClientMessage represents incoming multimodal packets from the browser.
type ClientMessage struct {
	Type        string `json:"type"`                   // "auth", "audio", "video_frame", "text", "ping"
	PCMBase64   string `json:"pcm_base64,omitempty"`   // Audio chunk base64 (16kHz PCM 16-bit mono)
	ImageBase64 string `json:"image_base64,omitempty"` // Snapshot of webcam or screen
	Source      string `json:"source,omitempty"`       // "webcam" or "screen"
	Text        string `json:"text,omitempty"`         // Optional transcribed or written text
	Token       string `json:"token,omitempty"`        // Bearer ID token, only for the initial "auth" handshake message
}

// ServerMessage represents outgoing packets sent back to the browser.
type ServerMessage struct {
	Type        string `json:"type"`                  // "transcript", "audio", "turn_complete", "interrupted", "pong", "error"
	Text        string `json:"text,omitempty"`        // Transcribed words or Buddy response
	IsFinal     bool   `json:"is_final,omitempty"`    // Whether turn finished
	PCMBase64   string `json:"pcm_base64,omitempty"`  // Synthesized native audio (24kHz PCM 16-bit mono)
	SampleRate  int    `json:"sample_rate,omitempty"` // Sample rate of PCM audio
	Interrupted bool   `json:"interrupted,omitempty"` // Interruption signal
	Message     string `json:"message,omitempty"`
}

type activeSessionRecord struct {
	cancel context.CancelFunc
	conn   *websocket.Conn
}

// closeWithError sends a JSON error frame followed by a proper WebSocket close control
// frame, then closes the connection. Bailing out with a bare conn.Close() (no close
// frame) makes the browser — and any intermediary reverse proxy relaying the socket,
// such as the Next.js dev server's rewrite proxy — see an abrupt TCP reset (WebSocket
// close code 1006 / ECONNRESET) instead of a clean, expected closure. Always use this
// helper for a deliberate early termination of a live-talk connection.
func closeWithError(conn *websocket.Conn, message string) {
	_ = conn.WriteJSON(ServerMessage{Type: "error", Message: message})
	deadline := time.Now().Add(2 * time.Second)
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, message), deadline)
}

// Handler manages bidirectional real-time multimodal live talk sessions with Gemini 2.5.
type Handler struct {
	authClient     domain.AuthClient
	vertexClient   *platformVertex.Client
	chatService    *chat.Service
	ragService     *rag.Service
	devMode        bool
	activeSessions sync.Map // map[string]*activeSessionRecord (studentUID -> record)
}

// NewHandler initializes a streaming live handler.
func NewHandler(authClient domain.AuthClient, vertexClient *platformVertex.Client, chatService *chat.Service, ragService *rag.Service, devMode bool) *Handler {
	return &Handler{
		authClient:   authClient,
		vertexClient: vertexClient,
		chatService:  chatService,
		ragService:   ragService,
		devMode:      devMode,
	}
}

// HandleLiveTalk upgrades HTTP to WebSocket and manages live bidirectional audio/vision talk with Gemini 2.5 Live.
//
// Authentication supports two paths:
//  1. Legacy/tooling: a bearer token passed via the "token" query param or Authorization
//     header, verified before the upgrade (kept for Postman/manual testing).
//  2. Browser clients: no token is supplied on the URL at all (avoiding Firebase ID
//     tokens ending up in server/proxy access logs and browser history). The connection
//     is upgraded first, then the client's very first WebSocket message must be an
//     {"type":"auth","token":"..."} handshake, verified within a short deadline.
func (h *Handler) HandleLiveTalk(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			token = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}

	if token == "" && h.devMode {
		devUID := r.URL.Query().Get("dev_uid")
		if devUID == "" {
			devUID = "dev-student"
		}
		token = "dev:student:" + devUID + ":dev@buddyai.local"
	}

	preAuthenticated := token != ""

	var authUser *domain.AuthUser
	var err error
	if preAuthenticated {
		authUser, err = h.authClient.VerifyIDToken(r.Context(), token)
		if err != nil {
			http.Error(w, "Unauthorized: invalid authentication token", http.StatusUnauthorized)
			return
		}
	}

	// 2. Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[Live Talk] WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// 2.05 Post-upgrade auth handshake for browser clients that didn't pass a token on
	// the URL: the first message must be {"type":"auth","token":"<bearer id token>"}.
	if !preAuthenticated {
		_ = conn.SetReadDeadline(time.Now().Add(8 * time.Second))
		_, rawMsg, readErr := conn.ReadMessage()
		if readErr != nil {
			closeWithError(conn, "Authentication handshake timed out")
			return
		}

		var authMsg ClientMessage
		if unmarshalErr := json.Unmarshal(rawMsg, &authMsg); unmarshalErr != nil || authMsg.Type != "auth" || authMsg.Token == "" {
			closeWithError(conn, "First message must be an auth handshake")
			return
		}

		authUser, err = h.authClient.VerifyIDToken(r.Context(), authMsg.Token)
		if err != nil {
			closeWithError(conn, "Unauthorized: invalid authentication token")
			return
		}
		_ = conn.SetReadDeadline(time.Time{})
	}

	// 2.1 Enforce strictly ONE live session per user: cancel and close any previous active session
	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	defer sessionCancel()

	if old, exists := h.activeSessions.Load(authUser.UID); exists {
		if oldSess, ok := old.(*activeSessionRecord); ok {
			log.Printf("[Live Talk] Closing previous active session for user %s to enforce single live session", authUser.UID)
			oldSess.cancel()
			_ = oldSess.conn.Close()
		}
	}
	sessRecord := &activeSessionRecord{cancel: sessionCancel, conn: conn}
	h.activeSessions.Store(authUser.UID, sessRecord)
	defer func() {
		if cur, loaded := h.activeSessions.Load(authUser.UID); loaded && cur == sessRecord {
			h.activeSessions.Delete(authUser.UID)
		}
	}()

	var writeMu sync.Mutex
	safeWriteJSON := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteJSON(v)
	}

	log.Printf("[Live Talk] Live session established for user %s (%s)", authUser.UID, authUser.Email)

	// Fetch student personal memory (kept in memory for on-demand tool recall)
	studentMem, _ := h.chatService.GetPersonalIntelligence(r.Context(), authUser.UID)

	// The persona/policy layer comes from the admin's Live Talk prompt (editable at
	// /admin/prompt, independently of the text-chat prompt — see domain.PromptKind),
	// falling back to the seeded default if nothing has been saved yet. Tool-calling
	// instructions and the security constraints block are fixed technical scaffolding
	// appended below regardless of what's configured there, the same way text chat
	// layers its own formatting directive on top in GenerateChatResponse — this keeps
	// tool-calling and guardrails working even if an admin's prompt doesn't mention
	// them.
	liveSystemPrompt := domain.DefaultLiveTalkSystemPrompt
	if livePrompt, err := h.chatService.GetActivePrompt(r.Context(), domain.PromptKindLiveTalk); err == nil && livePrompt != nil && livePrompt.Content != "" {
		liveSystemPrompt = livePrompt.Content
	}

	liveSystemPrompt += `

### TOOL USAGE:
- When the student asks about foundation course notes, engineering syllabus, workshop guidelines, or study materials, call the tool "query_foundation_knowledge".
- When the student asks about past details or you genuinely need specific background context to assist them, call the tool "recall_student_memory".

` + platformVertex.MemoryToolRulesBlock + `

` + platformVertex.SecurityConstraintsBlock

	if authUser.DisplayName != "" {
		liveSystemPrompt += fmt.Sprintf("\n\nStudent Name: %s", authUser.DisplayName)
	}

	// Model selection for native audio live talk
	liveModel := os.Getenv("VERTEX_LIVE_MODEL")
	if liveModel == "" {
		liveModel = "gemini-live-2.5-flash-native-audio"
	}

	// 3. Connect to Vertex AI Live Session
	var liveSession *genai.Session
	if h.vertexClient != nil && h.vertexClient.LiveGenAIClient() != nil {
		liveConfig := &genai.LiveConnectConfig{
			ResponseModalities: []genai.Modality{genai.ModalityAudio},
			SpeechConfig: &genai.SpeechConfig{
				VoiceConfig: &genai.VoiceConfig{
					PrebuiltVoiceConfig: &genai.PrebuiltVoiceConfig{
						VoiceName: "Aoede",
					},
				},
			},
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{
					genai.NewPartFromText(liveSystemPrompt),
				},
			},
			Tools: []*genai.Tool{platformVertex.LiveTalkToolsDeclaration()},
			// The student's own words are deliberately never transcribed into chat
			// history at all (see extractUserMemory below) — both an automatic
			// ASR-transcription approach and a later function-call approach were tried
			// and dropped as unreliable/unwanted. Buddy's own spoken replies, by
			// contrast, are reliably transcribed via OutputAudioTranscription, so that
			// one stays enabled.
			OutputAudioTranscription: &genai.AudioTranscriptionConfig{},
		}

		sess, liveErr := h.vertexClient.LiveGenAIClient().Live.Connect(sessionCtx, liveModel, liveConfig)
		if liveErr != nil {
			log.Printf("[Live Talk] Native live connect to %s failed: %v. Will fall back to standard multimodal turn responses.", liveModel, liveErr)
		} else {
			liveSession = sess
			defer liveSession.Close()
			log.Printf("[Live Talk] Native live session opened with model %s", liveModel)
		}
	}

	// The Live Talk conversation session is only created in chat history the moment
	// there's an actual reply worth saving — not eagerly on connect. Creating it
	// upfront meant every single Live Talk open left behind a session in the
	// student's history, even one where the student said nothing and just closed
	// the call again, and the canned greeting below always counted as that "first
	// message" regardless. sessionCreateOnce makes the creation lazy and one-time.
	var liveSessionID string
	var sessionCreateOnce sync.Once
	ensureLiveSession := func() string {
		sessionCreateOnce.Do(func() {
			liveSessionTitle := domain.LiveTalkSessionTitlePrefix + " - " + time.Now().Format("Jan 02, 15:04")
			savedLiveSession, err := h.chatService.CreateSession(context.Background(), authUser.UID, liveSessionTitle)
			if err == nil && savedLiveSession != nil {
				liveSessionID = savedLiveSession.ID
			}
		})
		return liveSessionID
	}

	// Live Talk chat history only ever saves Buddy's spoken replies (recordModelTurn
	// below), never the student's own words — an automatic-transcription approach and
	// then a function-call approach were both tried for that and dropped as
	// unreliable/unwanted. The student's speech is still analyzed for durable
	// personal facts worth remembering (chat.Service.ExtractAndPersistMemory), just
	// without being saved as a chat message.
	extractUserMemory := func(text string) {
		trimmed := strings.TrimSpace(text)
		if trimmed != "" {
			go h.chatService.ExtractAndPersistMemory(authUser.UID, trimmed)
		}
	}

	recordModelTurn := func(text string) {
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return
		}
		if sid := ensureLiveSession(); sid != "" {
			_ = h.chatService.SaveLiveMessage(context.Background(), sid, domain.SenderModel, trimmed)
		}
	}

	// The canned opening greeting is spoken/shown to the student (below) but
	// deliberately never passed to recordModelTurn — it's not a real reply, so it
	// must not be what creates the session or count as its "starting message".
	greetingText := "Ayubowan! I am Buddy. I can hear you clearly and see your screen or camera. How can I guide you today?"

	// Send "ready" signal to notify frontend the session is established and listening
	_ = safeWriteJSON(ServerMessage{
		Type:    "ready",
		Message: "Session established and ready",
	})

	// Send initial connected greeting if fallback mode (non-native live)
	if liveSession == nil {
		_ = safeWriteJSON(ServerMessage{
			Type:    "transcript",
			Text:    greetingText,
			IsFinal: true,
		})
	}

	// Background worker to receive native audio/transcripts from Gemini Live session
	if liveSession != nil {
		go func() {
			var modelTurnBuilder strings.Builder
			for {
				serverMsg, recvErr := liveSession.Receive()
				if recvErr != nil {
					log.Printf("[Live Talk] Native receive stopped: %v", recvErr)
					return
				}
				if serverMsg == nil {
					continue
				}

				if serverMsg.ServerContent != nil {
					sc := serverMsg.ServerContent
					if sc.Interrupted {
						_ = safeWriteJSON(ServerMessage{
							Type:        "interrupted",
							Interrupted: true,
						})
					}

					if sc.OutputTranscription != nil && sc.OutputTranscription.Text != "" {
						_ = safeWriteJSON(ServerMessage{
							Type:    "transcript",
							Text:    sc.OutputTranscription.Text,
							IsFinal: false,
						})
						modelTurnBuilder.WriteString(sc.OutputTranscription.Text + " ")
					}

					if sc.ModelTurn != nil {
						for _, part := range sc.ModelTurn.Parts {
							if part.InlineData != nil && len(part.InlineData.Data) > 0 {
								_ = safeWriteJSON(ServerMessage{
									Type:       "audio",
									PCMBase64:  base64.StdEncoding.EncodeToString(part.InlineData.Data),
									SampleRate: 24000,
								})
							}
							if part.Text != "" {
								_ = safeWriteJSON(ServerMessage{
									Type:    "transcript",
									Text:    part.Text,
									IsFinal: false,
								})
								modelTurnBuilder.WriteString(part.Text + " ")
							}
						}
					}

					if sc.TurnComplete {
						_ = safeWriteJSON(ServerMessage{
							Type:    "turn_complete",
							IsFinal: true,
						})
						// The student's turn is never saved as chat history at all (see
						// extractUserMemory above) — this flush only covers Buddy's own
						// spoken reply.
						if modelTurnBuilder.Len() > 0 {
							recordModelTurn(modelTurnBuilder.String())
							modelTurnBuilder.Reset()
						}
					}
				}

				// Handle real-time function calling during live talk
				if serverMsg.ToolCall != nil && len(serverMsg.ToolCall.FunctionCalls) > 0 {
					var responses []*genai.FunctionResponse
					for _, fc := range serverMsg.ToolCall.FunctionCalls {
						if fc == nil {
							continue
						}
						switch fc.Name {
						case "save_student_memory":
							category := ""
							fact := ""
							details := ""
							if v, ok := fc.Args["category"].(string); ok {
								category = v
							}
							if v, ok := fc.Args["fact"].(string); ok {
								fact = v
							}
							if v, ok := fc.Args["details"].(string); ok {
								details = v
							}

							slog.Info(fmt.Sprintf("[Live Talk Tool Call] Recorded memory: [%s] %s", category, fact), slog.String("student_id", authUser.UID))
							if h.chatService != nil {
								// Execute persistence asynchronously so live audio streaming loop is never blocked or paused
								go func(cat, f, det string) {
									_ = h.chatService.SavePersonalMemoryFact(context.Background(), authUser.UID, cat, f, det)
								}(category, fact, details)
							}

							responses = append(responses, &genai.FunctionResponse{
								ID:   fc.ID,
								Name: fc.Name,
								Response: map[string]any{
									"status":  "success",
									"saved":   true,
									"message": "Student personal memory context successfully saved to personal intelligence profile.",
								},
							})

						case "query_foundation_knowledge":
							query := ""
							if v, ok := fc.Args["query"].(string); ok {
								query = v
							}
							resultText := "No specific foundation notes found for this query."
							if h.ragService != nil && strings.TrimSpace(query) != "" {
								ragCtx, ragCancel := context.WithTimeout(context.Background(), 3*time.Second)
								sources, ragErr := h.ragService.RetrieveGroundingSources(ragCtx, query, 3)
								ragCancel()
								if ragErr == nil && len(sources) > 0 {
									var sb strings.Builder
									for _, s := range sources {
										sb.WriteString(fmt.Sprintf("- [%s]: %s\n", s.Title, s.Snippet))
									}
									resultText = sb.String()
								}
							}
							slog.Info("[Live Talk Tool Call] Queried foundation knowledge", slog.String("query", query), slog.String("student_id", authUser.UID))
							responses = append(responses, &genai.FunctionResponse{
								ID:   fc.ID,
								Name: fc.Name,
								Response: map[string]any{
									"query":   query,
									"results": resultText,
								},
							})

						case "recall_student_memory":
							topic := ""
							if v, ok := fc.Args["topic"].(string); ok {
								topic = v
							}
							memResult := "No prior personal memory recorded for this topic."
							if studentMem != nil {
								formatted := studentMem.FormatRelevantContext(topic)
								if formatted != "" {
									memResult = formatted
								}
							}
							if h.chatService != nil {
								if freshMem, err := h.chatService.GetPersonalIntelligence(context.Background(), authUser.UID); err == nil && freshMem != nil {
									studentMem = freshMem
									formatted := freshMem.FormatRelevantContext(topic)
									if formatted != "" {
										memResult = formatted
									}
								}
							}
							slog.Info("[Live Talk Tool Call] Recalled student memory", slog.String("topic", topic), slog.String("student_id", authUser.UID))
							responses = append(responses, &genai.FunctionResponse{
								ID:   fc.ID,
								Name: fc.Name,
								Response: map[string]any{
									"topic":  topic,
									"memory": memResult,
								},
							})
						}
					}

					if len(responses) > 0 {
						if sendErr := liveSession.SendToolResponse(genai.LiveToolResponseInput{
							FunctionResponses: responses,
						}); sendErr != nil {
							log.Printf("[Live Talk] Error sending tool response to liveSession: %v", sendErr)
						}
					}
				}
			}
		}()
	}

	// Buffer latest visual context frame (either webcam or screen share)
	var latestVisionData []byte

	for {
		_, rawMsg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("[Live Talk] WebSocket closed: %v", err)
			}
			break
		}

		var clientMsg ClientMessage
		if err := json.Unmarshal(rawMsg, &clientMsg); err != nil {
			continue
		}

		switch clientMsg.Type {
		case "ping":
			_ = safeWriteJSON(ServerMessage{Type: "pong"})

		case "video_frame":
			// Received snapshot of webcam OR screen share (mutually exclusive)
			if clientMsg.ImageBase64 != "" {
				raw := clientMsg.ImageBase64
				if commaIdx := strings.Index(raw, ","); commaIdx != -1 {
					raw = raw[commaIdx+1:]
				}
				imgBytes, decErr := base64.StdEncoding.DecodeString(raw)
				if decErr == nil {
					latestVisionData = imgBytes
					if liveSession != nil {
						_ = liveSession.SendRealtimeInput(genai.LiveSendRealtimeInputParameters{
							Video: &genai.Blob{
								MIMEType: "image/jpeg",
								Data:     imgBytes,
							},
						})
					}
				}
			}

		case "audio":
			// Client microphone audio chunk (16kHz PCM Little Endian)
			if clientMsg.PCMBase64 != "" {
				audioBytes, decErr := base64.StdEncoding.DecodeString(clientMsg.PCMBase64)
				if decErr == nil && len(audioBytes) > 0 {
					if liveSession != nil {
						_ = liveSession.SendRealtimeInput(genai.LiveSendRealtimeInputParameters{
							Audio: &genai.Blob{
								MIMEType: "audio/pcm;rate=16000",
								Data:     audioBytes,
							},
						})
					}
				}
			}

			// If client also attached recognized text
			if clientMsg.Text != "" && liveSession == nil {
				extractUserMemory(clientMsg.Text)
				go func(prompt string, visionBytes []byte) {
					replyText := h.generateEmpatheticReply(prompt, visionBytes, studentMem)
					recordModelTurn(replyText)
					_ = safeWriteJSON(ServerMessage{
						Type:    "transcript",
						Text:    replyText,
						IsFinal: true,
					})
					_ = safeWriteJSON(ServerMessage{
						Type: "turn_complete",
					})
				}(clientMsg.Text, latestVisionData)
			}

		case "text", "user_speech_turn":
			userPrompt := clientMsg.Text
			if userPrompt == "" {
				userPrompt = "What do you see on my screen or camera?"
			}

			extractUserMemory(userPrompt)

			if liveSession != nil {
				// Search RAG per-turn with the user's actual query for grounded responses
				ragContextPart := ""
				if h.ragService != nil && strings.TrimSpace(userPrompt) != "" {
					ragCtx, ragCancel := context.WithTimeout(context.Background(), 3*time.Second)
					sources, ragErr := h.ragService.RetrieveGroundingSources(ragCtx, userPrompt, 3)
					ragCancel()
					if ragErr == nil && len(sources) > 0 {
						var sb strings.Builder
						sb.WriteString("\n\n[KNOWLEDGE BASE — relevant to this question]:\n")
						for _, s := range sources {
							sb.WriteString(fmt.Sprintf("• %s: %s\n", s.Title, s.Snippet))
						}
						ragContextPart = sb.String()
					}
				}

				turnComplete := true
				turnText := userPrompt
				if ragContextPart != "" {
					turnText = userPrompt + ragContextPart
				}
				_ = liveSession.SendClientContent(genai.LiveSendClientContentParameters{
					Turns: []*genai.Content{
						{
							Role: "user",
							Parts: []*genai.Part{
								genai.NewPartFromText(turnText),
							},
						},
					},
					TurnComplete: &turnComplete,
				})
			} else {
				go func(prompt string, visionBytes []byte) {
					replyText := h.generateEmpatheticReply(prompt, visionBytes, studentMem)
					recordModelTurn(replyText)
					_ = safeWriteJSON(ServerMessage{
						Type:    "transcript",
						Text:    replyText,
						IsFinal: true,
					})
					_ = safeWriteJSON(ServerMessage{
						Type: "turn_complete",
					})
				}(userPrompt, latestVisionData)
			}
		}
	}
}

// generateEmpatheticReply calls Vertex AI Gemini 2.5 with vision and Dr. Tissa Jinasena's voice (fallback when Live Session is inactive)
func (h *Handler) generateEmpatheticReply(userPrompt string, visionBytes []byte, studentMem *domain.StudentPersonalIntelligence) string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var attachments []domain.Attachment
	if len(visionBytes) > 0 {
		attachments = append(attachments, domain.Attachment{
			Name:       "live_feed.jpg",
			MimeType:   "image/jpeg",
			DataBase64: base64.StdEncoding.EncodeToString(visionBytes),
		})
	}

	// Same admin-configured Live Talk prompt as the native path — GenerateChatResponse
	// (called below) already appends its own fixed technical layer (memory tool rules,
	// security constraints), so unlike the native path this doesn't need to append
	// them again itself.
	liveSystemPrompt := domain.DefaultLiveTalkSystemPrompt
	if livePrompt, err := h.chatService.GetActivePrompt(ctx, domain.PromptKindLiveTalk); err == nil && livePrompt != nil && livePrompt.Content != "" {
		liveSystemPrompt = livePrompt.Content
	}

	if h.vertexClient != nil {
		var memoryCallback func(category, fact, details string) error
		if studentMem != nil && studentMem.StudentID != "" && h.chatService != nil {
			memoryCallback = func(category, fact, details string) error {
				return h.chatService.SavePersonalMemoryFact(context.Background(), studentMem.StudentID, category, fact, details)
			}
		}

		resp, err := h.vertexClient.GenerateChatResponse(
			ctx,
			liveSystemPrompt,
			studentMem,
			nil,
			nil,
			userPrompt,
			attachments,
			memoryCallback,
			// No image generation in Live Talk (native or fallback): a spoken-only
			// session has no chat UI to display a generated image in.
			false,
		)
		if err == nil && resp != nil && resp.ReplyText != "" {
			clean := strings.ReplaceAll(resp.ReplyText, "###", "")
			clean = strings.ReplaceAll(clean, "**", "")
			clean = strings.ReplaceAll(clean, "*", "")
			return strings.TrimSpace(clean)
		}
	}

	if len(visionBytes) > 0 {
		return "I can see your display clearly. Let us focus calmly on this problem step-by-step. What specific part would you like us to review first?"
	}
	return "I am listening closely. Dr. Tissa Jinasena always reminded us that steady, calm focus overcomes any hurdle. What is on your mind?"
}
