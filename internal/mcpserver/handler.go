package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/neural-chilli/aceryx/internal/observability"
)

const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
	rpcInternalError  = -32603
	rpcRateLimited    = -32029
)

type TenantFeatureAllEnabled struct{}

func (TenantFeatureAllEnabled) IsRAGEnabled(uuid.UUID) bool { return true }

type Handler struct {
	tools          map[string]ToolHandler
	toolList       []ToolDefinition
	auth           *AuthMiddleware
	rateLimiter    *RateLimiter
	audit          *AuditLogger
	config         ServerConfig
	tenantFeatures TenantFeatureChecker
}

func NewHandler(cfg ServerConfig, tools []ToolHandler, auth *AuthMiddleware, limiter *RateLimiter, audit *AuditLogger) *Handler {
	cfg = cfg.WithDefaults()
	toolMap := make(map[string]ToolHandler, len(tools))
	toolList := make([]ToolDefinition, 0, len(tools))
	for _, t := range tools {
		if t == nil {
			continue
		}
		toolMap[t.Name()] = t
		toolList = append(toolList, t.Definition())
	}
	return &Handler{
		tools:          toolMap,
		toolList:       toolList,
		auth:           auth,
		rateLimiter:    limiter,
		audit:          audit,
		config:         cfg,
		tenantFeatures: TenantFeatureAllEnabled{},
	}
}

func (h *Handler) SetTenantFeatureChecker(checker TenantFeatureChecker) {
	if checker == nil {
		return
	}
	h.tenantFeatures = checker
}

func (h *Handler) SetConfig(cfg ServerConfig) {
	h.config = cfg.WithDefaults()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeRPCErrorHTTP(w, http.StatusMethodNotAllowed, nil, rpcInvalidRequest, "method not allowed")
		return
	}

	depth := ReadDepth(r)
	if err := CheckDepth(depth, h.config.MaxDepth); err != nil {
		writeRPCErrorHTTP(w, http.StatusBadRequest, nil, rpcInvalidRequest, err.Error())
		return
	}
	if depth > 0 && isLoopbackRequest(r) {
		slog.WarnContext(r.Context(), "mcp self-call detected", "depth", depth, "remote_addr", r.RemoteAddr)
		depth++
	}

	headerTimeout := ReadTimeout(r)
	ctx, cancel := ApplyTimeout(r.Context(), headerTimeout, h.config.MaxRequestTimeout)
	defer cancel()

	correlationID := strings.TrimSpace(r.Header.Get(observability.CorrelationHeader))
	if correlationID == "" {
		correlationID = uuid.NewString()
	}
	w.Header().Set(observability.CorrelationHeader, correlationID)
	ctx = observability.WithCorrelationID(ctx, correlationID)

	conn, err := h.auth.Authenticate(r)
	if err != nil {
		writeRPCErrorHTTP(w, http.StatusUnauthorized, nil, rpcInvalidRequest, "unauthorized")
		return
	}
	ctx = observability.WithTenantID(ctx, conn.TenantID)
	ctx = observability.WithPrincipalID(ctx, conn.UserID)

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCErrorHTTP(w, http.StatusBadRequest, nil, rpcParseError, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.JSONRPC) != "2.0" || strings.TrimSpace(req.Method) == "" {
		writeRPCErrorHTTP(w, http.StatusBadRequest, req.ID, rpcInvalidRequest, "invalid JSON-RPC request")
		return
	}

	reqCtx := context.WithValue(ctx, correlationIDCtxKey{}, correlationID)
	reqCtx = context.WithValue(reqCtx, depthCtxKey{}, depth)
	resp := h.handleRPC(reqCtx, req, conn)

	if acceptsSSE(r) {
		writeRPCSSE(w, resp)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleRPC(ctx context.Context, req JSONRPCRequest, conn *Connection) JSONRPCResponse {
	base := JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "tools/list":
		return JSONRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": h.visibleTools(conn)}}
	case "tools/call":
		var payload struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &payload); err != nil {
			base.Error = &JSONRPCError{Code: rpcInvalidParams, Message: "invalid params"}
			return base
		}
		if payload.Name == "" {
			base.Error = &JSONRPCError{Code: rpcInvalidParams, Message: "tool name is required"}
			return base
		}
		if len(payload.Arguments) == 0 {
			payload.Arguments = json.RawMessage(`{}`)
		}
		tool, ok := h.tools[payload.Name]
		if !ok || h.isToolDisabled(payload.Name) {
			base.Error = &JSONRPCError{Code: rpcMethodNotFound, Message: "tool not found"}
			return base
		}
		if !hasPermission(conn, tool.RequiredPermission()) {
			base.Error = &JSONRPCError{Code: rpcInvalidRequest, Message: "permission denied"}
			h.logInvocation(ctx, conn, payload.Name, payload.Arguments, "permission_denied", 0)
			return base
		}
		if err := h.rateLimiter.Allow(conn.APIKeyID, payload.Name); err != nil {
			base.Error = &JSONRPCError{Code: rpcRateLimited, Message: "rate limit exceeded"}
			h.logInvocation(ctx, conn, payload.Name, payload.Arguments, "rate_limited", 0)
			return base
		}

		start := time.Now()
		result, err := tool.Execute(ctx, conn, payload.Arguments)
		durationMS := int(time.Since(start).Milliseconds())
		if err != nil {
			h.logInvocation(ctx, conn, payload.Name, payload.Arguments, "error", durationMS)
			base.Error = &JSONRPCError{Code: rpcInternalError, Message: err.Error()}
			return base
		}
		h.logInvocation(ctx, conn, payload.Name, payload.Arguments, "success", durationMS)
		encoded, err := json.Marshal(result)
		if err != nil {
			base.Error = &JSONRPCError{Code: rpcInternalError, Message: "failed to marshal tool result"}
			return base
		}
		base.Result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": string(encoded)}},
			"isError": false,
		}
		return base
	default:
		base.Error = &JSONRPCError{Code: rpcMethodNotFound, Message: "method not found"}
		return base
	}
}

type correlationIDCtxKey struct{}
type depthCtxKey struct{}

func (h *Handler) logInvocation(ctx context.Context, conn *Connection, toolName string, args json.RawMessage, status string, durationMS int) {
	if h.audit == nil || conn == nil {
		return
	}
	correlationID, _ := ctx.Value(correlationIDCtxKey{}).(string)
	depth, _ := ctx.Value(depthCtxKey{}).(int)
	_ = h.audit.LogInvocation(ctx, AuditEntry{
		TenantID:      conn.TenantID,
		UserID:        conn.UserID,
		APIKeyID:      conn.APIKeyID,
		ToolName:      toolName,
		Arguments:     args,
		ResultStatus:  status,
		DurationMS:    durationMS,
		Source:        "mcp",
		CorrelationID: correlationID,
		Depth:         depth,
	})
}

func writeRPCErrorHTTP(w http.ResponseWriter, status int, id json.RawMessage, code int, msg string) {
	resp := JSONRPCResponse{JSONRPC: "2.0", ID: id, Error: &JSONRPCError{Code: code, Message: msg}}
	if bytes.Equal(bytes.TrimSpace(id), nil) || len(bytes.TrimSpace(id)) == 0 {
		resp.ID = json.RawMessage("null")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func writeRPCSSE(w http.ResponseWriter, resp JSONRPCResponse) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	payload, _ := json.Marshal(resp)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
}

func acceptsSSE(r *http.Request) bool {
	if r == nil {
		return false
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("transport")), "sse")
}

func isLoopbackRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(strings.TrimSpace(host))
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
