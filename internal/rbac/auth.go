package rbac

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AuthService struct {
	db           *sql.DB
	jwtSecret    []byte
	sessionTTL   time.Duration
	cleanupEvery time.Duration
}

func NewAuthService(db *sql.DB, jwtSecret string, sessionTTL time.Duration) *AuthService {
	if sessionTTL <= 0 {
		sessionTTL = DefaultSessionTTL
	}
	if jwtSecret == "" {
		jwtSecret = "aceryx-dev-secret"
	}
	return &AuthService{db: db, jwtSecret: []byte(jwtSecret), sessionTTL: sessionTTL, cleanupEvery: DefaultSessionSweepTick}
}

func (a *AuthService) SetCleanupInterval(interval time.Duration) {
	if interval > 0 {
		a.cleanupEvery = interval
	}
}

func (a *AuthService) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	tenant, err := a.resolveTenant(ctx, req.TenantID, req.TenantSlug)
	if err != nil {
		_ = recordAuthEvent(ctx, a.db, authEvent{EventType: "login", Success: false, IPAddress: req.IPAddress, UserAgent: req.UserAgent, Data: map[string]interface{}{"email": req.Email}})
		return nil, ErrInvalidCredential
	}

	principal, passwordHash, err := a.lookupPrincipalForLogin(ctx, tenant.ID, req.Email)
	if err != nil {
		_ = recordAuthEvent(ctx, a.db, authEvent{TenantID: &tenant.ID, EventType: "login", Success: false, IPAddress: req.IPAddress, UserAgent: req.UserAgent, Data: map[string]interface{}{"email": req.Email}})
		return nil, ErrInvalidCredential
	}
	if principal.Status != "active" {
		_ = recordAuthEvent(ctx, a.db, authEvent{TenantID: &tenant.ID, PrincipalID: &principal.ID, EventType: "login", Success: false, IPAddress: req.IPAddress, UserAgent: req.UserAgent, Data: map[string]interface{}{"reason": "disabled"}})
		return nil, ErrInvalidCredential
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		_ = recordAuthEvent(ctx, a.db, authEvent{TenantID: &tenant.ID, PrincipalID: &principal.ID, EventType: "login", Success: false, IPAddress: req.IPAddress, UserAgent: req.UserAgent})
		return nil, ErrInvalidCredential
	}

	sessionToken, tokenHash, err := generateSessionToken()
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	_ = sessionToken

	expiresAt := time.Now().UTC().Add(a.sessionTTL)
	var sessionID uuid.UUID
	err = a.db.QueryRowContext(ctx, `
INSERT INTO sessions (principal_id, token_hash, expires_at, ip_address, user_agent)
VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''))
RETURNING id
`, principal.ID, tokenHash, expiresAt, req.IPAddress, req.UserAgent).Scan(&sessionID)
	if err != nil {
		return nil, fmt.Errorf("insert session: %w", err)
	}

	claims := tokenClaims{PrincipalID: principal.ID, TenantID: principal.TenantID, SessionID: sessionID, ExpiresAt: expiresAt.Unix()}
	jwtToken, err := signJWT(a.jwtSecret, claims)
	if err != nil {
		return nil, fmt.Errorf("sign jwt: %w", err)
	}

	roles, err := listPrincipalRoleNames(ctx, a.db, principal.ID)
	if err != nil {
		return nil, err
	}
	principal.Roles = roles

	_ = recordAuthEvent(ctx, a.db, authEvent{TenantID: &tenant.ID, PrincipalID: &principal.ID, EventType: "login", Success: true, IPAddress: req.IPAddress, UserAgent: req.UserAgent})

	return &LoginResponse{Token: jwtToken, Principal: principal, Tenant: tenant, ExpiresAt: expiresAt}, nil
}

func (a *AuthService) AuthenticateBearer(ctx context.Context, bearerToken string) (*AuthPrincipal, error) {
	token := strings.TrimSpace(bearerToken)
	if token == "" {
		return nil, ErrInvalidToken
	}
	if strings.Count(token, ".") == 2 {
		return a.authenticateJWT(ctx, token)
	}
	return a.authenticateAPIKey(ctx, token)
}

func (a *AuthService) authenticateJWT(ctx context.Context, jwtToken string) (*AuthPrincipal, error) {
	claims, err := parseAndVerifyJWT(a.jwtSecret, jwtToken, time.Now().UTC())
	if err != nil {
		return nil, err
	}

	var principal AuthPrincipal
	err = a.db.QueryRowContext(ctx, `
SELECT p.id, p.tenant_id, p.type, p.name, COALESCE(p.email, ''), s.id
FROM sessions s
JOIN principals p ON p.id = s.principal_id
WHERE s.id = $1
  AND p.id = $2
  AND p.tenant_id = $3
  AND p.status = 'active'
  AND s.expires_at > now()
`, claims.SessionID, claims.PrincipalID, claims.TenantID).Scan(&principal.ID, &principal.TenantID, &principal.Type, &principal.Name, &principal.Email, &claims.SessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("validate session token: %w", err)
	}
	principal.SessionID = &claims.SessionID
	roles, err := listPrincipalRoleNames(ctx, a.db, principal.ID)
	if err == nil {
		principal.Roles = roles
	}
	return &principal, nil
}

func (a *AuthService) authenticateAPIKey(ctx context.Context, rawKey string) (*AuthPrincipal, error) {
	hash := hashSecret(rawKey)
	var principal AuthPrincipal
	err := a.db.QueryRowContext(ctx, `
SELECT id, tenant_id, type, name, COALESCE(email, '')
FROM principals
WHERE api_key_hash = $1
  AND status = 'active'
`, hash).Scan(&principal.ID, &principal.TenantID, &principal.Type, &principal.Name, &principal.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("lookup api key principal: %w", err)
	}
	roles, err := listPrincipalRoleNames(ctx, a.db, principal.ID)
	if err == nil {
		principal.Roles = roles
	}
	return &principal, nil
}

func (a *AuthService) Logout(ctx context.Context, tenantID, principalID uuid.UUID, sessionID uuid.UUID) error {
	_, err := a.db.ExecContext(ctx, `
DELETE FROM sessions s
USING principals p
WHERE s.id = $1
  AND s.principal_id = p.id
  AND p.id = $2
  AND p.tenant_id = $3
`, sessionID, principalID, tenantID)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	_ = recordAuthEvent(ctx, a.db, authEvent{TenantID: &tenantID, PrincipalID: &principalID, EventType: "logout", Success: true})
	return nil
}

func (a *AuthService) ChangePassword(ctx context.Context, tenantID, principalID, sessionID uuid.UUID, req ChangePasswordRequest) error {
	if err := ValidatePassword(req.NewPassword); err != nil {
		return err
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin change password tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentHash string
	err = tx.QueryRowContext(ctx, `
SELECT password_hash
FROM principals
WHERE id = $1 AND tenant_id = $2 AND type = 'human' AND status = 'active'
`, principalID, tenantID).Scan(&currentHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidCredential
		}
		return fmt.Errorf("load password hash: %w", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.CurrentPassword)); err != nil {
		return ErrInvalidCredential
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE principals
SET password_hash = $3
WHERE id = $1 AND tenant_id = $2
`, principalID, tenantID, string(hash)); err != nil {
		return fmt.Errorf("update password hash: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
DELETE FROM sessions s
USING principals p
WHERE s.principal_id = p.id
  AND p.id = $1
  AND p.tenant_id = $2
  AND s.id <> $3
`, principalID, tenantID, sessionID); err != nil {
		return fmt.Errorf("invalidate other sessions: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit change password tx: %w", err)
	}

	_ = recordAuthEvent(ctx, a.db, authEvent{TenantID: &tenantID, PrincipalID: &principalID, EventType: "password_changed", Success: true})
	return nil
}

func (a *AuthService) CleanupExpiredSessions(ctx context.Context) error {
	_, err := a.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

func (a *AuthService) StartSessionCleanup(ctx context.Context) {
	ticker := time.NewTicker(a.cleanupEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = a.CleanupExpiredSessions(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (a *AuthService) GetPreferences(ctx context.Context, tenantID, principalID uuid.UUID) (UserPreferences, error) {
	var pref UserPreferences
	err := a.db.QueryRowContext(ctx, `
SELECT up.principal_id, up.theme_id, up.locale, up.notifications, up.preferences, up.updated_at
FROM user_preferences up
JOIN principals p ON p.id = up.principal_id
WHERE up.principal_id = $1
  AND p.tenant_id = $2
`, principalID, tenantID).Scan(&pref.PrincipalID, &pref.ThemeID, &pref.Locale, &pref.Notifications, &pref.Preferences, &pref.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			pref = UserPreferences{PrincipalID: principalID, Locale: "en", Notifications: []byte("{}"), Preferences: []byte("{}")}
			return pref, nil
		}
		return UserPreferences{}, fmt.Errorf("load user preferences: %w", err)
	}
	return pref, nil
}

func (a *AuthService) UpdatePreferences(ctx context.Context, tenantID, principalID uuid.UUID, req UpdatePreferencesRequest) (UserPreferences, error) {
	locale := req.Locale
	if locale == "" {
		locale = "en"
	}
	notifications := req.Notifications
	if len(notifications) == 0 {
		notifications = []byte("{}")
	}
	preferences := req.Preferences
	if len(preferences) == 0 {
		preferences = []byte("{}")
	}

	if _, err := a.db.ExecContext(ctx, `
INSERT INTO user_preferences (principal_id, theme_id, locale, notifications, preferences, updated_at)
SELECT $1, $2, $3, $4::jsonb, $5::jsonb, now()
WHERE EXISTS (SELECT 1 FROM principals p WHERE p.id = $1 AND p.tenant_id = $6)
ON CONFLICT (principal_id) DO UPDATE
SET
    theme_id = EXCLUDED.theme_id,
    locale = EXCLUDED.locale,
    notifications = EXCLUDED.notifications,
    preferences = EXCLUDED.preferences,
    updated_at = now()
`, principalID, req.ThemeID, locale, string(notifications), string(preferences), tenantID); err != nil {
		return UserPreferences{}, fmt.Errorf("upsert user preferences: %w", err)
	}

	return a.GetPreferences(ctx, tenantID, principalID)
}

func (a *AuthService) RecordDenied(ctx context.Context, principal AuthPrincipal, permission, path string) {
	_ = recordAuthEvent(ctx, a.db, authEvent{TenantID: &principal.TenantID, PrincipalID: &principal.ID, EventType: "permission_denied", Success: false, Permission: permission, Path: path})
}

func (a *AuthService) resolveTenant(ctx context.Context, tenantID *uuid.UUID, tenantSlug string) (TenantContext, error) {
	var t TenantContext
	query := `
SELECT id, name, slug, branding, terminology, settings
FROM tenants
WHERE `
	args := []interface{}{}
	if tenantID != nil && *tenantID != uuid.Nil {
		query += `id = $1`
		args = append(args, *tenantID)
	} else if tenantSlug != "" {
		query += `slug = $1`
		args = append(args, tenantSlug)
	} else {
		return TenantContext{}, ErrInvalidCredential
	}

	err := a.db.QueryRowContext(ctx, query, args...).Scan(&t.ID, &t.Name, &t.Slug, &t.Branding, &t.Terminology, &t.Settings)
	if err != nil {
		return TenantContext{}, err
	}
	return t, nil
}

func (a *AuthService) lookupPrincipalForLogin(ctx context.Context, tenantID uuid.UUID, email string) (Principal, string, error) {
	var p Principal
	var passwordHash string
	err := a.db.QueryRowContext(ctx, `
SELECT id, tenant_id, type, name, email, status, COALESCE(metadata, '{}'::jsonb), created_at, COALESCE(password_hash, '')
FROM principals
WHERE tenant_id = $1
  AND email = $2
  AND type = 'human'
`, tenantID, strings.ToLower(strings.TrimSpace(email))).Scan(&p.ID, &p.TenantID, &p.Type, &p.Name, &p.Email, &p.Status, &p.Metadata, &p.CreatedAt, &passwordHash)
	if err != nil {
		return Principal{}, "", err
	}
	return p, passwordHash, nil
}

func generateSessionToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	plaintext := hex.EncodeToString(raw)
	return plaintext, hashSecret(plaintext), nil
}

func GenerateAPIKey() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	plaintext := "acx_key_" + hex.EncodeToString(raw)
	return plaintext, hashSecret(plaintext), nil
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

var (
	passwordLetter = regexp.MustCompile(`[A-Za-z]`)
	passwordDigit  = regexp.MustCompile(`[0-9]`)
)

func ValidatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if !passwordLetter.MatchString(password) || !passwordDigit.MatchString(password) {
		return fmt.Errorf("password must contain at least one letter and one number")
	}
	return nil
}
