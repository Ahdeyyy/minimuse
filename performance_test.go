package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"minimuse/internal/audio"
)

// Exercise real CPU rendering, layout and visible-row construction, including
// changing word highlights. Library size should not multiply frame cost.
func BenchmarkPlaybackFrame(b *testing.B) {
	for _, screen := range []string{"library", "lyrics", "mini"} {
		b.Run(screen, func(b *testing.B) {
			a := testApp(b, 10000)
			a.prefs.Current = a.prefs.Tracks[0].Path
			a.status = audio.Status{Path: a.prefs.Current, Playing: true, Duration: time.Hour}
			a.songScreen = screen == "lyrics"
			a.mini = screen == "mini"
			if a.songScreen {
				text := ""
				for i := range 200 {
					text += fmt.Sprintf("[%02d:%02d.00]<%02d:%02d.00>One <%02d:%02d.30>more <%02d:%02d.60>lyric line\n", i*2/60, i*2%60, i*2/60, i*2%60, i*2/60, i*2%60, i*2/60, i*2%60)
				}
				a.lyrics = parseLyrics(text)
				a.lyricsPath = a.prefs.Current
				a.lyricMode = "Words"
			}
			w, h := 1080, 740
			if a.mini {
				w, h = 420, 240
			}
			tt := ui.NewTester(a.view, w, h)
			tt.Frame()
			b.ReportAllocs()
			for b.Loop() {
				// Advance animation time too; Frame alone freezes springs and
				// accumulates uncompleted scrolling transitions unrealistically.
				tt.Advance(time.Second / 60)
				a.status.Position += time.Second / 60
				a.seek = a.status.Position.Seconds()
				tt.Frame()
			}
		})
	}
}

// Measure frames as a display paces them, without explicit refreshes: a
// Tester settles each Frame by running up to 20 frames while anything
// moves, and the turning record always moves while music plays. One
// second of playback per op.
func BenchmarkPlaybackSecond(b *testing.B) {
	for _, screen := range []string{"library", "lyrics"} {
		b.Run(screen, func(b *testing.B) {
			a := testApp(b, 10000)
			a.prefs.Current = a.prefs.Tracks[0].Path
			a.status = audio.Status{Path: a.prefs.Current, Playing: true, Duration: time.Hour}
			a.songScreen = screen == "lyrics"
			if a.songScreen {
				text := ""
				for i := range 200 {
					text += fmt.Sprintf("[%02d:%02d.00]<%02d:%02d.00>One <%02d:%02d.30>more <%02d:%02d.60>lyric line\n", i*2/60, i*2%60, i*2/60, i*2%60, i*2/60, i*2%60, i*2/60, i*2%60)
				}
				a.lyrics = parseLyrics(text)
				a.lyricsPath = a.prefs.Current
				a.lyricMode = "Words"
			}
			tt := ui.NewTester(a.view, 1080, 740)
			tt.Frame()
			b.ReportAllocs()
			for b.Loop() {
				// A playback sample, as the progress ticker delivers.
				a.status.Position += time.Second
				a.seek = a.status.Position.Seconds()
				tt.Frame()
				tt.Advance(time.Second)
			}
		})
	}
}
