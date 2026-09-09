package chat

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"buddy/server/internal/domain"
	platformVertex "buddy/server/internal/platform/vertex"
)

// ExtractAndPersistMemory analyzes student messages asynchronously in the background
// to extract bio data, family dynamics, medications, ambitions, and personal context.
func (s *Service) ExtractAndPersistMemory(studentID, userMessage string) {
	if strings.TrimSpace(userMessage) == "" || studentID == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var extracted *platformVertex.ExtractedPersonalData
	var err error

	if s.vertexClient != nil {
		extracted, err = s.vertexClient.ExtractPersonalFacts(ctx, userMessage)
		if err != nil {
			slog.Warn("[Memory Extractor] Vertex AI extraction error", slog.String("student_id", studentID), slog.String("error", err.Error()))
		}
	}

	// Fallback heuristic extraction if vertex is unavailable or offline
	if extracted == nil || !extracted.HasNewFacts {
		extracted = heuristicFactExtraction(userMessage)
	}

	if extracted == nil || !extracted.HasNewFacts {
		return
	}

	// Fetch current memory
	mem, err := s.repo.GetPersonalIntelligence(ctx, studentID)
	if err != nil || mem == nil {
		mem = &domain.StudentPersonalIntelligence{
			StudentID:      studentID,
			Strengths:      make([]string, 0),
			Weaknesses:     make([]string, 0),
			ExtractedFacts: make([]string, 0),
		}
	}

	// Merge non-destructively
	if extracted.Background != "" {
		if mem.Background == "" {
			mem.Background = extracted.Background
		} else if !strings.Contains(mem.Background, extracted.Background) {
			mem.Background = mem.Background + "; " + extracted.Background
		}
	}

	if extracted.FamilyContext != "" {
		if mem.FamilyContext == "" {
			mem.FamilyContext = extracted.FamilyContext
		} else if !strings.Contains(mem.FamilyContext, extracted.FamilyContext) {
			mem.FamilyContext = mem.FamilyContext + "; " + extracted.FamilyContext
		}
	}

	if extracted.Ambitions != "" {
		if mem.Ambitions == "" {
			mem.Ambitions = extracted.Ambitions
		} else if !strings.Contains(mem.Ambitions, extracted.Ambitions) {
			mem.Ambitions = mem.Ambitions + "; " + extracted.Ambitions
		}
	}

	if extracted.MedicalNotes != "" {
		if mem.MedicalNotes == "" {
			mem.MedicalNotes = extracted.MedicalNotes
		} else if !strings.Contains(mem.MedicalNotes, extracted.MedicalNotes) {
			mem.MedicalNotes = mem.MedicalNotes + "; " + extracted.MedicalNotes
		}
	}

	if extracted.SpiritualBeliefs != "" {
		if mem.SpiritualBeliefs == "" {
			mem.SpiritualBeliefs = extracted.SpiritualBeliefs
		} else if !strings.Contains(mem.SpiritualBeliefs, extracted.SpiritualBeliefs) {
			mem.SpiritualBeliefs = mem.SpiritualBeliefs + "; " + extracted.SpiritualBeliefs
		}
	}

	// Merge slices with deduplication
	mem.Strengths = mergeUniqueStrings(mem.Strengths, extracted.Strengths)
	mem.Weaknesses = mergeUniqueStrings(mem.Weaknesses, extracted.Weaknesses)
	mem.ExtractedFacts = mergeUniqueStrings(mem.ExtractedFacts, extracted.ExtractedFacts)
	mem.UpdatedAt = time.Now()

	if err := s.repo.SavePersonalIntelligence(ctx, mem); err != nil {
		slog.Error("[Memory Extractor] Failed to save updated memory", slog.String("student_id", studentID), slog.String("error", err.Error()))
	} else {
		slog.Info(fmt.Sprintf("[Memory Extractor] Updated memory profile (%d total facts)", len(mem.ExtractedFacts)), slog.String("student_id", studentID))
	}
}

// heuristicFactExtraction provides rule-based memory detection for offline/test environments.
func heuristicFactExtraction(msg string) *platformVertex.ExtractedPersonalData {
	lower := strings.ToLower(msg)
	data := &platformVertex.ExtractedPersonalData{}

	// Family detection
	if strings.Contains(lower, "my father") || strings.Contains(lower, "my mother") ||
		strings.Contains(lower, "my parents") || strings.Contains(lower, "my brother") ||
		strings.Contains(lower, "my sister") {
		data.HasNewFacts = true
		data.FamilyContext = strings.TrimSpace(msg)
		data.ExtractedFacts = append(data.ExtractedFacts, "Family mention: "+strings.TrimSpace(msg))
	}

	// Medical/Health/Meds detection
	if strings.Contains(lower, "meds") || strings.Contains(lower, "medicine") ||
		strings.Contains(lower, "migraine") || strings.Contains(lower, "allergy") ||
		strings.Contains(lower, "doctor") || strings.Contains(lower, "asthma") ||
		strings.Contains(lower, "adhd") || strings.Contains(lower, "headache") {
		data.HasNewFacts = true
		data.MedicalNotes = strings.TrimSpace(msg)
		data.ExtractedFacts = append(data.ExtractedFacts, "Health/Medication note: "+strings.TrimSpace(msg))
	}

	// Ambitions/Goals detection
	if strings.Contains(lower, "i want to become") || strings.Contains(lower, "my dream is") ||
		strings.Contains(lower, "my ambition") || strings.Contains(lower, "i want to study") ||
		strings.Contains(lower, "my goal is") {
		data.HasNewFacts = true
		data.Ambitions = strings.TrimSpace(msg)
		data.ExtractedFacts = append(data.ExtractedFacts, "Aspiration: "+strings.TrimSpace(msg))
	}

	return data
}

func mergeUniqueStrings(existing, newItems []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range existing {
		clean := strings.TrimSpace(item)
		if clean != "" && !seen[strings.ToLower(clean)] {
			seen[strings.ToLower(clean)] = true
			result = append(result, clean)
		}
	}
	for _, item := range newItems {
		clean := strings.TrimSpace(item)
		if clean != "" && !seen[strings.ToLower(clean)] {
			seen[strings.ToLower(clean)] = true
			result = append(result, clean)
		}
	}
	return result
}
