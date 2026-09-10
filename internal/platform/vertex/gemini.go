package vertex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"

	"buddy/server/internal/domain"
)

// GenerateChatResponse generates an empathetic, structured Markdown completion with Gemini.
//
// enableImageGeneration controls whether the "generate_educational_image" tool is
// offered to the model at all (text chat does; Live Talk's fallback path does not,
// since a spoken-only session has nowhere to show an image). This method never
// actually runs image generation itself — it's slow enough (several seconds) that
// doing so would hold up the text reply — it only detects that the model called the
// tool and reports the requested prompt back via StructuredAIResponse.PendingImagePrompt
// for the caller (chat.Service) to generate in the background after the text reply is
// already on its way to the student.
func (c *Client) GenerateChatResponse(
	ctx context.Context,
	systemPrompt string,
	studentMemory *domain.StudentPersonalIntelligence,
	ragSources []domain.RAGSource,
	history []*domain.ChatMessage,
	userMessage string,
	attachments []domain.Attachment,
	onMemoryToolCall func(category, fact, details string) error,
	enableImageGeneration bool,
) (*domain.StructuredAIResponse, error) {
	imageToolsEnabled := enableImageGeneration

	// 1. Compose System Instructions
	var systemInstructions strings.Builder
	systemInstructions.WriteString(systemPrompt)
	systemInstructions.WriteString("\n\n")

	systemInstructions.WriteString(`### FORMATTING & COMMUNICATION DIRECTIVE:
- Respond in beautifully structured, clean, and clear **GitHub-flavored Markdown**.
- Use descriptive headings (e.g. ### 💡 Mindful Reflection, ### 🎯 Practical Action Steps, ### 📖 Key Concept).
- Use bullet points, bold emphasis, and numbered lists for readability.
- **TABLES**: Always format comparative or structured data into proper GitHub-Flavored Markdown tables using vertical pipes and divider rows:
  | Header 1 | Header 2 | Header 3 |
  | :--- | :--- | :--- |
  | Value 1 | Value 2 | Value 3 |
  NEVER output tab-separated columns or raw unformatted tabbed text.
- **MATHEMATICAL & SCIENTIFIC FORMULAS**: Use standard LaTeX syntax:
  - Inline formulas/units: e.g. $V = I \times R$, $\Omega$, $230\text{ V}$, $50\text{ Hz}$.
  - Standalone equations: use double dollar signs:
    $$
    V = I \times R
    $$
- When explaining programming code, use formatted syntax code blocks with the language identifier (e.g. ` + "```python" + `).
- Be warm, empathetic, respectful, and direct. Avoid overwhelming the student with wall-of-text paragraphs.
- If the student expresses stress, anxiety, or emotional burden, start with calming, reassuring empathy inspired by Dr. Tissa Jinasena before offering practical solutions.
- Do NOT wrap your entire response inside a JSON code block. Output rich Markdown text directly.`)
	systemInstructions.WriteString("\n\n")
	systemInstructions.WriteString(MemoryToolRulesBlock)
	systemInstructions.WriteString("\n\n")
	if imageToolsEnabled {
		systemInstructions.WriteString(EducationalImageToolRulesBlock)
		systemInstructions.WriteString("\n\n")
	}
	systemInstructions.WriteString(SecurityConstraintsBlock)
	systemInstructions.WriteString("\n\n")

	if studentMemory != nil {
		systemInstructions.WriteString(studentMemory.FormatRelevantContext(userMessage))
		systemInstructions.WriteString("\n\n")
	}

	if len(ragSources) > 0 {
		systemInstructions.WriteString(FormatRAGContext(ragSources))
		systemInstructions.WriteString("\n\n")
	}

	tools := []*genai.Tool{MemoryToolDeclaration()}
	if imageTool := ImageGenerationToolDeclaration(enableImageGeneration); imageTool != nil {
		tools = append(tools, imageTool)
	}

	temp := float32(0.7)
	cfg := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: systemInstructions.String()},
			},
		},
		Temperature: &temp,
		Tools:       tools,
	}

	// 2. Build Multi-turn Conversation Contents
	var contents []*genai.Content

	// Add recent history messages (sliding window of last 8 messages)
	startIdx := 0
	if len(history) > 8 {
		startIdx = len(history) - 8
	}

	for _, h := range history[startIdx:] {
		role := "user"
		if h.Sender == domain.SenderModel {
			role = "model"
		}
		contents = append(contents, &genai.Content{
			Role: role,
			Parts: []*genai.Part{
				{Text: h.Content},
			},
		})
	}

	// 3. Append Current User Turn
	var currentParts []*genai.Part
	for _, att := range attachments {
		if att.DataBase64 != "" {
			rawB64 := att.DataBase64
			if commaIdx := strings.Index(rawB64, ","); commaIdx != -1 {
				rawB64 = rawB64[commaIdx+1:]
			}

			data, err := base64.StdEncoding.DecodeString(rawB64)
			if err == nil {
				currentParts = append(currentParts, &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: att.MimeType,
						Data:     data,
					},
				})
			}
		}
	}

	queryText := userMessage
	if strings.TrimSpace(queryText) == "" && len(currentParts) > 0 {
		// A voice-only (or attachment-only) send has no typed text at all — tell the
		// model explicitly what it's looking at instead of handing it an empty
		// <student_query>, which otherwise reads like the student sent nothing.
		queryText = "(The student sent this message as a voice recording / attachment with no typed text. Listen to or examine the attached content and respond to it directly.)"
	}
	currentParts = append(currentParts, &genai.Part{
		Text: fmt.Sprintf("<student_query>\n%s\n</student_query>", queryText),
	})

	contents = append(contents, &genai.Content{
		Role:  "user",
		Parts: currentParts,
	})

	// 4. Call Vertex AI via Google GenAI SDK (Single-Turn with Tool Call Support)
	var markdownReply string
	var pendingImagePrompt string

	for turn := 0; turn < 2; turn++ {
		result, err := c.client.Models.GenerateContent(ctx, c.modelName, contents, cfg)
		if err != nil {
			return nil, fmt.Errorf("vertex ai generation failed: %w", err)
		}

		if len(result.Candidates) == 0 || result.Candidates[0].Content == nil {
			break
		}

		candidateContent := result.Candidates[0].Content
		var functionCalls []*genai.FunctionCall
		var candidateText string

		for _, part := range candidateContent.Parts {
			if part.FunctionCall != nil {
				functionCalls = append(functionCalls, part.FunctionCall)
			}
			if part.Text != "" {
				candidateText += part.Text
			}
		}

		// Handle each tool call. save_student_memory is fire-and-forget (nothing
		// about its result needs to reach the model or the reply), but the image
		// tools run synchronously and their results feed straight into this same
		// turn's function-response parts — see the doc comment on this method for why.
		var respParts []*genai.Part
		for _, fc := range functionCalls {
			switch fc.Name {
			case "save_student_memory":
				if onMemoryToolCall == nil {
					continue
				}
				category, _ := fc.Args["category"].(string)
				fact, _ := fc.Args["fact"].(string)
				details, _ := fc.Args["details"].(string)
				go func(cat, f, det string) {
					_ = onMemoryToolCall(cat, f, det)
				}(category, fact, details)
				respParts = append(respParts, functionResponsePart(fc, map[string]any{
					"status": "success", "saved": true, "message": "Student personal memory context saved.",
				}))

			case "generate_educational_image":
				if !enableImageGeneration {
					continue
				}
				prompt, _ := fc.Args["prompt"].(string)
				if strings.TrimSpace(prompt) == "" {
					continue
				}
				// Just record the request — actually generating happens in the
				// background after this method returns (see the doc comment above).
				// The response handed back to Gemini here is an immediate
				// acknowledgment, not a report of a real result, since no image has
				// been generated yet at this point.
				pendingImagePrompt = prompt
				respParts = append(respParts, functionResponsePart(fc, map[string]any{
					"status": "success", "message": "Image generation started; it will appear shortly after your reply.",
				}))
			}
		}

		// If candidate text was returned in this same response, use it immediately (single-turn)!
		if strings.TrimSpace(candidateText) != "" {
			markdownReply = candidateText
			break
		}

		// Fallback only if Gemini returned ONLY function calls without conversational text
		if len(functionCalls) > 0 && turn == 0 && len(respParts) > 0 {
			contents = append(contents, candidateContent)
			contents = append(contents, &genai.Content{
				Role:  "user",
				Parts: respParts,
			})
			continue
		}
		break
	}

	if strings.TrimSpace(markdownReply) == "" {
		markdownReply = "I am listening attentively. How can I help you today?"
	}

	// Clean any citation remnants or trailing hr dividers
	markdownReply = stripReferenceSections(markdownReply)

	return &domain.StructuredAIResponse{
		ReplyText:          markdownReply,
		EmotionalTone:      "Empathetic, Wise & Supportive",
		Sources:            ragSources,
		PendingImagePrompt: pendingImagePrompt,
	}, nil
}

// functionResponsePart builds the genai.Part Gemini expects back for a given function
// call, carrying its call ID through so the model can match the response to the right
// invocation.
func functionResponsePart(fc *genai.FunctionCall, response map[string]any) *genai.Part {
	part := genai.NewPartFromFunctionResponse(fc.Name, response)
	if fc.ID != "" && part.FunctionResponse != nil {
		part.FunctionResponse.ID = fc.ID
	}
	return part
}

// GenerateSessionTitle calls Gemini to generate a concise, natural 2-5 word session title.
func (c *Client) GenerateSessionTitle(ctx context.Context, userPrompt string) (string, error) {
	if c == nil || c.client == nil || strings.TrimSpace(userPrompt) == "" {
		return "", fmt.Errorf("vertex client or prompt unavailable")
	}

	titlePrompt := fmt.Sprintf(`You are an AI assistant generating titles for conversation sessions.
Generate a short, succinct, natural title (strictly 2 to 5 words, no quotation marks, no ending punctuation) that summarizes the student's initial prompt.
If the prompt is in Sinhala, provide a clean Sinhala or English title.
Do NOT output any markdown tags or prefixes. Output ONLY the plain title text.

Student Initial Prompt:
%s`, userPrompt)

	temp := float32(0.2)
	cfg := &genai.GenerateContentConfig{
		Temperature: &temp,
	}

	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{Text: titlePrompt},
			},
		},
	}

	result, err := c.client.Models.GenerateContent(ctx, c.modelName, contents, cfg)
	if err != nil {
		return "", err
	}

	for _, cand := range result.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if part.Text != "" {
					title := strings.TrimSpace(part.Text)
					title = strings.Trim(title, "\"`*#_")
					title = strings.Trim(title, "\n\r")
					if title != "" {
						return title, nil
					}
				}
			}
		}
	}
	return "", fmt.Errorf("no title candidate returned")
}

func stripReferenceSections(text string) string {
	lines := strings.Split(text, "\n")
	var cleaned []string
	skipSection := false
	for _, l := range lines {
		trimmed := strings.TrimSpace(strings.ToLower(l))
		if strings.HasPrefix(trimmed, "### references") || strings.HasPrefix(trimmed, "## references") ||
			strings.HasPrefix(trimmed, "### 📚 references") || strings.HasPrefix(trimmed, "**references:**") ||
			strings.HasPrefix(trimmed, "**📚 knowledge grounding") || strings.HasPrefix(trimmed, "### sources") ||
			strings.HasPrefix(trimmed, "## sources") {
			skipSection = true
			continue
		}
		if skipSection {
			if strings.HasPrefix(trimmed, "#") {
				skipSection = false
				cleaned = append(cleaned, l)
			}
			continue
		}
		cleaned = append(cleaned, l)
	}
	result := strings.TrimSpace(strings.Join(cleaned, "\n"))
	for {
		trimmed := strings.TrimSpace(result)
		if strings.HasSuffix(trimmed, "---") || strings.HasSuffix(trimmed, "***") || strings.HasSuffix(trimmed, "___") {
			result = strings.TrimSuffix(trimmed, "---")
			result = strings.TrimSuffix(result, "***")
			result = strings.TrimSuffix(result, "___")
			continue
		}
		break
	}
	return strings.TrimSpace(result)
}

// ExtractedPersonalData models student facts detected during conversations.
type ExtractedPersonalData struct {
	HasNewFacts      bool     `json:"has_new_facts"`
	Background       string   `json:"background,omitempty"`
	FamilyContext    string   `json:"family_context,omitempty"`
	Ambitions        string   `json:"ambitions,omitempty"`
	Strengths        []string `json:"strengths,omitempty"`
	Weaknesses       []string `json:"weaknesses,omitempty"`
	MedicalNotes     string   `json:"medical_notes,omitempty"`
	SpiritualBeliefs string   `json:"spiritual_beliefs,omitempty"`
	ExtractedFacts   []string `json:"extracted_facts,omitempty"`
}

// ExtractPersonalFacts analyzes student messages with Gemini to passively capture bio data, family context, medications, and ambitions.
func (c *Client) ExtractPersonalFacts(ctx context.Context, userMessage string) (*ExtractedPersonalData, error) {
	prompt := fmt.Sprintf(`You are an intelligent, discreet memory extraction system for Buddy AI, an academic and life mentor inspired by Dr. Tissa Jinasena.
Analyze the following student message and determine if the student explicitly shared any personal facts, background information, family context, medical/health details, ambitions, strengths, or weaknesses.

Student Message:
"%s"

Rules:
1. If the message is strictly academic, greeting, or contains NO personal details about the student, return:
{"has_new_facts": false}
2. If the student reveals personal information (e.g. mentions their family members, parental jobs, health conditions or medications, career goals or ambitions, hobbies, struggles, etc.), extract and summarize them concisely in English:
{
  "has_new_facts": true,
  "background": "Brief bio/location/interests or empty string",
  "family_context": "Family members/home situation or empty string",
  "ambitions": "Aspirations/career dreams or empty string",
  "strengths": ["list of identified strengths"],
  "weaknesses": ["list of identified academic or emotional struggles"],
  "medical_notes": "Medications/health issues or empty string",
  "spiritual_beliefs": "Values/beliefs or empty string",
  "extracted_facts": ["concise factual statement 1", "concise factual statement 2"]
}

Output ONLY valid JSON. No markdown ticks, no commentary.`, userMessage)

	temp := float32(0.1)
	cfg := &genai.GenerateContentConfig{
		Temperature: &temp,
	}

	contents := []*genai.Content{
		{
			Role: "user",
			Parts: []*genai.Part{
				{Text: prompt},
			},
		},
	}

	result, err := c.client.Models.GenerateContent(ctx, c.modelName, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("memory extraction failed: %w", err)
	}

	var rawJSON string
	for _, cand := range result.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				rawJSON += part.Text
			}
		}
	}

	rawJSON = strings.TrimSpace(rawJSON)
	rawJSON = strings.TrimPrefix(rawJSON, "```json")
	rawJSON = strings.TrimPrefix(rawJSON, "```")
	rawJSON = strings.TrimSuffix(rawJSON, "```")
	rawJSON = strings.TrimSpace(rawJSON)

	var data ExtractedPersonalData
	if err := json.Unmarshal([]byte(rawJSON), &data); err != nil {
		return nil, fmt.Errorf("failed to parse memory json: %w", err)
	}

	return &data, nil
}

// GeneratedImage is one image produced by GenerateEducationalImage.
type GeneratedImage struct {
	Data     []byte
	MimeType string
}

// GenerateEducationalImage produces an original illustration/diagram via Gemini's
// image generation model, for the "generate_educational_image" chat tool — the
// fallback used only when Google Image Search wouldn't already have a suitable real
// photo (a novel diagram, a specific custom illustration, a conceptual visualization
// that doesn't exist as a real photo). Uses a separate model from ordinary chat
// (VERTEX_IMAGE_MODEL, see client.go) since image generation is a distinct capability
// not every Gemini model supports.
func (c *Client) GenerateEducationalImage(ctx context.Context, prompt string) (*GeneratedImage, error) {
	if c == nil || c.imageClient == nil {
		return nil, fmt.Errorf("vertex client unavailable")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, fmt.Errorf("prompt is required")
	}

	// Defense in depth: the caller (chat.Service, gated by the calling model's own
	// EducationalImageToolRulesBlock instructions) should already only reach this with
	// an academic request, but this constraint is restated directly to the image
	// generation model itself too, since it makes the actual generation call.
	fullPrompt := fmt.Sprintf(`Generate a clear, accurate STRICTLY ACADEMIC/EDUCATIONAL illustration for a student: %s

This must be genuine educational content only — a textbook-style diagram, scientific/technical illustration, or conceptual visualization tied to academic or vocational learning. Refuse (by generating nothing resembling the request) if the request is not academic in nature: entertainment, memes, decorative art, or a depiction of a real, identifiable person.

Style: clean textbook-diagram quality, appropriate for an academic/vocational training context. Label parts only where labels are essential to understanding. No decorative text, watermarks, or logos.`, prompt)

	cfg := &genai.GenerateContentConfig{
		ResponseModalities: []string{string(genai.ModalityText), string(genai.ModalityImage)},
	}

	contents := []*genai.Content{
		{
			Role:  "user",
			Parts: []*genai.Part{{Text: fullPrompt}},
		},
	}

	// model=%q logged on every error below: the single most common cause of a silent
	// "stuck generating" failure is VERTEX_IMAGE_MODEL naming a model that doesn't
	// exist, isn't enabled, or isn't available in this client's region (see
	// VERTEX_IMAGE_LOCATION in client.go) — that comes back as an ordinary API error
	// from GenerateContent, not something distinguishable in Go without inspecting the
	// message, so surfacing the exact model/error together is what makes it diagnosable.
	result, err := c.imageClient.Models.GenerateContent(ctx, c.imageModelName, contents, cfg)
	if err != nil {
		return nil, fmt.Errorf("image generation request failed (model=%q): %w", c.imageModelName, err)
	}

	if result.PromptFeedback != nil && result.PromptFeedback.BlockReason != "" {
		return nil, fmt.Errorf("image generation blocked before generating (model=%q, reason=%s): %s",
			c.imageModelName, result.PromptFeedback.BlockReason, result.PromptFeedback.BlockReasonMessage)
	}
	if len(result.Candidates) == 0 || result.Candidates[0].Content == nil {
		return nil, fmt.Errorf("image generation returned no candidates (model=%q)", c.imageModelName)
	}

	candidate := result.Candidates[0]
	var responseText strings.Builder
	for _, part := range candidate.Content.Parts {
		if part.InlineData != nil && len(part.InlineData.Data) > 0 {
			mimeType := part.InlineData.MIMEType
			if mimeType == "" {
				mimeType = "image/png"
			}
			return &GeneratedImage{Data: part.InlineData.Data, MimeType: mimeType}, nil
		}
		if part.Text != "" {
			responseText.WriteString(part.Text)
		}
	}

	// No image part came back. This usually means the model declined (safety
	// filtering, or it judged the request non-academic per our own prompt
	// instruction) and explained why in its text instead — surface that reason
	// rather than a bare "no image" if we have it.
	if responseText.Len() > 0 {
		return nil, fmt.Errorf("model did not return an image (model=%q, finish_reason=%s): %s",
			c.imageModelName, candidate.FinishReason, strings.TrimSpace(responseText.String()))
	}
	return nil, fmt.Errorf("model did not return an image (model=%q, finish_reason=%s, finish_message=%s)",
		c.imageModelName, candidate.FinishReason, candidate.FinishMessage)
}
