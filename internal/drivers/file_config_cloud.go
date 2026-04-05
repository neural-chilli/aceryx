package drivers

import "strings"

func NormalizeCloudFileConfig(cfg *FileConfig) {
	if cfg == nil {
		return
	}
	if strings.TrimSpace(cfg.AccessKeyID) == "" {
		cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyIDSecret)
	}
	if strings.TrimSpace(cfg.SecretAccessKey) == "" {
		cfg.SecretAccessKey = strings.TrimSpace(cfg.SecretAccessKeySecret)
	}
	if strings.TrimSpace(cfg.CredentialsJSON) == "" {
		cfg.CredentialsJSON = strings.TrimSpace(cfg.CredentialsSecret)
	}
	if strings.TrimSpace(cfg.AccountKey) == "" {
		cfg.AccountKey = strings.TrimSpace(cfg.AccountKeySecret)
	}
}
