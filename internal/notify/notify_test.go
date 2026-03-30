package notify

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type emailSpy struct {
	calls []struct {
		to      string
		subject string
		plain   string
		html    string
	}
	err error
}

func (e *emailSpy) Send(_ context.Context, to, subject, plainBody, htmlBody string) error {
	e.calls = append(e.calls, struct {
		to      string
		subject string
		plain   string
		html    string
	}{to: to, subject: subject, plain: plainBody, html: htmlBody})
	return e.err
}

func TestNotifyDispatchRoutesChannels(t *testing.T) {
	email := &emailSpy{}
	wsCalls := 0
	svc := &Service{
		email:  email,
		logger: testLogger(),
		appURL: "http://app.local",
		now:    fixedNow,
		sendFunc: func(_ context.Context, _ uuid.UUID, payload map[string]any) error {
			wsCalls++
			if payload["type"] != "task_update" || payload["action"] != "created" {
				t.Fatalf("unexpected websocket payload shape: %+v", payload)
			}
			return nil
		},
		activeFn: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		ctxFn: func(_ context.Context, _ uuid.UUID) (TenantBranding, TenantTerms, error) {
			return defaultBranding(), defaultTerminology(), nil
		},
		async: false,
	}

	err := svc.Notify(context.Background(), NotifyEvent{
		Type:       "task_assigned",
		TenantID:   uuid.New(),
		CaseID:     uuid.New(),
		CaseNumber: "CASE-001",
		StepID:     "review",
		StepLabel:  "Review",
		Recipients: []Recipient{{PrincipalID: uuid.New(), Email: "user@example.com", Channels: []string{"websocket", "email"}}},
		Data:       map[string]any{},
	})
	if err != nil {
		t.Fatalf("notify returned error: %v", err)
	}
	if wsCalls != 1 {
		t.Fatalf("expected one websocket dispatch, got %d", wsCalls)
	}
	if len(email.calls) != 1 {
		t.Fatalf("expected one email dispatch, got %d", len(email.calls))
	}
}

func TestNotifyEmailFailureDoesNotFailOperation(t *testing.T) {
	email := &emailSpy{err: errors.New("smtp down")}
	svc := &Service{
		email:    email,
		logger:   testLogger(),
		appURL:   "http://app.local",
		now:      fixedNow,
		sendFunc: func(_ context.Context, _ uuid.UUID, _ map[string]any) error { return nil },
		activeFn: func(_ context.Context, _ uuid.UUID) (bool, error) { return true, nil },
		ctxFn: func(_ context.Context, _ uuid.UUID) (TenantBranding, TenantTerms, error) {
			return defaultBranding(), defaultTerminology(), nil
		},
		async: false,
	}

	if err := svc.Notify(context.Background(), NotifyEvent{
		Type:       "case_completed",
		TenantID:   uuid.New(),
		CaseID:     uuid.New(),
		CaseNumber: "CASE-002",
		Recipients: []Recipient{{PrincipalID: uuid.New(), Email: "user@example.com", Channels: []string{"email"}}},
		Data:       map[string]any{},
	}); err != nil {
		t.Fatalf("expected notify to swallow email errors, got %v", err)
	}
}

func TestNotifySkipsDisabledPrincipal(t *testing.T) {
	email := &emailSpy{}
	wsCalls := 0
	svc := &Service{
		email:  email,
		logger: testLogger(),
		appURL: "http://app.local",
		now:    fixedNow,
		sendFunc: func(_ context.Context, _ uuid.UUID, _ map[string]any) error {
			wsCalls++
			return nil
		},
		activeFn: func(_ context.Context, _ uuid.UUID) (bool, error) { return false, nil },
		ctxFn: func(_ context.Context, _ uuid.UUID) (TenantBranding, TenantTerms, error) {
			return defaultBranding(), defaultTerminology(), nil
		},
		async: false,
	}
	if err := svc.Notify(context.Background(), NotifyEvent{
		Type:       "task_assigned",
		TenantID:   uuid.New(),
		CaseID:     uuid.New(),
		CaseNumber: "CASE-003",
		Recipients: []Recipient{{PrincipalID: uuid.New(), Email: "disabled@example.com", Channels: []string{"email", "websocket"}}},
		Data:       map[string]any{},
	}); err != nil {
		t.Fatalf("notify returned error: %v", err)
	}
	if wsCalls != 0 {
		t.Fatalf("expected no websocket dispatch for disabled principal, got %d", wsCalls)
	}
	if len(email.calls) != 0 {
		t.Fatalf("expected no email dispatch for disabled principal, got %d", len(email.calls))
	}
}

func TestWebsocketMessageFormatJSON(t *testing.T) {
	svc := &Service{now: fixedNow}
	msg := svc.websocketMessage(NotifyEvent{
		Type:       "task_claimed",
		CaseID:     uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		CaseNumber: "CASE-004",
		StepID:     "review",
		StepLabel:  "Review",
		Data:       map[string]any{"priority": 2},
	})
	raw, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal websocket message: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal websocket message: %v", err)
	}
	if decoded["type"] != "task_update" || decoded["action"] != "claimed" {
		t.Fatalf("unexpected websocket top-level fields: %+v", decoded)
	}
}

func TestEmailTemplateRenderingIncludesBrandingTerminologyAndLink(t *testing.T) {
	html, plain, err := renderEmailTemplate("task_assigned", EmailData{
		CaseNumber: "CASE-123",
		CaseID:     uuid.MustParse("11111111-1111-1111-1111-111111111111").String(),
		StepLabel:  "Underwriter Review",
		Branding: TenantBranding{
			CompanyName: "TenantCo",
			LogoURL:     "https://cdn.example.com/logo.png",
			Colors:      BrandingColors{Primary: "#111111", Secondary: "#222222", Accent: "#333333"},
			PoweredBy:   true,
		},
		Terminology: TenantTerms{"task": "task", "Task": "Task", "case": "application", "Case": "Application"},
		CaseURL:     "https://app.example.com/cases/11111111-1111-1111-1111-111111111111",
		AppURL:      "https://app.example.com",
	})
	if err != nil {
		t.Fatalf("render email template: %v", err)
	}
	if !strings.Contains(html, "TenantCo") || !strings.Contains(html, "application") || !strings.Contains(html, "View in TenantCo") {
		t.Fatalf("expected branding/terminology/link in html: %s", html)
	}
	if !strings.Contains(plain, "Underwriter Review") {
		t.Fatalf("expected task label in plain text output: %s", plain)
	}
}

func TestMultipartEmailHasPlainAndHTMLParts(t *testing.T) {
	raw, contentType, err := buildMultipartMessage("from@example.com", "to@example.com", "Subject", "Plain body", "<p>HTML body</p>")
	if err != nil {
		t.Fatalf("build multipart message: %v", err)
	}
	msg := string(raw)
	if !strings.Contains(contentType, "multipart/alternative") {
		t.Fatalf("expected multipart content type, got %s", contentType)
	}
	if !strings.Contains(msg, "Content-Type: text/plain; charset=UTF-8") {
		t.Fatalf("expected plain text part in MIME message: %s", msg)
	}
	if !strings.Contains(msg, "Content-Type: text/html; charset=UTF-8") {
		t.Fatalf("expected html part in MIME message: %s", msg)
	}
}
