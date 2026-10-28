package main

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) searchVisible() bool {
	return !a.mini && !a.songScreen && !a.settings && !a.playlistDialog
}

func (a *app) view(c *ui.Context) {
	// Menu choices are delivered while rows are being built. Apply them at
	// the next pass's start, before any list captures its count or indices.
	if action := a.trackAction; action != nil {
		a.trackAction = nil
		action()
	}
	c.SetTheme(a.theme)
	defer a.toasts(c)
	t := c.Theme()
	c.Root().Background(t.Background)
	c.OnShortcut(ui.Ctrl, ui.KeyO, func() { a.browse(false) })
	if a.searchVisible() {
		c.OnShortcut(ui.Ctrl, ui.KeyF, func() { a.searchHandle.Focus(c) })
	}
	c.OnShortcut(ui.Ctrl, ui.KeyM, func() { a.setMini(!a.mini) })
	c.OnShortcut(ui.Ctrl, ui.KeyR, a.refreshFolders)
	c.OnShortcut(ui.Ctrl, ui.KeyL, a.openSong)
	c.OnShortcut(ui.Ctrl, ui.KeyRight, func() { a.advance(false) })
	c.OnShortcut(ui.Ctrl, ui.KeyLeft, a.previous)
	c.OnShortcut(ui.Ctrl, ui.KeyP, a.toggle)
	if (a.mini || !a.searchHandle.Focused(c)) && !a.settings && !a.playlistDialog {
		c.OnShortcut(0, ui.KeySpace, a.toggle)
	}
	c.OnShortcut(ui.Ctrl, ui.KeyQ, func() {
		if a.window != nil {
			a.quit = true
			a.window.Close()
		}
	})
	if a.mini {
		ui.Column(c).Fill().Children(func() { a.titlebar(c); a.miniView(c) })
		return
	}
	w, _ := c.Size()
	ui.Column(c).Fill().Children(func() {
		a.titlebar(c)
		ui.Row(c).FillWidth().Grow(1).MinHeight(0).Padding(20, 20, 14, 14).Gap(16).Children(func() {
			a.sidebar(c)
			panel := ui.Column(c.Key("main-panel")).Grow(1).MinWidth(0).FillHeight().Background(t.Surface).Radius(12).Border(1, t.Border.Alpha(.65)).Clip()
			if a.songScreen {
				a.songBackdrop(c, panel)
			}
			panel.Children(func() {
				if a.songScreen {
					a.songView(c, w)
				} else {
					a.libraryView(c, w)
				}
			})
		})
		ui.Row(c).FillWidth().Shrink(0).Padding(0, 16, 20, 16).Justify(ui.Center).Children(func() {
			a.transport(c, min(1200, w-32))
		})
	})
	a.settingsDialog(c)
	a.savePlaylistDialog(c)
}

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(180).FillHeight().Padding(8, 4).Gap(6).Children(func() {
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Padding(0, 16, 0, 0).Children(func() {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				ui.Text(c, "LIBRARY").FontSize(10).Bold().TextColor(t.TextMuted).Padding(0, 12, 8, 12)
				for _, item := range []struct {
					label, icon string
					count       int
				}{{"All songs", "songs", len(a.prefs.Tracks)}, {"Albums", "albums", len(a.albums)}, {"Favorites", "heart", a.favoriteCount}, {"Queue", "queue", len(a.prefs.Queue)}} {
					active := !a.songScreen && a.section == item.label
					b := ui.Button(c.Key(item.label), "").Label(item.label).FillWidth().Padding(12).Gap(10).AlignItems(ui.Center).Border(0, ui.Transparent).Background(ui.Transparent).Radius(8)
					if active {
						b.Background(t.Selection).TextColor(t.Accent)
					} else if b.Hovered() {
						b.Background(t.SurfaceHover)
					}
					b.Children(func() {
						ui.Icon(c, icons[item.icon]).Size(17, 17)
						ui.Text(c, item.label).Grow(1).NoWrap()
						ui.Textf(c, "%d", item.count).FontSize(11).TextColor(t.TextMuted)
					})
					b.OnClick(func() {
						a.songScreen = false
						a.updatePulse()
						a.section = item.label
						a.albumKey = ""
						a.refreshFilter()
					})
				}
				playing := textButton(c, "record", "Playing now").FillWidth().Disabled(a.current() == nil)
				if a.songScreen {
					playing.Background(t.Selection).TextColor(t.Accent)
				}
				playing.OnClick(a.openSong)
				ui.Spacer(c).Height(16).Grow(0)
				textButton(c, "folder", "Add folder").FillWidth().Disabled(a.importing).OnClick(func() { a.browse(true) })
				textButton(c, "plus", "Add songs").FillWidth().Background(ui.Transparent).Border(0, ui.Transparent).Disabled(a.importing).OnClick(func() { a.browse(false) })
				textButton(c, "refresh", "Refresh folders").FillWidth().Background(ui.Transparent).Border(0, ui.Transparent).Disabled(a.importing || len(a.prefs.Folders) == 0).Tooltip("Scan saved music folders for newly added songs · Ctrl+R").OnClick(a.refreshFolders)
				ui.Text(c, "PLAYLISTS").FontSize(10).Bold().TextColor(t.TextMuted).Padding(20, 12, 6, 12)
				if len(a.prefs.Playlists) == 0 {
					ui.Text(c, "No playlists").FontSize(10).TextColor(t.TextMuted).Padding(4, 12)
				} else {
					// Menu actions can delete items during construction. Keep the row
					// count, keys and names stable through this build and its layout.
					playlists := slices.Clone(a.prefs.Playlists)
					ui.List(c.Key("playlists"), &a.playlistList, len(playlists)).FillWidth().Padding(0, 16, 0, 0).Height(float32(min(160, max(42, len(playlists)*42)))).ItemKey(func(i int) any { return playlists[i].Name }).Rows(func(row ui.ListRow) {
						c := row.Context
						name := playlists[row.Index].Name
						b := ui.Button(c, "").Label(name).Tooltip(name).FillWidth().Padding(9, 12).Gap(8).AlignItems(ui.Center).Background(ui.Transparent).Border(0, ui.Transparent)
						b.Children(func() {
							ui.Icon(c, icons["music"]).Size(16, 16).Shrink(0)
							ui.Text(c, name).Width(0).Grow(1).SingleLine()
						})
						if !a.songScreen && a.section == "playlist:"+name {
							b.Background(t.Selection).TextColor(t.Accent)
						}
						b.OnClick(func() { a.songScreen = false; a.updatePulse(); a.section = "playlist:" + name; a.refreshFilter() }).ContextMenu(func(m *ui.Menu) {
							if m.Item("Delete playlist").Chosen() {
								a.prefs.Playlists = slices.DeleteFunc(a.prefs.Playlists, func(p playlist) bool { return p.Name == name })
								if a.section == "playlist:"+name {
									a.section = "All songs"
									a.refreshFilter()
								}
								a.persist()
							}
						})
					})
				}
			})
		})
		ui.Row(c).FillWidth().AlignItems(ui.Center).Shrink(0).Children(func() {
			textButton(c, "settings", "Settings").Background(ui.Transparent).Border(0, ui.Transparent).OnClick(func() { a.settings = true })
			ui.Spacer(c).Grow(1)
			name, label := "moon", "Use dark theme"
			if a.prefs.Dark {
				name, label = "sun", "Use light theme"
			}
			iconButton(c, name, label, false).OnClick(a.toggleTheme)
		})
	})
}

func (a *app) libraryView(c *ui.Context, width float32) {
	if a.section == "Albums" {
		if i, ok := a.albumIndex[a.albumKey]; ok {
			a.albumDetail(c, width, a.albums[i])
			return
		}
	}
	t := c.Theme()
	section := strings.TrimPrefix(a.section, "playlist:")
	subtitle := songCount(len(a.filtered))
	albumOverview := a.section == "Albums" && a.albumKey == ""
	if albumOverview {
		word := "albums"
		if len(a.visibleAlbums) == 1 {
			word = "album"
		}
		subtitle = fmt.Sprintf("%d %s", len(a.visibleAlbums), word)
	}
	ui.Column(c).FillWidth().Padding(24, 24, 18, 24).Gap(16).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
				ui.Text(c, section).FontSize(25).Bold().SingleLine()
				ui.Text(c, subtitle).FontSize(11).TextColor(t.TextMuted).SingleLine()
			})
			textButton(c, "play", "Play").Background(t.Accent).TextColor(t.AccentText).Border(0, ui.Transparent).Disabled(len(a.filtered) == 0).OnClick(func() { a.playFrom(0) })
			if a.section == "Queue" {
				iconButton(c, "plus", "Save queue as playlist", false).Disabled(len(a.prefs.Queue) == 0).OnClick(func() { a.playlistDialog = true })
			}
		})
	})
	if albumOverview {
		a.albumOverview(c, width)
		return
	}
	if len(a.filtered) == 0 {
		a.emptyView(c)
		return
	}
	a.songTableHeader(c, width)
	ui.List(c.Key(a.section), &a.list, len(a.filtered)).FillWidth().Grow(1).MinHeight(0).Padding(8, 28, 8, 12).
		ItemKey(func(i int) any { return a.prefs.Tracks[a.filtered[i]].Path }).Rows(func(row ui.ListRow) {
		a.songRow(row.Context, row.Index, row.Selected(), width)
	}).OnSubmit(func() {
		if a.selected >= 0 {
			if a.prefs.Tracks[a.filtered[a.selected]].Path == a.prefs.Current && a.status.Path == a.prefs.Current && !a.status.Ended && a.status.Error == "" {
				a.openSong()
			} else {
				a.playFrom(a.selected)
			}
		}
	})
	ui.Row(c).FillWidth().Height(38).Padding(0, 24).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Double-click a song to play").FontSize(10).TextColor(t.TextMuted)
	})
}

func (a *app) songTableHeader(c *ui.Context, width float32) {
	t := c.Theme()
	ui.Row(c).FillWidth().Height(32).Padding(0, 40, 0, 24).Gap(12).AlignItems(ui.Center).Background(t.Background.Alpha(.6)).Children(func() {
		ui.Text(c, "#").Width(20).FontSize(10).TextColor(t.TextMuted)
		ui.Text(c, "TITLE / ARTIST").Width(0).Grow(1).FontSize(10).TextColor(t.TextMuted)
		if width >= 1000 {
			ui.Text(c, "ALBUM").Width(125).FontSize(10).TextColor(t.TextMuted)
		}
		ui.Text(c, "FORMAT").Width(49).FontSize(10).TextColor(t.TextMuted)
		ui.Text(c, "TIME").Width(46).FontSize(10).TextColor(t.TextMuted).TextAlign(ui.End)
		ui.Spacer(c).Width(68).Grow(0).Shrink(0)
	})
}

func (a *app) songRow(c *ui.Context, i int, selected bool, width float32) {
	t := c.Theme()
	tr := a.prefs.Tracks[a.filtered[i]]
	current := tr.Path == a.prefs.Current
	e := ui.Row(c).FillWidth().Height(62).Padding(8, 12).Gap(12).AlignItems(ui.Center).Radius(8).TextColor(t.Text)
	if selected {
		e.Background(t.Selection)
	} else if current {
		e.Background(t.Background)
	} else if e.Hovered() {
		e.Background(t.SurfaceHover)
	}
	e.Children(func() {
		if current {
			ui.Icon(c, icons["music"]).Size(16, 16).Width(20).TextColor(t.Accent)
		} else {
			ui.Textf(c, "%02d", i+1).Width(20).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Image(c, a.coverFor(&tr)).Size(38, 38).Radius(5).Clip().Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(5).Children(func() {
			color := t.Text
			ui.Text(c, tr.Title).FontSize(13).Bold().TextColor(color).SingleLine().Tooltip(tr.Title)
			ui.Text(c, tr.Artist).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
		if width >= 1000 {
			ui.Text(c, tr.Album).Width(125).FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
		ui.Text(c, codec(tr.Path)).Width(49).FontSize(9).TextColor(t.TextMuted)
		duration := "—"
		if tr.Duration > 0 {
			duration = clockTime(tr.Duration)
		}
		ui.Text(c, duration).Width(46).FontSize(11).TextColor(t.TextMuted).TextAlign(ui.End).Label("Duration of " + tr.Title)
		iconButton(c, "heart", "Favorite "+tr.Title, tr.Favorite).Size(28, 28).OnClick(func() { a.favorite(tr.Path) })
		iconButton(c, "more", "Actions for "+tr.Title, false).Size(28, 28).Menu(func(m *ui.Menu) {
			if action := a.trackMenu(m, tr.Path, i); action != nil {
				a.trackAction = action
				c.Invalidate()
			}
		})
	})
	if current && a.status.Path == tr.Path && !a.status.Ended && a.status.Error == "" {
		e.OnClick(a.openSong)
	}
}

func (a *app) emptyView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).FillWidth().Grow(1).Center().Padding(28).Gap(16).Children(func() {
		ui.Image(c, covers[0]).Size(116, 116).Radius(22).Clip()
		title, description := "No songs", "Add music files or a folder."
		if a.query != "" {
			title, description = "No results", "Try another song, artist, or album."
		} else if a.section == "Favorites" {
			title, description = "No favorites", "Use the heart button to add favorites."
		} else if a.section == "Queue" {
			title, description = "Queue is empty", "Play music or add songs to the queue."
		} else if a.section == "Albums" {
			title, description = "No albums", "Add music to browse albums."
		}
		ui.Text(c, title).FontSize(21).Bold().TextAlign(ui.Center)
		ui.Text(c, description).FontSize(12).TextColor(t.TextMuted).TextAlign(ui.Center)
		if len(a.prefs.Tracks) == 0 {
			textButton(c, "folder", "Choose a music folder").Background(t.Accent).TextColor(t.AccentText).Disabled(a.importing).OnClick(func() { a.browse(true) })
			ui.Text(c, "or drop songs and folders anywhere here").FontSize(10).TextColor(t.TextMuted)
			ui.Text(c, "MP3  ·  FLAC  ·  WAV  ·  OGG").FontSize(10).TextColor(t.TextMuted)
		}
	})
}

func (a *app) transport(c *ui.Context, width float32) {
	t := c.Theme()
	tr := a.current()
	ui.Row(c.Key("transport")).Label("Playback controls").Width(width).Height(100).Shrink(0).Padding(12, 20).Gap(20).AlignItems(ui.Center).Background(t.Surface).Radius(16).Border(1, t.Border.Alpha(.65)).Shadow(0, 5, 18, 0, ui.RGBA(0, 0, 0, .08)).Children(func() {
		b := ui.Button(c, "").Label("Open song screen").Tooltip("Open currently playing song · Ctrl+L").Width(min(280, max(184, width*.25))).Shrink(0).Padding(0).Gap(12).AlignItems(ui.Center).Background(ui.Transparent).Border(0, ui.Transparent).Disabled(tr == nil)
		b.Children(func() {
			ui.Image(c, a.coverFor(tr)).Size(46, 46).Radius(7).Clip().Shrink(0)
			ui.Column(c).Width(0).Grow(1).MinWidth(0).Gap(6).Children(func() {
				title, artist := "Nothing playing", ""
				if tr != nil {
					title, artist = tr.Title, tr.Artist
				}
				ui.Text(c, title).FontSize(12).Bold().FillWidth().SingleLine().Tooltip(title)
				ui.Text(c, artist).FontSize(10).TextColor(t.TextMuted).FillWidth().SingleLine().Tooltip(artist)
			})
		})
		b.OnClick(a.openSong)
		ui.Column(c).Grow(1).MinWidth(0).Gap(7).Children(func() {
			ui.Row(c).FillWidth().Justify(ui.Center).Gap(10).AlignItems(ui.Center).Children(func() { a.playbackButtons(c) })
			a.seekControl(c)
		})
		a.volumeControl(c, 110)
		iconButton(c, "mini", "Mini player", false).OnClick(func() { a.setMini(true) })
	})
}

func (a *app) playbackButtons(c *ui.Context) {
	a.playbackControls(c, true)
}

func (a *app) playbackControls(c *ui.Context, extended bool) {
	t := c.Theme()
	if extended {
		iconButton(c, "shuffle", "Shuffle", a.prefs.Shuffle).OnClick(func() { a.prefs.Shuffle = !a.prefs.Shuffle; a.bag = nil; a.history = nil; a.persist() })
	}
	iconButton(c, "previous", "Previous song", false).Disabled(len(a.prefs.Queue) == 0).OnClick(a.previous)
	name := "play"
	if a.status.Playing {
		name = "pause"
	}
	b := ui.Button(c, "").Label("Play / Pause").Tooltip("Play / Pause · Space / Ctrl+P").Size(42, 42).Padding(0).Gap(0).Center().Shrink(0).Radius(21).Background(t.Accent).TextColor(t.AccentText).Border(0, ui.Transparent).Disabled(len(a.prefs.Tracks) == 0)
	b.Children(func() { ui.Icon(c, icons[name]).Size(20, 20).Shrink(0).Label("Play / Pause icon") })
	b.OnClick(a.toggle)
	iconButton(c, "next", "Next song", false).Disabled(len(a.prefs.Queue) == 0).OnClick(func() { a.advance(false) })
	if extended {
		repeat := []string{"Repeat off", "Repeat all", "Repeat one"}[a.prefs.Repeat]
		repeatIcon := "repeat"
		if a.prefs.Repeat == 2 {
			repeatIcon = "repeat-one"
		}
		iconButton(c, repeatIcon, repeat, a.prefs.Repeat != 0).OnClick(func() { a.prefs.Repeat = (a.prefs.Repeat + 1) % 3; a.persist() })
	}
}

func (a *app) seekControl(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, clockTime(a.status.Position)).FontSize(10).TextColor(t.TextMuted).Width(40).Shrink(0)
		ui.Slider(c.Key("seek"), &a.seek, 0, max(1, a.status.Duration.Seconds())).Width(0).Grow(1).MinWidth(20).Label("Seek").Disabled(a.status.Duration == 0 || a.status.Ended).OnChange(func() {
			if a.engine != nil {
				a.engine.Seek(time.Duration(a.seek * float64(time.Second)))
			}
		})
		ui.Text(c, clockTime(a.status.Duration)).FontSize(10).TextColor(t.TextMuted).Width(40).Shrink(0)
	})
}

func (a *app) volumeControl(c *ui.Context, width float32) {
	ui.Row(c).Width(width).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
		name := "volume"
		if a.muted || a.prefs.Volume == 0 {
			name = "mute"
		}
		iconButton(c, name, "Mute", a.muted).OnClick(a.mute)
		ui.Slider(c.Key("volume"), &a.prefs.Volume, 0, 100).Width(0).Grow(1).MinWidth(20).Label("Volume").Tooltip(fmt.Sprintf("Volume: %d%%", int(a.prefs.Volume))).OnChange(a.setVolume)
	})
}

func clockTime(d time.Duration) string {
	s := max(0, int(d.Seconds()))
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

func (a *app) trackMenu(m *ui.Menu, path string, index int) func() {
	if m.Item("Play now").Chosen() {
		return func() { a.playFrom(index) }
	}
	if m.Item("Play next").Chosen() {
		return func() { a.enqueue(path, true) }
	}
	if m.Item("Add to queue").Chosen() {
		return func() { a.enqueue(path, false) }
	}
	if len(a.prefs.Playlists) > 0 {
		var action func()
		m.Submenu("Add to playlist", func(m *ui.Menu) {
			for _, p := range a.prefs.Playlists {
				if m.Item(p.Name).Chosen() {
					action = func() { a.addToPlaylist(p.Name, path) }
				}
			}
		})
		if action != nil {
			return action
		}
	}
	m.Separator()
	if m.Item("Show in folder").Chosen() {
		return func() { mygo.Shell.ShowItemInFolder(path) }
	}
	if a.section == "Queue" {
		if m.Item("Move up").Chosen() {
			return func() { a.moveQueue(path, -1) }
		}
		if m.Item("Move down").Chosen() {
			return func() { a.moveQueue(path, 1) }
		}
		if m.Item("Remove from queue").Chosen() {
			return func() { a.remove(path, false) }
		}
	} else if strings.HasPrefix(a.section, "playlist:") {
		if m.Item("Remove from playlist").Chosen() {
			return func() {
				for i := range a.prefs.Playlists {
					p := &a.prefs.Playlists[i]
					if "playlist:"+p.Name == a.section {
						p.Paths = slices.DeleteFunc(p.Paths, func(s string) bool { return s == path })
						a.refreshFilter()
						a.persist()
						break
					}
				}
			}
		}
	} else if m.Item("Remove from library").Chosen() {
		return func() { a.remove(path, true) }
	}
	return nil
}

func (a *app) savePlaylistDialog(c *ui.Context) {
	ui.DialogBase(c.Key("save-playlist"), &a.playlistDialog, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .2))
		panel.Width(400).Padding(24).Gap(18).Radius(12).Background(c.Theme().Surface).Border(1, c.Theme().Border)
		ui.Text(c, "New playlist").FontSize(22).Bold()
		ui.TextInput(c, &a.playlistName).Label("Playlist name").Placeholder("Name your playlist").FillWidth().AutoFocus().OnSubmit(a.savePlaylist)
		ui.Row(c).FillWidth().Justify(ui.End).Gap(8).Children(func() {
			textButton(c, "", "Cancel").OnClick(func() { a.playlistDialog = false })
			textButton(c, "plus", "Save playlist").Disabled(strings.TrimSpace(a.playlistName) == "").OnClick(a.savePlaylist)
		})
	})
}
func (a *app) moveQueue(path string, delta int) {
	i := slices.Index(a.prefs.Queue, path)
	j := i + delta
	if i >= 0 && j >= 0 && j < len(a.prefs.Queue) {
		a.prefs.Queue[i], a.prefs.Queue[j] = a.prefs.Queue[j], a.prefs.Queue[i]
		a.bag = nil
		a.refreshFilter()
		a.persist()
	}
}

func (a *app) toggleTheme() {
	a.prefs.Dark = !a.prefs.Dark
	a.setTheme()
	a.persist()
	if a.window != nil {
		source := mygo.ThemeLight
		if a.prefs.Dark {
			source = mygo.ThemeDark
		}
		mygo.Theme.SetSource(source)
	}
}

func (a *app) settingsDialog(c *ui.Context) {
	ui.DialogBase(c.Key("settings"), &a.settings, func(backdrop, panel ui.Element) {
		width, height := c.Size()
		backdrop.Background(ui.RGBA(0, 0, 0, .2))
		panel.Label("Settings dialog").Width(min(640, width-48)).MaxHeight(height-32).Padding(24).Gap(16).Radius(12).Background(c.Theme().Surface).Border(1, c.Theme().Border)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Settings").FontSize(20).Bold()
			ui.Spacer(c).Grow(1)
			iconButton(c, "close", "Close settings", false).OnClick(func() { a.settings = false })
		})
		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Padding(0, 16, 0, 0).Children(func() {
			ui.Column(c).FillWidth().Gap(16).Children(func() {
				ui.Checkbox(c, &a.prefs.CloseToTray, "Close to tray").OnChange(a.persist)
				ui.Text(c, "Keep playing when you close the window. Use Quit MiniMuse in the tray menu to fully exit.").FillWidth().FontSize(12).TextColor(c.Theme().TextMuted).LineHeight(1.6)
				ui.Checkbox(c, &a.prefs.Dark, "Dark appearance").OnChange(func() {
					a.setTheme()
					a.persist()
					if a.window != nil {
						source := mygo.ThemeLight
						if a.prefs.Dark {
							source = mygo.ThemeDark
						}
						mygo.Theme.SetSource(source)
					}
				})
				ui.Text(c, "Audio").Bold()
				ui.Text(c, "48 kHz stereo · 32-bit float").FillWidth().FontSize(12).TextColor(c.Theme().TextMuted)
				ui.Checkbox(c, &a.prefs.Crossfade, "Crossfade between tracks").OnChange(a.crossfadeChanged)
				ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Fade duration").FontSize(12).Grow(1)
					ui.NumberInput(c, &a.prefs.CrossfadeSeconds, 1, 12, .5).Width(160).Shrink(0).Label("Crossfade duration (seconds)").Disabled(!a.prefs.Crossfade).OnChange(a.crossfadeChanged)
					ui.Text(c, "seconds").FontSize(11).TextColor(c.Theme().TextMuted)
				})
				ui.Text(c, "Blend the end of a song into the next. Shorter fades switch faster; longer fades blend more gradually.").FillWidth().FontSize(12).TextColor(c.Theme().TextMuted).LineHeight(1.6)
				ui.Collapsible(c.Key("music-folders"), fmt.Sprintf("Music folders (%d)", len(a.prefs.Folders)), &a.foldersOpen, func() {
					if len(a.prefs.Folders) == 0 {
						ui.Text(c, "Add a folder from the library to keep it available for refresh.").FillWidth().FontSize(12).TextColor(c.Theme().TextMuted)
					} else {
						ui.List(c.Key("saved-folders"), &a.folderList, len(a.prefs.Folders)).FillWidth().Height(float32(min(168, len(a.prefs.Folders)*42))).Padding(0, 16, 0, 0).ItemKey(func(i int) any { return a.prefs.Folders[i] }).Rows(func(row ui.ListRow) {
							c := row.Context
							folder := a.prefs.Folders[row.Index]
							ui.Row(c).FillWidth().Height(42).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Text(c, folder).Width(0).Grow(1).SingleLine().Tooltip(folder).FontSize(11).TextColor(c.Theme().TextMuted)
								iconButton(c, "refresh", "Refresh folder "+folder, false).Disabled(a.importing).OnClick(func() { a.importMusic([]string{folder}, true) })
							})
						})
					}
				}).FillWidth()
				ui.Text(c, "Keyboard").Bold()
				ui.Text(c, "Space / Ctrl+P   Play / pause\nCtrl+← / →   Previous / next\nCtrl+M   Mini player\nCtrl+L   Playing now\nCtrl+R   Refresh folders\nCtrl+O   Add songs\nCtrl+F   Search\nCtrl+Q   Quit").FillWidth().FontSize(12).TextColor(c.Theme().TextMuted).LineHeight(1.8)
			})
		})
		textButton(c, "check", "Done").OnClick(func() { a.settings = false })
	})
}
