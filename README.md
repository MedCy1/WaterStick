# WaterStick

Debloat utility, hardware optimizer, and sideload manager for the Amazon Fire TV Stick 4K Max (2nd Gen, Fire OS 8). Written in Go with CLI, TUI, and GUI frontends sharing a single engine.

## Overview

Fire OS 8 introduces changes that break older debloat scripts, including an updated content observer for home screen previews, strict background app controls, and new privacy options. WaterStick targets these specific mechanisms through ADB without requiring root access.

| Feature | Default behavior | With WaterStick | Details |
| :--- | :--- | :--- | :--- |
| Remote voice search | Active | Active and protected | Preserves Alexa voice input and microphone remote buttons |
| Home screen video previews | Autoplays video banners with audio | Disabled | Silences previews and frees cache storage |
| Network ad blocking | Standard Amazon DNS | Private DNS over TLS (DoT) | Resolves via AdGuard, Quad9, or Cloudflare |
| Viewing surveillance | ACR background scanner active | Disabled | Stops continuous screen reporting (`com.amazon.tv.acr`) |
| Frame rate matching | Fixed 60 Hz | Native 24 fps cinema cadence | Matches source frame rate in Prime Video, Kodi, and SmartTube |
| Animation speed | 1.0x scale | 0.5x scale | Reduces transition latency across the interface |
| Background memory | Streaming apps remain cached | Restricted via AppOps | Reclaims memory when exiting heavy streaming apps |
| Stock launcher | Default | Preserved | Prevents black screen issues common on Fire OS 8 |

## Interfaces

WaterStick provides three interfaces built on the same core packages:

* **Desktop GUI (`cmd/waterstick-gui`)**: Desktop application built with Wails v2. Includes storage and RAM gauges, package management lists, and one-click actions.
* **Terminal UI (`cmd/waterstick-tui`)**: Interactive terminal dashboard built with Bubble Tea v2 and Lipgloss v2. Supports keyboard navigation and mouse selection.
* **Command-line interface (`cmd/waterstick`)**: Headless tool built with Cobra for automation and terminal workflows.

## Quick start

### 1. Enable ADB on the Fire TV Stick

1. Open **Settings > My Fire TV > About**.
2. Select the device name and press the remote Select button 7 times to reveal Developer Options.
3. Open **Developer Options > ADB Debugging** and toggle it **ON**.
4. Note the device IP address shown in **Settings > My Fire TV > About > Network**.

### 2. Installation options

#### Windows graphic setup
Download `waterstick-setup-<version>-windows-amd64.exe` from the latest GitHub Release, run the setup wizard, and launch WaterStick directly from the Desktop or Start Menu.

#### Linux and macOS script installer
```bash
curl -fsSL https://raw.githubusercontent.com/MedCy1/WaterStick/main/install.sh | bash
```

#### Windows PowerShell script installer
```powershell
irm https://raw.githubusercontent.com/MedCy1/WaterStick/main/install.ps1 | iex
```

#### Standalone portable archives
Download the matching archive for your operating system (`.tar.gz` for Linux and macOS, `.zip` for Windows) from GitHub Releases and run `waterstick` directly without installing.

#### Building from source
```bash
git clone https://github.com/MedCy1/WaterStick.git
cd WaterStick
make build
```

#### Automatic updates
* In the desktop GUI: An update banner appears when a new version is available; click "Installer" to update.
* In the CLI: Run `waterstick update` to download and apply the latest release in-place, or `waterstick update --check` to inspect available updates.

### 3. Usage examples

If `adb` is not installed on your system, WaterStick downloads Google platform-tools into your user cache directory automatically.

## Package safety model

WaterStick disables packages in user space using `pm disable-user --user 0 <pkg>` instead of removing APKs from `/system`. System binaries remain intact on disk, allowing every modification to be reverted.

### Package tiers

* **Tier 0 (Hard-Locked)**: Critical system components that cannot be disabled through any profile:
  * `com.amazon.tv.ime` (Virtual keyboard)
  * `com.amazon.tv.settings.v2` (System settings)
  * `com.amazon.tv.launcher` (Stock launcher)
  * `com.amazon.whisperplay.contracts` and companion services (Remote pairing and screen casting)
* **Tier Voice (Protected by default)**: Alexa voice processing services:
  * `com.amazon.avl.ftv` (Alexa voice layer)
  * `com.amazon.aria` (Aria runtime)
  * `com.amazon.vizzini` (Voice audio support)
  * `com.amazon.ale` (Language engine)
* **Tier Safe**: Telemetry, ad networks, and unused background daemons:
  * `com.amazon.tv.acr` (Automated content recognition)
  * `com.amazon.hybridadidservice` (Advertising ID tracking)
  * `com.amazon.dp.logger`, `com.amazon.device.metrics`, `com.amazon.tv.fw.metrics` (Telemetry loggers)
  * `com.amazon.recess` (Casual games hub)
  * `com.amazon.ceviche` (Live TV aggregator)
  * `com.amazon.ftv.screensaver` (Ad wallpaper fetcher)

## Reverting changes

To restore default system settings and re-enable all packages:

```bash
waterstick rollback
```

This command re-enables all disabled packages, restores video and audio preview settings, resets private DNS, and sets animation scales back to 1.0x.

## Sideload catalog and hardware tweaks

The built-in installer downloads and verifies release APKs for common TV applications:

* SmartTube: YouTube client with SponsorBlock and video ad blocking.
* Projectivy Launcher: Lightweight TV launcher.
* Stremio: Streaming media organizer.
* Kodi: Home theater media center.
* VLC: Media player for local files and network streams.

```bash
# Manage applications
waterstick apps
waterstick install smarttube
waterstick install stremio kodi vlc

# Inspect packages live
waterstick packages --disabled
waterstick packages --active
waterstick freeze com.amazon.tv.acr
waterstick unfreeze com.amazon.tv.acr

# Hardware tweaks
waterstick tweak status
waterstick tweak audio --auto
waterstick tweak audio --optical-51
waterstick tweak display --match-framerate on
waterstick tweak power --ambient off
waterstick tweak dns adguard

# Shell autocompletion (auto-detects Zsh, Bash, Fish, PowerShell)
waterstick completion install
```

## Testing

```bash
make test
```

## Documentation

* [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): Core engine structure, interfaces, and safety guarantees.
* [docs/REVERSE_ENGINEERING.md](docs/REVERSE_ENGINEERING.md): Reverse engineering analysis of Fire OS 8 system APKs, settings keys, and telemetry daemons.
* [docs/FEATURES_ROADMAP.md](docs/FEATURES_ROADMAP.md): Technical feature roadmap derived from APK decompilation.
* [docs/WALKTHROUGH.md](docs/WALKTHROUGH.md): Verification notes and metrics collected on physical hardware.

## Author

Developed by [MedCy1](https://github.com/MedCy1).

## License

PolyForm Noncommercial License 1.0.0. Free for personal, non-commercial, and community use. Commercial use, monetization, and closed-source redistribution are strictly prohibited.
