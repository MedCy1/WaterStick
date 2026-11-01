package tuning

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// DNS Providers presets
const (
	DNSAdGuard    = "dns.adguard-dns.com"
	DNSQuad9      = "dns.quad9.net"
	DNSCloudflare = "one.one.one.one"
	DNSFamily     = "family.adguard-dns.com"
)

// AudioTweakConfig holds sound configuration for Fire OS.
type AudioTweakConfig struct {
	CompressedAudioPref    string `json:"compressed_audio_pref"` // 0=Stereo, 1=DD+, 3=Auto_HDMI, 4=Dolby_HDMI (AC-3)
	EncodedSurroundOutput  string `json:"encoded_surround_output"` // 0=Auto, 1=Never, 2=Always, 3=Manual
	SurroundEnabledFormats string `json:"surround_enabled_formats"` // 5=AC3, 6=E-AC3, 7=DTS, 8=DTS-HD
	NavigationSounds       bool   `json:"navigation_sounds"`
	IsOptical51Optimized   bool   `json:"is_optical_51_optimized"`
}

// DisplayTweakConfig holds display and gaming video configuration.
type DisplayTweakConfig struct {
	MatchFrameRate   bool   `json:"match_frame_rate"` // cinema_mode_status
	Resolution       string `json:"resolution"`
	HDRSetting       string `json:"hdr_setting"` // adaptive, forced, sdr
	DeepColor        string `json:"deep_color"`  // 8, 10, 12 bit
	LowLatencyMode   string `json:"low_latency_mode"` // auto, enable, disable
	SuperResolution  string `json:"super_resolution"`  // enable, disable
}

// PowerTweakConfig holds sleep, screensaver and ambient mode settings.
type PowerTweakConfig struct {
	AmbientExperienceEnabled bool `json:"ambient_experience_enabled"`
	ScreensaverActivateSleep bool `json:"screensaver_activate_sleep"`
	SleepTimeoutMinutes      int  `json:"sleep_timeout_minutes"`
}

// DNSTweakConfig holds Android Private DNS (DoT) configuration.
type DNSTweakConfig struct {
	Mode         string `json:"mode"`          // hostname, opportunistic, off
	Specifier    string `json:"specifier"`     // provider host
	ProviderName string `json:"provider_name"` // Human-friendly label
}

// GetAudioTweakConfig retrieves active audio output & surround settings.
func (t *Tuner) GetAudioTweakConfig(ctx context.Context, deviceSerial string) (AudioTweakConfig, error) {
	pref, _ := t.GetSetting(ctx, deviceSerial, "secure", "amazon_settings_audio_format_compressed_pref")
	surround, _ := t.GetSetting(ctx, deviceSerial, "global", "encoded_surround_output")
	formats, _ := t.GetSetting(ctx, deviceSerial, "global", "encoded_surround_output_enabled_formats")
	nav, _ := t.GetSetting(ctx, deviceSerial, "system", "sound_effects_enabled")

	cfg := AudioTweakConfig{
		CompressedAudioPref:    pref,
		EncodedSurroundOutput:  surround,
		SurroundEnabledFormats: formats,
		NavigationSounds:       nav != "0",
	}

	// Optical 5.1 is active when AC-3 standard (4) and manual/ac3 surround are configured
	if pref == "4" && (surround == "3" || surround == "2") {
		cfg.IsOptical51Optimized = true
	}

	return cfg, nil
}

// SetOptical51Mode configures audio pass-through specifically optimized for 5.1 audio amplifiers
// connected via Optical TOSLink/SPDIF (forcing standard Dolby Digital AC-3 bitstream).
func (t *Tuner) SetOptical51Mode(ctx context.Context, deviceSerial string, enable bool) error {
	if enable {
		// 1. Force Dolby Digital AC-3 (AudioOutputDevice.Dolby_HDMI = 4)
		if _, err := t.client.Shell(ctx, deviceSerial, "settings put secure amazon_settings_audio_format_compressed_pref 4"); err != nil {
			return err
		}
		// 2. Manual surround selection (3) with AC-3 (5) and DTS (7) formats
		if _, err := t.client.Shell(ctx, deviceSerial, "settings put global encoded_surround_output 3"); err != nil {
			return err
		}
		if _, err := t.client.Shell(ctx, deviceSerial, "settings put global encoded_surround_output_enabled_formats '5,7'"); err != nil {
			return err
		}
		if _, err := t.client.Shell(ctx, deviceSerial, "settings put global firetv_hdmi_dolby_passthrough_available 1"); err != nil {
			return err
		}
	} else {
		// Reset to default Auto (AudioOutputDevice.Auto_HDMI = 3)
		if _, err := t.client.Shell(ctx, deviceSerial, "settings put secure amazon_settings_audio_format_compressed_pref 3"); err != nil {
			return err
		}
		if _, err := t.client.Shell(ctx, deviceSerial, "settings put global encoded_surround_output 0"); err != nil {
			return err
		}
		if _, err := t.client.Shell(ctx, deviceSerial, "settings put global encoded_surround_output_enabled_formats '5,6,7,8'"); err != nil {
			return err
		}
	}
	return nil
}

// SetNavigationSounds toggles UI click audio effects.
func (t *Tuner) SetNavigationSounds(ctx context.Context, deviceSerial string, enable bool) error {
	val := "1"
	if !enable {
		val = "0"
	}
	_, err := t.client.Shell(ctx, deviceSerial, fmt.Sprintf("settings put system sound_effects_enabled %s", val))
	return err
}

// GetDisplayTweakConfig retrieves display, refresh rate and HDR/resolution information.
func (t *Tuner) GetDisplayTweakConfig(ctx context.Context, deviceSerial string) (DisplayTweakConfig, error) {
	cinema, _ := t.GetSetting(ctx, deviceSerial, "global", "cinema_mode_status")
	matchContent, _ := t.GetSetting(ctx, deviceSerial, "global", "match_content_frame_rate")
	hdr, _ := t.client.Shell(ctx, deviceSerial, "getprop persist.sys.hdmi.hdr.enable")
	deepColor, _ := t.client.Shell(ctx, deviceSerial, "getprop persist.sys.open.deepcolor")
	ll, _ := t.client.Shell(ctx, deviceSerial, "getprop persist.sys.hdmi.ll.mode")
	sr, _ := t.client.Shell(ctx, deviceSerial, "getprop persist.sys.open.sr.mode")
	tvRes, _ := t.client.Shell(ctx, deviceSerial, "getprop persist.sys.hdmi.tvresolution")

	isMatch := cinema == "1" || matchContent == "1"

	return DisplayTweakConfig{
		MatchFrameRate:  isMatch,
		Resolution:      strings.TrimSpace(tvRes),
		HDRSetting:      strings.TrimSpace(hdr),
		DeepColor:       strings.TrimSpace(deepColor),
		LowLatencyMode:  strings.TrimSpace(ll),
		SuperResolution: strings.TrimSpace(sr),
	}, nil
}

// SetMatchFrameRate toggles native 24p cinema refresh rate switching without judder.
func (t *Tuner) SetMatchFrameRate(ctx context.Context, deviceSerial string, enable bool) error {
	val := "1"
	if !enable {
		val = "0"
	}
	if _, err := t.client.Shell(ctx, deviceSerial, fmt.Sprintf("settings put global cinema_mode_status %s", val)); err != nil {
		return err
	}
	_, err := t.client.Shell(ctx, deviceSerial, fmt.Sprintf("settings put global match_content_frame_rate %s", val))
	return err
}

// GetPowerTweakConfig reads ambient experience and sleep configurations.
func (t *Tuner) GetPowerTweakConfig(ctx context.Context, deviceSerial string) (PowerTweakConfig, error) {
	ambient, _ := t.GetSetting(ctx, deviceSerial, "secure", "ambient_experience_enabled")
	screensaver, _ := t.GetSetting(ctx, deviceSerial, "secure", "screensaver_activate_on_sleep")
	sleepRaw, _ := t.GetSetting(ctx, deviceSerial, "secure", "sleep_timeout")

	timeoutMs, err := strconv.Atoi(sleepRaw)
	timeoutMin := 21
	if err == nil && timeoutMs > 0 {
		timeoutMin = timeoutMs / 60000
	}

	return PowerTweakConfig{
		AmbientExperienceEnabled: ambient == "1",
		ScreensaverActivateSleep: screensaver != "0",
		SleepTimeoutMinutes:      timeoutMin,
	}, nil
}

// SetAmbientExperience enables or completely disables the background Ambient Experience widgets/ads.
func (t *Tuner) SetAmbientExperience(ctx context.Context, deviceSerial string, enable bool) error {
	val := "1"
	if !enable {
		val = "0"
	}
	_, err := t.client.Shell(ctx, deviceSerial, fmt.Sprintf("settings put secure ambient_experience_enabled %s", val))
	return err
}

// SetSleepTimeout sets the inactive sleep timeout in minutes.
func (t *Tuner) SetSleepTimeout(ctx context.Context, deviceSerial string, minutes int) error {
	if minutes < 1 {
		minutes = 1
	}
	timeoutMs := minutes * 60000
	_, err := t.client.Shell(ctx, deviceSerial, fmt.Sprintf("settings put secure sleep_timeout %d", timeoutMs))
	return err
}

// GetDNSTweakConfig returns the active DoT configuration and its preset friendly name.
func (t *Tuner) GetDNSTweakConfig(ctx context.Context, deviceSerial string) (DNSTweakConfig, error) {
	mode, _ := t.GetSetting(ctx, deviceSerial, "global", "private_dns_mode")
	spec, _ := t.GetSetting(ctx, deviceSerial, "global", "private_dns_specifier")

	provider := "Inactif / Opérateur"
	if mode == "hostname" {
		switch spec {
		case DNSAdGuard, "dns.adguard.com":
			provider = "AdGuard (Anti-Pub & Télémétrie)"
		case DNSQuad9:
			provider = "Quad9 (Sécurité & Malwares)"
		case DNSCloudflare:
			provider = "Cloudflare (1.1.1.1 Ultra-Rapide)"
		case DNSFamily:
			provider = "AdGuard Family (Contrôle Parental)"
		default:
			provider = fmt.Sprintf("Personnalisé (%s)", spec)
		}
	} else if mode == "opportunistic" {
		provider = "Automatique (Opportuniste)"
	}

	return DNSTweakConfig{
		Mode:         mode,
		Specifier:    spec,
		ProviderName: provider,
	}, nil
}

// SetDNSProvider configures DNS-over-TLS with one of the curated presets or turns it off.
func (t *Tuner) SetDNSProvider(ctx context.Context, deviceSerial, providerKey string) error {
	var mode, specifier string

	switch strings.ToLower(providerKey) {
	case "adguard":
		mode = "hostname"
		specifier = DNSAdGuard
	case "quad9":
		mode = "hostname"
		specifier = DNSQuad9
	case "cloudflare":
		mode = "hostname"
		specifier = DNSCloudflare
	case "family":
		mode = "hostname"
		specifier = DNSFamily
	case "off", "disable", "none":
		mode = "off"
		specifier = ""
	case "auto", "opportunistic":
		mode = "opportunistic"
		specifier = ""
	default:
		// If user passed a custom hostname
		if strings.Contains(providerKey, ".") {
			mode = "hostname"
			specifier = providerKey
		} else {
			return fmt.Errorf("inconnu preset DNS: %s (choix: adguard, quad9, cloudflare, family, off)", providerKey)
		}
	}

	if _, err := t.client.Shell(ctx, deviceSerial, fmt.Sprintf("settings put global private_dns_mode %s", mode)); err != nil {
		return err
	}
	if specifier != "" {
		if _, err := t.client.Shell(ctx, deviceSerial, fmt.Sprintf("settings put global private_dns_specifier %s", specifier)); err != nil {
			return err
		}
	}
	return nil
}
