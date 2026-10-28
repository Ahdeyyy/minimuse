package main

import (
	"github.com/egoist/mygo/ui"
)

const miniWidth, miniHeight = 420, 210

// Mode switches reuse the same window and engine, so playback and queues do
// not restart, and no second UI runtime or polling loop remains resident.
func (a *app) setMini(enabled bool) {
	if a.mini == enabled {
		return
	}
	a.mini = enabled
	a.updatePulse()
	a.settings = false
	a.playlistDialog = false
	a.miniVolumeOpen = false
	if a.window == nil {
		return
	}
	w := a.window
	if enabled {
		a.mainMaximized = w.IsMaximized()
		if a.mainMaximized {
			w.Unmaximize()
		}
		a.mainBounds = w.Bounds()
		w.SetMinimumSize(260, 180)
		w.SetMaximizable(false)
		w.SetAlwaysOnTop(true)
		if a.miniBounds.Width > 0 {
			w.SetBounds(a.miniBounds)
		} else {
			w.SetContentSize(miniWidth, miniHeight)
		}
		w.SetTitle("MiniMuse — Mini player")
	} else {
		a.miniBounds = w.Bounds()
		w.SetAlwaysOnTop(false)
		w.SetMaximizable(true)
		w.SetMinimumSize(820, 580)
		if a.mainBounds.Width > 0 {
			w.SetBounds(a.mainBounds)
		} else {
			w.SetSize(1080, 740)
		}
		if a.mainMaximized {
			w.Maximize()
		}
		w.SetTitle("MiniMuse")
	}
	w.Invalidate()
}

func (a *app) miniView(c *ui.Context) {
	tr := a.current()
	w, h := c.Size()
	wide := w >= 640
	spacious := h >= 340
	ui.Column(c.Key("mini-player")).FillWidth().Grow(1).MinHeight(0).Padding(8, 12).Clip().Children(func() {
		ui.Column(c.Key("mini-stage")).FillWidth().MaxWidth(900).AlignSelf(ui.Center).Grow(1).MinHeight(0).Gap(8).Justify(ui.Center).Children(func() {
			if spacious {
				ui.Row(c).FillWidth().Justify(ui.End).Children(func() { a.miniExpand(c) })
				ui.Row(c).FillWidth().Grow(1).MinHeight(0).Gap(28).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					if wide {
						size := min(float32(320), (w-100)*.4, h-170)
						ui.Image(c, a.coverFor(tr)).Size(size, size).Radius(14).Clip().Shrink(0).Label("Mini album art")
						ui.Column(c).Width(0).Grow(1).MinWidth(0).MaxWidth(400).Gap(18).Children(func() {
							a.miniMetadata(c, tr, true)
							a.miniControls(c, true)
							a.seekControl(c)
							ui.Row(c).FillWidth().Justify(ui.Center).Children(func() { a.volumeControl(c, 160) })
						})
					} else {
						ui.Column(c).Fill().Gap(12).Justify(ui.Center).Children(func() {
							size := min(float32(300), w-64, h-280)
							ui.Image(c, a.coverFor(tr)).Size(size, size).Radius(12).Clip().AlignSelf(ui.Center).Label("Mini album art")
							a.miniMetadata(c, tr, true)
							a.miniControls(c, w >= 360)
							a.seekControl(c)
							ui.Row(c).FillWidth().Justify(ui.Center).Children(func() { a.volumeControl(c, min(float32(160), w-48)) })
						})
					}
				})
			} else if wide {
				ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Image(c, a.coverFor(tr)).Size(48, 48).Radius(8).Clip().Shrink(0).Label("Mini album art")
					ui.Column(c).Width(0).Grow(1).MinWidth(0).Children(func() { a.miniMetadata(c, tr, false) })
					a.miniControls(c, true)
					if w >= 820 {
						a.volumeControl(c, 120)
					} else {
						a.miniVolume(c)
					}
					a.miniExpand(c)
				})
				a.seekControl(c)
			} else {
				ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
					size := float32(46)
					if h < 200 {
						size = 36
					}
					ui.Image(c, a.coverFor(tr)).Size(size, size).Shrink(0).Radius(7).Clip().Label("Mini album art")
					ui.Column(c).Width(0).Grow(1).MinWidth(0).Children(func() { a.miniMetadata(c, tr, false) })
					a.miniExpand(c)
				})
				ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Width(0).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Center).Justify(ui.Center).Children(func() { a.miniControls(c, w >= 360) })
					if w >= 360 {
						a.volumeControl(c, 104)
					} else {
						a.miniVolume(c)
					}
				})
				a.seekControl(c)
			}
		})
	})
}

func (a *app) miniMetadata(c *ui.Context, tr *track, large bool) {
	title, artist := "Nothing playing", "Add music in the library"
	if tr != nil {
		title, artist = tr.Title, tr.Artist
	}
	size := float32(13)
	if large {
		size = 20
	}
	ui.Column(c).FillWidth().MinWidth(0).Gap(5).Children(func() {
		ui.Text(c, title).FillWidth().FontSize(size).Bold().SingleLine().Tooltip(title).Label("Mini song title")
		ui.Text(c, artist).FillWidth().FontSize(11).TextColor(c.Theme().TextMuted).SingleLine().Tooltip(artist)
	})
}

func (a *app) miniExpand(c *ui.Context) {
	iconButton(c, "expand", "Exit mini player", false).OnClick(func() { a.setMini(false) })
}

func (a *app) miniControls(c *ui.Context, extended bool) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		a.playbackControls(c, extended)
		if !extended {
			iconButton(c, "more", "Playback options", false).Menu(func(m *ui.Menu) {
				if m.Item("Shuffle").Checked(a.prefs.Shuffle).Chosen() {
					a.prefs.Shuffle = !a.prefs.Shuffle
					a.bag = nil
					a.history = nil
					a.persist()
				}
				for i, label := range []string{"Repeat off", "Repeat all", "Repeat one"} {
					if m.Item(label).Checked(a.prefs.Repeat == i).Chosen() {
						a.prefs.Repeat = i
						a.persist()
					}
				}
			})
		}
	})
}

func (a *app) miniVolume(c *ui.Context) {
	name := "volume"
	if a.muted || a.prefs.Volume == 0 {
		name = "mute"
	}
	b := iconButton(c, name, "Volume controls", a.miniVolumeOpen).OnClick(func() { a.miniVolumeOpen = !a.miniVolumeOpen })
	ui.Popover(c.Key("mini-volume"), b, &a.miniVolumeOpen, func() { a.volumeControl(c, 200) }).Padding(12).Background(c.Theme().Surface)
}
