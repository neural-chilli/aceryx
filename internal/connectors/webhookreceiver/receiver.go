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
	eval    DAGEvaluator
}

func NewHandler(db *sql.DB, secrets connectors.SecretStore) *Handler {
	return &Handler{db: db, secrets: secrets}
}

type DAGEvaluator interface {
	EvaluateDAG(ctx context.Context, caseID uuid.UUID) error
}

func (h *Handler) SetEvaluator(evaluator DAGEvaluator) {
	h.eval = evaluator
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	paths := normalizeCandidatePaths(r)
	var (
		cfg RouteConfig
		err error
	)
	for _, path := range paths {
		cfg, err = h.loadRoute(r.Context(), path)
		if err == nil {
			break
		}
	}
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
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}

	if cfg.SignatureHeader != "" && cfg.SignatureSecretKey != "" {
		secret, serr := h.secrets.Get(r.Context(), cfg.TenantID, cfg.SignatureSecretKey)
		if serr != nil || !validateSignature(secret, body, r.Header.Get(cfg.SignatureHeader)) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" && cfg.IdempotencyKeyPath != "" {
		idempotencyKey = strings.TrimSpace(connectors.ResolveTemplateString("{{"+cfg.IdempotencyKeyPath+"}}", payload))
		if idempotencyKey == "" {
			idempotencyKey = strings.TrimSpace(connectors.ResolveTemplateString("{{"+cfg.IdempotencyKeyPath+"}}", map[string]any{"payload": payload}))
		}
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
	_, _ = fmt.Fprintf(w, `{"status":"ok","case_id":"%s"}`, caseID)
}

func (h *Handler) loadRoute(ctx context.Context, path string) (RouteConfig, error) {
	var cfg RouteConfig
	err := h.db.QueryRowContext(ctx, `
SELECT tenant_id,
       path,
       case_type,
       mode,
       COALESCE(signature_header, ''),
       COALESCE(signature_secret_key, ''),
       COALESCE(idempotency_key_path, ''),
       COALESCE(case_number_field_path, ''),
       created_by
FROM webhook_routes
WHERE trim(both '/' from path) = trim(both '/' from $1)
`, path).Scan(&cfg.TenantID, &cfg.Path, &cfg.CaseType, &cfg.Mode, &cfg.SignatureHeader, &cfg.SignatureSecretKey, &cfg.IdempotencyKeyPath, &cfg.CaseNumberFieldPath, &cfg.CreatedBy)
	return cfg, err
}

func normalizeCandidatePaths(r *http.Request) []string {
	rawPathValue := strings.TrimSpace(r.PathValue("path"))
	rawURLPath := strings.TrimSpace(r.URL.Path)
	candidates := []string{
		rawPathValue,
		strings.TrimPrefix(rawPathValue, "/webhooks/"),
		strings.TrimPrefix(rawPathValue, "/webhooks"),
		rawURLPath,
		strings.TrimPrefix(rawURLPath, "/webhooks/"),
		strings.TrimPrefix(rawURLPath, "/webhooks"),
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		c = "/" + strings.Trim(strings.TrimSpace(c), "/")
		if c == "/" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

func (h *Handler) recordWebhookDelivery(ctx context.Context, tenantID uuid.UUID, key string, payload map[string]any) (bool, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("marshal webhook delivery payload: %w", err)
	}
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
		caseNumber := strings.TrimSpace(connectors.ResolveTemplateString("{{"+cfg.CaseNumberFieldPath+"}}", payload))
		if caseNumber == "" {
			caseNumber = strings.TrimSpace(connectors.ResolveTemplateString("{{payload."+cfg.CaseNumberFieldPath+"}}", map[string]any{"payload": payload}))
		}
		if caseNumber != "" {
			var caseID uuid.UUID
			if err := h.db.QueryRowContext(ctx, `SELECT id FROM cases WHERE tenant_id = $1 AND case_number = $2`, cfg.TenantID, caseNumber).Scan(&caseID); err == nil {
				raw, mErr := json.Marshal(payload)
				if mErr != nil {
					return uuid.Nil, fmt.Errorf("marshal webhook update payload: %w", mErr)
				}
				if _, uErr := h.db.ExecContext(ctx, `UPDATE cases SET data = COALESCE(data,'{}'::jsonb) || $3::jsonb, updated_at = now(), version = version + 1 WHERE id = $1 AND tenant_id = $2`, caseID, cfg.TenantID, string(raw)); uErr != nil {
					return uuid.Nil, fmt.Errorf("update case from webhook: %w", uErr)
				}
				if h.eval != nil {
					_ = h.eval.EvaluateDAG(ctx, caseID)
				}
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
	var workflowAST []byte
	if err := h.db.QueryRowContext(ctx, `
SELECT w.id, wv.version, wv.ast
FROM workflows w
JOIN workflow_versions wv ON wv.workflow_id = w.id
WHERE w.tenant_id = $1 AND w.case_type = $2 AND wv.status = 'published'
ORDER BY wv.version DESC
LIMIT 1
`, cfg.TenantID, cfg.CaseType).Scan(&workflowID, &workflowVersion, &workflowAST); err != nil {
		return uuid.Nil, fmt.Errorf("resolve workflow for webhook: %w", err)
	}
	stepIDs, err := parseWorkflowStepIDs(workflowAST)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse workflow steps for webhook case init: %w", err)
	}

	rawData, err := json.Marshal(payload)
	if err != nil {
		return uuid.Nil, fmt.Errorf("marshal webhook create payload: %w", err)
	}
	caseNumber := "WEB-" + strings.ToUpper(uuid.NewString()[:8])
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin webhook case create tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var caseID uuid.UUID
	err = tx.QueryRowContext(ctx, `
INSERT INTO cases (tenant_id, case_type_id, case_number, status, data, created_by, workflow_id, workflow_version)
VALUES ($1, $2, $3, 'open', $4::jsonb, $5, $6, $7)
RETURNING id
`, cfg.TenantID, caseTypeID, caseNumber, string(rawData), cfg.CreatedBy, workflowID, workflowVersion).Scan(&caseID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create case from webhook: %w", err)
	}
	for _, stepID := range stepIDs {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state, result, events, error, retry_count, draft_data, metadata)
VALUES ($1, $2, 'pending', '{}'::jsonb, '[]'::jsonb, '{}'::jsonb, 0, '{}'::jsonb, '{}'::jsonb)
`, caseID, stepID); err != nil {
			return uuid.Nil, fmt.Errorf("init webhook case step %s: %w", stepID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return uuid.Nil, fmt.Errorf("commit webhook case create tx: %w", err)
	}
	if h.eval != nil {
		_ = h.eval.EvaluateDAG(ctx, caseID)
	}
	return caseID, nil
}

func parseWorkflowStepIDs(astRaw []byte) ([]string, error) {
	var ast struct {
		Steps []struct {
			ID string `json:"id"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(astRaw, &ast); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(ast.Steps))
	seen := make(map[string]struct{}, len(ast.Steps))
	for _, step := range ast.Steps {
		id := strings.TrimSpace(step.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
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
