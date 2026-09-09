package rag

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
)

// ExtractText extracts plain text from PDF, Word (.docx), Markdown, or Text files.
func ExtractText(filename string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))

	switch ext {
	case ".docx":
		return extractDocx(data)
	case ".pdf":
		return extractPDF(data)
	case ".txt", ".md", ".csv":
		return string(data), nil
	default:
		// Attempt plain UTF-8 interpretation if unrecognized
		if len(data) > 0 {
			return string(data), nil
		}
		return "", fmt.Errorf("unsupported file format: %s", ext)
	}
}

// extractDocx reads word/document.xml inside the DOCX zip container.
func extractDocx(data []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("invalid docx zip archive: %w", err)
	}

	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			docFile = f
			break
		}
	}

	if docFile == nil {
		return "", errors.New("invalid docx: word/document.xml not found")
	}

	rc, err := docFile.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open word/document.xml: %w", err)
	}
	defer rc.Close()

	decoder := xml.NewDecoder(rc)
	var sb strings.Builder
	inText := false

	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}

		switch elem := tok.(type) {
		case xml.StartElement:
			// <w:p> indicates a new paragraph
			if elem.Name.Local == "p" {
				sb.WriteString("\n")
			} else if elem.Name.Local == "t" { // <w:t> indicates text node
				inText = true
			}
		case xml.EndElement:
			if elem.Name.Local == "t" {
				inText = false
			}
		case xml.CharData:
			if inText {
				sb.WriteString(string(elem))
			}
		}
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		return "", errors.New("no readable text found in docx")
	}

	return result, nil
}

// extractPDF parses text blocks and uncompressed/flate streams from a PDF.
func extractPDF(data []byte) (string, error) {
	if len(data) < 4 || string(data[:4]) != "%PDF" {
		return "", errors.New("invalid pdf: missing %PDF header")
	}

	var sb strings.Builder

	// 1. Scan for stream ... endstream blocks
	streamRegex := regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	matches := streamRegex.FindAllSubmatch(data, -1)

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		streamData := m[1]

		// Attempt FlateDecode decompression
		zr, err := zlib.NewReader(bytes.NewReader(streamData))
		var decompressed []byte
		if err == nil {
			decompressed, _ = io.ReadAll(zr)
			_ = zr.Close()
		} else {
			decompressed = streamData
		}

		// Extract text from (text) Tj / [(text)] TJ
		parsed := extractPDFTextTokens(decompressed)
		if len(parsed) > 0 {
			sb.WriteString(parsed)
			sb.WriteString("\n")
		}
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		// Fallback: search for parenthesized text anywhere in the binary
		parenRegex := regexp.MustCompile(`\(([A-Za-z0-9 .,!?:;'\-]{3,})\)`)
		pMatches := parenRegex.FindAllSubmatch(data, -1)
		for _, pm := range pMatches {
			if len(pm) > 1 {
				sb.WriteString(string(pm[1]))
				sb.WriteString(" ")
			}
		}
		result = strings.TrimSpace(sb.String())
	}

	if result == "" {
		return "", errors.New("could not extract text from pdf (file may be scanned/image-only or encrypted)")
	}

	return result, nil
}

// extractPDFTextTokens decodes text tokens inside a PDF content stream.
func extractPDFTextTokens(stream []byte) string {
	var sb strings.Builder
	inBT := false

	lines := strings.Split(string(stream), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "BT" {
			inBT = true
			continue
		} else if trimmed == "ET" {
			inBT = false
			sb.WriteString("\n")
			continue
		}

		if inBT || strings.HasSuffix(trimmed, "Tj") || strings.HasSuffix(trimmed, "TJ") {
			// Extract strings enclosed in parentheses: (Hello World) Tj
			inParen := false
			var current strings.Builder
			for i := 0; i < len(trimmed); i++ {
				c := trimmed[i]
				if c == '(' && (i == 0 || trimmed[i-1] != '\\') {
					inParen = true
					current.Reset()
				} else if c == ')' && (i == 0 || trimmed[i-1] != '\\') {
					inParen = false
					if current.Len() > 0 {
						sb.WriteString(current.String())
						sb.WriteString(" ")
					}
				} else if inParen {
					current.WriteByte(c)
				}
			}
		}
	}

	return strings.TrimSpace(sb.String())
}
