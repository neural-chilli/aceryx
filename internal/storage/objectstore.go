package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var ErrNotSupported = errors.New("operation not supported")

type ObjectStore interface {
	Put(ctx context.Context, key string, data io.Reader, metadata ObjectMetadata) error
	Get(ctx context.Context, key string) (io.ReadCloser, ObjectMetadata, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string, opts ListOpts) ([]ObjectInfo, error)
	Exists(ctx context.Context, key string) (bool, error)
	PresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}

type ObjectMetadata struct {
	ContentType   string
	ContentLength int64
	Checksum      string
	Custom        map[string]string
}

type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
	ContentType  string
	Checksum     string
}

type ListOpts struct {
	MaxResults int
	Cursor     string
	Delimiter  string
}
