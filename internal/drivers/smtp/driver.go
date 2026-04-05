package smtp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"mime/quotedprintable"
	"net/smtp"
	"strings"

	"github.com/neural-chilli/aceryx/internal/drivers"
)

type Driver struct{}

func New() *Driver { return &Driver{} }

func (d *Driver) ID() string          { return "smtp" }
func (d *Driver) DisplayName() string { return "SMTP" }

func (d *Driver) Send(ctx context.Context, config drivers.SMTPConfig, msg drivers.EmailMessage) error {
	_ = ctx
	if len(msg.To) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}
	if config.Host == "" || config.Port == 0 {
		return fmt.Errorf("smtp host and port are required")
	}
	if config.From == "" {
		return fmt.Errorf("smtp from is required")
	}
	payload, err := buildMessage(config.From, msg)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	auth := smtp.PlainAuth("", config.Username, config.Password, config.Host)
	if config.TLS {
		conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return fmt.Errorf("smtp tls dial: %w", err)
		}
		client, err := smtp.NewClient(conn, config.Host)
		if err != nil {
			return fmt.Errorf("smtp new client: %w", err)
		}
		defer func() { _ = client.Close() }()
		if config.Username != "" {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
		if err := sendWithClient(client, config.From, recipients(msg), payload); err != nil {
			return err
		}
		return nil
	}
	if err := smtp.SendMail(addr, auth, config.From, recipients(msg), payload); err != nil {
		return fmt.Errorf("smtp send mail: %w", err)
	}
	return nil
}

func sendWithClient(client *smtp.Client, from string, rcpts []string, payload []byte) error {
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, rcpt := range rcpts {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", rcpt, err)
		}
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := wc.Write(payload); err != nil {
		_ = wc.Close()
		return fmt.Errorf("smtp write body: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("smtp finalize body: %w", err)
	}
	return nil
}

func buildMessage(from string, msg drivers.EmailMessage) ([]byte, error) {
	buf := &bytes.Buffer{}
	mixed := multipart.NewWriter(buf)
	altBoundary := "aceryx-alt"

	headers := []string{
		"MIME-Version: 1.0",
		"From: " + from,
		"To: " + strings.Join(msg.To, ", "),
		"Subject: " + msg.Subject,
		"Content-Type: multipart/mixed; boundary=" + mixed.Boundary(),
	}
	if len(msg.CC) > 0 {
		headers = append(headers, "Cc: "+strings.Join(msg.CC, ", "))
	}
	for _, h := range headers {
		_, _ = buf.WriteString(h + "\r\n")
	}
	_, _ = buf.WriteString("\r\n")

	part, err := mixed.CreatePart(mapHeader(map[string]string{
		"Content-Type": "multipart/alternative; boundary=" + altBoundary,
	}))
	if err != nil {
		return nil, fmt.Errorf("create alternative part: %w", err)
	}

	if msg.BodyText != "" {
		if _, err := writeQuotedPrintablePart(part, altBoundary, "text/plain; charset=utf-8", msg.BodyText); err != nil {
			return nil, err
		}
	}
	if msg.BodyHTML != "" {
		if _, err := writeQuotedPrintablePart(part, altBoundary, "text/html; charset=utf-8", msg.BodyHTML); err != nil {
			return nil, err
		}
	}
	_, _ = part.Write([]byte("\r\n--" + altBoundary + "--\r\n"))

	for _, a := range msg.Attachments {
		contentType := a.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		ap, err := mixed.CreatePart(mapHeader(map[string]string{
			"Content-Type":              contentType,
			"Content-Disposition":       fmt.Sprintf("attachment; filename=%q", a.Filename),
			"Content-Transfer-Encoding": "base64",
		}))
		if err != nil {
			return nil, fmt.Errorf("create attachment part: %w", err)
		}
		encoded := make([]byte, base64.StdEncoding.EncodedLen(len(a.Data)))
		base64.StdEncoding.Encode(encoded, a.Data)
		for len(encoded) > 76 {
			_, _ = ap.Write(encoded[:76])
			_, _ = ap.Write([]byte("\r\n"))
			encoded = encoded[76:]
		}
		if len(encoded) > 0 {
			_, _ = ap.Write(encoded)
			_, _ = ap.Write([]byte("\r\n"))
		}
	}

	if err := mixed.Close(); err != nil {
		return nil, fmt.Errorf("finalize mime message: %w", err)
	}
	return buf.Bytes(), nil
}

func writeQuotedPrintablePart(w ioWriter, boundary, contentType, body string) (int, error) {
	header := fmt.Sprintf("--%s\r\nContent-Type: %s\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", boundary, contentType)
	n, err := w.Write([]byte(header))
	if err != nil {
		return n, fmt.Errorf("write mime header: %w", err)
	}
	qp := quotedprintable.NewWriter(w)
	m, err := qp.Write([]byte(body))
	if err != nil {
		return n + m, fmt.Errorf("write quoted-printable body: %w", err)
	}
	if err := qp.Close(); err != nil {
		return n + m, fmt.Errorf("finalize quoted-printable body: %w", err)
	}
	k, err := w.Write([]byte("\r\n"))
	return n + m + k, err
}

type ioWriter interface {
	Write(p []byte) (n int, err error)
}

func mapHeader(values map[string]string) map[string][]string {
	headers := map[string][]string{}
	for k, v := range values {
		headers[k] = []string{v}
	}
	return headers
}

func recipients(msg drivers.EmailMessage) []string {
	out := make([]string, 0, len(msg.To)+len(msg.CC))
	out = append(out, msg.To...)
	out = append(out, msg.CC...)
	return out
}
