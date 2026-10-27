package main

import (
	"testing"
	"time"

	"minimuse/internal/audio"
)

func TestSeekBurstDefersLibraryCopiesAndPauseSavesImmediately(t *testing.T) {
	a := testApp(t, 10000)
	a.saves = make(chan preferences, 1)
	t.Cleanup(func() {
		a.closed.Store(true)
		if a.saveTimer != nil {
			a.saveTimer.Stop()
		}
	})
	a.prefs.Current = a.prefs.Tracks[0].Path
	a.prefs.Playing = true
	a.status = audio.Status{Path: a.prefs.Current, Playing: true, Duration: time.Hour}
	for i := range 100 {
		s := a.status
		s.Position = time.Duration(i) * time.Second
		a.acceptStatus(s)
	}
	if len(a.saves) != 0 || a.saveTimer == nil || a.prefs.Position != 99*time.Second {
		t.Fatal("seeking saved the library on every event or lost the position")
	}
	s := a.status
	s.Playing = false
	// A progress callback may already have sampled the paused engine before
	// its notification runs. The persisted session still needs to be flushed.
	a.status = s
	a.acceptStatus(s)
	if len(a.saves) != 1 || a.saveTimer != nil {
		t.Fatal("pause failed to flush pending changes")
	}
	p := <-a.saves
	if p.Playing || p.Position != 99*time.Second {
		t.Fatal("saved playback state", p.Position, p.Playing)
	}
	a.prefs.Tracks[0].Favorite = true
	if p.Tracks[0].Favorite {
		t.Fatal("queued save retained mutable library storage")
	}
}
