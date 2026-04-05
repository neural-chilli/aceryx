package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ToolCache struct {
	db              *sql.DB
	refreshInterval time.Duration
}

func NewToolCache(db *sql.DB, refreshInterval time.Duration) *ToolCache {
	if refreshInterval <= 0 {
		refreshInterval = 24 * time.Hour
	}
	return &ToolCache{db: db, refreshInterval: refreshInterval}
}

func (tc *ToolCache) GetTools(ctx context.Context, tenantID uuid.UUID, serverURL string) ([]MCPTool, error) {
	if tc == nil || tc.db == nil {
		return nil, nil
	}
	var (
		toolsRaw       []byte
		lastDiscovered time.Time
	)
	err := tc.db.QueryRowContext(ctx, `
SELECT tools, last_discovered
FROM mcp_server_cache
WHERE tenant_id = $1 AND server_url = $2
`, tenantID, strings.TrimSpace(serverURL)).Scan(&toolsRaw, &lastDiscovered)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load mcp tool cache: %w", err)
	}
	if time.Since(lastDiscovered) > tc.refreshInterval {
		return nil, nil
	}
	return decodeTools(toolsRaw)
}

func (tc *ToolCache) SetTools(ctx context.Context, tenantID uuid.UUID, serverURL string, tools []MCPTool) error {
	if tc == nil || tc.db == nil {
		return nil
	}
	raw, err := json.Marshal(tools)
	if err != nil {
		return fmt.Errorf("marshal mcp tools: %w", err)
	}
	_, err = tc.db.ExecContext(ctx, `
INSERT INTO mcp_server_cache (tenant_id, server_url, tools, last_discovered, status, error_message)
VALUES ($1, $2, $3::jsonb, now(), 'active', NULL)
ON CONFLICT (tenant_id, server_url)
DO UPDATE SET
	tools = EXCLUDED.tools,
	last_discovered = EXCLUDED.last_discovered,
	status = 'active',
	error_message = NULL
`, tenantID, strings.TrimSpace(serverURL), string(raw))
	if err != nil {
		return fmt.Errorf("upsert mcp tool cache: %w", err)
	}
	return nil
}

func (tc *ToolCache) SetError(ctx context.Context, tenantID uuid.UUID, serverURL string, err error) error {
	if tc == nil || tc.db == nil {
		return nil
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	_, qerr := tc.db.ExecContext(ctx, `
UPDATE mcp_server_cache
SET status = 'error', error_message = $3, last_discovered = now()
WHERE tenant_id = $1 AND server_url = $2
`, tenantID, strings.TrimSpace(serverURL), msg)
	if qerr != nil {
		return fmt.Errorf("set mcp cache error: %w", qerr)
	}
	return nil
}

func (tc *ToolCache) MarkStale(ctx context.Context, tenantID uuid.UUID, serverURL string) error {
	if tc == nil || tc.db == nil {
		return nil
	}
	_, err := tc.db.ExecContext(ctx, `
UPDATE mcp_server_cache
SET status = 'stale'
WHERE tenant_id = $1 AND server_url = $2
`, tenantID, strings.TrimSpace(serverURL))
	if err != nil {
		return fmt.Errorf("mark mcp cache stale: %w", err)
	}
	return nil
}

func (tc *ToolCache) ListServers(ctx context.Context, tenantID uuid.UUID) ([]CachedServer, error) {
	if tc == nil || tc.db == nil {
		return nil, nil
	}
	rows, err := tc.db.QueryContext(ctx, `
SELECT tenant_id, server_url, tools, last_discovered, status, COALESCE(error_message, '')
FROM mcp_server_cache
WHERE tenant_id = $1
ORDER BY server_url ASC
`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list mcp servers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanCachedServers(rows)
}

func (tc *ToolCache) ListAllServers(ctx context.Context) ([]CachedServer, error) {
	if tc == nil || tc.db == nil {
		return nil, nil
	}
	rows, err := tc.db.QueryContext(ctx, `
SELECT tenant_id, server_url, tools, last_discovered, status, COALESCE(error_message, '')
FROM mcp_server_cache
ORDER BY last_discovered ASC
`)
	if err != nil {
		return nil, fmt.Errorf("list all mcp servers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanCachedServers(rows)
}

func (tc *ToolCache) Delete(ctx context.Context, tenantID uuid.UUID, serverURL string) error {
	if tc == nil || tc.db == nil {
		return nil
	}
	_, err := tc.db.ExecContext(ctx, `
DELETE FROM mcp_server_cache
WHERE tenant_id = $1 AND server_url = $2
`, tenantID, strings.TrimSpace(serverURL))
	if err != nil {
		return fmt.Errorf("delete mcp server cache: %w", err)
	}
	return nil
}

func scanCachedServers(rows *sql.Rows) ([]CachedServer, error) {
	out := []CachedServer{}
	for rows.Next() {
		var (
			item     CachedServer
			toolsRaw []byte
		)
		if err := rows.Scan(&item.TenantID, &item.ServerURL, &toolsRaw, &item.LastDiscovered, &item.Status, &item.ErrorMessage); err != nil {
			return nil, fmt.Errorf("scan mcp server cache row: %w", err)
		}
		tools, err := decodeTools(toolsRaw)
		if err != nil {
			return nil, err
		}
		item.Tools = tools
		item.LastDiscoveredAge = time.Since(item.LastDiscovered)
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mcp server cache rows: %w", err)
	}
	return out, nil
}

func decodeTools(raw []byte) ([]MCPTool, error) {
	if len(raw) == 0 {
		return []MCPTool{}, nil
	}
	tools := []MCPTool{}
	if err := json.Unmarshal(raw, &tools); err != nil {
		return nil, fmt.Errorf("decode mcp tools cache: %w", err)
	}
	return tools, nil
}
