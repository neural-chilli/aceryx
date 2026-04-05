package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neural-chilli/aceryx/internal/storage"
)

type Store struct {
	basePath string
}

type sidecarMetadata struct {
	ContentType   string            `json:"content_type,omitempty"`
	ContentLength int64             `json:"content_length"`
	Checksum      string            `json:"checksum,omitempty"`
	Custom        map[string]string `json:"custom,omitempty"`
}

func New(basePath string) (*Store, error) {
	basePath = strings.TrimSpace(basePath)
	if basePath == "" {
		basePath = "./data/vault"
	}
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve base path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create base path: %w", err)
	}
	return &Store{basePath: abs}, nil
}

func (s *Store) Put(_ context.Context, key string, data io.Reader, metadata storage.ObjectMetadata) error {
	resolved, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	buf, err := io.ReadAll(data)
	if err != nil {
		return fmt.Errorf("read object payload: %w", err)
	}
	if metadata.ContentLength <= 0 {
		metadata.ContentLength = int64(len(buf))
	}
	if strings.TrimSpace(metadata.Checksum) == "" {
		metadata.Checksum = storage.SHA256Hex(buf)
	}
	if err := os.WriteFile(resolved, buf, 0o600); err != nil {
		return fmt.Errorf("write object file: %w", err)
	}
	meta := sidecarMetadata{
		ContentType:   metadata.ContentType,
		ContentLength: metadata.ContentLength,
		Checksum:      metadata.Checksum,
		Custom:        metadata.Custom,
	}
	raw, _ := json.Marshal(meta)
	if err := os.WriteFile(resolved+".meta.json", raw, 0o600); err != nil {
		return fmt.Errorf("write object metadata: %w", err)
	}
	return nil
}

func (s *Store) Get(_ context.Context, key string) (io.ReadCloser, storage.ObjectMetadata, error) {
	resolved, err := s.resolvePath(key)
	if err != nil {
		return nil, storage.ObjectMetadata{}, err
	}
	buf, err := os.ReadFile(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ObjectMetadata{}, fs.ErrNotExist
		}
		return nil, storage.ObjectMetadata{}, fmt.Errorf("read object file: %w", err)
	}
	meta, _ := s.readMetadata(resolved)
	if meta.ContentLength <= 0 {
		meta.ContentLength = int64(len(buf))
	}
	if strings.TrimSpace(meta.Checksum) == "" {
		meta.Checksum = storage.SHA256Hex(buf)
	}
	if got := storage.SHA256Hex(buf); got != meta.Checksum {
		return nil, storage.ObjectMetadata{}, fmt.Errorf("checksum mismatch for key %q", key)
	}
	return io.NopCloser(bytes.NewReader(buf)), storage.ObjectMetadata{
		ContentType:   meta.ContentType,
		ContentLength: meta.ContentLength,
		Checksum:      meta.Checksum,
		Custom:        meta.Custom,
	}, nil
}

func (s *Store) Delete(_ context.Context, key string) error {
	resolved, err := s.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(resolved); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete object file: %w", err)
	}
	_ = os.Remove(resolved + ".meta.json")
	return nil
}

func (s *Store) List(_ context.Context, prefix string, opts storage.ListOpts) ([]storage.ObjectInfo, error) {
	max := opts.MaxResults
	if max <= 0 {
		max = 1000
	}
	prefix = storage.NormalizeKey(prefix)
	root := s.basePath
	if prefix != "" {
		candidate := filepath.Join(s.basePath, filepath.FromSlash(prefix))
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			root = candidate
		}
	}
	out := make([]storage.ObjectInfo, 0, max)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".meta.json") {
			return nil
		}
		rel, err := filepath.Rel(s.basePath, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
		if prefix != "" && !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		meta, _ := s.readMetadata(path)
		out = append(out, storage.ObjectInfo{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime().UTC(),
			ContentType:  meta.ContentType,
			Checksum:     meta.Checksum,
		})
		if len(out) >= max {
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	return out, nil
}

func (s *Store) Exists(_ context.Context, key string) (bool, error) {
	resolved, err := s.resolvePath(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(resolved)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat object file: %w", err)
}

func (s *Store) PresignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", storage.ErrNotSupported
}

func (s *Store) resolvePath(key string) (string, error) {
	clean := filepath.Clean(storage.NormalizeKey(key))
	if clean == "." || clean == "" || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid object key")
	}
	candidate := filepath.Join(s.basePath, filepath.FromSlash(clean))
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve object path: %w", err)
	}
	rel, err := filepath.Rel(s.basePath, abs)
	if err != nil {
		return "", fmt.Errorf("validate object path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("object key escapes base path")
	}
	return abs, nil
}

func (s *Store) readMetadata(objectPath string) (sidecarMetadata, error) {
	raw, err := os.ReadFile(objectPath + ".meta.json")
	if err != nil {
		if os.IsNotExist(err) {
			return sidecarMetadata{}, nil
		}
		return sidecarMetadata{}, err
	}
	var meta sidecarMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return sidecarMetadata{}, err
	}
	return meta, nil
}
