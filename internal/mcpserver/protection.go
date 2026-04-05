package mcpserver

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func ReadDepth(r *http.Request) int {
	if r == nil {
		return 0
	}
	v := r.Header.Get("X-MCP-Depth")
	if v == "" {
		return 0
	}
	depth, err := strconv.Atoi(v)
	if err != nil || depth < 0 {
		return 0
	}
	return depth
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

func ReadTimeout(r *http.Request) time.Duration {
	if r == nil {
		return 0
	}
	v := r.Header.Get("X-MCP-Timeout-Ms")
	if v == "" {
		return 0
	}
	timeoutMS, err := strconv.Atoi(v)
	if err != nil || timeoutMS <= 0 {
		return 0
	}
	return time.Duration(timeoutMS) * time.Millisecond
}

func ApplyTimeout(ctx context.Context, headerTimeout, serverMax time.Duration) (context.Context, context.CancelFunc) {
	if serverMax <= 0 {
		serverMax = DefaultMaxToolTimeout
	}
	effective := serverMax
	if headerTimeout > 0 && headerTimeout < effective {
		effective = headerTimeout
	}
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < effective {
			effective = remaining
		}
	}
	if effective <= 0 {
		effective = 1 * time.Millisecond
	}
	return context.WithTimeout(ctx, effective)
}
