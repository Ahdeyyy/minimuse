package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

const maxArtworkBytes = 16 << 20

type artEntry struct {
	bitmap  *ui.Bitmap
	used    uint64
	loading bool
}

// Only visible artwork is requested; one worker and 48 resized covers bound
// memory and file activity independently of library size.
func (a *app) coverFor(t *track) ui.ImageSource {
	if t == nil {
		return coverFor(nil)
	}
	key := pathKey(t.Path)
	a.artClock++
	if e, ok := a.art[key]; ok {
		e.used = a.artClock
		if e.bitmap != nil {
			return e.bitmap
		}
		return coverFor(t)
	}
	if a.window != nil {
		if len(a.art) >= 48 {
			oldest := ""
			var age uint64 = ^uint64(0)
			for k, e := range a.art {
				if !e.loading && e.used < age {
					oldest, age = k, e.used
				}
			}
			if oldest == "" {
				return coverFor(t)
			}
			delete(a.art, oldest)
		}
		select {
		case a.artJobs <- *t:
			a.art[key] = &artEntry{used: a.artClock, loading: true}
		default:
		}
	}
	return coverFor(t)
}

func (a *app) startArtwork() {
	go func() {
		for {
			select {
			case <-a.ctx.Done():
				return
			case t := <-a.artJobs:
				bitmap := loadArtwork(t.Path)
				if a.closed.Load() {
					return
				}
				a.window.Update(func() {
					if e := a.art[pathKey(t.Path)]; e != nil && !a.closed.Load() {
						e.bitmap, e.loading = bitmap, false
					}
				})
			}
		}
	}()
}

func loadArtwork(path string) *ui.Bitmap {
	if b := embeddedArtwork(path); len(b) > 0 {
		if bitmap := artworkBitmap(b); bitmap != nil {
			return bitmap
		}
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, name := range []string{base, "cover", "folder", "front", "album", "artwork"} {
		for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
			for _, e := range entries {
				if !e.IsDir() && strings.EqualFold(e.Name(), name+ext) {
					if b, err := readSmallFile(filepath.Join(filepath.Dir(path), e.Name()), maxArtworkBytes); err == nil {
						if bitmap := artworkBitmap(b); bitmap != nil {
							return bitmap
						}
					}
				}
			}
		}
	}
	return nil
}

func readSmallFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if int64(len(b)) > limit {
		return nil, io.ErrShortBuffer
	}
	return b, err
}

func artworkBitmap(b []byte) *ui.Bitmap {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 32_000_000 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	w, h := cfg.Width, cfg.Height
	if max(w, h) > 512 {
		scale := 512 / float64(max(w, h))
		w, h = max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	return ui.NewBitmap(dst)
}

func embeddedArtwork(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var head [10]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return nil
	}
	if string(head[:3]) == "ID3" && (head[3] == 3 || head[3] == 4) && head[5]&0xc0 == 0 {
		remaining := synchsafe(head[6:])
		if remaining > maxArtworkBytes {
			return nil
		}
		for remaining >= 10 {
			var h [10]byte
			if _, err := io.ReadFull(f, h[:]); err != nil {
				break
			}
			remaining -= 10
			n := int(binary.BigEndian.Uint32(h[4:8]))
			if head[3] == 4 {
				n = synchsafe(h[4:8])
			}
			if n <= 0 || n > remaining {
				break
			}
			remaining -= n
			if string(h[:4]) == "APIC" && h[9] == 0 {
				b := make([]byte, n)
				if _, err := io.ReadFull(f, b); err != nil {
					break
				}
				if len(b) < 4 {
					continue
				}
				enc := b[0]
				b = b[1:]
				i := bytes.IndexByte(b, 0)
				if i < 0 || i+2 >= len(b) {
					continue
				}
				b = b[i+2:]
				if enc == 1 || enc == 2 {
					for len(b) >= 2 {
						end := b[0] == 0 && b[1] == 0
						b = b[2:]
						if end {
							return b
						}
					}
				} else if i := bytes.IndexByte(b, 0); i >= 0 {
					return b[i+1:]
				}
			} else {
				if _, err := f.Seek(int64(n), io.SeekCurrent); err != nil {
					break
				}
			}
		}
	} else if string(head[:4]) == "fLaC" {
		f.Seek(4, io.SeekStart)
		for block := 0; block < 128; block++ {
			var h [4]byte
			if _, err := io.ReadFull(f, h[:]); err != nil {
				break
			}
			n := int(h[1])<<16 | int(h[2])<<8 | int(h[3])
			if h[0]&127 == 6 && n <= maxArtworkBytes {
				b := make([]byte, n)
				if _, err := io.ReadFull(f, b); err == nil {
					return flacPicture(b)
				}
				break
			}
			if h[0]&128 != 0 {
				break
			}
			f.Seek(int64(n), io.SeekCurrent)
		}
	} else if string(head[:4]) == "OggS" {
		f.Seek(0, io.SeekStart)
		if packet := vorbisCommentPacket(f); len(packet) > 0 {
			b := packet[7:]
			if len(b) < 4 {
				return nil
			}
			n := int(binary.LittleEndian.Uint32(b))
			if n > len(b)-8 {
				return nil
			}
			b = b[4+n:]
			count := int(binary.LittleEndian.Uint32(b))
			b = b[4:]
			for j := 0; j < count && len(b) >= 4; j++ {
				n = int(binary.LittleEndian.Uint32(b))
				b = b[4:]
				if n > len(b) {
					break
				}
				comment := string(b[:n])
				b = b[n:]
				if value, ok := strings.CutPrefix(comment, "METADATA_BLOCK_PICTURE="); ok {
					decoded, err := base64.StdEncoding.DecodeString(value)
					if err == nil {
						return flacPicture(decoded)
					}
				}
			}
		}
	}
	return nil
}

// Stop at the comment packet instead of reading the song's audio pages.
func vorbisCommentPacket(r io.Reader) []byte {
	r = io.LimitReader(r, maxArtworkBytes)
	var packet []byte
	packets := 0
	for page := 0; page < 1024; page++ {
		var head [27]byte
		if _, err := io.ReadFull(r, head[:]); err != nil || string(head[:4]) != "OggS" {
			return nil
		}
		laces := make([]byte, int(head[26]))
		if _, err := io.ReadFull(r, laces); err != nil {
			return nil
		}
		for _, size := range laces {
			start := len(packet)
			packet = append(packet, make([]byte, int(size))...)
			if _, err := io.ReadFull(r, packet[start:]); err != nil {
				return nil
			}
			if size < 255 {
				if bytes.HasPrefix(packet, []byte("\x03vorbis")) {
					return packet
				}
				packet = packet[:0]
				packets++
				if packets >= 3 {
					return nil
				}
			}
		}
	}
	return nil
}

func flacPicture(b []byte) []byte {
	if len(b) < 8 {
		return nil
	}
	b = b[4:]
	for range 2 {
		if len(b) < 4 {
			return nil
		}
		n := int(binary.BigEndian.Uint32(b))
		b = b[4:]
		if n > len(b) {
			return nil
		}
		b = b[n:]
	}
	if len(b) < 20 {
		return nil
	}
	n := int(binary.BigEndian.Uint32(b[16:20]))
	b = b[20:]
	if n > len(b) {
		return nil
	}
	return b[:n]
}

func (a *app) upgradeMetadata() {
	var pending []track
	for _, t := range a.prefs.Tracks {
		if t.MediaVersion == 0 {
			pending = append(pending, t)
		}
	}
	go func() {
		for _, old := range pending {
			if a.ctx.Err() != nil {
				return
			}
			t := readTrack(old.Path)
			a.window.Update(func() {
				if a.closed.Load() {
					return
				}
				if i, ok := a.byPath[pathKey(t.Path)]; ok {
					a.prefs.Tracks[i].Duration = t.Duration
					a.prefs.Tracks[i].MediaVersion = 1
				}
			})
		}
		if len(pending) > 0 && !a.closed.Load() {
			a.window.Update(func() {
				if !a.closed.Load() {
					a.persist()
				}
			})
		}
	}()
}

func (a *app) refreshFolders() {
	if a.importing {
		return
	}
	paths := append([]string(nil), a.prefs.Folders...)
	if len(paths) == 0 {
		a.notice = "Add a music folder to enable refresh"
		return
	}
	// Retry artwork after a refresh, including covers added beside old songs.
	for k, e := range a.art {
		if !e.loading {
			delete(a.art, k)
		}
	}
	a.importPaths(paths)
}
