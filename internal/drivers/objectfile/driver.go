package objectfile

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/neural-chilli/aceryx/internal/drivers"
	"github.com/neural-chilli/aceryx/internal/storage"
)

type Factory func(ctx context.Context, cfg drivers.FileConfig) (storage.ObjectStore, error)

type Driver struct {
	id          string
	displayName string
	factory     Factory
	store       storage.ObjectStore
	prefix      string
}

func New(id, displayName string, factory Factory) *Driver {
	return &Driver{id: id, displayName: displayName, factory: factory}
}

func (d *Driver) ID() string          { return d.id }
func (d *Driver) DisplayName() string { return d.displayName }

func (d *Driver) Connect(ctx context.Context, config drivers.FileConfig) error {
	if d.factory == nil {
		return fmt.Errorf("object file driver factory is required")
	}
	store, err := d.factory(ctx, config)
	if err != nil {
		return err
	}
	d.store = store
	d.prefix = strings.Trim(strings.TrimSpace(config.Prefix), "/")
	return nil
}

func (d *Driver) List(ctx context.Context, p string) ([]drivers.FileEntry, error) {
	if d.store == nil {
		return nil, fmt.Errorf("file driver not connected")
	}
	infos, err := d.store.List(ctx, d.fullPath(p), storage.ListOpts{MaxResults: 1000, Delimiter: "/"})
	if err != nil {
		return nil, err
	}
	out := make([]drivers.FileEntry, 0, len(infos))
	for _, info := range infos {
		out = append(out, drivers.FileEntry{Path: info.Key, Name: path.Base(info.Key), Size: info.Size, ModTime: info.LastModified})
	}
	return out, nil
}

func (d *Driver) Read(ctx context.Context, p string) (io.ReadCloser, error) {
	if d.store == nil {
		return nil, fmt.Errorf("file driver not connected")
	}
	rc, _, err := d.store.Get(ctx, d.fullPath(p))
	return rc, err
}

func (d *Driver) Write(ctx context.Context, p string, data io.Reader) error {
	if d.store == nil {
		return fmt.Errorf("file driver not connected")
	}
	return d.store.Put(ctx, d.fullPath(p), data, storage.ObjectMetadata{ContentType: "application/octet-stream"})
}

func (d *Driver) Delete(ctx context.Context, p string) error {
	if d.store == nil {
		return fmt.Errorf("file driver not connected")
	}
	return d.store.Delete(ctx, d.fullPath(p))
}

func (d *Driver) Close() error {
	d.store = nil
	return nil
}

func (d *Driver) fullPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return d.prefix
	}
	if d.prefix == "" {
		return p
	}
	return path.Join(d.prefix, p)
}
