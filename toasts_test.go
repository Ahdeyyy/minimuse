package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestNotificationToastsDoNotMoveContent(t *testing.T) {
	for _, size := range [][2]int{{260, 180}, {420, 210}, {820, 580}, {1080, 740}} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/dark=%v", size[0], size[1], dark), func(t *testing.T) {
				a := testApp(t, 3)
				a.mini = size[0] < 820
				a.prefs.Dark = dark
				a.setTheme()
				a.playFrom(0)
				tt := ui.NewTester(a.view, size[0], size[1])
				labels := []string{"Window title bar", "Play / Pause", "Seek"}
				if a.mini {
					labels = append(labels, "Mini song title", "Mini album art")
				} else {
					labels = append(labels, "Search music", "Song 00000")
				}
				before := make(map[string]ui.Rect)
				for _, label := range labels {
					r, ok := tt.Find(label)
					if !ok {
						t.Fatal("missing control before notification", label)
					}
					before[label] = r
				}
				a.notice = strings.Repeat("A long playback or folder refresh message. ", 20)
				tt.Frame()
				tt.Frame()
				for label, want := range before {
					if got, ok := tt.Find(label); !ok || got != want {
						t.Errorf("notification moved %s: got %+v, want %+v", label, got, want)
					}
				}
				toast, ok := tt.Find("Notification toast")
				if !ok || toast.Y < 0 || toast.X < 0 || toast.X+toast.W > float32(size[0]) || toast.Y+toast.H > float32(size[1]) {
					t.Fatal("toast outside the content area", toast)
				}
				if a.mini {
					minimize, _ := tt.Find("Minimize window")
					if toast.Y+toast.H > 42 || toast.X+toast.W > minimize.X {
						t.Fatal("compact toast overlaps playback or window controls", toast, minimize)
					}
				} else if toast.Y < 42 {
					t.Fatal("library toast overlaps the title bar", toast)
				}
				captureUI(t, tt, fmt.Sprintf("toast-%dx%d-dark-%v", size[0], size[1], dark))
				click(t, tt, "Dismiss message")
				tt.Frame()
				tt.Frame()
				if _, ok := tt.Find("Notification toast"); ok {
					t.Fatal("dismissed toast returned on a subsequent build")
				}
			})
		}
	}
}

func TestNotificationToastReplacesAndExpires(t *testing.T) {
	a := testApp(t, 3)
	a.notice = "Reading your music…"
	tt := ui.NewTester(a.view, 820, 580)
	tt.Move(0, 0)
	tt.Advance(time.Second)
	a.notice = "Added 3 songs"
	// Autoplay must not erase an import result before the UI displays it.
	a.playFrom(0)
	tt.Frame()
	if !tt.HasText("Added 3 songs") || tt.HasText("Reading your music…") {
		t.Fatal("completion must replace the progress toast", tt.Texts())
	}
	tt.Advance(7 * time.Second)
	tt.Frame()
	if !tt.HasText("Added 3 songs") {
		t.Fatal("replacement toast did not get a fresh timeout")
	}
	tt.Advance(2 * time.Second)
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("Notification toast"); ok {
		t.Fatal("toast did not expire")
	}
	// Repeating the same action after expiration should show a new toast.
	a.notice = "Added 3 songs"
	tt.Frame()
	if !tt.HasText("Added 3 songs") {
		t.Fatal("repeated notification was lost")
	}
}

func TestLyricLoadingNotificationDoesNotMoveContent(t *testing.T) {
	a := testApp(t, 1)
	a.playFrom(0)
	a.songScreen = true
	a.lyricsPath = a.prefs.Current
	a.lyrics = parseLyrics("[00:01]First line\n[00:03]Second line")
	a.lyricMode = "Timed"
	tt := ui.NewTester(a.view, 820, 580)
	before, ok := tt.Find("Lyric controls")
	if !ok {
		t.Fatal("lyric controls missing")
	}
	a.lyricsBusy = true
	a.notice = "Checking local lyrics, then LRCLIB…"
	tt.Frame()
	if after, ok := tt.Find("Lyric controls"); !ok || after != before {
		t.Fatal("loading notification moved lyric controls", before, after)
	}
	if _, ok := tt.Find("Notification toast"); !ok {
		t.Fatal("lyric progress notification is not a toast")
	}
}
