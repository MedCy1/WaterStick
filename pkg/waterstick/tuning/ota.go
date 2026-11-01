package tuning

import (
	"context"
	"fmt"
	"strings"

	"github.com/MedCy1/WaterStick/pkg/waterstick/adb"
)

// OTAManager controls background Fire OS update mechanisms and anti-rollback defenses.
type OTAManager struct {
	client *adb.Client
}

// NewOTAManager creates a new OTA shield controller.
func NewOTAManager(client *adb.Client) *OTAManager {
	return &OTAManager{client: client}
}

// IsOTABlocked verifies whether background Amazon OTA updates are frozen.
func (o *OTAManager) IsOTABlocked(ctx context.Context, deviceSerial string) (bool, error) {
	// 1. Check if forcedotaupdater is disabled
	disabledOut, err := o.client.Shell(ctx, deviceSerial, "pm list packages -d")
	if err != nil {
		return false, fmt.Errorf("failed listing disabled packages: %w", err)
	}
	fotaDisabled := strings.Contains(disabledOut, "package:com.amazon.tv.forcedotaupdater.v2")

	// 2. Check if AppOps background execution is ignored on OTA engine
	appopsOut, err := o.client.Shell(ctx, deviceSerial, "cmd appops get com.amazon.device.software.ota")
	if err != nil {
		return false, fmt.Errorf("failed querying OTA appops: %w", err)
	}
	bgIgnored := strings.Contains(appopsOut, "RUN_IN_BACKGROUND: ignore")

	return fotaDisabled && bgIgnored, nil
}

// BlockOTA enforces the OTA Shield: restricts background execution and disables forced update nag screens.
func (o *OTAManager) BlockOTA(ctx context.Context, deviceSerial string) error {
	// 1. Disable the OOBE forced update wizard
	_, _ = o.client.Shell(ctx, deviceSerial, "pm disable-user --user 0 com.amazon.tv.forcedotaupdater.v2")

	// 2. Restrict background wake & sync for core OTA daemon
	cmds := []string{
		"cmd appops set com.amazon.device.software.ota RUN_IN_BACKGROUND ignore",
		"cmd appops set com.amazon.device.software.ota RUN_ANY_IN_BACKGROUND ignore",
		"cmd appops set com.amazon.device.software.ota.override RUN_IN_BACKGROUND ignore",
		"cmd appops set com.amazon.device.software.ota.override RUN_ANY_IN_BACKGROUND ignore",
	}

	for _, cmd := range cmds {
		if _, err := o.client.Shell(ctx, deviceSerial, cmd); err != nil {
			return fmt.Errorf("failed executing %s: %w", cmd, err)
		}
	}

	return nil
}

// UnblockOTA restores factory defaults for Amazon OTA services.
func (o *OTAManager) UnblockOTA(ctx context.Context, deviceSerial string) error {
	// 1. Re-enable forced updater
	_, _ = o.client.Shell(ctx, deviceSerial, "pm enable com.amazon.tv.forcedotaupdater.v2")

	// 2. Allow background execution
	cmds := []string{
		"cmd appops set com.amazon.device.software.ota RUN_IN_BACKGROUND allow",
		"cmd appops set com.amazon.device.software.ota RUN_ANY_IN_BACKGROUND allow",
		"cmd appops set com.amazon.device.software.ota.override RUN_IN_BACKGROUND allow",
		"cmd appops set com.amazon.device.software.ota.override RUN_ANY_IN_BACKGROUND allow",
	}

	for _, cmd := range cmds {
		if _, err := o.client.Shell(ctx, deviceSerial, cmd); err != nil {
			return fmt.Errorf("failed executing %s: %w", cmd, err)
		}
	}

	return nil
}
