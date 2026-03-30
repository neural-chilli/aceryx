package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/neural-chilli/aceryx/internal/observability"
)

func DoJSONRequest(ctx context.Context, method string, url string, headers map[string]string, body any, timeout time.Duration) (int, http.Header, []byte, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	requestBody := []byte{}
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("marshal request body: %w", err)
		}
		requestBody = raw
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(requestBody))
	if err != nil {
		return 0, nil, nil, fmt.Errorf("build request: %w", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if cid := observability.CorrelationIDFromContext(ctx); cid != "" {
		req.Header.Set(observability.CorrelationHeader, cid)
	}

	client := &http.Client{Timeout: timeout}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer func() { _ = res.Body.Close() }()
	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, res.Header, nil, fmt.Errorf("read response body: %w", err)
	}
	return res.StatusCode, res.Header, payload, nil
}
