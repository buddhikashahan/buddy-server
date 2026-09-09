package rag

import (
	"strings"
)

// ChunkText splits a raw document into chunks of target word size with overlap.
func ChunkText(text string, targetWordSize, overlapWords int) []string {
	cleanText := strings.TrimSpace(text)
	if cleanText == "" {
		return nil
	}

	words := strings.Fields(cleanText)
	if len(words) <= targetWordSize {
		return []string{cleanText}
	}

	if targetWordSize <= 0 {
		targetWordSize = 250
	}
	if overlapWords <= 0 || overlapWords >= targetWordSize {
		overlapWords = 40
	}

	var chunks []string
	step := targetWordSize - overlapWords

	for i := 0; i < len(words); i += step {
		end := i + targetWordSize
		if end > len(words) {
			end = len(words)
		}

		chunkWords := words[i:end]
		chunks = append(chunks, strings.Join(chunkWords, " "))

		if end == len(words) {
			break
		}
	}

	return chunks
}
