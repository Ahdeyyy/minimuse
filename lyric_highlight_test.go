package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestFastOverlappingLineTailsHighlightImmediately(t *testing.T) {
	a := testApp(t, 1)
	a.playFrom(0)
	a.songScreen = true
	a.lyricsPath = a.prefs.Current
	a.lyrics = parseLyrics("[00:01]<00:01>First <00:01.980>tail<00:02>\n[00:01.990]<00:01.990>Other<00:02.020>\n[00:03]Next")
	a.lyricMode = "Words"
	a.status.Duration = time.Minute
	a.status.Position = 1999 * time.Millisecond
	tt := ui.NewTester(a.view, 1000, 740)
	tt.Advance(time.Second)
	assertLyricAccent(t, tt, "Lyric line 1 word 2", a.theme.Accent, true)
	assertLyricAccent(t, tt, "Lyric line 2 word 1", a.theme.Accent, true)
	// Both highlights survive the release stamps on the next playback update.
	a.status.Position = 2040 * time.Millisecond
	tt.Frame()
	assertLyricAccent(t, tt, "Lyric line 1 word 2", a.theme.Accent.Mix(a.theme.Text, .75), true)
	assertLyricAccent(t, tt, "Lyric line 2 word 1", a.theme.Accent.Mix(a.theme.Text, .75), true)
	captureUI(t, tt, "lyrics-fast-tails")
	a.status.Position = 2200 * time.Millisecond
	tt.Frame()
	assertLyricAccent(t, tt, "Lyric line 1 word 2", a.theme.Accent.Mix(a.theme.Text, .75), true)
	a.status.Position = 1970 * time.Millisecond
	tt.Frame()
	assertLyricAccent(t, tt, "Lyric line 1 word 2", a.theme.Accent, false)
}

func TestCurrentWordsHaveDistinctColorsWithoutBackground(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(fmt.Sprint("dark=", dark), func(t *testing.T) {
			a := testApp(t, 1)
			a.prefs.Dark = dark
			a.setTheme()
			a.playFrom(0)
			a.songScreen, a.lyricMode, a.lyricsPath = true, "Words", a.prefs.Current
			a.lyrics = parseLyrics("[00:01]<00:01>Done <00:02>Now <00:03>Later<00:04>\n[bg:<00:02>I<00:02.300>]")
			a.status.Duration, a.status.Position = time.Minute, 2*time.Second
			tt := ui.NewTester(a.view, 1000, 740)
			tt.Advance(time.Second)
			activeWord, _ := tt.Find("Lyric line 1 word 2")
			// At the exact onset, even a single-letter backing vocal must
			// already have a distinct foreground, without a selection rectangle.
			for _, label := range []string{"Lyric line 1 word 2", "Lyric line 2 word 1"} {
				assertLyricAccent(t, tt, label, a.theme.Accent.Mix(a.theme.Text, .25), true)
			}
			assertLyricAccent(t, tt, "Lyric line 1 word 1", a.theme.Accent.Mix(a.theme.Text, .75), true)
			assertLyricAccent(t, tt, "Lyric line 1 word 3", a.theme.TextMuted, true)
			a.status.Position = 2400 * time.Millisecond
			tt.Frame()
			assertLyricAccent(t, tt, "Lyric line 2 word 1", a.theme.Accent.Mix(a.theme.Text, .75), true)
			captureUI(t, tt, fmt.Sprintf("lyrics-current-word-dark-%v", dark))
			a.status.Position = 1500 * time.Millisecond
			tt.Frame()
			tt.Advance(time.Second)
			assertLyricAccent(t, tt, "Lyric line 1 word 2", a.theme.TextMuted, true)
			idleWord, _ := tt.Find("Lyric line 1 word 2")
			if activeWord.W < idleWord.W*1.05 {
				t.Fatal("current word did not visibly enlarge", activeWord, idleWord)
			}
		})
	}
}

func TestWordProgressBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, lrc string
		word      int
		ms        int
		want      float32
	}{
		{"future", "[00:01]<00:01>word<00:02>", 0, 999, 0},
		{"start", "[00:01]<00:01>word<00:02>", 0, 1000, 0},
		{"halfway", "[00:01]<00:01>word<00:02>", 0, 1500, .5},
		{"released", "[00:01]<00:01>word<00:02>", 0, 2000, 1},
		{"gap after release", "[00:01]<00:01>word<00:02>", 0, 5000, 1},
		{"empty stamp", "[00:01]<00:01>word<00:02>", 1, 5000, 0},
		{"same stamp one", "[00:01]<00:01>one <00:01>two<00:02>", 0, 1500, .5},
		{"same stamp two", "[00:01]<00:01>one <00:01>two<00:02>", 1, 1500, .5},
		{"negative offset completed", "[offset:-1500]\n[00:01]<00:01>one <00:02>two<00:03>", 0, 0, 0},
		{"negative offset half", "[offset:-1500]\n[00:01]<00:01>one <00:02>two<00:03>", 0, 250, .5},
		{"clamped completed", "[offset:-2500]\n[00:01]<00:01>one <00:02>two<00:03>", 0, 0, 1},
		{"open end halfway", "[00:01]<00:01>word", 0, 1300, .5},
		{"open end finished", "[00:01]<00:01>word", 0, 2000, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line := parseLyrics(tc.lrc).Lines[0]
			if got := line.wordProgress(tc.word, time.Duration(tc.ms)*time.Millisecond); math.Abs(float64(got-tc.want)) > .0001 {
				t.Fatalf("progress = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestKaraokeFillKeepsGraphemesAndCompletedColor(t *testing.T) {
	fill := newLyricWordFill("e\u0301👩‍🎤Go ")
	if len(fill.spans) != 5 || fill.sung != 4 {
		t.Fatal("split a grapheme or counted trailing whitespace", fill)
	}
	base, accent := ui.RGB(40, 40, 40), ui.RGB(160, 80, 240)
	spans := fill.paint(.375, base, accent)
	if spans[0].Color != accent || spans[1].Color != base.Mix(accent, .5) || spans[2].Color != base {
		t.Fatal("fill did not interpolate across graphemes", spans)
	}
	for _, progress := range []float32{1, 0, 1} {
		spans = fill.paint(progress, base, accent)
		var text strings.Builder
		for _, span := range spans {
			text.WriteString(span.Text)
			if strings.TrimSpace(span.Text) != "" && span.Color != base.Mix(accent, progress) {
				t.Fatal("stale fill after seek", span)
			}
		}
		if text.String() != "e\u0301👩‍🎤Go " {
			t.Fatal("fill changed the lyric text")
		}
	}
}

func TestRootLineTailSurvivesFrameGap(t *testing.T) {
	data, err := os.ReadFile("testdata/root-overlap.lrc")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{820, 1440} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := testApp(t, 1)
			a.playFrom(0)
			a.songScreen, a.lyricMode, a.lyricsPath = true, "Words", a.prefs.Current
			a.lyrics = parseLyrics(string(data))
			a.status.Duration, a.status.Position = 7*time.Minute, 278400*time.Millisecond
			tt := ui.NewTester(a.view, width, 740)
			tt.Advance(time.Second)
			// No frame occurs during "sword", or its 10 ms gap to the next line.
			a.status.Position = 279800 * time.Millisecond
			tt.Frame()
			assertLyricAccent(t, tt, "Lyric line 1 word 13", a.theme.Accent.Mix(a.theme.Text, .75), true)
			captureUI(t, tt, fmt.Sprintf("root-frame-gap-%d", width))
			// Overlapping backing vocals continue filling after the lead moves on.
			a.status.Position = 285 * time.Second
			tt.Frame()
			tt.Advance(time.Second)
			assertLyricAccent(t, tt, "Lyric line 3 word 3", a.theme.Accent, true)
			captureUI(t, tt, fmt.Sprintf("root-overlap-%d", width))
			// Seeking backwards must clear completed fill, with no stale animation.
			a.status.Position = 278400 * time.Millisecond
			tt.Frame()
			tt.Advance(time.Second)
			assertLyricAccent(t, tt, "Lyric line 1 word 13", a.theme.Accent, false)
		})
	}
}

func TestWordHighlightSurvivesSkippedTimingWindow(t *testing.T) {
	d := parseLyrics("[00:01]<00:01>First <00:01.980>tail<00:02>\n[00:01.990]<00:01.990>Other<00:02.020>")
	for _, tc := range []struct{ line, word int }{{0, 1}, {1, 0}} {
		line := d.Lines[tc.line]
		if line.wordProgress(tc.word, 1970*time.Millisecond) != 0 || line.wordProgress(tc.word, 2040*time.Millisecond) != 1 || line.wordProgress(tc.word, 10*time.Second) != 1 {
			t.Fatal("fast word skipped or completed highlight was erased", tc)
		}
	}
}
