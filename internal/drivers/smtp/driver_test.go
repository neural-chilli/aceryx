package smtp

import (
	"strings"
	"testing"

	"github.com/neural-chilli/aceryx/internal/drivers"
)

func TestBuildMessage(t *testing.T) {
	raw, err := buildMessage("noreply@example.com", drivers.EmailMessage{
		To:       []string{"a@example.com"},
		Subject:  "Subject",
		BodyText: "Hello",
	})
	if err != nil {
		t.Fatalf("buildMessage: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "Subject: Subject") {
		t.Fatalf("missing subject in message: %s", text)
	}
	if !strings.Contains(text, "Hello") {
		t.Fatalf("missing body in message: %s", text)
	}
}
