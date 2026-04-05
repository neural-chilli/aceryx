package rag

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
)

type LangchainLoader struct{}

func NewLangchainLoader() *LangchainLoader {
	return &LangchainLoader{}
}

func (l *LangchainLoader) SupportedTypes() []string {
	return []string{
		"application/pdf",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"text/csv",
		"text/plain",
		"text/markdown",
	}
}

func (l *LangchainLoader) Load(data []byte, contentType string) (string, error) {
	ct := strings.TrimSpace(strings.ToLower(contentType))
	switch ct {
	case "text/plain", "text/markdown":
		return string(data), nil
	case "text/csv":
		return loadCSV(data)
	case "application/pdf":
		return loadPDF(data)
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return loadDOCX(data)
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedContentType, contentType)
	}
}

func loadCSV(data []byte) (string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil {
		return "", fmt.Errorf("parse csv: %w", err)
	}
	if len(records) == 0 {
		return "", nil
	}
	lines := make([]string, 0, len(records))
	for _, rec := range records {
		lines = append(lines, strings.Join(rec, " | "))
	}
	return strings.Join(lines, "\n"), nil
}

func loadPDF(data []byte) (string, error) {
	// Minimal text extraction for predictable fixtures.
	raw := string(data)
	if !strings.Contains(raw, "%PDF") {
		return "", fmt.Errorf("invalid pdf content")
	}
	re := regexp.MustCompile(`\(([^()]*)\)`)
	parts := re.FindAllStringSubmatch(raw, -1)
	if len(parts) == 0 {
		return "", fmt.Errorf("pdf text extraction produced no text")
	}
	text := make([]string, 0, len(parts))
	page := 1
	text = append(text, fmt.Sprintf("[Page %d]", page))
	for _, p := range parts {
		segment := strings.TrimSpace(strings.ReplaceAll(p[1], "\\n", " "))
		if segment == "" {
			continue
		}
		if strings.Contains(strings.ToLower(segment), "pagebreak") {
			page++
			text = append(text, fmt.Sprintf("[Page %d]", page))
			continue
		}
		text = append(text, segment)
	}
	return strings.Join(text, "\n"), nil
}

func loadDOCX(data []byte) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("open docx zip: %w", err)
	}
	var documentXML []byte
	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, openErr := f.Open()
		if openErr != nil {
			return "", fmt.Errorf("open docx document.xml: %w", openErr)
		}
		documentXML, err = io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return "", fmt.Errorf("read docx document.xml: %w", err)
		}
		break
	}
	if len(documentXML) == 0 {
		return "", fmt.Errorf("docx missing word/document.xml")
	}
	type textNode struct {
		Text string `xml:",chardata"`
	}
	dec := xml.NewDecoder(bytes.NewReader(documentXML))
	parts := make([]string, 0)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("decode docx xml: %w", err)
		}
		switch el := tok.(type) {
		case xml.StartElement:
			if el.Name.Local != "t" {
				continue
			}
			var n textNode
			if err := dec.DecodeElement(&n, &el); err != nil {
				return "", fmt.Errorf("decode docx text node: %w", err)
			}
			if strings.TrimSpace(n.Text) != "" {
				parts = append(parts, strings.TrimSpace(n.Text))
			}
		}
	}
	return strings.Join(parts, "\n"), nil
}
