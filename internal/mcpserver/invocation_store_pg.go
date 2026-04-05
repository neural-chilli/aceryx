package mcpserver

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type PostgresInvocationStore struct {
	db *sql.DB
}

func NewPostgresInvocationStore(db *sql.DB) *PostgresInvocationStore {
	return &PostgresInvocationStore{db: db}
}

func (s *PostgresInvocationStore) LogMCPInvocation(ctx context.Context, entry AuditEntry) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO mcp_tool_invocations (
    tenant_id, user_id, api_key_id, tool_name, arguments, result_status,
    duration_ms, source, correlation_id, depth, created_at
) VALUES ($1, $2, $3, $4, COALESCE($5::jsonb, '{}'::jsonb), $6, $7, $8, $9, $10, COALESCE($11, now()))
`, entry.TenantID, entry.UserID, entry.APIKeyID, strings.TrimSpace(entry.ToolName), string(entry.Arguments), entry.ResultStatus, entry.DurationMS, entry.Source, entry.CorrelationID, entry.Depth, entry.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert mcp invocation: %w", err)
	}
	return nil
}
