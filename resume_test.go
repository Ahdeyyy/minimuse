package main

import (
	"path/filepath"
	"testing"
	"time"

	"minimuse/internal/audio"
)

func TestPlaybackResumesAfterShutdown(t *testing.T) {
	for _, playing := range []bool{false, true} {
		t.Run(map[bool]string{false: "paused", true: "playing"}[playing], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "library.json")
			p := preferences{Current: "song.flac", Tracks: []track{{Path: "song.flac", Duration: 3 * time.Minute}}}
			a := newApp(p, path)
			// Shutdown must use the worker's final snapshot, including commands
			// completed after the last UI notification and while hidden.
			final := audio.Status{Path: p.Current, Position: 83456 * time.Millisecond, Playing: playing}
			a.engine = &fakeEngine{snapshot: audio.Status{Path: p.Current, Position: time.Second}, closeSnapshot: &final}
			a.visible = false
			a.saves = make(chan preferences, 1)
			a.saved = make(chan struct{})
			errs := make(chan error, 1)
			go func() {
				defer close(a.saved)
				for p := range a.saves {
					if err := savePreferences(path, p); err != nil {
						errs <- err
					}
				}
			}()
			a.shutdown()
			select {
			case err := <-errs:
				t.Fatal(err)
			default:
			}
			got, err := loadPreferences(path)
			if err != nil {
				t.Fatal(err)
			}
			b := newApp(got, path)
			defer b.cancel()
			engine := &fakeEngine{}
			b.engine = engine
			b.restorePlayback()
			if len(engine.restores) != 1 || engine.restores[0] != final || b.seek != final.Position.Seconds() {
				t.Fatalf("restored %v, seek %v; want %v", engine.restores, b.seek, final)
			}
		})
	}
}

func TestResumePositionLifecycle(t *testing.T) {
	a := testApp(t, 2)
	a.prefs.Current = a.prefs.Tracks[0].Path
	a.prefs.Position = 83 * time.Second
	a.prefs.Playing = true
	a.rememberPlayback(audio.Status{Path: "stale.flac", Position: time.Minute})
	if a.prefs.Position != 83*time.Second || !a.prefs.Playing {
		t.Fatal("stale snapshot replaced the bookmark")
	}
	a.prefs.Tracks[0].Duration = time.Minute
	a.restorePlayback()
	if a.prefs.Position != 0 {
		t.Fatal("out-of-range resume was not reset")
	}
	a.prefs.Position = 20 * time.Second
	a.play(a.prefs.Tracks[1].Path)
	if a.prefs.Position != 0 || !a.prefs.Playing {
		t.Fatal("new song inherited the old position")
	}
	a.rememberPlayback(audio.Status{Path: a.prefs.Current, Position: time.Minute, Ended: true})
	if a.prefs.Position != 0 || a.prefs.Playing {
		t.Fatal("completed session should reopen paused at the beginning")
	}
	a.prefs.Position = time.Second
	a.remove(a.prefs.Current, true)
	if a.prefs.Position != 0 || a.prefs.Playing {
		t.Fatal("removed song retained a resume bookmark")
	}
}
