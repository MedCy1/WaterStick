package launcher

import (
	"testing"
)

func TestDefaultCleanOptions(t *testing.T) {
	opts := DefaultCleanOptions()
	if !opts.DisableAutoplay {
		t.Errorf("expected DisableAutoplay to be true")
	}
	if !opts.MuteBanner {
		t.Errorf("expected MuteBanner to be true")
	}
	if !opts.FreezeAdRows {
		t.Errorf("expected FreezeAdRows to be true")
	}
	if !opts.SpeedupUI {
		t.Errorf("expected SpeedupUI to be true")
	}
	if opts.AnimationScale != 0.5 {
		t.Errorf("expected AnimationScale to be 0.5, got %f", opts.AnimationScale)
	}
	if !opts.ClearCache {
		t.Errorf("expected ClearCache to be true")
	}
}

func TestCommercialAdPackagesList(t *testing.T) {
	if len(CommercialAdPackages) == 0 {
		t.Fatalf("expected CommercialAdPackages to contain packages")
	}

	expected := map[string]bool{
		"com.amazon.kso.blackbird":         true,
		"com.amazon.media.recommendations": true,
		"com.amazon.shoptv.client":         true,
		"com.amazon.shoptv.firetv.client":  true,
		"com.amazon.gamehub":               true,
		"com.amazon.ftvads.deeplinking":     true,
	}

	for _, pkg := range CommercialAdPackages {
		if !expected[pkg] {
			t.Errorf("unexpected package in CommercialAdPackages: %s", pkg)
		}
	}
}
