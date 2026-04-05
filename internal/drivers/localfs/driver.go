package localfs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/neural-chilli/aceryx/internal/drivers"
)

type Driver struct {
	basePath string
	baseReal string
}

func New() *Driver { return &Driver{} }

func (d *Driver) ID() string          { return "local" }
func (d *Driver) DisplayName() string { return "Local Filesystem" }

func (d *Driver) Connect(ctx context.Context, config drivers.FileConfig) error {
	_ = ctx
	base := strings.TrimSpace(config.BasePath)
	if base == "" {
		base = "."
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return fmt.Errorf("resolve base path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return fmt.Errorf("create base path: %w", err)
	}
	d.basePath = abs
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		d.baseReal = real
	} else {
		d.baseReal = abs
	}
	return nil
}

func (d *Driver) List(ctx context.Context, path string) ([]drivers.FileEntry, error) {
	_ = ctx
	resolved, err := d.resolve(path)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	out := make([]drivers.FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("read file info: %w", err)
		}
		full := filepath.Join(resolved, e.Name())
		rel, _ := filepath.Rel(d.basePath, full)
		out = append(out, drivers.FileEntry{
			Path:    filepath.ToSlash(rel),
			Name:    e.Name(),
			Size:    info.Size(),
			IsDir:   e.IsDir(),
			ModTime: info.ModTime(),
		})
	}
	return out, nil
}

func (d *Driver) Read(ctx context.Context, path string) (io.ReadCloser, error) {
	_ = ctx
	resolved, err := d.resolve(path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return file, nil
}

func (d *Driver) Write(ctx context.Context, path string, data io.Reader) error {
	_ = ctx
	resolved, err := d.resolve(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return fmt.Errorf("create parent dirs: %w", err)
	}
	file, err := os.OpenFile(resolved, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open file for write: %w", err)
	}
	defer func() { _ = file.Close() }()
	if _, err := io.Copy(file, data); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (d *Driver) Delete(ctx context.Context, path string) error {
	_ = ctx
	resolved, err := d.resolve(path)
	if err != nil {
		return err
	}
	if err := os.Remove(resolved); err != nil {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}

func (d *Driver) Close() error { return nil }

func (d *Driver) resolve(path string) (string, error) {
	if d.basePath == "" {
		return "", fmt.Errorf("localfs driver not connected")
	}
	clean := filepath.Clean(path)
	if clean == "." {
		clean = ""
	}
	candidate := filepath.Join(d.basePath, clean)
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}
	if !isUnderBase(d.basePath, abs) {
		return "", fmt.Errorf("path escapes base path")
	}
	if evaluated, err := filepath.EvalSymlinks(abs); err == nil {
		if !isUnderBase(d.basePath, evaluated) && !isUnderBase(d.baseReal, evaluated) {
			return "", fmt.Errorf("path escapes base path")
		}
	}
	return abs, nil
}

func isUnderBase(base, target string) bool {
	base = filepath.Clean(base)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
