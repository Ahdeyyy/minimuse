package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"minimuse/internal/audio"
)

func TestRefreshRemovesMissingSourcesAndReconcilesLibrary(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "deleted")
	live := filepath.Join(root, "music")
	other := filepath.Join(root, "music-other")
	if err := os.Mkdir(live, 0700); err != nil {
		t.Fatal(err)
	}
	song := filepath.Join(live, "keep.flac")
	if err := os.WriteFile(song, []byte("present"), 0600); err != nil {
		t.Fatal(err)
	}
	tracks := []track{{Path: filepath.Join(missing, "gone.flac")}, {Path: song, Favorite: true}, {Path: filepath.Join(live, "deleted-subfolder", "gone.flac")}, {Path: filepath.Join(other, "outside.flac")}}
	a := newApp(preferences{Folders: []string{missing, live, other}, Tracks: tracks, Current: tracks[0].Path, Queue: []string{tracks[0].Path, song}, Playlists: []playlist{{Name: "Mix", Paths: []string{tracks[0].Path, song}}}}, "")
	defer a.cancel()
	a.engine = &fakeEngine{}
	a.status = audio.Status{Path: tracks[0].Path, Playing: true}
	a.seek = 10
	available, folders, songs := missingMusic(context.Background(), []string{missing, live}, tracks)
	if !slices.Equal(available, []string{live}) || len(folders) != 1 || len(songs) != 2 || songs[pathKey(tracks[3].Path)] {
		t.Fatal(available, folders, songs)
	}
	a.pruneMissingMusic(folders, songs)
	a.reindex()
	a.refreshFilter()
	if len(a.prefs.Folders) != 2 || len(a.prefs.Tracks) != 2 || !a.prefs.Tracks[0].Favorite || a.prefs.Current != "" || a.status.Playing || a.seek != 0 || a.engine.(*fakeEngine).stops != 1 {
		t.Fatal("stale music or playback remains", a.prefs, a.status)
	}
	if !slices.Equal(a.prefs.Queue, []string{song}) || !slices.Equal(a.prefs.Playlists[0].Paths, []string{song}) {
		t.Fatal("references to removed songs remain", a.prefs)
	}
	// Refreshing unrelated sources must leave the current song playing.
	a.prefs.Current = song
	a.status = audio.Status{Path: song, Playing: true, Position: time.Second}
	a.pruneMissingMusic(nil, nil)
	if !a.status.Playing || a.engine.(*fakeEngine).stops != 1 {
		t.Fatal("refresh disturbed existing playback")
	}
}

func TestRefreshRetainsSourcesOnOtherErrors(t *testing.T) {
	// A path through a regular file produces ENOTDIR, not a confirmed absence.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(file, "inaccessible")
	_, err := os.Stat(root)
	if os.IsNotExist(err) {
		t.Skip("platform reports ENOTDIR as absence")
	}
	available, folders, songs := missingMusic(context.Background(), []string{root}, []track{{Path: filepath.Join(root, "song.flac")}})
	if len(available) != 1 || len(folders) != 0 || len(songs) != 0 {
		t.Fatal("removed a source on an I/O error", available, folders, songs)
	}
}
