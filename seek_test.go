package main

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSeekDragIsSilentAndCommitsOnRelease(t *testing.T) {
	a := testApp(t, 1)
	a.playFrom(0)
	a.status.Duration = time.Minute
	f := a.engine.(*fakeEngine)
	tt := ui.NewTester(a.view, 1000, 740)
	r, ok := tt.Find("Seek")
	if !ok {
		t.Fatal("missing seek slider")
	}
	y := r.Y + r.H/2
	tt.Press(r.X+r.W/4, y)
	tt.Move(r.X+r.W/2, y)
	if !a.seeking || !slices.Equal(f.scrubs, []bool{true}) || len(f.seeks) != 0 {
		t.Fatal("drag should silence playback without seeking", a.seeking, f.scrubs, f.seeks)
	}
	position := a.seek
	a.acceptStatus(a.status)
	tt.Frame()
	if a.seek != position {
		t.Fatal("playback status overwrote the drag position")
	}
	// Leaving the slider must keep playback silent until pointer release.
	tt.Move(r.X+r.W+20, y)
	if !a.seeking || len(f.seeks) != 0 {
		t.Fatal("leaving slider resumed playback")
	}
	tt.Release(r.X+r.W+20, y)
	if a.seeking || !slices.Equal(f.scrubs, []bool{true, false}) || len(f.seeks) != 1 {
		t.Fatal("release should commit once and end scrubbing", a.seeking, f.scrubs, f.seeks)
	}
}
