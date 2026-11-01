package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/MedCy1/WaterStick/pkg/waterstick/adb"
	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
)

// Manager manages state snapshots and rollback execution.
type Manager struct {
	client *adb.Client
}

// NewManager creates a new backup & rollback manager.
func NewManager(client *adb.Client) *Manager {
	return &Manager{client: client}
}

// DefaultBackupDir returns the directory where snapshots are stored.
func DefaultBackupDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "waterstick", "backups"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "waterstick", "backups"), nil
	default:
		return filepath.Join(home, ".config", "waterstick", "backups"), nil
	}
}

// CreateBackup captures currently disabled packages and critical settings.
func (m *Manager) CreateBackup(ctx context.Context, device *models.Device) (*models.Backup, error) {
	out, err := m.client.Shell(ctx, device.Serial, "pm list packages -d")
	if err != nil {
		return nil, fmt.Errorf("failed to query disabled packages: %w", err)
	}

	var disabled []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "package:") {
			disabled = append(disabled, strings.TrimPrefix(l, "package:"))
		}
	}

	backupID := fmt.Sprintf("backup_%s_%s", sanitize(device.Serial), time.Now().Format("20060102_150405"))
	snap := &models.Backup{
		ID:               backupID,
		CreatedAt:        time.Now(),
		DeviceSerial:     device.Serial,
		DeviceModel:      device.ModelName,
		FireOSVersion:    device.FireOSVersion,
		DisabledPackages: disabled,
	}

	dir, err := DefaultBackupDir()
	if err != nil {
		return snap, nil // Continue in-memory even if disk save fails
	}

	_ = os.MkdirAll(dir, 0755)
	file := filepath.Join(dir, backupID+".json")

	data, err := json.MarshalIndent(snap, "", "  ")
	if err == nil {
		_ = os.WriteFile(file, data, 0644)
	}

	return snap, nil
}

// RollbackAll enables all disabled packages on the device and restores default settings.
func (m *Manager) RollbackAll(ctx context.Context, deviceSerial string, progressFn func(pkg string, err error)) error {
	out, err := m.client.Shell(ctx, deviceSerial, "pm list packages -d")
	if err != nil {
		return fmt.Errorf("failed to query disabled packages: %w", err)
	}

	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "package:") {
			pkg := strings.TrimPrefix(l, "package:")
			enableCmd := fmt.Sprintf("pm enable %s", pkg)
			_, cmdErr := m.client.Shell(ctx, deviceSerial, enableCmd)
			if progressFn != nil {
				progressFn(pkg, cmdErr)
			}
		}
	}

	// Restore settings to safe defaults
	_, _ = m.client.Shell(ctx, deviceSerial, "settings put global auto_play_video 1")
	_, _ = m.client.Shell(ctx, deviceSerial, "settings put global auto_play_audio 1")
	_, _ = m.client.Shell(ctx, deviceSerial, "settings put global private_dns_mode off")
	_, _ = m.client.Shell(ctx, deviceSerial, "settings delete global private_dns_specifier")
	_, _ = m.client.Shell(ctx, deviceSerial, "settings put secure limit_ad_tracking 0")
	_, _ = m.client.Shell(ctx, deviceSerial, "settings put secure interest_based_ads 1")
	_, _ = m.client.Shell(ctx, deviceSerial, "settings put global match_content_frame_rate 0")
	_, _ = m.client.Shell(ctx, deviceSerial, "settings put global window_animation_scale 1.0")
	_, _ = m.client.Shell(ctx, deviceSerial, "settings put global transition_animation_scale 1.0")
	_, _ = m.client.Shell(ctx, deviceSerial, "settings put global animator_duration_scale 1.0")

	// Reset AppOps background constraints
	appOpsReset := []string{"com.netflix.ninja", "in.startv.hotstar", "com.amazon.device.crashmanager", "com.crunchyroll.crunchyroid"}
	for _, app := range appOpsReset {
		_, _ = m.client.Shell(ctx, deviceSerial, fmt.Sprintf("cmd appops set %s RUN_IN_BACKGROUND default", app))
	}

	return nil
}

func sanitize(s string) string {
	r := strings.NewReplacer(":", "_", ".", "_", "/", "_", "\\", "_")
	return r.Replace(s)
}
