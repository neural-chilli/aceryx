package rbac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidToken      = errors.New("invalid token")
	ErrExpiredToken      = errors.New("token expired")
	ErrInvalidCredential = errors.New("invalid credentials")
)

type tokenClaims struct {
	PrincipalID uuid.UUID `json:"principal_id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	SessionID   uuid.UUID `json:"session_id"`
	ExpiresAt   int64     `json:"exp"`
}

func signJWT(secret []byte, claims tokenClaims) (string, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerRaw, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}
	claimsRaw, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal jwt claims: %w", err)
	}

	enc := base64.RawURLEncoding
	headerPart := enc.EncodeToString(headerRaw)
	claimsPart := enc.EncodeToString(claimsRaw)
	unsigned := headerPart + "." + claimsPart

	h := hmac.New(sha256.New, secret)
	_, _ = h.Write([]byte(unsigned))
	sig := enc.EncodeToString(h.Sum(nil))
	return unsigned + "." + sig, nil
}

func parseAndVerifyJWT(secret []byte, token string, now time.Time) (tokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return tokenClaims{}, ErrInvalidToken
	}

	unsigned := parts[0] + "." + parts[1]
	enc := base64.RawURLEncoding
	h := hmac.New(sha256.New, secret)
	_, _ = h.Write([]byte(unsigned))
	expectedSig := enc.EncodeToString(h.Sum(nil))
	if !hmac.Equal([]byte(expectedSig), []byte(parts[2])) {
		return tokenClaims{}, ErrInvalidToken
	}

	claimsBytes, err := enc.DecodeString(parts[1])
	if err != nil {
		return tokenClaims{}, ErrInvalidToken
	}
	var claims tokenClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return tokenClaims{}, ErrInvalidToken
	}
	if claims.ExpiresAt <= now.Unix() {
		return tokenClaims{}, ErrExpiredToken
	}
	if claims.PrincipalID == uuid.Nil || claims.TenantID == uuid.Nil || claims.SessionID == uuid.Nil {
		return tokenClaims{}, ErrInvalidToken
	}

	return claims, nil
}
