package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/MedCy1/WaterStick/pkg/waterstick"
	"github.com/MedCy1/WaterStick/pkg/waterstick/launcher"
	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
	"github.com/MedCy1/WaterStick/pkg/waterstick/updater"
)

// App struct handles desktop UI interactions and routes them to the WaterStick core engine.
type App struct {
	ctx          context.Context
	engine       *waterstick.Engine
	targetDevice string
}

// NewApp creates a new App application struct.
func NewApp() *App {
	engine, err := waterstick.NewEngine("")
	if err != nil {
		fmt.Printf("Warning: failed to init engine in NewApp: %v\n", err)
	}
	return &App{
		engine: engine,
	}
}

// startup is called when the app starts. The context is saved so we can call the runtime methods.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	// Try to find pre-connected devices
	devs, err := a.engine.ADB.ListDevices(ctx)
	if err == nil && len(devs) > 0 {
		a.targetDevice = devs[0]
	}
}

// EnsureADB checks and auto-downloads ADB if missing.
func (a *App) EnsureADB() (string, error) {
	return a.engine.ADB.EnsureADB(a.ctx, nil)
}

// Connect connects to a Fire TV over Wi-Fi.
func (a *App) Connect(ip string) (string, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return "", fmt.Errorf("IP address cannot be empty")
	}
	if err := a.engine.ADB.Connect(a.ctx, ip); err != nil {
		return "", err
	}
	if !strings.Contains(ip, ":") {
		ip += ":5555"
	}
	a.targetDevice = ip
	return fmt.Sprintf("Successfully connected to %s", ip), nil
}

// GetConnectedDevice returns the currently active target device address.
func (a *App) GetConnectedDevice() string {
	if a.targetDevice != "" {
		return a.targetDevice
	}
	devs, err := a.engine.ADB.ListDevices(a.ctx)
	if err == nil && len(devs) > 0 {
		a.targetDevice = devs[0]
		return a.targetDevice
	}
	return ""
}

// InspectDevice retrieves hardware, OS, and memory metrics.
func (a *App) InspectDevice() (*models.Device, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return nil, fmt.Errorf("no connected Fire TV device found")
	}
	return a.engine.Inspector.Inspect(a.ctx, target)
}

// ApplyProfile applies the chosen optimization profile (e.g. 'balanced' or 'streamer').
func (a *App) ApplyProfile(profileID string) (*waterstick.ExecutionReport, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return nil, fmt.Errorf("no connected Fire TV device found")
	}
	return a.engine.ApplyProfile(a.ctx, target, profileID, nil)
}

// RollbackAll restores all disabled packages and defaults.
func (a *App) RollbackAll() (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("no connected Fire TV device found")
	}
	err := a.engine.Backup.RollbackAll(a.ctx, target, nil)
	if err != nil {
		return "", err
	}
	return "Rollback complete! All packages re-enabled and settings restored.", nil
}

// GetApps returns the list of sideloadable TV apps.
func (a *App) GetApps() []models.SideloadApp {
	target := a.GetConnectedDevice()
	if target != "" {
		apps, err := a.engine.Sideload.AnnotateInstalled(a.ctx, target)
		if err == nil {
			return apps
		}
	}
	return a.engine.Sideload.GetApps()
}

// InstallApp downloads and installs an APK via ADB.
func (a *App) InstallApp(appID string) (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("no connected Fire TV device found")
	}
	err := a.engine.Sideload.InstallApp(a.ctx, target, appID, nil)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Successfully installed %s!", appID), nil
}

// ToggleOTA toggles the OTA Shield (freezes or restores background updates).
func (a *App) ToggleOTA() (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("no connected Fire TV device found")
	}
	blocked, err := a.engine.IsOTABlocked(a.ctx, target)
	if err != nil {
		return "", err
	}
	if blocked {
		if err := a.engine.UnblockOTA(a.ctx, target); err != nil {
			return "", err
		}
		return "OTA Shield Deactivated. Factory updates restored.", nil
	}
	if err := a.engine.BlockOTA(a.ctx, target); err != nil {
		return "", err
	}
	return "OTA Shield Activated! Background Amazon updates are frozen.", nil
}

// PurgeRAM triggers cache and video buffer reclamation.
func (a *App) PurgeRAM() (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("no connected Fire TV device found")
	}
	before, after, freed, err := a.engine.PurgeMemory(a.ctx, target)
	if err != nil {
		return "", err
	}
	if freed > 0 {
		return fmt.Sprintf("RAM Purged! +%d MB freed (%d MB -> %d MB free)", freed, before, after), nil
	}
	return fmt.Sprintf("RAM Purged! Memory already optimal (%d MB free)", after), nil
}

// Disconnect disconnects from the currently connected Fire TV stick.
func (a *App) Disconnect() (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("aucun périphérique connecté à déconnecter")
	}
	err := a.engine.ADB.Disconnect(a.ctx, target)
	if err != nil {
		return "", err
	}
	a.targetDevice = ""
	return fmt.Sprintf("Déconnecté de %s", target), nil
}

// GetPackages returns the list of all managed packages with their live status.
func (a *App) GetPackages() ([]models.Package, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return nil, fmt.Errorf("aucun périphérique connecté")
	}
	return a.engine.Packages.GetPackagesWithStatus(a.ctx, target)
}

// TogglePackage enables or disables a package on user 0, with Alexa voice preservation.
func (a *App) TogglePackage(pkgID string) (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("aucun périphérique connecté")
	}
	pkg, known := a.engine.Packages.GetPackage(pkgID)
	if known && pkg.Tier == models.TierVoice {
		return "", fmt.Errorf("action interdite : le module vocal Alexa (%s) est sanctuarisé", pkgID)
	}

	disabledMap, err := a.engine.Packages.ListDisabled(a.ctx, target)
	if err != nil {
		return "", err
	}

	if disabledMap[pkgID] {
		if err := a.engine.Packages.EnablePackage(a.ctx, target, pkgID); err != nil {
			return "", err
		}
		return fmt.Sprintf("Paquet %s réactivé", pkgID), nil
	} else {
		if err := a.engine.Packages.DisablePackage(a.ctx, target, pkgID, false); err != nil {
			return "", err
		}
		return fmt.Sprintf("Paquet %s gelé", pkgID), nil
	}
}

// GetTweakStatus returns a consolidated view of audio, display, power, and DNS tweaks.
func (a *App) GetTweakStatus() (map[string]interface{}, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return nil, fmt.Errorf("aucun périphérique connecté")
	}

	audio, _ := a.engine.GetAudioTweakConfig(a.ctx, target)
	disp, _ := a.engine.GetDisplayTweakConfig(a.ctx, target)
	pwr, _ := a.engine.GetPowerTweakConfig(a.ctx, target)
	dns, _ := a.engine.GetDNSTweakConfig(a.ctx, target)

	return map[string]interface{}{
		"audio":   audio,
		"display": disp,
		"power":   pwr,
		"dns":     dns,
	}, nil
}

// SetOptical51 toggles standard Dolby Digital AC-3 5.1 bitstream for optical audio systems.
func (a *App) SetOptical51(enable bool) (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("aucun périphérique connecté")
	}
	if err := a.engine.SetOptical51Mode(a.ctx, target, enable); err != nil {
		return "", err
	}
	if enable {
		return "Mode Ampli 5.1 Optique ACTIVÉ (AC-3 Bitstream)", nil
	}
	return "Mode Audio restauré en Automatique", nil
}

// SetNavSounds toggles UI click sounds.
func (a *App) SetNavSounds(enable bool) (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("aucun périphérique connecté")
	}
	if err := a.engine.SetNavigationSounds(a.ctx, target, enable); err != nil {
		return "", err
	}
	if enable {
		return "Sons de navigation activés", nil
	}
	return "Sons de navigation désactivés (silencieux)", nil
}

// SetMatchFrameRate toggles native 24p cinema refresh rate switching.
func (a *App) SetMatchFrameRate(enable bool) (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("aucun périphérique connecté")
	}
	if err := a.engine.SetMatchFrameRate(a.ctx, target, enable); err != nil {
		return "", err
	}
	if enable {
		return "Match Original Frame Rate 24p ACTIVÉ", nil
	}
	return "Match Frame Rate désactivé (60Hz fixe)", nil
}

// SetAmbient toggles background Ambient ads.
func (a *App) SetAmbient(enable bool) (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("aucun périphérique connecté")
	}
	if err := a.engine.SetAmbientExperience(a.ctx, target, enable); err != nil {
		return "", err
	}
	if enable {
		return "Ambient Experience activé", nil
	}
	return "Ambient Experience DÉSACTIVÉ (veille publicitaire coupée)", nil
}

// SetSleepTimeout sets the inactivity sleep timeout in minutes.
func (a *App) SetSleepTimeout(minutes int) (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("aucun périphérique connecté")
	}
	if err := a.engine.SetSleepTimeout(a.ctx, target, minutes); err != nil {
		return "", err
	}
	return fmt.Sprintf("Délai de veille configuré à %d min", minutes), nil
}

// SetDNS configures DNS-over-TLS (adguard, quad9, cloudflare, family, off).
func (a *App) SetDNS(provider string) (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("aucun périphérique connecté")
	}
	if err := a.engine.SetDNSProvider(a.ctx, target, provider); err != nil {
		return "", err
	}
	cfg, _ := a.engine.GetDNSTweakConfig(a.ctx, target)
	return fmt.Sprintf("DoT DNS configuré : %s", cfg.ProviderName), nil
}

// UpdateCheckResult holds release update info for the desktop GUI.
type UpdateCheckResult struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	ReleaseNotes   string `json:"release_notes"`
}

// CheckUpdate queries GitHub for the latest release and reports if an update is available.
func (a *App) CheckUpdate() (*UpdateCheckResult, error) {
	u := updater.NewUpdateChecker()
	rel, isNewer, err := u.CheckLatest(a.ctx, version)
	if err != nil {
		return nil, err
	}
	return &UpdateCheckResult{
		Available:      isNewer,
		CurrentVersion: updater.FormatVersion(version),
		LatestVersion:  rel.TagName,
		ReleaseNotes:   rel.Body,
	}, nil
}

// ApplyUpdate downloads and installs the latest version in-place.
func (a *App) ApplyUpdate() (string, error) {
	u := updater.NewUpdateChecker()
	err := u.PerformSelfUpdate(a.ctx, version, nil)
	if err != nil {
		return "", err
	}
	return "Mise à jour installée avec succès. Veuillez relancer l'application.", nil
}

// CleanStockLauncher applies Stock Clean WaterOS optimizations to the stock Amazon launcher.
func (a *App) CleanStockLauncher(instant bool) (*launcher.CleanResult, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return nil, fmt.Errorf("aucun périphérique connecté")
	}
	opts := launcher.DefaultCleanOptions()
	if instant {
		opts.AnimationScale = 0.0
	}
	return a.engine.CleanStockLauncher(a.ctx, target, opts)
}

// RestoreStockLauncher restores the stock Amazon launcher to factory defaults.
func (a *App) RestoreStockLauncher() (*launcher.RestoreResult, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return nil, fmt.Errorf("aucun périphérique connecté")
	}
	return a.engine.RestoreStockLauncher(a.ctx, target)
}

// GetLauncherStatus queries current stock launcher sterilisation status.
func (a *App) GetLauncherStatus() (*launcher.Status, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return nil, fmt.Errorf("aucun périphérique connecté")
	}
	return a.engine.GetLauncherStatus(a.ctx, target)
}

// ReplaceWithWaterOS makes WaterOS the only home screen (reversible).
func (a *App) ReplaceWithWaterOS() (*launcher.HomeStatus, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return nil, fmt.Errorf("aucun périphérique connecté")
	}
	return a.engine.ReplaceWithWaterOS(a.ctx, target)
}

// RestoreStockHome gives the Home button back to the Amazon launcher.
func (a *App) RestoreStockHome() (*launcher.HomeStatus, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return nil, fmt.Errorf("aucun périphérique connecté")
	}
	return a.engine.RestoreStockHome(a.ctx, target)
}

// HomeStatus reports which launcher owns the Home button.
func (a *App) HomeStatus() (*launcher.HomeStatus, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return nil, fmt.Errorf("aucun périphérique connecté")
	}
	return a.engine.HomeStatus(a.ctx, target)
}

// RestartLauncher force-stops com.amazon.tv.launcher to flush RAM and UI state.
func (a *App) RestartLauncher() (string, error) {
	target := a.GetConnectedDevice()
	if target == "" {
		return "", fmt.Errorf("aucun périphérique connecté")
	}
	if err := a.engine.RestartLauncher(a.ctx, target); err != nil {
		return "", err
	}
	return "Launcher redémarré avec succès", nil
}



