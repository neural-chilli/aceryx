package httpfw

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	nethttp "net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ClientManager manages shared HTTP transport and request policy.
type ClientManager struct {
	client      *nethttp.Client
	authManager *AuthManager
	rateLimiter *RateLimitManager
	validator   *URLValidator
	config      ClientConfig
}

type ClientConfig struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	MaxResponseBodySize int64
	SystemMaxTimeout    time.Duration
}

func NewClientManager(config ClientConfig) *ClientManager {
	cfg := applyConfigDefaults(config)
	transport := &nethttp.Transport{
		MaxIdleConns:        cfg.MaxIdleConns,
		MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
		IdleConnTimeout:     cfg.IdleConnTimeout,
	}
	return &ClientManager{
		client:      &nethttp.Client{Transport: transport},
		authManager: NewAuthManager(nil),
		rateLimiter: NewRateLimitManager(RateLimitDefaults{}),
		validator:   NewURLValidator(true),
		config:      cfg,
	}
}

func (cm *ClientManager) SetHTTPClient(client *nethttp.Client) {
	if cm == nil || client == nil {
		return
	}
	cm.client = client
}

func (cm *ClientManager) SetAuthManager(auth *AuthManager) {
	if cm == nil || auth == nil {
		return
	}
	cm.authManager = auth
}

func (cm *ClientManager) SetRateLimiter(rlm *RateLimitManager) {
	if cm == nil || rlm == nil {
		return
	}
	cm.rateLimiter = rlm
}

func (cm *ClientManager) SetValidator(validator *URLValidator) {
	if cm == nil || validator == nil {
		return
	}
	cm.validator = validator
}

// Execute validates URL, injects auth, applies rate limits, performs request, and enforces response size limits.
func (cm *ClientManager) Execute(ctx context.Context, req PluginHTTPRequest) (PluginHTTPResponse, error) {
	if cm == nil {
		return PluginHTTPResponse{}, fmt.Errorf("client manager not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := cm.validator.Validate(req.TenantID, req.URL); err != nil {
		return PluginHTTPResponse{}, err
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = nethttp.MethodGet
	}

	effectiveTimeout := cm.config.SystemMaxTimeout
	if req.TimeoutMS > 0 {
		requested := time.Duration(req.TimeoutMS) * time.Millisecond
		if requested < effectiveTimeout {
			effectiveTimeout = requested
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, effectiveTimeout)
	defer cancel()

	httpReq, err := nethttp.NewRequestWithContext(callCtx, method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return PluginHTTPResponse{}, fmt.Errorf("build request: %w", err)
	}
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}
	if err := cm.authManager.InjectAuth(callCtx, req.TenantID, httpReq, req.AuthConfig); err != nil {
		return PluginHTTPResponse{}, err
	}

	domain, err := extractDomain(req.URL)
	if err != nil {
		return PluginHTTPResponse{}, err
	}
	if err := cm.rateLimiter.Wait(callCtx, req.TenantID, req.PluginID, domain); err != nil {
		return PluginHTTPResponse{}, err
	}

	started := time.Now()
	resp, err := cm.client.Do(httpReq)
	if err != nil {
		return PluginHTTPResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, truncated, readErr := cm.readResponseBody(resp.Body)
	if readErr != nil {
		return PluginHTTPResponse{}, readErr
	}
	if truncated {
		slog.Warn("plugin HTTP response truncated", "tenant_id", req.TenantID, "plugin_id", req.PluginID, "url", req.URL, "max_bytes", cm.config.MaxResponseBodySize)
	}

	if resp.StatusCode == nethttp.StatusTooManyRequests {
		if until, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
			cm.rateLimiter.PauseDomain(req.TenantID, domain, until)
		}
	}

	headers := make(map[string]string, len(resp.Header))
	for key, values := range resp.Header {
		headers[key] = strings.Join(values, ",")
	}

	return PluginHTTPResponse{
		Status:     resp.StatusCode,
		StatusText: resp.Status,
		Headers:    headers,
		Body:       body,
		DurationMS: int(time.Since(started).Milliseconds()),
	}, nil
}

func applyConfigDefaults(config ClientConfig) ClientConfig {
	if config.MaxIdleConns <= 0 {
		config.MaxIdleConns = 1000
	}
	if config.MaxIdleConnsPerHost <= 0 {
		config.MaxIdleConnsPerHost = 100
	}
	if config.IdleConnTimeout <= 0 {
		config.IdleConnTimeout = 90 * time.Second
	}
	if config.MaxResponseBodySize <= 0 {
		config.MaxResponseBodySize = 10 << 20
	}
	if config.SystemMaxTimeout <= 0 {
		config.SystemMaxTimeout = 60 * time.Second
	}
	return config
}

func (cm *ClientManager) readResponseBody(body io.Reader) ([]byte, bool, error) {
	payload, err := io.ReadAll(io.LimitReader(body, cm.config.MaxResponseBodySize+1))
	if err != nil {
		return nil, false, fmt.Errorf("read response body: %w", err)
	}
	if int64(len(payload)) > cm.config.MaxResponseBodySize {
		return payload[:cm.config.MaxResponseBodySize], true, nil
	}
	return payload, false, nil
}

func parseRetryAfter(raw string, now time.Time) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		return now.Add(time.Duration(seconds) * time.Second), true
	}
	if parsed, err := nethttp.ParseTime(raw); err == nil {
		return parsed, true
	}
	return time.Time{}, false
}

func extractDomain(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("invalid URL: missing host")
	}
	return host, nil
}
