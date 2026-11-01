package launcher

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/MedCy1/WaterStick/pkg/waterstick/adb"
	"github.com/MedCy1/WaterStick/pkg/waterstick/packages"
)

// CleanOptions configures the Stock Clean WaterOS transformation.
type CleanOptions struct {
	DisableAutoplay  bool    `json:"disable_autoplay"`
	MuteBanner       bool    `json:"mute_banner"`
	FreezeAdRows     bool    `json:"freeze_ad_rows"`
	SpeedupUI        bool    `json:"speedup_ui"`
	AnimationScale   float64 `json:"animation_scale"` // 0.0 for instant, 0.5 for accelerated
	ClearCache       bool    `json:"clear_cache"`
	SilenceNavSounds bool    `json:"silence_nav_sounds"`
}

// DefaultCleanOptions returns the standard Stock Clean WaterOS settings.
func DefaultCleanOptions() CleanOptions {
	return CleanOptions{
		DisableAutoplay:  true,
		MuteBanner:       true,
		FreezeAdRows:     true,
		SpeedupUI:        true,
		AnimationScale:   0.5,
		ClearCache:       true,
		SilenceNavSounds: false,
	}
}

// CleanResult details the modifications applied during the launcher cleaning pass.
type CleanResult struct {
	AutoplayDisabled  bool     `json:"autoplay_disabled"`
	BannerMuted       bool     `json:"banner_muted"`
	FrozenAdPackages  []string `json:"frozen_ad_packages"`
	AnimationSpeedup  bool     `json:"animation_speedup"`
	CacheCleared      bool     `json:"cache_cleared"`
	LauncherRestarted bool     `json:"launcher_restarted"`
}

// RestoreResult details the state after reverting to stock Amazon launcher defaults.
type RestoreResult struct {
	AutoplayRestored   bool     `json:"autoplay_restored"`
	BannerUnmuted      bool     `json:"banner_unmuted"`
	UnfrozenAdPackages []string `json:"unfrozen_ad_packages"`
	AnimationRestored  bool     `json:"animation_restored"`
	LauncherRestarted  bool     `json:"launcher_restarted"`
}

// Status represents the current state of the stock launcher optimizations.
type Status struct {
	AutoplayDisabled    bool     `json:"autoplay_disabled"`
	BannerMuted         bool     `json:"banner_muted"`
	PvdeDisabled        bool     `json:"pvde_disabled"`
	WeblabGated         bool     `json:"weblab_gated"`
	WindowAnimScale     string   `json:"window_anim_scale"`
	TransitionAnimScale string   `json:"transition_anim_scale"`
	AnimatorScale       string   `json:"animator_scale"`
	FrozenAdPackages    []string `json:"frozen_ad_packages"`
	ActiveAdPackages    []string `json:"active_ad_packages"`
	IsClean             bool     `json:"is_clean"`
}

// CommercialAdPackages are the packages responsible for ads and recommendation row injections on Home.
var CommercialAdPackages = []string{
	"com.amazon.kso.blackbird",
	"com.amazon.media.recommendations",
	"com.amazon.shoptv.client",
	"com.amazon.shoptv.firetv.client",
	"com.amazon.gamehub",
	"com.amazon.ftvads.deeplinking",
}

// Cleaner orchestrates stock launcher optimization and sterilization.
type Cleaner struct {
	client  *adb.Client
	pkgMgr  *packages.Manager
}

// NewCleaner creates a new launcher cleaner instance.
func NewCleaner(client *adb.Client, pkgMgr *packages.Manager) *Cleaner {
	return &Cleaner{
		client: client,
		pkgMgr: pkgMgr,
	}
}

// GetStatus inspects the current state of launcher settings and commercial packages.
func (c *Cleaner) GetStatus(ctx context.Context, serial string) (*Status, error) {
	st := &Status{}

	// 1. Check video autoplay
	val, err := c.client.Shell(ctx, serial, "settings get global com.amazon.tv.settings.featured_item_video_enabled")
	if err == nil && strings.TrimSpace(val) == "0" {
		st.AutoplayDisabled = true
	}

	// 2. Check banner volume
	val, err = c.client.Shell(ctx, serial, "settings get global com.amazon.tv.settings.featured_item_video_volume")
	if err == nil && strings.TrimSpace(val) == "0.0" {
		st.BannerMuted = true
	}

	// 3. Check PVDE banner player
	val, err = c.client.Shell(ctx, serial, "settings get secure pvde_playback_api_enabled_for_launcher")
	if err == nil && strings.TrimSpace(val) == "0" {
		st.PvdeDisabled = true
	}

	// 4. Check Weblab flags
	val, err = c.client.Shell(ctx, serial, "settings get secure FTV_LAUNCHER_FR_AUTOPLAY_717667")
	if err == nil && strings.TrimSpace(val) == "C" {
		st.WeblabGated = true
	}

	// 5. Check animation scales
	val, _ = c.client.Shell(ctx, serial, "settings get global window_animation_scale")
	st.WindowAnimScale = strings.TrimSpace(val)
	val, _ = c.client.Shell(ctx, serial, "settings get global transition_animation_scale")
	st.TransitionAnimScale = strings.TrimSpace(val)
	val, _ = c.client.Shell(ctx, serial, "settings get global animator_duration_scale")
	st.AnimatorScale = strings.TrimSpace(val)

	// 6. Check commercial ad packages
	disabledMap, _ := c.pkgMgr.ListDisabled(ctx, serial)
	for _, pkg := range CommercialAdPackages {
		if c.isPackageFrozen(ctx, serial, pkg, disabledMap) {
			st.FrozenAdPackages = append(st.FrozenAdPackages, pkg)
		} else {
			st.ActiveAdPackages = append(st.ActiveAdPackages, pkg)
		}
	}

	// IsClean is true if autoplay is disabled, pvde is disabled, and at least some ad packages are frozen
	if st.AutoplayDisabled && st.BannerMuted && st.PvdeDisabled && len(st.FrozenAdPackages) > 0 {
		st.IsClean = true
	}

	return st, nil
}

// Clean applies the complete Stock Clean WaterOS optimizations to the launcher.
func (c *Cleaner) Clean(ctx context.Context, serial string, opts CleanOptions) (*CleanResult, error) {
	res := &CleanResult{}

	// 1. Disable autoplay video & muted banner audio
	if opts.DisableAutoplay {
		_, _ = c.client.Shell(ctx, serial, "settings put global com.amazon.tv.settings.featured_item_video_enabled 0")
		_, _ = c.client.Shell(ctx, serial, "settings put secure pvde_playback_api_enabled_for_launcher 0")
		_, _ = c.client.Shell(ctx, serial, "settings put secure FTV_LAUNCHER_FR_AUTOPLAY_717667 C")
		_, _ = c.client.Shell(ctx, serial, "settings put secure FTV_LAUNCHER_FR_AUTOPLAY_EXPERIMENT_723780 C")
		res.AutoplayDisabled = true
	}

	if opts.MuteBanner {
		_, _ = c.client.Shell(ctx, serial, "settings put global com.amazon.tv.settings.featured_item_video_volume 0.0")
		res.BannerMuted = true
	}

	// 2. Accelerate UI animations
	if opts.SpeedupUI {
		scaleStr := strconv.FormatFloat(opts.AnimationScale, 'f', 1, 64)
		_, _ = c.client.Shell(ctx, serial, fmt.Sprintf("settings put global window_animation_scale %s", scaleStr))
		_, _ = c.client.Shell(ctx, serial, fmt.Sprintf("settings put global transition_animation_scale %s", scaleStr))
		_, _ = c.client.Shell(ctx, serial, fmt.Sprintf("settings put global animator_duration_scale %s", scaleStr))
		res.AnimationSpeedup = true
	}

	// 3. Silence UI click sound effects if requested
	if opts.SilenceNavSounds {
		_, _ = c.client.Shell(ctx, serial, "settings put system sound_effects_enabled 0")
	}

	// 4. Freeze commercial ad and recommendation row packages
	if opts.FreezeAdRows {
		for _, pkg := range CommercialAdPackages {
			if err := c.freezePackage(ctx, serial, pkg); err == nil {
				res.FrozenAdPackages = append(res.FrozenAdPackages, pkg)
			}
		}
	}

	// 5. Trim disk cache
	if opts.ClearCache {
		_, _ = c.client.Shell(ctx, serial, "pm trim-caches 2G")
		res.CacheCleared = true
	}

	// 6. Force-stop launcher so VideoPreviewEnabledObserver flushes cache and picks up clean state
	_ = c.Restart(ctx, serial)
	res.LauncherRestarted = true

	return res, nil
}

// Restore reverts all launcher settings to Amazon factory defaults.
func (c *Cleaner) Restore(ctx context.Context, serial string) (*RestoreResult, error) {
	res := &RestoreResult{}

	// 1. Restore autoplay and audio volume
	_, _ = c.client.Shell(ctx, serial, "settings put global com.amazon.tv.settings.featured_item_video_enabled 1")
	_, _ = c.client.Shell(ctx, serial, "settings put global com.amazon.tv.settings.featured_item_video_volume 1.0")
	_, _ = c.client.Shell(ctx, serial, "settings put secure pvde_playback_api_enabled_for_launcher 1")
	_, _ = c.client.Shell(ctx, serial, "settings put secure FTV_LAUNCHER_FR_AUTOPLAY_717667 T")
	_, _ = c.client.Shell(ctx, serial, "settings put secure FTV_LAUNCHER_FR_AUTOPLAY_EXPERIMENT_723780 T")
	res.AutoplayRestored = true
	res.BannerUnmuted = true

	// 2. Restore standard animation scales
	_, _ = c.client.Shell(ctx, serial, "settings put global window_animation_scale 1.0")
	_, _ = c.client.Shell(ctx, serial, "settings put global transition_animation_scale 1.0")
	_, _ = c.client.Shell(ctx, serial, "settings put global animator_duration_scale 1.0")
	res.AnimationRestored = true

	// 3. Unfreeze commercial ad and recommendation row packages
	for _, pkg := range CommercialAdPackages {
		if err := c.unfreezePackage(ctx, serial, pkg); err == nil {
			res.UnfrozenAdPackages = append(res.UnfrozenAdPackages, pkg)
		}
	}

	// 4. Restart launcher to apply
	_ = c.Restart(ctx, serial)
	res.LauncherRestarted = true

	return res, nil
}

func (c *Cleaner) freezePackage(ctx context.Context, serial string, pkg string) error {
	err := c.pkgMgr.DisablePackage(ctx, serial, pkg, false)
	if err == nil {
		return nil
	}
	// Fallback to suspend if package is protected against disable-user on Fire OS 8
	out, sErr := c.client.Shell(ctx, serial, fmt.Sprintf("pm suspend %s", pkg))
	if sErr == nil && strings.Contains(out, "new suspended state: true") {
		return nil
	}
	return err
}

func (c *Cleaner) unfreezePackage(ctx context.Context, serial string, pkg string) error {
	_ = c.pkgMgr.EnablePackage(ctx, serial, pkg)
	_, _ = c.client.Shell(ctx, serial, fmt.Sprintf("pm unsuspend %s", pkg))
	return nil
}

func (c *Cleaner) isPackageFrozen(ctx context.Context, serial string, pkg string, disabledMap map[string]bool) bool {
	if disabledMap != nil && disabledMap[pkg] {
		return true
	}
	out, err := c.client.Shell(ctx, serial, fmt.Sprintf("dumpsys package %s", pkg))
	if err == nil && strings.Contains(out, "suspended=true") {
		return true
	}
	return false
}

// Restart restarts com.amazon.tv.launcher to flush memory and reset UI state.
func (c *Cleaner) Restart(ctx context.Context, serial string) error {
	_, err := c.client.Shell(ctx, serial, "am force-stop com.amazon.tv.launcher")
	return err
}
