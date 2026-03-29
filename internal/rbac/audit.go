package rbac

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type authEvent struct {
	TenantID    *uuid.UUID
	PrincipalID *uuid.UUID
	EventType   string
	Success     bool
	Permission  string
	Path        string
	IPAddress   string
	UserAgent   string
	Data        map[string]interface{}
}

func recordAuthEvent(ctx context.Context, db *sql.DB, event authEvent) error {
	raw := json.RawMessage("{}")
	if event.Data != nil {
		b, err := json.Marshal(event.Data)
		if err != nil {
			return fmt.Errorf("marshal auth event data: %w", err)
		}
		raw = b
	}

	_, err := db.ExecContext(ctx, `
INSERT INTO auth_events (
    tenant_id, principal_id, event_type, success, permission, resource_path, ip_address, user_agent, data
) VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), $9::jsonb)
`, event.TenantID, event.PrincipalID, event.EventType, event.Success, event.Permission, event.Path, event.IPAddress, event.UserAgent, string(raw))
	if err != nil {
		return fmt.Errorf("insert auth event: %w", err)
	}
	return nil
}
