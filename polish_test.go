package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"minimuse/internal/audio"
)

func TestCrossfadePreferencesAndSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	if err := os.WriteFile(path, []byte(`{"Volume":80}`), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := loadPreferences(path)
	if err != nil || p.Crossfade || p.CrossfadeSeconds != 4 {
		t.Fatal("older libraries should default to crossfade off with a four-second duration", p, err)
	}
	a := newApp(p, path)
	defer a.cancel()
	f := &fakeEngine{}
	a.engine = f
	a.settings = true
	tt := ui.NewTester(a.view, 820, 580)
	click(t, tt, "Crossfade between tracks")
	if !a.prefs.Crossfade || f.crossfade != 4*time.Second {
		t.Fatal("crossfade toggle did not configure playback", a.prefs.Crossfade, f.crossfade)
	}
	click(t, tt, "Increase")
	if a.prefs.CrossfadeSeconds != 4.5 || f.crossfade != 4500*time.Millisecond {
		t.Fatal("duration control did not configure playback", a.prefs.CrossfadeSeconds, f.crossfade)
	}
	a.prefs.CrossfadeSeconds = 7.5
	a.crossfadeChanged()
	if f.crossfade != 7500*time.Millisecond {
		t.Fatal(f.crossfade)
	}
	if err := savePreferences(path, a.prefs); err != nil {
		t.Fatal(err)
	}
	restored, err := loadPreferences(path)
	if err != nil || !restored.Crossfade || restored.CrossfadeSeconds != 7.5 {
		t.Fatal(restored, err)
	}
	click(t, tt, "Crossfade between tracks")
	if f.crossfade != 0 {
		t.Fatal("disabling crossfade did not reach the engine")
	}
	click(t, tt, "Done")
	tt.Frame()
	click(t, tt, "Settings")
	tt.Frame()
}

func TestCrossfadeQueueBoundaries(t *testing.T) {
	for _, mode := range []string{"sequential", "repeat-one", "repeat-all", "shuffle", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			a := testApp(t, 3)
			a.prefs.Crossfade = mode != "disabled"
			a.playFrom(0)
			if mode == "repeat-one" {
				a.prefs.Repeat = 2
			}
			if mode == "shuffle" {
				a.prefs.Shuffle = true
			}
			if mode == "repeat-all" {
				a.prefs.Repeat = 1
				a.play("song-2.flac")
			}
			old := a.prefs.Current
			event := audio.Status{Path: old, Playing: true, NearEnd: true}
			a.acceptStatus(event)
			if mode == "repeat-one" || mode == "disabled" {
				if a.prefs.Current != old {
					t.Fatal("advanced before playback ended")
				}
				a.acceptStatus(audio.Status{Path: old, Ended: true})
				want := "song-1.flac"
				if mode == "repeat-one" {
					want = old
				}
				if a.prefs.Current != want {
					t.Fatal(a.prefs.Current)
				}
				return
			}
			if a.prefs.Current == old {
				t.Fatal("did not start the next song during the overlap")
			}
			next := a.prefs.Current
			a.acceptStatus(audio.Status{Path: old, Ended: true})
			if a.prefs.Current != next {
				t.Fatal("stale completion skipped the new song")
			}
			if mode == "sequential" {
				a.play("song-2.flac")
				a.acceptStatus(audio.Status{Path: "song-2.flac", Playing: true, NearEnd: true})
				if a.prefs.Current != "song-2.flac" {
					t.Fatal("repeat off wrapped the queue")
				}
			}
		})
	}
}

func TestLyricsPolishLayoutAndMotion(t *testing.T) {
	for _, width := range []int{820, 1080, 1440} {
		a := testApp(t, 2)
		a.playFrom(0)
		a.songScreen = true
		a.lyricsPath = a.prefs.Current
		a.lyrics = parseLyrics("[00:01]<00:01>Hello <00:01.5>world\n[00:03]Next line")
		a.lyrics.Source = "song-0.elrc"
		a.lyricMode = "Words"
		a.status.Position = 1200 * time.Millisecond
		tt := ui.NewTester(a.view, width, 740)
		follow, ok := tt.Find("Follow playback")
		timing, _ := tt.Find("Timing")
		if !ok || follow.X < timing.X+timing.W || follow.Y >= timing.Y+timing.H {
			t.Fatalf("%d: follow must sit to the right of lyric toggles: %+v %+v", width, follow, timing)
		}
		controls, _ := tt.Find("Lyric controls")
		if gap := controls.X + controls.W - follow.X - follow.W; gap < -1 || gap > 1 {
			t.Fatalf("%d: follow must align with the right edge: %+v %+v", width, controls, follow)
		}
		if !tt.HasText("Local") || tt.HasText("/  your music, at home") {
			t.Fatal("missing source badge or old title bar tagline")
		}
		if tt.HasText("minimuse") || tt.HasText("Local · song-0.elrc") || tt.HasText("Local lyrics") {
			t.Fatal("sidebar branding or lyric source footer is still visible")
		}
		click(t, tt, "Follow playback")
		if a.followLyrics {
			t.Fatal("follow toggle did not turn off")
		}
		// Advance across a word, a line, and a mode change while motion is
		// running to catch transient element lifetime and identity mistakes.
		a.status.Position = 1600 * time.Millisecond
		tt.Frame()
		tt.Advance(70 * time.Millisecond)
		a.status.Position = 3100 * time.Millisecond
		tt.Frame()
		tt.Advance(120 * time.Millisecond)
		click(t, tt, "Static")
		tt.Advance(time.Second)
		click(t, tt, "Words")
		tt.Advance(time.Second)
	}
}
