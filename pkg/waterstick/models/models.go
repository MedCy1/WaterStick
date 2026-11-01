package models

import "time"

// PackageTier defines the safety level and policy applied to a package.
type PackageTier string

const (
	// TierHardLocked packages can NEVER be disabled (ime, settings, launcher, connectivity core).
	TierHardLocked PackageTier = "tier_0_hardlocked"
	// TierVoice packages control Alexa voice recognition and remote microphone search.
	TierVoice PackageTier = "tier_voice"
	// TierSafe packages are safe to disable across all profiles (ad-trackers, telemetry, bloat).
	TierSafe PackageTier = "tier_safe"
	// TierConsumer packages are Amazon consumer apps (Prime Video, Music, Photos, Kindle).
	TierConsumer PackageTier = "tier_consumer"
)

// Package represents a single Android/Fire OS package definition.
type Package struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Description    string      `json:"description"`
	Tier           PackageTier `json:"tier"`
	Category       string      `json:"category"`
	Risk           string      `json:"risk"`
	Recommendation string      `json:"recommendation"`
	Installed      bool        `json:"installed,omitempty"`
	Disabled       bool        `json:"disabled,omitempty"`
}

// PackageDatabase represents the root structure of data/packages.json.
type PackageDatabase struct {
	Version       string    `json:"version"`
	DeviceTargets []string  `json:"device_targets"`
	Packages      []Package `json:"packages"`
}

// Device represents the hardware and OS inspection profile of a connected Fire TV.
type Device struct {
	Serial            string    `json:"serial"`
	IP                string    `json:"ip"`
	Port              int       `json:"port"`
	Model             string    `json:"model"`
	ModelName         string    `json:"model_name"`
	FireOSVersion     string    `json:"fire_os_version"`
	AndroidVersion    string    `json:"android_version"`
	BuildVersionName  string    `json:"build_version_name"`
	IsFireTV4KMax2Gen bool      `json:"is_fire_tv_4k_max_2gen"`
	TotalStorage      string    `json:"total_storage"`
	FreeStorage       string    `json:"free_storage"`
	UsedStoragePct    string    `json:"used_storage_pct"`
	TotalRAMMB        int       `json:"total_ram_mb"`
	FreeRAMMB         int       `json:"free_ram_mb"`
	ZRAMUsedMB        int       `json:"zram_used_mb"`
	ActiveLauncher    string    `json:"active_launcher"`
	DisabledCount     int       `json:"disabled_count"`
	CPUTempC          float64   `json:"cpu_temp_c"`
	GPUTempC          float64   `json:"gpu_temp_c"`
	ThermalStatus     int       `json:"thermal_status"` // 0 = Normal, 1 = Light, 2 = Moderate, 3 = Severe
	CPUFreqMHz        []int     `json:"cpu_freq_mhz"`
	CPULoad           string    `json:"cpu_load"`
	OTABlocked        bool      `json:"ota_blocked"`
	ConnectedAt       time.Time `json:"connected_at"`
}

// SystemSetting represents an Android global/secure system setting.
type SystemSetting struct {
	Namespace    string `json:"namespace"` // "global", "secure", "system"
	Key          string `json:"key"`
	Value        string `json:"value"`
	Description  string `json:"description"`
	DefaultValue string `json:"default_value,omitempty"`
}

// AppOp represents an Android app operation restriction (e.g. RUN_IN_BACKGROUND).
type AppOp struct {
	Package string `json:"package"`
	Op      string `json:"op"`
	Mode    string `json:"mode"` // "ignore", "allow", "default"
}

// Profile represents a pre-configured debloat & optimization strategy.
type Profile struct {
	ID                 string          `json:"id"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	PreservesVoice     bool            `json:"preserves_voice"`
	PreservesStreaming bool            `json:"preserves_streaming"`
	DisabledPackages   []string        `json:"disabled_packages"`
	Settings           []SystemSetting `json:"settings"`
	AppOps             []AppOp         `json:"app_ops"`
}

// Backup represents a snapshot of the device state before optimization.
type Backup struct {
	ID               string          `json:"id"`
	CreatedAt        time.Time       `json:"created_at"`
	DeviceSerial     string          `json:"device_serial"`
	DeviceModel      string          `json:"device_model"`
	FireOSVersion    string          `json:"fire_os_version"`
	DisabledPackages []string        `json:"disabled_packages"`
	Settings         []SystemSetting `json:"settings"`
}

// SideloadApp represents an application in the 1-click install catalog.
type SideloadApp struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	PackageName string `json:"package_name"`
	DownloadURL string `json:"download_url"`
	Website     string `json:"website"`
	Installed   bool   `json:"installed,omitempty"`
}

// AppDatabase represents the root structure of data/apps.json.
type AppDatabase struct {
	Version string        `json:"version"`
	Apps    []SideloadApp `json:"apps"`
}
