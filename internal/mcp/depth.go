package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

const DefaultMaxDepth = 3

type depthCtxKey struct{}

func WithDepth(ctx context.Context, depth int) context.Context {
	return context.WithValue(ctx, depthCtxKey{}, depth)
}

func CheckDepth(depth, maxDepth int) error {
	if maxDepth <= 0 {
		maxDepth = DefaultMaxDepth
	}
	if depth > maxDepth {
		return fmt.Errorf("MCP call depth exceeded (max: %d)", maxDepth)
	}
	return nil
}

func setDepthHeader(req *http.Request, depth int) {
	if req == nil {
		return
	}
	req.Header.Set("X-MCP-Depth", strconv.Itoa(depth))
}

func getDepthFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	if v, ok := ctx.Value(depthCtxKey{}).(int); ok && v >= 0 {
		return v
	}
	return 0
}
