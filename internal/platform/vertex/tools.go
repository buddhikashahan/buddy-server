package vertex

import (
	"google.golang.org/genai"
)

// MemoryToolDeclaration returns the genai.Tool declaration for recording student personal intelligence.
func MemoryToolDeclaration() *genai.Tool {
	return &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "save_student_memory",
				Description: "Automatically save and record NEW, unique personal facts, background details, family context, " +
					"living conditions, health notes, medications, ambitions, hobbies, or food/lifestyle preferences " +
					"shared by the student in ANY language (Sinhala, English, Tamil, etc.). " +
					"STRICT RULES: " +
					"1. ONLY save genuine enduring personal attributes (e.g. name, age, family size, likes/dislikes). " +
					"2. NEVER save conversational questions, test prompts, or meta-statements (e.g. 'Student asked if I know...', 'Student asked what I remember', 'Student confirmed...'). " +
					"3. NEVER save information that is already present in the student's personal memory context above.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"category": {
							Type: genai.TypeString,
							Description: "The category of personal information: " +
								"'background' (name, age, hometown, hobbies, general bio), " +
								"'family' (parents, siblings, count of family members, home dynamics), " +
								"'ambitions' (career dreams, goals, aspirations), " +
								"'strengths' (talents, capabilities, positive attributes), " +
								"'weaknesses' (academic difficulties, stress triggers, areas needing help), " +
								"'medical' (medications, illness, allergies, physical/mental wellness), " +
								"'spiritual' (values, ethics, spiritual traditions), " +
								"or 'fact' (general specific detail).",
							Enum: []string{
								"background",
								"family",
								"ambitions",
								"strengths",
								"weaknesses",
								"medical",
								"spiritual",
								"fact",
							},
						},
						"fact": {
							Type: genai.TypeString,
							Description: "A clear, concise statement summarizing the personal fact learned about the student " +
								"(e.g. 'Student is named Buddhika and is 23 years old', 'Family has 6 members', 'Aspires to be a mechanical engineer').",
						},
						"details": {
							Type:        genai.TypeString,
							Description: "Detailed context or the student's original statement supporting this memory.",
						},
					},
					Required: []string{"category", "fact"},
				},
			},
		},
	}
}

// LiveTalkToolsDeclaration returns the full tool declaration for real-time Gemini Live sessions:
// 1. save_student_memory: saves new persistent facts
// 2. query_foundation_knowledge: retrieves Foundation syllabus, course notes, and engineering guidelines from RAG
// 3. recall_student_memory: retrieves specific background or past facts about the student on-demand
//
// There is deliberately no tool for logging the student's own transcript: Live Talk
// chat history only ever saves Buddy's spoken replies (see recordModelTurn in
// internal/streaming/handler.go), not the student's side of the conversation. An
// earlier version tried both an automatic-transcription approach and a function-call
// approach for that, and both were dropped as unreliable/unwanted.
func LiveTalkToolsDeclaration() *genai.Tool {
	return &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "save_student_memory",
				Description: "Automatically save and record NEW, unique personal facts, background details, family context, " +
					"living conditions, health notes, medications, ambitions, hobbies, or lifestyle preferences " +
					"shared by the student in ANY language. ONLY save genuine enduring personal attributes. " +
					"NEVER save temporary conversation states or meta statements.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"category": {
							Type:        genai.TypeString,
							Description: "The category: 'background', 'family', 'ambitions', 'strengths', 'weaknesses', 'medical', 'spiritual', or 'fact'.",
							Enum: []string{
								"background", "family", "ambitions", "strengths", "weaknesses", "medical", "spiritual", "fact",
							},
						},
						"fact": {
							Type:        genai.TypeString,
							Description: "A clear, concise statement summarizing the personal fact learned about the student.",
						},
						"details": {
							Type:        genai.TypeString,
							Description: "Detailed context or the student's original statement.",
						},
					},
					Required: []string{"category", "fact"},
				},
			},
			{
				Name:        "query_foundation_knowledge",
				Description: "Search the Dr. Tissa Jinasena Foundation knowledge base, course notes, engineering syllabus, guidelines, and study library when the student asks technical questions or questions about foundation courses and principles.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"query": {
							Type:        genai.TypeString,
							Description: "The search query to look up in the foundation library and course notes.",
						},
					},
					Required: []string{"query"},
				},
			},
			{
				Name:        "recall_student_memory",
				Description: "Retrieve past facts, background, family context, ambitions, or details about this student from their personal record ONLY when relevant to what the student is asking about or discussing.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"topic": {
							Type:        genai.TypeString,
							Description: "The topic or specific detail to recall (e.g. 'family', 'goals', 'hometown', 'medical', 'general').",
						},
					},
					Required: []string{"topic"},
				},
			},
		},
	}
}

// ImageGenerationToolDeclaration returns the tool declaration that lets Buddy generate
// an original illustration/diagram with Gemini image generation to help a student
// visualize an academic concept — the platform's only source of chat visuals; there is
// deliberately no image search tool (removed after evaluation). Returns nil when
// include is false, so callers can omit it from Tools entirely (e.g. Live Talk, which
// has no chat UI to display an image in) rather than send an empty declaration.
func ImageGenerationToolDeclaration(include bool) *genai.Tool {
	if !include {
		return nil
	}
	return &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{
			{
				Name: "generate_educational_image",
				Description: "Generate an original illustration or diagram with Gemini image generation, for " +
					"STRICTLY ACADEMIC/EDUCATIONAL purposes only — see the EDUCATIONAL IMAGE GENERATION rules in " +
					"your instructions for exactly what qualifies. Never call this for entertainment, personal, " +
					"decorative, or otherwise non-academic image requests.",
				Parameters: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"prompt": {
							Type: genai.TypeString,
							Description: "A clear, specific description of the educational illustration to generate, " +
								"including what should be labeled or emphasized.",
						},
					},
					Required: []string{"prompt"},
				},
			},
		},
	}
}
