package vault

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/neural-chilli/aceryx/internal/storage"
	storageazure "github.com/neural-chilli/aceryx/internal/storage/azure"
	storagegcs "github.com/neural-chilli/aceryx/internal/storage/gcs"
	storagelocal "github.com/neural-chilli/aceryx/internal/storage/local"
	storages3 "github.com/neural-chilli/aceryx/internal/storage/s3"
)

type BackendConfig struct {
	Backend string
}

type BackendStatus struct {
	BackendType string `json:"backend_type"`
	Healthy     bool   `json:"healthy"`
	Error       string `json:"error,omitempty"`
}

func BuildVaultStoreFromEnv(ctx context.Context, signingSecret string) (VaultStore, BackendStatus, error) {
	backend := strings.ToLower(strings.TrimSpace(os.Getenv("ACERYX_VAULT_BACKEND")))
	if backend == "" {
		backend = "local"
	}
	switch backend {
	case "local":
		root := firstNonEmpty(os.Getenv("ACERYX_VAULT_ROOT"), os.Getenv("ACERYX_VAULT_PATH"))
		store := NewLocalVaultStore(root, signingSecret)
		return store, BackendStatus{BackendType: backend, Healthy: true}, nil
	case "s3":
		obj, err := storages3.New(ctx, storages3.Config{
			Bucket:          os.Getenv("ACERYX_VAULT_S3_BUCKET"),
			Region:          os.Getenv("ACERYX_VAULT_S3_REGION"),
			Prefix:          os.Getenv("ACERYX_VAULT_S3_PREFIX"),
			Endpoint:        os.Getenv("ACERYX_VAULT_S3_ENDPOINT"),
			AccessKeyID:     os.Getenv("ACERYX_VAULT_S3_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("ACERYX_VAULT_S3_SECRET_ACCESS_KEY"),
			UseIAMRole:      parseBool(os.Getenv("ACERYX_VAULT_S3_USE_IAM_ROLE")),
			SSE:             firstNonEmpty(os.Getenv("ACERYX_VAULT_S3_SSE"), "sse-s3"),
			SSEKMSKeyID:     os.Getenv("ACERYX_VAULT_S3_SSE_KMS_KEY_ID"),
		})
		if err != nil {
			return nil, BackendStatus{BackendType: backend, Healthy: false, Error: err.Error()}, err
		}
		return NewObjectBackedVaultStore(obj), BackendStatus{BackendType: backend, Healthy: true}, nil
	case "minio":
		obj, err := storages3.New(ctx, storages3.Config{
			Bucket:          os.Getenv("ACERYX_VAULT_MINIO_BUCKET"),
			Region:          firstNonEmpty(os.Getenv("ACERYX_VAULT_MINIO_REGION"), "us-east-1"),
			Prefix:          os.Getenv("ACERYX_VAULT_MINIO_PREFIX"),
			Endpoint:        os.Getenv("ACERYX_VAULT_MINIO_ENDPOINT"),
			AccessKeyID:     os.Getenv("ACERYX_VAULT_MINIO_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("ACERYX_VAULT_MINIO_SECRET_ACCESS_KEY"),
			UseSSL:          parseBool(os.Getenv("ACERYX_VAULT_MINIO_USE_SSL")),
		})
		if err != nil {
			return nil, BackendStatus{BackendType: backend, Healthy: false, Error: err.Error()}, err
		}
		return NewObjectBackedVaultStore(obj), BackendStatus{BackendType: backend, Healthy: true}, nil
	case "gcs":
		obj, err := storagegcs.New(ctx, storagegcs.Config{
			Bucket:              os.Getenv("ACERYX_VAULT_GCS_BUCKET"),
			Prefix:              os.Getenv("ACERYX_VAULT_GCS_PREFIX"),
			CredentialsJSON:     os.Getenv("ACERYX_VAULT_GCS_CREDENTIALS_JSON"),
			UseWorkloadIdentity: parseBool(os.Getenv("ACERYX_VAULT_GCS_USE_WORKLOAD_IDENTITY")),
		})
		if err != nil {
			return nil, BackendStatus{BackendType: backend, Healthy: false, Error: err.Error()}, err
		}
		return NewObjectBackedVaultStore(obj), BackendStatus{BackendType: backend, Healthy: true}, nil
	case "azure_blob":
		obj, err := storageazure.New(ctx, storageazure.Config{
			Container:          os.Getenv("ACERYX_VAULT_AZURE_CONTAINER"),
			Prefix:             os.Getenv("ACERYX_VAULT_AZURE_PREFIX"),
			AccountName:        os.Getenv("ACERYX_VAULT_AZURE_ACCOUNT_NAME"),
			AccountKey:         os.Getenv("ACERYX_VAULT_AZURE_ACCOUNT_KEY"),
			UseManagedIdentity: parseBool(os.Getenv("ACERYX_VAULT_AZURE_USE_MANAGED_IDENTITY")),
		})
		if err != nil {
			return nil, BackendStatus{BackendType: backend, Healthy: false, Error: err.Error()}, err
		}
		return NewObjectBackedVaultStore(obj), BackendStatus{BackendType: backend, Healthy: true}, nil
	default:
		return nil, BackendStatus{BackendType: backend, Healthy: false, Error: "unknown backend"}, fmt.Errorf("unsupported vault backend %q", backend)
	}
}

func parseBool(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "1" || v == "true" || v == "yes"
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func NewLocalObjectStore(basePath string) (storage.ObjectStore, error) {
	return storagelocal.New(basePath)
}
