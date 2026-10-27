package audio

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mewkiz/flac"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

func flacSample(p, ch int) int32 {
	return int32((p*7919+ch*3571)%16000001 - 8000000)
}

// Generate frames without a SEEKTABLE, including a short final fixed frame.
func writeFLAC(t testing.TB, variable bool, blocks int) (string, int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seek.flac")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	enc, err := flac.NewEncoder(f, &meta.StreamInfo{BlockSizeMin: 4096, BlockSizeMax: 4096, SampleRate: 44100, NChannels: 2, BitsPerSample: 24})
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	pos := 0
	for i := range blocks {
		n := 4096
		if variable {
			n -= (i % 4) * 512
		}
		if i == blocks-1 {
			n = 123
		}
		fr := &frame.Frame{Header: frame.Header{HasFixedBlockSize: !variable, BlockSize: uint16(n), SampleRate: 44100, Channels: frame.ChannelsLR, BitsPerSample: 24}}
		for ch := range 2 {
			samples := make([]int32, n)
			for j := range samples {
				samples[j] = flacSample(pos+j, ch)
			}
			fr.Subframes = append(fr.Subframes, &frame.Subframe{SubHeader: frame.SubHeader{Pred: frame.PredVerbatim}, NSamples: n, Samples: samples})
		}
		if err := enc.WriteFrame(fr); err != nil {
			f.Close()
			t.Fatal(err)
		}
		pos += n
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	return path, pos
}

type countedFLAC struct {
	*os.File
	bytes int
}

func (f *countedFLAC) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	f.bytes += n
	return n, err
}

func TestFLACFirstSeekIsBoundedAndAccurate(t *testing.T) {
	for _, variable := range []bool{false, true} {
		t.Run(map[bool]string{false: "fixed", true: "variable"}[variable], func(t *testing.T) {
			path, total := writeFLAC(t, variable, 256)
			file, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			f := &countedFLAC{File: file}
			s, format, err := decodeFLAC(f)
			if err != nil {
				file.Close()
				t.Fatal(err)
			}
			defer s.Close()
			if s.Len() != total || format.Precision != 3 {
				t.Fatal(s.Len(), total, format)
			}
			// Check the first seek before sequential playback has built any state.
			f.bytes = 0
			start := time.Now()
			if err := s.Seek(total * 9 / 10); err != nil {
				t.Fatal(err)
			}
			info, _ := file.Stat()
			t.Logf("first seek: %s, read %d of %d compressed bytes", time.Since(start), f.bytes, info.Size())
			if int64(f.bytes) > info.Size()/2 {
				t.Fatal("first seek scanned the song", f.bytes, info.Size())
			}
			positions := []int{total * 9 / 10, 0, 4095, 4096, total - 124, total - 123, total - 1}
			for i := range 40 {
				positions = append(positions, (i*7919)%total)
			}
			buf := make([][2]float64, 32)
			for _, p := range positions {
				if err := s.Seek(p); err != nil {
					t.Fatalf("seek %d: %v", p, err)
				}
				n, _ := s.Stream(buf)
				if n != min(len(buf), total-p) {
					t.Fatalf("seek %d: read %d", p, n)
				}
				for j := range n {
					for ch := range 2 {
						want := float64(flacSample(p+j, ch)) / (1 << 23)
						if buf[j][ch] != want {
							t.Fatalf("sample %d channel %d: %v want %v", p+j, ch, buf[j][ch], want)
						}
					}
				}
				if s.Position() != p+n || s.Err() != nil {
					t.Fatal(s.Position(), s.Err())
				}
			}
			if err := s.Seek(total); err != nil {
				t.Fatal(err)
			}
			if n, _ := s.Stream(buf); n != 0 || s.Err() != nil {
				t.Fatal("seek to end", n, s.Err())
			}
			if err := s.Seek(0); err != nil {
				t.Fatal("seek after EOF", err)
			}
			if n, _ := s.Stream(buf); n == 0 {
				t.Fatal("EOF was not reset")
			}
			if err := s.Seek(-1); err == nil {
				t.Fatal("negative seek accepted")
			}
			if err := s.Seek(total + 1); err == nil {
				t.Fatal("past-end seek accepted")
			}
		})
	}
}

func TestFLACSeekMatchesSequentialDecode(t *testing.T) {
	s, _, err := Decode(filepath.Join("testdata", "tone.flac"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	all := make([][2]float64, s.Len())
	if n, _ := s.Stream(all); n != len(all) {
		t.Fatal(n, len(all))
	}
	for _, p := range []int{0, 1, 4095, 4096, 16384, s.Len() - 100, s.Len() - 1} {
		if err := s.Seek(p); err != nil {
			t.Fatal(p, err)
		}
		got := make([][2]float64, min(100, s.Len()-p))
		if n, _ := s.Stream(got); n != len(got) {
			t.Fatal(n, len(got))
		}
		for i := range got {
			if got[i] != all[p+i] {
				t.Fatal("seek differs from sequential decode", p+i)
			}
		}
	}
}
