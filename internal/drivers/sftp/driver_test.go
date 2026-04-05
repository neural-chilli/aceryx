package sftp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neural-chilli/aceryx/internal/drivers"
)

func TestAuthMethodsPassword(t *testing.T) {
	methods, err := authMethods(drivers.FileConfig{Password: "secret"})
	if err != nil {
		t.Fatalf("password auth: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("expected 1 auth method, got %d", len(methods))
	}
}

func TestAuthMethodsKey(t *testing.T) {
	tmp := t.TempDir()
	keyPath := filepath.Join(tmp, "id_rsa")
	key := `-----BEGIN OPENSSH PRIVATE KEY-----
-----END OPENSSH PRIVATE KEY-----`
	if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	_, err := authMethods(drivers.FileConfig{KeyPath: keyPath})
	if err == nil {
		t.Fatal("expected invalid key parse error")
	}
}
