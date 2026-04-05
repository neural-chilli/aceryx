package files3

import (
	"context"

	"github.com/neural-chilli/aceryx/internal/drivers"
	"github.com/neural-chilli/aceryx/internal/drivers/objectfile"
	"github.com/neural-chilli/aceryx/internal/storage"
	storages3 "github.com/neural-chilli/aceryx/internal/storage/s3"
)

func New() drivers.FileDriver {
	return objectfile.New("s3", "Amazon S3", func(ctx context.Context, cfg drivers.FileConfig) (storage.ObjectStore, error) {
		drivers.NormalizeCloudFileConfig(&cfg)
		return storages3.New(ctx, storages3.Config{
			Bucket:          cfg.Bucket,
			Region:          cfg.Region,
			Prefix:          cfg.Prefix,
			Endpoint:        cfg.Endpoint,
			AccessKeyID:     cfg.AccessKeyID,
			SecretAccessKey: cfg.SecretAccessKey,
			UseIAMRole:      cfg.UseIAMRole,
			UseSSL:          cfg.UseSSL,
			SSE:             cfg.SSE,
			SSEKMSKeyID:     cfg.SSEKMSKeyID,
		})
	})
}
