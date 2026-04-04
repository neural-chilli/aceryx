package hostfns

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestHTTPRequestBlocksPrivateIP(t *testing.T) {
	h := NewHTTPHost(http.DefaultClient, nil, 60*time.Second)
	_, err := h.HTTPRequest(http.MethodGet, "http://192.168.1.1/internal", nil, nil, 1000)
	if err == nil {
		t.Fatal("expected private IP block error")
	}
}

func TestHTTPRequestAllowlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`x`))
	}))
	defer srv.Close()

	h := NewHTTPHost(http.DefaultClient, []string{"example.com"}, 60*time.Second)
	_, err := h.HTTPRequest(http.MethodGet, srv.URL, nil, nil, 1000)
	if err == nil {
		t.Fatal("expected domain not allowed error")
	}
}

func TestHTTPRequestReturnsNon2xxWithoutError(t *testing.T) {
	fake := HTTPClientFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"X-Test": []string{"1"}},
			Body:       io.NopCloser(strings.NewReader("bad")),
		}, nil
	})
	h := NewHTTPHost(fake, nil, 60*time.Second)
	h.DialLookupFunc = func(_ context.Context, _ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("93.184.216.34")}, nil
	}
	resp, err := h.HTTPRequest(http.MethodGet, "https://example.com/test", nil, nil, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

type HTTPClientFunc func(req *http.Request) (*http.Response, error)

func (f HTTPClientFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }
