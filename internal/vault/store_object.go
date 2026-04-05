package vault

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/storage"
)

type ObjectBackedVaultStore struct {
	store storage.ObjectStore
	now   func() time.Time
}

func NewObjectBackedVaultStore(store storage.ObjectStore) *ObjectBackedVaultStore {
	return &ObjectBackedVaultStore{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (s *ObjectBackedVaultStore) Put(tenantID, hash, ext string, data []byte) (string, error) {
	if s.store == nil {
		return "", fmt.Errorf("object store not configured")
	}
	ext = normalizeExt(ext)
	now := s.now()
	key := fmt.Sprintf("%s/%04d/%02d/%s/%s/%s.%s", tenantID, now.Year(), int(now.Month()), hashPrefix(hash, 0), hashPrefix(hash, 2), hash, ext)
	if err := s.store.Put(
		contextBackground(),
		key,
		bytes.NewReader(data),
		storage.ObjectMetadata{ContentType: detectMime("file." + ext), ContentLength: int64(len(data)), Checksum: hash},
	); err != nil {
		return "", err
	}
	return filepath.ToSlash(strings.TrimPrefix(key, "/")), nil
}

func (s *ObjectBackedVaultStore) Get(uri string) ([]byte, error) {
	if s.store == nil {
		return nil, fmt.Errorf("object store not configured")
	}
	rc, meta, err := s.store.Get(contextBackground(), uri)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	buf, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read object body: %w", err)
	}
	if strings.TrimSpace(meta.Checksum) != "" {
		if got := storage.SHA256Hex(buf); got != meta.Checksum {
			return nil, fmt.Errorf("checksum mismatch for uri %q", uri)
		}
	}
	return buf, nil
}

func (s *ObjectBackedVaultStore) Delete(uri string) error {
	if s.store == nil {
		return fmt.Errorf("object store not configured")
	}
	return s.store.Delete(contextBackground(), uri)
}

func (s *ObjectBackedVaultStore) SignedURL(uri string, expirySeconds int) (string, error) {
	if s.store == nil {
		return "", fmt.Errorf("object store not configured")
	}
	d := time.Duration(expirySeconds) * time.Second
	if d <= 0 {
		d = 15 * time.Minute
	}
	return s.store.PresignedURL(contextBackground(), uri, d)
}

func contextBackground() context.Context {
	return context.Background()
}
