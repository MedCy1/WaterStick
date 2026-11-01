# Features roadmap and settings inventory

This document catalogs hardware and system properties discovered through decompiling Fire OS 8 system APKs on the Fire TV Stick 4K Max 2nd Gen (`karat`). It outlines configuration options and their implementation status across the CLI, TUI, and GUI.

## Display and video output

### Auto Low Latency Mode (ALLM)
* Property: `persist.sys.hdmi.ll.mode` (`auto`, `enable`, `disable`), referenced in `LowLatencyModeLayout.java`.
* CLI: `waterstick tweak allm --mode auto|enable|disable`
* Application: Switches HDMI 2.1 displays into low-latency gaming mode for cloud gaming (GeForce NOW, Xbox Cloud Gaming) and local game streaming (Moonlight).

### Super resolution upscaling
* Property: `persist.sys.open.sr.mode` (`enable`, `disable`), referenced in `SuperResolutionModeLayout.java`.
* CLI: `waterstick tweak display --super-resolution on|off`
* Application: Toggles the MediaTek MT8696T hardware video upscaling filter on 1080p video streams displayed on 4K panels.

### Dynamic range matching
* Property: `persist.sys.hdmi.hdr.enable` (`adaptive`, `always_hdr`, `always_sdr`), managed by `ResolutionManager.java`.
* CLI: `waterstick tweak display --hdr adaptive`
* Application: Prevents the device from forcing permanent HDR output, preserving correct color grading for standard dynamic range content.

### Color depth and pixel format
* Properties: `persist.sys.open.deepcolor` (`8`, `10`, `12` bit) and `persist.sys.open.pixfmt` (`RGB`, `YCbCr422`, `YCbCr420`), exposed in `ResolutionManager.java`.
* Application: Configures 10-bit or 12-bit Rec.2020 output to reduce gradient banding on OLED and Mini-LED displays.

## Audio and HDMI power management

### HDMI audio passthrough
* Property: `persist.sys.hdmi.passthrough` (`enabled`, `disabled`), toggled in `HdmiInputPassThroughModeLayout.java`.
* CLI: `waterstick tweak audio --passthrough on|off`
* Application: Passes raw bitstream audio (Dolby Atmos, DTS:X, TrueHD) directly to AV receivers and soundbars without downmixing.

### HDMI sleep behavior
* Property: `persist.sys.hdmi.sleep_mode` (`off` for no signal, `on` for black screen), read by `SleepModeBehaviourLayout.java`.
* CLI: `waterstick tweak power --sleep-mode no-signal`
* Application: Cuts the HDMI output clock on standby instead of sending a black video frame, allowing connected monitors and TVs to enter low-power standby via CEC.

### Ambient experience background task
* Setting: `Settings.Secure.ambient_experience_enabled` (`0`, `1`), verified in `AmbientModeLayout.java`.
* CLI: `waterstick tweak power --ambient off`
* Application: Stops background artwork pre-rendering and photo rendering during standby.

## System update controls

### OTA update shield
* Components:
  * Update service: `com.amazon.device.software.ota`
  * Standby wake trigger: `com.amazon.device.software.ota.override`
  * Setup updater: `com.amazon.tv.forcedotaupdater.v2`
  * Remote pairing daemons: `com.amazon.device.bluetoothpa` and `com.amazon.vizzini`
* CLI: `waterstick ota block` and `waterstick ota unblock`
* Application: Disables background firmware downloads while keeping remote pairing and voice functionality active.

## Telemetry and privacy

### Privacy controls
* Keys managed via `SettingHandlerFactory.java`:
  * `usage_metrics_marketing_enabled = 0`
  * `USAGE_METRICS_UPLOAD_ENABLED = 0`
  * `limit_ad_tracking = 1`
  * `DATA_MONITORING_ENABLED = 0`
  * `cross_device_recents_sync_enabled = 0`
* Application: Cuts telemetry event submission and sync operations across Amazon backends.

### Encrypted DNS resolvers
* Setting: `Settings.Global.private_dns_mode hostname` with `private_dns_specifier`.
* Presets:
  * AdGuard: `dns.adguard.com` (Blocks ad domains and tracking hosts)
  * Quad9: `dns.quad9.net` (Malware blocking)
  * Cloudflare: `one.one.one.one` (Low-latency standard resolution)

## Maintenance and system health

### Cache and memory reclamation
* Mechanism: Calls `pm trim-caches 2G` and restarts `com.amazon.tv.launcher` to force `VideoPreviewEnabledObserver.java` to clear its internal preview disk cache (`cachedVideoClipUrls`).
* CLI: `waterstick ram trim`
* Application: Releases orphaned video preview files and frees temporary storage in `/data/cache`.

### Thermal monitoring
* Interface: Linux kernel thermal sensors under `/sys/class/thermal/` and CPU frequency nodes under `/sys/devices/system/cpu/cpu*/cpufreq/`.
* CLI: `waterstick health`
* Application: Reports real-time SoC temperature, Cortex-A55 core frequencies, and memory utilization without requiring root privileges.

## Feature matrix

| Feature | Complexity | Risk | Status |
| :--- | :--- | :--- | :--- |
| Preview autoplay suppression | Moderate | Low | Implemented |
| Privacy and telemetry controls | Low | Low | Implemented |
| OTA update shield | Low | Low | Implemented |
| Frame rate matching | Low | Low | Implemented |
| HDMI sleep mode configuration | Low | Low | Implemented |
| Private DNS over TLS | Low | Low | Implemented |
| Cache and memory trimmer | Low | Low | Implemented |
| Thermal and hardware diagnostics | Moderate | Low | Implemented |
