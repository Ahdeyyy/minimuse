package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestUnavailableUpdatesCannotStartJobs(t *testing.T) {
	a := testApp(t, 0)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Gap(12).Children(func() { a.updateControls(c) })
	}, 760, 580)
	for _, label := range []string{"Automatically check for updates", "Automatically download and install updates", "Check for updates"} {
		// No plugin is registered in tests. A disabled control must neither
		// mutate preferences nor call the plugin (which would panic).
		click(t, tt, label)
		tt.Frame()
	}
	if a.updates.checks || a.updates.downloads {
		t.Fatal("unavailable update controls changed preferences")
	}
	if !strings.Contains(strings.Join(tt.Texts(), " "), "Automatic updates are unavailable") {
		t.Fatal("missing explanation for unavailable updates")
	}
}

func TestAutomaticDownloadsRequireChecks(t *testing.T) {
	a := testApp(t, 0)
	a.updates.enabled = true
	a.updates.lastCheck = time.Date(2026, time.October, 31, 12, 30, 0, 0, time.Local)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Gap(12).Children(func() { a.updateControls(c) })
	}, 760, 580)
	click(t, tt, "Automatically download and install updates")
	if a.updates.downloads {
		t.Fatal("automatic downloads enabled while automatic checks are off")
	}
	if !tt.HasText("Last checked: 31 Oct 2026, 12:30") {
		t.Fatal("last successful check is missing")
	}
}
