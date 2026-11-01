package tuning

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/MedCy1/WaterStick/pkg/waterstick/adb"
	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
)

// Tuner configures Android and Fire OS system-level performance & privacy tweaks.
type Tuner struct {
	client *adb.Client
}

// NewTuner creates a new system tuner.
func NewTuner(client *adb.Client) *Tuner {
	return &Tuner{client: client}
}

// DefaultTuningSettings returns the balanced optimization tweaks for Fire OS 8.
func DefaultTuningSettings(dnsProvider string) []models.SystemSetting {
	if dnsProvider == "" {
		dnsProvider = "dns.adguard.com"
	}

	return []models.SystemSetting{
		{
			Namespace:   "global",
			Key:         "com.amazon.tv.settings.featured_item_video_enabled",
			Value:       "0",
			Description: "Fire OS 8 native setting: disable featured video autoplay",
		},
		{
			Namespace:   "global",
			Key:         "com.amazon.tv.settings.featured_item_video_volume",
			Value:       "0.0",
			Description: "Fire OS 8 native setting: mute featured video audio",
		},
		{
			Namespace:   "secure",
			Key:         "pvde_playback_api_enabled_for_launcher",
			Value:       "0",
			Description: "Fire OS 8 PVDE direct banner video playback engine",
		},
		{
			Namespace:   "secure",
			Key:         "FTV_LAUNCHER_FR_AUTOPLAY_717667",
			Value:       "C",
			Description: "Fire OS 8 launcher featured rotator autoplay gating experiment (C=Control/Disabled)",
		},
		{
			Namespace:   "secure",
			Key:         "FTV_LAUNCHER_FR_AUTOPLAY_EXPERIMENT_723780",
			Value:       "C",
			Description: "Fire OS 8 launcher featured rotator autoplay secondary experiment (C=Control/Disabled)",
		},
		{
			Namespace:   "global",
			Key:         "auto_play_video",
			Value:       "0",
			Description: "Legacy Android setting: disable video autoplay",
		},
		{
			Namespace:   "global",
			Key:         "auto_play_audio",
			Value:       "0",
			Description: "Legacy Android setting: disable audio autoplay",
		},
		{
			Namespace:   "secure",
			Key:         "limit_ad_tracking",
			Value:       "1",
			Description: "Limit personalized advertising tracking",
		},
		{
			Namespace:   "secure",
			Key:         "amz_limit_ad_tracking",
			Value:       "1",
			Description: "Amazon internal ad tracking limiter",
		},
		{
			Namespace:   "secure",
			Key:         "non_amz_limit_ad_tracking",
			Value:       "1",
			Description: "Non-Amazon ad tracking limiter",
		},
		{
			Namespace:   "secure",
			Key:         "interest_based_ads",
			Value:       "0",
			Description: "Opt out of interest-based ad profiles",
		},
		{
			Namespace:   "secure",
			Key:         "USAGE_METRICS_UPLOAD_ENABLED",
			Value:       "0",
			Description: "Disable Amazon usage metrics telemetry upload",
		},
		{
			Namespace:   "secure",
			Key:         "usage_metrics_marketing_enabled",
			Value:       "0",
			Description: "Disable marketing usage metrics telemetry",
		},
		{
			Namespace:   "global",
			Key:         "DATA_MONITORING_ENABLED",
			Value:       "0",
			Description: "Disable Amazon background data monitoring and network metrics profiling",
		},
		{
			Namespace:   "secure",
			Key:         "cross_device_recents_sync_enabled",
			Value:       "0",
			Description: "Disable cross-device recent app & video sync with Firebat",
		},
		{
			Namespace:   "global",
			Key:         "private_dns_mode",
			Value:       "hostname",
			Description: "Enable native DNS-over-TLS (DoT)",
		},
		{
			Namespace:   "global",
			Key:         "private_dns_specifier",
			Value:       dnsProvider,
			Description: "Encrypted ad-blocking Private DNS resolver",
		},
		{
			Namespace:   "global",
			Key:         "match_content_frame_rate",
			Value:       "1",
			Description: "Match original 24 fps cinema frame rate without judder",
		},
		{
			Namespace:   "global",
			Key:         "window_animation_scale",
			Value:       "0.5",
			Description: "Accelerate window animations (2x faster)",
		},
		{
			Namespace:   "global",
			Key:         "transition_animation_scale",
			Value:       "0.5",
			Description: "Accelerate transition animations (2x faster)",
		},
		{
			Namespace:   "global",
			Key:         "animator_duration_scale",
			Value:       "0.5",
			Description: "Accelerate animator duration (2x faster)",
		},
	}
}

// DefaultAppOps returns background restrictions for resource-heavy streaming applications.
func DefaultAppOps() []models.AppOp {
	return []models.AppOp{
		{Package: "com.netflix.ninja", Op: "RUN_IN_BACKGROUND", Mode: "ignore"},
		{Package: "in.startv.hotstar", Op: "RUN_IN_BACKGROUND", Mode: "ignore"},
		{Package: "com.amazon.device.crashmanager", Op: "RUN_IN_BACKGROUND", Mode: "ignore"},
		{Package: "com.crunchyroll.crunchyroid", Op: "RUN_IN_BACKGROUND", Mode: "ignore"},
	}
}

// ApplySetting writes a single Android system/global/secure setting.
func (t *Tuner) ApplySetting(ctx context.Context, deviceSerial string, s models.SystemSetting) error {
	cmd := fmt.Sprintf("settings put %s %s %s", s.Namespace, s.Key, s.Value)
	_, err := t.client.Shell(ctx, deviceSerial, cmd)
	return err
}

// GetSetting reads a single Android system/global/secure setting.
func (t *Tuner) GetSetting(ctx context.Context, deviceSerial, namespace, key string) (string, error) {
	cmd := fmt.Sprintf("settings get %s %s", namespace, key)
	out, err := t.client.Shell(ctx, deviceSerial, cmd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// ApplyTuning applies a complete set of system tweaks.
func (t *Tuner) ApplyTuning(ctx context.Context, deviceSerial string, settings []models.SystemSetting) error {
	for _, s := range settings {
		if err := t.ApplySetting(ctx, deviceSerial, s); err != nil {
			return fmt.Errorf("failed setting %s/%s: %w", s.Namespace, s.Key, err)
		}
	}
	// Restart launcher so settings take effect immediately
	_, _ = t.client.Shell(ctx, deviceSerial, "am force-stop com.amazon.tv.launcher")
	return nil
}

// ApplyAppOps enforces background runtime execution constraints on heavy streaming apps.
func (t *Tuner) ApplyAppOps(ctx context.Context, deviceSerial string, ops []models.AppOp) error {
	for _, op := range ops {
		cmd := fmt.Sprintf("cmd appops set %s %s %s", op.Package, op.Op, op.Mode)
		// Ignore errors if app is not installed
		_, _ = t.client.Shell(ctx, deviceSerial, cmd)
	}
	return nil
}

// TrimCaches requests Android package manager to reclaim stale cached image/video buffers.
func (t *Tuner) TrimCaches(ctx context.Context, deviceSerial string) error {
	_, err := t.client.Shell(ctx, deviceSerial, "pm trim-caches 2G")
	return err
}

// PurgeMemory triggers a thorough cache reclamation and launcher cycle to recover orphaned video buffers.
func (t *Tuner) PurgeMemory(ctx context.Context, deviceSerial string) (beforeMB int, afterMB int, freedMB int, err error) {
	parseFree := func() int {
		out, _ := t.client.Shell(ctx, deviceSerial, "dumpsys meminfo | grep 'Free RAM:'")
		for _, part := range strings.Split(out, "\n") {
			if strings.Contains(part, "Free RAM:") {
				fields := strings.Fields(part)
				if len(fields) >= 3 {
					raw := strings.ReplaceAll(fields[2], ",", "")
					raw = strings.TrimSuffix(strings.TrimSuffix(raw, "K"), "kB")
					if val, parseErr := strconv.Atoi(raw); parseErr == nil {
						return val / 1024
					}
				}
			}
		}
		return 0
	}

	beforeMB = parseFree()

	// 1. Reclaim filesystem buffer caches
	_, _ = t.client.Shell(ctx, deviceSerial, "pm trim-caches 2G")

	// 2. Cycle launcher to trigger VideoPreviewEnabledObserver prune
	_, _ = t.client.Shell(ctx, deviceSerial, "am force-stop com.amazon.tv.launcher")

	afterMB = parseFree()
	freedMB = afterMB - beforeMB
	if freedMB < 0 {
		freedMB = 0
	}

	return beforeMB, afterMB, freedMB, nil
}

