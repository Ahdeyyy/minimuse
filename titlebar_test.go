package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTitlebarSearch(t *testing.T) {
	for _, size := range [][2]int{{820, 580}, {1080, 740}} {
		for _, dark := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/dark=%v", size[0], size[1], dark), func(t *testing.T) {
				a := testApp(t, 5)
				a.prefs.Dark = dark
				a.setTheme()
				tt := ui.NewTester(a.view, size[0], size[1])
				for _, section := range []string{"All songs", "Albums", "Favorites", "Queue", "playlist:Test"} {
					a.section = section
					a.refreshFilter()
					tt.Frame()
					search, ok := tt.Find("Search music")
					bar, _ := tt.Find("Window title bar")
					minimize, _ := tt.Find("Minimize window")
					if !ok || search.W < 200 || search.Y < bar.Y || search.Y+search.H > bar.Y+bar.H || search.X+search.W > minimize.X {
						t.Fatal("search is outside the title bar or overlaps window controls", section, search, bar, minimize)
					}
				}
				a.section = "All songs"
				a.refreshFilter()
				tt.Frame()
				tt.Key(ui.Ctrl, ui.KeyF)
				tt.Type("Song 00002")
				if len(a.filtered) != 1 || a.filtered[0] != 2 {
					t.Fatal("title bar search did not filter songs", a.query, a.filtered)
				}
				for _, screen := range []string{"playing", "settings", "playlist", "mini"} {
					a.songScreen = screen == "playing"
					a.settings = screen == "settings"
					a.playlistDialog = screen == "playlist"
					a.mini = screen == "mini"
					tt.Frame()
					tt.Frame()
					if _, ok := tt.Find("Search music"); ok {
						t.Fatal("search appeared on a screen without search", screen)
					}
					tt.Key(ui.Ctrl, ui.KeyF)
					if screen == "playing" && !a.songScreen {
						t.Fatal("hidden search shortcut left the song screen")
					}
				}
				a.songScreen, a.settings, a.playlistDialog, a.mini = false, false, false, false
				tt.Frame()
				tt.Key(ui.Ctrl, ui.KeyF)
				tt.Key(ui.Ctrl, ui.KeyA)
				tt.Type("Artist 1")
				if a.query != "Artist 1" || len(a.filtered) != 2 {
					t.Fatal("search did not recover focus after returning to the library", a.query, a.filtered)
				}
				captureUI(t, tt, fmt.Sprintf("titlebar-search-%dx%d-dark-%v", size[0], size[1], dark))
			})
		}
	}
}
