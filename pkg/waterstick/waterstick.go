package waterstick

import (
	"context"
	"fmt"

	"github.com/MedCy1/WaterStick/pkg/waterstick/adb"
	"github.com/MedCy1/WaterStick/pkg/waterstick/backup"
	"github.com/MedCy1/WaterStick/pkg/waterstick/diagnostics"
	"github.com/MedCy1/WaterStick/pkg/waterstick/launcher"
	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
	"github.com/MedCy1/WaterStick/pkg/waterstick/packages"
	"github.com/MedCy1/WaterStick/pkg/waterstick/sideload"
	"github.com/MedCy1/WaterStick/pkg/waterstick/tuning"
)

// Engine is the central orchestrator coordinating ADB, diagnostics, packages, tuning, and backups.
type Engine struct {
	ADB       *adb.Client
	Inspector *diagnostics.Inspector
	Packages  *packages.Manager
	Tuner     *tuning.Tuner
	OTA       *tuning.OTAManager
	Backup    *backup.Manager
	Sideload  *sideload.Catalog
	Launcher  *launcher.Cleaner
	Profiles  map[string]models.Profile
}

// NewEngine initializes the core WaterStick engine.
func NewEngine(customAdbPath string) (*Engine, error) {
	adbClient := adb.NewClient(customAdbPath)

	pkgMgr, err := packages.NewManager(adbClient)
	if err != nil {
		return nil, fmt.Errorf("failed to init package manager: %w", err)
	}

	sideMgr, err := sideload.NewCatalog(adbClient)
	if err != nil {
		return nil, fmt.Errorf("failed to init sideload catalog: %w", err)
	}

	engine := &Engine{
		ADB:       adbClient,
		Inspector: diagnostics.NewInspector(adbClient),
		Packages:  pkgMgr,
		Tuner:     tuning.NewTuner(adbClient),
		OTA:       tuning.NewOTAManager(adbClient),
		Backup:    backup.NewManager(adbClient),
		Sideload:  sideMgr,
		Launcher:  launcher.NewCleaner(adbClient, pkgMgr),
		Profiles:  make(map[string]models.Profile),
	}

	engine.initProfiles()
	return engine, nil
}

func (e *Engine) initProfiles() {
	// 1. Profile: Balanced / Daily Driver (Preserves Voice & Remote 100%)
	balancedPackages := []string{
		"com.amazon.tv.acr",
		"com.amazon.hybridadidservice",
		"com.amazon.dp.logger",
		"com.amazon.device.metrics",
		"com.amazon.device.telemetry.emitter",
		"com.amazon.wirelessmetrics.service",
		"com.amazon.client.metrics.api",
		"com.amazon.adep",
		"com.amazon.logan",
		"com.amazon.neo.minerva.logcat",
		"com.amazon.ceviche",
		"com.amazon.ftv.screensaver",
		"com.amazon.ods.kindleconnect",
		"com.amazon.bueller.music",
		"com.amazon.shoptv.client",
		"com.amazon.shoptv.firetv.client",
		"com.amazon.gamehub",
		"com.amazon.ftvads.deeplinking",
		"com.amazon.kso.blackbird",
		"com.amazon.minitv.android.app",
	}

	e.Profiles["balanced"] = models.Profile{
		ID:                 "balanced",
		Name:               "Recommandé / Équilibré",
		Description:        "Garde 100% de la voix Alexa et des streaming apps. Coupe trackers, pubs et autoplay.",
		PreservesVoice:     true,
		PreservesStreaming: true,
		DisabledPackages:   balancedPackages,
		Settings:           tuning.DefaultTuningSettings("dns.adguard.com"),
		AppOps:             tuning.DefaultAppOps(),
	}

	// 2. Profile: Streamer Pur (No Amazon bloat, keeps Alexa voice)
	streamerPackages := append([]string{}, balancedPackages...)
	streamerPackages = append(streamerPackages,
		"com.amazon.avod",
		"com.amazon.bueller.photos",
		"com.amazon.kindle",
		"com.amazon.cloud9",
	)

	e.Profiles["streamer"] = models.Profile{
		ID:                 "streamer",
		Name:               "Streamer Pur (Ultra-Light)",
		Description:        "Pour utilisateurs de Stremio, SmartTube, Kodi, Plex. Coupe tout Amazon mais conserve la télécommande et la voix.",
		PreservesVoice:     true,
		PreservesStreaming: false,
		DisabledPackages:   streamerPackages,
		Settings:           tuning.DefaultTuningSettings("dns.adguard.com"),
		AppOps:             tuning.DefaultAppOps(),
	}
}

// ExecutionReport captures the outcome of applying a profile.
type ExecutionReport struct {
	ProfileID        string   `json:"profile_id"`
	BackupID         string   `json:"backup_id"`
	DisabledCount    int      `json:"disabled_count"`
	SkippedCount     int      `json:"skipped_count"`
	FailedPackages   []string `json:"failed_packages"`
	SettingsApplied  int      `json:"settings_applied"`
	SettingsFailures []string `json:"settings_failures"`
	RAMBeforeMB      int      `json:"ram_before_mb"`
	RAMAfterMB       int      `json:"ram_after_mb"`
	RAMFreedMB       int      `json:"ram_freed_mb"`
}

// ApplyProfile executes safety checks, creates a snapshot, and applies all debloat & tuning rules.
func (e *Engine) ApplyProfile(ctx context.Context, deviceSerial string, profileID string, progressFn func(step int, total int, msg string)) (*ExecutionReport, error) {
	profile, exists := e.Profiles[profileID]
	if !exists {
		return nil, fmt.Errorf("unknown profile: %s", profileID)
	}

	report := &ExecutionReport{
		ProfileID: profileID,
	}

	// Step 1: Diagnostics & Validation
	if progressFn != nil {
		progressFn(1, 6, "Inspect device hardware & OS...")
	}
	device, err := e.Inspector.Inspect(ctx, deviceSerial)
	if err != nil {
		return nil, fmt.Errorf("device inspection failed: %w", err)
	}
	report.RAMBeforeMB = device.FreeRAMMB

	// Step 2: Automatic Safety Snapshot
	if progressFn != nil {
		progressFn(2, 6, "Creating pre-flight backup snapshot...")
	}
	snap, err := e.Backup.CreateBackup(ctx, device)
	if err == nil && snap != nil {
		report.BackupID = snap.ID
	}

	// Step 3: Package Debloating (filter by currently installed)
	installedMap, _ := e.Packages.ListInstalled(ctx, deviceSerial)

	totalPkgs := len(profile.DisabledPackages)
	if progressFn != nil {
		progressFn(3, 6, fmt.Sprintf("Debloating packages (0/%d)...", totalPkgs))
	}

	for idx, pkgID := range profile.DisabledPackages {
		if installedMap != nil && !installedMap[pkgID] {
			report.SkippedCount++
			continue
		}

		err := e.Packages.DisablePackage(ctx, deviceSerial, pkgID, false)
		if err != nil {
			report.FailedPackages = append(report.FailedPackages, fmt.Sprintf("%s: %v", pkgID, err))
		} else {
			report.DisabledCount++
		}
		if progressFn != nil && idx%2 == 0 {
			progressFn(3, 6, fmt.Sprintf("Debloating packages (%d/%d)...", idx+1, totalPkgs))
		}
	}

	// Step 4: System Settings Tuning
	if progressFn != nil {
		progressFn(4, 6, "Applying system settings (silent autoplay, DoT DNS, 24fps)...")
	}
	for _, s := range profile.Settings {
		if err := e.Tuner.ApplySetting(ctx, deviceSerial, s); err != nil {
			report.SettingsFailures = append(report.SettingsFailures, fmt.Sprintf("%s: %v", s.Key, err))
		} else {
			report.SettingsApplied++
		}
	}

	// Step 5: AppOps Background RAM Lockdown
	if progressFn != nil {
		progressFn(5, 6, "Enforcing background RAM execution restrictions...")
	}
	_ = e.Tuner.ApplyAppOps(ctx, deviceSerial, profile.AppOps)

	// Step 6: Trim Caches & Measure Outcome
	if progressFn != nil {
		progressFn(6, 6, "Trimming accumulated stale cache junk...")
	}
	_ = e.Tuner.TrimCaches(ctx, deviceSerial)

	afterDev, err := e.Inspector.Inspect(ctx, deviceSerial)
	if err == nil && afterDev != nil {
		report.RAMAfterMB = afterDev.FreeRAMMB
		report.RAMFreedMB = report.RAMAfterMB - report.RAMBeforeMB
	}

	return report, nil
}

// BlockOTA enforces the OTA Shield to permanently stop background firmware updates.
func (e *Engine) BlockOTA(ctx context.Context, deviceSerial string) error {
	return e.OTA.BlockOTA(ctx, deviceSerial)
}

// UnblockOTA removes the OTA Shield and restores factory Amazon OTA behavior.
func (e *Engine) UnblockOTA(ctx context.Context, deviceSerial string) error {
	return e.OTA.UnblockOTA(ctx, deviceSerial)
}

// IsOTABlocked reports whether OTA updates are currently frozen.
func (e *Engine) IsOTABlocked(ctx context.Context, deviceSerial string) (bool, error) {
	return e.OTA.IsOTABlocked(ctx, deviceSerial)
}

// PurgeMemory executes a deep RAM and video cache reclamation and returns freed MB.
func (e *Engine) PurgeMemory(ctx context.Context, deviceSerial string) (beforeMB int, afterMB int, freedMB int, err error) {
	return e.Tuner.PurgeMemory(ctx, deviceSerial)
}

// GetAudioTweakConfig returns the sound and surround output configuration.
func (e *Engine) GetAudioTweakConfig(ctx context.Context, deviceSerial string) (tuning.AudioTweakConfig, error) {
	return e.Tuner.GetAudioTweakConfig(ctx, deviceSerial)
}

// SetOptical51Mode enables or disables bitstream Dolby Digital 5.1 AC-3 optimized for optical audio setups.
func (e *Engine) SetOptical51Mode(ctx context.Context, deviceSerial string, enable bool) error {
	return e.Tuner.SetOptical51Mode(ctx, deviceSerial, enable)
}

// SetNavigationSounds toggles UI remote click sounds.
func (e *Engine) SetNavigationSounds(ctx context.Context, deviceSerial string, enable bool) error {
	return e.Tuner.SetNavigationSounds(ctx, deviceSerial, enable)
}

// GetDisplayTweakConfig retrieves refresh rate, resolution and HDR information.
func (e *Engine) GetDisplayTweakConfig(ctx context.Context, deviceSerial string) (tuning.DisplayTweakConfig, error) {
	return e.Tuner.GetDisplayTweakConfig(ctx, deviceSerial)
}

// SetMatchFrameRate toggles native 24p cinema refresh rate switching.
func (e *Engine) SetMatchFrameRate(ctx context.Context, deviceSerial string, enable bool) error {
	return e.Tuner.SetMatchFrameRate(ctx, deviceSerial, enable)
}

// GetPowerTweakConfig reads ambient experience and sleep configurations.
func (e *Engine) GetPowerTweakConfig(ctx context.Context, deviceSerial string) (tuning.PowerTweakConfig, error) {
	return e.Tuner.GetPowerTweakConfig(ctx, deviceSerial)
}

// SetAmbientExperience toggles the background Ambient Experience widgets and ads.
func (e *Engine) SetAmbientExperience(ctx context.Context, deviceSerial string, enable bool) error {
	return e.Tuner.SetAmbientExperience(ctx, deviceSerial, enable)
}

// SetSleepTimeout sets the inactive sleep timeout in minutes.
func (e *Engine) SetSleepTimeout(ctx context.Context, deviceSerial string, minutes int) error {
	return e.Tuner.SetSleepTimeout(ctx, deviceSerial, minutes)
}

// GetDNSTweakConfig returns the active DoT configuration and its preset friendly name.
func (e *Engine) GetDNSTweakConfig(ctx context.Context, deviceSerial string) (tuning.DNSTweakConfig, error) {
	return e.Tuner.GetDNSTweakConfig(ctx, deviceSerial)
}

// SetDNSProvider configures DNS-over-TLS with one of the curated presets (adguard, quad9, cloudflare, family, off).
func (e *Engine) SetDNSProvider(ctx context.Context, deviceSerial string, providerKey string) error {
	return e.Tuner.SetDNSProvider(ctx, deviceSerial, providerKey)
}

// CleanStockLauncher cleans, accelerates and sterilizes the stock Amazon launcher.
func (e *Engine) CleanStockLauncher(ctx context.Context, deviceSerial string, opts launcher.CleanOptions) (*launcher.CleanResult, error) {
	return e.Launcher.Clean(ctx, deviceSerial, opts)
}

// RestoreStockLauncher reverts all launcher settings to Amazon factory defaults.
func (e *Engine) RestoreStockLauncher(ctx context.Context, deviceSerial string) (*launcher.RestoreResult, error) {
	return e.Launcher.Restore(ctx, deviceSerial)
}

// GetLauncherStatus inspects the active launcher settings and ad rows status.
func (e *Engine) GetLauncherStatus(ctx context.Context, deviceSerial string) (*launcher.Status, error) {
	return e.Launcher.GetStatus(ctx, deviceSerial)
}

// HomeStatus reports which launcher owns the Home button.
func (e *Engine) HomeStatus(ctx context.Context, deviceSerial string) (*launcher.HomeStatus, error) {
	return e.Launcher.HomeStatus(ctx, deviceSerial)
}

// ReplaceWithWaterOS makes WaterOS the only home and disables the stock launcher (reversible).
func (e *Engine) ReplaceWithWaterOS(ctx context.Context, deviceSerial string) (*launcher.HomeStatus, error) {
	return e.Launcher.ReplaceWithWaterOS(ctx, deviceSerial)
}

// RestoreStockHome gives the Home button back to the Amazon launcher.
func (e *Engine) RestoreStockHome(ctx context.Context, deviceSerial string) (*launcher.HomeStatus, error) {
	return e.Launcher.RestoreStockHome(ctx, deviceSerial)
}

// RestartLauncher restarts com.amazon.tv.launcher to flush memory and reset UI state.
func (e *Engine) RestartLauncher(ctx context.Context, deviceSerial string) error {
	return e.Launcher.Restart(ctx, deviceSerial)
}



