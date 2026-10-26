package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnhancedLyricsVoiceMarkers(t *testing.T) {
	for _, prefix := range []string{"v1:", "v2: ", "[v1]", "<v1>", "<00:01.00>v1: ", "[v12:]"} {
		d := parseLyrics("[00:01.00]" + prefix + "<00:01.00>Hello <00:01.50>world")
		if !d.WordTimed || d.Lines[0].Text != "Hello world" {
			t.Fatalf("%q: %+v", prefix, d)
		}
		var words string
		for _, word := range d.Lines[0].Words {
			words += word.Text
		}
		if words != "Hello world" {
			t.Fatalf("%q left a voice marker in words: %q", prefix, words)
		}
	}
	d := parseLyrics("[00:01]I wrote v1: yesterday")
	if d.Lines[0].Text != "I wrote v1: yesterday" {
		t.Fatal("stripped actual lyric text", d)
	}
}

func TestLyricsFormatsAndSeekBoundaries(t *testing.T) {
	plain := parseLyrics("\ufeffFirst line\r\n\r\nSecond line\r\n")
	if plain.Timed || plain.WordTimed || len(plain.Lines) != 3 || plain.Lines[2].Text != "Second line" {
		t.Fatal(plain)
	}
	d := parseLyrics("[ar:Artist]\n[offset:-100]\n[00:03.000]Third\n[00:01.20][00:05.20]<00:01.20>Hello <00:01.700>world<00:02.000>\n[00:02.00]\n")
	if !d.Timed || !d.WordTimed || len(d.Lines) != 4 {
		t.Fatal(d)
	}
	if d.Lines[0].At != 1100*time.Millisecond || d.Lines[0].Text != "Hello world" || d.Lines[0].Words[1].At != 1600*time.Millisecond {
		t.Fatal(d.Lines[0])
	}
	if d.Lines[3].Words[1].At != 5600*time.Millisecond {
		t.Fatal("repeated line word offset", d.Lines[3])
	}
	for _, tc := range []struct {
		position time.Duration
		want     int
	}{{0, -1}, {1100 * time.Millisecond, 0}, {1900 * time.Millisecond, 1}, {2900 * time.Millisecond, 2}, {10 * time.Second, 3}, {time.Second, -1}} {
		if got := d.active(tc.position); got != tc.want {
			t.Errorf("%v: got %d, want %d", tc.position, got, tc.want)
		}
	}
	if got := decodeLyrics([]byte{255, 254, 'H', 0, 'i', 0}); got != "Hi" {
		t.Fatal(got)
	}
	malformed := parseLyrics("[00:99]bad\n[00:02.00]Valid\n[not:lyrics]\n")
	if len(malformed.Lines) != 1 || malformed.Lines[0].Text != "Valid" {
		t.Fatal(malformed)
	}
}

func TestLocalLyricsPrecedence(t *testing.T) {
	dir := t.TempDir()
	song := track{Path: filepath.Join(dir, "Song.flac"), Title: "Title", Artist: "Artist"}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("Song.txt", "Static lyrics")
	write("SONG.LRC", "[00:01.00]Timed lyrics")
	d, err := localLyrics(song)
	if err != nil || !d.Timed || !strings.HasSuffix(d.Source, "SONG.LRC") {
		t.Fatal(d, err)
	}
	write("Song.elrc", "[00:01.00]<00:01.00>Word <00:01.50>lyrics")
	d, err = localLyrics(song)
	if err != nil || !d.WordTimed {
		t.Fatal(d, err)
	}
	if err := os.Remove(filepath.Join(dir, "Song.elrc")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "SONG.LRC")); err != nil {
		t.Fatal(err)
	}
	d, err = localLyrics(song)
	if err != nil || d.Timed || d.Lines[0].Text != "Static lyrics" {
		t.Fatal(d, err)
	}
}

func TestLRCLIBDownloadSaveErrorsAndPreserveLocal(t *testing.T) {
	song := track{Path: filepath.Join(t.TempDir(), "test.flac"), Title: "A & B", Artist: "Artist / One", Album: "Album + Two", Duration: 183500 * time.Millisecond}
	status := http.StatusOK
	record := lrclibRecord{SyncedLyrics: "[00:01.00]Downloaded line"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/get" || r.URL.Query().Get("track_name") != song.Title || r.URL.Query().Get("artist_name") != song.Artist || r.URL.Query().Get("duration") != "183.500" || r.Header.Get("User-Agent") == "" {
			t.Error("incorrect request", r.URL)
		}
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(record)
	}))
	defer server.Close()
	d, err := downloadLyrics(context.Background(), server.Client(), server.URL, song)
	if err != nil || !d.Timed || filepath.Ext(d.Source) != ".lrc" {
		t.Fatal(d, err)
	}
	local, err := localLyrics(song)
	if err != nil || local.Lines[0].Text != "Downloaded line" {
		t.Fatal(local, err)
	}
	if err := os.WriteFile(d.Source, []byte("[00:01]User edited line"), 0600); err != nil {
		t.Fatal(err)
	}
	d, err = downloadLyrics(context.Background(), server.Client(), server.URL, song)
	if err != nil || d.Lines[0].Text != "User edited line" {
		t.Fatal("overwrote local lyrics", d, err)
	}
	status = http.StatusNotFound
	if _, err := downloadLyrics(context.Background(), server.Client(), server.URL, song); err == nil {
		t.Fatal("404 accepted")
	}
	status = http.StatusTooManyRequests
	if _, err := downloadLyrics(context.Background(), server.Client(), server.URL, song); err == nil {
		t.Fatal("rate limit accepted")
	}
	status = http.StatusOK
	record = lrclibRecord{Instrumental: true}
	if _, err := downloadLyrics(context.Background(), server.Client(), server.URL, song); err == nil {
		t.Fatal("instrumental accepted")
	}
	record = lrclibRecord{PlainLyrics: "Plain lyrics"}
	song.Path = filepath.Join(t.TempDir(), "plain.mp3")
	d, err = downloadLyrics(context.Background(), server.Client(), server.URL, song)
	if err != nil || d.Timed || filepath.Ext(d.Source) != ".txt" {
		t.Fatal(d, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := downloadLyrics(ctx, server.Client(), server.URL, song); err == nil {
		t.Fatal("cancel ignored")
	}
}
