package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"minimuse/internal/audio"
)

type track struct {
	Path          string
	Title         string
	Artist        string
	Album         string
	AlbumArtist   string
	Number        int
	Disc          int
	Favorite      bool
	Duration      time.Duration
	MediaVersion  int
	LyricOffsetMS float64 `json:",omitempty"`
	search        string
}

func (t *track) index() {
	t.search = strings.ToLower(t.Title + "\x00" + t.Artist + "\x00" + t.Album + "\x00" + t.AlbumArtist)
}
func pathKey(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(filepath.Clean(path))
	}
	return filepath.Clean(path)
}
func supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".flac", ".wav", ".ogg", ".oga":
		return true
	}
	return false
}
func codec(path string) string { return strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")) }

// Import reads metadata and duration on a worker without decoding full songs
// into memory. It does not follow directory symlinks.
func scanTracks(ctx context.Context, paths []string, known map[string]bool) ([]track, int, error) {
	var found []track
	skipped := 0
	var firstErr error
	add := func(path string, info fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			skipped++
			if firstErr == nil {
				firstErr = err
			}
			return nil
		}
		if info.IsDir() || !info.Type().IsRegular() || !supported(path) {
			return nil
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			skipped++
			return nil
		}
		key := pathKey(abs)
		if known[key] {
			return nil
		}
		known[key] = true
		found = append(found, readTrack(abs))
		return nil
	}
	for _, path := range paths {
		if err := filepath.WalkDir(path, add); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil, skipped, err
			}
			skipped++
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	slices.SortFunc(found, func(a, b track) int { return strings.Compare(a.search, b.search) })
	return found, skipped, firstErr
}

func readTrack(path string) track {
	t := track{Path: path, Title: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), Artist: "Unknown artist", Album: filepath.Base(filepath.Dir(path))}
	f, err := os.Open(path)
	if err != nil {
		t.index()
		return t
	}
	defer f.Close()
	var header [10]byte
	_, err = io.ReadFull(f, header[:])
	if err == nil && string(header[:3]) == "ID3" && (header[3] == 3 || header[3] == 4) && header[5]&0xc0 == 0 {
		size := synchsafe(header[6:10])
		// Skip artwork and unknown frames. Only tiny text frames allocate.
		r := bufio.NewReader(io.LimitReader(f, int64(size)))
		for used := 0; used+10 <= size; {
			var h [10]byte
			if _, err := io.ReadFull(r, h[:]); err != nil {
				break
			}
			used += 10
			n := int(binary.BigEndian.Uint32(h[4:8]))
			if header[3] == 4 {
				n = synchsafe(h[4:8])
			}
			if n <= 0 || n > size-used {
				break
			}
			used += n
			id := string(h[:4])
			if (id == "TIT2" || id == "TPE1" || id == "TALB" || id == "TPE2" || id == "TRCK" || id == "TPOS") && n <= 16384 && h[9] == 0 {
				b := make([]byte, n)
				if _, err := io.ReadFull(r, b); err != nil {
					break
				}
				setTag(&t, id, decodeText(b))
			} else {
				if _, err := io.CopyN(io.Discard, r, int64(n)); err != nil {
					break
				}
			}
		}
	} else if string(header[:4]) == "fLaC" {
		f.Seek(4, io.SeekStart)
		for block := 0; block < 128; block++ {
			var h [4]byte
			if _, err := io.ReadFull(f, h[:]); err != nil {
				break
			}
			n := int(h[1])<<16 | int(h[2])<<8 | int(h[3])
			if h[0]&127 == 4 && n <= 256*1024 {
				b := make([]byte, n)
				if _, err := io.ReadFull(f, b); err != nil {
					break
				}
				readVorbisComments(&t, b)
				break
			}
			if h[0]&128 != 0 {
				break
			}
			if _, err := f.Seek(int64(n), io.SeekCurrent); err != nil {
				break
			}
		}
	} else if strings.EqualFold(filepath.Ext(path), ".ogg") || strings.EqualFold(filepath.Ext(path), ".oga") {
		// Vorbis comments are near the beginning. Read at most 64 KiB, once.
		f.Seek(0, io.SeekStart)
		b, _ := io.ReadAll(io.LimitReader(f, 64*1024))
		// Strip Ogg page headers/lacing to reconstruct the initial packets.
		var packets []byte
		for len(b) >= 27 && string(b[:4]) == "OggS" {
			segments := int(b[26])
			if len(b) < 27+segments {
				break
			}
			n := 0
			for _, v := range b[27 : 27+segments] {
				n += int(v)
			}
			if len(b) < 27+segments+n {
				break
			}
			packets = append(packets, b[27+segments:27+segments+n]...)
			b = b[27+segments+n:]
		}
		if i := strings.Index(string(packets), "\x03vorbis"); i >= 0 {
			readVorbisComments(&t, packets[i+7:])
		}
	}
	t.index()
	if decoder, format, err := audio.Decode(path); err == nil {
		t.Duration = format.SampleRate.D(decoder.Len())
		decoder.Close()
	}
	t.MediaVersion = 1
	return t
}

func synchsafe(b []byte) int {
	return int(b[0]&127)<<21 | int(b[1]&127)<<14 | int(b[2]&127)<<7 | int(b[3]&127)
}
func decodeText(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	enc := b[0]
	b = b[1:]
	var s string
	switch enc {
	case 0:
		var out strings.Builder
		for _, v := range b {
			out.WriteRune(rune(v))
		}
		s = out.String()
	case 1, 2:
		var order binary.ByteOrder = binary.BigEndian
		if enc == 1 && len(b) >= 2 {
			if b[0] == 255 && b[1] == 254 {
				order = binary.LittleEndian
			}
			if b[0] == 255 || b[0] == 254 {
				b = b[2:]
			}
		}
		u := make([]uint16, 0, len(b)/2)
		for len(b) >= 2 {
			u = append(u, order.Uint16(b))
			b = b[2:]
		}
		s = string(utf16.Decode(u))
	case 3:
		s = string(b)
	}
	return strings.TrimSpace(strings.Trim(s, "\x00"))
}
func setTag(t *track, id, value string) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", " / "))
	if value == "" {
		return
	}
	switch id {
	case "TIT2", "TITLE":
		t.Title = value
	case "TPE1", "ARTIST":
		t.Artist = value
	case "TALB", "ALBUM":
		t.Album = value
	case "TPE2", "ALBUMARTIST", "ALBUM ARTIST":
		t.AlbumArtist = value
	case "TRCK", "TRACKNUMBER":
		n, _, _ := strings.Cut(value, "/")
		t.Number, _ = strconv.Atoi(strings.TrimSpace(n))
	case "TPOS", "DISCNUMBER":
		n, _, _ := strings.Cut(value, "/")
		t.Disc, _ = strconv.Atoi(strings.TrimSpace(n))
	}
}
func readVorbisComments(t *track, b []byte) {
	if len(b) < 4 {
		return
	}
	n := int(binary.LittleEndian.Uint32(b))
	b = b[4:]
	if n > len(b) {
		return
	}
	b = b[n:]
	if len(b) < 4 {
		return
	}
	count := int(binary.LittleEndian.Uint32(b))
	b = b[4:]
	for i := 0; i < count && len(b) >= 4; i++ {
		n = int(binary.LittleEndian.Uint32(b))
		b = b[4:]
		if n > len(b) {
			return
		}
		if n <= 16384 {
			key, value, ok := strings.Cut(string(b[:n]), "=")
			if ok {
				setTag(t, strings.ToUpper(key), value)
			}
		}
		b = b[n:]
	}
}

type playlist struct {
	Name  string
	Paths []string
}

type preferences struct {
	Folders          []string
	Tracks           []track
	Queue            []string
	Current          string
	Volume           float64
	Shuffle          bool
	Repeat           int // 0 off, 1 all, 2 one
	Dark             bool
	CloseToTray      bool
	Crossfade        bool
	CrossfadeSeconds float64
	Playlists        []playlist
}

func loadPreferences(path string) (preferences, error) {
	p := preferences{Volume: 80, CloseToTray: true, CrossfadeSeconds: 4}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err = json.Unmarshal(b, &p); err != nil {
		return preferences{Volume: 80, CloseToTray: true, CrossfadeSeconds: 4}, fmt.Errorf("could not read saved library: %w", err)
	}
	p.Volume = max(0, min(100, p.Volume))
	p.CrossfadeSeconds = max(1, min(12, p.CrossfadeSeconds))
	if p.Repeat < 0 || p.Repeat > 2 {
		p.Repeat = 0
	}
	for i := range p.Tracks {
		p.Tracks[i].LyricOffsetMS = max(-300000, min(300000, p.Tracks[i].LyricOffsetMS))
		p.Tracks[i].index()
	}
	return p, nil
}

// Atomic replacement protects the last good library against interrupted writes.
func savePreferences(path string, p preferences) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "library-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, path)
}
