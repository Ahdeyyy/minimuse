package audio

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestM4APresentationAndSeek(t *testing.T) {
	s, f, err := Decode(filepath.Join("testdata", "tone.m4a"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.Len() != 44100 || f.SampleRate.D(s.Len()) != time.Second {
		t.Fatal(s.Len(), f)
	}
	var linear [][2]float64
	buf := make([][2]float64, 517)
	for {
		n, ok := s.Stream(buf)
		linear = append(linear, buf[:n]...)
		if !ok {
			break
		}
	}
	if s.Err() != nil || len(linear) != s.Len() {
		t.Fatal(len(linear), s.Err())
	}
	var energy float64
	for _, v := range linear {
		energy += v[0] * v[0]
	}
	if energy < 1 {
		t.Fatal("silent output")
	}
	for _, p := range []int{22050, 0, 1, 44099, 44100, 1023, 1024} {
		if err := s.Seek(p); err != nil {
			t.Fatal(p, err)
		}
		if s.Position() != p {
			t.Fatal(s.Position(), p)
		}
		n, ok := s.Stream(buf)
		if p == s.Len() {
			if n != 0 || ok || s.Err() != nil {
				t.Fatal(n, ok, s.Err())
			}
			continue
		}
		if n == 0 || !ok || s.Err() != nil {
			t.Fatal(p, n, ok, s.Err())
		}
		for i := 0; i < n; i++ {
			if math.Abs(buf[i][0]-linear[p+i][0]) > 1.0/32768 {
				t.Fatalf("seek %d differs at %d", p, i)
			}
		}
	}
	for _, p := range []int{-1, s.Len() + 1} {
		if s.Seek(p) == nil {
			t.Fatal("invalid seek accepted", p)
		}
	}
}

func TestInvalidM4AReleasesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.m4a")
	if err := os.WriteFile(path, []byte("not an MP4"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Decode(path); err == nil {
		t.Fatal("invalid M4A accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestM4ACloseReleasesFile(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "tone.m4a"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "song.M4A")
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	s, f, err := Decode(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, newPCM(s, f)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
