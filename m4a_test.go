package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"minimuse/internal/audio"
)

func TestM4ATrackMetadata(t *testing.T) {
	path := filepath.Join("internal", "audio", "testdata", "tone.m4a")
	if !supported(path) || !supported("SONG.M4A") {
		t.Fatal("M4A not recognized")
	}
	song := readTrack(path)
	if song.Title != "M4A Tone" || song.Artist != "MiniMuse" || song.Album != "Codec tests" || song.AlbumArtist != "MiniMuse ensemble" || song.Number != 2 || song.Disc != 1 || song.Duration != time.Second {
		t.Fatalf("metadata: %+v", song)
	}
}

func TestM4AArtworkAndMalformedAtoms(t *testing.T) {
	atom := func(name string, b []byte) []byte {
		h := make([]byte, 8)
		binary.BigEndian.PutUint32(h, uint32(len(b)+8))
		copy(h[4:], name)
		return append(h, b...)
	}
	image := []byte("test image")
	data := atom("data", append(make([]byte, 8), image...))
	b := atom("moov", atom("udta", atom("meta", append(make([]byte, 4), atom("ilst", atom("covr", data))...))))
	path := filepath.Join(t.TempDir(), "art.m4a")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(embeddedArtwork(path), image) {
		t.Fatal("missing artwork")
	}
	if audio.M4ATags(bytes.NewReader(b), 0)["covr"] != nil {
		t.Fatal("artwork read during metadata import")
	}
	if audio.M4ATags(bytes.NewReader(b), 2)["covr"] != nil {
		t.Fatal("artwork size limit ignored")
	}
	for i := 0; i < len(b); i++ {
		audio.M4ATags(bytes.NewReader(b[:i]), 100)
	}
}
