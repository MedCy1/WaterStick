# Development walkthrough and hardware verification

This document records the empirical validation of WaterStick against a physical retail Fire TV Stick 4K Max 2nd Gen.

## Device environment and baseline metrics

* Model identifier: `AFTKRT` (Board codename `karat`)
* Operating system: Fire OS 8.1.8.5 (Android 11 base, API level 30)
* Package count: 307 packages installed, 181 Amazon system components analyzed
* Verified bloatware targets:
  * All 20 target packages specified in the balanced profile were identified running on the target hardware (including `ceviche`, `acr`, `adep`, `kso.blackbird`, `gamehub`, `minitv`, `shoptv`, `wirelessmetrics`).
  * Alexa voice services (`aria`, `ale`, `vizzini`, `aca`) were confirmed operational under the voice protection tier.
* Memory baseline: 1,669 MB total physical memory, 355 to 484 MB free, with 293 MB physical RAM utilized by ZRAM swap.
* Installed application detection: SmartTube (`org.smarttube.stable`) was detected and identified as already present on user 0.

## Implementation details validated on hardware

1. Pre-flight package verification: `ApplyProfile` queries `pm list packages --user 0` before executing commands, skipping packages that are already absent without generating error warnings.
2. Memory delta reporting: Measures physical RAM before and after debloat operations to report the exact reclaimed memory in megabytes.
3. System rollback (`RollbackAll`): Queries `pm list packages -d`, re-enables each disabled package, restores video and audio preview settings, resets private DNS, sets animation scales back to 1.0x, and restores default AppOps background execution policies.
4. Interface telemetry updates: The TUI and GUI update diagnostic readings following profile application or rollback execution.

## Supported interfaces

* CLI (`cmd/waterstick`): Built with Cobra for direct terminal execution and scripting.
* TUI (`cmd/waterstick-tui`): Interactive terminal dashboard built with Bubble Tea v2 and Lipgloss v2.
* GUI (`cmd/waterstick-gui`): Desktop interface built with Wails v2.
