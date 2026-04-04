package plugins

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

type PluginRecord struct {
	Plugin
	LoadedAt time.Time
}

type InvocationRecord struct {
	ID             uuid.UUID       `json:"id"`
	TenantID       uuid.UUID       `json:"tenant_id"`
	PluginID       string          `json:"plugin_id"`
	PluginVersion  string          `json:"plugin_version"`
	WASMHash       string          `json:"wasm_hash"`
	CaseID         *uuid.UUID      `json:"case_id,omitempty"`
	StepID         string          `json:"step_id,omitempty"`
	InvocationType string          `json:"invocation_type"`
	InputHash      string          `json:"input_hash"`
	OutputHash     string          `json:"output_hash,omitempty"`
	DurationMS     int             `json:"duration_ms"`
	HostCalls      json.RawMessage `json:"host_calls"`
	Status         string          `json:"status"`
	ErrorMessage   string          `json:"error_message,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
}

func (s *Store) UpsertPlugin(ctx context.Context, p *Plugin) error {
	if s == nil || s.db == nil || p == nil {
		return nil
	}
	metadata, _ := json.Marshal(p.Manifest.Metadata)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO plugins (
    plugin_id, name, version, type, category, licence_tier, maturity_tier, manifest_hash, wasm_hash, is_latest, status, error_message, metadata
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb
)
ON CONFLICT (plugin_id, version)
DO UPDATE SET
    name = EXCLUDED.name,
    type = EXCLUDED.type,
    category = EXCLUDED.category,
    licence_tier = EXCLUDED.licence_tier,
    maturity_tier = EXCLUDED.maturity_tier,
    manifest_hash = EXCLUDED.manifest_hash,
    wasm_hash = EXCLUDED.wasm_hash,
    is_latest = EXCLUDED.is_latest,
    status = EXCLUDED.status,
    error_message = EXCLUDED.error_message,
    metadata = EXCLUDED.metadata,
    loaded_at = now()
`, p.ID, p.Name, p.Version, string(p.Type), p.Category, p.LicenceTier, p.MaturityTier, p.ManifestHash, p.WASMHash, p.IsLatest, string(p.Status), p.ErrorMessage, string(metadata))
	if err != nil {
		return fmt.Errorf("upsert plugin %s@%s: %w", p.ID, p.Version, err)
	}
	return nil
}

func (s *Store) SetLatestByPluginID(ctx context.Context, pluginID, latestVersion string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE plugins
SET is_latest = (version = $2)
WHERE plugin_id = $1
`, pluginID, latestVersion)
	if err != nil {
		return fmt.Errorf("update latest plugin version for %s: %w", pluginID, err)
	}
	return nil
}

func (s *Store) SetStatusByPluginID(ctx context.Context, pluginID string, status PluginStatus) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE plugins SET status = $2 WHERE plugin_id = $1`, pluginID, string(status))
	if err != nil {
		return fmt.Errorf("set plugin status for %s: %w", pluginID, err)
	}
	return nil
}

func (s *Store) DeleteByRef(ctx context.Context, ref PluginRef) error {
	if s == nil || s.db == nil {
		return nil
	}
	if ref.Version == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM plugins WHERE plugin_id = $1`, ref.ID)
		if err != nil {
			return fmt.Errorf("delete plugin %s: %w", ref.ID, err)
		}
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM plugins WHERE plugin_id = $1 AND version = $2`, ref.ID, ref.Version)
	if err != nil {
		return fmt.Errorf("delete plugin %s@%s: %w", ref.ID, ref.Version, err)
	}
	return nil
}

func (s *Store) InsertInvocation(ctx context.Context, in InvocationRecord) error {
	if s == nil || s.db == nil {
		return nil
	}
	if in.ID == uuid.Nil {
		in.ID = uuid.New()
	}
	if len(in.HostCalls) == 0 {
		in.HostCalls = []byte("[]")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO plugin_invocations (
    id, tenant_id, plugin_id, plugin_version, wasm_hash, case_id, step_id, invocation_type, input_hash, output_hash, duration_ms, host_calls, status, error_message
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, NULLIF($10, ''), $11, $12::jsonb, $13, NULLIF($14, '')
)
`, in.ID, in.TenantID, in.PluginID, in.PluginVersion, in.WASMHash, in.CaseID, in.StepID, in.InvocationType, in.InputHash, in.OutputHash, in.DurationMS, string(in.HostCalls), in.Status, in.ErrorMessage)
	if err != nil {
		return fmt.Errorf("insert plugin invocation: %w", err)
	}
	return nil
}

func (s *Store) ListInvocationsByPlugin(ctx context.Context, tenantID uuid.UUID, pluginID string, limit int) ([]InvocationRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, plugin_id, plugin_version, wasm_hash, case_id, step_id, invocation_type, input_hash, COALESCE(output_hash, ''), duration_ms, host_calls, status, COALESCE(error_message, ''), created_at
FROM plugin_invocations
WHERE tenant_id = $1
  AND plugin_id = $2
ORDER BY created_at DESC
LIMIT $3
`, tenantID, pluginID, limit)
	if err != nil {
		return nil, fmt.Errorf("query plugin invocations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]InvocationRecord, 0, limit)
	for rows.Next() {
		var row InvocationRecord
		var caseID sql.NullString
		if err := rows.Scan(&row.ID, &row.TenantID, &row.PluginID, &row.PluginVersion, &row.WASMHash, &caseID, &row.StepID, &row.InvocationType, &row.InputHash, &row.OutputHash, &row.DurationMS, &row.HostCalls, &row.Status, &row.ErrorMessage, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan plugin invocation: %w", err)
		}
		if caseID.Valid {
			parsed, err := uuid.Parse(caseID.String)
			if err == nil {
				row.CaseID = &parsed
			}
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate plugin invocations: %w", err)
	}
	return out, nil
}

func (s *Store) FindWorkflowsUsingPlugin(ctx context.Context, pluginID string, changes []PropertyChange) ([]WorkflowReference, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT w.id, w.name, wv.version, wv.ast
FROM workflow_versions wv
JOIN workflows w ON w.id = wv.workflow_id
`)
	if err != nil {
		return nil, fmt.Errorf("query workflows for plugin impact: %w", err)
	}
	defer func() { _ = rows.Close() }()

	changedKeys := make(map[string]struct{})
	for _, change := range changes {
		switch change.Type {
		case "removed", "renamed", "type_changed":
			changedKeys[change.Key] = struct{}{}
		}
	}

	out := make([]WorkflowReference, 0)
	for rows.Next() {
		var (
			ref WorkflowReference
			ast []byte
		)
		if err := rows.Scan(&ref.WorkflowID, &ref.WorkflowName, &ref.Version, &ast); err != nil {
			return nil, fmt.Errorf("scan workflow impact row: %w", err)
		}
		if workflowUsesPluginProperties(ast, pluginID, changedKeys) {
			out = append(out, ref)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workflow impact rows: %w", err)
	}
	return out, nil
}

func workflowUsesPluginProperties(ast []byte, pluginID string, keys map[string]struct{}) bool {
	if len(ast) == 0 {
		return false
	}
	var tree map[string]any
	if err := json.Unmarshal(ast, &tree); err != nil {
		return false
	}
	rawSteps, ok := tree["steps"].([]any)
	if !ok {
		return false
	}
	for _, rawStep := range rawSteps {
		stepMap, ok := rawStep.(map[string]any)
		if !ok {
			continue
		}
		cfg, ok := stepMap["config"].(map[string]any)
		if !ok {
			continue
		}
		pluginRef, _ := cfg["plugin"].(string)
		if strings.TrimSpace(pluginRef) == "" {
			continue
		}
		ref := ParsePluginRef(pluginRef)
		if ref.ID != pluginID {
			continue
		}
		inputMap, _ := cfg["input"].(map[string]any)
		for key := range keys {
			if _, exists := inputMap[key]; exists {
				return true
			}
		}
	}
	return false
}
