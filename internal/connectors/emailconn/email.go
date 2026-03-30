package emailconn

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
	"strings"

	"github.com/neural-chilli/aceryx/internal/connectors"
)

//go:embed templates/*.html
var templateFS embed.FS

type Connector struct{}

func New() *Connector { return &Connector{} }

func (c *Connector) Meta() connectors.ConnectorMeta {
	return connectors.ConnectorMeta{Key: "email", Name: "Email", Description: "SMTP email connector", Version: "v1", Icon: "pi pi-envelope"}
}

func (c *Connector) Auth() connectors.AuthSpec {
	return connectors.AuthSpec{
		Type: "basic",
		Fields: []connectors.AuthField{
			{Key: "smtp_host", Label: "SMTP Host", Type: "string", Required: true},
			{Key: "smtp_port", Label: "SMTP Port", Type: "string", Required: true},
			{Key: "username", Label: "Username", Type: "string", Required: true},
			{Key: "password", Label: "Password", Type: "password", Required: true},
			{Key: "from", Label: "From", Type: "string", Required: true},
		},
	}
}

func (c *Connector) Triggers() []connectors.TriggerSpec { return nil }

func (c *Connector) Actions() []connectors.ActionSpec {
	return []connectors.ActionSpec{
		{
			Key:          "send",
			Name:         "Send",
			Description:  "Send templated email",
			InputSchema:  map[string]any{"type": "object"},
			OutputSchema: map[string]any{"type": "object"},
			Execute:      c.send,
		},
	}
}

func (c *Connector) send(ctx context.Context, auth map[string]string, input map[string]any) (map[string]any, error) {
	_ = ctx
	to := asString(input["to"])
	subject := asString(input["subject"])
	templateName := asString(input["template"])
	if templateName == "" {
		templateName = "task_assigned"
	}
	if to == "" {
		return nil, fmt.Errorf("to is required")
	}

	data := map[string]any{}
	if raw, ok := input["template_data"].(map[string]any); ok {
		for k, v := range raw {
			data[k] = v
		}
	}
	if _, ok := data["CompanyName"]; !ok {
		data["CompanyName"] = asString(input["company_name"])
	}
	if _, ok := data["BrandPrimary"]; !ok {
		data["BrandPrimary"] = asString(input["brand_primary"])
		if asString(data["BrandPrimary"]) == "" {
			data["BrandPrimary"] = "#1f6feb"
		}
	}
	if _, ok := data["LogoURL"]; !ok {
		data["LogoURL"] = asString(input["logo_url"])
	}
	if _, ok := data["CaseNumber"]; !ok {
		data["CaseNumber"] = asString(input["case_number"])
	}
	if _, ok := data["Body"]; !ok {
		data["Body"] = asString(input["body"])
	}

	htmlBody, err := renderTemplate(templateName, data)
	if err != nil {
		return nil, err
	}
	plain := stripHTML(htmlBody)
	message, contentType, err := buildMultipartMessage(auth["from"], to, subject, plain, htmlBody)
	if err != nil {
		return nil, err
	}

	host := auth["smtp_host"]
	port := auth["smtp_port"]
	addr := host + ":" + port
	if strings.EqualFold(host, "mock") {
		return map[string]any{"to": to, "subject": subject, "content_type": contentType, "mime": string(message)}, nil
	}
	smtpAuth := smtp.PlainAuth("", auth["username"], auth["password"], host)
	if err := smtp.SendMail(addr, smtpAuth, auth["from"], []string{to}, message); err != nil {
		return nil, err
	}
	return map[string]any{"to": to, "subject": subject, "content_type": contentType}, nil
}

func renderTemplate(name string, data map[string]any) (string, error) {
	files := []string{
		"templates/_base.html",
		"templates/" + name + ".html",
	}
	tpl, err := template.ParseFS(templateFS, files...)
	if err != nil {
		return "", fmt.Errorf("parse email template %s: %w", name, err)
	}
	var out bytes.Buffer
	if err := tpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("execute email template %s: %w", name, err)
	}
	return out.String(), nil
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
	_ = writer.Close()

	return append([]byte(headers), body.Bytes()...), "multipart/alternative; boundary=" + boundary, nil
}

func mapToMIMEHeader(values map[string]string) textproto.MIMEHeader {
	out := make(textproto.MIMEHeader, len(values))
	for k, v := range values {
		out[k] = []string{v}
	}
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func stripHTML(input string) string {
	replacer := strings.NewReplacer("<br>", "\n", "<br/>", "\n", "</p>", "\n", "<p>", "", "<strong>", "", "</strong>", "")
	plain := replacer.Replace(input)
	plain = strings.ReplaceAll(plain, "<", "")
	plain = strings.ReplaceAll(plain, ">", "")
	return plain
}
