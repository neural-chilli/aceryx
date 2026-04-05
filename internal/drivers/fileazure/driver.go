package fileazure

import (
	"context"

	"github.com/neural-chilli/aceryx/internal/drivers"
	"github.com/neural-chilli/aceryx/internal/drivers/objectfile"
	"github.com/neural-chilli/aceryx/internal/storage"
	storageazure "github.com/neural-chilli/aceryx/internal/storage/azure"
)

func New() drivers.FileDriver {
	return objectfile.New("azure_blob", "Azure Blob Storage", func(ctx context.Context, cfg drivers.FileConfig) (storage.ObjectStore, error) {
		drivers.NormalizeCloudFileConfig(&cfg)
		return storageazure.New(ctx, storageazure.Config{
			Container:          cfg.Container,
			Prefix:             cfg.Prefix,
			AccountName:        cfg.AccountName,
			AccountKey:         cfg.AccountKey,
			UseManagedIdentity: cfg.UseManagedIdentity,
		})
	})
}
