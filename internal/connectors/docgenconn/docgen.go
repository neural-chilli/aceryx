package docgenconn

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
	"github.com/neural-chilli/aceryx/internal/vault"
)

type Connector struct {
	db    *sql.DB
	vault vault.VaultStore
}

func New(db *sql.DB, vaultStore vault.VaultStore) *Connector {
	return &Connector{db: db, vault: vaultStore}
}

func (c *Connector) Meta() connectors.ConnectorMeta {
	return connectors.ConnectorMeta{Key: "docgen", Name: "Document Generator", Description: "Generate branded PDFs from declarative templates", Version: "v1", Icon: "pi pi-file-pdf"}
}

func (c *Connector) Auth() connectors.AuthSpec { return connectors.AuthSpec{Type: "none"} }

func (c *Connector) Triggers() []connectors.TriggerSpec { return nil }

func (c *Connector) Actions() []connectors.ActionSpec {
	return []connectors.ActionSpec{
		{
			Key:          "generate_pdf",
			Name:         "Generate PDF",
			Description:  "Generate PDF and store in vault",
			InputSchema:  map[string]any{"type": "object"},
			OutputSchema: map[string]any{"type": "object"},
			Execute:      c.generatePDF,
		},
	}
}

type documentTemplate struct {
	Name     string                   `json:"name"`
	Filename string                   `json:"filename"`
	Layout   []map[string]interface{} `json:"layout"`
}

func (c *Connector) generatePDF(ctx context.Context, _ map[string]string, input map[string]any) (map[string]any, error) {
	templateName, _ := input["template"].(string)
	if templateName == "" {
		return nil, fmt.Errorf("template is required")
	}

	tpl, err := c.loadTemplate(ctx, templateName)
	if err != nil {
		return nil, err
	}

	renderedFilename := connectors.ResolveTemplateString(tpl.Filename, input)
	if renderedFilename == "" {
		renderedFilename = "document-" + uuid.NewString() + ".pdf"
	}
	if !strings.HasSuffix(strings.ToLower(renderedFilename), ".pdf") {
		renderedFilename += ".pdf"
	}

	lines := make([]string, 0, len(tpl.Layout))
	for _, elem := range tpl.Layout {
		t := strings.ToLower(asString(elem["type"]))
		switch t {
		case "header":
			lines = append(lines, "HEADER: "+c.resolveText(elem["company"], input)+" | "+c.resolveText(elem["date"], input))
		case "paragraph":
			lines = append(lines, c.resolveText(elem["text"], input))
		case "table":
			lines = append(lines, "TABLE:")
			if rows, ok := elem["rows"].([]interface{}); ok {
				for _, row := range rows {
					if pair, ok := row.([]interface{}); ok {
						col := make([]string, 0, len(pair))
						for _, v := range pair {
							col = append(col, c.resolveText(v, input))
						}
						lines = append(lines, strings.Join(col, " | "))
					}
				}
			}
		case "signature":
			lines = append(lines, "SIGNATURE: "+c.resolveText(elem["name"], input))
		case "footer":
			lines = append(lines, "FOOTER: "+c.resolveText(elem["text"], input))
		case "divider":
			lines = append(lines, "----------------------------------------")
		case "spacer":
			lines = append(lines, "")
		}
	}

	pdfBytes := buildSimplePDF(lines)
	hash := sha256.Sum256(pdfBytes)
	contentHash := hex.EncodeToString(hash[:])
	sizeBytes := int64(len(pdfBytes))

	tenantID, _ := uuid.Parse(asString(input["_tenant_id"]))
	caseID, _ := uuid.Parse(asString(input["_case_id"]))
	stepID := asString(input["_step_id"])
	uploaderID, _ := uuid.Parse(asString(input["_actor_id"]))
	if uploaderID == uuid.Nil {
		uploaderID = c.resolveFallbackUploader(ctx, tenantID)
	}

	storageURI := "memory://" + contentHash + ".pdf"
	if c.vault != nil {
		uri, err := c.vault.Put(tenantID.String(), contentHash, "pdf", pdfBytes)
		if err == nil && uri != "" {
			storageURI = uri
		}
	}

	var documentID uuid.UUID
	err = c.db.QueryRowContext(ctx, `
INSERT INTO vault_documents (tenant_id, case_id, step_id, filename, mime_type, size_bytes, content_hash, storage_uri, uploaded_by, metadata)
VALUES ($1, $2, $3, $4, 'application/pdf', $5, $6, $7, $8, $9::jsonb)
RETURNING id
`, tenantID, caseID, stepID, renderedFilename, sizeBytes, contentHash, storageURI, uploaderID, `{"connector":"docgen"}`).Scan(&documentID)
	if err != nil {
		return nil, fmt.Errorf("insert vault document metadata: %w", err)
	}

	return map[string]any{
		"document_id":  documentID.String(),
		"filename":     renderedFilename,
		"size_bytes":   sizeBytes,
		"content_hash": contentHash,
		"pdf_bytes":    pdfBytes,
	}, nil
}

func (c *Connector) resolveText(raw any, input map[string]any) string {
	text := asString(raw)
	text = resolveWithFunctions(text, input)
	return text
}

func resolveWithFunctions(raw string, input map[string]any) string {
	if raw == "" {
		return ""
	}
	// Handle simple pipe functions: {{path | func}}.
	if strings.Contains(raw, "|") {
		parts := strings.Split(raw, "|")
		if len(parts) >= 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.TrimSpace(strings.TrimSuffix(parts[1], "}}"))
			resolved := connectors.ResolveTemplateString(left+"}}", input)
			switch right {
			case "formatCurrency":
				return formatCurrency(resolved)
			case "formatDate":
				return formatDate(resolved)
			case "titleCase":
				return titleCase(resolved)
			case "upper":
				return strings.ToUpper(resolved)
			case "lower":
				return strings.ToLower(resolved)
			}
		}
	}
	return connectors.ResolveTemplateString(raw, input)
}

func (c *Connector) loadTemplate(ctx context.Context, name string) (documentTemplate, error) {
	var raw []byte
	err := c.db.QueryRowContext(ctx, `
SELECT template
FROM document_templates
WHERE name = $1 AND status = 'active'
ORDER BY version DESC
LIMIT 1
`, name).Scan(&raw)
	if err != nil {
		return documentTemplate{}, fmt.Errorf("load document template %s: %w", name, err)
	}
	tpl := documentTemplate{}
	if err := json.Unmarshal(raw, &tpl); err != nil {
		return documentTemplate{}, fmt.Errorf("parse document template %s: %w", name, err)
	}
	if tpl.Filename == "" {
		tpl.Filename = "Document-" + name + ".pdf"
	}
	return tpl, nil
}

func (c *Connector) resolveFallbackUploader(ctx context.Context, tenantID uuid.UUID) uuid.UUID {
	var principalID uuid.UUID
	_ = c.db.QueryRowContext(ctx, `SELECT id FROM principals WHERE tenant_id = $1 ORDER BY created_at ASC LIMIT 1`, tenantID).Scan(&principalID)
	return principalID
}

func buildSimplePDF(lines []string) []byte {
	escaped := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.ReplaceAll(line, "(", "\\(")
		line = strings.ReplaceAll(line, ")", "\\)")
		escaped = append(escaped, line)
	}
	contents := "BT /F1 11 Tf 40 800 Td 14 TL "
	for i, line := range escaped {
		if i == 0 {
			contents += fmt.Sprintf("(%s) Tj ", line)
			continue
		}
		contents += fmt.Sprintf("T* (%s) Tj ", line)
	}
	contents += "ET"
	stream := fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(contents), contents)
	pdf := "%PDF-1.4\n" +
		"1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n" +
		"2 0 obj << /Type /Pages /Kids [3 0 R] /Count 1 >> endobj\n" +
		"3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >> endobj\n" +
		"4 0 obj << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >> endobj\n" +
		"5 0 obj " + stream + " endobj\n" +
		"xref\n0 6\n0000000000 65535 f \n0000000010 00000 n \n0000000060 00000 n \n0000000117 00000 n \n0000000243 00000 n \n0000000314 00000 n \n" +
		"trailer << /Size 6 /Root 1 0 R >>\nstartxref\n470\n%%EOF"
	return []byte(pdf)
}

func formatCurrency(raw string) string {
	if raw == "" {
		return ""
	}
	var v float64
	_, err := fmt.Sscanf(raw, "%f", &v)
	if err != nil {
		return raw
	}
	return fmt.Sprintf("£%0.2f", v)
}

func formatDate(raw string) string {
	if raw == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return t.Format("2 January 2006")
		}
	}
	return raw
}

func titleCase(raw string) string {
	parts := strings.Fields(strings.ToLower(raw))
	for i, p := range parts {
		if len(p) == 0 {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func asString(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}
