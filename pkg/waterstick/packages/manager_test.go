package packages

import (
	"context"
	"testing"

	"github.com/MedCy1/WaterStick/pkg/waterstick/adb"
	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
)

func TestSafetyChecks(t *testing.T) {
	client := adb.NewClient("")
	mgr, err := NewManager(client)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()

	// 1. Hard-locked Tier 0 packages MUST be refused
	hardLockedPkgs := []string{
		"com.amazon.tv.ime",
		"com.amazon.tv.launcher",
		"com.amazon.tv.settings.v2",
		"com.amazon.whisperplay.contracts",
	}

	for _, p := range hardLockedPkgs {
		err := mgr.DisablePackage(ctx, "mock-device", p, true)
		if err == nil {
			t.Errorf("expected safety violation when attempting to disable hard-locked package %s, but got nil", p)
		}
	}

	// 2. Tier Voice packages MUST be refused unless explicit bypass is given
	voicePkgs := []string{
		"com.amazon.avl.ftv",
		"com.amazon.aria",
		"com.amazon.vizzini",
		"com.amazon.ale",
	}

	for _, p := range voicePkgs {
		// Attempt without bypass -> should fail
		err := mgr.DisablePackage(ctx, "mock-device", p, false)
		if err == nil {
			t.Errorf("expected voice protection error for %s without bypass, but got nil", p)
		}
	}
}

func TestDatabaseIntegrity(t *testing.T) {
	client := adb.NewClient("")
	mgr, err := NewManager(client)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	db := mgr.GetDatabase()
	if len(db.Packages) == 0 {
		t.Fatal("packages database is empty")
	}

	seen := make(map[string]bool)
	for _, p := range db.Packages {
		if p.ID == "" {
			t.Errorf("found package with empty ID")
		}
		if p.Name == "" {
			t.Errorf("package %s has empty Name", p.ID)
		}
		if seen[p.ID] {
			t.Errorf("duplicate package ID found: %s", p.ID)
		}
		seen[p.ID] = true

		switch p.Tier {
		case models.TierHardLocked, models.TierVoice, models.TierSafe, models.TierConsumer:
			// OK
		default:
			t.Errorf("package %s has invalid tier: %s", p.ID, p.Tier)
		}
	}
}
