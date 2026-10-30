package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *app) toasts(c *ui.Context) {
	if a.notice != "" {
		// Reuse one toast so progress is replaced by its result and bursts of
		// notifications cannot cover the compact player's controls.
		c.AddToast(ui.Toast{ID: "app-notice", Title: a.notice, Timeout: 1500 * time.Millisecond})
		a.notice = ""
	}
	t := c.Theme()
	w, _ := c.Size()
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(54, 12, 12, 12).Justify(ui.Start).AlignItems(ui.End).Gap(8)
		width, padding, lines := min(float32(520), w-24), float32(12), 3
		if a.mini {
			// Use the branding area in compact mode, keeping the playback,
			// volume, seek and native-style window controls accessible.
			viewport.Padding(4, 100, 0, 12)
			width, padding, lines = min(float32(520), w-112), 6, 1
		}
		for _, message := range toasts {
			toast := ui.ToastBase(c, message)
			toast.Root.Label("Notification toast").Row().Width(width).Padding(padding).Gap(10).AlignItems(ui.Center).
				Background(t.Surface).TextColor(t.Text).Border(1, t.Border).Radius(10).Shadow(0, 4, 16, 0, ui.RGBA(0, 0, 0, .2))
			toast.Root.Children(func() {
				ui.Text(c, message.Title).Width(0).Grow(1).MinWidth(0).FontSize(11).MaxLines(lines).Tooltip(message.Title)
				toast.CloseButton().Label("Dismiss message").Size(24, 24).Shrink(0).Radius(6).Children(func() {
					ui.Icon(c, icons["close"]).Size(14, 14)
				})
			})
		}
	})
}
