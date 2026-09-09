package domain

import (
	"fmt"
	"strings"
	"time"
)

// StudentPersonalIntelligence represents the longitudinal memory context of a student.
type StudentPersonalIntelligence struct {
	StudentID        string    `json:"student_id" firestore:"student_id"`
	Background       string    `json:"background" firestore:"background"`               // Cultural/regional background, personality traits, interests
	FamilyContext    string    `json:"family_context" firestore:"family_context"`       // Family dynamics, living situation, guardian relationship
	Ambitions        string    `json:"ambitions" firestore:"ambitions"`                 // Career aspirations, short/long-term goals, dreams
	Strengths        []string  `json:"strengths" firestore:"strengths"`                 // Academic & emotional strengths, talents
	Weaknesses       []string  `json:"weaknesses" firestore:"weaknesses"`               // Areas needing guidance, subject difficulties
	MedicalNotes     string    `json:"medical_notes" firestore:"medical_notes"`         // Health/wellness notes, special accessibility needs, allergies
	SpiritualBeliefs string    `json:"spiritual_beliefs" firestore:"spiritual_beliefs"` // Spiritual, moral, or philosophical preferences
	ExtractedFacts   []string  `json:"extracted_facts" firestore:"extracted_facts"`     // Dynamic facts learned from conversations
	UpdatedAt        time.Time `json:"updated_at" firestore:"updated_at"`
}

// FormatContext converts the personal intelligence into a structured prompt injection snippet.
// Formats and filters student facts dynamically (like RAG retrieval) to ground the response.
func (m *StudentPersonalIntelligence) FormatContext() string {
	return m.FormatRelevantContext("")
}

// FormatRelevantContext retrieves and highlights memory facts most relevant to the student query.
func (m *StudentPersonalIntelligence) FormatRelevantContext(query string) string {
	if m == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### 🧠 STUDENT PERSONAL MEMORY (Retrieved Context - discreetly adapt tone and advice):\n")
	hasAny := false

	if m.Background != "" {
		sb.WriteString(fmt.Sprintf("- Bio/Background: %s\n", m.Background))
		hasAny = true
	}
	if m.FamilyContext != "" {
		sb.WriteString(fmt.Sprintf("- Family Context: %s\n", m.FamilyContext))
		hasAny = true
	}
	if m.Ambitions != "" {
		sb.WriteString(fmt.Sprintf("- Ambitions & Career Goals: %s\n", m.Ambitions))
		hasAny = true
	}
	if len(m.Strengths) > 0 {
		sb.WriteString(fmt.Sprintf("- Strengths & Talents: %s\n", strings.Join(m.Strengths, ", ")))
		hasAny = true
	}
	if len(m.Weaknesses) > 0 {
		sb.WriteString(fmt.Sprintf("- Academic/Focus Challenges: %s\n", strings.Join(m.Weaknesses, ", ")))
		hasAny = true
	}
	if m.MedicalNotes != "" {
		sb.WriteString(fmt.Sprintf("- Health & Medications (Crucial - be gentle and mindful): %s\n", m.MedicalNotes))
		hasAny = true
	}
	if m.SpiritualBeliefs != "" {
		sb.WriteString(fmt.Sprintf("- Moral & Value System: %s\n", m.SpiritualBeliefs))
		hasAny = true
	}

	// Filter and prioritize extracted facts matching query terms (RAG-like dynamic memory)
	if len(m.ExtractedFacts) > 0 {
		var prioritized []string
		var others []string
		queryTerms := strings.Fields(strings.ToLower(query))

		for _, fact := range m.ExtractedFacts {
			lowerFact := strings.ToLower(fact)
			matched := false
			for _, term := range queryTerms {
				if len(term) > 3 && strings.Contains(lowerFact, term) {
					matched = true
					break
				}
			}
			if matched {
				prioritized = append(prioritized, "⭐ "+fact)
			} else {
				others = append(others, fact)
			}
		}

		allFacts := append(prioritized, others...)
		// Keep up to top 15 most relevant facts
		if len(allFacts) > 15 {
			allFacts = allFacts[:15]
		}
		sb.WriteString(fmt.Sprintf("- Key Memory Facts: %s\n", strings.Join(allFacts, " | ")))
		hasAny = true
	}

	if !hasAny {
		return ""
	}

	return sb.String()
}

// UpdateMemoryRequest is the DTO payload to update student personal intelligence.
type UpdateMemoryRequest struct {
	Background       *string   `json:"background,omitempty"`
	FamilyContext    *string   `json:"family_context,omitempty"`
	Ambitions        *string   `json:"ambitions,omitempty"`
	Strengths        *[]string `json:"strengths,omitempty"`
	Weaknesses       *[]string `json:"weaknesses,omitempty"`
	MedicalNotes     *string   `json:"medical_notes,omitempty"`
	SpiritualBeliefs *string   `json:"spiritual_beliefs,omitempty"`
	ExtractedFacts   *[]string `json:"extracted_facts,omitempty"`
}

// DeleteFactRequest is the payload to delete a memory observation.
type DeleteFactRequest struct {
	Index int    `json:"index"`
	Fact  string `json:"fact"`
}

