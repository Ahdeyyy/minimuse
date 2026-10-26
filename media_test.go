package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDurationsAndFolderRefresh(t *testing.T) {
	dir := t.TempDir()
	copyTone := func(name string) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("internal", "audio", "testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	copyTone("tone.mp3")
	first, _, err := scanTracks(context.Background(), []string{dir}, map[string]bool{})
	if err != nil || len(first) != 1 || first[0].Duration <= 0 {
		t.Fatal(first, err)
	}
	copyTone("tone.flac")
	copyTone("tone.ogg")
	newTracks, _, err := scanTracks(context.Background(), []string{dir, dir}, map[string]bool{pathKey(first[0].Path): true})
	if err != nil || len(newTracks) != 2 {
		t.Fatal(newTracks, err)
	}
	for _, track := range newTracks {
		if track.Duration <= 0 {
			t.Error("missing duration", track)
		}
	}
	p := preferences{Tracks: first, Folders: []string{dir}}
	path := filepath.Join(t.TempDir(), "library.json")
	if err := savePreferences(path, p); err != nil {
		t.Fatal(err)
	}
	got, err := loadPreferences(path)
	if err != nil || len(got.Folders) != 1 || got.Folders[0] != dir || got.Tracks[0].Duration != first[0].Duration {
		t.Fatal(got, err)
	}
}

func TestEmbeddedAndFolderArtwork(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	img.Set(2, 2, color.RGBA{R: 200, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	payload := append([]byte{3}, []byte("image/png\x00\x03\x00")...)
	payload = append(payload, data...)
	frame := make([]byte, 10)
	copy(frame, "APIC")
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
	body := append(frame, payload...)
	header := []byte{'I', 'D', '3', 3, 0, 0, byte(len(body) >> 21 & 127), byte(len(body) >> 14 & 127), byte(len(body) >> 7 & 127), byte(len(body) & 127)}
	path := filepath.Join(dir, "embedded.mp3")
	if err := os.WriteFile(path, append(header, body...), 0600); err != nil {
		t.Fatal(err)
	}
	if got := embeddedArtwork(path); !bytes.Equal(got, data) {
		t.Fatal("ID3 art mismatch")
	}
	if loadArtwork(path) == nil {
		t.Fatal("embedded artwork not decoded")
	}
	var picture bytes.Buffer
	for _, n := range []uint32{3, 9} {
		binary.Write(&picture, binary.BigEndian, n)
	}
	picture.WriteString("image/png")
	binary.Write(&picture, binary.BigEndian, uint32(0))
	for _, n := range []uint32{8, 8, 24, 0, uint32(len(data))} {
		binary.Write(&picture, binary.BigEndian, n)
	}
	picture.Write(data)
	if !bytes.Equal(flacPicture(picture.Bytes()), data) {
		t.Fatal("FLAC picture mismatch")
	}
	block := picture.Bytes()
	flac := append([]byte("fLaC"), []byte{0x86, byte(len(block) >> 16), byte(len(block) >> 8), byte(len(block))}...)
	flac = append(flac, block...)
	path = filepath.Join(dir, "embedded.flac")
	os.WriteFile(path, flac, 0600)
	if !bytes.Equal(embeddedArtwork(path), data) {
		t.Fatal("FLAC embedded mismatch")
	}
	var comments bytes.Buffer
	binary.Write(&comments, binary.LittleEndian, uint32(0))
	binary.Write(&comments, binary.LittleEndian, uint32(1))
	comment := "METADATA_BLOCK_PICTURE=" + base64.StdEncoding.EncodeToString(picture.Bytes())
	binary.Write(&comments, binary.LittleEndian, uint32(len(comment)))
	comments.WriteString(comment)
	packet := append([]byte("\x03vorbis"), comments.Bytes()...)
	var ogg bytes.Buffer
	for _, p := range [][]byte{[]byte("\x01vorbis"), packet} {
		var h [27]byte
		copy(h[:], "OggS")
		h[26] = byte(len(p)/255 + 1)
		ogg.Write(h[:])
		for n := len(p); n >= 255; n -= 255 {
			ogg.WriteByte(255)
		}
		ogg.WriteByte(byte(len(p) % 255))
		ogg.Write(p)
	}
	path = filepath.Join(dir, "embedded.ogg")
	os.WriteFile(path, ogg.Bytes(), 0600)
	if !bytes.Equal(embeddedArtwork(path), data) {
		t.Fatal("Ogg embedded mismatch")
	}
	if err := os.WriteFile(filepath.Join(dir, "COVER.PNG"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if loadArtwork(filepath.Join(dir, "no-embedded.wav")) == nil {
		t.Fatal("folder artwork missing")
	}
	if artworkBitmap([]byte("bad image")) != nil || flacPicture([]byte{1, 2, 3}) != nil {
		t.Fatal("invalid artwork accepted")
	}
}

func TestVinylIconTransparency(t *testing.T) {
	for _, path := range []string{"resources/icon.png", "resources/icon-32.png"} {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		_, _, _, corner := img.At(0, 0).RGBA()
		_, _, _, center := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
		if corner != 0 || center == 0 {
			t.Fatalf("%s: expected transparent background and visible record", path)
		}
	}
}
