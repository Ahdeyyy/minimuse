package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSongBackdrop(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	for y := range 128 {
		for x := range 128 {
			col := color.RGBA{R: 240, G: 40, B: 60, A: 255}
			if x >= 64 {
				col = color.RGBA{R: 30, G: 80, B: 240, A: 255}
			}
			img.SetRGBA(x, y, col)
		}
	}
	backdrop := artworkBackdrop(img)
	// A sharp boundary in the original cover must become a gradual blend.
	blur := ui.NewTester(func(c *ui.Context) {
		ui.Image(c, backdrop).Size(64, 64)
	}, 64, 64).Image().RGBAAt(31, 32)
	if blur.R < 80 || blur.B < 80 {
		t.Fatalf("cover boundary was not blurred: %v", blur)
	}

	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			a := testApp(t, 2)
			a.prefs.Dark = dark
			a.setTheme()
			a.prefs.Current = a.prefs.Tracks[0].Path
			a.songScreen = true
			a.lyricsPath = a.prefs.Current
			a.lyrics = parseLyrics("A little room for the music\nLet the needle find its way\nAnd the evening drifts away")
			a.lyricMode = "Static"
			tt := ui.NewTester(a.view, 1080, 740)
			plain := tt.Image().RGBAAt(850, 112)
			a.art[pathKey(a.prefs.Current)] = &artEntry{bitmap: ui.NewBitmap(img), backdrop: backdrop}
			tt.Frame()
			shown := tt.Image().RGBAAt(850, 112)
			if shown == plain {
				t.Fatal("cover artwork did not appear behind Now Playing")
			}
			// The theme veil keeps the background close to its original tone,
			// even with a bright, highly saturated cover.
			for _, pair := range [][2]uint8{{plain.R, shown.R}, {plain.G, shown.G}, {plain.B, shown.B}} {
				if delta := int(pair[0]) - int(pair[1]); delta < -52 || delta > 52 {
					t.Fatalf("backdrop overwhelms the theme: plain %v, backdrop %v", plain, shown)
				}
			}
			if dir := os.Getenv("MINIMUSE_CAPTURE_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				for _, size := range []struct {
					name string
					w, h int
				}{{name, 1080, 740}, {name + "-compact", 820, 580}} {
					capture := newApp(a.prefs, "")
					defer capture.cancel()
					capture.songScreen = true
					capture.lyrics, capture.lyricsPath = a.lyrics, a.lyricsPath
					capture.lyricMode = a.lyricMode
					capture.art = a.art
					f, err := os.Create(filepath.Join(dir, "song-backdrop-"+size.name+".png"))
					if err != nil {
						t.Fatal(err)
					}
					err = png.Encode(f, ui.NewTester(capture.view, size.w, size.h).Image())
					f.Close()
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			a.prefs.Current = a.prefs.Tracks[1].Path
			tt.Frame()
			if got := tt.Image().RGBAAt(850, 112); got != plain {
				t.Fatalf("song without artwork retained the previous backdrop: %v", got)
			}
			a.prefs.Current = a.prefs.Tracks[0].Path
			tt.Frame()
			click(t, tt, "Back to library")
			if a.songScreen {
				t.Fatal("backdrop blocked navigation")
			}
			tt.Frame()
			a.openSong()
			tt.Frame()
			if got := tt.Image().RGBAAt(850, 112); got != shown {
				t.Fatalf("returning to Now Playing lost the backdrop: %v", got)
			}
		})
	}
}
