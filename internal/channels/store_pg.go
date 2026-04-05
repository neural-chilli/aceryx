package channels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/audit"
	"github.com/neural-chilli/aceryx/internal/cases"
)

type PostgresStore struct {
	db    *sql.DB
	audit *audit.Service
}

func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db, audit: audit.NewService(db)}
}

func (s *PostgresStore) Create(ctx context.Context, channel *Channel) error {
	if channel == nil {
		return fmt.Errorf("channel is nil")
	}
	adapterRaw, _ := json.Marshal(channel.AdapterConfig)
	dedupRaw, _ := json.Marshal(channel.DedupConfig)
	_, err := s.db.ExecContext(ctx, `
INSERT INTO channels (
    id, tenant_id, name, type, plugin_ref, config, case_type_id, workflow_id, adapter_config, dedup_config, enabled
) VALUES (
    $1, $2, $3, $4, NULLIF($5, ''), $6::jsonb, $7, $8, $9::jsonb, $10::jsonb, $11
)
`, channel.ID, channel.TenantID, channel.Name, channel.Type, channel.PluginRef, string(channel.Config), channel.CaseTypeID, channel.WorkflowID, string(adapterRaw), string(dedupRaw), channel.Enabled)
	if err != nil {
		return fmt.Errorf("create channel: %w", err)
	}
	return nil
}

func (s *PostgresStore) Update(ctx context.Context, channel *Channel) error {
	if channel == nil {
		return fmt.Errorf("channel is nil")
	}
	adapterRaw, _ := json.Marshal(channel.AdapterConfig)
	dedupRaw, _ := json.Marshal(channel.DedupConfig)
	_, err := s.db.ExecContext(ctx, `
UPDATE channels
SET name = $3,
    type = $4,
    plugin_ref = NULLIF($5, ''),
    config = $6::jsonb,
    case_type_id = $7,
    workflow_id = $8,
    adapter_config = $9::jsonb,
    dedup_config = $10::jsonb,
    enabled = $11,
    updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
`, channel.TenantID, channel.ID, channel.Name, channel.Type, channel.PluginRef, string(channel.Config), channel.CaseTypeID, channel.WorkflowID, string(adapterRaw), string(dedupRaw), channel.Enabled)
	if err != nil {
		return fmt.Errorf("update channel: %w", err)
	}
	return nil
}

func (s *PostgresStore) Get(ctx context.Context, tenantID, channelID uuid.UUID) (*Channel, error) {
	return s.scanChannel(s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, name, type, COALESCE(plugin_ref,''), config, case_type_id, workflow_id,
       adapter_config, dedup_config, enabled, created_at, updated_at
FROM channels
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
`, tenantID, channelID))
}

func (s *PostgresStore) GetByID(ctx context.Context, channelID uuid.UUID) (*Channel, error) {
	return s.scanChannel(s.db.QueryRowContext(ctx, `
SELECT id, tenant_id, name, type, COALESCE(plugin_ref,''), config, case_type_id, workflow_id,
       adapter_config, dedup_config, enabled, created_at, updated_at
FROM channels
WHERE id = $1 AND deleted_at IS NULL
`, channelID))
}

func (s *PostgresStore) List(ctx context.Context, tenantID uuid.UUID) ([]*Channel, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, name, type, COALESCE(plugin_ref,''), config, case_type_id, workflow_id,
       adapter_config, dedup_config, enabled, created_at, updated_at
FROM channels
WHERE tenant_id = $1 AND deleted_at IS NULL
ORDER BY created_at DESC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return s.scanChannels(rows)
}

func (s *PostgresStore) ListEnabled(ctx context.Context) ([]*Channel, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, name, type, COALESCE(plugin_ref,''), config, case_type_id, workflow_id,
       adapter_config, dedup_config, enabled, created_at, updated_at
FROM channels
WHERE enabled = TRUE AND deleted_at IS NULL
ORDER BY created_at ASC
`)
	if err != nil {
		return nil, fmt.Errorf("list enabled channels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return s.scanChannels(rows)
}

func (s *PostgresStore) SoftDelete(ctx context.Context, tenantID, channelID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE channels
SET deleted_at = now(), enabled = false, updated_at = now()
WHERE tenant_id = $1 AND id = $2
`, tenantID, channelID)
	if err != nil {
		return fmt.Errorf("soft delete channel: %w", err)
	}
	return nil
}

func (s *PostgresStore) SetEnabled(ctx context.Context, tenantID, channelID uuid.UUID, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE channels
SET enabled = $3, updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
`, tenantID, channelID, enabled)
	if err != nil {
		return fmt.Errorf("update channel enabled flag: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListEvents(ctx context.Context, tenantID, channelID uuid.UUID, limit, offset int) ([]*ChannelEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, tenant_id, channel_id, COALESCE(raw_payload, '{}'::jsonb), COALESCE(attachments, '[]'::jsonb), case_id,
       status, COALESCE(error_message, ''), COALESCE(processing_ms, 0), created_at
FROM channel_events
WHERE tenant_id = $1 AND channel_id = $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4
`, tenantID, channelID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list channel events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEvents(rows)
}

func (s *PostgresStore) RecordFailedEvent(ctx context.Context, event *ChannelEvent) (uuid.UUID, error) {
	if event == nil {
		return uuid.Nil, fmt.Errorf("event is nil")
	}
	return insertEvent(ctx, s.db, event)
}

func (s *PostgresStore) WithTx(ctx context.Context, fn func(txCtx context.Context, tx TxStore) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin channel transaction: %w", err)
	}
	txStore := &pgTxStore{tx: tx, audit: s.audit}
	if err := fn(ctx, txStore); err != nil {
		_ = s.audit.RollbackTx(tx)
		return err
	}
	if err := s.audit.CommitTx(tx); err != nil {
		return fmt.Errorf("commit channel transaction: %w", err)
	}
	return nil
}

type pgTxStore struct {
	tx    *sql.Tx
	audit *audit.Service
}

func (s *pgTxStore) GetChannel(ctx context.Context, tenantID, channelID uuid.UUID) (*Channel, error) {
	row := s.tx.QueryRowContext(ctx, `
SELECT id, tenant_id, name, type, COALESCE(plugin_ref,''), config, case_type_id, workflow_id,
       adapter_config, dedup_config, enabled, created_at, updated_at
FROM channels
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
FOR UPDATE
`, tenantID, channelID)
	return (&PostgresStore{}).scanChannel(row)
}

func (s *pgTxStore) FindRecentEvents(ctx context.Context, tenantID, channelID uuid.UUID, since time.Time) ([]*ChannelEvent, error) {
	rows, err := s.tx.QueryContext(ctx, `
SELECT id, tenant_id, channel_id, COALESCE(raw_payload, '{}'::jsonb), COALESCE(attachments, '[]'::jsonb), case_id,
       status, COALESCE(error_message, ''), COALESCE(processing_ms, 0), created_at
FROM channel_events
WHERE tenant_id = $1 AND channel_id = $2 AND created_at >= $3
ORDER BY created_at DESC
`, tenantID, channelID, since)
	if err != nil {
		return nil, fmt.Errorf("query recent channel events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanEvents(rows)
}

func (s *pgTxStore) FindCaseByFields(ctx context.Context, tenantID, caseTypeID uuid.UUID, fields []string, inbound json.RawMessage) (*uuid.UUID, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(inbound, &payload); err != nil {
		return nil, nil
	}
	parts := make([]string, 0, len(fields))
	args := []any{tenantID, caseTypeID}
	for _, field := range fields {
		value, ok := lookupPath(payload, field)
		if !ok {
			continue
		}
		args = append(args, fmt.Sprint(value))
		path := strings.Join(strings.Split(field, "."), ",")
		parts = append(parts, fmt.Sprintf("c.data #>> '{%s}' = $%d", path, len(args)))
	}
	if len(parts) == 0 {
		return nil, nil
	}
	query := `
SELECT c.id
FROM cases c
WHERE c.tenant_id = $1 AND c.case_type_id = $2 AND ` + strings.Join(parts, " AND ") + `
ORDER BY c.updated_at DESC
LIMIT 1`
	var caseID uuid.UUID
	if err := s.tx.QueryRowContext(ctx, query, args...).Scan(&caseID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find matching case: %w", err)
	}
	return &caseID, nil
}

func (s *pgTxStore) CreateCase(ctx context.Context, in CreateOrUpdateCaseInput) (uuid.UUID, error) {
	if in.ActorID == uuid.Nil {
		if err := s.tx.QueryRowContext(ctx, `SELECT id FROM principals WHERE tenant_id = $1 AND status = 'active' ORDER BY created_at ASC LIMIT 1`, in.TenantID).Scan(&in.ActorID); err != nil {
			return uuid.Nil, fmt.Errorf("resolve channel actor: %w", err)
		}
	}

	var (
		caseTypeName string
		rawSchema    []byte
	)
	if err := s.tx.QueryRowContext(ctx, `
SELECT name, schema
FROM case_types
WHERE tenant_id = $1 AND id = $2 AND status = 'active'
ORDER BY version DESC
LIMIT 1
`, in.TenantID, in.CaseTypeID).Scan(&caseTypeName, &rawSchema); err != nil {
		return uuid.Nil, fmt.Errorf("resolve case type for channel: %w", err)
	}
	var schema cases.CaseTypeSchema
	if err := json.Unmarshal(rawSchema, &schema); err == nil {
		if validation := cases.ValidateCaseData(schema, in.Data); len(validation) > 0 {
			return uuid.Nil, fmt.Errorf("case validation failed: %s", validation[0].Message)
		}
	}

	workflowID := in.WorkflowID
	workflowVersion := 1
	rawAST := []byte(`{"nodes":[]}`)
	if workflowID != nil {
		if err := s.tx.QueryRowContext(ctx, `
SELECT version, ast
FROM workflow_versions
WHERE workflow_id = $1 AND status = 'published'
ORDER BY version DESC
LIMIT 1
`, *workflowID).Scan(&workflowVersion, &rawAST); err != nil {
			return uuid.Nil, fmt.Errorf("resolve workflow version: %w", err)
		}
	} else {
		if err := s.tx.QueryRowContext(ctx, `
SELECT w.id, wv.version, wv.ast
FROM workflows w
JOIN workflow_versions wv ON wv.workflow_id = w.id
WHERE w.tenant_id = $1 AND w.case_type = $2 AND wv.status = 'published'
ORDER BY wv.version DESC
LIMIT 1
`, in.TenantID, caseTypeName).Scan(&workflowID, &workflowVersion, &rawAST); err != nil {
			return uuid.Nil, fmt.Errorf("resolve default workflow for channel case type: %w", err)
		}
	}

	var lastNum int64
	if err := s.tx.QueryRowContext(ctx, `
INSERT INTO case_number_sequences (tenant_id, case_type, last_number)
VALUES ($1, $2, 1)
ON CONFLICT (tenant_id, case_type)
DO UPDATE SET last_number = case_number_sequences.last_number + 1
RETURNING last_number
`, in.TenantID, caseTypeName).Scan(&lastNum); err != nil {
		return uuid.Nil, fmt.Errorf("allocate case number: %w", err)
	}
	caseNumber := strings.ToUpper(strings.ReplaceAll(caseTypeName, " ", "-")) + "-" + fmt.Sprintf("%06d", lastNum)

	rawData, _ := json.Marshal(in.Data)
	var caseID uuid.UUID
	if err := s.tx.QueryRowContext(ctx, `
INSERT INTO cases (
    tenant_id, case_type_id, case_number, status, data, created_by, priority, workflow_id, workflow_version
) VALUES (
    $1, $2, $3, 'open', $4::jsonb, $5, 0, $6, $7
)
RETURNING id
`, in.TenantID, in.CaseTypeID, caseNumber, string(rawData), in.ActorID, workflowID, workflowVersion).Scan(&caseID); err != nil {
		return uuid.Nil, fmt.Errorf("insert channel-created case: %w", err)
	}

	for _, stepID := range extractStepIDs(rawAST) {
		if _, err := s.tx.ExecContext(ctx, `
INSERT INTO case_steps (case_id, step_id, state, result, events, error, retry_count, draft_data, metadata)
VALUES ($1, $2, 'pending', '{}'::jsonb, '[]'::jsonb, '{}'::jsonb, 0, '{}'::jsonb, '{}'::jsonb)
`, caseID, stepID); err != nil {
			return uuid.Nil, fmt.Errorf("initialize case step %s: %w", stepID, err)
		}
	}

	if err := s.audit.RecordCaseEventTx(ctx, s.tx, caseID, "", "case", in.ActorID, "system", "created", map[string]any{
		"source":     "channel",
		"channel_id": in.ChannelID.String(),
	}); err != nil {
		return uuid.Nil, err
	}
	return caseID, nil
}

func (s *pgTxStore) UpdateCaseData(ctx context.Context, tenantID, caseID uuid.UUID, patch map[string]any) error {
	if len(patch) == 0 {
		return nil
	}
	var (
		rawData   []byte
		rawSchema []byte
		actorID   uuid.UUID
	)
	if err := s.tx.QueryRowContext(ctx, `
SELECT c.data, ct.schema, c.created_by
FROM cases c
JOIN case_types ct ON ct.id = c.case_type_id
WHERE c.tenant_id = $1 AND c.id = $2
FOR UPDATE
`, tenantID, caseID).Scan(&rawData, &rawSchema, &actorID); err != nil {
		return fmt.Errorf("load existing case for channel update: %w", err)
	}
	current := map[string]any{}
	_ = json.Unmarshal(rawData, &current)
	merged := cases.DeepMerge(current, patch)
	var schema cases.CaseTypeSchema
	if err := json.Unmarshal(rawSchema, &schema); err == nil {
		if validation := cases.ValidateCaseData(schema, merged); len(validation) > 0 {
			return fmt.Errorf("case validation failed: %s", validation[0].Message)
		}
	}
	rawMerged, _ := json.Marshal(merged)
	if _, err := s.tx.ExecContext(ctx, `
UPDATE cases
SET data = $3::jsonb,
    version = version + 1,
    updated_at = now()
WHERE tenant_id = $1 AND id = $2
`, tenantID, caseID, string(rawMerged)); err != nil {
		return fmt.Errorf("update channel-matched case data: %w", err)
	}
	if err := s.audit.RecordCaseEventTx(ctx, s.tx, caseID, "", "case", actorID, "system", "updated", map[string]any{
		"source": "channel",
	}); err != nil {
		return err
	}
	return nil
}

func (s *pgTxStore) RecordEvent(ctx context.Context, event *ChannelEvent) (uuid.UUID, error) {
	return insertEvent(ctx, s.tx, event)
}

func (s *PostgresStore) scanChannel(row *sql.Row) (*Channel, error) {
	var (
		item     Channel
		adapter  []byte
		dedup    []byte
		workflow sql.Null[uuid.UUID]
	)
	if err := row.Scan(&item.ID, &item.TenantID, &item.Name, &item.Type, &item.PluginRef, &item.Config, &item.CaseTypeID, &workflow, &adapter, &dedup, &item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	if workflow.Valid {
		item.WorkflowID = &workflow.V
	}
	_ = json.Unmarshal(adapter, &item.AdapterConfig)
	_ = json.Unmarshal(dedup, &item.DedupConfig)
	return &item, nil
}

func (s *PostgresStore) scanChannels(rows *sql.Rows) ([]*Channel, error) {
	out := make([]*Channel, 0)
	for rows.Next() {
		var (
			item     Channel
			adapter  []byte
			dedup    []byte
			workflow sql.Null[uuid.UUID]
		)
		if err := rows.Scan(&item.ID, &item.TenantID, &item.Name, &item.Type, &item.PluginRef, &item.Config, &item.CaseTypeID, &workflow, &adapter, &dedup, &item.Enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		if workflow.Valid {
			item.WorkflowID = &workflow.V
		}
		_ = json.Unmarshal(adapter, &item.AdapterConfig)
		_ = json.Unmarshal(dedup, &item.DedupConfig)
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channels: %w", err)
	}
	return out, nil
}

func scanEvents(rows *sql.Rows) ([]*ChannelEvent, error) {
	out := make([]*ChannelEvent, 0)
	for rows.Next() {
		var (
			item        ChannelEvent
			attachments []byte
			caseID      sql.Null[uuid.UUID]
		)
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ChannelID, &item.RawPayload, &attachments, &caseID, &item.Status, &item.ErrorMessage, &item.ProcessingMS, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan channel event: %w", err)
		}
		if caseID.Valid {
			item.CaseID = &caseID.V
		}
		_ = json.Unmarshal(attachments, &item.Attachments)
		out = append(out, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate channel events: %w", err)
	}
	return out, nil
}

func insertEvent(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, event *ChannelEvent) (uuid.UUID, error) {
	if event == nil {
		return uuid.Nil, fmt.Errorf("event is nil")
	}
	attachmentsRaw, _ := json.Marshal(event.Attachments)
	var eventID uuid.UUID
	if err := db.QueryRowContext(ctx, `
INSERT INTO channel_events (tenant_id, channel_id, raw_payload, attachments, case_id, status, error_message, processing_ms)
VALUES ($1, $2, $3::jsonb, $4::jsonb, $5, $6, NULLIF($7,''), $8)
RETURNING id
`, event.TenantID, event.ChannelID, string(event.RawPayload), string(attachmentsRaw), event.CaseID, event.Status, event.ErrorMessage, event.ProcessingMS).Scan(&eventID); err != nil {
		return uuid.Nil, fmt.Errorf("insert channel event: %w", err)
	}
	return eventID, nil
}

func extractStepIDs(rawAST []byte) []string {
	var ast struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(rawAST, &ast); err != nil {
		return nil
	}
	out := make([]string, 0, len(ast.Nodes))
	for _, node := range ast.Nodes {
		if strings.TrimSpace(node.ID) == "" {
			continue
		}
		out = append(out, node.ID)
	}
	return out
}
