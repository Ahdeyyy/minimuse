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
		w.SetMinimumSize(360, miniHeight)
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
	t := c.Theme()
	tr := a.current()
	ui.Column(c.Key("mini-player")).FillWidth().Grow(1).MinHeight(0).Padding(10, 12).Gap(6).Justify(ui.SpaceBetween).Children(func() {
		ui.Row(c).FillWidth().Height(46).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Image(c, a.coverFor(tr)).Size(46, 46).Shrink(0).Radius(7).Clip()
			ui.Column(c).Width(0).Grow(1).MinWidth(0).Gap(6).Children(func() {
				title, artist := "Nothing playing", ""
				if tr != nil {
					title, artist = tr.Title, tr.Artist
				}
				ui.Text(c, title).FontSize(13).Bold().SingleLine().Tooltip(title)
				ui.Text(c, artist).FontSize(11).TextColor(t.TextMuted).SingleLine()
			})
			iconButton(c, "expand", "Exit mini player", false).OnClick(func() { a.setMini(false) })
		})
		ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Width(0).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Center).Justify(ui.Center).Children(func() { a.playbackButtons(c) })
			a.volumeControl(c, 108)
		})
		a.seekControl(c)
		if a.notice != "" {
			ui.Text(c, a.notice).FillWidth().FontSize(10).TextColor(t.TextMuted).SingleLine().Tooltip(a.notice)
		}
	})
}
