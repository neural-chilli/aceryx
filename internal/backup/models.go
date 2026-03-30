package backup

import (
	"time"

	"github.com/google/uuid"
)

const (
	BackupFormatVersion = "1.0"
	AceryxVersion       = "1.0.0"
)

type Metadata struct {
	Version         string     `json:"version"`
	AceryxVersion   string     `json:"aceryx_version"`
	CreatedAt       time.Time  `json:"created_at"`
	PostgresVersion string     `json:"postgres_version"`
	TenantFilter    *uuid.UUID `json:"tenant_filter"`
	SchemaVersion   int        `json:"schema_version"`
	CaseCount       int64      `json:"case_count"`
	DocumentCount   int64      `json:"document_count"`
	SizeBytes       int64      `json:"size_bytes"`
}

type BackupOptions struct {
	OutputPath string
	TenantID   *uuid.UUID
	Pause      bool
}

type RestoreOptions struct {
	InputPath   string
	TargetDBURL string
	Confirm     bool
}

type RestoreResult struct {
	CasesCount         int64
	DocumentsCount     int64
	SchemaVersion      int
	MigratedFrom       int
	MigrationsApplied  bool
	VaultFilesVerified int
	VaultFilesSampled  int
}

type VerifyResult struct {
	Metadata       Metadata
	DumpValid      bool
	VaultFileCount int
	VaultSizeBytes int64
	Status         string
}
