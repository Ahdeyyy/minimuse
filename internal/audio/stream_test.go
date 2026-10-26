package audio

import (
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/gopxl/beep/v2"
)

func writeWAV(t testing.TB, rate, seconds int) string {
	t.Helper()
	n := rate * seconds
	data := make([]byte, 44+n*6)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 2)
	binary.LittleEndian.PutUint32(data[24:], uint32(rate))
	binary.LittleEndian.PutUint32(data[28:], uint32(rate*6))
	binary.LittleEndian.PutUint16(data[32:], 6)
	binary.LittleEndian.PutUint16(data[34:], 24)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(n*6))
	for i := 0; i < n; i++ {
		v := int32(math.Sin(float64(i)*440*2*math.Pi/float64(rate)) * 2000000)
		for ch := 0; ch < 2; ch++ {
			j := 44 + i*6 + ch*3
			data[j] = byte(v)
			data[j+1] = byte(v >> 8)
			data[j+2] = byte(v >> 16)
		}
	}
	path := filepath.Join(t.TempDir(), "tone.wav")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFloatOutputDurationAndSeek(t *testing.T) {
	for _, rate := range []int{44100, 48000, 96000} {
		t.Run(strconv.Itoa(rate), func(t *testing.T) {
			path := writeWAV(t, rate, 1)
			s, f, err := Decode(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			r := newPCM(s, f)
			if r.duration() != time.Second || f.Precision != 3 {
				t.Fatal(r.duration(), f)
			}
			out, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) != OutputRate*8 {
				t.Fatalf("rate %d: output length %d", rate, len(out))
			}
			// More than 16-bit precision survives the float path.
			if rate == 48000 {
				sample := float64(math.Float32frombits(binary.LittleEndian.Uint32(out[8:])))
				if sample == math.Round(sample*32768)/32768 {
					t.Fatal("24-bit source reduced to 16-bit")
				}
			}
			if _, err = r.Seek(OutputRate/2*8, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			out, err = io.ReadAll(r)
			if err != nil || len(out) != OutputRate/2*8 {
				t.Fatal(len(out), err)
			}
			if _, err = r.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			if _, err = r.Read(make([]byte, 1)); err != io.ErrShortBuffer {
				t.Fatal(err)
			}
		})
	}
}

func TestCodecFixtures(t *testing.T) {
	for _, ext := range []string{"mp3", "flac", "ogg"} {
		t.Run(ext, func(t *testing.T) {
			s, f, err := Decode(filepath.Join("testdata", "tone."+ext))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			r := newPCM(s, f)
			out, err := io.ReadAll(r)
			if err != nil || len(out) < OutputRate*8/3 {
				t.Fatal(len(out), err)
			}
			if _, err = r.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 4096)
			if n, err := r.Read(buf); err != nil || n == 0 {
				t.Fatal(n, err)
			}
		})
	}
}

func BenchmarkWAVDecode(b *testing.B) {
	path := writeWAV(b, 48000, 1)
	s, f, err := Decode(path)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	r := newPCM(s, f)
	buf := make([]byte, 4096)
	r.Read(buf)
	b.ReportAllocs()
	b.SetBytes(int64(len(buf)))
	for b.Loop() {
		if _, err = r.Read(buf); err == io.EOF {
			r.Seek(0, io.SeekStart)
		} else if err != nil {
			b.Fatal(err)
		}
	}
}

func TestInvalidDecoderReleasesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.mp3")
	os.WriteFile(path, []byte("broken"), 0600)
	if _, _, err := Decode(path); err == nil {
		t.Fatal("invalid audio accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal("decoder leaked file handle", err)
	}
}

type infiniteSilence struct{}

func (infiniteSilence) Stream(s [][2]float64) (int, bool) { clear(s); return len(s), true }
func (infiniteSilence) Err() error                        { return nil }
func (infiniteSilence) Len() int                          { return 48000 * 3600 }
func (infiniteSilence) Position() int                     { return 0 }
func (infiniteSilence) Seek(int) error                    { return nil }
func (infiniteSilence) Close() error                      { return nil }

func BenchmarkPCMRead(b *testing.B) {
	for _, rate := range []int{48000, 44100} {
		b.Run(map[int]string{48000: "native-rate", 44100: "resample"}[rate], func(b *testing.B) {
			r := newPCM(infiniteSilence{}, beep.Format{SampleRate: beep.SampleRate(rate), NumChannels: 2, Precision: 3})
			buf := make([]byte, 4096)
			r.Read(buf)
			b.ReportAllocs()
			b.SetBytes(int64(len(buf)))
			b.ResetTimer()
			for b.Loop() {
				r.Read(buf)
			}
		})
	}
}

func TestRealDeviceLifecycle(t *testing.T) {
	if os.Getenv("MINIMUSE_AUDIO_INTEGRATION") != "1" {
		t.Skip("set MINIMUSE_AUDIO_INTEGRATION=1 to exercise the sound device (muted)")
	}
	e := New(nil)
	defer e.Close()
	e.Volume(0)
	path := writeWAV(t, 44100, 3)
	if !e.Load(path) {
		t.Fatal("load rejected")
	}
	wait := func(predicate func(Status) bool) Status {
		t.Helper()
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			s := e.Snapshot()
			if s.Error != "" {
				t.Fatal(s.Error)
			}
			if predicate(s) {
				return s
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("timed out", e.Snapshot())
		return Status{}
	}
	wait(func(s Status) bool { return s.Playing && s.Position > 200*time.Millisecond })
	e.Toggle()
	paused := wait(func(s Status) bool { return !s.Playing })
	time.Sleep(300 * time.Millisecond)
	if e.Snapshot().Position != paused.Position {
		t.Fatal("paused playback advanced")
	}
	e.Seek(time.Second)
	wait(func(s Status) bool { return s.Position >= time.Second })
	e.Toggle()
	wait(func(s Status) bool { return s.Playing })
	wait(func(s Status) bool { return s.Ended })
	if err := os.Remove(path); err != nil {
		t.Fatal("finished decoder still holds file", err)
	}
	if seconds, _ := strconv.Atoi(os.Getenv("MINIMUSE_AUDIO_SOAK_SECONDS")); seconds > 0 {
		path = writeWAV(t, 48000, seconds+5)
		e.Load(path)
		wait(func(s Status) bool { return s.Path == path && s.Playing })
		runtime.GC()
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		time.Sleep(time.Duration(seconds) * time.Second)
		runtime.GC()
		runtime.ReadMemStats(&after)
		if s := e.Snapshot(); !s.Playing || s.Error != "" {
			t.Fatal(s)
		}
		t.Logf("%ds soak: live Go heap %.2f -> %.2f MiB; total allocated %.2f MiB", seconds, float64(before.HeapAlloc)/(1<<20), float64(after.HeapAlloc)/(1<<20), float64(after.TotalAlloc-before.TotalAlloc)/(1<<20))
		e.Stop()
		wait(func(s Status) bool { return !s.Playing })
	}
}
