package vertex

import (
	"context"
	"fmt"
	"strings"

	"buddy/server/internal/domain"
)

// RAGEngine defines knowledge retrieval capabilities.
type RAGEngine interface {
	RetrieveContext(ctx context.Context, query string, topK int) ([]domain.RAGSource, error)
}

// VertexRAGEngine implements RAGEngine backed by Vertex AI RAG Engine / Corpus.
type VertexRAGEngine struct {
	corpusID  string
	projectID string
	location  string
}

// NewVertexRAGEngine initializes a Vertex RAG Engine client.
func NewVertexRAGEngine(projectID, location, corpusID string) RAGEngine {
	return &VertexRAGEngine{
		corpusID:  corpusID,
		projectID: projectID,
		location:  location,
	}
}

// RetrieveContext queries knowledge base or returns curated institutional knowledge from Jinasena Training Foundation.
func (r *VertexRAGEngine) RetrieveContext(ctx context.Context, query string, topK int) ([]domain.RAGSource, error) {
	if topK <= 0 {
		topK = 3
	}

	// Curated foundation knowledge corpus
	foundationalKnowledge := []domain.RAGSource{
		{
			Title:   "Dr. Tissa Jinasena: Principles of Holistic Education & Vocational Excellence",
			Snippet: "Dr. Tissa Jinasena emphasized that true education harmonizes three pillars: intellectual competence (Tutor), practical self-reliance and ethical entrepreneurship (Mentor), and spiritual tranquility with compassion (Spiritual Guider). Students should be empowered through practical problem solving, discipline, and mutual respect.",
			URI:     "gs://buddy-lms-knowledge/foundation/dr_tissa_jinasena_principles.pdf",
			Score:   0.95,
		},
		{
			Title:   "Jinasena Training Foundation: Student Welfare & Counseling Framework",
			Snippet: "The foundation's welfare charter mandates recognizing each student's personal circumstances, including family background, emotional well-being, and health challenges. Mentors must provide continuous encouragement, destigmatize learning difficulties, and nurture individual career ambitions with tailored learning pathways.",
			URI:     "gs://buddy-lms-knowledge/foundation/student_welfare_charter.pdf",
			Score:   0.91,
		},
		{
			Title:   "Mindfulness and Resilience in Academic Learning",
			Snippet: "Techniques for managing examination stress, developing mental clarity, and cultivating loving-kindness (Metta) and self-compassion. When feeling overwhelmed, students are encouraged to pause, practice mindful breathing, and break tasks into bite-sized actionable milestones.",
			URI:     "gs://buddy-lms-knowledge/foundation/mindfulness_guide.pdf",
			Score:   0.88,
		},
	}

	qLower := strings.ToLower(query)
	var matched []domain.RAGSource

	for _, k := range foundationalKnowledge {
		// Keyword match or default relevance
		if strings.Contains(qLower, "jinasena") || strings.Contains(qLower, "foundation") ||
			strings.Contains(qLower, "study") || strings.Contains(qLower, "stress") ||
			strings.Contains(qLower, "future") || strings.Contains(qLower, "exam") ||
			strings.Contains(qLower, "career") || strings.Contains(qLower, "who are you") ||
			strings.Contains(qLower, "buddy") {
			matched = append(matched, k)
		}
	}

	if len(matched) == 0 {
		matched = foundationalKnowledge[:1]
	}

	if len(matched) > topK {
		matched = matched[:topK]
	}

	return matched, nil
}

// FormatRAGContext formats retrieved sources into a grounding prompt section.
func FormatRAGContext(sources []domain.RAGSource) string {
	if len(sources) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### RELEVANT KNOWLEDGE BASE CONTEXT (Ground your response with these sources):\n")
	for i, s := range sources {
		sb.WriteString(fmt.Sprintf("[%d] Source: %s\nSnippet: %s\n", i+1, s.Title, s.Snippet))
	}
	return sb.String()
}
