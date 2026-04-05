package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type AuditEntry struct {
	TenantID      uuid.UUID       `json:"tenant_id"`
	UserID        uuid.UUID       `json:"user_id"`
	APIKeyID      uuid.UUID       `json:"api_key_id"`
	ToolName      string          `json:"tool_name"`
	Arguments     json.RawMessage `json:"arguments"`
	ResultStatus  string          `json:"result_status"`
	DurationMS    int             `json:"duration_ms"`
	Source        string          `json:"source"`
	CorrelationID string          `json:"correlation_id"`
	Depth         int             `json:"depth"`
	CreatedAt     time.Time       `json:"created_at"`
}

type InvocationStore interface {
	LogMCPInvocation(ctx context.Context, entry AuditEntry) error
}

type AuditLogger struct {
	store InvocationStore
}

func NewAuditLogger(store InvocationStore) *AuditLogger {
	return &AuditLogger{store: store}
}

func (al *AuditLogger) LogInvocation(ctx context.Context, entry AuditEntry) error {
	if al == nil || al.store == nil {
		return nil
	}
	if entry.Source == "" {
		entry.Source = "mcp"
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	if err := al.store.LogMCPInvocation(ctx, entry); err != nil {
		return fmt.Errorf("log mcp invocation: %w", err)
	}
	return nil
}
