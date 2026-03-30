package webhookreceiver

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/connectors"
)

type Connector struct{}

func New() *Connector { return &Connector{} }

func (c *Connector) Meta() connectors.ConnectorMeta {
	return connectors.ConnectorMeta{Key: "webhook_receiver", Name: "Webhook Receiver", Description: "Inbound webhook trigger", Version: "v1", Icon: "pi pi-link"}
}

func (c *Connector) Auth() connectors.AuthSpec {
	return connectors.AuthSpec{Type: "api_key", Fields: []connectors.AuthField{{Key: "signature_secret", Label: "Signature Secret", Type: "password", Required: false}}}
}

func (c *Connector) Triggers() []connectors.TriggerSpec {
	return []connectors.TriggerSpec{{Key: "webhook", Name: "Webhook", Description: "Inbound webhook", Type: "webhook", OutputSchema: map[string]any{"type": "object"}}}
}

func (c *Connector) Actions() []connectors.ActionSpec { return nil }

type RouteConfig struct {
	TenantID            uuid.UUID
	Path                string
	CaseType            string
	Mode                string
	SignatureHeader     string
	SignatureSecretKey  string
	IdempotencyKeyPath  string
	CaseNumberFieldPath string
	CreatedBy           uuid.UUID
}

type Handler struct {
	db      *sql.DB
	secrets connectors.SecretStore
}

func NewHandler(db *sql.DB, secrets connectors.SecretStore) *Handler {
	return &Handler{db: db, secrets: secrets}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	cfg, err := h.loadRoute(r.Context(), "/"+strings.TrimPrefix(path, "/"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	payload := map[string]any{}
	_ = json.Unmarshal(body, &payload)

	if cfg.SignatureHeader != "" && cfg.SignatureSecretKey != "" {
		secret, serr := h.secrets.Get(r.Context(), cfg.TenantID, cfg.SignatureSecretKey)
		if serr != nil || !validateSignature(secret, body, r.Header.Get(cfg.SignatureHeader)) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" && cfg.IdempotencyKeyPath != "" {
		idempotencyKey = strings.TrimSpace(connectors.ResolveTemplateString("{{"+cfg.IdempotencyKeyPath+"}}", map[string]any{"payload": payload}))
	}
	if idempotencyKey == "" {
		sum := sha256.Sum256(body)
		idempotencyKey = strings.ToLower(hex.EncodeToString(sum[:]))
	}

	done, derr := h.recordWebhookDelivery(r.Context(), cfg.TenantID, idempotencyKey, payload)
	if derr != nil {
		http.Error(w, derr.Error(), http.StatusInternalServerError)
		return
	}
	if done {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"duplicate"}`))
		return
	}

	caseID, err := h.createOrUpdateCase(r.Context(), cfg, payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ok","case_id":"%s"}`, caseID)))
}

func (h *Handler) loadRoute(ctx context.Context, path string) (RouteConfig, error) {
	var cfg RouteConfig
	err := h.db.QueryRowContext(ctx, `
SELECT tenant_id, path, case_type, mode, signature_header, signature_secret_key, idempotency_key_path, case_number_field_path, created_by
FROM webhook_routes
WHERE path = $1
`, path).Scan(&cfg.TenantID, &cfg.Path, &cfg.CaseType, &cfg.Mode, &cfg.SignatureHeader, &cfg.SignatureSecretKey, &cfg.IdempotencyKeyPath, &cfg.CaseNumberFieldPath, &cfg.CreatedBy)
	return cfg, err
}

func (h *Handler) recordWebhookDelivery(ctx context.Context, tenantID uuid.UUID, key string, payload map[string]any) (bool, error) {
	raw, _ := json.Marshal(payload)
	res, err := h.db.ExecContext(ctx, `
INSERT INTO webhook_deliveries (idempotency_key, tenant_id, payload, status)
VALUES ($1, $2, $3::jsonb, 'processed')
ON CONFLICT (idempotency_key) DO NOTHING
`, key, tenantID, string(raw))
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected == 0, nil
}

func (h *Handler) createOrUpdateCase(ctx context.Context, cfg RouteConfig, payload map[string]any) (uuid.UUID, error) {
	if cfg.Mode == "update" && cfg.CaseNumberFieldPath != "" {
		caseNumber := connectors.ResolveTemplateString("{{payload."+cfg.CaseNumberFieldPath+"}}", map[string]any{"payload": payload})
		if caseNumber != "" {
			var caseID uuid.UUID
			if err := h.db.QueryRowContext(ctx, `SELECT id FROM cases WHERE tenant_id = $1 AND case_number = $2`, cfg.TenantID, caseNumber).Scan(&caseID); err == nil {
				raw, _ := json.Marshal(payload)
				_, _ = h.db.ExecContext(ctx, `UPDATE cases SET data = COALESCE(data,'{}'::jsonb) || $3::jsonb, updated_at = now(), version = version + 1 WHERE id = $1 AND tenant_id = $2`, caseID, cfg.TenantID, string(raw))
				return caseID, nil
			}
		}
	}

	var caseTypeID uuid.UUID
	if err := h.db.QueryRowContext(ctx, `
SELECT id FROM case_types WHERE tenant_id = $1 AND name = $2 AND status = 'active' ORDER BY version DESC LIMIT 1
`, cfg.TenantID, cfg.CaseType).Scan(&caseTypeID); err != nil {
		return uuid.Nil, fmt.Errorf("resolve case type for webhook: %w", err)
	}

	var workflowID uuid.UUID
	var workflowVersion int
	if err := h.db.QueryRowContext(ctx, `
SELECT w.id, wv.version
FROM workflows w
JOIN workflow_versions wv ON wv.workflow_id = w.id
WHERE w.tenant_id = $1 AND w.case_type = $2 AND wv.status = 'published'
ORDER BY wv.version DESC
LIMIT 1
`, cfg.TenantID, cfg.CaseType).Scan(&workflowID, &workflowVersion); err != nil {
		return uuid.Nil, fmt.Errorf("resolve workflow for webhook: %w", err)
	}

	rawData, _ := json.Marshal(payload)
	caseNumber := "WEB-" + strings.ToUpper(uuid.NewString()[:8])
	var caseID uuid.UUID
	err := h.db.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, $3, 'open', $4::jsonb, $5, $6, $7)
RETURNING id
`, cfg.TenantID, caseTypeID, caseNumber, string(rawData), cfg.CreatedBy, workflowID, workflowVersion).Scan(&caseID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create case from webhook: %w", err)
	}
	return caseID, nil
}

func validateSignature(secret string, payload []byte, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	expected := hex.EncodeToString(mac.Sum(nil))
	signature = strings.TrimPrefix(signature, "sha256=")
	return hmac.Equal([]byte(expected), []byte(signature))
}
