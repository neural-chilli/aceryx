package gcs

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	gcstorage "cloud.google.com/go/storage"
	"github.com/neural-chilli/aceryx/internal/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

type Config struct {
	Bucket              string
	Prefix              string
	CredentialsJSON     string
	UseWorkloadIdentity bool
}

type Store struct {
	bucket string
	prefix string
	client *gcstorage.Client
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("gcs bucket is required")
	}
	opts := make([]option.ClientOption, 0, 1)
	if strings.TrimSpace(cfg.CredentialsJSON) != "" {
		credentialsRef := strings.TrimSpace(cfg.CredentialsJSON)
		if strings.HasPrefix(credentialsRef, "{") {
			opts = append(opts, option.WithAuthCredentialsJSON(option.ServiceAccount, []byte(credentialsRef)))
		} else {
			opts = append(opts, option.WithAuthCredentialsFile(option.ServiceAccount, credentialsRef))
		}
	}
	client, err := gcstorage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create gcs client: %w", err)
	}
	return &Store{bucket: cfg.Bucket, prefix: strings.Trim(strings.TrimSpace(cfg.Prefix), "/"), client: client}, nil
}

func (s *Store) Put(ctx context.Context, key string, data io.Reader, metadata storage.ObjectMetadata) error {
	obj := s.client.Bucket(s.bucket).Object(s.objectKey(key))
	w := obj.NewWriter(ctx)
	w.ContentType = metadata.ContentType
	w.Metadata = map[string]string{}
	for k, v := range metadata.Custom {
		w.Metadata[k] = v
	}
	if metadata.Checksum != "" {
		w.Metadata["checksum"] = metadata.Checksum
	}
	if metadata.ContentLength > 0 {
		w.ChunkSize = 256 * 1024
	}
	if _, err := io.Copy(w, data); err != nil {
		_ = w.Close()
		return fmt.Errorf("write gcs object: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close gcs writer: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, storage.ObjectMetadata, error) {
	obj := s.client.Bucket(s.bucket).Object(s.objectKey(key))
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return nil, storage.ObjectMetadata{}, fmt.Errorf("read gcs attrs: %w", err)
	}
	rc, err := obj.NewReader(ctx)
	if err != nil {
		return nil, storage.ObjectMetadata{}, fmt.Errorf("open gcs object: %w", err)
	}
	meta := storage.ObjectMetadata{ContentType: attrs.ContentType, ContentLength: attrs.Size, Custom: attrs.Metadata}
	if attrs.Metadata != nil {
		meta.Checksum = attrs.Metadata["checksum"]
	}
	return rc, meta, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	if err := s.client.Bucket(s.bucket).Object(s.objectKey(key)).Delete(ctx); err != nil {
		return fmt.Errorf("delete gcs object: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context, prefix string, opts storage.ListOpts) ([]storage.ObjectInfo, error) {
	max := opts.MaxResults
	if max <= 0 {
		max = 1000
	}
	query := &gcstorage.Query{Prefix: s.objectKey(prefix), Delimiter: strings.TrimSpace(opts.Delimiter)}
	it := s.client.Bucket(s.bucket).Objects(ctx, query)
	out := make([]storage.ObjectInfo, 0, max)
	for len(out) < max {
		obj, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list gcs objects: %w", err)
		}
		out = append(out, storage.ObjectInfo{
			Key:          trimPrefix(obj.Name, s.prefix),
			Size:         obj.Size,
			LastModified: obj.Updated.UTC(),
			ContentType:  obj.ContentType,
			Checksum:     obj.Metadata["checksum"],
		})
	}
	return out, nil
}

func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.Bucket(s.bucket).Object(s.objectKey(key)).Attrs(ctx)
	if err == nil {
		return true, nil
	}
	if err == gcstorage.ErrObjectNotExist {
		return false, nil
	}
	return false, fmt.Errorf("gcs exists check: %w", err)
}

func (s *Store) PresignedURL(_ context.Context, key string, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		expiry = 15 * time.Minute
	}
	url, err := gcstorage.SignedURL(s.bucket, s.objectKey(key), &gcstorage.SignedURLOptions{Method: "GET", Expires: time.Now().Add(expiry)})
	if err != nil {
		return "", fmt.Errorf("generate gcs signed url: %w", err)
	}
	return url, nil
}

func (s *Store) objectKey(key string) string {
	return storage.JoinKey(s.prefix, key)
}

func trimPrefix(key, prefix string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return key
	}
	key = strings.TrimPrefix(key, prefix)
	return strings.TrimPrefix(key, "/")
}
