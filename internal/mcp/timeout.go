package mcp

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

func CascadeTimeout(ctx context.Context, configuredTimeoutMS int) (context.Context, context.CancelFunc) {
	if configuredTimeoutMS <= 0 {
		configuredTimeoutMS = int(defaultRequestTimeout / time.Millisecond)
	}
	toolTimeout := time.Duration(configuredTimeoutMS) * time.Millisecond
	if deadline, hasDeadline := ctx.Deadline(); hasDeadline {
		remaining := time.Until(deadline)
		if remaining < toolTimeout {
			toolTimeout = remaining
		}
	}
	if toolTimeout <= 0 {
		toolTimeout = 1 * time.Millisecond
	}
	return context.WithTimeout(ctx, toolTimeout)
}

func setTimeoutHeader(req *http.Request, timeout time.Duration) {
	if req == nil {
		return
	}
	if timeout < 0 {
		timeout = 0
	}
	req.Header.Set("X-MCP-Timeout-Ms", strconv.FormatInt(timeout.Milliseconds(), 10))
}
