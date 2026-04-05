package httpfw

import (
	"sync"
	"time"
)

type TokenCache struct {
	mu     sync.RWMutex
	tokens map[tokenKey]*cachedToken
}

type tokenKey struct {
	TenantID      string
	TokenEndpoint string
	ClientID      string
}

type cachedToken struct {
	AccessToken string
	ExpiresAt   time.Time
}

func NewTokenCache() *TokenCache {
	return &TokenCache{tokens: make(map[tokenKey]*cachedToken)}
}

// Get returns a valid token if one exists, or ("", false).
func (tc *TokenCache) Get(key tokenKey) (string, bool) {
	if tc == nil {
		return "", false
	}
	tc.mu.RLock()
	token, ok := tc.tokens[key]
	tc.mu.RUnlock()
	if !ok || token == nil || !time.Now().Before(token.ExpiresAt) {
		return "", false
	}
	return token.AccessToken, true
}

// Set stores a token with its expiry time.
func (tc *TokenCache) Set(key tokenKey, token string, expiresIn time.Duration) {
	if tc == nil {
		return
	}
	effective := expiresIn - 60*time.Second
	if effective < 0 {
		effective = 0
	}
	tc.mu.Lock()
	tc.tokens[key] = &cachedToken{AccessToken: token, ExpiresAt: time.Now().Add(effective)}
	tc.mu.Unlock()
}

// Invalidate removes a cached token (e.g. on auth failure).
func (tc *TokenCache) Invalidate(key tokenKey) {
	if tc == nil {
		return
	}
	tc.mu.Lock()
	delete(tc.tokens, key)
	tc.mu.Unlock()
}
