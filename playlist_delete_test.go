package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDeletePlaylistContextMenu(t *testing.T) {
	for _, tc := range []struct {
		name      string
		playlists []string
		selected  string
		deleted   string
		want      []string
		section   string
	}{
		{"last", []string{"Mix"}, "Mix", "Mix", nil, "All songs"},
		{"first", []string{"Mix", "Evening", "Weekend"}, "Mix", "Mix", []string{"Evening", "Weekend"}, "All songs"},
		{"middle", []string{"Mix", "Evening", "Weekend"}, "Evening", "Evening", []string{"Mix", "Weekend"}, "All songs"},
		{"other", []string{"Mix", "Evening"}, "Mix", "Evening", []string{"Mix"}, "playlist:Mix"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, 3)
			for _, name := range tc.playlists {
				a.prefs.Playlists = append(a.prefs.Playlists, playlist{Name: name, Paths: []string{"song-0.flac"}})
			}
			tt := ui.NewTester(a.view, 1080, 740)
			click(t, tt, tc.selected)
			if err := tt.RightClick(tc.deleted); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Delete playlist"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			tt.Frame()
			var got []string
			for _, p := range a.prefs.Playlists {
				got = append(got, p.Name)
			}
			if !slices.Equal(got, tc.want) || a.section != tc.section {
				t.Fatalf("playlists %v, section %q; want %v, %q", got, a.section, tc.want, tc.section)
			}
			wantTracks := 1
			if tc.section == "All songs" {
				wantTracks = 3
			}
			if len(a.filtered) != wantTracks {
				t.Fatalf("filtered tracks = %d, want %d", len(a.filtered), wantTracks)
			}
			if len(tc.want) == 0 {
				if !tt.HasText("No playlists") {
					t.Fatal("missing empty playlist message")
				}
				// Reuse the list after it disappears, including its retained focus state.
				a.prefs.Queue = []string{"song-1.flac"}
				a.playlistName = "Replacement"
				a.savePlaylist()
				tt.Frame()
				click(t, tt, "Replacement")
				tt.Frame()
			} else {
				click(t, tt, tc.want[len(tc.want)-1])
				tt.Frame()
			}
		})
	}
}
