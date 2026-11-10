package main

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"minimuse/internal/audio"
)

func TestVinylFollowsPlayback(t *testing.T) {
	a := testApp(t, 3)
	a.playFrom(0)
	a.status = audio.Status{Path: a.prefs.Current, Duration: 3 * time.Minute, Playing: true}
	var clock vinylClock
	tt := ui.NewTester(func(c *ui.Context) { clock = a.turnRecord(c) }, 100, 100)
	at := func(seconds float64) {
		t.Helper()
		a.seek = seconds
		a.status.Position = time.Duration(seconds * float64(time.Second))
		tt.Frame()
	}
	near := func(got, want float64) bool { return math.Abs(got-want) < 1e-6 }
	// Each sample keeps the record where it showed, then eases it to the
	// position's angle: it never jumps.
	continuous := func(before vinylClock) {
		t.Helper()
		if shown := before.value(clock.at); !near(clock.value(clock.at), shown) {
			t.Fatalf("record jumped from %v to %v", shown, clock.value(clock.at))
		}
	}
	at(10)
	start := a.vinylAngle
	tt.Advance(time.Second)
	at(11)
	if got := a.vinylAngle - start; got != vinylDegreesPerSecond || clock.rate != vinylDegreesPerSecond || clock.settle != vinylDrift {
		t.Fatalf("one second of playback turned %v degrees, %+v", got, clock)
	}
	if later := clock.at.Add(time.Second); !near(clock.value(later), a.vinylAngle+vinylDegreesPerSecond) {
		t.Fatal("record does not turn on between samples", clock.value(later))
	}
	// Rebuilding without a new sample keeps the clock.
	same := clock
	tt.Frame()
	if clock != same {
		t.Fatal("rebuild rebased the record", same, clock)
	}
	// Rewinding turns the record backwards, by at most a turn and a half.
	tt.Advance(time.Second)
	before := clock
	at(6)
	continuous(before)
	if clock.settle != vinylJump || clock.value(clock.at.Add(100*time.Millisecond)) >= clock.value(clock.at) {
		t.Fatal("rewind did not turn the record back", clock)
	}
	if settled := clock.at.Add(vinylJump); !near(clock.value(settled), a.vinylAngle+vinylDegreesPerSecond*vinylJump.Seconds()) {
		t.Fatal("rewind did not settle on the new position")
	}
	tt.Advance(time.Second)
	angle := a.vinylAngle
	at(170)
	if got := a.vinylAngle - angle; got != vinylMaxSpin {
		t.Fatal("long jump was not clamped", got)
	}
	// Scrubbing holds the record on the pointer's position.
	a.seeking = true
	tt.Advance(time.Second)
	before = clock
	at(160)
	continuous(before)
	if clock.rate != 0 || clock.settle != vinylScrub || !near(clock.value(clock.at.Add(vinylScrub)), a.vinylAngle) {
		t.Fatal("scrubbing back did not turn the record back", clock)
	}
	// Releasing the pointer spins the record up from rest.
	a.seeking = false
	tt.Advance(time.Second)
	at(160)
	if clock.rate != vinylDegreesPerSecond || clock.settle != vinylStop {
		t.Fatal("released record does not play on", clock)
	}
	// Pausing coasts to a stop; resuming starts from rest.
	tt.Advance(time.Second)
	a.status.Playing = false
	before = clock
	at(160)
	continuous(before)
	if clock.rate != 0 || clock.settle != vinylStop || !near(clock.value(clock.at.Add(vinylStop)), a.vinylAngle+vinylCoast) {
		t.Fatal("paused record does not coast to rest", clock)
	}
	tt.Advance(time.Second)
	a.status.Playing = true
	at(160)
	speed := (clock.value(clock.at.Add(time.Millisecond)) - clock.value(clock.at)) * 1000
	if speed < 0 || speed > 2 {
		t.Fatal("resumed record does not start from rest", speed)
	}
	// A new track restarts its position without spinning the record back.
	angle = a.vinylAngle
	a.status.Path = a.prefs.Tracks[1].Path
	tt.Advance(time.Second)
	at(0)
	if a.vinylAngle != angle {
		t.Fatal("track change spun the record", a.vinylAngle-angle)
	}
	// Repeating a song starts it again rather than rewinding it.
	at(9)
	angle = a.vinylAngle
	a.prefs.Repeat = 2
	a.advance(true)
	tt.Advance(time.Second)
	at(0)
	if a.vinylAngle != angle {
		t.Fatal("repeating the song spun the record back", a.vinylAngle-angle)
	}
}

func TestVinylRespectsReducedMotion(t *testing.T) {
	a := testApp(t, 1)
	a.playFrom(0)
	a.status = audio.Status{Path: a.prefs.Current, Duration: time.Minute, Playing: true}
	var clock vinylClock
	tt := ui.NewTesterWith(func(c *ui.Context) { clock = a.turnRecord(c) }, 100, 100, ui.TesterOptions{Preferences: ui.Preferences{ReduceMotion: true}})
	a.seek = 30
	tt.Frame()
	if clock.moving(time.Now()) || clock.value(time.Now()) != 0 {
		t.Fatal("record turns while the desktop asks for less motion", clock)
	}
}

func TestSongScreenMergesTransport(t *testing.T) {
	// Every animated frame renders in software, so cover the extremes only.
	for _, tc := range []struct {
		w, h int
		dark bool
	}{{820, 580, false}, {1440, 900, true}} {
		t.Run(fmt.Sprintf("%dx%d/dark=%v", tc.w, tc.h, tc.dark), func(t *testing.T) {
			a := testApp(t, 5)
			a.prefs.Dark = tc.dark
			a.setTheme()
			a.playFrom(0)
			a.status = audio.Status{Path: a.prefs.Current, Position: 40 * time.Second, Duration: 3 * time.Minute, Playing: true}
			a.seek = 40
			tt := ui.NewTester(a.view, tc.w, tc.h)
			tt.Frame()
			bar, ok := tt.Find("Playback controls")
			if !ok || bar.Y+bar.H > float32(tc.h) || bar.Y < float32(tc.h)/2 {
				t.Fatal("compact player is not at the bottom", bar)
			}
			if _, ok := tt.Find("Compact album art"); !ok {
				t.Fatal("compact player has no record")
			}
			name := fmt.Sprintf("%dx%d-dark-%v", tc.w, tc.h, tc.dark)
			captureUI(t, tt, "merge-library-"+name)

			click(t, tt, "Open song screen")
			for i := range 4 {
				tt.Advance(70 * time.Millisecond)
				captureUI(t, tt, fmt.Sprintf("merge-open-%d-%s", i, name))
			}
			tt.Advance(time.Second)
			captureUI(t, tt, "merge-song-"+name)
			if _, ok := tt.Find("Compact album art"); ok {
				t.Fatal("compact player still shows over the song screen")
			}
			docked, ok := tt.Find("Playback controls")
			lyrics, _ := tt.Find("Get lyrics")
			art, _ := tt.Find("Album cover art")
			if !ok || docked.Y+docked.H > float32(tc.h) || docked.Y < lyrics.Y || docked.Y < art.Y+art.H || docked.H >= bar.H {
				t.Fatal("controls did not merge below the song screen", docked, art, lyrics)
			}
			for _, label := range []string{"Play / Pause", "Previous song", "Next song", "Shuffle", "Seek", "Volume", "Mini player"} {
				r, ok := tt.Find(label)
				if !ok || r.W <= 0 || r.X < docked.X || r.X+r.W > docked.X+docked.W+1 || r.Y < docked.Y || r.Y+r.H > docked.Y+docked.H+1 {
					t.Errorf("%s is not inside the merged controls: %+v in %+v", label, r, docked)
				}
			}
			// The merged controls take input, uncovered by anything flying.
			click(t, tt, "Shuffle")
			if !a.prefs.Shuffle {
				t.Fatal("merged shuffle did not apply")
			}

			click(t, tt, "Back to library")
			for i := range 4 {
				tt.Advance(70 * time.Millisecond)
				captureUI(t, tt, fmt.Sprintf("merge-close-%d-%s", i, name))
			}
			tt.Advance(time.Second)
			if r, ok := tt.Find("Playback controls"); !ok || r != bar {
				t.Fatal("compact player did not return to its place", r, bar)
			}
		})
	}
}
