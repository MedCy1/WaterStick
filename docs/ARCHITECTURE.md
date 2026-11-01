# Architecture and contributor guide

This document outlines the internal structure of WaterStick, its core design patterns, and its safety guarantees.

## High-level design

WaterStick uses a ports-and-adapters architecture. The core logic in `pkg/waterstick` operates independently of any user interface.

```
                             ┌────────────────────────┐
                             │  cmd/waterstick (CLI)  │
                             │      spf13/cobra       │
                             └───────────┬────────────┘
                                         │
┌───────────────────────────┐            │            ┌──────────────────────────┐
│ cmd/waterstick-tui (TUI)  │────────────┼────────────│ cmd/waterstick-gui (GUI) │
│ charm.land/bubbletea/v2   │            │            │     wailsapp/wails/v2    │
└───────────────────────────┘            │            └──────────────────────────┘
                                         ▼
                    ┌──────────────────────────────────────────┐
                    │        pkg/waterstick (Core Engine)       │
                    ├──────────────────────────────────────────┤
                    │  • adb: Transport & Auto-Installer       │
                    │  • diagnostics: Hardware & OS Inspector  │
                    │  • packages: Safe Manager & Whitelist    │
                    │  • tuning: Settings & AppOps Enforcer    │
                    │  • backup: Snapshot & Rollback Engine    │
                    │  • sideload: Verified TV Apps Catalog    │
                    │  • models: Shared Domain Types           │
                    └──────────────────────────────────────────┘
                                         │
                                         ▼
                             Fire TV Stick 4K Max (ADB)
```

## Core packages (`pkg/waterstick/`)

### adb (`adb.go`)
Wraps the Android Debug Bridge executable.
If `adb` is missing from system `$PATH`, `EnsureADB()` downloads official Google platform-tools for Linux, macOS, or Windows into the user cache directory (`~/.cache/waterstick/bin` or `%LOCALAPPDATA%\waterstick\bin`) and sets executable permissions.

Primary methods: `Connect`, `Disconnect`, `ListDevices`, `Shell`, `InstallAPK`.

### diagnostics (`diagnostics.go`)
Reads hardware properties and system state over ADB:
* `ro.product.model` (`AFTKM` or `karaf`) identifies the 4K Max generation.
* `ro.build.version.release` confirms the Android 11 base (Fire OS 8).
* Partition capacity via `df -h /data` and memory metrics via `dumpsys meminfo`.
* Current launcher via `pm resolve-activity -a android.intent.action.MAIN -c android.intent.category.HOME`.

### packages (`manager.go`)
Handles package operations and enforces anti-brick constraints.
* Embeds `packages.json` containing package descriptions, risk levels, and categories.
* Hard-locked check: Rejects any attempt to disable packages in `TierHardLocked` (`com.amazon.tv.ime`, `com.amazon.tv.settings.v2`, `com.amazon.tv.launcher`, `whisperplay`).
* Voice check: Preserves `TierVoice` packages unless explicitly bypassed with the `forceVoice` parameter.
* Disables targets through `pm disable-user --user 0 <pkg>` to leave system partitions untouched.

### tuning (`tuning.go`)
Applies display, audio, and privacy configurations:
* Disables video preview autoplay via `com.amazon.tv.settings.featured_item_video_enabled 0`.
* Sets private DNS over TLS to AdGuard, Quad9, or Cloudflare via `Settings.Global private_dns_mode`.
* Enables 24 fps cinema frame rate matching via `match_content_frame_rate 1`.
* Restricts background memory usage using `cmd appops set <pkg> RUN_IN_BACKGROUND ignore`.
* Flushes temporary storage with `pm trim-caches 2G`.

### backup (`backup.go`)
Creates state snapshots and handles full restorations. Snapshots are stored as JSON files in `~/.config/waterstick/backups/`.
The `RollbackAll` function inspects `pm list packages -d`, re-enables all disabled packages, and resets system settings to factory values.

### sideload (`sideload.go`)
Maintains a curated catalog of tested TV applications (SmartTube, Projectivy Launcher, Stremio, Kodi, VLC). Downloads official release APKs to temporary storage and runs `adb install -r -d`.

## User interfaces

### Command-line interface (`cmd/waterstick`)
Built with `github.com/spf13/cobra`. Provides scriptable subcommands for device connection, diagnostics, debloating, hardware tweaks, package operations, and app installations.

### Terminal UI (`cmd/waterstick-tui`)
Built with `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2`. Implements the Elm architecture (`Init`, `Update`, `View`) with tab navigation, real-time status bars, and mouse support.

### Desktop GUI (`cmd/waterstick-gui`)
Built with `github.com/wailsapp/wails/v2`. Exposes Go methods to an embedded web frontend through JavaScript bindings, packaging the entire application into a single executable without external browser dependencies.
