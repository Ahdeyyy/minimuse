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
	"minimuse/internal/audio"
)

type fakeEngine struct {
	loads          []string
	seeks          []time.Duration
	volume         float64
	crossfade      time.Duration
	snapshot       audio.Status
	closeSnapshot  *audio.Status
	restores       []audio.Status
	toggles, stops int
}

func (f *fakeEngine) Load(path string) bool     { f.loads = append(f.loads, path); return true }
func (f *fakeEngine) Toggle()                   { f.toggles++ }
func (f *fakeEngine) Stop()                     { f.stops++ }
func (f *fakeEngine) Seek(d time.Duration)      { f.seeks = append(f.seeks, d) }
func (f *fakeEngine) Volume(v float64)          { f.volume = v }
func (f *fakeEngine) Crossfade(d time.Duration) { f.crossfade = d }
func (f *fakeEngine) Snapshot() audio.Status    { return f.snapshot }
func (f *fakeEngine) Close() {
	if f.closeSnapshot != nil {
		f.snapshot = *f.closeSnapshot
	}
}
func (f *fakeEngine) Restore(path string, position time.Duration, playing bool) bool {
	f.restores = append(f.restores, audio.Status{Path: path, Position: position, Playing: playing})
	return true
}

func testApp(t testing.TB, n int) *app {
	t.Helper()
	p := preferences{Volume: 80, CloseToTray: true}
	for i := 0; i < n; i++ {
		p.Tracks = append(p.Tracks, track{Path: fmt.Sprintf("song-%d.flac", i), Title: fmt.Sprintf("Song %05d", i), Artist: fmt.Sprintf("Artist %d", i%3), Album: "Quiet hours"})
	}
	a := newApp(p, "")
	a.engine = &fakeEngine{}
	t.Cleanup(a.cancel)
	return a
}

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatal(err)
	}
}

func TestLibraryUI(t *testing.T) {
	a := testApp(t, 40)
	tt := ui.NewTester(a.view, 1080, 740)
	click(t, tt, "Favorite Song 00000")
	if a.favoriteCount != 1 {
		t.Fatal("favorite action not applied")
	}
	click(t, tt, "Favorites")
	if len(a.filtered) != 1 {
		t.Fatalf("favorites: %v", a.filtered)
	}
	click(t, tt, "Play")
	if !slices.Equal(a.prefs.Queue, []string{"song-0.flac"}) {
		t.Fatal(a.prefs.Queue)
	}
	click(t, tt, "All songs")
	click(t, tt, "Search music")
	tt.Type("Artist 2")
	if len(a.filtered) != 13 {
		t.Fatalf("search returned %d", len(a.filtered))
	}
	click(t, tt, "Play")
	if len(a.prefs.Queue) != 13 {
		t.Fatal("search playback did not create queue")
	}
	click(t, tt, "Queue")
	click(t, tt, "Save queue as playlist")
	tt.Type("Late nights")
	click(t, tt, "Save playlist")
	if len(a.prefs.Playlists) != 1 || len(a.prefs.Playlists[0].Paths) != 13 {
		t.Fatal(a.prefs.Playlists)
	}
	click(t, tt, "Late nights")
	tt.Frame()
	tt.Frame()
	click(t, tt, "Settings")
	tt.Frame()
	click(t, tt, "Dark appearance")
	if !a.theme.Dark {
		t.Fatal("theme did not change")
	}
	click(t, tt, "Done")
	tt.Frame()
	tt.Frame()
}

func TestViewSizesAndVirtualization(t *testing.T) {
	for _, size := range [][2]int{{820, 580}, {1080, 740}, {1440, 900}} {
		for _, dark := range []bool{false, true} {
			a := testApp(t, 10000)
			a.prefs.Dark = dark
			a.setTheme()
			tt := ui.NewTester(a.view, size[0], size[1])
			tt.Frame()
			if len(tt.Texts()) > 400 {
				t.Fatalf("rendered too many rows: %d", len(tt.Texts()))
			}
			for _, label := range []string{"Play / Pause", "Volume", "All songs", "Settings"} {
				r, ok := tt.Find(label)
				if !ok || r.W <= 0 || r.X < 0 || r.Y < 0 || r.X+r.W > float32(size[0])+1 || r.Y+r.H > float32(size[1])+1 {
					t.Errorf("%dx%d %s out of bounds: %+v", size[0], size[1], label, r)
				}
			}
		}
	}
}

func TestQueueTransitions(t *testing.T) {
	a := testApp(t, 3)
	f := a.engine.(*fakeEngine)
	a.playFrom(0)
	a.status.Position = 4 * time.Second
	a.previous()
	if !slices.Equal(f.seeks, []time.Duration{0}) {
		t.Fatal(f.seeks)
	}
	a.acceptStatus(audio.Status{Path: "song-0.flac", Ended: true})
	if a.prefs.Current != "song-1.flac" {
		t.Fatal(a.prefs.Current)
	}
	a.acceptStatus(audio.Status{Path: "song-1.flac", Ended: true})
	a.acceptStatus(audio.Status{Path: "song-2.flac", Ended: true})
	if len(f.loads) != 3 {
		t.Fatal("repeat off wrapped")
	}
	a.prefs.Repeat = 2
	a.advance(true)
	if f.loads[len(f.loads)-1] != "song-2.flac" {
		t.Fatal("repeat one skipped")
	}
	a.prefs.Repeat = 1
	a.advance(true)
	if a.prefs.Current != "song-0.flac" {
		t.Fatal("repeat all did not wrap")
	}
	a.remove(a.prefs.Current, true)
	if a.current() != nil || f.stops != 1 {
		t.Fatal("removing current did not stop")
	}
}

func TestStalePlaybackNotificationCannotSkipNewSelection(t *testing.T) {
	a := testApp(t, 3)
	a.playFrom(0)
	a.playFrom(1)
	a.acceptStatus(audio.Status{Path: "song-0.flac", Ended: true})
	if a.prefs.Current != "song-1.flac" || len(a.engine.(*fakeEngine).loads) != 2 {
		t.Fatal("stale end advanced queue")
	}
}

func TestCommandLineAndSpaceShortcut(t *testing.T) {
	files, tray, play := parseSecondInstance([]string{"MiniMuse.exe", "-data", "C:/data", "-tray", "-play", "song.mp3"})
	if !tray || !play || !slices.Equal(files, []string{"song.mp3"}) {
		t.Fatal(files, tray, play)
	}
	a := testApp(t, 3)
	a.playFrom(0)
	a.status = audio.Status{Path: a.prefs.Current}
	tt := ui.NewTester(a.view, 1080, 740)
	click(t, tt, "Song 00000")
	tt.Key(0, ui.KeySpace)
	if a.engine.(*fakeEngine).toggles != 1 {
		t.Fatal("space did not toggle playback")
	}
	if !a.songScreen || len(a.engine.(*fakeEngine).loads) != 1 {
		t.Fatal("selecting the current song should open Playing now without reloading")
	}
	click(t, tt, "Back to library")
	click(t, tt, "Search music")
	tt.Key(0, ui.KeySpace)
	if a.engine.(*fakeEngine).toggles != 1 {
		t.Fatal("typing a space toggled playback")
	}
}

func TestShuffleVisitsEachSongOnce(t *testing.T) {
	a := testApp(t, 20)
	a.prefs.Shuffle = true
	a.playFrom(0)
	seen := map[string]bool{a.prefs.Current: true}
	for range 19 {
		a.advance(true)
		if seen[a.prefs.Current] {
			t.Fatal("shuffle repeated", a.prefs.Current)
		}
		seen[a.prefs.Current] = true
	}
	if len(seen) != 20 {
		t.Fatal(len(seen))
	}
	before := len(a.engine.(*fakeEngine).loads)
	a.advance(true)
	if len(a.engine.(*fakeEngine).loads) != before {
		t.Fatal("shuffle did not stop")
	}
}

func TestCurrentSongSelectionBeforeAndAfterLoading(t *testing.T) {
	a := testApp(t, 3)
	a.prefs.Current = a.prefs.Tracks[0].Path
	tt := ui.NewTester(a.view, 1080, 740)
	click(t, tt, "Song 00000")
	if a.songScreen {
		t.Fatal("a saved song is not yet playing")
	}
	tt.Key(0, ui.KeyEnter)
	if len(a.engine.(*fakeEngine).loads) != 1 || a.songScreen {
		t.Fatal("Enter should start the saved song")
	}
	tt.Key(0, ui.KeyEnter)
	if !a.songScreen || len(a.engine.(*fakeEngine).loads) != 1 {
		t.Fatal("Enter on the loaded song should open Playing now")
	}
}

func TestMiniPlayerAndCenteredIcons(t *testing.T) {
	a := testApp(t, 6)
	a.playFrom(0)
	a.status = audio.Status{Path: a.prefs.Current, Duration: 180 * time.Second, Position: time.Second}
	tt := ui.NewTester(a.view, 1080, 740)
	checkCentered := func(label string) {
		t.Helper()
		b, ok := tt.Find(label)
		if !ok {
			t.Fatal("missing button", label)
		}
		i, ok := tt.Find(label + " icon")
		if !ok {
			t.Fatal("missing icon", label)
		}
		if math.Abs(float64(b.X+b.W/2-i.X-i.W/2)) > .5 || math.Abs(float64(b.Y+b.H/2-i.Y-i.H/2)) > .5 {
			t.Errorf("%s: icon %+v not centered in %+v", label, i, b)
		}
	}
	for _, label := range []string{"Play / Pause", "Shuffle", "Previous song", "Next song", "Repeat off", "Mute", "Mini player"} {
		checkCentered(label)
	}
	before := a.prefs.Current
	engine := a.engine
	click(t, tt, "Mini player")
	tt.SetSize(miniWidth, miniHeight)
	tt.Frame()
	tt.Frame()
	if !a.mini || a.engine != engine || a.prefs.Current != before {
		t.Fatal("mode switch changed playback")
	}
	for _, label := range []string{"Play / Pause", "Mute", "Exit mini player"} {
		checkCentered(label)
	}
	for _, label := range []string{"Play / Pause", "Previous song", "Next song", "Volume", "Seek", "Exit mini player"} {
		r, ok := tt.Find(label)
		if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > miniWidth+.5 || r.Y+r.H > miniHeight+.5 {
			t.Errorf("mini control %s outside bounds: %+v", label, r)
		}
	}
	tt.SetSize(360, miniHeight)
	for _, label := range []string{"Play / Pause", "Volume", "Seek", "Exit mini player"} {
		r, ok := tt.Find(label)
		if !ok || r.X < 0 || r.Y < 0 || r.X+r.W > 360.5 || r.Y+r.H > miniHeight+.5 {
			t.Errorf("minimum mini width clips %s: %+v", label, r)
		}
	}
	tt.SetSize(miniWidth, miniHeight)
	click(t, tt, "Play / Pause")
	if a.engine.(*fakeEngine).toggles != 1 {
		t.Fatal("mini play did not reach engine")
	}
	click(t, tt, "Next song")
	if a.prefs.Current == before {
		t.Fatal("mini next did not advance")
	}
	click(t, tt, "Exit mini player")
	tt.SetSize(1080, 740)
	tt.Frame()
	tt.Frame()
	if a.mini || !tt.HasText("All songs") {
		t.Fatal("library did not return")
	}
	for range 3 {
		tt.Key(ui.Ctrl, ui.KeyM)
		tt.Frame()
		tt.Key(ui.Ctrl, ui.KeyM)
		tt.Frame()
	}
}

func TestAlbumsBrowsePlayAndRemove(t *testing.T) {
	p := preferences{Volume: 80, Tracks: []track{
		{Path: "one/02.flac", Title: "Second", Artist: "Artist A", Album: "Release", Number: 2},
		{Path: "one/01.flac", Title: "First", Artist: "Artist A", Album: "Release", Number: 1},
		{Path: "two/01.flac", Title: "Other", Artist: "Artist B", Album: "Release"},
		{Path: "mix/a.flac", Title: "Mix A", Artist: "Artist A", Album: "Compilation"},
		{Path: "mix/b.flac", Title: "Mix B", Artist: "Artist B", Album: "Compilation"},
	}}
	a := newApp(p, "")
	defer a.cancel()
	a.engine = &fakeEngine{}
	if len(a.albums) != 3 {
		t.Fatalf("groups: %+v", a.albums)
	}
	tt := ui.NewTester(a.view, 1080, 740)
	click(t, tt, "Albums")
	if len(a.visibleAlbums) != 3 {
		t.Fatal(a.visibleAlbums)
	}
	click(t, tt, "Play album Release · Artist A")
	if !slices.Equal(a.prefs.Queue, []string{"one/01.flac", "one/02.flac"}) {
		t.Fatal("album track order", a.prefs.Queue)
	}
	click(t, tt, "Open album Release · Artist A")
	tt.Frame()
	if len(a.filtered) != 2 || a.prefs.Tracks[a.filtered[0]].Title != "First" {
		t.Fatal("album filter", a.filtered)
	}
	click(t, tt, "Back to albums")
	click(t, tt, "Search music")
	tt.Type("Compilation")
	if len(a.visibleAlbums) != 1 || a.albums[a.visibleAlbums[0]].Artist != "Various artists" {
		t.Fatal("compilation/search", a.visibleAlbums)
	}
	a.remove("mix/a.flac", true)
	a.remove("mix/b.flac", true)
	tt.Frame()
	if len(a.visibleAlbums) != 0 {
		t.Fatal("removed album remains visible")
	}
}

func TestSongScreenLyricsModesAndNavigation(t *testing.T) {
	for _, size := range [][2]int{{820, 580}, {1080, 740}} {
		for _, dark := range []bool{false, true} {
			a := testApp(t, 5)
			a.prefs.Dark = dark
			a.setTheme()
			a.playFrom(0)
			a.lyricsPath = a.prefs.Current
			a.lyrics = parseLyrics("[00:01.00]<00:01.00>Hello <00:01.50>world\n[00:03.00]Next line")
			a.lyricMode = "Words"
			a.status.Position = 1200 * time.Millisecond
			tt := ui.NewTester(a.view, size[0], size[1])
			click(t, tt, "Open song screen")
			tt.Frame()
			tt.Frame()
			if !a.songScreen || !tt.HasText("Lyrics") {
				t.Fatal("song screen did not open")
			}
			for _, label := range []string{"Album cover art", "Get lyrics", "Static", "Timed", "Words", "Back to library", "Follow playback"} {
				r, ok := tt.Find(label)
				if !ok || r.W <= 0 || r.H <= 0 || r.X < 0 || r.Y < 0 || r.X+r.W > float32(size[0])+1 || r.Y+r.H > float32(size[1])+1 {
					t.Errorf("%dx%d %s out of bounds: %+v", size[0], size[1], label, r)
				}
			}
			click(t, tt, "Timed")
			click(t, tt, "Lyric line 1")
			if !slices.Equal(a.engine.(*fakeEngine).seeks, []time.Duration{time.Second}) {
				t.Fatal("lyric line seek failed")
			}
			click(t, tt, "Static")
			tt.Frame()
			if a.lyricMode != "Static" {
				t.Fatal(a.lyricMode)
			}
			click(t, tt, "Timed")
			tt.Frame()
			click(t, tt, "Words")
			tt.Frame()
			click(t, tt, "Back to library")
			tt.Frame()
			tt.Frame()
			if a.songScreen || !tt.HasText("TITLE / ARTIST") {
				t.Fatal("library did not return")
			}
			tt.Key(ui.Ctrl, ui.KeyL)
			tt.Frame()
			click(t, tt, "All songs")
			tt.Frame()
			tt.Frame()
			if a.songScreen {
				t.Fatal("sidebar did not leave song screen")
			}
		}
	}
}

func TestCompactSettingsWithManyFolders(t *testing.T) {
	a := testApp(t, 1)
	a.prefs.Folders = nil
	for i := range 200 {
		a.prefs.Folders = append(a.prefs.Folders, fmt.Sprintf("C:/Music/Collection %d", i))
	}
	tt := ui.NewTester(a.view, 820, 580)
	click(t, tt, "Settings")
	tt.Frame()
	tt.Frame()
	if tt.HasText("C:/Music/Collection 0") {
		t.Fatal("folders should start collapsed")
	}
	click(t, tt, "Music folders (200)")
	tt.Advance(300 * time.Millisecond)
	if !a.foldersOpen || !tt.HasText("C:/Music/Collection 0") || tt.HasText("C:/Music/Collection 199") {
		t.Fatal("expanded folders should use a bounded virtual list")
	}
	r, ok := tt.Find("Done")
	if !ok || r.Y < 0 || r.Y+r.H > 580 {
		t.Fatal("settings footer clipped", r)
	}
	click(t, tt, "Done")
	tt.Frame()
	tt.Frame()
	if a.settings {
		t.Fatal("settings did not close")
	}
}

func TestMiniPlayerResponsiveLayouts(t *testing.T) {
	sizes := [][2]int{{260, 180}, {300, 210}, {359, 339}, {360, 180}, {420, 210}, {639, 240}, {640, 180}, {819, 300}, {820, 180}, {260, 340}, {420, 600}, {640, 340}, {900, 500}, {1600, 900}}
	for _, dark := range []bool{false, true} {
		for _, empty := range []bool{false, true} {
			a := testApp(t, 3)
			a.prefs.Dark = dark
			a.setTheme()
			a.mini = true
			if !empty {
				a.playFrom(0)
				a.prefs.Tracks[0].Title = strings.Repeat("A very long song title ", 12)
				a.prefs.Tracks[0].Artist = strings.Repeat("A very long artist ", 8)
				a.status = audio.Status{Path: a.prefs.Current, Playing: true, Duration: 12 * time.Hour, Position: 2 * time.Hour}
			}
			a.notice = strings.Repeat("A long playback or refresh message. ", 20)
			tt := ui.NewTester(a.view, 420, 210)
			for _, size := range sizes {
				tt.SetSize(size[0], size[1])
				tt.Frame()
				tt.Frame()
				labels := []string{"Mini song title", "Mini album art", "Play / Pause", "Previous song", "Next song", "Seek", "Exit mini player", "Minimize window", "Close window"}
				if size[0] < 360 || size[0] >= 640 && size[0] < 820 && size[1] < 340 {
					if size[1] < 340 {
						labels = append(labels, "Volume controls")
					} else {
						labels = append(labels, "Volume")
					}
				} else {
					labels = append(labels, "Volume")
				}
				for _, label := range labels {
					r, ok := tt.Find(label)
					if !ok || r.W < 1 || r.H < 1 || r.X < -.5 || r.Y < -.5 || r.X+r.W > float32(size[0])+.5 || r.Y+r.H > float32(size[1])+.5 {
						t.Errorf("%v dark=%v empty=%v: %s outside window: %+v", size, dark, empty, label, r)
					}
				}
				for _, label := range []string{"Play / Pause", "Previous song", "Next song", "Exit mini player", "Volume", "Volume controls", "Seek"} {
					if r, ok := tt.Find(label); ok && (r.X < 11.5 || r.X+r.W > float32(size[0])-11.5) {
						t.Errorf("%v: %s extends into the player's side padding: %+v", size, label, r)
					}
				}
				if _, ok := tt.Find("Volume controls"); ok {
					click(t, tt, "Volume controls")
					tt.Frame()
					r, ok := tt.Find("Volume")
					if !ok || r.W < 40 || r.X < 0 || r.Y < 0 || r.X+r.W > float32(size[0])+.5 || r.Y+r.H > float32(size[1])+.5 {
						t.Errorf("%v: volume popover out of bounds: %+v", size, r)
					}
					tt.Key(0, ui.KeyEscape)
					tt.Frame()
				}
			}
		}
	}
}

func TestLyricTimingControls(t *testing.T) {
	a := testApp(t, 2)
	a.playFrom(0)
	a.lyricsPath = a.prefs.Current
	a.lyrics = parseLyrics("[00:01]<00:01>Hello <00:01.5>world\n[00:03]Next")
	a.lyricMode = "Words"
	a.songScreen = true
	a.status.Position = 1400 * time.Millisecond
	tt := ui.NewTester(a.view, 820, 580)
	click(t, tt, "Timing")
	tt.Frame()
	for range 5 {
		click(t, tt, "Later")
	}
	if a.lyricOffset() != 500*time.Millisecond || a.lyricPosition() != 900*time.Millisecond || a.lyricActive != -1 {
		t.Fatal("delay should apply to line and word playback", a.lyricOffset(), a.lyricPosition(), a.lyricActive)
	}
	tt.Key(0, ui.KeyEscape)
	click(t, tt, "Timed")
	click(t, tt, "Lyric line 1")
	if got := a.engine.(*fakeEngine).seeks; !slices.Equal(got, []time.Duration{1500 * time.Millisecond}) {
		t.Fatal("line seeking did not include the timing offset", got)
	}
	click(t, tt, "Timing")
	click(t, tt, "Reset")
	click(t, tt, "Earlier")
	if a.lyricOffset() != -100*time.Millisecond {
		t.Fatal(a.lyricOffset())
	}
	// The value can also be edited directly, including keyboard stepping.
	offsetField, ok := tt.Find("Lyric offset (ms)")
	if !ok {
		t.Fatal("missing numeric offset control")
	}
	// NumberInput contains the text field followed by its step buttons.
	tt.ClickAt(offsetField.X+24, offsetField.Y+offsetField.H/2)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("1250")
	tt.Key(0, ui.KeyEnter)
	if a.lyricOffset() != 1250*time.Millisecond {
		t.Fatal("numeric offset was not applied", a.lyricOffset())
	}
	click(t, tt, "Reset")
	click(t, tt, "Earlier")
	path := filepath.Join(t.TempDir(), "library.json")
	if err := savePreferences(path, a.prefs); err != nil {
		t.Fatal(err)
	}
	p, err := loadPreferences(path)
	if err != nil || p.Tracks[0].LyricOffsetMS != -100 || p.Tracks[1].LyricOffsetMS != 0 {
		t.Fatal(p, err)
	}
}

// Opt in to reproducible software-rendered captures for visual review.
func TestCaptureUI(t *testing.T) {
	dir := os.Getenv("MINIMUSE_CAPTURE_DIR")
	if dir == "" {
		t.Skip("set MINIMUSE_CAPTURE_DIR")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	a := testApp(t, 12)
	names := []string{"Weightless", "A Walk", "First Breath After Coma", "Near Light", "Open Eye Signal", "Says", "Awake", "An Ending (Ascent)", "Dayvan Cowboy", "Your Hand in Mine", "Kiara", "Ambre"}
	artists := []string{"Marconi Union", "Tycho", "Explosions in the Sky", "Ólafur Arnalds", "Jon Hopkins", "Nils Frahm", "Tycho", "Brian Eno", "Boards of Canada", "Explosions in the Sky", "Bonobo", "Nils Frahm"}
	albums := []string{"Weightless", "Dive", "The Earth Is Not a Cold Dead Place", "Living Room Songs", "Immunity", "Spaces", "Awake", "Apollo", "The Campfire Headphase", "The Earth Is Not a Cold Dead Place", "Black Sands", "Wintermusik"}
	for i := range a.prefs.Tracks {
		a.prefs.Tracks[i].Title = names[i]
		a.prefs.Tracks[i].Artist = artists[i]
		a.prefs.Tracks[i].Album = albums[i]
		a.prefs.Tracks[i].AlbumArtist = artists[i]
		a.prefs.Tracks[i].Duration = time.Duration(180+i*13) * time.Second
	}
	a.prefs.Tracks[1].Favorite = true
	a.reindex()
	a.refreshFilter()
	a.playFrom(1)
	a.status = audio.Status{Path: a.prefs.Current, Playing: true, Position: 83 * time.Second, Duration: 316 * time.Second, SampleRate: 44100, Bits: 24, Channels: 2}
	a.seek = 83
	for _, state := range []struct {
		name                  string
		w, h                  int
		dark, empty, settings bool
	}{{"library-light", 1080, 740, false, false, false}, {"library-dark", 1080, 740, true, false, false}, {"compact", 820, 580, false, false, false}, {"empty", 1080, 740, false, true, false}, {"settings", 1080, 740, false, false, true}} {
		b := newApp(a.prefs, "")
		defer b.cancel()
		b.prefs.Dark = state.dark
		b.setTheme()
		b.settings = state.settings
		b.status, b.seek = a.status, a.seek
		b.selected = 1
		if state.empty {
			b.filtered = nil
			b.prefs.Tracks = nil
			b.byPath = nil
			b.prefs.Current = ""
			b.status = audio.Status{}
		}
		tt := ui.NewTester(b.view, state.w, state.h)
		f, err := os.Create(filepath.Join(dir, state.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, state := range []struct {
		name               string
		mini, dark, albums bool
	}{{"mini-light", true, false, false}, {"mini-dark", true, true, false}, {"albums", false, false, true}, {"song-light", false, false, false}, {"song-dark", false, true, false}, {"song-compact", false, false, false}} {
		b := newApp(a.prefs, "")
		defer b.cancel()
		b.status, b.seek = a.status, a.seek
		b.mini = state.mini
		b.prefs.Dark = state.dark
		b.setTheme()
		w, h := 1080, 740
		if state.mini {
			w, h = miniWidth, miniHeight
		}
		if state.albums {
			b.section = "Albums"
			b.refreshFilter()
		}
		if strings.HasPrefix(state.name, "song-") {
			b.songScreen = true
			b.lyricsPath = b.prefs.Current
			b.lyrics = parseLyrics("[01:10.00]A little room for the music\n[01:18.00]Let the needle find its way\n[01:23.00]<01:23.00>Every <01:23.40>word <01:23.80>has <01:24.20>its <01:24.60>own <01:25.00>rhythm\n[01:30.00]And the evening drifts away\n[01:38.00]One more song before the morning\n[01:45.00]One more moment here to stay")
			b.lyricMode = "Words"
			b.lyrics.Source = "C:/Music/Tycho/Dive/02. A Walk.elrc"
			b.status.Position = 83*time.Second + 600*time.Millisecond
			if state.name == "song-compact" {
				w, h = 820, 580
			}
		}
		tt := ui.NewTester(b.view, w, h)
		f, err := os.Create(filepath.Join(dir, state.name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, size := range [][2]int{{260, 180}, {360, 210}, {640, 180}, {820, 210}, {260, 340}, {420, 600}, {640, 340}, {900, 500}} {
		b := newApp(a.prefs, "")
		defer b.cancel()
		b.mini = true
		b.status, b.seek = a.status, a.seek
		b.prefs.Tracks[1].Title = "A long song title that should fit gracefully at every size"
		tt := ui.NewTester(b.view, size[0], size[1])
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("mini-%dx%d.png", size[0], size[1])))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"folders-expanded", "lyric-timing"} {
		b := newApp(a.prefs, "")
		defer b.cancel()
		b.status, b.seek = a.status, a.seek
		if name == "folders-expanded" {
			b.prefs.Folders = nil
			for i := range 200 {
				b.prefs.Folders = append(b.prefs.Folders, fmt.Sprintf("C:/Music/Collection %03d — a long folder name", i))
			}
			b.settings, b.foldersOpen = true, true
		} else {
			b.songScreen, b.lyricTimingOpen = true, true
			b.lyricsPath = b.prefs.Current
			b.lyrics = parseLyrics("[01:10]The first line\n[01:23]The current line\n[01:30]The next line")
			b.lyricMode = "Timed"
		}
		tt := ui.NewTester(b.view, 820, 580)
		tt.Advance(300 * time.Millisecond)
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, tt.Image())
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}
