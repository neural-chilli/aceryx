package httpfw

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type mockSecretStore struct {
	values map[string]string
}

func (m *mockSecretStore) Get(_ context.Context, tenantID, key string) (string, error) {
	v, ok := m.values[tenantID+":"+key]
	if !ok {
		return "", fmt.Errorf("secret not found")
	}
	return v, nil
}

func TestAuthManagerAPIKeyBearerBasicNone(t *testing.T) {
	am := NewAuthManager(&mockSecretStore{values: map[string]string{
		"t1:api_key":   "k1",
		"t1:bearer":    "b1",
		"t1:user":      "u1",
		"t1:pass":      "p1",
		"t1:hmac_key":  "secret",
		"t1:client_id": "cid",
	}})

	apiReq, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err := am.InjectAuth(context.Background(), "t1", apiReq, &AuthConfig{Type: "api_key", HeaderName: "X-API-Key", SecretRef: "api_key"}); err != nil {
		t.Fatalf("api_key: %v", err)
	}
	if got := apiReq.Header.Get("X-API-Key"); got != "k1" {
		t.Fatalf("unexpected api key header: %q", got)
	}

	bearerReq, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err := am.InjectAuth(context.Background(), "t1", bearerReq, &AuthConfig{Type: "bearer", SecretRef: "bearer"}); err != nil {
		t.Fatalf("bearer: %v", err)
	}
	if got := bearerReq.Header.Get("Authorization"); got != "Bearer b1" {
		t.Fatalf("unexpected bearer header: %q", got)
	}

	basicReq, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err := am.InjectAuth(context.Background(), "t1", basicReq, &AuthConfig{Type: "basic", UsernameRef: "user", PasswordRef: "pass"}); err != nil {
		t.Fatalf("basic: %v", err)
	}
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("u1:p1"))
	if got := basicReq.Header.Get("Authorization"); got != expected {
		t.Fatalf("unexpected basic header: %q", got)
	}

	noneReq, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err := am.InjectAuth(context.Background(), "t1", noneReq, &AuthConfig{Type: "none"}); err != nil {
		t.Fatalf("none: %v", err)
	}
}

func TestAuthManagerOAuth2CachingAndRefresh(t *testing.T) {
	var calls int32
	tokens := []string{"tok-1", "tok-2"}
	idx := int32(0)
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		current := atomic.AddInt32(&idx, 1) - 1
		resp := map[string]any{"access_token": tokens[current], "expires_in": 61}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer tokenSrv.Close()

	am := NewAuthManager(&mockSecretStore{values: map[string]string{
		"t1:cid": "client-id",
		"t1:sec": "client-secret",
	}})

	cfg := &AuthConfig{Type: "oauth2_client_credentials", TokenEndpoint: tokenSrv.URL, ClientIDRef: "cid", ClientSecretRef: "sec"}

	req1, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err := am.InjectAuth(context.Background(), "t1", req1, cfg); err != nil {
		t.Fatalf("oauth first request: %v", err)
	}
	if got := req1.Header.Get("Authorization"); got != "Bearer tok-1" {
		t.Fatalf("unexpected token on first request: %q", got)
	}

	req2, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err := am.InjectAuth(context.Background(), "t1", req2, cfg); err != nil {
		t.Fatalf("oauth cached request: %v", err)
	}
	if got := req2.Header.Get("Authorization"); got != "Bearer tok-1" {
		t.Fatalf("unexpected token on cached request: %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one token endpoint call, got %d", got)
	}

	time.Sleep(1200 * time.Millisecond)
	req3, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err := am.InjectAuth(context.Background(), "t1", req3, cfg); err != nil {
		t.Fatalf("oauth refresh request: %v", err)
	}
	if got := req3.Header.Get("Authorization"); got != "Bearer tok-2" {
		t.Fatalf("unexpected token on refresh request: %q", got)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected refresh token call, got %d", got)
	}
}

func TestAuthManagerOAuth2Failure(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad_client"}`))
	}))
	defer tokenSrv.Close()

	am := NewAuthManager(&mockSecretStore{values: map[string]string{
		"t1:cid": "client-id",
		"t1:sec": "client-secret",
	}})

	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	err := am.InjectAuth(context.Background(), "t1", req, &AuthConfig{Type: "oauth2_client_credentials", TokenEndpoint: tokenSrv.URL, ClientIDRef: "cid", ClientSecretRef: "sec"})
	if err == nil {
		t.Fatal("expected oauth2 failure")
	}
}

func TestAuthManagerHMAC(t *testing.T) {
	am := NewAuthManager(&mockSecretStore{values: map[string]string{"t1:hmac": "secret"}})
	body := []byte("payload")
	req, _ := http.NewRequest(http.MethodPost, "https://example.com", bytes.NewReader(body))

	err := am.InjectAuth(context.Background(), "t1", req, &AuthConfig{Type: "hmac", Algorithm: "sha256", SignatureHeader: "X-Signature", SecretRef: "hmac"})
	if err != nil {
		t.Fatalf("hmac: %v", err)
	}
	h := hmac.New(sha256.New, []byte("secret"))
	_, _ = h.Write(body)
	expected := hex.EncodeToString(h.Sum(nil))
	if got := req.Header.Get("X-Signature"); got != expected {
		t.Fatalf("unexpected signature: got %q want %q", got, expected)
	}
}

func TestAuthManagerOAuth2ConcurrencySingleflight(t *testing.T) {
	var calls int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	am := NewAuthManager(&mockSecretStore{values: map[string]string{
		"t1:cid": "client-id",
		"t1:sec": "client-secret",
	}})
	cfg := &AuthConfig{Type: "oauth2_client_credentials", TokenEndpoint: tokenSrv.URL, ClientIDRef: "cid", ClientSecretRef: "sec"}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
			if err := am.InjectAuth(context.Background(), "t1", req, cfg); err != nil {
				t.Errorf("InjectAuth error: %v", err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected one token request under concurrency, got %d", got)
	}
}
