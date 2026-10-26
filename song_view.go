package main

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *app) titlebar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c.Key("titlebar")).FillWidth().Height(42).Shrink(0).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).FillHeight().Padding(0, 18).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
			ui.Image(c, vinylLogo).Size(20, 20)
			ui.Text(c, "MiniMuse").FontSize(11).Bold().TextColor(t.Text)
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
		ui.Scroll(c).Width(artSize+16).Padding(0, 16, 0, 0).FillHeight().Children(func() {
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
		ui.Row(c).Gap(5).AlignItems(ui.Start).Children(func() {
			ui.Text(c, "Lyrics").FontSize(19).Bold()
			if len(a.lyrics.Lines) > 0 && a.lyrics.Source != "" {
				ui.Text(c, "Local").FontSize(9).Bold().Padding(3, 6).Radius(5).Background(t.Selection).TextColor(t.Accent).Tooltip(a.lyrics.Source)
			}
		})
		ui.Spacer(c).Grow(1)
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
	ui.Row(c).Label("Lyric controls").FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Row(c.Key("lyric-modes")).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
			for _, mode := range []string{"Static", "Timed", "Words"} {
				b := textButton(c.Key(mode), "", mode).Padding(7, 8).FontSize(11).Border(0, ui.Transparent).Background(t.Background).TextColor(t.Text)
				if a.lyricMode == mode {
					b.Background(t.Selection).TextColor(t.Accent)
				}
				b.Disabled(mode == "Timed" && !a.lyrics.Timed || mode == "Words" && !a.lyrics.WordTimed).OnClick(func() { a.lyricMode = mode; a.lyricActive = -1; a.updatePulse() })
			}
			if a.lyrics.Timed {
				a.lyricTimingControl(c)
			}
		})
		ui.Spacer(c).Grow(1)
		if a.lyrics.Timed && a.lyricMode != "Static" {
			width, _ := c.Size()
			if width < 1000 {
				iconButton(c, "follow", "Follow playback", a.followLyrics).Size(28, 28).OnClick(func() { a.followLyrics = !a.followLyrics; a.lyricActive = -1 })
			} else {
				ui.Checkbox(c, &a.followLyrics, "Follow playback").Shrink(0).FontSize(10).OnChange(func() { a.lyricActive = -1 })
			}
		}
	})
	position := a.lyricPosition()
	active := a.lyrics.active(position)
	if active != a.lyricActive {
		a.lyricActive = active
		if a.followLyrics && a.lyricMode != "Static" && active >= 0 {
			a.lyricList.ScrollTo(active, ui.Center)
		}
	}
	ui.List(c.Key(a.lyricsPath+"/"+a.lyricMode), &a.lyricList, len(a.lyrics.Lines)).FillWidth().Grow(1).MinHeight(0).Padding(16, 16, 20, 0).ItemKey(func(i int) any { return i }).Rows(func(row ui.ListRow) {
		c := row.Context
		line := a.lyrics.Lines[row.Index]
		isActive := a.lyricMode != "Static" && row.Index == active
		color := t.Text
		fontSize := float32(16)
		pose := ui.Pose{}
		if a.lyricMode != "Static" {
			// Keep layout stable; visual emphasis animates without resizing rows.
			fontSize = 23
			distance := math.Abs(float64(row.Index - max(0, active)))
			strength := float32(math.Exp(-.32 * distance))
			pose = ui.Pose{Scale: .52 + .48*strength, Fade: .6 * (1 - strength)}
			color = t.Text.Mix(t.TextMuted, float32(1-math.Exp(-.85*distance)))
		}
		if isActive {
			color = t.Text
			pose = ui.Pose{X: 6, Scale: 1}
		}
		b := ui.Button(c, "").Label("Lyric line "+fmt.Sprint(row.Index+1)).FillWidth().Padding(14, 4).Border(0, ui.Transparent).Background(ui.Transparent).TextColor(color)
		if a.lyrics.Timed && a.lyricMode != "Static" {
			b.Tooltip("Seek to this line")
		}
		b.Children(func() {
			text := ui.RichText(c).FontSize(fontSize).LineHeight(1.5).FillWidth().TransformOrigin(0, .5).AnimatePose(pose, ui.Smooth)
			text.TextColor(text.AnimateColor("line-color", color, ui.Animation{Duration: 220 * time.Millisecond}))
			if a.lyricMode != "Static" {
				text.Bold()
			}
			text.Children(func() {
				if a.lyricMode == "Words" && len(line.Words) > 0 {
					for i, w := range line.Words {
						wordColor := color
						wordPose := ui.Pose{}
						if isActive {
							wordColor = t.TextMuted
						}
						if isActive && w.At <= position {
							wordColor = t.Text
							if i+1 == len(line.Words) || line.Words[i+1].At > position {
								wordColor = t.Accent
								wordPose = ui.Pose{Y: -3, Scale: 1.025}
							}
						}
						word := ui.Text(c.Key(i), w.Text).AnimatePose(wordPose, ui.Snappy)
						word.TextColor(word.AnimateColor("word-color", wordColor, ui.Animation{Duration: 150 * time.Millisecond}))
					}
				} else {
					value := line.Text
					if value == "" {
						value = "♪"
					}
					ui.Text(c, value)
				}
			})
		})
		b.OnClick(func() {
			if a.lyrics.Timed && a.lyricMode != "Static" && a.engine != nil {
				a.engine.Seek(a.lyricSeekTime(line.At))
			}
		})
	})
}

func (a *app) lyricTimingControl(c *ui.Context) {
	tr := a.current()
	if tr == nil {
		return
	}
	b := textButton(c.Key("lyric-timing"), "", "Timing").Padding(7, 10).FontSize(11).Tooltip("Adjust lyric timing for this song").OnClick(func() { a.lyricTimingOpen = !a.lyricTimingOpen })
	ui.Popover(c.Key("lyric-timing-panel"), b, &a.lyricTimingOpen, func() {
		ui.Column(c).Width(260).Gap(12).Children(func() {
			ui.Text(c, "Lyric timing").FontSize(15).Bold()
			ui.Text(c, "Offset in milliseconds").FontSize(11).TextColor(c.Theme().TextMuted)
			ui.NumberInput(c, &tr.LyricOffsetMS, -300000, 300000, 100).FillWidth().FontSize(12).Label("Lyric offset (ms)").OnChange(a.lyricTimingChanged)
			ui.Row(c).FillWidth().Gap(6).Children(func() {
				textButton(c, "", "Earlier").Grow(1).Tooltip("Show lyrics 100 ms earlier").OnClick(func() { tr.LyricOffsetMS -= 100; a.lyricTimingChanged() })
				textButton(c, "", "Later").Grow(1).Tooltip("Show lyrics 100 ms later").OnClick(func() { tr.LyricOffsetMS += 100; a.lyricTimingChanged() })
				textButton(c, "", "Reset").Disabled(tr.LyricOffsetMS == 0).OnClick(func() { tr.LyricOffsetMS = 0; a.lyricTimingChanged() })
			})
			ui.Text(c, "Negative: earlier · Positive: later\nSaved for this song. Applies to lines and words.").FillWidth().FontSize(11).LineHeight(1.5).TextColor(c.Theme().TextMuted)
		})
	}).Padding(16).Background(c.Theme().Surface)
}
