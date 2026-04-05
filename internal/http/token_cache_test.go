package httpfw

import (
	"testing"
	"time"
)

func TestTokenCacheGetSetInvalidate(t *testing.T) {
	cache := NewTokenCache()
	key := tokenKey{TenantID: "t1", TokenEndpoint: "https://idp/token", ClientID: "client"}

	if _, ok := cache.Get(key); ok {
		t.Fatal("expected missing token")
	}

	cache.Set(key, "abc", 2*time.Minute)
	if token, ok := cache.Get(key); !ok || token != "abc" {
		t.Fatalf("unexpected token state: %q %v", token, ok)
	}

	cache.Invalidate(key)
	if _, ok := cache.Get(key); ok {
		t.Fatal("expected invalidated token")
	}
}

func TestTokenCacheExpiryBuffer(t *testing.T) {
	cache := NewTokenCache()
	key := tokenKey{TenantID: "t1", TokenEndpoint: "https://idp/token", ClientID: "client"}

	cache.Set(key, "abc", 30*time.Second)
	if _, ok := cache.Get(key); ok {
		t.Fatal("expected token to be considered expired due to safety buffer")
	}
}
