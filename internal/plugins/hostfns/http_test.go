package hostfns

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	httpfw "github.com/neural-chilli/aceryx/internal/http"
)

func TestHTTPRequestBlocksPrivateIP(t *testing.T) {
	h := NewHTTPHost(http.DefaultClient, nil, 60*time.Second)
	_, err := h.HTTPRequest(http.MethodGet, "http://192.168.1.1/internal", nil, nil, 1000)
	if err == nil {
		t.Fatal("expected private IP block error")
	}
}

func TestHTTPRequestAllowlist(t *testing.T) {
	h := NewHTTPHost(http.DefaultClient, []string{"example.com"}, 60*time.Second)
	_, err := h.HTTPRequest(http.MethodGet, "http://not-example.com", nil, nil, 1000)
	if err == nil {
		t.Fatal("expected domain not allowed error")
	}
}

func TestHTTPRequestReturnsNon2xxWithoutError(t *testing.T) {
	manager := httpfw.NewClientManager(httpfw.ClientConfig{SystemMaxTimeout: 2 * time.Second})
	manager.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Header:     http.Header{"X-Test": []string{"1"}},
				Body:       io.NopCloser(strings.NewReader("bad")),
			}, nil
		}),
	})
	validator := httpfw.NewURLValidator(true)
	validator.SetAllowlist("t1", []string{"93.184.216.34"})
	manager.SetValidator(validator)
	h := &HTTPHost{ClientManager: manager, TenantID: "t1", Ctx: context.Background()}

	resp, err := h.HTTPRequest(http.MethodGet, "https://93.184.216.34/test", nil, nil, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
