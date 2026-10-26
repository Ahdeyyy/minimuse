package main

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

func (a *app) titlebar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c.Key("titlebar")).FillWidth().Height(42).Shrink(0).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).FillHeight().Padding(0, 18).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Image(c, vinylLogo).Size(20, 20)
			ui.Text(c, "MiniMuse").FontSize(11).Bold().TextColor(t.Text)
			if !a.mini {
				ui.Text(c, "/  your music, at home").FontSize(10).TextColor(t.TextMuted)
			}
		})
		iconButton(c, "minimize", "Minimize window", false).Size(44, 42).Radius(0).OnClick(func() {
			if a.window != nil {
				a.window.Minimize()
			}
		})
		if !a.mini {
			name, label := "maximize", "Maximize window"
			if a.window != nil && a.window.IsMaximized() {
				name, label = "restore", "Restore window"
			}
			iconButton(c, name, label, false).Size(44, 42).Radius(0).OnClick(func() {
				if a.window != nil {
					a.window.ToggleMaximize()
				}
			})
		}
		b := iconButton(c, "close", "Close window", false).Size(44, 42).Radius(0)
		if b.Hovered() {
			b.Background(ui.Hex("#b6293d")).TextColor(ui.Hex("#ffffff"))
		}
		b.OnClick(func() {
			if a.window != nil {
				a.window.Close()
			}
		})
	})
}

func (a *app) openSong() {
	if a.current() == nil {
		return
	}
	a.songScreen = true
	if a.lyricsPath != a.prefs.Current {
		a.loadLyrics(false)
	}
	a.updatePulse()
}

func (a *app) songView(c *ui.Context, width float32) {
	t := c.Theme()
	tr := a.current()
	ui.Row(c).FillWidth().Height(66).Shrink(0).Padding(12, 20).Gap(10).AlignItems(ui.Center).Children(func() {
		iconButton(c, "back", "Back to library", false).OnClick(func() { a.songScreen = false; a.updatePulse() })
		ui.Text(c, "Playing now").FontSize(20).Bold().Grow(1)
		if tr != nil {
			iconButton(c, "heart", "Favorite current song", tr.Favorite).OnClick(func() { a.favorite(tr.Path) })
		}
	})
	if tr == nil {
		ui.Column(c).FillWidth().Grow(1).Center().Gap(16).Children(func() {
			ui.Image(c, vinylLogo).Size(100, 100)
			ui.Text(c, "Choose a song to start listening").FontSize(18).Bold()
			textButton(c, "back", "Browse music").OnClick(func() { a.songScreen = false; a.updatePulse() })
		})
		return
	}
	_, height := c.Size()
	artSize := min(float32(280), max(float32(140), min((width-260)*.36, height-390)))
	ui.Row(c).FillWidth().Grow(1).MinHeight(0).Padding(4, 24, 24, 24).Gap(28).Children(func() {
		ui.Scroll(c).Width(artSize).FillHeight().Children(func() {
			ui.Column(c).FillWidth().Gap(16).Children(func() {
				ui.Image(c, a.coverFor(tr)).Size(artSize, artSize).Radius(12).Clip().Label("Album cover art")
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					ui.Text(c, tr.Title).FontSize(23).Bold().FillWidth().LineHeight(1.25)
					ui.Text(c, tr.Artist).FontSize(13).TextColor(t.TextMuted).FillWidth()
					ui.Text(c, tr.Album).FontSize(12).TextColor(t.TextMuted).FillWidth()
				})
				quality := codec(tr.Path)
				if a.status.SampleRate > 0 {
					quality = fmt.Sprintf("%s · %.1f kHz · %d-bit", quality, float64(a.status.SampleRate)/1000, a.status.Bits)
				}
				ui.Text(c, quality).FontSize(10).TextColor(t.TextMuted).FillWidth()
			})
		})
		ui.Column(c.Key("lyrics-panel")).Width(0).Grow(1).MinWidth(0).FillHeight().Gap(12).Children(func() { a.lyricsView(c) })
	})
}

func (a *app) lyricsView(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Text(c, "Lyrics").FontSize(19).Bold().Grow(1)
		iconButton(c, "refresh", "Reload local lyrics", false).Disabled(a.lyricsBusy).OnClick(func() { a.loadLyrics(false) })
		textButton(c, "plus", "Get lyrics").Tooltip("Download from LRCLIB and save beside this song").Disabled(a.lyricsBusy).OnClick(func() { a.loadLyrics(true) })
	})
	if a.lyricsBusy {
		ui.Text(c, a.lyricsMessage).FontSize(12).TextColor(t.TextMuted).FillWidth()
	}
	if len(a.lyrics.Lines) == 0 {
		ui.Column(c).FillWidth().Grow(1).Justify(ui.Center).Gap(14).Children(func() {
			ui.Icon(c, icons["lyrics"]).Size(32, 32).TextColor(t.Accent)
			message := ""
			if !a.lyricsBusy {
				message = a.lyricsMessage
			}
			if message == "" {
				message = "Lyrics live here"
			}
			ui.Text(c, message).FontSize(17).Bold().FillWidth()
			ui.Text(c, "Place a .txt, .lrc, or enhanced .elrc file beside the song, using the same filename. Or use Get lyrics to download from LRCLIB.").FontSize(12).TextColor(t.TextMuted).FillWidth().LineHeight(1.6)
			ui.Text(c, "Word highlighting needs word timestamps in the lyric file.").FontSize(11).TextColor(t.TextMuted).FillWidth().LineHeight(1.5)
		})
		return
	}
	ui.Row(c).FillWidth().Gap(6).Children(func() {
		for _, mode := range []string{"Static", "Timed", "Words"} {
			b := textButton(c.Key(mode), "", mode).Padding(7, 10).FontSize(11).Border(0, ui.Transparent).Background(t.Background).TextColor(t.Text)
			if a.lyricMode == mode {
				b.Background(t.Selection).TextColor(t.Accent)
			}
			b.Disabled(mode == "Timed" && !a.lyrics.Timed || mode == "Words" && !a.lyrics.WordTimed).OnClick(func() { a.lyricMode = mode; a.lyricActive = -1; a.updatePulse() })
		}
	})
	active := a.lyrics.active(a.status.Position)
	if active != a.lyricActive {
		a.lyricActive = active
		if a.followLyrics && a.lyricMode != "Static" && active >= 0 {
			a.lyricList.ScrollTo(active, ui.Center)
		}
	}
	ui.List(c.Key(a.lyricsPath+"/"+a.lyricMode), &a.lyricList, len(a.lyrics.Lines)).FillWidth().Grow(1).MinHeight(0).Padding(8, 0).ItemKey(func(i int) any { return i }).Rows(func(row ui.ListRow) {
		c := row.Context
		line := a.lyrics.Lines[row.Index]
		isActive := a.lyricMode != "Static" && row.Index == active
		color := t.TextMuted
		if a.lyricMode == "Static" {
			color = t.Text
		}
		if isActive {
			color = t.Accent
		}
		var spans []ui.Span
		if isActive && a.lyricMode == "Words" && len(line.Words) > 0 {
			for i, w := range line.Words {
				span := ui.Span{Text: w.Text, Color: t.TextMuted}
				if w.At <= a.status.Position {
					span.Color = t.Text
					if i+1 == len(line.Words) || line.Words[i+1].At > a.status.Position {
						span.Color = t.Accent
						span.Background = t.Selection
						span.Radius = 3
					}
				}
				spans = append(spans, span)
			}
		} else {
			text := line.Text
			if text == "" {
				text = "♪"
			}
			spans = []ui.Span{{Text: text, Color: color}}
		}
		b := ui.Button(c, "").Label("Lyric line "+fmt.Sprint(row.Index+1)).FillWidth().Padding(12, 10).Border(0, ui.Transparent).Background(ui.Transparent).TextColor(color)
		if a.lyrics.Timed && a.lyricMode != "Static" {
			b.Tooltip("Seek to this line")
		}
		if isActive {
			b.Background(t.Background)
		}
		b.Children(func() { ui.RichText(c, spans...).FontSize(17).Bold().LineHeight(1.5).FillWidth() })
		b.OnClick(func() {
			if a.lyrics.Timed && a.lyricMode != "Static" && a.engine != nil {
				a.engine.Seek(line.At)
			}
		})
	})
	if a.lyrics.Timed && a.lyricMode != "Static" {
		ui.Checkbox(c, &a.followLyrics, "Follow playback").FontSize(11).OnChange(func() { a.lyricActive = -1 })
	}
	source := "Local lyrics"
	if a.lyrics.Source != "" {
		source = "Local · " + filepath.Base(a.lyrics.Source)
	}
	ui.Text(c, source).FontSize(10).TextColor(t.TextMuted).FillWidth().SingleLine().Tooltip(a.lyrics.Source)
}
