package main

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"
)

// Snapshot plugin preferences on the main thread; rendering does no disk I/O.
type updateSettings struct {
	enabled   bool
	checks    bool
	downloads bool
	lastCheck time.Time
}

func (a *app) setupUpdates() {
	a.updates.enabled = mygo.Updater.Enabled()
	refresh := func() {
		if a.closed.Load() {
			return
		}
		a.updates.checks = updater.AutomaticChecks()
		a.updates.downloads = updater.AutomaticDownloads()
		a.updates.lastCheck = updater.LastCheck()
		a.window.Invalidate()
	}
	refresh()
	off := updater.OnChange(refresh)
	a.window.OnClosed(off)
}

func (a *app) updateControls(c *ui.Context) {
	ui.Text(c, "Updates").Bold()
	version := mygo.App.Version()
	if version == "" {
		version = "development"
	}
	ui.Text(c, "MiniMuse "+version).FontSize(12).TextColor(c.Theme().TextMuted)
	ui.Checkbox(c.Key("automatic-update-checks"), &a.updates.checks, "Automatically check for updates").Disabled(!a.updates.enabled).OnChange(func() {
		updater.SetAutomaticChecks(a.updates.checks)
	})
	ui.Checkbox(c.Key("automatic-update-downloads"), &a.updates.downloads, "Automatically download and install updates").Disabled(!a.updates.enabled || !a.updates.checks).OnChange(func() {
		updater.SetAutomaticDownloads(a.updates.downloads)
	})
	message := "Checks daily. Updates are verified before installation and take effect the next time MiniMuse starts."
	if !a.updates.enabled {
		message = "Automatic updates are unavailable for this build or install location. Use a signed release installed in a writable location; package manager installs should be updated through the package manager."
	}
	ui.Text(c, message).FillWidth().FontSize(12).TextColor(c.Theme().TextMuted).LineHeight(1.6)
	if !a.updates.lastCheck.IsZero() {
		ui.Text(c, "Last checked: "+a.updates.lastCheck.Local().Format("2 Jan 2006, 15:04")).FontSize(12).TextColor(c.Theme().TextMuted)
	}
	textButton(c, "refresh", "Check for updates").Disabled(!a.updates.enabled).OnClick(updater.CheckForUpdates)
}
