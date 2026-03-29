package rbac

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidatePasswordRules(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "too short", in: "Abc123", wantErr: true},
		{name: "no number", in: "PasswordOnly", wantErr: true},
		{name: "no letter", in: "12345678", wantErr: true},
		{name: "valid", in: "Passw0rd", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.in)
			if tt.wantErr && err == nil {
				t.Fatal("expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestJWTGenerationAndVerification(t *testing.T) {
	secret := []byte("test-secret")
	claims := tokenClaims{PrincipalID: uuid.New(), TenantID: uuid.New(), SessionID: uuid.New(), ExpiresAt: time.Now().Add(30 * time.Minute).Unix()}

	token, err := signJWT(secret, claims)
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}
	parsed, err := parseAndVerifyJWT(secret, token, time.Now())
	if err != nil {
		t.Fatalf("parse jwt: %v", err)
	}
	if parsed.PrincipalID != claims.PrincipalID || parsed.TenantID != claims.TenantID || parsed.SessionID != claims.SessionID {
		t.Fatalf("parsed claims mismatch: got %+v want %+v", parsed, claims)
	}

	if _, err := parseAndVerifyJWT([]byte("wrong-secret"), token, time.Now()); err == nil {
		t.Fatal("expected verification failure with wrong secret")
	}
}

func TestSessionTokenGenerationEntropy(t *testing.T) {
	t1, h1, err := generateSessionToken()
	if err != nil {
		t.Fatalf("generate session token 1: %v", err)
	}
	t2, h2, err := generateSessionToken()
	if err != nil {
		t.Fatalf("generate session token 2: %v", err)
	}
	if len(t1) != 64 || len(t2) != 64 {
		t.Fatalf("expected 64-char hex tokens, got %d and %d", len(t1), len(t2))
	}
	if len(h1) != 64 || len(h2) != 64 {
		t.Fatalf("expected 64-char hex hashes, got %d and %d", len(h1), len(h2))
	}
	if t1 == t2 {
		t.Fatal("expected generated session tokens to differ")
	}
	if strings.EqualFold(t1, h1) || strings.EqualFold(t2, h2) {
		t.Fatal("expected hash to differ from plaintext token")
	}
}
