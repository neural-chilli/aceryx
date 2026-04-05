package httpfw

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newTestClientManager(rt roundTripFunc) *ClientManager {
	cm := NewClientManager(ClientConfig{MaxResponseBodySize: 1024, SystemMaxTimeout: 2 * time.Second})
	cm.SetHTTPClient(&http.Client{Transport: rt})
	validator := NewURLValidator(true)
	validator.resolveHost = func(_ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	cm.SetValidator(validator)
	cm.SetAuthManager(NewAuthManager(&mockSecretStore{values: map[string]string{"t1:api": "k1"}}))
	return cm
}

func TestClientManagerExecuteLifecycle(t *testing.T) {
	cm := newTestClientManager(func(r *http.Request) (*http.Response, error) {
		if got := r.Header.Get("X-API-Key"); got != "k1" {
			t.Fatalf("missing auth header, got %q", got)
		}
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Status:     "202 Accepted",
			Header:     http.Header{"X-Test": []string{"1"}},
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	resp, err := cm.Execute(context.Background(), PluginHTTPRequest{
		TenantID:   "t1",
		PluginID:   "p1",
		Method:     http.MethodGet,
		URL:        "https://api.example.com/data",
		AuthConfig: &AuthConfig{Type: "api_key", HeaderName: "X-API-Key", SecretRef: "api"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Status != http.StatusAccepted {
		t.Fatalf("unexpected status: %d", resp.Status)
	}
	if string(resp.Body) != "ok" {
		t.Fatalf("unexpected body: %q", string(resp.Body))
	}
}

func TestClientManagerResponseTruncation(t *testing.T) {
	cm := NewClientManager(ClientConfig{MaxResponseBodySize: 5, SystemMaxTimeout: 2 * time.Second})
	cm.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader("0123456789"))}, nil
	})})
	validator := NewURLValidator(true)
	validator.resolveHost = func(_ string) ([]netip.Addr, error) { return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil }
	cm.SetValidator(validator)

	resp, err := cm.Execute(context.Background(), PluginHTTPRequest{TenantID: "t1", PluginID: "p1", Method: http.MethodGet, URL: "https://api.example.com"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := string(resp.Body); got != "01234" {
		t.Fatalf("expected truncated body, got %q", got)
	}
}

func TestClientManagerErrorSemantics(t *testing.T) {
	cm := NewClientManager(ClientConfig{SystemMaxTimeout: 100 * time.Millisecond})
	validator := NewURLValidator(true)
	validator.resolveHost = func(_ string) ([]netip.Addr, error) { return nil, errors.New("dns failure") }
	cm.SetValidator(validator)
	if _, err := cm.Execute(context.Background(), PluginHTTPRequest{TenantID: "t1", PluginID: "p1", Method: http.MethodGet, URL: "https://missing.example.com"}); err == nil {
		t.Fatal("expected transport-style error")
	}

	var calls int32
	cm2 := newTestClientManager(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/not-found" {
			return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		atomic.AddInt32(&calls, 1)
		return &http.Response{StatusCode: http.StatusBadGateway, Status: "502 Bad Gateway", Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})

	resp404, err := cm2.Execute(context.Background(), PluginHTTPRequest{TenantID: "t1", PluginID: "p1", Method: http.MethodGet, URL: "https://api.example.com/not-found"})
	if err != nil {
		t.Fatalf("expected 404 response without error: %v", err)
	}
	if resp404.Status != http.StatusNotFound {
		t.Fatalf("unexpected 404 status: %d", resp404.Status)
	}

	resp502, err := cm2.Execute(context.Background(), PluginHTTPRequest{TenantID: "t1", PluginID: "p1", Method: http.MethodGet, URL: "https://api.example.com/bad-gateway"})
	if err != nil {
		t.Fatalf("expected 5xx response without error: %v", err)
	}
	if resp502.Status != http.StatusBadGateway {
		t.Fatalf("unexpected 5xx status: %d", resp502.Status)
	}
}

func TestClientManagerRetryAfterDomainPause(t *testing.T) {
	var calls int32
	cm := newTestClientManager(func(_ *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&calls, 1) == 1 {
			return &http.Response{StatusCode: http.StatusTooManyRequests, Status: "429 Too Many Requests", Header: http.Header{"Retry-After": []string{"1"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})

	first, err := cm.Execute(context.Background(), PluginHTTPRequest{TenantID: "t1", PluginID: "p1", Method: http.MethodGet, URL: "https://api.example.com/data"})
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	if first.Status != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", first.Status)
	}

	start := time.Now()
	second, err := cm.Execute(context.Background(), PluginHTTPRequest{TenantID: "t1", PluginID: "p1", Method: http.MethodGet, URL: "https://api.example.com/data"})
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	if second.Status != http.StatusOK {
		t.Fatalf("expected 200, got %d", second.Status)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Fatalf("expected pause delay from Retry-After, got %v", elapsed)
	}
}

func TestClientManagerConcurrentRequests(t *testing.T) {
	cm := newTestClientManager(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := cm.Execute(context.Background(), PluginHTTPRequest{TenantID: "t1", PluginID: "p1", Method: http.MethodGet, URL: "https://api.example.com/data"})
			if err != nil {
				t.Errorf("Execute error: %v", err)
				return
			}
			if resp.Status != http.StatusOK {
				t.Errorf("unexpected status: %d", resp.Status)
			}
		}()
	}
	wg.Wait()
}
