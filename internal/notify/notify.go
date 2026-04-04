package notify

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:embed templates/email/*.html
var emailTemplateFS embed.FS

type Recipient struct {
	PrincipalID uuid.UUID `json:"principal_id"`
	Email       string    `json:"email"`
	Channels    []string  `json:"channels"`
}

type NotifyEvent struct {
	Type       string         `json:"type"`
	TenantID   uuid.UUID      `json:"tenant_id"`
	CaseID     uuid.UUID      `json:"case_id"`
	CaseNumber string         `json:"case_number"`
	StepID     string         `json:"step_id"`
	StepLabel  string         `json:"step_label"`
	Recipients []Recipient    `json:"recipients"`
	Data       map[string]any `json:"data"`
}

type BrandingColors struct {
	Primary   string `json:"primary"`
	Secondary string `json:"secondary"`
	Accent    string `json:"accent"`
}

type TenantBranding struct {
	CompanyName string         `json:"company_name"`
	LogoURL     string         `json:"logo_url"`
	FaviconURL  string         `json:"favicon_url"`
	Colors      BrandingColors `json:"colors"`
	PoweredBy   bool           `json:"powered_by"`
}

type TenantTerms map[string]string

type EmailData struct {
	CaseNumber  string         `json:"case_number"`
	CaseID      string         `json:"case_id"`
	StepLabel   string         `json:"step_label"`
	SLADeadline string         `json:"sla_deadline"`
	Reason      string         `json:"reason"`
	Branding    TenantBranding `json:"branding"`
	Terminology TenantTerms    `json:"terminology"`
	CaseURL     string         `json:"case_url"`
	AppURL      string         `json:"app_url"`
	Custom      map[string]any `json:"custom"`
}

type EmailSender interface {
	Send(ctx context.Context, to, subject, plainBody, htmlBody string) error
}

type smtpSender struct {
	host     string
	port     string
	username string
	password string
	from     string
}

func (s smtpSender) Send(_ context.Context, to, subject, plainBody, htmlBody string) error {
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("empty recipient email")
	}
	if strings.TrimSpace(s.host) == "" || strings.TrimSpace(s.port) == "" || strings.TrimSpace(s.from) == "" {
		return fmt.Errorf("smtp configuration is incomplete")
	}
	raw, _, err := buildMultipartMessage(s.from, to, subject, plainBody, htmlBody)
	if err != nil {
		return err
	}
	addr := s.host + ":" + s.port
	if strings.EqualFold(s.host, "mock") {
		return nil
	}
	auth := smtp.PlainAuth("", s.username, s.password, s.host)
	return smtp.SendMail(addr, auth, s.from, []string{to}, raw)
}

type Service struct {
	db       *sql.DB
	hub      *Hub
	email    EmailSender
	logger   *log.Logger
	appURL   string
	now      func() time.Time
	sendFunc func(context.Context, uuid.UUID, map[string]any) error
	activeFn func(context.Context, uuid.UUID) (bool, error)
	ctxFn    func(context.Context, uuid.UUID) (TenantBranding, TenantTerms, error)
	async    bool
}

func NewService(db *sql.DB, hub *Hub) *Service {
	s := &Service{
		db:     db,
		hub:    hub,
		email:  smtpSender{host: os.Getenv("ACERYX_SMTP_HOST"), port: os.Getenv("ACERYX_SMTP_PORT"), username: os.Getenv("ACERYX_SMTP_USERNAME"), password: os.Getenv("ACERYX_SMTP_PASSWORD"), from: os.Getenv("ACERYX_SMTP_FROM")},
		logger: log.Default(),
		appURL: firstNonEmpty(os.Getenv("ACERYX_APP_URL"), "http://localhost:5173"),
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	s.async = true
	s.sendFunc = func(ctx context.Context, principalID uuid.UUID, payload map[string]any) error {
		if s.hub == nil {
			return nil
		}
		return s.hub.Send(principalID, payload)
	}
	s.activeFn = s.isPrincipalActive
	s.ctxFn = s.loadTenantContext
	return s
}

func (s *Service) Notify(ctx context.Context, event NotifyEvent) error {
	// System notifications are explicitly best-effort in v1:
	// websocket delivery requires an active connection and email is fire-and-forget.
	// Notification failures are logged and never block the triggering operation.
	if s == nil {
		return nil
	}
	if event.Data == nil {
		event.Data = map[string]any{}
	}
	ctxFn := s.ctxFn
	if ctxFn == nil {
		ctxFn = s.loadTenantContext
	}
	branding, terms, err := ctxFn(ctx, event.TenantID)
	if err != nil {
		s.logger.Printf("notify: load tenant context failed tenant=%s event=%s err=%v", event.TenantID, event.Type, err)
		branding = defaultBranding()
		terms = defaultTerminology()
	}

	seen := map[uuid.UUID]map[string]struct{}{}
	for _, rec := range event.Recipients {
		if rec.PrincipalID != uuid.Nil {
			activeFn := s.activeFn
			if activeFn == nil {
				activeFn = s.isPrincipalActive
			}
			active, aerr := activeFn(ctx, rec.PrincipalID)
			if aerr != nil {
				s.logger.Printf("notify: principal active check failed principal=%s err=%v", rec.PrincipalID, aerr)
				continue
			}
			if !active {
				continue
			}
		}
		for _, channel := range rec.Channels {
			ch := strings.ToLower(strings.TrimSpace(channel))
			if ch == "" {
				continue
			}
			if rec.PrincipalID != uuid.Nil {
				if _, ok := seen[rec.PrincipalID]; !ok {
					seen[rec.PrincipalID] = map[string]struct{}{}
				}
				if _, ok := seen[rec.PrincipalID][ch]; ok {
					continue
				}
				seen[rec.PrincipalID][ch] = struct{}{}
			}

			switch ch {
			case "websocket":
				if rec.PrincipalID == uuid.Nil {
					continue
				}
				msg := s.websocketMessage(event)
				if err := s.sendFunc(ctx, rec.PrincipalID, msg); err != nil {
					s.logger.Printf("notify: websocket send failed principal=%s event=%s err=%v", rec.PrincipalID, event.Type, err)
				}
			case "email":
				if strings.TrimSpace(rec.Email) == "" {
					continue
				}
				subject, templateName, ok := emailTemplateForEvent(event.Type, terms)
				if !ok {
					continue
				}
				emailData := EmailData{
					CaseNumber:  event.CaseNumber,
					CaseID:      event.CaseID.String(),
					StepLabel:   event.StepLabel,
					Branding:    branding,
					Terminology: terms,
					CaseURL:     strings.TrimRight(s.appURL, "/") + "/cases/" + event.CaseID.String(),
					AppURL:      s.appURL,
					Custom:      event.Data,
				}
				if v, ok := event.Data["sla_deadline"].(string); ok {
					emailData.SLADeadline = v
				}
				if v, ok := event.Data["reason"].(string); ok {
					emailData.Reason = v
				}
				htmlBody, plainBody, rerr := renderEmailTemplate(templateName, emailData)
				if rerr != nil {
					s.logger.Printf("notify: render email failed template=%s event=%s err=%v", templateName, event.Type, rerr)
					continue
				}
				sendEmail := func(to string, sub string, plain string, html string) {
					sendCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					if eerr := s.email.Send(sendCtx, to, sub, plain, html); eerr != nil {
						s.logger.Printf("notify: email send failed to=%s event=%s err=%v", to, event.Type, eerr)
					}
				}
				if s.async {
					go sendEmail(rec.Email, subject, plainBody, htmlBody)
				} else {
					sendEmail(rec.Email, subject, plainBody, htmlBody)
				}
			}
		}
	}
	return nil
}

func (s *Service) websocketMessage(event NotifyEvent) map[string]any {
	ts := s.now().Format(time.RFC3339)
	payload := map[string]any{
		"case_id":     event.CaseID.String(),
		"case_number": event.CaseNumber,
		"step_id":     event.StepID,
		"step_label":  event.StepLabel,
	}
	for k, v := range event.Data {
		payload[k] = v
	}
	msgType, action := websocketEventShape(event.Type)
	msg := map[string]any{
		"type":      msgType,
		"payload":   payload,
		"timestamp": ts,
	}
	if action != "" {
		msg["action"] = action
	}
	return msg
}

func websocketEventShape(eventType string) (string, string) {
	switch eventType {
	case "task_assigned":
		return "task_update", "created"
	case "task_claimed":
		return "task_update", "claimed"
	case "task_completed":
		return "task_update", "completed"
	case "task_reassigned":
		return "task_update", "reassigned"
	case "task_escalated":
		return "task_update", "escalated"
	case "sla_warning":
		return "sla_warning", ""
	case "sla_breach":
		return "sla_breach", ""
	case "case_completed":
		return "case_update", "completed"
	case "case_cancelled":
		return "case_update", "cancelled"
	default:
		return "event", eventType
	}
}

func emailTemplateForEvent(eventType string, terms TenantTerms) (subject string, templateName string, ok bool) {
	taskLower := terms.Get("task")
	taskTitle := terms.Get("Task")
	caseLower := terms.Get("case")
	caseTitle := terms.Get("Case")
	switch eventType {
	case "task_assigned":
		return fmt.Sprintf("You have a new %s", taskLower), "task_assigned", true
	case "task_escalated":
		return fmt.Sprintf("%s escalated to you", taskTitle), "task_escalated", true
	case "sla_breach":
		return "SLA breached", "sla_breach", true
	case "case_completed":
		return fmt.Sprintf("%s completed", caseTitle), "case_completed", true
	case "case_cancelled":
		return fmt.Sprintf("%s cancelled", caseTitle), "case_cancelled", true
	default:
		_ = caseLower
		return "", "", false
	}
}

func renderEmailTemplate(name string, data EmailData) (string, string, error) {
	tpl, err := template.ParseFS(emailTemplateFS, "templates/email/_base.html", "templates/email/"+name+".html")
	if err != nil {
		return "", "", fmt.Errorf("parse email template %s: %w", name, err)
	}
	var htmlOut bytes.Buffer
	if err := tpl.Execute(&htmlOut, data); err != nil {
		return "", "", fmt.Errorf("execute email template %s: %w", name, err)
	}
	htmlBody := htmlOut.String()
	return htmlBody, stripHTML(htmlBody), nil
}

func buildMultipartMessage(from string, to string, subject string, plain string, html string) ([]byte, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	boundary := writer.Boundary()
	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=%s\r\n\r\n", from, to, subject, boundary)

	partText, err := writer.CreatePart(mapToMIMEHeader(map[string]string{"Content-Type": "text/plain; charset=UTF-8"}))
	if err != nil {
		return nil, "", err
	}
	_, _ = partText.Write([]byte(plain))

	partHTML, err := writer.CreatePart(mapToMIMEHeader(map[string]string{"Content-Type": "text/html; charset=UTF-8"}))
	if err != nil {
		return nil, "", err
	}
	_, _ = partHTML.Write([]byte(html))
	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	return append([]byte(headers), body.Bytes()...), "multipart/alternative; boundary=" + boundary, nil
}

func mapToMIMEHeader(values map[string]string) textproto.MIMEHeader {
	out := make(textproto.MIMEHeader, len(values))
	for k, v := range values {
		out[k] = []string{v}
	}
	return out
}

func (s *Service) isPrincipalActive(ctx context.Context, principalID uuid.UUID) (bool, error) {
	var active bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM principals
    WHERE id = $1 AND status = 'active'
)
`, principalID).Scan(&active); err != nil {
		return false, err
	}
	return active, nil
}

func (s *Service) loadTenantContext(ctx context.Context, tenantID uuid.UUID) (TenantBranding, TenantTerms, error) {
	var (
		brandingRaw []byte
		termsRaw    []byte
	)
	if err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(branding, '{}'::jsonb), COALESCE(terminology, '{}'::jsonb)
FROM tenants
WHERE id = $1
`, tenantID).Scan(&brandingRaw, &termsRaw); err != nil {
		return TenantBranding{}, nil, err
	}
	branding := defaultBranding()
	if err := json.Unmarshal(brandingRaw, &branding); err != nil {
		return TenantBranding{}, nil, fmt.Errorf("decode tenant branding: %w", err)
	}
	terms := defaultTerminology()
	rawTerms := map[string]string{}
	if err := json.Unmarshal(termsRaw, &rawTerms); err != nil {
		return TenantBranding{}, nil, fmt.Errorf("decode tenant terminology: %w", err)
	}
	for k, v := range rawTerms {
		if strings.TrimSpace(v) != "" {
			terms[k] = v
		}
	}
	ensureTerminologyVariants(terms)
	return branding, terms, nil
}

func defaultBranding() TenantBranding {
	return TenantBranding{
		CompanyName: "Aceryx",
		Colors: BrandingColors{
			Primary:   "#1f6feb",
			Secondary: "#0f172a",
			Accent:    "#f59e0b",
		},
		PoweredBy: true,
	}
}

func defaultTerminology() TenantTerms {
	return TenantTerms{
		"case":  "case",
		"Case":  "Case",
		"cases": "cases",
		"Cases": "Cases",
		"task":  "task",
		"Task":  "Task",
		"tasks": "tasks",
		"Tasks": "Tasks",
		"inbox": "inbox",
		"Inbox": "Inbox",
	}
}

func ensureTerminologyVariants(terms TenantTerms) {
	pairs := [][2]string{{"case", "Case"}, {"cases", "Cases"}, {"task", "Task"}, {"tasks", "Tasks"}, {"inbox", "Inbox"}}
	for _, pair := range pairs {
		lower := strings.TrimSpace(terms[pair[0]])
		upper := strings.TrimSpace(terms[pair[1]])
		if lower == "" && upper == "" {
			continue
		}
		if lower == "" && upper != "" {
			terms[pair[0]] = strings.ToLower(upper)
		}
		if upper == "" && lower != "" {
			terms[pair[1]] = upperFirst(lower)
		}
	}
}

func (t TenantTerms) Get(key string) string {
	if t == nil {
		return key
	}
	if v := strings.TrimSpace(t[key]); v != "" {
		return v
	}
	return key
}

func stripHTML(input string) string {
	replacer := strings.NewReplacer(
		"<br>", "\n",
		"<br/>", "\n",
		"</p>", "\n",
		"<p>", "",
		"</div>", "\n",
		"<div>", "",
		"&nbsp;", " ",
	)
	plain := replacer.Replace(input)
	for strings.Contains(plain, "<") && strings.Contains(plain, ">") {
		start := strings.Index(plain, "<")
		end := strings.Index(plain[start:], ">")
		if start < 0 || end < 0 {
			break
		}
		plain = plain[:start] + plain[start+end+1:]
	}
	return strings.TrimSpace(plain)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func upperFirst(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if len(v) == 1 {
		return strings.ToUpper(v)
	}
	return strings.ToUpper(v[:1]) + v[1:]
}
