package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maintenanceMarkerFile = ".aceryx-maintenance"

func MaintenanceMarkerPath(vaultPath string) string {
	if strings.TrimSpace(vaultPath) == "" {
		return maintenanceMarkerFile
	}
	return filepath.Join(vaultPath, maintenanceMarkerFile)
}

func EnableMaintenanceMode(vaultPath string) error {
	marker := MaintenanceMarkerPath(vaultPath)
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return fmt.Errorf("create maintenance marker directory: %w", err)
	}
	if err := os.WriteFile(marker, []byte("on\n"), 0o644); err != nil {
		return fmt.Errorf("write maintenance marker: %w", err)
	}
	return nil
}

func DisableMaintenanceMode(vaultPath string) error {
	marker := MaintenanceMarkerPath(vaultPath)
	if err := os.Remove(marker); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove maintenance marker: %w", err)
	}
	return nil
}

func IsMaintenanceMode(vaultPath string) bool {
	marker := MaintenanceMarkerPath(vaultPath)
	_, err := os.Stat(marker)
	return err == nil
}
