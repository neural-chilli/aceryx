package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	defaultRequestTimeout = 30 * time.Second
)

type Client struct {
	serverURL  string
	httpClient *http.Client
	auth       AuthConfig
}

func NewClient(serverURL string, httpClient *http.Client, auth AuthConfig) *Client {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: defaultRequestTimeout}
	}
	return &Client{serverURL: strings.TrimSpace(serverURL), httpClient: client, auth: auth}
}

func (c *Client) Discover(ctx context.Context) ([]MCPTool, error) {
	resp, err := c.sendRPC(ctx, jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      1,
	})
	if err != nil {
		return nil, err
	}
	var payload struct {
		Tools []MCPTool `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &payload); err != nil {
		return nil, fmt.Errorf("parse tools/list result: %w", err)
	}
	return payload.Tools, nil
}

func (c *Client) Invoke(ctx context.Context, toolName string, arguments json.RawMessage) (MCPToolResult, error) {
	if strings.TrimSpace(toolName) == "" {
		return MCPToolResult{}, fmt.Errorf("tool name is required")
	}
	if len(arguments) == 0 {
		arguments = []byte("{}")
	}
	resp, err := c.sendRPC(ctx, jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/call",
		ID:      2,
		Params: map[string]any{
			"name":      toolName,
			"arguments": json.RawMessage(arguments),
		},
	})
	if err != nil {
		return MCPToolResult{}, err
	}
	var result MCPToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return MCPToolResult{}, fmt.Errorf("parse tools/call result: %w", err)
	}
	return result, nil
}

func (c *Client) sendRPC(ctx context.Context, payload jsonRPCRequest) (jsonRPCResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return jsonRPCResponse{}, fmt.Errorf("marshal json-rpc request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL, bytes.NewReader(body))
	if err != nil {
		return jsonRPCResponse{}, fmt.Errorf("build mcp request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	setDepthHeader(req, getDepthFromContext(ctx))
	if deadline, ok := ctx.Deadline(); ok {
		setTimeoutHeader(req, time.Until(deadline))
	}
	c.applyAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if isUnreachableErr(err) {
			return jsonRPCResponse{}, fmt.Errorf("MCP server unreachable: %s", c.serverURL)
		}
		return jsonRPCResponse{}, fmt.Errorf("mcp request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 500 {
		return jsonRPCResponse{}, fmt.Errorf("mcp server error: status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return jsonRPCResponse{}, fmt.Errorf("mcp request failed: status %d body %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.Contains(contentType, "text/event-stream") {
		return decodeSSEResponse(resp.Body)
	}
	return decodeJSONRPCResponse(resp.Body)
}

func (c *Client) applyAuth(req *http.Request) {
	authType := strings.ToLower(strings.TrimSpace(c.auth.Type))
	secret := strings.TrimSpace(c.auth.SecretRef)
	if authType == "" || authType == "none" || secret == "" {
		return
	}
	switch authType {
	case "bearer", "oauth2":
		req.Header.Set("Authorization", "Bearer "+secret)
	case "api_key":
		header := strings.TrimSpace(c.auth.HeaderName)
		if header == "" {
			header = "X-API-Key"
		}
		req.Header.Set(header, secret)
	}
}

func decodeJSONRPCResponse(r io.Reader) (jsonRPCResponse, error) {
	var resp jsonRPCResponse
	if err := json.NewDecoder(r).Decode(&resp); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("invalid JSON-RPC response: %w", err)
	}
	if resp.Error != nil {
		return jsonRPCResponse{}, fmt.Errorf("mcp rpc error: %s", strings.TrimSpace(resp.Error.Message))
	}
	if strings.TrimSpace(resp.JSONRPC) != "2.0" {
		return jsonRPCResponse{}, fmt.Errorf("invalid JSON-RPC response: missing jsonrpc=2.0")
	}
	return resp, nil
}

func decodeSSEResponse(r io.Reader) (jsonRPCResponse, error) {
	scanner := bufio.NewScanner(r)
	var dataLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if len(dataLines) == 0 {
				continue
			}
			joined := strings.Join(dataLines, "\n")
			return decodeJSONRPCResponse(strings.NewReader(joined))
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return jsonRPCResponse{}, fmt.Errorf("read sse stream: %w", err)
	}
	return jsonRPCResponse{}, fmt.Errorf("invalid JSON-RPC response: no SSE data event")
}

func isUnreachableErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      int    `json:"id"`
	Params  any    `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
