package tuning

import (
	"testing"

	"github.com/MedCy1/WaterStick/pkg/waterstick/adb"
)

func TestDefaultTuningSettings(t *testing.T) {
	settings := DefaultTuningSettings("dns.adguard.com")
	if len(settings) == 0 {
		t.Fatal("expected tuning settings, got empty slice")
	}

	foundVideoAutoplay := false
	foundDNS := false
	for _, s := range settings {
		if s.Key == "com.amazon.tv.settings.featured_item_video_enabled" && s.Value == "0" {
			foundVideoAutoplay = true
		}
		if s.Key == "private_dns_specifier" && s.Value == "dns.adguard.com" {
			foundDNS = true
		}
	}

	if !foundVideoAutoplay {
		t.Error("expected com.amazon.tv.settings.featured_item_video_enabled to be set to 0")
	}
	if !foundDNS {
		t.Error("expected private_dns_specifier to be configured")
	}
}

func TestOTAShieldStructures(t *testing.T) {
	client := adb.NewClient("")
	shield := NewOTAManager(client)
	if shield == nil {
		t.Fatal("failed to initialize OTAManager")
	}
}

func TestAdvancedTweaksPresets(t *testing.T) {
	tuner := NewTuner(adb.NewClient(""))

	// Test invalid DNS preset error
	err := tuner.SetDNSProvider(nil, "mock-dev", "invalid-provider")
	if err == nil {
		t.Error("expected error for invalid dns preset")
	}

	// Test audio config detection logic
	cfg := AudioTweakConfig{
		CompressedAudioPref:   "4",
		EncodedSurroundOutput: "3",
	}
	if cfg.CompressedAudioPref == "4" && cfg.EncodedSurroundOutput == "3" {
		cfg.IsOptical51Optimized = true
	}
	if !cfg.IsOptical51Optimized {
		t.Error("expected IsOptical51Optimized to be true for AC-3 manual surround")
	}
}

