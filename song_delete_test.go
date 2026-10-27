package main

import (
	"fmt"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestRemoveSongMenu(t *testing.T) {
	for _, section := range []string{"All songs", "Favorites", "Queue", "playlist:Mix", "Albums"} {
		for _, tc := range []struct {
			name string
			n, i int
		}{
			{"first", 87, 0},
			{"middle", 3, 1},
			{"last", 3, 2},
			{"scrolled last", 87, 86},
			{"only", 1, 0},
		} {
			t.Run(section+"/"+tc.name, func(t *testing.T) {
				a := testApp(t, tc.n)
				var paths []string
				for i := range a.prefs.Tracks {
					a.prefs.Tracks[i].Favorite = true
					a.prefs.Tracks[i].Number = i + 1
					paths = append(paths, a.prefs.Tracks[i].Path)
				}
				a.prefs.Queue = slices.Clone(paths)
				a.prefs.Playlists = []playlist{{Name: "Mix", Paths: slices.Clone(paths)}}
				a.reindex()
				a.section = section
				if section == "Albums" {
					a.openAlbum(a.albums[0].Key)
				} else {
					a.refreshFilter()
				}
				a.play(paths[tc.i])
				state := &a.list
				if section == "Albums" {
					state = &a.albumTracks
				}
				state.ScrollTo(tc.i, ui.End)
				tt := ui.NewTester(a.view, 1080, 740)
				label := fmt.Sprintf("Actions for Song %05d", tc.i)
				click(t, tt, label)
				choice := "Remove from library"
				if section == "Queue" {
					choice = "Remove from queue"
				} else if section == "playlist:Mix" {
					choice = "Remove from playlist"
				}
				if err := tt.ChooseMenuItem(choice); err != nil {
					t.Fatal(err)
				}
				tt.Frame()
				tt.Frame()
				path := paths[tc.i]
				want := slices.Delete(slices.Clone(paths), tc.i, tc.i+1)
				var visible []string
				for _, i := range a.filtered {
					visible = append(visible, a.prefs.Tracks[i].Path)
				}
				if !slices.Equal(visible, want) {
					t.Fatalf("visible songs = %v, want %v", visible, want)
				}
				removedFromLibrary := section != "Queue" && section != "playlist:Mix"
				_, exists := a.byPath[pathKey(path)]
				if exists == removedFromLibrary {
					t.Fatalf("song in library = %v, library removal = %v", exists, removedFromLibrary)
				}
				wantStops := 1
				if section == "playlist:Mix" {
					wantStops = 0
				}
				if got := a.engine.(*fakeEngine).stops; got != wantStops {
					t.Fatalf("playback stopped %d times, want %d", got, wantStops)
				}
				if section == "playlist:Mix" || removedFromLibrary {
					if !slices.Equal(a.prefs.Playlists[0].Paths, want) {
						t.Fatal("playlist retained removed song", a.prefs.Playlists[0].Paths)
					}
				}
				if section == "Queue" || removedFromLibrary {
					if !slices.Equal(a.prefs.Queue, want) {
						t.Fatal("queue retained removed song", a.prefs.Queue)
					}
				}
				// Reuse the same list after it disappears, retaining its focus state.
				if tc.n == 1 {
					a.prefs.Tracks = append(a.prefs.Tracks, track{Path: "replacement.flac", Title: "Replacement", Favorite: true})
					a.prefs.Queue = []string{"replacement.flac"}
					a.prefs.Playlists[0].Paths = []string{"replacement.flac"}
					a.reindex()
					if section == "Albums" {
						a.openAlbum(a.albums[0].Key)
					} else {
						a.refreshFilter()
					}
					tt.Frame()
					click(t, tt, "Replacement")
					tt.Key(0, ui.KeyEnter)
					if a.prefs.Current != "replacement.flac" {
						t.Fatal("replacement song could not be played")
					}
				} else {
					state.ScrollTo(0, ui.Start)
					tt.Frame()
					click(t, tt, "Actions for "+a.prefs.Tracks[a.filtered[0]].Title)
					if err := tt.ChooseMenuItem("Play now"); err != nil {
						t.Fatal(err)
					}
					if a.prefs.Current != want[0] {
						t.Fatalf("played %q after removal, want %q", a.prefs.Current, want[0])
					}
				}
			})
		}
	}
}

func TestTrackMenuActions(t *testing.T) {
	for _, tc := range []struct {
		name, section string
		choice        []string
		wantQueue     []string
		wantPlaylist  []string
		wantPlay      bool
	}{
		{"play", "All songs", []string{"Play now"}, []string{"song-0.flac", "song-1.flac", "song-2.flac"}, nil, true},
		{"enqueue", "All songs", []string{"Add to queue"}, []string{"song-1.flac", "song-2.flac", "song-0.flac"}, nil, false},
		{"play next", "All songs", []string{"Play next"}, []string{"song-0.flac", "song-1.flac", "song-2.flac"}, nil, false},
		{"add to playlist", "All songs", []string{"Add to playlist", "Mix"}, []string{"song-1.flac", "song-2.flac"}, []string{"song-1.flac", "song-2.flac", "song-0.flac"}, false},
		{"move up", "Queue", []string{"Move up"}, []string{"song-0.flac", "song-1.flac", "song-2.flac"}, nil, false},
		{"move down", "Queue", []string{"Move down"}, []string{"song-1.flac", "song-2.flac", "song-0.flac"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t, 3)
			a.prefs.Queue = []string{"song-1.flac", "song-2.flac"}
			if tc.section == "Queue" {
				a.prefs.Queue = []string{"song-1.flac", "song-0.flac", "song-2.flac"}
			}
			a.prefs.Playlists = []playlist{{Name: "Mix", Paths: []string{"song-1.flac", "song-2.flac"}}}
			a.section = tc.section
			a.refreshFilter()
			tt := ui.NewTester(a.view, 1080, 740)
			click(t, tt, "Actions for Song 00000")
			if err := tt.ChooseMenuItem(tc.choice...); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			tt.Frame()
			if !slices.Equal(a.prefs.Queue, tc.wantQueue) {
				t.Fatalf("queue = %v, want %v", a.prefs.Queue, tc.wantQueue)
			}
			wantPlaylist := tc.wantPlaylist
			if wantPlaylist == nil {
				wantPlaylist = []string{"song-1.flac", "song-2.flac"}
			}
			if !slices.Equal(a.prefs.Playlists[0].Paths, wantPlaylist) {
				t.Fatalf("playlist = %v, want %v", a.prefs.Playlists[0].Paths, wantPlaylist)
			}
			var wantLoads []string
			if tc.wantPlay {
				wantLoads = []string{"song-0.flac"}
			}
			if got := a.engine.(*fakeEngine).loads; !slices.Equal(got, wantLoads) {
				t.Fatalf("loaded songs = %v, want %v", got, wantLoads)
			}
		})
	}
}
