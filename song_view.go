package main

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"
	"github.com/go-text/typesetting/segmenter"
)

// Non-bouncing springs retain velocity when a new line or word arrives
// before the previous transition has settled.
var lyricLineMotion = ui.Animation{Spring: ui.SpringOptions{Duration: 650 * time.Millisecond}}
var lyricWordMotion = ui.Animation{Spring: ui.SpringOptions{Duration: 280 * time.Millisecond}}

type lyricRowLabels struct {
	line   string
	words  []string
	fills  []lyricWordFill
	ranges []ui.TextInputRange
	glow   lyricGlow
}

// Keep span boundaries fixed throughout playback so fill updates only change
// paint, not shaping, wrapping, hit areas, or accessibility word identity.
type lyricWordFill struct {
	spans []ui.Span
	sung  int
}

func newLyricWordFill(text string) lyricWordFill {
	var seg segmenter.Segmenter
	seg.Init([]rune(text))
	iter := seg.GraphemeIterator()
	var fill lyricWordFill
	for iter.Next() {
		cluster := string(iter.Grapheme().Text)
		fill.spans = append(fill.spans, ui.Span{Text: cluster})
		if strings.TrimSpace(cluster) != "" {
			fill.sung++
		}
	}
	return fill
}

func (fill *lyricWordFill) paint(progress float32, base, accent ui.Color) []ui.Span {
	remaining := progress * float32(fill.sung)
	for i := range fill.spans {
		amount := float32(0)
		if strings.TrimSpace(fill.spans[i].Text) != "" {
			amount = max(0, min(1, remaining))
			remaining--
		}
		fill.spans[i].Color = base.Mix(accent, amount)
	}
	return fill.spans
}

func (a *app) stopLyricScroll() {
	s := a.lyricScroll
	a.lyricScroll = ui.ScrollState{Handle: s.Handle, X: s.X, Y: s.Y, MaxX: s.MaxX, MaxY: s.MaxY}
}

func (a *app) lyricFollowChanged() {
	a.lyricActive = -1
	if !a.followLyrics || a.lyricMode == "Static" {
		a.stopLyricScroll()
	}
}

func (a *app) scrollToLyric(c *ui.Context, active int) {
	if active < 0 || active >= len(a.lyricRows) {
		return
	}
	viewport := a.lyricScroll.Bounds(c)
	line := a.lyricRows[active].Bounds(c)
	first, last := a.lyricList.Visible()
	if viewport.H > 0 && line.H > 0 && active >= first && active <= last {
		// Measure the actual wrapped row, so long lines glide to the
		// same center as short ones without changing their layout.
		y := a.lyricScroll.Y + line.Y + line.H/2 - viewport.Y - viewport.H/2
		a.lyricScroll.ScrollTo(0, y, lyricLineMotion)
	} else {
		// Distant jumps place the passage immediately instead of
		// animating through unrelated verses.
		a.stopLyricScroll()
		a.lyricList.ScrollTo(active, ui.Center)
	}
}

func (a *app) titlebar(c *ui.Context) {
	t := c.Theme()
	width, _ := c.Size()
	ui.Row(c.Key("titlebar")).Label("Window title bar").FillWidth().Height(42).Shrink(0).AlignItems(ui.Center).Children(func() {
		brand := ui.Row(c.Key("titlebar-brand")).Grow(1).FillHeight().Padding(0, 18).Gap(8).AlignItems(ui.Center).DragWindow()
		if a.searchVisible() {
			brand.Width(210).Grow(0).Shrink(0)
		}
		brand.Children(func() {
			ui.Image(c, vinylLogo).Size(20, 20)
			ui.Text(c, "MiniMuse").FontSize(11).Bold().TextColor(t.Text)
		})
		if a.searchVisible() {
			placeholder := "Search songs, artists, albums…"
			if a.section == "Albums" {
				placeholder = "Search albums or artists…"
				if _, ok := a.albumIndex[a.albumKey]; ok {
					placeholder = "Search songs or artists in this album…"
				}
			}
			ui.Row(c.Key("titlebar-search")).Width(min(float32(560), width-374)).Height(30).Padding(0, 10).Gap(8).AlignItems(ui.Center).Background(t.Surface).Border(1, t.Border).Radius(8).Children(func() {
				ui.Icon(c, icons["search"]).Size(15, 15).TextColor(t.TextMuted)
				ui.TextInput(c.Key("search"), &a.query).Bind(&a.searchHandle).Label("Search music").Placeholder(placeholder).Width(0).Grow(1).MinWidth(0).Height(28).Padding(0).FontSize(11).Background(ui.Transparent).Border(0, ui.Transparent).OnChange(a.refreshFilter)
			})
			ui.Box(c.Key("titlebar-drag-space")).Grow(1).FillHeight().DragWindow()
		}
		iconButton(c.Key("minimize"), "minimize", "Minimize window", false).Size(44, 42).Radius(0).OnClick(func() {
			if a.window != nil {
				a.window.Minimize()
			}
		})
		if !a.mini {
			name, label := "maximize", "Maximize window"
			if a.window != nil && a.window.IsMaximized() {
				name, label = "restore", "Restore window"
			}
			iconButton(c.Key("maximize"), name, label, false).Size(44, 42).Radius(0).OnClick(func() {
				if a.window != nil {
					a.window.ToggleMaximize()
				}
			})
		}
		b := iconButton(c.Key("close"), "close", "Close window", false).Size(44, 42).Radius(0)
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

func (a *app) songBackdrop(c *ui.Context, panel ui.Element) {
	tr := a.current()
	if tr == nil {
		return
	}
	a.coverFor(tr)
	art := a.art[pathKey(tr.Path)]
	if art == nil || art.backdrop == nil {
		return
	}
	backdrop := art.backdrop
	t := c.Theme()
	opacity := float32(.86)
	if t.Dark {
		opacity = .80
	}
	veil := t.Surface.Alpha(opacity)
	panel.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Clip(r, 12, func() {
			p.Image(backdrop, r, ui.Cover)
			p.Fill(r, veil, 0)
		})
	})
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
	if len(a.lyrics.Lines) == 0 {
		ui.Column(c).FillWidth().Grow(1).Justify(ui.Center).Gap(14).Children(func() {
			ui.Icon(c, icons["lyrics"]).Size(32, 32).TextColor(t.Accent)
			ui.Text(c, "Lyrics live here").FontSize(17).Bold().FillWidth()
			ui.Text(c, "Place a .txt, .lrc, or enhanced .elrc file beside the song, using the same filename. Or use Get lyrics to download from LRCLIB.").FontSize(12).TextColor(t.TextMuted).FillWidth().LineHeight(1.6)
			ui.Text(c, "Word highlighting needs word timestamps in the lyric file.").FontSize(11).TextColor(t.TextMuted).FillWidth().LineHeight(1.5)
		})
		return
	}
	position := a.lyricPosition()
	active := a.lyrics.active(position)
	ui.Row(c).Label("Lyric controls").FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Row(c.Key("lyric-modes")).Shrink(0).Gap(3).AlignItems(ui.Center).Children(func() {
			for _, mode := range []string{"Static", "Timed", "Words"} {
				b := textButton(c.Key(mode), "", mode).Padding(7, 6).FontSize(11).Border(0, ui.Transparent).Background(t.Background).TextColor(t.Text)
				if a.lyricMode == mode {
					b.Background(t.Selection).TextColor(t.Accent)
				}
				b.Disabled(mode == "Timed" && !a.lyrics.Timed || mode == "Words" && !a.lyrics.WordTimed).OnClick(func() { a.lyricMode = mode; a.lyricFollowChanged(); a.updatePulse() })
			}
			if a.lyrics.Timed {
				a.lyricTimingControl(c)
			}
		})
		ui.Spacer(c).Grow(1)
		if a.lyrics.Timed && a.lyricMode != "Static" {
			iconButton(c.Key("current-lyric"), "locate", "Return to current lyric", false).Size(28, 28).Disabled(active < 0).OnClick(func() {
				a.scrollToLyric(c, a.lyrics.active(a.lyricPosition()))
			})
			width, _ := c.Size()
			if width < 1000 {
				iconButton(c, "follow", "Follow playback", a.followLyrics).Size(28, 28).OnClick(func() { a.followLyrics = !a.followLyrics; a.lyricFollowChanged() })
			} else {
				ui.Checkbox(c, &a.followLyrics, "Follow playback").Shrink(0).FontSize(10).OnChange(a.lyricFollowChanged)
			}
		}
	})
	if len(a.lyricRows) != len(a.lyrics.Lines) {
		a.lyricRows = make([]ui.Handle, len(a.lyrics.Lines))
		a.lyricLabels = make([]lyricRowLabels, len(a.lyrics.Lines))
		for i, line := range a.lyrics.Lines {
			labels := &a.lyricLabels[i]
			labels.line = fmt.Sprintf("Lyric line %d", i+1)
			labels.words = make([]string, len(line.Words))
			labels.fills = make([]lyricWordFill, len(line.Words))
			labels.ranges = make([]ui.TextInputRange, len(line.Words))
			labels.glow.text = line.Text
			offset := 0
			for j := range line.Words {
				labels.words[j] = fmt.Sprintf("Lyric line %d word %d", i+1, j+1)
				labels.fills[j] = newLyricWordFill(line.Words[j].Text)
				end := offset + len(utf16.Encode([]rune(line.Words[j].Text)))
				labels.ranges[j] = ui.TextInputRange{Start: offset, End: end}
				offset = end
			}
		}
	}
	if active != a.lyricActive {
		a.lyricActive = active
		if a.followLyrics && a.lyricMode != "Static" && active >= 0 {
			a.scrollToLyric(c, active)
		}
	}
	ui.List(c.Key(a.lyricsPath+"/"+a.lyricMode), &a.lyricList, len(a.lyrics.Lines)).TrackScroll(&a.lyricScroll).Label("Lyric lines").FillWidth().Grow(1).MinHeight(0).Padding(16, 16, 20, 0).ItemKey(func(i int) any { return i }).Rows(func(row ui.ListRow) {
		c := row.Context
		line := a.lyrics.Lines[row.Index]
		isActive := a.lyricMode != "Static" && line.singing(position)
		color := t.Text
		fontSize := float32(16)
		pose := ui.Pose{}
		if a.lyricMode != "Static" {
			// Keep layout stable; visual emphasis animates without resizing rows.
			fontSize = 23
			distance := math.Abs(float64(row.Index - max(0, active)))
			strength := float32(math.Exp(-.32 * distance))
			pose = ui.Pose{Scale: .72 + .28*strength, Fade: .45 * (1 - strength)}
			color = t.Text.Mix(t.TextMuted, float32(1-math.Exp(-.85*distance)))
		}
		if isActive {
			color = t.Text
			pose = ui.Pose{X: 6, Scale: 1}
		}
		b := ui.ButtonBase(c).Bind(&a.lyricRows[row.Index]).Label(a.lyricLabels[row.Index].line).FillWidth().Padding(14, 4).Radius(t.Radius).Border(0, ui.Transparent).Background(ui.Transparent).TextColor(color)
		b.Children(func() {
			ui.Grid(c).Columns(1).FillWidth().Children(func() {
				drawText := func(glow bool) {
					key := "text"
					if glow {
						key = "glow"
					}
					text := ui.RichText(c.Key(key)).ColumnStart(1).RowStart(1).FontSize(fontSize).LineHeight(1.5).FillWidth().TransformOrigin(0, .5).AnimatePose(pose, lyricLineMotion)
					if glow {
						// Paint a glyph halo behind the crisp paragraph. The transparent
						// copy shares its layout, without changing spacing or hit areas.
						fx := &a.lyricLabels[row.Index].glow
						font := ui.Font{Family: t.Font, Size: fontSize, Weight: 700, LineHeight: 1.5}
						if fx.font != font {
							fx.font, fx.layout = font, nil
						}
						fx.ranges = fx.ranges[:0]
						fx.color = t.Accent
						for i := range line.Words {
							if line.wordActive(i, position) {
								fx.ranges = append(fx.ranges, a.lyricLabels[row.Index].ranges[i])
							}
						}
						text.TextColor(ui.Transparent).Draw(fx.Paint).PassThrough().Role(ui.RoleNone)
					} else {
						text.TextColor(text.AnimateColor("line-color", color, lyricLineMotion))
					}
					if a.lyricMode != "Static" {
						text.Bold()
					}
					text.Children(func() {
						if a.lyricMode == "Words" && len(line.Words) > 0 {
							for i, w := range line.Words {
								wordPose := ui.Pose{}
								singing := line.wordActive(i, position)
								base, filled := t.TextMuted, t.Accent.Mix(t.Text, .75)
								if singing {
									wordPose = ui.Pose{Y: -2.5, Scale: 1.075}
									// Make the whole current word identifiable from its first
									// frame, while its fill continues towards the full accent.
									base, filled = t.Accent.Mix(t.Text, .25), t.Accent
								}
								var word ui.Element
								if glow {
									word = ui.Text(c.Key(i), w.Text).TextColor(ui.Transparent).PassThrough().Role(ui.RoleNone)
								} else {
									spans := a.lyricLabels[row.Index].fills[i].paint(line.wordProgress(i, position), base, filled)
									word = ui.RichText(c.Key(i), spans...)
								}
								word.TransformOrigin(.5, .65).AnimatePose(wordPose, lyricWordMotion)
								if !glow && strings.TrimSpace(w.Text) != "" {
									word.Label(a.lyricLabels[row.Index].words[i]).Focusable().Cursor(ui.CursorPointer).OnClick(func() {
										if a.engine != nil {
											a.engine.Seek(a.lyricSeekTime(w.At))
										}
									})
								}
							}
						} else {
							value := line.Text
							if value == "" {
								value = "♪"
							}
							ui.Text(c, value)
						}
					})
				}
				if a.lyricMode == "Words" {
					for i := range line.Words {
						if line.wordActive(i, position) {
							drawText(true)
							break
						}
					}
				}
				drawText(false)
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
