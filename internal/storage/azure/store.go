package azure

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/neural-chilli/aceryx/internal/storage"
)

type Config struct {
	Container          string
	Prefix             string
	AccountName        string
	AccountKey         string
	UseManagedIdentity bool
}

type Store struct {
	container string
	prefix    string
	client    *azblob.Client
}

func New(ctx context.Context, cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Container) == "" {
		return nil, fmt.Errorf("azure blob container is required")
	}
	if strings.TrimSpace(cfg.AccountName) == "" {
		return nil, fmt.Errorf("azure account name is required")
	}
	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net/", cfg.AccountName)
	var (
		client *azblob.Client
		err    error
	)
	if strings.TrimSpace(cfg.AccountKey) != "" {
		cred, credErr := azblob.NewSharedKeyCredential(cfg.AccountName, cfg.AccountKey)
		if credErr != nil {
			return nil, fmt.Errorf("create azure shared key credential: %w", credErr)
		}
		client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, cred, nil)
	} else {
		cred, credErr := azidentity.NewDefaultAzureCredential(nil)
		if credErr != nil {
			return nil, fmt.Errorf("create azure default credential: %w", credErr)
		}
		client, err = azblob.NewClient(serviceURL, cred, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("create azure blob client: %w", err)
	}
	return &Store{container: cfg.Container, prefix: strings.Trim(strings.TrimSpace(cfg.Prefix), "/"), client: client}, nil
}

func (s *Store) Put(ctx context.Context, key string, data io.Reader, metadata storage.ObjectMetadata) error {
	blobName := s.objectKey(key)
	meta := map[string]*string{}
	for k, v := range metadata.Custom {
		value := v
		meta[k] = &value
	}
	if metadata.Checksum != "" {
		value := metadata.Checksum
		meta["checksum"] = &value
	}
	_, err := s.client.UploadStream(ctx, s.container, blobName, data, &azblob.UploadStreamOptions{
		HTTPHeaders: &blob.HTTPHeaders{BlobContentType: to.Ptr(metadata.ContentType)},
		Metadata:    meta,
	})
	if err != nil {
		return fmt.Errorf("upload azure blob: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, key string) (io.ReadCloser, storage.ObjectMetadata, error) {
	blobName := s.objectKey(key)
	resp, err := s.client.DownloadStream(ctx, s.container, blobName, nil)
	if err != nil {
		return nil, storage.ObjectMetadata{}, fmt.Errorf("download azure blob: %w", err)
	}
	meta := storage.ObjectMetadata{ContentType: value(resp.ContentType), ContentLength: valueInt64(resp.ContentLength), Custom: map[string]string{}}
	for k, v := range resp.Metadata {
		if v == nil {
			continue
		}
		meta.Custom[k] = *v
	}
	meta.Checksum = meta.Custom["checksum"]
	return resp.Body, meta, nil
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteBlob(ctx, s.container, s.objectKey(key), nil)
	if err != nil {
		return fmt.Errorf("delete azure blob: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context, prefix string, opts storage.ListOpts) ([]storage.ObjectInfo, error) {
	max := opts.MaxResults
	if max <= 0 {
		max = 1000
	}
	pager := s.client.NewListBlobsFlatPager(s.container, &azblob.ListBlobsFlatOptions{Prefix: to.Ptr(s.objectKey(prefix))})
	out := make([]storage.ObjectInfo, 0, max)
	for pager.More() && len(out) < max {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list azure blobs: %w", err)
		}
		for _, item := range page.Segment.BlobItems {
			if len(out) >= max {
				break
			}
			out = append(out, storage.ObjectInfo{
				Key:          trimPrefix(value(item.Name), s.prefix),
				Size:         valueInt64(item.Properties.ContentLength),
				LastModified: valueTime(item.Properties.LastModified),
				ContentType:  value(item.Properties.ContentType),
			})
		}
	}
	return out, nil
}

func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	blobClient := s.client.ServiceClient().NewContainerClient(s.container).NewBlobClient(s.objectKey(key))
	_, err := blobClient.GetProperties(ctx, nil)
	if err == nil {
		return true, nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "blobnotfound") || strings.Contains(msg, "status code 404") {
		return false, nil
	}
	return false, fmt.Errorf("azure exists check: %w", err)
}

func (s *Store) PresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	_ = ctx
	_ = key
	_ = expiry
	return "", storage.ErrNotSupported
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

func value(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func valueInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func valueTime(v *time.Time) time.Time {
	if v == nil {
		return time.Time{}
	}
	return v.UTC()
}
