package main

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

type albumGroup struct {
	Key, Title, Artist string
	Tracks             []int
	search             string
}

// Cache groups when the library changes, never while drawing. Album artist
// identifies tagged releases; folders distinguish releases when it is absent,
// while keeping multi-artist compilations in one album.
func (a *app) indexAlbums() {
	a.albums = nil
	index := make(map[string]int)
	for i, t := range a.prefs.Tracks {
		title := strings.TrimSpace(t.Album)
		if title == "" {
			title = "Unknown album"
		}
		owner := strings.ToLower(strings.TrimSpace(t.AlbumArtist))
		if owner == "" {
			owner = "folder:" + pathKey(filepath.Dir(t.Path))
		}
		key := strings.ToLower(title) + "\x00" + owner
		j, ok := index[key]
		if !ok {
			j = len(a.albums)
			index[key] = j
			artist := t.AlbumArtist
			if artist == "" {
				artist = t.Artist
			}
			a.albums = append(a.albums, albumGroup{Key: key, Title: title, Artist: artist})
		}
		g := &a.albums[j]
		g.Tracks = append(g.Tracks, i)
		if t.AlbumArtist == "" && g.Artist != t.Artist {
			g.Artist = "Various artists"
		}
	}
	for i := range a.albums {
		g := &a.albums[i]
		g.search = strings.ToLower(g.Title + "\x00" + g.Artist)
		slices.SortStableFunc(g.Tracks, func(i, j int) int {
			x, y := a.prefs.Tracks[i], a.prefs.Tracks[j]
			if v := cmp.Compare(max(1, x.Disc), max(1, y.Disc)); v != 0 {
				return v
			}
			if x.Number > 0 && y.Number > 0 {
				if v := cmp.Compare(x.Number, y.Number); v != 0 {
					return v
				}
			}
			return strings.Compare(strings.ToLower(x.Path), strings.ToLower(y.Path))
		})
	}
	slices.SortFunc(a.albums, func(x, y albumGroup) int {
		if v := strings.Compare(strings.ToLower(x.Title), strings.ToLower(y.Title)); v != 0 {
			return v
		}
		return strings.Compare(x.Key, y.Key)
	})
	a.albumIndex = make(map[string]int, len(a.albums))
	for i, g := range a.albums {
		a.albumIndex[g.Key] = i
	}
	if _, ok := a.albumIndex[a.albumKey]; !ok {
		a.albumKey = ""
	}
}

func (a *app) filterAlbums(query string) {
	a.visibleAlbums = a.visibleAlbums[:0]
	if i, ok := a.albumIndex[a.albumKey]; ok {
		for _, j := range a.albums[i].Tracks {
			if strings.Contains(a.prefs.Tracks[j].search, query) {
				a.filtered = append(a.filtered, j)
			}
		}
		return
	}
	for i, g := range a.albums {
		if strings.Contains(g.search, query) {
			a.visibleAlbums = append(a.visibleAlbums, i)
			a.filtered = append(a.filtered, g.Tracks...)
		}
	}
}

func (a *app) openAlbum(key string) {
	a.section = "Albums"
	a.albumKey = key
	a.query = ""
	a.refreshFilter()
}
func (a *app) playAlbum(key string) {
	i, ok := a.albumIndex[key]
	if !ok {
		return
	}
	g := a.albums[i]
	if len(g.Tracks) == 0 {
		return
	}
	a.prefs.Queue = make([]string, len(g.Tracks))
	for j, k := range g.Tracks {
		a.prefs.Queue[j] = a.prefs.Tracks[k].Path
	}
	a.bag = nil
	a.history = nil
	a.play(a.prefs.Queue[0])
}

func (a *app) albumOverview(c *ui.Context, width float32) {
	t := c.Theme()
	if len(a.visibleAlbums) == 0 {
		a.emptyView(c)
		return
	}
	panelWidth := width - 230
	columns := max(1, int((panelWidth-32)/172))
	rows := (len(a.visibleAlbums) + columns - 1) / columns
	ui.List(c.Key("album-grid"), &a.albumList, rows).FillWidth().Grow(1).MinHeight(0).Padding(8, 20, 20, 20).
		ItemKey(func(i int) any { return a.albums[a.visibleAlbums[i*columns]].Key }).Rows(func(row ui.ListRow) {
		c := row.Context
		ui.Row(c).FillWidth().Height(244).Gap(12).Padding(0, 0, 12, 0).Children(func() {
			for col := 0; col < columns; col++ {
				i := row.Index*columns + col
				if i >= len(a.visibleAlbums) {
					ui.Box(c).Width(0).Grow(1)
					continue
				}
				g := a.albums[a.visibleAlbums[i]]
				tr := &a.prefs.Tracks[g.Tracks[0]]
				ui.Column(c.Key(g.Key)).Width(0).Grow(1).MinWidth(0).FillHeight().Gap(8).Padding(10).Background(t.Background).Radius(10).Children(func() {
					label := "Open album " + g.Title + " · " + g.Artist
					b := ui.Button(c, "").Label(label).Tooltip(g.Title+" · "+g.Artist).FillWidth().Grow(1).MinHeight(0).Padding(0).Center().Background(ui.Transparent).Border(0, ui.Transparent)
					b.Children(func() { ui.Image(c, a.coverFor(tr)).Size(116, 116).Radius(8).Clip() })
					b.OnClick(func() { a.openAlbum(g.Key) })
					ui.Text(c, g.Title).FontSize(13).Bold().FillWidth().SingleLine().Tooltip(g.Title)
					ui.Text(c, g.Artist).FontSize(11).TextColor(t.TextMuted).FillWidth().SingleLine()
					ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
						ui.Text(c, songCount(len(g.Tracks))).FontSize(10).TextColor(t.TextMuted).Grow(1)
						iconButton(c, "play", "Play album "+g.Title+" · "+g.Artist, false).Size(28, 28).OnClick(func() { a.playAlbum(g.Key) })
					})
				})
			}
		})
	})
}

func songCount(n int) string {
	word := "songs"
	if n == 1 {
		word = "song"
	}
	return fmt.Sprintf("%d %s", n, word)
}
