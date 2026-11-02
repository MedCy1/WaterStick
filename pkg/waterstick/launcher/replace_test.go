package launcher

import (
	"strings"
	"testing"
)

// Android records the preferred home against the candidate set at the time it is set, so the
// set-home step must come after the disables (replace) and after the enables (restore).
func TestSwapOrder(t *testing.T) {
	r := replaceSteps()
	if !strings.Contains(r[len(r)-1], "set-home-activity "+WaterOSHome) {
		t.Fatalf("replace must end with set-home-activity WaterOS, got %v", r)
	}
	for _, pkg := range []string{HomeStarter, StockLauncher} {
		found := false
		for _, s := range r[:len(r)-1] {
			found = found || s == "pm disable-user --user 0 "+pkg
		}
		if !found {
			t.Errorf("replace must disable %s before set-home-activity: %v", pkg, r)
		}
	}
	s := restoreSteps()
	if !strings.Contains(s[len(s)-1], "set-home-activity "+StockHome) || s[0] != "pm enable "+StockLauncher {
		t.Fatalf("restore must enable the stock launcher first and set it home last, got %v", s)
	}
}

func TestHomeComponent(t *testing.T) {
	out := "priority=0 preferredOrder=0 match=0x108000 specificIndex=-1 isDefault=true\r\ncom.medcy1.wateros.launcher/.MainActivity\r\n"
	if got := homeComponent(out); got != WaterOSHome {
		t.Fatalf("got %q", got)
	}
}
