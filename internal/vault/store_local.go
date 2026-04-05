package vault

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultVaultRoot       = "./data/vault"
	defaultSignedURLExpiry = 300
)

var (
	safePathSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	sha256HexPattern       = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

type LocalVaultStore struct {
	root   string
	secret []byte
	now    func() time.Time
}

func NewLocalVaultStore(root string, signingSecret string) *LocalVaultStore {
	if strings.TrimSpace(root) == "" {
		root = defaultVaultRoot
	}
	if signingSecret == "" {
		signingSecret = "aceryx-vault-dev-secret"
	}
	return &LocalVaultStore{root: root, secret: []byte(signingSecret), now: func() time.Time { return time.Now().UTC() }}
}

func (s *LocalVaultStore) Put(tenantID, hash, ext string, data []byte) (string, error) {
	tenantID = strings.TrimSpace(tenantID)
	if !safePathSegmentPattern.MatchString(tenantID) {
		return "", fmt.Errorf("invalid tenant id")
	}
	hash = strings.ToLower(strings.TrimSpace(hash))
	if !sha256HexPattern.MatchString(hash) {
		return "", fmt.Errorf("invalid content hash")
	}
	ext = normalizeExt(ext)
	now := s.now()
	uri := fmt.Sprintf("%s/%04d/%02d/%s/%s/%s.%s", tenantID, now.Year(), int(now.Month()), hashPrefix(hash, 0), hashPrefix(hash, 2), hash, ext)
	fullPath, err := s.resolvePath(uri)
	if err != nil {
		return "", fmt.Errorf("resolve vault path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", fmt.Errorf("create vault directory: %w", err)
	}
	if _, err := os.Stat(fullPath); err == nil {
		return uri, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat vault file: %w", err)
	}
	if err := os.WriteFile(fullPath, data, 0o600); err != nil {
		return "", fmt.Errorf("write vault file: %w", err)
	}
	return uri, nil
}

func (s *LocalVaultStore) Get(uri string) ([]byte, error) {
	fullPath, err := s.resolvePath(uri)
	if err != nil {
		return nil, err
	}
	buf, err := os.ReadFile(fullPath)
	if err != nil {
		if errorsIsNotExist(err) {
			return nil, fs.ErrNotExist
		}
		return nil, fmt.Errorf("read vault file: %w", err)
	}
	return buf, nil
}

func (s *LocalVaultStore) Delete(uri string) error {
	fullPath, err := s.resolvePath(uri)
	if err != nil {
		return err
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete vault file: %w", err)
	}
	return nil
}

func (s *LocalVaultStore) SignedURL(uri string, expirySeconds int) (string, error) {
	if strings.TrimSpace(uri) == "" {
		return "", fmt.Errorf("uri is required")
	}
	if expirySeconds <= 0 {
		expirySeconds = defaultSignedURLExpiry
	}
	expiresAt := s.now().Add(time.Duration(expirySeconds) * time.Second).Unix()
	sig := signURI(s.secret, uri, expiresAt)
	return fmt.Sprintf("%s?exp=%d&sig=%s", uri, expiresAt, sig), nil
}

func (s *LocalVaultStore) VerifySignedURL(uri, expiryRaw, signature string) error {
	expiresAt, err := strconv.ParseInt(strings.TrimSpace(expiryRaw), 10, 64)
	if err != nil || expiresAt <= 0 {
		return fmt.Errorf("invalid expiry")
	}
	if s.now().Unix() > expiresAt {
		return fmt.Errorf("signed url expired")
	}
	expected := signURI(s.secret, uri, expiresAt)
	if !hmac.Equal([]byte(expected), []byte(signature)) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func (s *LocalVaultStore) resolvePath(uri string) (string, error) {
	clean := filepath.Clean(strings.TrimPrefix(strings.TrimSpace(uri), "/"))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid vault uri")
	}
	root := filepath.Clean(s.root)
	fullPath := filepath.Join(root, clean)
	rel, err := filepath.Rel(root, fullPath)
	if err != nil {
		return "", fmt.Errorf("compute relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid vault uri")
	}
	return fullPath, nil
}

func signURI(secret []byte, uri string, expiresAt int64) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(uri))
	_, _ = mac.Write([]byte("|"))
	_, _ = mac.Write([]byte(strconv.FormatInt(expiresAt, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

func normalizeExt(ext string) string {
	ext = strings.TrimSpace(strings.TrimPrefix(ext, "."))
	if ext == "" || !safePathSegmentPattern.MatchString(ext) {
		return "bin"
	}
	return ext
}

func hashPrefix(hash string, start int) string {
	if len(hash) < start+2 {
		return "00"
	}
	return hash[start : start+2]
}

func errorsIsNotExist(err error) bool {
	return os.IsNotExist(err) || strings.Contains(strings.ToLower(err.Error()), "no such file")
}
