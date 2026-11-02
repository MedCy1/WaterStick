package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/MedCy1/WaterStick/pkg/waterstick"
	"github.com/MedCy1/WaterStick/pkg/waterstick/launcher"
	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
	"github.com/MedCy1/WaterStick/pkg/waterstick/updater"
	"github.com/spf13/cobra"
)

var (
	version      = "dev"
	targetDevice string
	adbPath      string
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "waterstick",
		Version: updater.FormatVersion(version),
		Args:    cobra.NoArgs,
		Short:   "WaterStick - Fire TV Stick 4K Max 2nd Gen Debloat & Optimization Suite",
		Long: `WaterStick is a safe, non-destructive, and reversible optimization suite
tailored for Amazon Fire TV Stick 4K Max 2nd Gen (Fire OS 8 / Android 11).

It silences video/audio ads, blocks tracking via private encrypted DNS,
boosts memory, and accelerates UI responsiveness while preserving 100% of
the Alexa voice remote control.`,
	}

	rootCmd.PersistentFlags().StringVarP(&targetDevice, "device", "d", "", "Target Fire TV IP or serial (e.g. 192.168.1.50:5555)")
	rootCmd.PersistentFlags().StringVar(&adbPath, "adb-path", "", "Custom path to ADB executable")

	// 1. connect
	connectCmd := &cobra.Command{
		Use:   "connect <IP[:port]>",
		Short: "Connect to a Fire TV stick over Wi-Fi",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()

			fmt.Println("Ensuring ADB platform-tools are ready...")
			_, err = engine.ADB.EnsureADB(ctx, func(pct int, msg string) {
				fmt.Printf("[%d%%] %s\n", pct, msg)
			})
			if err != nil {
				return err
			}

			ip := args[0]
			fmt.Printf("Connecting to %s...\n", ip)
			if err := engine.ADB.Connect(ctx, ip); err != nil {
				return err
			}
			fmt.Printf("[OK] Connexion établie avec succès : %s\n", ip)
			return nil
		},
	}

	// 1b. disconnect
	disconnectCmd := &cobra.Command{
		Use:   "disconnect [IP[:port]]",
		Short: "Disconnect from a Fire TV stick",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			target := targetDevice
			if len(args) > 0 {
				target = args[0]
			}
			if target == "" {
				target = resolveDevice(engine, "")
			}
			if target == "" {
				return fmt.Errorf("no connected device to disconnect")
			}
			fmt.Printf("Disconnecting from %s...\n", target)
			if err := engine.ADB.Disconnect(ctx, target); err != nil {
				return err
			}
			fmt.Printf("[OK] Déconnecté de %s\n", target)
			return nil
		},
	}

	// 2. devices
	devicesCmd := &cobra.Command{
		Use:   "devices",
		Short: "List connected ADB devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devices, err := engine.ADB.ListDevices(ctx)
			if err != nil {
				return err
			}
			if len(devices) == 0 {
				fmt.Println("No connected Fire TV devices found. Run 'waterstick connect <IP>' first.")
				return nil
			}
			fmt.Println("Connected devices:")
			for _, d := range devices {
				fmt.Printf("  • %s\n", d)
			}
			return nil
		},
	}

	// 3. health / diagnostics
	healthCmd := &cobra.Command{
		Use:   "health",
		Short: "Run diagnostics on the connected Fire TV",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			fmt.Printf("Running diagnostics on %s...\n\n", devSerial)
			info, err := engine.Inspector.Inspect(ctx, devSerial)
			if err != nil {
				return err
			}

			fmt.Println("════════════════════════════════════════════════════════════════")
			fmt.Printf(" Device Model:        %s (%s)\n", info.ModelName, info.Model)
			fmt.Printf(" Operating System:    %s\n", info.FireOSVersion)
			fmt.Printf(" Android Base:        API / Release %s\n", info.AndroidVersion)
			if info.IsFireTV4KMax2Gen {
				fmt.Println(" Model Compatibility: [OK] Confirmed Fire TV Stick 4K Max 2nd Gen")
			} else {
				fmt.Println(" Model Compatibility: [WARN] Generic Fire TV detected")
			}
			fmt.Printf(" Storage (/data):     %s free of %s (%s used)\n", info.FreeStorage, info.TotalStorage, info.UsedStoragePct)
			fmt.Printf(" Total RAM:           %d MB (Free: %d MB | ZRAM Swap: %d MB)\n", info.TotalRAMMB, info.FreeRAMMB, info.ZRAMUsedMB)
			if info.CPUTempC > 0 {
				thermalStatusDesc := "Normal"
				if info.ThermalStatus == 1 {
					thermalStatusDesc = "Light Throttle"
				} else if info.ThermalStatus >= 2 {
					thermalStatusDesc = "Heavy Throttle"
				}
				fmt.Printf(" SoC Thermal Sensor:  %.1f °C CPU | %.1f °C GPU (%s)\n", info.CPUTempC, info.GPUTempC, thermalStatusDesc)
			}
			if len(info.CPUFreqMHz) > 0 {
				var freqStrs []string
				for idx, f := range info.CPUFreqMHz {
					freqStrs = append(freqStrs, fmt.Sprintf("C%d: %dMHz", idx, f))
				}
				fmt.Printf(" CPU Clocks (4 cores):%s\n", strings.Join(freqStrs, " | "))
			}
			if info.CPULoad != "" {
				fmt.Printf(" CPU Load Average:    %s\n", info.CPULoad)
			}
			if info.OTABlocked {
				fmt.Println(" OTA Shield Status:   [ACTIVE] FROZEN / BLOCKED (Safe from forced Amazon OS updates)")
			} else {
				fmt.Println(" OTA Shield Status:   [INACTIVE] UNPROTECTED (Amazon background updates active)")
			}
			fmt.Printf(" Active Home:         %s\n", info.ActiveLauncher)
			fmt.Printf(" Disabled Packages:   %d packages currently disabled\n", info.DisabledCount)
			fmt.Println("════════════════════════════════════════════════════════════════")
			return nil
		},
	}

	// 3b. packages
	var filterDisabled bool
	var filterActive bool
	packagesCmd := &cobra.Command{
		Use:   "packages",
		Short: "List managed Fire TV packages and their active/frozen status",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}
			pkgs, err := engine.Packages.GetPackagesWithStatus(ctx, devSerial)
			if err != nil {
				return err
			}
			fmt.Printf("Managed packages on %s:\n\n", devSerial)
			fmt.Printf("  %-38s %-10s %s\n", "PACKAGE ID", "STATUT", "TIER / ROLE")
			fmt.Println("  ────────────────────────────────────────────────────────────────────────")
			for _, p := range pkgs {
				if filterDisabled && !p.Disabled {
					continue
				}
				if filterActive && p.Disabled {
					continue
				}
				status := "[ACTIVE]"
				if p.Disabled {
					status = "[FROZEN]"
				}
				tier := string(p.Tier)
				if p.Tier == models.TierVoice {
					tier = "[ALEXA VOICE - PROTECTED]"
				}
				fmt.Printf("  %-38s %-10s %s\n", p.ID, status, tier)
			}
			return nil
		},
	}
	packagesCmd.Flags().BoolVar(&filterDisabled, "disabled", false, "Show only disabled/frozen packages")
	packagesCmd.Flags().BoolVar(&filterActive, "active", false, "Show only active packages")

	// 3c. freeze
	freezeCmd := &cobra.Command{
		Use:   "freeze <package-id>",
		Short: "Freeze (disable) a specific package on the Fire TV (voice-protected)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}
			pkgID := args[0]
			fmt.Printf("Freezing package %s on %s...\n", pkgID, devSerial)
			if err := engine.Packages.DisablePackage(ctx, devSerial, pkgID, false); err != nil {
				return err
			}
			fmt.Printf("[OK] Package %s is now frozen (disabled)\n", pkgID)
			return nil
		},
	}

	// 3d. unfreeze
	unfreezeCmd := &cobra.Command{
		Use:   "unfreeze <package-id>",
		Short: "Unfreeze (re-enable) a specific package on the Fire TV",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}
			pkgID := args[0]
			fmt.Printf("Unfreezing package %s on %s...\n", pkgID, devSerial)
			if err := engine.Packages.EnablePackage(ctx, devSerial, pkgID); err != nil {
				return err
			}
			fmt.Printf("[OK] Package %s is now unfrozen (re-enabled)\n", pkgID)
			return nil
		},
	}

	// 4. debloat
	var profileFlag string
	var forceVoiceFlag bool
	debloatCmd := &cobra.Command{
		Use:   "debloat",
		Short: "Apply a debloat and performance profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			if profileFlag == "" {
				profileFlag = "balanced"
			}

			prof, ok := engine.Profiles[profileFlag]
			if !ok {
				return fmt.Errorf("unknown profile %q. Available: balanced, streamer", profileFlag)
			}

			fmt.Printf("Applying Profile: %s\n", prof.Name)
			fmt.Printf("   Description: %s\n", prof.Description)
			if prof.PreservesVoice {
				fmt.Println("   Alexa Voice Remote: PRESERVED & PROTECTED")
			} else if forceVoiceFlag {
				fmt.Println("   [WARN] Alexa Voice Remote: DISABLED BY REQUEST")
			}
			fmt.Println()

			report, err := engine.ApplyProfile(ctx, devSerial, profileFlag, func(step, total int, msg string) {
				fmt.Printf("[%d/%d] %s\n", step, total, msg)
			})
			if err != nil {
				return err
			}

			fmt.Println("\nOptimization complete.")
			fmt.Printf(" • Safety Snapshot ID: %s\n", report.BackupID)
			fmt.Printf(" • Packages Disabled:  %d\n", report.DisabledCount)
			if report.SkippedCount > 0 {
				fmt.Printf(" • Packages Skipped:   %d (not installed on this device)\n", report.SkippedCount)
			}
			fmt.Printf(" • System Settings:    %d applied\n", report.SettingsApplied)
			if report.RAMFreedMB > 0 {
				fmt.Printf(" • RAM Reclaimed:      +%d MB (Free RAM: %d MB -> %d MB)\n", report.RAMFreedMB, report.RAMBeforeMB, report.RAMAfterMB)
			}
			if len(report.FailedPackages) > 0 {
				fmt.Println(" • Warnings / Failures:")
				for _, f := range report.FailedPackages {
					fmt.Printf("   - %s\n", f)
				}
			}
			fmt.Println("\nTo revert everything at any time, run: waterstick rollback")
			return nil
		},
	}
	debloatCmd.Flags().StringVarP(&profileFlag, "profile", "p", "balanced", "Profile to apply: 'balanced' (recommended) or 'streamer'")
	debloatCmd.Flags().BoolVar(&forceVoiceFlag, "force-voice-disable", false, "Explicitly allow disabling Alexa voice packages")

	// 5. rollback
	rollbackCmd := &cobra.Command{
		Use:   "rollback",
		Short: "Revert all changes and restore every package to factory state",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found")
			}

			fmt.Printf("Rolling back all disabled packages on %s...\n", devSerial)
			err = engine.Backup.RollbackAll(ctx, devSerial, func(pkg string, cmdErr error) {
				if cmdErr != nil {
					fmt.Printf("  [WARN] Failed restoring %s: %v\n", pkg, cmdErr)
				} else {
					fmt.Printf("  [OK] Restored %s\n", pkg)
				}
			})
			if err != nil {
				return err
			}
			fmt.Println("\nRollback complete. All packages re-enabled and settings restored to default.")
			return nil
		},
	}

	// 6. sideload apps
	var installAllFlag bool
	sideloadCmd := &cobra.Command{
		Use:   "install [app-id...]",
		Short: "Sideload open-source TV apps (smarttube, stremio, kodi, nova, vlc, jellyfin)",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found")
			}

			var targets []string
			if installAllFlag {
				for _, a := range engine.Sideload.GetApps() {
					targets = append(targets, a.ID)
				}
			} else {
				if len(args) == 0 {
					return fmt.Errorf("specify at least one app-id to install, or use --all")
				}
				targets = args
			}

			for _, id := range targets {
				appID := strings.ToLower(id)
				fmt.Printf("Installing %s on %s...\n", appID, devSerial)
				err := engine.Sideload.InstallApp(ctx, devSerial, appID, func(pct int, msg string) {
					fmt.Printf("  [%d%%] %s\n", pct, msg)
				})
				if err != nil {
					fmt.Printf("Failed to install %s: %v\n", appID, err)
				} else {
					fmt.Printf("[OK] Application %s successfully installed.\n", appID)
				}
			}
			return nil
		},
	}
	sideloadCmd.Flags().BoolVarP(&installAllFlag, "all", "a", false, "Install all available apps in catalog")

	// 7. list-apps
	listAppsCmd := &cobra.Command{
		Use:   "apps",
		Short: "List available sideloadable applications in the catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)

			var apps []models.SideloadApp
			if devSerial != "" {
				apps, _ = engine.Sideload.AnnotateInstalled(ctx, devSerial)
			} else {
				apps = engine.Sideload.GetApps()
			}

			fmt.Println("Available Sideload Applications:")
			for _, a := range apps {
				status := ""
				if a.Installed {
					status = " [INSTALLED]"
				}
				fmt.Printf(" • %-12s - %s%s\n", a.ID, a.Name, status)
				fmt.Printf("   %s\n", a.Description)
			}
			return nil
		},
	}

	// 8. ota (shield management)
	otaCmd := &cobra.Command{
		Use:   "ota",
		Short: "Manage the OTA Shield (freeze or unfreeze Amazon background OS updates)",
	}

	otaStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Check whether background Amazon OTA updates are frozen",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}
			blocked, err := engine.IsOTABlocked(ctx, devSerial)
			if err != nil {
				return err
			}
			if blocked {
				fmt.Println("OTA Shield is ACTIVE: Background Amazon OS updates are completely frozen.")
				fmt.Println("   Your custom launchers, settings, and sideloaded apps are safe from forced updates.")
			} else {
				fmt.Println("OTA Shield is INACTIVE: Background Amazon OS updates are active.")
				fmt.Println("   Run 'waterstick ota block' to freeze automatic updates safely.")
			}
			return nil
		},
	}

	otaBlockCmd := &cobra.Command{
		Use:   "block",
		Short: "Block background Amazon OTA updates permanently",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}
			fmt.Printf("Activating OTA Shield on %s...\n", devSerial)
			if err := engine.BlockOTA(ctx, devSerial); err != nil {
				return err
			}
			fmt.Println("OTA Shield Activated.")
			fmt.Println(" • Background updater services restricted (RUN_IN_BACKGROUND: ignore)")
			fmt.Println(" • OOBE forced update wizard disabled")
			fmt.Println(" • Bluetooth remote control & Alexa voice remote mic: 100% PRESERVED")
			return nil
		},
	}

	otaUnblockCmd := &cobra.Command{
		Use:   "unblock",
		Short: "Unblock background Amazon OTA updates and restore factory defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}
			fmt.Printf("Deactivating OTA Shield on %s...\n", devSerial)
			if err := engine.UnblockOTA(ctx, devSerial); err != nil {
				return err
			}
			fmt.Println("OTA Shield Deactivated. Factory Amazon update services restored.")
			return nil
		},
	}

	otaCmd.AddCommand(otaStatusCmd, otaBlockCmd, otaUnblockCmd)

	// 9. purge / clean-ram
	purgeCmd := &cobra.Command{
		Use:     "purge",
		Aliases: []string{"clean-ram", "ram-clean"},
		Short:   "Reclaim RAM and flush orphaned video buffer caches in 1-click",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}
			fmt.Printf("Purging memory and video buffer caches on %s...\n", devSerial)
			before, after, freed, err := engine.PurgeMemory(ctx, devSerial)
			if err != nil {
				return err
			}
			fmt.Printf("[OK] Mémoire et tampons vidéo purgés avec succès.\n")
			fmt.Printf(" - RAM libre avant : %d Mo\n", before)
			fmt.Printf(" - RAM libre après : %d Mo\n", after)
			if freed > 0 {
				fmt.Printf(" - RAM récupérée   : +%d Mo\n", freed)
			} else {
				fmt.Println(" - RAM récupérée   : Mémoire déjà optimisée.")
			}
			return nil
		},
	}

	// 9. tweak
	tweakCmd := &cobra.Command{
		Use:   "tweak",
		Short: "Manage advanced hardware & system tweaks (Audio, Display, Power, DNS)",
	}

	tweakStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show current configuration of all advanced tweaks",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			audio, _ := engine.GetAudioTweakConfig(ctx, devSerial)
			disp, _ := engine.GetDisplayTweakConfig(ctx, devSerial)
			pwr, _ := engine.GetPowerTweakConfig(ctx, devSerial)
			dns, _ := engine.GetDNSTweakConfig(ctx, devSerial)

			fmt.Println()
			fmt.Printf("=== RÉGLAGES AVANCÉS & MATÉRIEL [%s] ===\n\n", devSerial)

			fmt.Println("[AUDIO & PASSTHROUGH]")
			optStatus := "Auto / Par défaut"
			if audio.IsOptical51Optimized {
				optStatus = "ACTIVÉ (Dolby Digital 5.1 AC-3 pour Ampli Optique/TOSLink)"
			}
			fmt.Printf("  Mode Ampli Optique 5.1  : %s\n", optStatus)
			navStatus := "Activés"
			if !audio.NavigationSounds {
				navStatus = "Désactivés (Silencieux)"
			}
			fmt.Printf("  Sons de navigation      : %s\n", navStatus)
			fmt.Printf("  Format audio brut       : Device=%s, Surround=%s, Formats=%s\n",
				audio.CompressedAudioPref, audio.EncodedSurroundOutput, audio.SurroundEnabledFormats)
			fmt.Println()

			fmt.Println("[AFFICHAGE & GAMING]")
			matchStatus := "Désactivé (60Hz fixe)"
			if disp.MatchFrameRate {
				matchStatus = "ACTIVÉ (Match 24p Cinéma natif sans saccades)"
			}
			fmt.Printf("  Match Original Frame Rate : %s\n", matchStatus)
			fmt.Printf("  Résolution TV             : %s\n", disp.Resolution)
			fmt.Printf("  Plage Dynamique HDR       : %s\n", disp.HDRSetting)
			fmt.Printf("  Profondeur de couleur     : %s-bit\n", disp.DeepColor)
			fmt.Printf("  Mode Faible Latence (ALLM): %s\n", disp.LowLatencyMode)
			fmt.Printf("  Super Résolution IA       : %s\n", disp.SuperResolution)
			fmt.Println()

			fmt.Println("[ALIMENTATION & VEILLE]")
			ambStatus := "Actif"
			if !pwr.AmbientExperienceEnabled {
				ambStatus = "DÉSACTIVÉ (Écrans sponsorisés coupés)"
			}
			fmt.Printf("  Ambient Experience (Pubs): %s\n", ambStatus)
			fmt.Printf("  Délai d'inactivité veille: %d minutes\n", pwr.SleepTimeoutMinutes)
			fmt.Println()

			fmt.Println("[RÉSEAU & VIE PRIVÉE (DoT DNS)]")
			fmt.Printf("  Fournisseur DoT : %s\n", dns.ProviderName)
			fmt.Printf("  Mode / Serveur  : Mode=%s, Hôte=%s\n", dns.Mode, dns.Specifier)
			fmt.Println()

			return nil
		},
	}

	var opt51Flag, autoAudioFlag bool
	var navSoundsFlag string
	tweakAudioCmd := &cobra.Command{
		Use:   "audio",
		Short: "Configure audio output & 5.1 optical passthrough",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			if opt51Flag {
				if err := engine.SetOptical51Mode(ctx, devSerial, true); err != nil {
					return err
				}
				fmt.Println("[OK] Mode Ampli 5.1 Optique ACTIVÉ (Sortie AC-3 Bitstream Dolby Digital forcée).")
			} else if autoAudioFlag {
				if err := engine.SetOptical51Mode(ctx, devSerial, false); err != nil {
					return err
				}
				fmt.Println("[OK] Mode Audio restauré en Automatique (Négociation HDMI par défaut).")
			}

			if navSoundsFlag != "" {
				enable := navSoundsFlag == "on" || navSoundsFlag == "1" || navSoundsFlag == "true"
				if err := engine.SetNavigationSounds(ctx, devSerial, enable); err != nil {
					return err
				}
				if enable {
					fmt.Println("[OK] Sons de navigation réactivés.")
				} else {
					fmt.Println("[OK] Sons de navigation désactivés (mode silencieux).")
				}
			}

			return nil
		},
	}
	tweakAudioCmd.Flags().BoolVar(&opt51Flag, "optical-51", false, "Force standard Dolby Digital 5.1 AC-3 for Optical/TOSLink audio amplifiers")
	tweakAudioCmd.Flags().BoolVar(&autoAudioFlag, "auto", false, "Reset audio format to default automatic HDMI negotiation")
	tweakAudioCmd.Flags().StringVar(&navSoundsFlag, "nav-sounds", "", "Toggle remote navigation click sounds (on/off)")

	var matchFrameRateFlag string
	tweakDisplayCmd := &cobra.Command{
		Use:   "display",
		Short: "Configure display and 24p cinema match frame rate",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			if matchFrameRateFlag != "" {
				enable := matchFrameRateFlag == "on" || matchFrameRateFlag == "1" || matchFrameRateFlag == "true"
				if err := engine.SetMatchFrameRate(ctx, devSerial, enable); err != nil {
					return err
				}
				if enable {
					fmt.Println("[OK] Match Original Frame Rate ACTIVÉ (lecture fluide 24p cinéma native).")
				} else {
					fmt.Println("[OK] Match Original Frame Rate désactivé (60Hz fixe).")
				}
			} else {
				fmt.Println("Usage: waterstick tweak display --match-framerate [on|off]")
			}
			return nil
		},
	}
	tweakDisplayCmd.Flags().StringVar(&matchFrameRateFlag, "match-framerate", "", "Enable or disable native 24p match frame rate (on/off)")

	var ambientFlag string
	var sleepMinFlag int
	tweakPowerCmd := &cobra.Command{
		Use:   "power",
		Short: "Configure ambient ads experience and sleep timeouts",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			if ambientFlag != "" {
				enable := ambientFlag == "on" || ambientFlag == "1" || ambientFlag == "true"
				if err := engine.SetAmbientExperience(ctx, devSerial, enable); err != nil {
					return err
				}
				if enable {
					fmt.Println("[OK] Ambient Experience réactivé.")
				} else {
					fmt.Println("[OK] Ambient Experience DÉSACTIVÉ (veille publicitaire coupée).")
				}
			}

			if sleepMinFlag > 0 {
				if err := engine.SetSleepTimeout(ctx, devSerial, sleepMinFlag); err != nil {
					return err
				}
				fmt.Printf("[OK] Délai d'inactivité avant veille configuré à %d minutes.\n", sleepMinFlag)
			}
			return nil
		},
	}
	tweakPowerCmd.Flags().StringVar(&ambientFlag, "ambient", "", "Toggle Ambient Experience ads (on/off)")
	tweakPowerCmd.Flags().IntVar(&sleepMinFlag, "sleep", 0, "Set sleep timeout in minutes (e.g. 15, 20, 30)")

	tweakDNSCmd := &cobra.Command{
		Use:   "dns [adguard|quad9|cloudflare|family|off]",
		Short: "Configure encrypted DNS-over-TLS (DoT) ad-blocking resolver",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			provider := args[0]
			if err := engine.SetDNSProvider(ctx, devSerial, provider); err != nil {
				return err
			}
			cfg, _ := engine.GetDNSTweakConfig(ctx, devSerial)
			fmt.Printf("[OK] Résolveur DNS DoT configuré : %s (%s)\n", cfg.ProviderName, cfg.Specifier)
			return nil
		},
	}

	tweakCmd.AddCommand(tweakStatusCmd, tweakAudioCmd, tweakDisplayCmd, tweakPowerCmd, tweakDNSCmd)

	// Launcher - Stock Clean WaterOS optimizations
	launcherCmd := &cobra.Command{
		Use:     "launcher",
		Aliases: []string{"home", "clean-launcher"},
		Short:   "Clean, optimize, and sterilize the stock Amazon launcher (Stock Clean WaterOS)",
		Long: `Transform the stock Amazon launcher into a blazing-fast, ad-free experience.
Silences trailer video autoplay and audio, accelerates or zeroes UI animations,
freezes commercial recommendation rows, and flushes launcher cache.`,
	}

	var instantAnimFlag bool
	var customScaleFlag float64
	var noFreezeAdsFlag bool
	var silenceNavFlag bool
	var keepCacheFlag bool

	launcherCleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Apply Stock Clean WaterOS optimizations to the Amazon launcher",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			opts := launcher.DefaultCleanOptions()
			if instantAnimFlag {
				opts.AnimationScale = 0.0
			} else if customScaleFlag >= 0.0 {
				opts.AnimationScale = customScaleFlag
			}
			if noFreezeAdsFlag {
				opts.FreezeAdRows = false
			}
			if silenceNavFlag {
				opts.SilenceNavSounds = true
			}
			if keepCacheFlag {
				opts.ClearCache = false
			}

			fmt.Println("Applying Stock Clean WaterOS optimizations...")
			res, err := engine.CleanStockLauncher(ctx, devSerial, opts)
			if err != nil {
				return fmt.Errorf("failed to clean launcher: %w", err)
			}

			fmt.Println("\n[OK] Launcher optimization complete:")
			if res.AutoplayDisabled {
				fmt.Println(" • Video autoplay banner disabled (featured trailer playback blocked)")
			}
			if res.BannerMuted {
				fmt.Println(" • Banner video volume muted (0.0)")
			}
			if res.AnimationSpeedup {
				fmt.Printf(" • UI animations accelerated (scale: %.1fx)\n", opts.AnimationScale)
			}
			if len(res.FrozenAdPackages) > 0 {
				fmt.Printf(" • Frozen commercial ad & recommendation row packages (%d):\n", len(res.FrozenAdPackages))
				for _, pkg := range res.FrozenAdPackages {
					fmt.Printf("   - %s\n", pkg)
				}
			}
			if res.CacheCleared {
				fmt.Println(" • System & launcher caches purged")
			}
			if res.LauncherRestarted {
				fmt.Println(" • Amazon Launcher restarted (flushed memory and applied clean UI)")
			}
			return nil
		},
	}
	launcherCleanCmd.Flags().BoolVar(&instantAnimFlag, "instant", false, "Set UI animation scale to 0.0 for 0ms instantaneous navigation")
	launcherCleanCmd.Flags().Float64Var(&customScaleFlag, "scale", -1.0, "Custom animation scale (e.g. 0.0, 0.25, 0.5)")
	launcherCleanCmd.Flags().BoolVar(&noFreezeAdsFlag, "no-freeze", false, "Do not freeze commercial ad and recommendation row packages")
	launcherCleanCmd.Flags().BoolVar(&silenceNavFlag, "silence-nav", false, "Silence remote navigation click sounds")
	launcherCleanCmd.Flags().BoolVar(&keepCacheFlag, "keep-cache", false, "Skip clearing launcher and disk caches")

	launcherStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Display current launcher optimization and sterilization status",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			st, err := engine.GetLauncherStatus(ctx, devSerial)
			if err != nil {
				return fmt.Errorf("failed to get launcher status: %w", err)
			}

			stateLabel := "[STOCK CLEAN WATEROS]"
			if !st.IsClean {
				stateLabel = "[STOCK AMAZON (ADS ACTIVE)]"
			}
			fmt.Printf("Launcher Status on %s: %s\n\n", devSerial, stateLabel)

			autoStatus := "ENABLED (Standard Amazon)"
			if st.AutoplayDisabled {
				autoStatus = "DISABLED (Blocked)"
			}
			fmt.Printf(" • Featured Video Autoplay: %s\n", autoStatus)

			audioStatus := "UNMUTED"
			if st.BannerMuted {
				audioStatus = "MUTED (0.0)"
			}
			fmt.Printf(" • Banner Audio Volume:     %s\n", audioStatus)

			pvdeStatus := "ENABLED"
			if st.PvdeDisabled {
				pvdeStatus = "DISABLED (Blocked direct Prime stream)"
			}
			fmt.Printf(" • PVDE Direct Playback:    %s\n", pvdeStatus)

			fmt.Printf(" • Animation Scales:        Window=%s, Transition=%s, Animator=%s\n",
				st.WindowAnimScale, st.TransitionAnimScale, st.AnimatorScale)

			fmt.Printf(" • Commercial Ad Packages:  %d frozen / %d total\n",
				len(st.FrozenAdPackages), len(st.FrozenAdPackages)+len(st.ActiveAdPackages))
			if len(st.FrozenAdPackages) > 0 {
				fmt.Println("   Frozen packages:")
				for _, p := range st.FrozenAdPackages {
					fmt.Printf("     - %s\n", p)
				}
			}
			if len(st.ActiveAdPackages) > 0 {
				fmt.Println("   Active commercial packages:")
				for _, p := range st.ActiveAdPackages {
					fmt.Printf("     - %s\n", p)
				}
			}

			return nil
		},
	}

	launcherRestoreCmd := &cobra.Command{
		Use:   "restore",
		Short: "Restore Amazon launcher settings and commercial packages to factory defaults",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			fmt.Println("Reverting launcher to Amazon factory defaults...")
			res, err := engine.RestoreStockLauncher(ctx, devSerial)
			if err != nil {
				return fmt.Errorf("failed to restore launcher: %w", err)
			}

			fmt.Println("\n[OK] Launcher restored to factory state:")
			if res.AutoplayRestored {
				fmt.Println(" • Featured video autoplay & banner audio re-enabled")
			}
			if res.AnimationRestored {
				fmt.Println(" • Standard animation scales restored (1.0x)")
			}
			if len(res.UnfrozenAdPackages) > 0 {
				fmt.Printf(" • Unfrozen commercial packages (%d):\n", len(res.UnfrozenAdPackages))
				for _, pkg := range res.UnfrozenAdPackages {
					fmt.Printf("   - %s\n", pkg)
				}
			}
			if res.LauncherRestarted {
				fmt.Println(" • Launcher restarted to apply stock settings")
			}
			return nil
		},
	}

	launcherRestartCmd := &cobra.Command{
		Use:   "restart",
		Short: "Restart com.amazon.tv.launcher to flush RAM and UI state",
		RunE: func(cmd *cobra.Command, args []string) error {
			engine, err := waterstick.NewEngine(adbPath)
			if err != nil {
				return err
			}
			ctx := context.Background()
			devSerial := resolveDevice(engine, targetDevice)
			if devSerial == "" {
				return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
			}

			if err := engine.RestartLauncher(ctx, devSerial); err != nil {
				return fmt.Errorf("failed to restart launcher: %w", err)
			}
			fmt.Printf("[OK] Launcher restarted on %s\n", devSerial)
			return nil
		},
	}

	homeCmd := func(use, short string, run func(*waterstick.Engine, context.Context, string) (*launcher.HomeStatus, error)) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, args []string) error {
				engine, err := waterstick.NewEngine(adbPath)
				if err != nil {
					return err
				}
				ctx := context.Background()
				devSerial := resolveDevice(engine, targetDevice)
				if devSerial == "" {
					return fmt.Errorf("no target device found. Connect first with 'waterstick connect <IP>'")
				}
				st, err := run(engine, ctx, devSerial)
				if err != nil {
					return err
				}
				fmt.Printf("Home activity:        %s\n", st.HomeActivity)
				fmt.Printf("WaterOS installed:    %v\n", st.WaterOSInstalled)
				fmt.Printf("Stock launcher off:   %v\n", st.StockDisabled)
				fmt.Printf("Home starter off:     %v\n", st.StarterDisabled)
				if st.WaterOSIsHome {
					fmt.Println("[OK] WaterOS is the home screen; the Amazon launcher uses no RAM.")
				} else {
					fmt.Println("[OK] The Amazon launcher is the home screen.")
				}
				return nil
			},
		}
	}
	launcherReplaceCmd := homeCmd("replace", "Make WaterOS the only home screen and disable the Amazon launcher (reversible with 'launcher stock')",
		func(e *waterstick.Engine, ctx context.Context, s string) (*launcher.HomeStatus, error) { return e.ReplaceWithWaterOS(ctx, s) })
	launcherStockCmd := homeCmd("stock", "Re-enable the Amazon launcher and give it the Home button back",
		func(e *waterstick.Engine, ctx context.Context, s string) (*launcher.HomeStatus, error) { return e.RestoreStockHome(ctx, s) })
	launcherHomeCmd := homeCmd("home-status", "Show which launcher owns the Home button",
		func(e *waterstick.Engine, ctx context.Context, s string) (*launcher.HomeStatus, error) { return e.HomeStatus(ctx, s) })

	launcherCmd.AddCommand(launcherCleanCmd, launcherStatusCmd, launcherRestoreCmd, launcherRestartCmd, launcherReplaceCmd, launcherStockCmd, launcherHomeCmd)

	// 10. tui
	tuiCmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the interactive Terminal User Interface (Bubble Tea v2)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Execute waterstick-tui
			execCmd := exec.Command("waterstick-tui")
			execCmd.Stdin = os.Stdin
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr
			return execCmd.Run()
		},
	}

	// 9. gui
	guiCmd := &cobra.Command{
		Use:   "gui",
		Short: "Launch the Desktop Graphical User Interface (Wails v2)",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Execute waterstick-gui
			execCmd := exec.Command("waterstick-gui")
			execCmd.Stdin = os.Stdin
			execCmd.Stdout = os.Stdout
			execCmd.Stderr = os.Stderr
			return execCmd.Run()
		},
	}

	// 14. about
	aboutCmd := &cobra.Command{
		Use:   "about",
		Short: "Display information about WaterStick and author",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("WaterStick " + updater.FormatVersion(version))
			fmt.Println("Fire TV Stick 4K Max 2nd Gen Debloat & Optimization Suite")
			fmt.Println("Author: MedCy1 (https://github.com/MedCy1)")
			fmt.Println("Repository: https://github.com/MedCy1/WaterStick")
			fmt.Println("License: PolyForm Noncommercial 1.0.0")
		},
	}

	// 15. update
	var checkOnlyFlag bool
	updateCmd := &cobra.Command{
		Use:   "update",
		Short: "Check for and apply automatic WaterStick updates from GitHub",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			u := updater.NewUpdateChecker()

			fmt.Println("Checking for latest release on GitHub...")
			rel, isNewer, err := u.CheckLatest(ctx, version)
			if err != nil {
				return fmt.Errorf("update check failed: %w", err)
			}

			if !isNewer {
				fmt.Printf("WaterStick is already up to date (current: %s, latest: %s).\n", updater.FormatVersion(version), rel.TagName)
				return nil
			}

			fmt.Printf("A new version of WaterStick is available: %s -> %s\n", updater.FormatVersion(version), rel.TagName)
			if rel.Body != "" {
				fmt.Println("\nRelease Notes:")
				lines := strings.Split(rel.Body, "\n")
				for _, line := range lines {
					fmt.Println("  " + line)
				}
				fmt.Println()
			}

			if checkOnlyFlag {
				fmt.Println("Run 'waterstick update' to download and install this update.")
				return nil
			}

			fmt.Println("Starting update...")
			err = u.PerformSelfUpdate(ctx, version, func(pct int, msg string) {
				fmt.Printf("[%d%%] %s\n", pct, msg)
			})
			if err != nil {
				return err
			}

			fmt.Println("\nUpdate completed successfully! Run 'waterstick --version' or 'waterstick about' to verify.")
			return nil
		},
	}
	updateCmd.Flags().BoolVar(&checkOnlyFlag, "check", false, "Only check if an update is available without installing")

	// 16. completion
	completionCmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell|install]",
		Short: "Generate shell completion scripts or install them automatically",
		Long: `Generate shell autocompletion scripts for WaterStick or install them automatically.

To automatically configure completion for your current shell:
  waterstick completion install

Manual configuration:
  Bash:
    $ source <(waterstick completion bash)
  Zsh:
    $ source <(waterstick completion zsh)
  Fish:
    $ waterstick completion fish | source
  PowerShell:
    PS> waterstick completion powershell | Out-String | Invoke-Expression`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			switch args[0] {
			case "install":
				return installCompletion(rootCmd, "")
			case "bash":
				return rootCmd.GenBashCompletion(os.Stdout)
			case "zsh":
				return rootCmd.GenZshCompletion(os.Stdout)
			case "fish":
				return rootCmd.GenFishCompletion(os.Stdout, true)
			case "powershell":
				return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
			default:
				return fmt.Errorf("unknown shell %q. Supported shells: install, bash, zsh, fish, powershell", args[0])
			}
		},
	}

	installCmd := &cobra.Command{
		Use:   "install [bash|zsh|fish|powershell]",
		Short: "Automatically install shell autocompletion for your shell",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetShell := ""
			if len(args) > 0 {
				targetShell = args[0]
			}
			return installCompletion(rootCmd, targetShell)
		},
	}
	completionCmd.AddCommand(installCmd)

	rootCmd.AddCommand(connectCmd, disconnectCmd, devicesCmd, healthCmd, packagesCmd, freezeCmd, unfreezeCmd, debloatCmd, rollbackCmd, sideloadCmd, listAppsCmd, otaCmd, purgeCmd, tweakCmd, launcherCmd, tuiCmd, guiCmd, aboutCmd, updateCmd, completionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func resolveDevice(engine *waterstick.Engine, explicitDevice string) string {
	if explicitDevice != "" {
		return explicitDevice
	}
	devices, err := engine.ADB.ListDevices(context.Background())
	if err == nil && len(devices) > 0 {
		return devices[0]
	}
	return ""
}

func installCompletion(rootCmd *cobra.Command, requestedShell string) error {
	shellName := strings.ToLower(requestedShell)
	if shellName == "" {
		userShell := os.Getenv("SHELL")
		if userShell != "" {
			shellName = strings.ToLower(filepath.Base(userShell))
		} else if runtime.GOOS == "windows" {
			shellName = "powershell"
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("unable to determine user home directory: %w", err)
	}

	switch {
	case strings.Contains(shellName, "zsh"):
		// Determine ZDOTDIR:
		// 1. Explicit $ZDOTDIR
		// 2. ~/.config/zsh if it exists (standard HyDE and XDG zsh setups)
		zdotdir := os.Getenv("ZDOTDIR")
		if zdotdir == "" {
			cfgZsh := filepath.Join(homeDir, ".config", "zsh")
			if fi, err := os.Stat(cfgZsh); err == nil && fi.IsDir() {
				zdotdir = cfgZsh
			}
		}

		// Tier 1: HyDE's modular completions directory ($ZDOTDIR/completions/)
		if zdotdir != "" {
			completionsDir := filepath.Join(zdotdir, "completions")
			if fi, err := os.Stat(completionsDir); err == nil && fi.IsDir() {
				targetFile := filepath.Join(completionsDir, "waterstick.zsh")
				snippet := "# waterstick tab completion for HyDE\nif command -v waterstick &>/dev/null; then\n    compdef _waterstick waterstick\n    eval \"$(waterstick completion zsh)\"\nfi\n"
				_ = os.WriteFile(targetFile, []byte(snippet), 0644)

				// Also write native _waterstick for fpath autoloaders
				fpathTarget := filepath.Join(completionsDir, "_waterstick")
				if f, err := os.Create(fpathTarget); err == nil {
					_ = rootCmd.GenZshCompletion(f)
					f.Close()
				}
			}
		}

		// Tier 2: Oh-My-Zsh custom completions directory
		omzCustom := os.Getenv("ZSH_CUSTOM")
		if omzCustom == "" {
			omzCustom = filepath.Join(homeDir, ".oh-my-zsh", "custom")
		}
		omzCompDir := filepath.Join(omzCustom, "completions")
		if fi, err := os.Stat(omzCompDir); err == nil && fi.IsDir() {
			omzTarget := filepath.Join(omzCompDir, "_waterstick")
			if f, err := os.Create(omzTarget); err == nil {
				_ = rootCmd.GenZshCompletion(f)
				f.Close()
			}
		}

		// Tier 3: Active configuration file (HyDE $ZDOTDIR/.zshrc or standard ~/.zshrc)
		zshrc := filepath.Join(homeDir, ".zshrc")
		if zdotdir != "" {
			candidate := filepath.Join(zdotdir, ".zshrc")
			if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() {
				zshrc = candidate
			}
		}

		content, _ := os.ReadFile(zshrc)
		if strings.Contains(string(content), "waterstick completion zsh") {
			fmt.Printf("Shell autocompletion for zsh is already configured in %s.\n", zshrc)
			return nil
		}
		snippet := "\n# WaterStick shell completion\nif command -v waterstick >/dev/null 2>&1; then\n  eval \"$(waterstick completion zsh)\"\nfi\n"
		f, err := os.OpenFile(zshrc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", zshrc, err)
		}
		defer f.Close()
		if _, err := f.WriteString(snippet); err != nil {
			return fmt.Errorf("failed to append completion snippet: %w", err)
		}
		fmt.Printf("Shell autocompletion for zsh successfully installed in %s!\n", zshrc)
		fmt.Printf("Run 'source %s' or open a new terminal tab to activate it.\n", zshrc)
		return nil

	case strings.Contains(shellName, "bash"):
		xdgDir := filepath.Join(homeDir, ".local", "share", "bash-completion", "completions")
		if err := os.MkdirAll(xdgDir, 0755); err == nil {
			compFile := filepath.Join(xdgDir, "waterstick")
			f, err := os.Create(compFile)
			if err == nil {
				defer f.Close()
				if err := rootCmd.GenBashCompletion(f); err == nil {
					fmt.Printf("Shell autocompletion for bash successfully installed in %s!\n", compFile)
					fmt.Println("Autocompletion will be available in your next bash session.")
					return nil
				}
			}
		}
		bashrc := filepath.Join(homeDir, ".bashrc")
		content, _ := os.ReadFile(bashrc)
		if strings.Contains(string(content), "waterstick completion bash") {
			fmt.Printf("Shell autocompletion for bash is already configured in %s.\n", bashrc)
			return nil
		}
		snippet := "\n# WaterStick shell completion\nif command -v waterstick >/dev/null 2>&1; then\n  eval \"$(waterstick completion bash)\"\nfi\n"
		f, err := os.OpenFile(bashrc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", bashrc, err)
		}
		defer f.Close()
		if _, err := f.WriteString(snippet); err != nil {
			return fmt.Errorf("failed to append completion snippet: %w", err)
		}
		fmt.Printf("Shell autocompletion for bash successfully installed in %s!\n", bashrc)
		fmt.Println("Run 'source ~/.bashrc' or open a new terminal tab to activate it.")
		return nil

	case strings.Contains(shellName, "fish"):
		fishDir := filepath.Join(homeDir, ".config", "fish", "completions")
		if err := os.MkdirAll(fishDir, 0755); err != nil {
			return fmt.Errorf("failed to create fish directory: %w", err)
		}
		fishFile := filepath.Join(fishDir, "waterstick.fish")
		f, err := os.Create(fishFile)
		if err != nil {
			return fmt.Errorf("failed to create %s: %w", fishFile, err)
		}
		defer f.Close()
		if err := rootCmd.GenFishCompletion(f, true); err != nil {
			return fmt.Errorf("failed to generate fish completion: %w", err)
		}
		fmt.Printf("Shell autocompletion for fish successfully installed in %s!\n", fishFile)
		return nil

	case strings.Contains(shellName, "powershell") || runtime.GOOS == "windows":
		psDir := filepath.Join(homeDir, "Documents", "PowerShell")
		os.MkdirAll(psDir, 0755)
		profile := filepath.Join(psDir, "Microsoft.PowerShell_profile.ps1")
		content, _ := os.ReadFile(profile)
		if strings.Contains(string(content), "waterstick completion powershell") {
			fmt.Printf("Shell autocompletion for PowerShell is already configured in %s.\n", profile)
			return nil
		}
		snippet := "\n# WaterStick shell completion\nInvoke-Expression (&waterstick completion powershell | Out-String)\n"
		f, err := os.OpenFile(profile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to write to %s: %w", profile, err)
		}
		defer f.Close()
		if _, err := f.WriteString(snippet); err != nil {
			return fmt.Errorf("failed to append completion snippet: %w", err)
		}
		fmt.Printf("Shell autocompletion for PowerShell successfully installed in %s!\n", profile)
		return nil

	default:
		return fmt.Errorf("unsupported or undetected shell '%s'. Supported shells: zsh, bash, fish, powershell", shellName)
	}
}
