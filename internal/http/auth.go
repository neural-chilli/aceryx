package httpfw

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

type SecretStore interface {
	Get(ctx context.Context, tenantID, key string) (string, error)
}

type AuthManager struct {
	secretStore SecretStore
	tokenCache  *TokenCache
	oauthClient *http.Client
	flight      singleflight.Group

	mu sync.RWMutex
}

func NewAuthManager(secretStore SecretStore) *AuthManager {
	return &AuthManager{
		secretStore: secretStore,
		tokenCache:  NewTokenCache(),
		oauthClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (am *AuthManager) SetTokenCache(cache *TokenCache) {
	if am == nil || cache == nil {
		return
	}
	am.mu.Lock()
	am.tokenCache = cache
	am.mu.Unlock()
}

func (am *AuthManager) SetOAuthClient(client *http.Client) {
	if am == nil || client == nil {
		return
	}
	am.mu.Lock()
	am.oauthClient = client
	am.mu.Unlock()
}

// InjectAuth modifies the request to add authentication headers.
func (am *AuthManager) InjectAuth(ctx context.Context, tenantID string, req *http.Request, config *AuthConfig) error {
	if config == nil || strings.TrimSpace(config.Type) == "" || strings.EqualFold(config.Type, "none") {
		return nil
	}
	if req == nil {
		return fmt.Errorf("request is nil")
	}

	authType := strings.ToLower(strings.TrimSpace(config.Type))
	switch authType {
	case "api_key":
		secret, err := am.getSecret(ctx, tenantID, config.SecretRef)
		if err != nil {
			return err
		}
		header := strings.TrimSpace(config.HeaderName)
		if header == "" {
			header = "X-API-Key"
		}
		req.Header.Set(header, secret)
		return nil

	case "bearer":
		token, err := am.getSecret(ctx, tenantID, config.SecretRef)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil

	case "basic":
		user, err := am.getSecret(ctx, tenantID, config.UsernameRef)
		if err != nil {
			return err
		}
		pass, err := am.getSecret(ctx, tenantID, config.PasswordRef)
		if err != nil {
			return err
		}
		payload := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
		req.Header.Set("Authorization", "Basic "+payload)
		return nil

	case "oauth2_client_credentials":
		accessToken, err := am.resolveOAuthToken(ctx, tenantID, config)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		return nil

	case "hmac":
		secret, err := am.getSecret(ctx, tenantID, config.SecretRef)
		if err != nil {
			return err
		}
		header := strings.TrimSpace(config.SignatureHeader)
		if header == "" {
			header = "X-Signature"
		}
		sig, err := computeHMACSignature(req, config.Algorithm, secret)
		if err != nil {
			return err
		}
		req.Header.Set(header, sig)
		return nil

	default:
		return fmt.Errorf("unsupported auth type: %s", config.Type)
	}
}

func (am *AuthManager) resolveOAuthToken(ctx context.Context, tenantID string, cfg *AuthConfig) (string, error) {
	clientID, err := am.getSecret(ctx, tenantID, cfg.ClientIDRef)
	if err != nil {
		return "", err
	}
	clientSecret, err := am.getSecret(ctx, tenantID, cfg.ClientSecretRef)
	if err != nil {
		return "", err
	}
	endpoint := strings.TrimSpace(cfg.TokenEndpoint)
	if endpoint == "" {
		return "", fmt.Errorf("oauth2 token endpoint is required")
	}
	key := tokenKey{TenantID: tenantID, TokenEndpoint: endpoint, ClientID: clientID}

	cache := am.getTokenCache()
	if token, ok := cache.Get(key); ok {
		return token, nil
	}

	flightKey := tenantID + "|" + endpoint + "|" + clientID
	result, err, _ := am.flight.Do(flightKey, func() (any, error) {
		if token, ok := cache.Get(key); ok {
			return token, nil
		}
		token, expiresIn, tokenErr := am.fetchOAuthToken(ctx, endpoint, clientID, clientSecret, cfg.Scopes)
		if tokenErr != nil {
			return "", tokenErr
		}
		cache.Set(key, token, expiresIn)
		return token, nil
	})
	if err != nil {
		return "", err
	}
	out, _ := result.(string)
	if out == "" {
		return "", fmt.Errorf("oauth2 token acquisition returned empty token")
	}
	return out, nil
}

func (am *AuthManager) fetchOAuthToken(ctx context.Context, endpoint, clientID, clientSecret string, scopes []string) (string, time.Duration, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	if len(scopes) > 0 {
		form.Set("scope", strings.Join(scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := am.getOAuthClient().Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("oauth2 token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", 0, fmt.Errorf("oauth2 token endpoint failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var parsed struct {
		AccessToken string      `json:"access_token"`
		ExpiresIn   interface{} `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", 0, fmt.Errorf("decode oauth2 token response: %w", err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", 0, fmt.Errorf("oauth2 token response missing access_token")
	}
	expiresIn := parseExpiresIn(parsed.ExpiresIn)
	if expiresIn <= 0 {
		expiresIn = 3600 * time.Second
	}
	return parsed.AccessToken, expiresIn, nil
}

func parseExpiresIn(raw interface{}) time.Duration {
	switch v := raw.(type) {
	case float64:
		return time.Duration(v) * time.Second
	case int:
		return time.Duration(v) * time.Second
	case int64:
		return time.Duration(v) * time.Second
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0
		}
		return time.Duration(n) * time.Second
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0
		}
		return time.Duration(n) * time.Second
	default:
		return 0
	}
}

func (am *AuthManager) getSecret(ctx context.Context, tenantID, key string) (string, error) {
	if am == nil || am.secretStore == nil {
		return "", fmt.Errorf("secret store unavailable")
	}
	if strings.TrimSpace(key) == "" {
		return "", fmt.Errorf("secret reference required")
	}
	value, err := am.secretStore.Get(ctx, tenantID, key)
	if err != nil {
		return "", fmt.Errorf("resolve secret %s: %w", key, err)
	}
	if value == "" {
		return "", fmt.Errorf("resolve secret %s: empty value", key)
	}
	return value, nil
}

func (am *AuthManager) getTokenCache() *TokenCache {
	am.mu.RLock()
	cache := am.tokenCache
	am.mu.RUnlock()
	if cache == nil {
		return NewTokenCache()
	}
	return cache
}

func (am *AuthManager) getOAuthClient() *http.Client {
	am.mu.RLock()
	client := am.oauthClient
	am.mu.RUnlock()
	if client == nil {
		return http.DefaultClient
	}
	return client
}

func computeHMACSignature(req *http.Request, algorithm, secret string) (string, error) {
	hasher, err := hmacHasher(algorithm, secret)
	if err != nil {
		return "", err
	}

	payload := []byte{}
	switch {
	case req == nil:
		return "", fmt.Errorf("request is nil")
	case req.GetBody != nil:
		copyBody, bodyErr := req.GetBody()
		if bodyErr != nil {
			return "", fmt.Errorf("read request body for hmac: %w", bodyErr)
		}
		defer func() { _ = copyBody.Close() }()
		payload, err = io.ReadAll(copyBody)
		if err != nil {
			return "", fmt.Errorf("read request body for hmac: %w", err)
		}
	case req.Body != nil:
		payload, err = io.ReadAll(req.Body)
		if err != nil {
			return "", fmt.Errorf("read request body for hmac: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(payload))
	}
	return hasher(payload), nil
}

func hmacHasher(algorithm string, secret string) (func([]byte) string, error) {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case "", "sha256":
		return func(payload []byte) string {
			h := hmac.New(sha256.New, []byte(secret))
			_, _ = h.Write(payload)
			return hex.EncodeToString(h.Sum(nil))
		}, nil
	case "sha512":
		return func(payload []byte) string {
			h := hmac.New(sha512.New, []byte(secret))
			_, _ = h.Write(payload)
			return hex.EncodeToString(h.Sum(nil))
		}, nil
	default:
		return nil, fmt.Errorf("unsupported hmac algorithm: %s", algorithm)
	}
}
