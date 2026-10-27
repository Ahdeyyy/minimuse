package main

import (
	"cmp"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type albumGroup struct {
	Key, Title, Artist string
	Tracks             []int
	Guests             []string
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
		g.Guests = a.albumGuests(*g)
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
			t := a.prefs.Tracks[j]
			if strings.Contains(strings.ToLower(t.Title+"\x00"+t.Artist), query) {
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
	a.albumTracks = ui.ListState{Selected: &a.albumSelected}
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
	ui.List(c.Key("album-grid"), &a.albumList, rows).FillWidth().Grow(1).MinHeight(0).Padding(8, 36, 20, 20).
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

// Feature credits are taken only from tags, including common title credits.
// Keep names intact (AC/DC, Earth, Wind & Fire) instead of splitting every
// comma, slash or ampersand as if it always separated individual artists.
var featureCredit = regexp.MustCompile(`(?i)\b(?:feat\.?|ft\.?|featuring)\s+`)

func (a *app) albumGuests(g albumGroup) []string {
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(g.Artist)): true, "": true, "unknown artist": true}
	var guests []string
	add := func(credit string) {
		for _, name := range strings.Split(credit, ";") {
			name = strings.TrimSpace(strings.Trim(name, " ()[]"))
			key := strings.ToLower(name)
			if !seen[key] {
				seen[key] = true
				guests = append(guests, name)
			}
		}
	}
	for _, i := range g.Tracks {
		tr := a.prefs.Tracks[i]
		credits := featureCredit.Split(tr.Artist, -1)
		lead := strings.TrimSpace(credits[0])
		// A tagged album artist may be followed by collaborators.
		for _, join := range []string{" & ", ", ", " / ", "; "} {
			prefix := g.Artist + join
			if len(lead) >= len(prefix) && strings.EqualFold(lead[:len(prefix)], prefix) {
				lead = lead[len(prefix):]
				break
			}
		}
		add(lead)
		for _, credit := range credits[1:] {
			add(credit)
		}
		if match := featureCredit.FindStringIndex(tr.Title); match != nil {
			credit := tr.Title[match[1]:]
			if end := strings.IndexAny(credit, ")]"); end >= 0 {
				credit = credit[:end]
			}
			add(credit)
		}
	}
	return guests
}

func (a *app) albumDuration(g albumGroup) string {
	var total time.Duration
	known := 0
	for _, i := range g.Tracks {
		if d := a.prefs.Tracks[i].Duration; d > 0 {
			total += d
			known++
		}
	}
	if known == 0 {
		return "Duration unavailable"
	}
	seconds := int(total.Round(time.Second) / time.Second)
	value := fmt.Sprintf("%d min %d sec", seconds/60, seconds%60)
	if seconds >= 3600 {
		value = fmt.Sprintf("%d hr %d min", seconds/3600, seconds/60%60)
	}
	if known < len(g.Tracks) {
		return "Known duration · " + value
	}
	return "Total duration · " + value
}

func (a *app) albumDetail(c *ui.Context, width float32, g albumGroup) {
	t := c.Theme()
	_, height := c.Size()
	cover := a.coverFor(&a.prefs.Tracks[g.Tracks[0]])
	var coverWidth, coverHeight float32
	switch cover := cover.(type) {
	case *ui.Bitmap:
		w, h := cover.Size()
		coverWidth, coverHeight = float32(w), float32(h)
	case *ui.SVG:
		coverWidth, coverHeight = cover.Size()
	}
	ui.Column(c).Label("Album cover art").FillWidth().Grow(1).MinHeight(0).Draw(func(p *ui.Painter, r ui.Rect) {
		// Paint behind the whole page so the fade can continue into the
		// song list. Align the source's top edge rather than cropping its center.
		r.H = min(r.H, max(float32(320), height*.52))
		if coverWidth > 0 && coverHeight > 0 {
			scale := max(r.W/coverWidth, r.H/coverHeight)
			art := ui.Rect{X: r.X + (r.W-coverWidth*scale)/2, Y: r.Y, W: coverWidth * scale, H: coverHeight * scale}
			p.Clip(r, 0, func() { p.Image(cover, art, ui.Contain) })
		}
		p.FillGradient(r, ui.LinearGradient{Angle: 180, Stops: []ui.GradientStop{
			{Color: t.Surface.Alpha(0), At: 0},
			{Color: t.Surface.Alpha(.88), At: .25},
			{Color: t.Surface.Alpha(.97), At: .65},
			{Color: t.Surface, At: 1},
		}}, 0)
	}).Children(func() {
		a.albumHero(c, g, width)
		if len(a.filtered) == 0 {
			ui.Text(c, "No matching songs. Try another song or artist.").FontSize(12).TextColor(t.TextMuted).FillWidth().Padding(20, 24)
			return
		}
		a.songTableHeader(c, width)
		ui.List(c.Key("album/"+g.Key), &a.albumTracks, len(a.filtered)).FillWidth().Grow(1).MinHeight(0).Padding(8, 28, 8, 12).
			ItemKey(func(i int) any { return a.prefs.Tracks[a.filtered[i]].Path }).Rows(func(row ui.ListRow) {
			a.songRow(row.Context, row.Index, row.Selected(), width)
		}).OnChange(func() { a.selected = a.albumSelected }).OnSubmit(func() {
			i := a.albumSelected
			if i < 0 || i >= len(a.filtered) {
				return
			}
			if a.prefs.Tracks[a.filtered[i]].Path == a.prefs.Current && a.status.Path == a.prefs.Current && !a.status.Ended && a.status.Error == "" {
				a.openSong()
			} else {
				a.playFrom(i)
			}
		})
	})
}

func (a *app) albumHero(c *ui.Context, g albumGroup, width float32) {
	t := c.Theme()
	_, height := c.Size()
	artSpace := min(float32(100), max(float32(56), height*.12))
	ui.Column(c).FillWidth().Shrink(0).Padding(artSpace, 24, 12, 24).Gap(6).Children(func() {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			iconButton(c, "back", "Back to albums", false).Size(24, 24).OnClick(func() { a.albumKey = ""; a.query = ""; a.refreshFilter() })
			ui.Text(c, "ALBUM").FontSize(10).Bold().TextColor(t.TextMuted)
		})
		ui.Text(c, g.Title).FontSize(min(float32(30), max(float32(22), width*.025))).Bold().FillWidth().LineHeight(1.2)
		ui.Text(c, g.Artist).FontSize(13).Bold().FillWidth()
		if len(g.Guests) > 0 {
			label := "Featuring · "
			if g.Artist == "Various artists" {
				label = "Artists · "
			}
			credits := label + strings.Join(g.Guests, ", ")
			ui.Text(c, credits).FontSize(11).TextColor(t.TextMuted).FillWidth().MaxLines(2).Tooltip(credits)
		}
		ui.Text(c, songCount(len(g.Tracks))+" · "+a.albumDuration(g)).FontSize(11).TextColor(t.TextMuted).FillWidth()
		textButton(c, "play", "Play album").AlignSelf(ui.Start).Background(t.Accent).TextColor(t.AccentText).Border(0, ui.Transparent).OnClick(func() { a.playAlbum(g.Key) })
	})
}
