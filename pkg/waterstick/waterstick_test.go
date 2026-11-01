package waterstick

import (
	"testing"

	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
)

func TestProfileSafetyGuarantees(t *testing.T) {
	engine, err := NewEngine("")
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	if len(engine.Profiles) == 0 {
		t.Fatal("no profiles configured in engine")
	}

	for id, profile := range engine.Profiles {
		// 1. None of the profiles must EVER contain a Tier 0 hard-locked package
		for _, pkgID := range profile.DisabledPackages {
			pkg, found := engine.Packages.GetPackage(pkgID)
			if found && pkg.Tier == models.TierHardLocked {
				t.Fatalf("CRITICAL SECURITY DEFECT: profile %q contains hard-locked package %s", id, pkgID)
			}
		}

		// 2. Default profiles (balanced, streamer) must preserve voice
		if id == "balanced" || id == "streamer" {
			if !profile.PreservesVoice {
				t.Errorf("profile %q must preserve voice", id)
			}
			for _, pkgID := range profile.DisabledPackages {
				pkg, found := engine.Packages.GetPackage(pkgID)
				if found && pkg.Tier == models.TierVoice {
					t.Fatalf("VIOLATION: profile %q disables voice package %s", id, pkgID)
				}
			}
		}
	}
}
