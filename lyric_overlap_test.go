package main

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The overlapping passage and standalone backing vocal from the supplied LRC.
const overlappingLyrics = `[03:47.911]v1:<03:47.911>Shoot <03:48.332>your <03:48.682>best <03:49.065><03:49.324>shot <03:49.960>
[03:48.875]v1:<03:48.875>I <03:49.277>see <03:49.645>right <03:49.994>through <03:50.346>your <03:50.762>ri<03:51.462>ddle <03:52.162>
[bg:<03:50.369>I <03:50.892>dri<03:51.591>bble <03:52.292>it <03:52.890>]
[03:53.518]Next line`

func activeLyricWords(d lyricDocument, at time.Duration) []string {
	var words []string
	for _, line := range d.Lines {
		for i, word := range line.Words {
			if line.wordActive(i, at) {
				words = append(words, strings.TrimSpace(word.Text))
			}
		}
	}
	return words
}

func TestOverlappingLyricVocals(t *testing.T) {
	d := parseLyrics(overlappingLyrics)
	if !d.WordTimed || len(d.Lines) != 4 || !d.Lines[2].Background || d.Lines[2].Text != "I dribble it " {
		t.Fatal("backing vocal was lost or left format markers in the text", d)
	}
	for _, tc := range []struct {
		ms   int
		want []string
	}{
		{227900, nil},
		{229100, []string{"I"}}, // Empty inline stamp releases "best".
		{229500, []string{"shot", "see"}},
		{230500, []string{"your", "I"}},
		{232500, []string{"it"}}, // Backing vocal outlasts the lead.
		{232900, nil},
		{229500, []string{"shot", "see"}}, // Seeking backward restores both.
	} {
		if got := activeLyricWords(d, time.Duration(tc.ms)*time.Millisecond); !slices.Equal(got, tc.want) {
			t.Errorf("%d ms: got %v, want %v", tc.ms, got, tc.want)
		}
	}
	if d.Lines[0].singing(d.Lines[0].End) || d.Lines[2].singing(d.Lines[2].End) {
		t.Fatal("explicit release stamps must be exclusive")
	}
	if got := d.active(230500 * time.Millisecond); got != 1 {
		t.Fatal("backing vocal moved the follow target away from the lead", got)
	}
}

func TestOverlappingLyricBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, lrc string
		at        time.Duration
		want      []string
	}{
		{"same word stamp", "[00:01]<00:01>One <00:01>two <00:02>next<00:03>", time.Second, []string{"One", "two"}},
		{"same line stamp", "[00:01]<00:01>Lead<00:03>\n[00:01]<00:01>Harmony<00:02>", time.Second, []string{"Lead", "Harmony"}},
		{"line timed duet", "[00:01]Lead\n[00:01]Harmony\n[00:03]Next", time.Second, nil},
		{"out of order", "[00:02]<00:02>Harmony<00:04>\n[00:01]<00:01>Lead<00:03>", 2500 * time.Millisecond, []string{"Lead", "Harmony"}},
		{"repeated offset", "[offset:+250]\n[00:01][00:05]<00:01>Lead<00:03>\n[00:06]<00:06>Harmony<00:08>", 6500 * time.Millisecond, []string{"Lead", "Harmony"}},
		{"repeated release", "[offset:+250]\n[00:01][00:05]<00:01>Lead<00:03>", 7250 * time.Millisecond, nil},
		{"clamped release", "[offset:-4000]\n[00:01]<00:01>Lead<00:03>", 0, nil},
		{"clamped starts", "[offset:-2500]\n[00:01]<00:01>One <00:02>two <00:03>next<00:04>", 0, []string{"two"}},
		{"inferred end", "[00:01]<00:01>Lead\n[bg:<00:02>Harmony<00:04>]\n[00:03]Next", 2500 * time.Millisecond, []string{"Lead", "Harmony"}},
		{"known words outlast next line", "[00:01]<00:01>Lead <00:03>continues\n[00:02]<00:02>Harmony<00:04>\n[00:05]Next", 3500 * time.Millisecond, []string{"continues", "Harmony"}},
		{"background only", "[BG:<00:01>Harmony<00:02>]", 1500 * time.Millisecond, []string{"Harmony"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := parseLyrics(tc.lrc)
			if got := activeLyricWords(d, tc.at); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if tc.name == "line timed duet" && (!d.Lines[0].singing(tc.at) || !d.Lines[1].singing(tc.at) || d.Lines[0].singing(3*time.Second)) {
				t.Fatal("line-timed duet did not share the next distinct line boundary", d)
			}
		})
	}
}

func TestOverlappingLyricsRenderingAndSeeking(t *testing.T) {
	for _, width := range []int{820, 1440} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			a := testApp(t, 1)
			a.playFrom(0)
			a.songScreen = true
			a.lyricsPath = a.prefs.Current
			a.lyrics = parseLyrics(overlappingLyrics)
			a.lyricMode = "Words"
			a.status.Position = 229500 * time.Millisecond
			a.status.Duration = 6 * time.Minute
			tt := ui.NewTester(a.view, width, 740)
			tt.Advance(time.Second)
			assertLyricAccent(t, tt, "Lyric line 1 word 5", a.theme.Accent, true)
			assertLyricAccent(t, tt, "Lyric line 2 word 2", a.theme.Accent, true)
			captureUI(t, tt, fmt.Sprintf("lyrics-overlap-%d", width))
			a.status.Position = 230500 * time.Millisecond
			tt.Frame()
			tt.Advance(time.Second)
			assertLyricAccent(t, tt, "Lyric line 1 word 5", a.theme.Accent, false)
			assertLyricAccent(t, tt, "Lyric line 2 word 5", a.theme.Accent, true)
			assertLyricAccent(t, tt, "Lyric line 3 word 1", a.theme.Accent, true)
			captureUI(t, tt, fmt.Sprintf("lyrics-backing-overlap-%d", width))
			if a.lyricActive != 1 {
				t.Fatal("backing vocal interrupted follow", a.lyricActive)
			}
			a.current().LyricOffsetMS = 250
			tt.Frame()
			for _, label := range []string{"Lyric line 1 word 5", "Lyric line 2 word 2", "Lyric line 3 word 1"} {
				click(t, tt, label)
			}
			f := a.engine.(*fakeEngine)
			for i, word := range []lyricWord{a.lyrics.Lines[0].Words[4], a.lyrics.Lines[1].Words[1], a.lyrics.Lines[2].Words[0]} {
				if len(f.seeks) != 3 || f.seeks[i] != word.At+250*time.Millisecond {
					t.Fatal("overlapping word seek lost its independent timing", f.seeks)
				}
			}
			click(t, tt, "Static")
			tt.Advance(time.Second)
			click(t, tt, "Words")
			a.status.Position = 229750 * time.Millisecond
			tt.Frame()
			tt.Advance(time.Second)
			assertLyricAccent(t, tt, "Lyric line 1 word 5", a.theme.Accent, true)
			assertLyricAccent(t, tt, "Lyric line 2 word 2", a.theme.Accent, true)
		})
	}
}

// Check the rendered glyphs, so these regressions also catch a view that still
// restricts accent highlighting to just the line used as the scroll target.
func assertLyricAccent(t *testing.T, tt *ui.Tester, label string, accent ui.Color, want bool) {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing word %q", label)
	}
	img := tt.Image()
	count := 0
	near := func(a, b uint8) bool { return math.Abs(float64(a)-float64(b)) <= 4 }
	for y := int(math.Floor(float64(r.Y))); y < int(math.Ceil(float64(r.Y+r.H))); y++ {
		for x := int(math.Floor(float64(r.X))); x < int(math.Ceil(float64(r.X+r.W))); x++ {
			p := img.RGBAAt(x, y)
			if near(p.R, accent.R) && near(p.G, accent.G) && near(p.B, accent.B) {
				count++
			}
		}
	}
	if got := count >= 3; got != want {
		t.Fatalf("%q: accent pixels = %d, want highlighted = %v", label, count, want)
	}
}
