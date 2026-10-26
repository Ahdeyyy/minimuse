package main

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestPreferencesReplaceAndCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "library.json")
	p, err := loadPreferences(path)
	if err != nil || p.Volume != 80 || !p.CloseToTray {
		t.Fatal(p, err)
	}
	p.Tracks = []track{{Path: "test.flac", Title: "Moon", Artist: "Night"}}
	p.Playlists = []playlist{{Name: "Mix", Paths: []string{"test.flac"}}}
	if err = savePreferences(path, p); err != nil {
		t.Fatal(err)
	}
	p.Volume = 42
	if err = savePreferences(path, p); err != nil {
		t.Fatal("replace existing:", err)
	}
	got, err := loadPreferences(path)
	if err != nil || got.Volume != 42 || got.Tracks[0].search == "" || len(got.Playlists) != 1 {
		t.Fatal(got, err)
	}
	if err = os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadPreferences(path); err == nil {
		t.Fatal("corrupt settings accepted")
	}
}

func TestImportMetadataDeduplicationAndCancel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fallback.MP3")
	var frames []byte
	for _, item := range []struct{ id, value string }{{"TIT2", "Café at dawn"}, {"TPE1", "Música"}, {"TALB", "Quiet hours"}, {"TPE2", "Album artist"}, {"TRCK", "3/12"}, {"TPOS", "2/2"}} {
		h := make([]byte, 10)
		copy(h, item.id)
		binary.BigEndian.PutUint32(h[4:8], uint32(len(item.value)+1))
		frames = append(frames, h...)
		frames = append(frames, 3)
		frames = append(frames, []byte(item.value)...)
	}
	h := []byte{'I', 'D', '3', 3, 0, 0, 0, 0, byte(len(frames) >> 7), byte(len(frames) & 127)}
	if err := os.WriteFile(path, append(h, frames...), 0600); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("ignored"), 0600)
	tracks, _, err := scanTracks(context.Background(), []string{dir, path}, map[string]bool{})
	if err != nil || len(tracks) != 1 {
		t.Fatal(tracks, err)
	}
	if tracks[0].Title != "Café at dawn" || tracks[0].Artist != "Música" || tracks[0].Album != "Quiet hours" {
		t.Fatal(tracks[0])
	}
	if tracks[0].AlbumArtist != "Album artist" || tracks[0].Number != 3 || tracks[0].Disc != 2 {
		t.Fatal("album tags not read", tracks[0])
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := scanTracks(ctx, []string{dir}, map[string]bool{}); err == nil {
		t.Fatal("canceled import continued")
	}
}

func TestPlaylistsDoNotKeepRemovedTracks(t *testing.T) {
	a := testApp(t, 3)
	a.playFrom(0)
	a.playlistName = "Evening"
	a.savePlaylist()
	a.remove("song-1.flac", true)
	if len(a.prefs.Playlists[0].Paths) != 2 {
		t.Fatal(a.prefs.Playlists)
	}
}
