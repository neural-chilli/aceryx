package filegcs

import (
	"context"

	"github.com/neural-chilli/aceryx/internal/drivers"
	"github.com/neural-chilli/aceryx/internal/drivers/objectfile"
	"github.com/neural-chilli/aceryx/internal/storage"
	storagegcs "github.com/neural-chilli/aceryx/internal/storage/gcs"
)

func New() drivers.FileDriver {
	return objectfile.New("gcs", "Google Cloud Storage", func(ctx context.Context, cfg drivers.FileConfig) (storage.ObjectStore, error) {
		drivers.NormalizeCloudFileConfig(&cfg)
		return storagegcs.New(ctx, storagegcs.Config{
			Bucket:              cfg.Bucket,
			Prefix:              cfg.Prefix,
			CredentialsJSON:     cfg.CredentialsJSON,
			UseWorkloadIdentity: cfg.UseWorkloadIdentity,
		})
	})
}
