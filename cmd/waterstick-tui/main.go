package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/MedCy1/WaterStick/pkg/waterstick"
	"github.com/MedCy1/WaterStick/pkg/waterstick/models"
	"github.com/MedCy1/WaterStick/pkg/waterstick/tuning"
	"github.com/MedCy1/WaterStick/pkg/waterstick/updater"
)

var version = "dev"

type tabIndex int

const (
	tabHealth tabIndex = iota
	tabDebloat
	tabRollback
	tabSideload
	tabConnect
)

type tickMsg time.Time
type pollMsg time.Time
type updateAvailableMsg string

type model struct {
	engine          *waterstick.Engine
	activeTab       tabIndex
	targetDevice    string
	deviceInfo      *models.Device
	ipInput         string
	statusMsg       string
	isBusy          bool
	updateAvailable string
	packages     []models.Package
	selectedProf string
	apps         []models.SideloadApp
	cursor       int
	width        int
	height       int

	// Advanced Hardware & System Tweaks
	tweakAudio    tuning.AudioTweakConfig
	tweakDisplay  tuning.DisplayTweakConfig
	tweakPower    tuning.PowerTweakConfig
	tweakDNS      tuning.DNSTweakConfig
	debloatCursor int // 0: Profil Équilibré, 1: Profil Streamer, 2: Ampli Optique, 3: Nav sounds, 4: Match 24p, 5: Ambient, 6: DNS DoT

	// Diagnostics Focus & Navigation
	healthFocusZone int // 0: Top Toolbar, 1: Package Explorer
	healthActionIdx int // 0: Purger cache RAM, 1: Bouclier OTA, 2: Rafraîchir
	pkgCursor       int // cursor inside filtered packages
	pkgFilter       int // 0: Tous, 1: Gelés, 2: Actifs

	// Rollback Focus & Confirmation
	rollbackConfirm bool // true when confirmation dialog is shown

	// Sideload Multi-selection
	selectedApps map[string]bool // map of appID -> isSelected

	// Connect Focus & Navigation
	connectFocus int // 0: IP Input, 1: Connect button, 2: Disconnect button (if connected)

	// Motion & Animation state
	animFrame       int
	displayedRamPct float64
	targetRamPct    float64
	toastMsg        string
	toastType       string
	toastTicks      int
	toastMaxTicks   int
}

// Initial state
func initialModel(engine *waterstick.Engine) model {
	devs, _ := engine.ADB.ListDevices(context.Background())
	target := ""
	ip := "192.168.1."
	initialTab := tabConnect
	status := "Prêt. [Tab] ou [1-5] pour naviguer entre les panneaux."

	if len(devs) > 0 {
		target = devs[0]
		ip = strings.Split(target, ":")[0]
		initialTab = tabHealth
		status = fmt.Sprintf("Connecté à %s — Télémétrie chargée", target)
	}

	apps := engine.Sideload.GetApps()
	if target != "" {
		if annotated, err := engine.Sideload.AnnotateInstalled(context.Background(), target); err == nil {
			apps = annotated
		}
	}

	return model{
		engine:          engine,
		activeTab:       initialTab,
		targetDevice:    target,
		ipInput:         ip,
		statusMsg:       status,
		selectedProf:    "balanced",
		apps:            apps,
		selectedApps:    make(map[string]bool),
		connectFocus:    1,
		width:           90,
		height:          28,
		displayedRamPct: 0.5,
		targetRamPct:    0.5,
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func pollCmd() tea.Cmd {
	return tea.Tick(4*time.Second, func(t time.Time) tea.Msg {
		return pollMsg(t)
	})
}

func (m model) Init() tea.Cmd {
	cmds := []tea.Cmd{tickCmd(), pollCmd(), func() tea.Msg { return checkUpdateCmd() }}
	if m.targetDevice != "" {
		cmds = append(cmds, m.fetchDiagnosticsCmd())
	}
	return tea.Batch(cmds...)
}

func checkUpdateCmd() tea.Msg {
	u := updater.NewUpdateChecker()
	rel, isNewer, err := u.CheckLatest(context.Background(), version)
	if err == nil && isNewer {
		return updateAvailableMsg(rel.TagName)
	}
	return nil
}

type diagMsg struct {
	dev   *models.Device
	pkgs  []models.Package
	apps  []models.SideloadApp
	audio tuning.AudioTweakConfig
	disp  tuning.DisplayTweakConfig
	pwr   tuning.PowerTweakConfig
	dns   tuning.DNSTweakConfig
	err   error
}

func (m model) fetchDiagnosticsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		info, err := m.engine.Inspector.Inspect(ctx, m.targetDevice)
		if err != nil {
			return diagMsg{err: err}
		}
		pkgs, _ := m.engine.Packages.GetPackagesWithStatus(ctx, m.targetDevice)
		apps, _ := m.engine.Sideload.AnnotateInstalled(ctx, m.targetDevice)
		audio, _ := m.engine.GetAudioTweakConfig(ctx, m.targetDevice)
		disp, _ := m.engine.GetDisplayTweakConfig(ctx, m.targetDevice)
		pwr, _ := m.engine.GetPowerTweakConfig(ctx, m.targetDevice)
		dns, _ := m.engine.GetDNSTweakConfig(ctx, m.targetDevice)
		return diagMsg{dev: info, pkgs: pkgs, apps: apps, audio: audio, disp: disp, pwr: pwr, dns: dns}
	}
}

type actionDoneMsg struct {
	msg       string
	toastType string
	err       error
}

func (m model) connectCmd(ip string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		target := ip
		if !strings.Contains(target, ":") {
			target += ":5555"
		}
		err := m.engine.ADB.Connect(ctx, target)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		return actionDoneMsg{msg: fmt.Sprintf("Connecté avec succès à %s", target), toastType: "success"}
	}
}

func (m model) disconnectCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.engine.ADB.Disconnect(ctx, m.targetDevice)
		return actionDoneMsg{msg: "Périphérique déconnecté", toastType: "info"}
	}
}

func (m model) applyProfileCmd(prof string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		report, err := m.engine.ApplyProfile(ctx, m.targetDevice, prof, nil)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		return actionDoneMsg{
			msg:       fmt.Sprintf("Profil %s appliqué (%d paquets désactivés)", prof, report.DisabledCount),
			toastType: "success",
		}
	}
}

func (m model) toggleOptical51Cmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		newState := !m.tweakAudio.IsOptical51Optimized
		err := m.engine.SetOptical51Mode(ctx, m.targetDevice, newState)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		if newState {
			return actionDoneMsg{msg: "Mode Ampli 5.1 Optique ACTIVÉ (AC-3 Bitstream)", toastType: "success"}
		}
		return actionDoneMsg{msg: "Mode Audio restauré en Automatique", toastType: "info"}
	}
}

func (m model) toggleNavSoundsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		newState := !m.tweakAudio.NavigationSounds
		err := m.engine.SetNavigationSounds(ctx, m.targetDevice, newState)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		if newState {
			return actionDoneMsg{msg: "Sons de navigation activés", toastType: "success"}
		}
		return actionDoneMsg{msg: "Sons de navigation désactivés (silencieux)", toastType: "info"}
	}
}

func (m model) toggleMatchFrameRateCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		newState := !m.tweakDisplay.MatchFrameRate
		err := m.engine.SetMatchFrameRate(ctx, m.targetDevice, newState)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		if newState {
			return actionDoneMsg{msg: "Match Original Frame Rate 24p ACTIVÉ", toastType: "success"}
		}
		return actionDoneMsg{msg: "Match Frame Rate désactivé (60Hz fixe)", toastType: "info"}
	}
}

func (m model) toggleAmbientCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		newState := !m.tweakPower.AmbientExperienceEnabled
		err := m.engine.SetAmbientExperience(ctx, m.targetDevice, newState)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		if newState {
			return actionDoneMsg{msg: "Ambient Experience activé", toastType: "success"}
		}
		return actionDoneMsg{msg: "Ambient Experience DÉSACTIVÉ (veille publicitaire coupée)", toastType: "info"}
	}
}

func (m model) cycleDNSCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		nextProvider := "adguard"
		switch m.tweakDNS.Specifier {
		case tuning.DNSAdGuard, "dns.adguard.com":
			nextProvider = "quad9"
		case tuning.DNSQuad9:
			nextProvider = "cloudflare"
		case tuning.DNSCloudflare:
			nextProvider = "family"
		case tuning.DNSFamily:
			nextProvider = "off"
		default:
			nextProvider = "adguard"
		}
		err := m.engine.SetDNSProvider(ctx, m.targetDevice, nextProvider)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		cfg, _ := m.engine.GetDNSTweakConfig(ctx, m.targetDevice)
		return actionDoneMsg{msg: fmt.Sprintf("DoT DNS : %s", cfg.ProviderName), toastType: "success"}
	}
}


func (m model) rollbackCmd() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := m.engine.Backup.RollbackAll(ctx, m.targetDevice, nil)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		return actionDoneMsg{
			msg:       "Restauration terminée : réglages d'usine Amazon rétablis",
			toastType: "success",
		}
	}
}

func (m model) installAppCmd(appID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		err := m.engine.Sideload.InstallApp(ctx, m.targetDevice, appID, nil)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		return actionDoneMsg{
			msg:       fmt.Sprintf("Application %s installée avec succès", appID),
			toastType: "success",
		}
	}
}

func (m model) installBatchAppsCmd(appIDs []string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var succeeded []string
		var failed []string
		for _, id := range appIDs {
			err := m.engine.Sideload.InstallApp(ctx, m.targetDevice, id, nil)
			if err != nil {
				failed = append(failed, id)
			} else {
				succeeded = append(succeeded, id)
			}
		}
		if len(failed) > 0 {
			return actionDoneMsg{
				msg:       fmt.Sprintf("%d installée(s), %d échec(s)", len(succeeded), len(failed)),
				toastType: "info",
			}
		}
		return actionDoneMsg{
			msg:       fmt.Sprintf("%d application(s) installée(s) avec succès", len(succeeded)),
			toastType: "success",
		}
	}
}

func (m model) toggleOTACmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		blocked, err := m.engine.IsOTABlocked(ctx, m.targetDevice)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		if blocked {
			if err := m.engine.UnblockOTA(ctx, m.targetDevice); err != nil {
				return actionDoneMsg{err: err, toastType: "error"}
			}
			return actionDoneMsg{msg: "Bouclier OTA désactivé : mises à jour rétablies", toastType: "info"}
		}
		if err := m.engine.BlockOTA(ctx, m.targetDevice); err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		return actionDoneMsg{msg: "Bouclier OTA activé : mises à jour Amazon gelées", toastType: "success"}
	}
}

func (m model) purgeRAMCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		before, after, freed, err := m.engine.PurgeMemory(ctx, m.targetDevice)
		if err != nil {
			return actionDoneMsg{err: err, toastType: "error"}
		}
		if freed > 0 {
			return actionDoneMsg{
				msg:       fmt.Sprintf("Cache purgé : +%d Mo libérés (%d Mo -> %d Mo)", freed, before, after),
				toastType: "success",
			}
		}
		return actionDoneMsg{
			msg:       "Mémoire vive déjà optimisée (aucun déchet)",
			toastType: "info",
		}
	}
}

func (m model) togglePackageCmd(pkg models.Package) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if pkg.Disabled {
			err := m.engine.Packages.EnablePackage(ctx, m.targetDevice, pkg.ID)
			if err != nil {
				return actionDoneMsg{err: err, toastType: "error"}
			}
			return actionDoneMsg{msg: fmt.Sprintf("Paquet %s réactivé", pkg.ID), toastType: "success"}
		} else {
			if pkg.Tier == models.TierVoice {
				return actionDoneMsg{msg: "Action bloquée : le module vocal Alexa est sanctuarisé", toastType: "error"}
			}
			err := m.engine.Packages.DisablePackage(ctx, m.targetDevice, pkg.ID, false)
			if err != nil {
				return actionDoneMsg{err: err, toastType: "error"}
			}
			return actionDoneMsg{msg: fmt.Sprintf("Paquet %s gelé", pkg.ID), toastType: "success"}
		}
	}
}

func (m model) filteredPackages() []models.Package {
	if m.pkgFilter == 0 {
		return m.packages
	}
	var res []models.Package
	for _, p := range m.packages {
		if m.pkgFilter == 1 && p.Disabled {
			res = append(res, p)
		} else if m.pkgFilter == 2 && !p.Disabled {
			res = append(res, p)
		}
	}
	return res
}

// -------------------------------------------------------------
// Update & Keyboard Navigation
// -------------------------------------------------------------
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case updateAvailableMsg:
		m.updateAvailable = string(msg)
		return m, nil

	case tickMsg:
		m.animFrame++
		// Smooth spring ease-out decay on RAM gauge
		diff := m.targetRamPct - m.displayedRamPct
		if diff > 0.005 || diff < -0.005 {
			m.displayedRamPct += diff * 0.22
		} else {
			m.displayedRamPct = m.targetRamPct
		}

		// Toast countdown decay
		if m.toastTicks > 0 {
			m.toastTicks--
			if m.toastTicks == 0 {
				m.toastMsg = ""
			}
		}
		return m, tickCmd()

	case pollMsg:
		// Auto-refresh telemetry every 4s on health tab when not busy
		if m.targetDevice != "" && m.activeTab == tabHealth && !m.isBusy {
			return m, tea.Batch(pollCmd(), m.fetchDiagnosticsCmd())
		}
		return m, pollCmd()

	case tea.KeyPressMsg:
		k := msg.String()

		if k == "ctrl+c" {
			return m, tea.Quit
		}

		// 1. Direct Tab Jump with Keys 1-5 (Allowed unless typing in IP input)
		if m.activeTab != tabConnect || m.connectFocus != 0 {
			switch k {
			case "1":
				m.activeTab = tabHealth
				m.cursor = 0
				m.rollbackConfirm = false
				if m.targetDevice != "" {
					m.isBusy = true
					return m, m.fetchDiagnosticsCmd()
				}
				return m, nil
			case "2":
				m.activeTab = tabDebloat
				m.cursor = 0
				m.rollbackConfirm = false
				return m, nil
			case "3":
				m.activeTab = tabRollback
				m.cursor = 0
				m.rollbackConfirm = false
				return m, nil
			case "4":
				m.activeTab = tabSideload
				m.cursor = 0
				m.rollbackConfirm = false
				return m, nil
			case "5":
				m.activeTab = tabConnect
				m.rollbackConfirm = false
				m.connectFocus = 1
				return m, nil
			case "q":
				return m, tea.Quit
			}
		}

		// 2. Universal Tab Bracket Navigation ([ and ] or < and >)
		if k == "[" || k == "<" {
			m.activeTab = (m.activeTab + 4) % 5
			m.cursor = 0
			m.rollbackConfirm = false
			if m.activeTab == tabHealth && m.targetDevice != "" {
				m.isBusy = true
				return m, m.fetchDiagnosticsCmd()
			}
			return m, nil
		}
		if k == "]" || k == ">" {
			m.activeTab = (m.activeTab + 1) % 5
			m.cursor = 0
			m.rollbackConfirm = false
			if m.activeTab == tabHealth && m.targetDevice != "" {
				m.isBusy = true
				return m, m.fetchDiagnosticsCmd()
			}
			return m, nil
		}

		// 3. Tab-Specific Interaction & Navigation
		switch m.activeTab {

		case tabHealth:
			switch k {
			case "tab":
				// Switch focus between Action Toolbar and Package Explorer
				if m.healthFocusZone == 0 {
					if len(m.packages) > 0 {
						m.healthFocusZone = 1
					}
				} else {
					m.healthFocusZone = 0
				}
				return m, nil

			case "shift+tab":
				if m.healthFocusZone == 1 {
					m.healthFocusZone = 0
				} else {
					m.activeTab = tabConnect
					m.connectFocus = 1
				}
				return m, nil

			case "c", "p":
				m.healthActionIdx = 0
				m.isBusy = true
				m.statusMsg = "Purge du cache RAM et des surfaces vidéo..."
				return m, m.purgeRAMCmd()

			case "o":
				m.healthActionIdx = 1
				m.isBusy = true
				m.statusMsg = "Mise à jour du Bouclier OTA..."
				return m, m.toggleOTACmd()

			case "r":
				m.healthActionIdx = 2
				m.isBusy = true
				m.statusMsg = "Rafraîchissement des télémétries..."
				return m, m.fetchDiagnosticsCmd()
			}

			if m.healthFocusZone == 0 {
				// Focused on Action Toolbar
				switch k {
				case "left", "h":
					if m.healthActionIdx > 0 {
						m.healthActionIdx--
					}
				case "right", "l":
					if m.healthActionIdx < 2 {
						m.healthActionIdx++
					}
				case "down", "j":
					if len(m.packages) > 0 {
						m.healthFocusZone = 1
					}
				case "enter", " ":
					switch m.healthActionIdx {
					case 0:
						m.isBusy = true
						m.statusMsg = "Purge du cache RAM et des surfaces vidéo..."
						return m, m.purgeRAMCmd()
					case 1:
						m.isBusy = true
						m.statusMsg = "Mise à jour du Bouclier OTA..."
						return m, m.toggleOTACmd()
					case 2:
						m.isBusy = true
						m.statusMsg = "Rafraîchissement des télémétries..."
						return m, m.fetchDiagnosticsCmd()
					}
				}
			} else {
				// Focused on Live Package Explorer
				pkgs := m.filteredPackages()
				switch k {
				case "up", "k":
					if m.pkgCursor > 0 {
						m.pkgCursor--
					} else {
						m.healthFocusZone = 0
					}
				case "down", "j":
					if m.pkgCursor < len(pkgs)-1 {
						m.pkgCursor++
					}
				case "pageup":
					m.pkgCursor -= 6
					if m.pkgCursor < 0 {
						m.pkgCursor = 0
					}
				case "pagedown":
					m.pkgCursor += 6
					if m.pkgCursor >= len(pkgs) {
						m.pkgCursor = len(pkgs) - 1
					}
				case "f":
					m.pkgFilter = (m.pkgFilter + 1) % 3
					m.pkgCursor = 0
				case "esc":
					m.healthFocusZone = 0
				case "enter", " ":
					if len(pkgs) > 0 && m.pkgCursor < len(pkgs) {
						targetPkg := pkgs[m.pkgCursor]
						m.isBusy = true
						m.statusMsg = fmt.Sprintf("Modification de %s...", targetPkg.ID)
						return m, m.togglePackageCmd(targetPkg)
					}
				}
			}

		case tabDebloat:
			switch k {
			case "tab":
				m.activeTab = tabRollback
				return m, nil
			case "shift+tab":
				m.activeTab = tabHealth
				return m, nil
			case "up", "k":
				if m.debloatCursor > 2 {
					m.debloatCursor--
				} else if m.debloatCursor == 2 {
					m.debloatCursor = 0
				} else {
					m.debloatCursor = 0
				}
			case "down", "j":
				if m.debloatCursor == 0 || m.debloatCursor == 1 {
					m.debloatCursor = 2
				} else if m.debloatCursor < 6 {
					m.debloatCursor++
				}
			case "left", "h":
				if m.debloatCursor == 0 || m.debloatCursor == 1 {
					m.debloatCursor = 0
					m.selectedProf = "balanced"
				} else if m.debloatCursor == 6 {
					m.isBusy = true
					m.statusMsg = "Changement du résolveur DNS DoT..."
					return m, m.cycleDNSCmd()
				}
			case "right", "l":
				if m.debloatCursor == 0 || m.debloatCursor == 1 {
					m.debloatCursor = 1
					m.selectedProf = "streamer"
				} else if m.debloatCursor == 6 {
					m.isBusy = true
					m.statusMsg = "Changement du résolveur DNS DoT..."
					return m, m.cycleDNSCmd()
				}
			case "enter", " ", "a":
				if m.targetDevice == "" {
					m.statusMsg = "Erreur : connectez d'abord un périphérique Fire TV"
					m.toastMsg = "Aucun périphérique connecté"
					m.toastType = "error"
					m.toastTicks = 30
					m.toastMaxTicks = 30
					return m, nil
				}
				if m.debloatCursor == 0 || m.debloatCursor == 1 {
					prof := "balanced"
					if m.debloatCursor == 1 {
						prof = "streamer"
					}
					m.isBusy = true
					m.statusMsg = fmt.Sprintf("Application du profil %s...", prof)
					return m, m.applyProfileCmd(prof)
				} else if m.debloatCursor == 2 {
					m.isBusy = true
					m.statusMsg = "Bascule mode Ampli 5.1 Optique..."
					return m, m.toggleOptical51Cmd()
				} else if m.debloatCursor == 3 {
					m.isBusy = true
					m.statusMsg = "Bascule sons de navigation..."
					return m, m.toggleNavSoundsCmd()
				} else if m.debloatCursor == 4 {
					m.isBusy = true
					m.statusMsg = "Bascule fluidité cinéma 24p..."
					return m, m.toggleMatchFrameRateCmd()
				} else if m.debloatCursor == 5 {
					m.isBusy = true
					m.statusMsg = "Bascule veille publicitaire Ambient..."
					return m, m.toggleAmbientCmd()
				} else if m.debloatCursor == 6 {
					m.isBusy = true
					m.statusMsg = "Changement du résolveur DNS DoT..."
					return m, m.cycleDNSCmd()
				}
			}

		case tabRollback:
			switch k {
			case "tab":
				m.activeTab = tabSideload
				m.rollbackConfirm = false
				return m, nil
			case "shift+tab":
				m.activeTab = tabDebloat
				m.rollbackConfirm = false
				return m, nil
			case "esc", "n":
				m.rollbackConfirm = false
				return m, nil
			case "r", "enter", "o", "y", " ":
				if !m.rollbackConfirm {
					m.rollbackConfirm = true
					return m, nil
				}
				// Confirmed!
				m.rollbackConfirm = false
				if m.targetDevice == "" {
					m.statusMsg = "Erreur : connectez d'abord un périphérique Fire TV"
					m.toastMsg = "Aucun périphérique connecté"
					m.toastType = "error"
					m.toastTicks = 30
					m.toastMaxTicks = 30
					return m, nil
				}
				m.isBusy = true
				m.statusMsg = "Exécution de la restauration d'usine réversible..."
				return m, m.rollbackCmd()
			}

		case tabSideload:
			switch k {
			case "tab":
				m.activeTab = tabConnect
				m.connectFocus = 1
				return m, nil
			case "shift+tab":
				m.activeTab = tabRollback
				return m, nil
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.apps)-1 {
					m.cursor++
				}
			case " ":
				// Toggle multi-select checkbox
				if m.cursor < len(m.apps) {
					appID := m.apps[m.cursor].ID
					m.selectedApps[appID] = !m.selectedApps[appID]
				}
			case "a":
				// Toggle all
				allSelected := true
				for _, app := range m.apps {
					if !m.selectedApps[app.ID] {
						allSelected = false
						break
					}
				}
				for _, app := range m.apps {
					m.selectedApps[app.ID] = !allSelected
				}
			case "enter", "i":
				if m.targetDevice == "" {
					m.statusMsg = "Erreur : connectez d'abord un périphérique Fire TV"
					m.toastMsg = "Aucun périphérique connecté"
					m.toastType = "error"
					m.toastTicks = 30
					m.toastMaxTicks = 30
					return m, nil
				}

				// Check how many selected
				var selectedIDs []string
				for _, app := range m.apps {
					if m.selectedApps[app.ID] {
						selectedIDs = append(selectedIDs, app.ID)
					}
				}

				if len(selectedIDs) > 0 {
					m.isBusy = true
					m.statusMsg = fmt.Sprintf("Installation de %d application(s)...", len(selectedIDs))
					return m, m.installBatchAppsCmd(selectedIDs)
				}

				// Otherwise install focused app
				app := m.apps[m.cursor]
				m.isBusy = true
				m.statusMsg = fmt.Sprintf("Installation de %s...", app.Name)
				return m, m.installAppCmd(app.ID)
			}

		case tabConnect:
			switch k {
			case "tab":
				if m.connectFocus == 0 {
					m.connectFocus = 1
					return m, nil
				}
				m.activeTab = tabHealth
				m.connectFocus = 1
				m.cursor = 0
				if m.targetDevice != "" {
					m.isBusy = true
					return m, m.fetchDiagnosticsCmd()
				}
				return m, nil

			case "shift+tab":
				if m.connectFocus == 0 {
					m.connectFocus = 1
					return m, nil
				}
				m.activeTab = tabSideload
				m.cursor = 0
				return m, nil
			}

			if m.connectFocus == 0 {
				// Focused on IP Text Input
				switch k {
				case "enter":
					m.isBusy = true
					m.statusMsg = "Connexion en cours à " + m.ipInput + "..."
					return m, m.connectCmd(m.ipInput)
				case "down":
					m.connectFocus = 1
					return m, nil
				case "esc":
					m.connectFocus = 1
					return m, nil
				case "backspace":
					if len(m.ipInput) > 0 {
						m.ipInput = m.ipInput[:len(m.ipInput)-1]
					}
					return m, nil
				default:
					if len(k) == 1 && ((k >= "0" && k <= "9") || k == "." || k == ":") {
						m.ipInput += k
						return m, nil
					}
				}
			} else if m.connectFocus == 1 {
				// Focused on Connect Button
				switch k {
				case "up":
					m.connectFocus = 0
					return m, nil
				case "down":
					if m.targetDevice != "" {
						m.connectFocus = 2
					}
					return m, nil
				case "right", "l":
					if m.targetDevice != "" {
						m.connectFocus = 2
					} else {
						m.activeTab = tabHealth
					}
					return m, nil
				case "enter", " ":
					m.isBusy = true
					m.statusMsg = "Connexion en cours à " + m.ipInput + "..."
					return m, m.connectCmd(m.ipInput)
				case "esc":
					m.connectFocus = 0
					return m, nil
				case "left", "h":
					m.activeTab = tabSideload
					return m, nil
				}
			} else if m.connectFocus == 2 {
				// Focused on Disconnect Button
				switch k {
				case "up", "left", "h":
					m.connectFocus = 1
					return m, nil
				case "enter", " ", "d":
					m.isBusy = true
					m.statusMsg = "Déconnexion du périphérique..."
					m.targetDevice = ""
					m.deviceInfo = nil
					m.connectFocus = 0
					return m, m.disconnectCmd()
				}
			}
		}

	case diagMsg:
		m.isBusy = false
		if msg.err != nil {
			m.statusMsg = "Erreur diagnostics : " + msg.err.Error()
		} else {
			m.deviceInfo = msg.dev
			m.packages = msg.pkgs
			m.tweakAudio = msg.audio
			m.tweakDisplay = msg.disp
			m.tweakPower = msg.pwr
			m.tweakDNS = msg.dns
			if len(msg.apps) > 0 {
				m.apps = msg.apps
			}
			if msg.dev.TotalRAMMB > 0 {
				m.targetRamPct = float64(msg.dev.TotalRAMMB-msg.dev.FreeRAMMB) / float64(msg.dev.TotalRAMMB)
			}
			if !strings.Contains(m.statusMsg, "appliqué") && !strings.Contains(m.statusMsg, "Restauration") {
				m.statusMsg = fmt.Sprintf("Connecté : %s (%s)", msg.dev.ModelName, msg.dev.FireOSVersion)
			}
		}

	case actionDoneMsg:
		m.isBusy = false
		if msg.err != nil {
			m.statusMsg = "Erreur : " + msg.err.Error()
			m.toastMsg = msg.err.Error()
			m.toastType = "error"
			m.toastTicks = 40
			m.toastMaxTicks = 40
		} else {
			m.statusMsg = msg.msg
			m.toastMsg = msg.msg
			m.toastType = msg.toastType
			m.toastTicks = 40
			m.toastMaxTicks = 40

			if strings.HasPrefix(msg.msg, "Connecté avec succès") {
				m.targetDevice = m.ipInput
				if !strings.Contains(m.targetDevice, ":") {
					m.targetDevice += ":5555"
				}
				return m, m.fetchDiagnosticsCmd()
			}
			if strings.Contains(msg.msg, "appliqué") || strings.Contains(msg.msg, "Restauration") || strings.Contains(msg.msg, "OTA") || strings.Contains(msg.msg, "purgé") || strings.Contains(msg.msg, "Paquet") || strings.Contains(msg.msg, "installée") || strings.Contains(msg.msg, "Optique") || strings.Contains(msg.msg, "navigation") || strings.Contains(msg.msg, "Frame Rate") || strings.Contains(msg.msg, "Ambient") || strings.Contains(msg.msg, "DNS") {
				return m, m.fetchDiagnosticsCmd()
			}
		}
	}

	return m, nil
}

// -------------------------------------------------------------
// Visual Styles — Restrained Technical Palette & Motion
// -------------------------------------------------------------
var (
	cTextWhite  = lipgloss.Color("#F8FAFC")
	cTextMuted  = lipgloss.Color("#64748B")
	cTextSub    = lipgloss.Color("#94A3B8")
	cBorderDim  = lipgloss.Color("#30363D")
	cBorderBold = lipgloss.Color("#38BDF8")
	cAccentCyan = lipgloss.Color("#38BDF8")
	cGreen      = lipgloss.Color("#10B981")
	cAmber      = lipgloss.Color("#F59E0B")
	cRed        = lipgloss.Color("#EF4444")

	bannerTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(cTextWhite).
			Render("WATERSTICK")

	bannerSub = lipgloss.NewStyle().
			Foreground(cTextMuted).
			Render("  v1.0 (Fire OS 8.1 / Android 11)")

	tabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(cTextWhite).
			Background(lipgloss.Color("#21262D")).
			Padding(0, 1)

	tabInactive = lipgloss.NewStyle().
			Foreground(cTextMuted).
			Padding(0, 1)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cBorderDim).
			Padding(1, 2)

	cardActiveStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cBorderBold).
			Padding(1, 2)

	// Motion spinner & breathing beacon frames
	spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	beaconFrames  = []string{"●", "○", "·", "○"}
)

func renderBar(pct float64, width int, filledHex, emptyHex string) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filledLen := int(pct * float64(width))
	emptyLen := width - filledLen

	filled := lipgloss.NewStyle().
		Foreground(lipgloss.Color(filledHex)).
		Render(strings.Repeat("■", filledLen))
	empty := lipgloss.NewStyle().
		Foreground(lipgloss.Color(emptyHex)).
		Render(strings.Repeat("░", emptyLen))
	return filled + empty
}

// -------------------------------------------------------------
// View Function
// -------------------------------------------------------------
func (m model) View() tea.View {
	contentWidth := m.width - 4
	if contentWidth < 80 {
		contentWidth = 80
	}

	var s strings.Builder

	// 1. Header Line with Breathing Beacon
	beaconIdx := (m.animFrame / 4) % len(beaconFrames)
	beaconGlyph := beaconFrames[beaconIdx]

	devicePill := lipgloss.NewStyle().
		Foreground(cRed).
		Render("[Non connecté]")
	if m.targetDevice != "" {
		devicePill = lipgloss.JoinHorizontal(lipgloss.Center,
			lipgloss.NewStyle().Foreground(cTextSub).Render(m.targetDevice+" "),
			lipgloss.NewStyle().Foreground(cGreen).Render(fmt.Sprintf("[%s CONNECTÉ]", beaconGlyph)),
		)
	}

	headerLeft := lipgloss.JoinHorizontal(lipgloss.Center, bannerTitle, bannerSub)
	gapWidth := contentWidth - lipgloss.Width(headerLeft) - lipgloss.Width(devicePill)
	if gapWidth < 2 {
		gapWidth = 2
	}
	topHeader := lipgloss.JoinHorizontal(lipgloss.Center, headerLeft, strings.Repeat(" ", gapWidth), devicePill)
	s.WriteString(topHeader + "\n")
	s.WriteString(lipgloss.NewStyle().Foreground(cBorderDim).Render(strings.Repeat("─", contentWidth)) + "\n\n")

	// 2. Navigation Tabs Bar
	tabDefs := []struct {
		num  string
		name string
	}{
		{"1", "Diagnostics"},
		{"2", "Optimisation"},
		{"3", "Restauration"},
		{"4", "Applications"},
		{"5", "Connexion"},
	}

	var renderedTabs []string
	for i, def := range tabDefs {
		label := fmt.Sprintf("[%s] %s", def.num, def.name)
		if tabIndex(i) == m.activeTab {
			renderedTabs = append(renderedTabs, tabActive.Render(label))
		} else {
			renderedTabs = append(renderedTabs, tabInactive.Render(label))
		}
	}
	s.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, strings.Join(renderedTabs, "  ")) + "\n\n")

	// 3. Tab Body
	var body string
	switch m.activeTab {
	case tabHealth:
		body = m.renderHealth(contentWidth)
	case tabDebloat:
		body = m.renderDebloat(contentWidth)
	case tabRollback:
		body = m.renderRollback(contentWidth)
	case tabSideload:
		body = m.renderSideload(contentWidth)
	case tabConnect:
		body = m.renderConnect(contentWidth)
	}
	s.WriteString(body + "\n\n")

	// 4. In-Terminal Animated Toast Notification
	if m.toastMsg != "" && m.toastMaxTicks > 0 {
		toastColor := cGreen
		toastHex := "#10B981"
		toastTag := "SUCCÈS"
		if m.toastType == "error" {
			toastColor = cRed
			toastHex = "#EF4444"
			toastTag = "ERREUR"
		} else if m.toastType == "info" {
			toastColor = cAccentCyan
			toastHex = "#38BDF8"
			toastTag = "INFO"
		}

		toastMeter := renderBar(float64(m.toastTicks)/float64(m.toastMaxTicks), 10, toastHex, "#1E293B")
		toastLine := lipgloss.JoinHorizontal(lipgloss.Center,
			lipgloss.NewStyle().Bold(true).Foreground(toastColor).Render(fmt.Sprintf("[%s]", toastTag)),
			"  ",
			lipgloss.NewStyle().Foreground(cTextWhite).Render(m.toastMsg),
			"  ",
			toastMeter,
		)
		s.WriteString(lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(toastColor).
			Padding(0, 2).
			Width(contentWidth).
			Render(toastLine) + "\n\n")
	}

	// 5. Status Bar with Braille Spinner when Busy
	statusPrefix := "• "
	if m.isBusy {
		spinIdx := m.animFrame % len(spinnerFrames)
		statusPrefix = lipgloss.NewStyle().Foreground(cAccentCyan).Render(spinnerFrames[spinIdx]) + " "
	}
	statusText := lipgloss.NewStyle().Foreground(cTextWhite).Render(statusPrefix + m.statusMsg)

	s.WriteString(lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(cBorderDim).
		Padding(0, 1).
		Width(contentWidth).
		Render(statusText) + "\n")

	// 6. Dynamic Context-Aware Action Hints
	var helpText string
	switch m.activeTab {
	case tabHealth:
		if m.healthFocusZone == 0 {
			helpText = "Toolbar : [←/→] Action  •  [Entrée] Exécuter  •  [Tab/↓] Liste paquets  •  [c/o/r] Raccourcis  •  [1-5] Onglets"
		} else {
			helpText = "Paquets : [↑/↓] Défiler  •  [Espace] Geler/Dégeler  •  [f] Filtrer  •  [Tab/Esc] Toolbar  •  [1-5] Onglets"
		}
	case tabDebloat:
		helpText = "Navigation : [↑/↓] Choisir élément  •  [←/→] Profils  •  [Entrée/Espace] Appliquer / Basculer  •  [1-5] Onglets  •  [q] Quitter"
	case tabRollback:
		if m.rollbackConfirm {
			helpText = "Confirmation : [o / Entrée] Confirmer la restauration  •  [n / Esc] Annuler  •  [q] Quitter"
		} else {
			helpText = "Restauration : [Entrée / r] Lancer procédure  •  [Tab] Onglet suivant  •  [1-5] Onglets  •  [q] Quitter"
		}
	case tabSideload:
		helpText = "Applications : [↑/↓] Défiler  •  [Espace] Cocher  •  [a] Tout cocher  •  [Entrée] Installer  •  [Tab] Onglet suivant"
	case tabConnect:
		if m.connectFocus == 0 {
			helpText = "Saisie IP : [0-9 / .] Taper  •  [Entrée] Connecter  •  [Tab / Esc / ↓] Valider saisie  •  [q] Quitter"
		} else if m.connectFocus == 1 {
			helpText = "Bouton Liaison : [Entrée] Connecter  •  [↑] Modifier IP  •  [Tab] Onglet suivant  •  [1-5] Onglets  •  [q] Quitter"
		} else {
			helpText = "Bouton Déconnexion : [Entrée / d] Déconnecter  •  [← / ↑] Revenir au bouton Liaison  •  [Tab] Onglet suivant"
		}
	}

	helpStyled := lipgloss.NewStyle().Foreground(cTextMuted).Render(helpText)
	creditsStyled := lipgloss.NewStyle().Foreground(cBorderDim).Render("WaterStick v" + version + " • github.com/MedCy1")
	if m.updateAvailable != "" {
		creditsStyled = lipgloss.NewStyle().Foreground(cAmber).Render(fmt.Sprintf("[Mise à jour %s disponible : tapez 'waterstick update'] ", m.updateAvailable)) + lipgloss.NewStyle().Foreground(cBorderDim).Render("WaterStick v" + version + " • github.com/MedCy1")
	}
	footerGap := contentWidth - lipgloss.Width(helpStyled) - lipgloss.Width(creditsStyled)
	if footerGap > 2 {
		s.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, helpStyled, strings.Repeat(" ", footerGap), creditsStyled))
	} else {
		s.WriteString(helpStyled + "\n" + creditsStyled)
	}

	v := tea.NewView(s.String())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// -------------------------------------------------------------
// Tab 1: Diagnostics & Live Telemetry
// -------------------------------------------------------------
func (m model) renderHealth(w int) string {
	if m.deviceInfo == nil {
		lines := []string{
			lipgloss.NewStyle().Foreground(cAmber).Render("[!] Aucune donnée télémétrique disponible"),
			"",
			lipgloss.NewStyle().Foreground(cTextSub).Render("Connectez votre Fire TV Stick dans l'onglet [5] Connexion pour afficher l'analyse temps-réel."),
		}
		return cardStyle.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	}

	d := m.deviceInfo
	cardW := (w - 4) / 2

	// Interactive Action Toolbar
	btnStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(cBorderDim).Padding(0, 1).Foreground(cTextSub)
	btnFocused := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cAccentCyan).
		Background(lipgloss.Color("#162638")).
		Padding(0, 1).
		Bold(true).
		Foreground(cTextWhite)

	lbl0 := "  1. Purger cache RAM (c)"
	lbl1 := "  2. Bouclier OTA (o)"
	lbl2 := "  3. Rafraîchir (r)"

	b0 := btnStyle
	b1 := btnStyle
	b2 := btnStyle

	if m.healthFocusZone == 0 {
		switch m.healthActionIdx {
		case 0:
			b0 = btnFocused
			lbl0 = "▶ 1. Purger cache RAM (c)"
		case 1:
			b1 = btnFocused
			lbl1 = "▶ 2. Bouclier OTA (o)"
		case 2:
			b2 = btnFocused
			lbl2 = "▶ 3. Rafraîchir (r)"
		}
	}

	toolbar := lipgloss.JoinHorizontal(lipgloss.Center, b0.Render(lbl0), "  ", b1.Render(lbl1), "  ", b2.Render(lbl2))

	// Left Column: System & Hardware
	ramBar := renderBar(m.displayedRamPct, 12, "#0284C7", "#1E293B")
	tempStatus := lipgloss.NewStyle().Foreground(cGreen).Render("[Nominal]")
	if d.ThermalStatus == 1 {
		tempStatus = lipgloss.NewStyle().Foreground(cAmber).Render("[Throttling]")
	} else if d.ThermalStatus > 1 {
		tempStatus = lipgloss.NewStyle().Foreground(cRed).Render("[Surchauffe]")
	}

	otaBadge := lipgloss.NewStyle().Foreground(cGreen).Render("[GELÉ - Bloqué]")
	if !d.OTABlocked {
		otaBadge = lipgloss.NewStyle().Foreground(cAmber).Render("[ACTIF - Vulnérable]")
	}

	leftLines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(cTextWhite).Render("MÉTRIQUES MATÉRIEL & SYSTÈME"),
		"",
		fmt.Sprintf("  Modèle       : %s", lipgloss.NewStyle().Foreground(cTextWhite).Render(d.ModelName)),
		fmt.Sprintf("  Identifiant  : %s (%s)", d.Model, d.BuildVersionName),
		fmt.Sprintf("  Système      : %s (Android %s)", d.FireOSVersion, d.AndroidVersion),
		fmt.Sprintf("  Noyau SoC    : %s", tempStatus),
		fmt.Sprintf("  Températures : %.1f °C CPU | %.1f °C GPU", d.CPUTempC, d.GPUTempC),
		fmt.Sprintf("  Charge CPU   : %s", d.CPULoad),
		fmt.Sprintf("  RAM Physique : %s %d%%", ramBar, int(m.displayedRamPct*100)),
		fmt.Sprintf("                 %d Mo libres / %d Mo (ZRAM: %d Mo)", d.FreeRAMMB, d.TotalRAMMB, d.ZRAMUsedMB),
		fmt.Sprintf("  Stockage     : %s libres / %s (%s)", d.FreeStorage, d.TotalStorage, d.UsedStoragePct),
		fmt.Sprintf("  Bouclier OTA : %s", otaBadge),
		fmt.Sprintf("  Module Vocal : %s", lipgloss.NewStyle().Foreground(cGreen).Render("[100% Préservé & Protégé]")),
	}

	leftCard := cardStyle.Width(cardW).Render(lipgloss.JoinVertical(lipgloss.Left, leftLines...))

	// Right Column: Live Interactive Package Explorer
	pkgs := m.filteredPackages()
	filterLabels := []string{"[Tous]", "[Gelés]", "[Actifs]"}
	filterLabels[m.pkgFilter] = lipgloss.NewStyle().Bold(true).Foreground(cAccentCyan).Render(filterLabels[m.pkgFilter])
	filterBar := lipgloss.NewStyle().Foreground(cTextMuted).Render(fmt.Sprintf("Filtre: %s  (%d/%d)", strings.Join(filterLabels, " "), len(pkgs), len(m.packages)))

	rightLines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(cTextWhite).Render("TÉLÉMÉTRIE PAQUETS (LIVE)") + "  " + filterBar,
		"",
	}

	// Windowed scrolling for packages
	pageSize := 7
	startIdx := 0
	if m.pkgCursor >= pageSize {
		startIdx = m.pkgCursor - pageSize + 1
	}
	endIdx := startIdx + pageSize
	if endIdx > len(pkgs) {
		endIdx = len(pkgs)
	}

	if len(pkgs) == 0 {
		rightLines = append(rightLines, lipgloss.NewStyle().Foreground(cTextMuted).Render("  Aucun paquet correspondant au filtre sélectionné."))
	} else {
		for i := startIdx; i < endIdx; i++ {
			p := pkgs[i]
			cursorGlyph := "  "
			lineStyle := lipgloss.NewStyle().Foreground(cTextSub)

			if m.healthFocusZone == 1 && i == m.pkgCursor {
				cursorGlyph = "▶ "
				lineStyle = lipgloss.NewStyle().Bold(true).Foreground(cTextWhite)
			}

			tag := lipgloss.NewStyle().Foreground(cGreen).Render("[ACTIF]")
			if p.Disabled {
				tag = lipgloss.NewStyle().Foreground(cAmber).Render("[GELÉ ]")
			}
			if p.Tier == models.TierVoice {
				tag = lipgloss.NewStyle().Foreground(cAccentCyan).Render("[VOIX ]")
			}

			idShort := p.ID
			if len(idShort) > 28 {
				idShort = idShort[:25] + "..."
			}

			line := fmt.Sprintf("%s%s %-28s", cursorGlyph, tag, lineStyle.Render(idShort))
			rightLines = append(rightLines, line)
		}
	}

	rightLines = append(rightLines, "")
	rightFooter := lipgloss.NewStyle().Foreground(cTextMuted).Render("  [Espace] Basculer  •  [f] Filtrer  •  [Tab] Toolbar")
	if m.healthFocusZone == 1 {
		rightFooter = lipgloss.NewStyle().Foreground(cAccentCyan).Render("▶ [Espace / Entrée] Geler ou Réactiver le paquet ciblé")
	}
	rightLines = append(rightLines, rightFooter)

	rightCardStyle := cardStyle
	if m.healthFocusZone == 1 {
		rightCardStyle = cardActiveStyle
	}
	rightCard := rightCardStyle.Width(cardW).Render(lipgloss.JoinVertical(lipgloss.Left, rightLines...))

	contentRow := lipgloss.JoinHorizontal(lipgloss.Top, leftCard, "  ", rightCard)
	return lipgloss.JoinVertical(lipgloss.Left, toolbar, "\n", contentRow)
}

// -------------------------------------------------------------
// Tab 2: Optimisation (Profiles)
// -------------------------------------------------------------
func (m model) renderDebloat(w int) string {
	cardW := (w - 4) / 2

	// Profile 1: Balanced
	isBSelected := m.debloatCursor == 0
	borderB := cardStyle
	badgeB := lipgloss.NewStyle().Foreground(cTextMuted).Render("[ ]")
	btnTextB := "  Sélectionner ce profil (←/→)"

	if isBSelected {
		borderB = cardActiveStyle
		badgeB = lipgloss.NewStyle().Foreground(cAccentCyan).Render("[SÉLECTIONNÉ]")
		btnTextB = "▶ [ Appliquer le profil Équilibré (Entrée) ]"
	}

	actionBtn1 := lipgloss.NewStyle().Foreground(cTextSub).Render(btnTextB)
	if isBSelected {
		actionBtn1 = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cAccentCyan).
			Background(lipgloss.Color("#162638")).
			Padding(0, 1).
			Bold(true).
			Foreground(cTextWhite).
			Render(btnTextB)
	}

	lines1 := []string{
		lipgloss.NewStyle().Bold(true).Foreground(cTextWhite).Render("PROFIL 1 — ÉQUILIBRÉ") + "  " + badgeB,
		"",
		lipgloss.NewStyle().Foreground(cGreen).Render("  + Voix & télécommande Alexa 100% préservées"),
		lipgloss.NewStyle().Foreground(cGreen).Render("  + Prime Video, Photos et streaming officiel conservés"),
		lipgloss.NewStyle().Foreground(cTextMuted).Render("  - Télémétrie ACR et profilage publicitaire coupés (20 paquets)"),
		lipgloss.NewStyle().Foreground(cTextMuted).Render("  - Lecture vidéo et audio d'accueil désactivées"),
		lipgloss.NewStyle().Foreground(cAccentCyan).Render("  + Résolveur DNS AdGuard DoT chiffré"),
		lipgloss.NewStyle().Foreground(cAccentCyan).Render("  + Fluidité cinéma 24 fps et interface accélérée (0.5x)"),
		"",
		lipgloss.NewStyle().Foreground(cTextSub).Render("Idéal au quotidien : zéro bloatware, zéro régression."),
		"",
		actionBtn1,
	}

	card1 := borderB.Width(cardW).Render(lipgloss.JoinVertical(lipgloss.Left, lines1...))

	// Profile 2: Streamer Pur
	isSSelected := m.debloatCursor == 1
	borderS := cardStyle
	badgeS := lipgloss.NewStyle().Foreground(cTextMuted).Render("[ ]")
	btnTextS := "  Sélectionner ce profil (←/→)"

	if isSSelected {
		borderS = cardActiveStyle
		badgeS = lipgloss.NewStyle().Foreground(cAccentCyan).Render("[SÉLECTIONNÉ]")
		btnTextS = "▶ [ Appliquer le profil Streamer (Entrée) ]"
	}

	actionBtn2 := lipgloss.NewStyle().Foreground(cTextSub).Render(btnTextS)
	if isSSelected {
		actionBtn2 = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cAccentCyan).
			Background(lipgloss.Color("#162638")).
			Padding(0, 1).
			Bold(true).
			Foreground(cTextWhite).
			Render(btnTextS)
	}

	lines2 := []string{
		lipgloss.NewStyle().Bold(true).Foreground(cTextWhite).Render("PROFIL 2 — STREAMER DÉDIÉ") + "  " + badgeS,
		"",
		lipgloss.NewStyle().Foreground(cGreen).Render("  + Voix & télécommande Alexa 100% préservées"),
		lipgloss.NewStyle().Foreground(cTextMuted).Render("  - Prime Video, Musique et Cloud Photos désactivés (24 paquets)"),
		lipgloss.NewStyle().Foreground(cTextMuted).Render("  - Zéro service de synchronisation d'arrière-plan"),
		lipgloss.NewStyle().Foreground(cAccentCyan).Render("  + Résolveur DNS AdGuard DoT chiffré"),
		lipgloss.NewStyle().Foreground(cAccentCyan).Render("  + Libération maximale de mémoire vive (+180 Mo libres)"),
		"",
		lipgloss.NewStyle().Foreground(cTextSub).Render("Optimisé pour lecteurs tiers : Stremio, Kodi, SmartTube."),
		"",
		actionBtn2,
	}

	card2 := borderS.Width(cardW).Render(lipgloss.JoinVertical(lipgloss.Left, lines2...))

	cardsRow := lipgloss.JoinHorizontal(lipgloss.Top, card1, "  ", card2)

	// Hardware & Advanced Tweaks Section
	tweakLines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(cTextWhite).Render("RÉGLAGES MATÉRIEL & TWEAKS AVANCÉS"),
		"",
	}

	// 2. Optical 5.1
	optCursor := "  "
	optStyle := lipgloss.NewStyle().Foreground(cTextSub)
	optStatus := lipgloss.NewStyle().Foreground(cTextMuted).Render("[ Auto / HDMI Standard ]")
	if m.tweakAudio.IsOptical51Optimized {
		optStatus = lipgloss.NewStyle().Foreground(cGreen).Render("[ ACTIVÉ (Dolby Digital 5.1 AC-3 Bitstream) ]")
	}
	if m.debloatCursor == 2 {
		optCursor = "▶ "
		optStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccentCyan)
	}
	tweakLines = append(tweakLines, fmt.Sprintf("%s%s %s", optCursor, optStyle.Render("Mode Ampli 5.1 Optique / SPDIF :"), optStatus))

	// 3. Navigation Sounds
	navCursor := "  "
	navStyle := lipgloss.NewStyle().Foreground(cTextSub)
	navStatus := lipgloss.NewStyle().Foreground(cGreen).Render("[ Silencieux (Désactivés) ]")
	if m.tweakAudio.NavigationSounds {
		navStatus = lipgloss.NewStyle().Foreground(cTextMuted).Render("[ Activés (Standard) ]")
	}
	if m.debloatCursor == 3 {
		navCursor = "▶ "
		navStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccentCyan)
	}
	tweakLines = append(tweakLines, fmt.Sprintf("%s%s %s", navCursor, navStyle.Render("Sons de navigation télécommande:"), navStatus))

	// 4. Match Frame Rate
	mfrCursor := "  "
	mfrStyle := lipgloss.NewStyle().Foreground(cTextSub)
	mfrStatus := lipgloss.NewStyle().Foreground(cTextMuted).Render("[ Désactivé (60Hz fixe) ]")
	if m.tweakDisplay.MatchFrameRate {
		mfrStatus = lipgloss.NewStyle().Foreground(cGreen).Render("[ ACTIVÉ (Match 24p Cinéma natif sans saccades) ]")
	}
	if m.debloatCursor == 4 {
		mfrCursor = "▶ "
		mfrStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccentCyan)
	}
	tweakLines = append(tweakLines, fmt.Sprintf("%s%s %s", mfrCursor, mfrStyle.Render("Fluidité vidéo originale 24p   :"), mfrStatus))

	// 5. Ambient Experience
	ambCursor := "  "
	ambStyle := lipgloss.NewStyle().Foreground(cTextSub)
	ambStatus := lipgloss.NewStyle().Foreground(cTextMuted).Render("[ Actif (Standard) ]")
	if !m.tweakPower.AmbientExperienceEnabled {
		ambStatus = lipgloss.NewStyle().Foreground(cGreen).Render("[ DÉSACTIVÉ (Veille publicitaire coupée) ]")
	}
	if m.debloatCursor == 5 {
		ambCursor = "▶ "
		ambStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccentCyan)
	}
	tweakLines = append(tweakLines, fmt.Sprintf("%s%s %s", ambCursor, ambStyle.Render("Ambient Experience (Publicités):"), ambStatus))

	// 6. DoT DNS
	dnsCursor := "  "
	dnsStyle := lipgloss.NewStyle().Foreground(cTextSub)
	dnsStatus := lipgloss.NewStyle().Foreground(cAccentCyan).Render(fmt.Sprintf("[ %s ]", m.tweakDNS.ProviderName)) + lipgloss.NewStyle().Foreground(cTextMuted).Render("  (Entrée pour cycler)")
	if m.debloatCursor == 6 {
		dnsCursor = "▶ "
		dnsStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccentCyan)
	}
	tweakLines = append(tweakLines, fmt.Sprintf("%s%s %s", dnsCursor, dnsStyle.Render("Résolveur DNS Sécurisé DoT    :"), dnsStatus))

	tweaksBoxStyle := cardStyle
	if m.debloatCursor >= 2 {
		tweaksBoxStyle = cardActiveStyle
	}
	tweaksCard := tweaksBoxStyle.Width(w - 2).Render(lipgloss.JoinVertical(lipgloss.Left, tweakLines...))

	return lipgloss.JoinVertical(lipgloss.Left, cardsRow, "\n", tweaksCard)
}

// -------------------------------------------------------------
// Tab 3: Restauration (Rollback)
// -------------------------------------------------------------
func (m model) renderRollback(w int) string {
	if m.rollbackConfirm {
		// Active Confirmation Dialog
		lines := []string{
			lipgloss.NewStyle().Bold(true).Foreground(cAmber).Render("CONFIRMATION DE RESTAURATION D'USINE"),
			"",
			lipgloss.NewStyle().Foreground(cTextWhite).Render("Êtes-vous certain de vouloir rétablir la configuration d'usine Amazon ?"),
			lipgloss.NewStyle().Foreground(cTextSub).Render("Tous les paquets gelés seront réactivés et les réglages d'origine restaurés."),
			"",
			lipgloss.JoinHorizontal(lipgloss.Center,
				lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(cRed).
					Background(lipgloss.Color("#3B1219")).
					Padding(0, 2).
					Bold(true).
					Foreground(cTextWhite).
					Render("▶ Confirmer la restauration (o / Entrée)"),
				"    ",
				lipgloss.NewStyle().
					Border(lipgloss.RoundedBorder()).
					BorderForeground(cBorderDim).
					Padding(0, 2).
					Foreground(cTextSub).
					Render("Annuler (n / Esc)"),
			),
		}
		return cardActiveStyle.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
	}

	btnStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cRed).
		Background(lipgloss.Color("#2D1216")).
		Padding(0, 2).
		Bold(true).
		Foreground(cRed).
		Render("▶ Lancer la procédure de restauration d'usine (Entrée / r)")

	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(cTextWhite).Render("RESTAURATION DE LA CONFIGURATION D'ORIGINE"),
		"",
		lipgloss.NewStyle().Foreground(cTextSub).Render("WaterStick fonctionne sans altération irréversible. Cette opération restaure"),
		lipgloss.NewStyle().Foreground(cTextSub).Render("l'ensemble des réglages et applications à leur état d'usine Amazon :"),
		"",
		lipgloss.NewStyle().Foreground(cTextWhite).Render("  - Réactivation des paquets système désactivés sur user 0"),
		lipgloss.NewStyle().Foreground(cTextWhite).Render("  - Rétablissement de la lecture vidéo et audio automatique de l'accueil"),
		lipgloss.NewStyle().Foreground(cTextWhite).Render("  - Réinitialisation du DNS privé vers le résolveur Wi-Fi par défaut"),
		lipgloss.NewStyle().Foreground(cTextWhite).Render("  - Remise à 1.0x des vitesses de transition d'interface"),
		"",
		btnStyle,
	}

	return cardStyle.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// -------------------------------------------------------------
// Tab 4: Applications (Sideload)
// -------------------------------------------------------------
func (m model) renderSideload(w int) string {
	var lines []string

	selectedCount := 0
	for _, app := range m.apps {
		if m.selectedApps[app.ID] {
			selectedCount++
		}
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(cTextWhite).Render("CATALOGUE D'APPLICATIONS SIDELOAD")
	subtitle := lipgloss.NewStyle().Foreground(cTextMuted).Render(fmt.Sprintf("%d application(s) sélectionnée(s)", selectedCount))
	lines = append(lines, title+"  "+subtitle, "")

	header := lipgloss.NewStyle().
		Foreground(cTextMuted).
		Render(fmt.Sprintf("  %-4s %-24s %-42s %s", "SÉL", "APPLICATION", "DESCRIPTION", "ACTION"))
	lines = append(lines, header)
	lines = append(lines, lipgloss.NewStyle().Foreground(cBorderDim).Render(strings.Repeat("─", w-6)))

	for i, a := range m.apps {
		cursorGlyph := "  "
		nameStyle := lipgloss.NewStyle().Foreground(cTextSub)

		checkbox := "[ ] "
		if m.selectedApps[a.ID] {
			checkbox = lipgloss.NewStyle().Foreground(cAccentCyan).Render("[x] ")
		}

		actionLabel := "[ Installer ]"
		if a.Installed {
			actionLabel = "[ Réinstaller ]"
		}

		if i == m.cursor {
			cursorGlyph = "▶ "
			nameStyle = lipgloss.NewStyle().Bold(true).Foreground(cTextWhite)
			if a.Installed {
				actionLabel = lipgloss.NewStyle().Foreground(cAccentCyan).Render("[ Mettre à jour (Entrée) ]")
			} else {
				actionLabel = lipgloss.NewStyle().Foreground(cGreen).Render("[ Installer (Entrée) ]")
			}
		}

		desc := a.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}

		namePadded := fmt.Sprintf("%-24s", a.Name)
		descPadded := fmt.Sprintf("%-42s", desc)

		row := fmt.Sprintf("%s%s%s %s %s",
			cursorGlyph,
			checkbox,
			nameStyle.Render(namePadded),
			lipgloss.NewStyle().Foreground(cTextMuted).Render(descPadded),
			actionLabel,
		)
		lines = append(lines, row)
	}

	lines = append(lines, "")
	actionHint := "[Espace] Cocher/Décocher  •  [a] Tout cocher  •  [Entrée] Installer l'élément ciblé"
	if selectedCount > 0 {
		actionHint = lipgloss.NewStyle().Foreground(cAccentCyan).Render(
			fmt.Sprintf("▶ [Entrée] Installer les %d applications sélectionnées par lot", selectedCount),
		)
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(cTextMuted).Render("  "+actionHint))

	return cardStyle.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// -------------------------------------------------------------
// Tab 5: Connexion (Pairing & ADB)
// -------------------------------------------------------------
func (m model) renderConnect(w int) string {
	colW := (w - 4) / 2

	// Left Column: WI-FI Pairing Instructions
	guideLines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(cTextWhite).Render("PROCÉDURE DE JUMELAGE ADB WI-FI"),
		"",
		lipgloss.NewStyle().Foreground(cTextWhite).Render("1. Activer le menu développeur :"),
		lipgloss.NewStyle().Foreground(cTextMuted).Render("   Paramètres > Ma Fire TV > À propos."),
		lipgloss.NewStyle().Foreground(cTextMuted).Render("   Appuyer 7 fois sur le nom de l'appareil."),
		"",
		lipgloss.NewStyle().Foreground(cTextWhite).Render("2. Activer le débogage réseau :"),
		lipgloss.NewStyle().Foreground(cTextMuted).Render("   Options développeurs > Débogage ADB > OUI."),
		"",
		lipgloss.NewStyle().Foreground(cTextWhite).Render("3. Relever l'adresse IP locale :"),
		lipgloss.NewStyle().Foreground(cTextMuted).Render("   À propos > Réseau (ex: 192.168.1.50)."),
		"",
		lipgloss.NewStyle().Foreground(cTextWhite).Render("4. Autoriser la connexion :"),
		lipgloss.NewStyle().Foreground(cTextMuted).Render("   Cochez 'Toujours autoriser' sur la TV."),
	}

	guideCol := cardStyle.Width(colW).Render(lipgloss.JoinVertical(lipgloss.Left, guideLines...))

	// Right Column: Connection Inputs & Controls
	var inputLines []string

	if m.targetDevice != "" {
		inputLines = append(inputLines,
			lipgloss.NewStyle().Bold(true).Foreground(cTextWhite).Render("PÉRIPHÉRIQUE ACTUELLEMENT ASSOCIÉ"),
			"",
			lipgloss.NewStyle().Foreground(cGreen).Render(fmt.Sprintf("  ● %s (Connecté)", m.targetDevice)),
			"",
		)

		discoBtn := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cBorderDim).
			Padding(0, 2).
			Foreground(cRed)

		if m.connectFocus == 2 {
			discoBtn = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(cRed).
				Background(lipgloss.Color("#2D1216")).
				Padding(0, 2).
				Bold(true).
				Foreground(cRed)
		}
		inputLines = append(inputLines, discoBtn.Render("Déconnecter le périphérique (d)"), "", "")
	}

	inputLines = append(inputLines,
		lipgloss.NewStyle().Bold(true).Foreground(cTextWhite).Render("CONNEXION AU PÉRIPHÉRIQUE"),
		"",
		lipgloss.NewStyle().Foreground(cTextSub).Render("Entrez l'adresse IP locale de la Fire TV :"),
		"",
	)

	var ipBoxText string
	if m.connectFocus == 0 {
		cursorGlyph := "_"
		if (m.animFrame/6)%2 == 0 {
			cursorGlyph = " "
		}
		ipBoxText = fmt.Sprintf("> %s%s  [Saisie active — Échap pour sortir]", m.ipInput, cursorGlyph)
	} else {
		ipBoxText = fmt.Sprintf("  %s  (Appuyer sur ↑ pour modifier l'IP)", m.ipInput)
	}

	inputBorder := cBorderDim
	if m.connectFocus == 0 {
		inputBorder = cAccentCyan
	}

	ipBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(inputBorder).
		Padding(0, 1).
		Foreground(cAccentCyan).
		Render(ipBoxText)

	inputLines = append(inputLines, ipBox, "")
	inputLines = append(inputLines, lipgloss.NewStyle().Foreground(cAmber).Render("[!] Acceptez l'empreinte RSA sur votre téléviseur."), "")

	btnStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cBorderDim).
		Padding(0, 2).
		Foreground(cTextSub)

	if m.connectFocus == 1 {
		btnStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cAccentCyan).
			Background(lipgloss.Color("#162638")).
			Padding(0, 2).
			Bold(true).
			Foreground(cTextWhite)
	}

	inputLines = append(inputLines, btnStyle.Render("▶ Établir la liaison (Entrée)"))

	rightCardStyle := cardStyle
	if m.connectFocus == 0 || m.connectFocus == 1 {
		rightCardStyle = cardActiveStyle
	}
	inputCol := rightCardStyle.Width(colW).Render(lipgloss.JoinVertical(lipgloss.Left, inputLines...))

	return lipgloss.JoinHorizontal(lipgloss.Top, guideCol, "  ", inputCol)
}

// -------------------------------------------------------------
// Entry Point
// -------------------------------------------------------------
func main() {
	engine, err := waterstick.NewEngine("")
	if err != nil {
		fmt.Printf("Erreur d'initialisation du moteur : %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(initialModel(engine))
	if _, err := p.Run(); err != nil {
		fmt.Printf("Erreur d'exécution TUI : %v\n", err)
		os.Exit(1)
	}
}
