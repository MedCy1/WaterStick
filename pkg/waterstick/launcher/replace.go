package launcher

import (
	"context"
	"fmt"
	"strings"
)

// Components involved in swapping the Fire OS home screen.
//
// Findings from the decompiled com.amazon.firehomestarter (research/decompiled/firehomestarter):
//   - HomeStarterActivity holds the HOME filter at priority 1. Fire OS's PackageManager returns the
//     highest-priority HOME candidate before consulting the user's preferred home, and it clamps
//     every non-system app's intent-filter priority to 0, so a sideloaded launcher can never win
//     while firehomestarter is enabled.
//   - HomeActivityResolver skips every MAIN_HOME activity whose priority is <= 0, then
//     EnableComponentStartStrategy re-enables com.amazon.tv.launcher's HomeActivity_vNext.
//     If that launcher is disabled, HomeStrategyExecutor shows a spinner and retries every
//     5 s forever: the "black screen / bootloop" seen when the stock launcher is disabled alone.
//
// The safe swap therefore disables firehomestarter AND the stock launcher (both user-space,
// reversible with `pm enable`), then pins WaterOS as the home activity. With both gone the HOME
// candidates are WaterOS (priority 0) and Settings' FallbackHome (priority -1000), so WaterOS
// wins on every Home press and at boot.
const (
	WaterOSPackage = "com.medcy1.wateros.launcher"
	WaterOSHome    = WaterOSPackage + "/.MainActivity"
	StockLauncher  = "com.amazon.tv.launcher"
	StockHome      = StockLauncher + "/.ui.HomeActivity_vNext"
	HomeStarter    = "com.amazon.firehomestarter"

	resolveHomeCmd = "cmd package resolve-activity --brief -a android.intent.action.MAIN -c android.intent.category.HOME"
)

// HomeStatus describes who owns the Home button.
type HomeStatus struct {
	WaterOSInstalled bool   `json:"wateros_installed"`
	StockDisabled    bool   `json:"stock_disabled"`
	StarterDisabled  bool   `json:"starter_disabled"`
	HomeActivity     string `json:"home_activity"`
	WaterOSIsHome    bool   `json:"wateros_is_home"`
}

// replaceSteps is the swap sequence. Order matters: the preferred home is recorded against the
// current candidate set, so it must be set after both disables or Android discards it.
func replaceSteps() []string {
	return []string{
		"pm disable-user --user 0 " + HomeStarter,
		"pm disable-user --user 0 " + StockLauncher,
		"cmd package set-home-activity " + WaterOSHome,
	}
}

// restoreSteps undoes replaceSteps: re-enable first, then hand Home back to Amazon.
func restoreSteps() []string {
	return []string{
		"pm enable " + StockLauncher,
		"pm enable " + HomeStarter,
		"cmd package set-home-activity " + StockHome,
	}
}

// homeComponent extracts the resolved component from `resolve-activity --brief` output
// (its last non-empty line, e.g. "com.medcy1.wateros.launcher/.MainActivity").
func homeComponent(out string) string {
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(out, "\r", "")), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// HomeStatus reports the current home owner and package states.
func (c *Cleaner) HomeStatus(ctx context.Context, serial string) (*HomeStatus, error) {
	st := &HomeStatus{}
	if out, err := c.client.Shell(ctx, serial, "pm path "+WaterOSPackage); err == nil && strings.Contains(out, "package:") {
		st.WaterOSInstalled = true
	}
	disabled, err := c.pkgMgr.ListDisabled(ctx, serial)
	if err != nil {
		return nil, err
	}
	st.StockDisabled = disabled[StockLauncher]
	st.StarterDisabled = disabled[HomeStarter]
	out, err := c.client.Shell(ctx, serial, resolveHomeCmd)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve home activity: %w", err)
	}
	st.HomeActivity = homeComponent(out)
	st.WaterOSIsHome = st.HomeActivity == WaterOSHome
	return st, nil
}

// ReplaceWithWaterOS makes WaterOS the only home screen and disables the stock launcher so it
// uses no RAM. Nothing is uninstalled; RestoreStockHome reverts every step. If Home does not
// resolve to WaterOS afterwards, the stock home is restored before returning the error.
func (c *Cleaner) ReplaceWithWaterOS(ctx context.Context, serial string) (*HomeStatus, error) {
	if out, err := c.client.Shell(ctx, serial, "pm path "+WaterOSPackage); err != nil || !strings.Contains(out, "package:") {
		return nil, fmt.Errorf("WaterOS Launcher is not installed: run 'waterstick install wateros-launcher' first")
	}
	for _, step := range replaceSteps() {
		out, err := c.client.Shell(ctx, serial, step)
		if err != nil || strings.Contains(strings.ToLower(out), "error") || strings.Contains(out, "Exception") {
			_, _ = c.RestoreStockHome(ctx, serial)
			return nil, fmt.Errorf("step %q failed (%s), stock home restored", step, strings.TrimSpace(out))
		}
	}
	st, err := c.HomeStatus(ctx, serial)
	if err != nil || !st.WaterOSIsHome {
		_, _ = c.RestoreStockHome(ctx, serial)
		got := ""
		if st != nil {
			got = st.HomeActivity
		}
		return nil, fmt.Errorf("home resolved to %q instead of WaterOS, stock home restored", got)
	}
	_, _ = c.client.Shell(ctx, serial, "input keyevent KEYCODE_HOME")
	return st, nil
}

// RestoreStockHome re-enables the Amazon launcher and its home starter and gives Home back to it.
func (c *Cleaner) RestoreStockHome(ctx context.Context, serial string) (*HomeStatus, error) {
	var errs []string
	for _, step := range restoreSteps() {
		if out, err := c.client.Shell(ctx, serial, step); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %s", step, strings.TrimSpace(out)))
		}
	}
	_, _ = c.client.Shell(ctx, serial, "input keyevent KEYCODE_HOME")
	st, err := c.HomeStatus(ctx, serial)
	if len(errs) > 0 {
		return st, fmt.Errorf("restore incomplete: %s", strings.Join(errs, "; "))
	}
	return st, err
}
