package main

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAlbumDetails(t *testing.T) {
	for _, size := range [][2]int{{820, 580}, {1080, 740}} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/dark=%v", size[0], size[1], dark), func(t *testing.T) {
				a := testApp(t, 20)
				for i := range a.prefs.Tracks {
					tr := &a.prefs.Tracks[i]
					tr.AlbumArtist, tr.Artist = "Lead", "Lead"
					tr.Duration = 185 * time.Second
				}
				a.prefs.Tracks[0].Artist = "Lead feat. Guest"
				a.prefs.Tracks[1].Title = "Second (feat. Guest Two)"
				a.prefs.Dark = dark
				a.setTheme()
				a.reindex()
				a.openAlbum(a.albums[0].Key)
				tt := ui.NewTester(a.view, size[0], size[1])
				for _, text := range []string{"Quiet hours", "Featuring · Guest, Guest Two", "20 songs · Total duration · 1 hr 1 min"} {
					if !tt.HasText(text) {
						t.Fatalf("missing album detail %q", text)
					}
				}
				art, ok := tt.Find("Album cover art")
				search, _ := tt.Find("Search music")
				play, _ := tt.Find("Play album")
				transport, _ := tt.Find("Playback controls")
				if !ok || art.W < float32(size[0]-260) || search.Y < 0 || search.Y+search.H > 42 || search.Y+search.H > art.Y || search.Y+search.H > play.Y || art.Y+art.H > transport.Y {
					t.Fatal("search is not in the title bar above the album artwork", art, search, play, transport)
				}
				captureUI(t, tt, fmt.Sprintf("album-detail-%dx%d-dark-%v", size[0], size[1], dark))
				click(t, tt, "Play album")
				if len(a.prefs.Queue) != 20 || a.prefs.Current != a.prefs.Tracks[a.albums[0].Tracks[0]].Path {
					t.Fatal("album play did not queue the full release", a.prefs.Queue)
				}
				// Searches stay inside this release and do not match its title.
				click(t, tt, "Search music")
				tt.Type("Quiet hours")
				if len(a.filtered) != 0 || !tt.HasText("No matching songs. Try another song or artist.") {
					t.Fatal("album search still matches album tags", a.filtered)
				}
				tt.Key(ui.Cmd, ui.KeyA)
				tt.Type("Guest Two")
				if !slices.Equal(a.filtered, []int{1}) {
					t.Fatal("song search did not select the guest's track", a.filtered)
				}
				// Selection and Enter use the same song index as other screens.
				tt.Key(0, ui.KeyEscape)
				r, ok := tt.Find("Duration of Second (feat. Guest Two)")
				if !ok {
					t.Fatal("filtered song is missing")
				}
				tt.Press(r.X+r.W/2, r.Y+r.H/2)
				tt.Release(r.X+r.W/2, r.Y+r.H/2)
				if a.albumSelected != 0 || a.selected != 0 {
					t.Fatal("album selection does not use the song index", a.albumSelected, a.selected)
				}
				tt.Key(0, ui.KeyEnter)
				if a.prefs.Current != a.prefs.Tracks[1].Path {
					t.Fatal("album header changed the submitted song index", a.albumSelected, a.prefs.Current)
				}
				click(t, tt, "Back to albums")
				a.openAlbum(a.albums[0].Key)
				tt.Frame()
				// Songs scroll independently below the artwork and search.
				r, ok = tt.Find("Duration of " + a.prefs.Tracks[0].Title)
				if !ok || r.Y+r.H > transport.Y {
					t.Fatal("first album song is not visible below search", r)
				}
				tt.Wheel(r.X+r.W/2, r.Y+r.H/2, 0, 420)
				tt.Advance(time.Second)
				after, _ := tt.Find("Search music")
				if after.Y != search.Y {
					t.Fatal("search moved with the album artwork")
				}
				if cover, visible := tt.Find("Album cover art"); !visible || cover != art {
					t.Fatal("album artwork moved while scrolling songs", cover)
				}
				if first, _ := a.albumTracks.Visible(); first == 0 {
					t.Fatal("album songs did not scroll")
				}
				captureUI(t, tt, fmt.Sprintf("album-scrolled-%dx%d-dark-%v", size[0], size[1], dark))
			})
		}
	}
}

func TestAlbumCreditsAndMissingDurations(t *testing.T) {
	a := testApp(t, 3)
	for i := range a.prefs.Tracks {
		a.prefs.Tracks[i].AlbumArtist = "AC/DC"
	}
	a.prefs.Tracks[0].Artist = "AC/DC ft. Earth, Wind & Fire"
	a.prefs.Tracks[1].Artist = "AC/DC & Guest"
	a.prefs.Tracks[2].Artist = "AC/DC feat. guest"
	a.reindex()
	g := a.albums[0]
	if !slices.Equal(g.Guests, []string{"Earth, Wind & Fire", "Guest"}) {
		t.Fatal("artist names were split or duplicated", g.Guests)
	}
	if got := a.albumDuration(g); got != "Duration unavailable" {
		t.Fatal(got)
	}
	a.prefs.Tracks[0].Duration = 185 * time.Second
	if got := a.albumDuration(g); got != "Known duration · 3 min 5 sec" {
		t.Fatal("missing lengths presented as the full duration", got)
	}
}

func TestLyricsFollowGlides(t *testing.T) {
	for _, reduced := range []bool{false, true} {
		t.Run(fmt.Sprintf("reduced=%v", reduced), func(t *testing.T) {
			a := testApp(t, 1)
			a.playFrom(0)
			a.songScreen = true
			a.lyricsPath = a.prefs.Current
			var lrc strings.Builder
			for i := 0; i < 30; i++ {
				fmt.Fprintf(&lrc, "[00:%02d]Line %d with a little room for the music\n", i+1, i+1)
			}
			a.lyrics = parseLyrics(lrc.String())
			a.lyricMode = "Timed"
			a.status.Position = 9 * time.Second
			tt := ui.NewTester(a.view, 1080, 740)
			tt.SetPreferences(ui.Preferences{ReduceMotion: reduced})
			start := a.lyricScroll.Y
			a.status.Position = 10 * time.Second
			tt.Frame()
			if !reduced && (math.Abs(float64(a.lyricScroll.Y-start)) > 1 || !a.lyricScroll.Scrolling()) {
				t.Fatal("following a line jumped instead of starting a glide", start, a.lyricScroll.Y)
			}
			tt.Advance(80 * time.Millisecond)
			middle := a.lyricScroll.Y
			if !reduced && middle <= start {
				t.Fatal("following did not move toward the next line", start, middle)
			}
			tt.Advance(time.Second)
			if !reduced && a.lyricScroll.Y <= middle {
				t.Fatal("following completed in one step", middle, a.lyricScroll.Y)
			}
			line, _ := tt.Find("Lyric line 10")
			viewport, _ := tt.Find("Lyric lines")
			if d := math.Abs(float64(line.Y + line.H/2 - viewport.Y - viewport.H/2)); d > 2 {
				t.Fatal("wrapped lyric line did not settle in the center", line, viewport)
			}
			if reduced && a.lyricScroll.Scrolling() {
				t.Fatal("reduced motion still animates scrolling")
			}
			// Retarget during motion, then stop immediately for manual browsing.
			a.status.Position = 11 * time.Second
			tt.Frame()
			tt.Advance(50 * time.Millisecond)
			a.status.Position = 12 * time.Second
			tt.Frame()
			click(t, tt, "Follow playback")
			stopped := a.lyricScroll.Y
			tt.Advance(time.Second)
			if a.lyricScroll.Scrolling() || a.lyricScroll.Y != stopped {
				t.Fatal("following continued after it was disabled", stopped, a.lyricScroll.Y)
			}
		})
	}
}

func captureUI(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("MINIMUSE_CAPTURE_DIR")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
