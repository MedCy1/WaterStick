package diagnostics

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MedCy1/WaterStick/pkg/waterstick/adb"
	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
)

// Inspector collects diagnostic telemetry from a connected Fire TV.
type Inspector struct {
	client *adb.Client
}

// NewInspector creates a new system inspector.
func NewInspector(client *adb.Client) *Inspector {
	return &Inspector{client: client}
}

// Inspect collects full system metrics and hardware identifiers.
func (i *Inspector) Inspect(ctx context.Context, deviceSerial string) (*models.Device, error) {
	dev := &models.Device{
		Serial:      deviceSerial,
		ConnectedAt: time.Now(),
	}

	// 1. Device Hardware & OS Build Properties
	props, err := i.client.Shell(ctx, deviceSerial, "getprop")
	if err == nil {
		dev.Model = extractProp(props, "ro.product.model")
		dev.AndroidVersion = extractProp(props, "ro.build.version.release")
		dev.BuildVersionName = extractProp(props, "ro.build.version.name")
		productDevice := extractProp(props, "ro.product.device")

		// Identify Fire OS 8 and Fire TV Stick 4K Max 2nd Gen
		if strings.EqualFold(dev.Model, "AFTKM") || strings.EqualFold(dev.Model, "AFTKRT") || strings.Contains(strings.ToLower(productDevice), "karat") || strings.Contains(strings.ToLower(productDevice), "karaf") {
			dev.IsFireTV4KMax2Gen = true
			dev.ModelName = "Fire TV Stick 4K / 4K Max (2nd Gen)"
		} else if dev.Model != "" {
			dev.ModelName = dev.Model
		} else {
			dev.ModelName = "Generic Fire TV"
		}

		if strings.HasPrefix(dev.BuildVersionName, "Fire OS 8") || dev.AndroidVersion == "11" {
			dev.FireOSVersion = "Fire OS 8 (Android 11)"
		} else if dev.BuildVersionName != "" {
			dev.FireOSVersion = dev.BuildVersionName
		} else {
			dev.FireOSVersion = "Fire OS (Android " + dev.AndroidVersion + ")"
		}
	}

	// 2. Storage metrics (/data partition)
	dfOut, err := i.client.Shell(ctx, deviceSerial, "df -h /data")
	if err == nil {
		lines := strings.Split(strings.TrimSpace(dfOut), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[len(lines)-1])
			if len(fields) >= 5 {
				dev.TotalStorage = fields[1]
				dev.FreeStorage = fields[3]
				dev.UsedStoragePct = fields[4]
			}
		}
	}

	// 3. RAM metrics
	memOut, err := i.client.Shell(ctx, deviceSerial, "dumpsys meminfo | grep -E 'Total RAM:|Free RAM:|ZRAM:'")
	if err == nil {
		dev.TotalRAMMB = parseMemMB(memOut, `Total RAM:\s+([\d,]+)\s*[kK]B?`)
		dev.FreeRAMMB = parseMemMB(memOut, `Free RAM:\s+([\d,]+)\s*[kK]B?`)
		dev.ZRAMUsedMB = parseMemMB(memOut, `ZRAM:\s+([\d,]+)\s*[kK]B?`)
	}

	// 4. Active Home Launcher
	launcherOut, err := i.client.Shell(ctx, deviceSerial, "pm resolve-activity -a android.intent.action.MAIN -c android.intent.category.HOME")
	if err == nil {
		dev.ActiveLauncher = extractPackageName(launcherOut)
	}

	// 5. Count disabled packages
	disabledOut, err := i.client.Shell(ctx, deviceSerial, "pm list packages -d")
	if err == nil {
		lines := strings.Split(strings.TrimSpace(disabledOut), "\n")
		count := 0
		for _, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "package:") {
				count++
			}
		}
		dev.DisabledCount = count
	}

	// 6. Thermal metrics (dumpsys thermalservice)
	thermalOut, err := i.client.Shell(ctx, deviceSerial, "dumpsys thermalservice")
	if err == nil {
		dev.CPUTempC = parseTemperature(thermalOut, `Temperature\{mValue=([\d\.]+)[^\}]*mName=CPU`)
		dev.GPUTempC = parseTemperature(thermalOut, `Temperature\{mValue=([\d\.]+)[^\}]*mName=GPU`)
		if m := regexp.MustCompile(`Thermal Status:\s*(\d+)`).FindStringSubmatch(thermalOut); len(m) > 1 {
			dev.ThermalStatus, _ = strconv.Atoi(m[1])
		}
	}

	// 7. CPU Frequencies & Load
	freqOut, err := i.client.Shell(ctx, deviceSerial, "cat /sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq")
	if err == nil {
		var freqs []int
		for _, line := range strings.Split(strings.TrimSpace(freqOut), "\n") {
			line = strings.TrimSpace(line)
			if val, parseErr := strconv.Atoi(line); parseErr == nil && val > 0 {
				freqs = append(freqs, val/1000)
			}
		}
		dev.CPUFreqMHz = freqs
	}

	cpuInfoOut, err := i.client.Shell(ctx, deviceSerial, "dumpsys cpuinfo | grep 'Load:'")
	if err == nil {
		if m := regexp.MustCompile(`Load:\s*([^\r\n]+)`).FindStringSubmatch(cpuInfoOut); len(m) > 1 {
			dev.CPULoad = strings.TrimSpace(m[1])
		}
	}

	// 8. OTA Shield Status
	fotaDisabled := strings.Contains(disabledOut, "package:com.amazon.tv.forcedotaupdater.v2")
	appopsOut, _ := i.client.Shell(ctx, deviceSerial, "cmd appops get com.amazon.device.software.ota")
	otaAppOpsIgnored := strings.Contains(appopsOut, "RUN_IN_BACKGROUND: ignore")
	dev.OTABlocked = fotaDisabled && otaAppOpsIgnored

	return dev, nil
}

func parseTemperature(output, regexStr string) float64 {
	re := regexp.MustCompile(regexStr)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
			return val
		}
	}
	return 0.0
}

func extractProp(getpropOutput, propName string) string {
	pattern := regexp.MustCompile(`\[` + regexp.QuoteMeta(propName) + `\]:\s*\[(.*?)\]`)
	matches := pattern.FindStringSubmatch(getpropOutput)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func parseMemMB(meminfo, regexStr string) int {
	re := regexp.MustCompile(regexStr)
	matches := re.FindStringSubmatch(meminfo)
	if len(matches) > 1 {
		clean := strings.ReplaceAll(matches[1], ",", "")
		if kb, err := strconv.Atoi(clean); err == nil {
			return kb / 1024
		}
	}
	return 0
}

func extractPackageName(output string) string {
	re := regexp.MustCompile(`packageName=([a-zA-Z0-9_\.]+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
