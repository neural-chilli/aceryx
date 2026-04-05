package rag

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoaderPDF(t *testing.T) {
	loader := NewLangchainLoader()
	pdf, err := os.ReadFile(filepath.Join("..", "..", "testdata", "test-document.pdf"))
	if err != nil {
		t.Fatalf("read test pdf: %v", err)
	}
	out, err := loader.Load(pdf, "application/pdf")
	if err != nil {
		t.Fatalf("load pdf: %v", err)
	}
	if !strings.Contains(out, "[Page 1]") || !strings.Contains(out, "[Page 2]") {
		t.Fatalf("expected page markers in extracted text, got %q", out)
	}
}

func TestLoaderPlainText(t *testing.T) {
	loader := NewLangchainLoader()
	out, err := loader.Load([]byte("hello world"), "text/plain")
	if err != nil {
		t.Fatalf("load text: %v", err)
	}
	if out != "hello world" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestLoaderUnsupported(t *testing.T) {
	loader := NewLangchainLoader()
	_, err := loader.Load([]byte("img"), "image/png")
	if !errors.Is(err, ErrUnsupportedContentType) {
		t.Fatalf("expected ErrUnsupportedContentType, got %v", err)
	}
}
