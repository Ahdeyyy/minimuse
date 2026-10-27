package main

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestReturnToCurrentLyric(t *testing.T) {
	for _, width := range []int{820, 1440} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := testApp(t, 1)
			a.playFrom(0)
			a.songScreen = true
			a.lyricsPath = a.prefs.Current
			var lyrics strings.Builder
			for i := 1; i <= 40; i++ {
				fmt.Fprintf(&lyrics, "[00:%02d]<00:%02d>Line %d with room for the music\n", i, i, i)
			}
			a.lyrics = parseLyrics(lyrics.String())
			a.lyricMode = "Words"
			a.status.Position = 16 * time.Second
			a.current().LyricOffsetMS = 1000
			tt := ui.NewTester(a.view, width, 740)
			button, ok := tt.Find("Return to current lyric")
			timing, _ := tt.Find("Timing")
			follow, _ := tt.Find("Follow playback")
			if !ok || button.X < timing.X+timing.W || button.X+button.W > follow.X || button.Y >= follow.Y+follow.H {
				t.Fatal("return button overlaps lyric controls", timing, button, follow)
			}
			captureUI(t, tt, fmt.Sprintf("lyrics-return-%d", width))
			f := a.engine.(*fakeEngine)
			before := len(f.seeks)
			for _, following := range []bool{true, false} {
				if a.followLyrics != following {
					click(t, tt, "Follow playback")
				}
				viewport, _ := tt.Find("Lyric lines")
				tt.Wheel(viewport.X+viewport.W/2, viewport.Y+viewport.H/2, 0, 1200)
				tt.Advance(time.Second)
				if first, _ := a.lyricList.Visible(); first <= 14 {
					t.Fatal("manual browsing did not scroll past the current lyric", first)
				}
				click(t, tt, "Return to current lyric")
				tt.Advance(time.Second)
				line, visible := tt.Find("Lyric line 15")
				viewport, _ = tt.Find("Lyric lines")
				if !visible || math.Abs(float64(line.Y+line.H/2-viewport.Y-viewport.H/2)) > 2 {
					t.Fatal("return did not center the lyric at the adjusted playback position", line, viewport)
				}
				if a.followLyrics != following || a.status.Position != 16*time.Second || len(f.seeks) != before {
					t.Fatal("return changed playback or the follow setting", a.followLyrics, a.status.Position, f.seeks)
				}
			}
			// Before the first lyric, clicking the disabled button does nothing.
			a.status.Position = 0
			tt.Frame()
			y := a.lyricScroll.Y
			click(t, tt, "Return to current lyric")
			tt.Advance(time.Second)
			if a.lyricScroll.Y != y {
				t.Fatal("return scrolled before the first lyric")
			}
			click(t, tt, "Static")
			tt.Frame()
			if _, visible := tt.Find("Return to current lyric"); visible {
				t.Fatal("return is visible in static mode")
			}
			click(t, tt, "Timed")
			a.status.Position = 16 * time.Second
			tt.Frame()
			click(t, tt, "Return to current lyric")
			tt.Advance(time.Second)
			if _, visible := tt.Find("Lyric line 15"); !visible {
				t.Fatal("return did not work after changing lyric modes")
			}
		})
	}
}
