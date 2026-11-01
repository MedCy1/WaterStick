package packages

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/MedCy1/WaterStick/pkg/waterstick/adb"
	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
)

//go:embed packages.json
var embeddedPackagesJSON []byte

// Manager oversees package safety, discovery, and lifecycle operations.
type Manager struct {
	mu     sync.RWMutex
	client *adb.Client
	db     models.PackageDatabase
}

// NewManager creates a package manager initialized with embedded definitions.
func NewManager(client *adb.Client) (*Manager, error) {
	var db models.PackageDatabase
	if err := json.Unmarshal(embeddedPackagesJSON, &db); err != nil {
		return nil, fmt.Errorf("failed to parse embedded packages.json: %w", err)
	}

	return &Manager{
		client: client,
		db:     db,
	}, nil
}

// GetDatabase returns the full loaded package database.
func (m *Manager) GetDatabase() models.PackageDatabase {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.db
}

// GetPackage searches for a package by ID.
func (m *Manager) GetPackage(id string) (*models.Package, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.db.Packages {
		if p.ID == id {
			return &p, true
		}
	}
	return nil, false
}

// ListInstalled returns a set of all package names installed on user 0.
func (m *Manager) ListInstalled(ctx context.Context, deviceSerial string) (map[string]bool, error) {
	out, err := m.client.Shell(ctx, deviceSerial, "pm list packages --user 0")
	if err != nil {
		return nil, err
	}

	installed := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package:") {
			pkg := strings.TrimPrefix(line, "package:")
			installed[pkg] = true
		}
	}
	return installed, nil
}

// ListDisabled returns a set of all package names currently disabled on the device.
func (m *Manager) ListDisabled(ctx context.Context, deviceSerial string) (map[string]bool, error) {
	out, err := m.client.Shell(ctx, deviceSerial, "pm list packages -d")
	if err != nil {
		return nil, err
	}

	disabled := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package:") {
			pkg := strings.TrimPrefix(line, "package:")
			disabled[pkg] = true
		}
	}
	return disabled, nil
}

// DisablePackage safely disables a package using 'pm disable-user --user 0'.
// Enforces strict safety rules against bricking and voice corruption.
func (m *Manager) DisablePackage(ctx context.Context, deviceSerial string, pkgID string, allowVoiceDisable bool) error {
	pkg, known := m.GetPackage(pkgID)

	// 1. Critical Hard-Lock Protection
	if known && pkg.Tier == models.TierHardLocked {
		return fmt.Errorf("safety violation: package %s is Tier 0 (Hard-Locked) and cannot be disabled to prevent device damage", pkgID)
	}

	// 2. Voice Protection
	if known && pkg.Tier == models.TierVoice && !allowVoiceDisable {
		return fmt.Errorf("voice protection: package %s belongs to Alexa/Voice layer. To disable, explicit bypass must be granted", pkgID)
	}

	cmd := fmt.Sprintf("pm disable-user --user 0 %s", pkgID)
	out, err := m.client.Shell(ctx, deviceSerial, cmd)
	if err != nil {
		return fmt.Errorf("failed to disable package %s: %s (%w)", pkgID, strings.TrimSpace(out), err)
	}

	if !strings.Contains(out, "new state: disabled-user") && !strings.Contains(out, "new state: disabled") {
		return fmt.Errorf("unexpected output when disabling %s: %s", pkgID, strings.TrimSpace(out))
	}

	return nil
}

// EnablePackage re-enables a package using 'pm enable'.
func (m *Manager) EnablePackage(ctx context.Context, deviceSerial string, pkgID string) error {
	cmd := fmt.Sprintf("pm enable %s", pkgID)
	out, err := m.client.Shell(ctx, deviceSerial, cmd)
	if err != nil {
		return fmt.Errorf("failed to enable package %s: %s (%w)", pkgID, strings.TrimSpace(out), err)
	}

	if !strings.Contains(out, "new state: enabled") {
		return fmt.Errorf("unexpected output when enabling %s: %s", pkgID, strings.TrimSpace(out))
	}

	return nil
}

// GetPackagesWithStatus returns all catalog packages annotated with live installation and disabled flags.
func (m *Manager) GetPackagesWithStatus(ctx context.Context, deviceSerial string) ([]models.Package, error) {
	installedMap, err := m.ListInstalled(ctx, deviceSerial)
	if err != nil {
		return nil, err
	}

	disabledMap, err := m.ListDisabled(ctx, deviceSerial)
	if err != nil {
		return nil, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]models.Package, len(m.db.Packages))
	for i, p := range m.db.Packages {
		p.Installed = installedMap[p.ID]
		p.Disabled = disabledMap[p.ID]
		result[i] = p
	}

	return result, nil
}
