package imap

import "testing"

func TestIMAPDriverMetadata(t *testing.T) {
	d := New()
	if d.ID() != "imap" {
		t.Fatalf("unexpected id: %s", d.ID())
	}
	if d.DisplayName() == "" {
		t.Fatal("display name should not be empty")
	}
}
