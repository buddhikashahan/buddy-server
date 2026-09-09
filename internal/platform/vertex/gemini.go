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
func (c *Client) GenerateChatResponse(
	ctx context.Context,
	systemPrompt string,
	studentMemory *domain.StudentPersonalIntelligence,
	ragSources []domain.RAGSource,
	history []*domain.ChatMessage,
	userMessage string,
	attachments []domain.Attachment,
	onMemoryToolCall func(category, fact, details string) error,
) (*domain.StructuredAIResponse, error) {
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

	temp := float32(0.7)
	cfg := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: systemInstructions.String()},
			},
		},
		Temperature: &temp,
		Tools:       []*genai.Tool{MemoryToolDeclaration()},
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

	currentParts = append(currentParts, &genai.Part{
		Text: fmt.Sprintf("<student_query>\n%s\n</student_query>", userMessage),
	})

	contents = append(contents, &genai.Content{
		Role:  "user",
		Parts: currentParts,
	})

	// 4. Call Vertex AI via Google GenAI SDK (Single-Turn with Tool Call Support)
	var markdownReply string

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

		// Asynchronously trigger memory saving if tool call was generated
		for _, fc := range functionCalls {
			if fc.Name == "save_student_memory" && onMemoryToolCall != nil {
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
				go func(cat, f, det string) {
					_ = onMemoryToolCall(cat, f, det)
				}(category, fact, details)
			}
		}

		// If candidate text was returned in this same response, use it immediately (single-turn)!
		if strings.TrimSpace(candidateText) != "" {
			markdownReply = candidateText
			break
		}

		// Fallback only if Gemini returned ONLY function calls without conversational text
		if len(functionCalls) > 0 && turn == 0 {
			contents = append(contents, candidateContent)
			var respParts []*genai.Part
			for _, fc := range functionCalls {
				respPart := genai.NewPartFromFunctionResponse(fc.Name, map[string]any{
					"status":  "success",
					"saved":   true,
					"message": "Student personal memory context saved.",
				})
				if fc.ID != "" && respPart.FunctionResponse != nil {
					respPart.FunctionResponse.ID = fc.ID
				}
				respParts = append(respParts, respPart)
			}
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
		ReplyText:     markdownReply,
		EmotionalTone: "Empathetic, Wise & Supportive",
		Sources:       ragSources,
	}, nil
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
