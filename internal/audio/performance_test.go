package audio

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	nativemp3 "github.com/egoist/mygo/audio/mp3"
)

type countedSeeker struct {
	*bytes.Reader
	reads int
}

func (r *countedSeeker) Read(p []byte) (int, error) {
	r.reads++
	return r.Reader.Read(p)
}

func TestBufferedSeekerOffsets(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789"), 10000)
	r := newBufferedSeeker(bytes.NewReader(data))
	want := bytes.NewReader(data)
	for _, seek := range []struct {
		offset int64
		whence int
	}{{0, io.SeekCurrent}, {8, io.SeekCurrent}, {-12, io.SeekCurrent}, {40000, io.SeekStart}, {-8, io.SeekEnd}, {100, io.SeekStart}, {100001, io.SeekStart}, {0, io.SeekStart}} {
		gotPos, err := r.Seek(seek.offset, seek.whence)
		wantPos, wantErr := want.Seek(seek.offset, seek.whence)
		if gotPos != wantPos || (err == nil) != (wantErr == nil) {
			t.Fatal(seek, gotPos, wantPos, err, wantErr)
		}
		got, expected := make([]byte, 7), make([]byte, 7)
		n, err := io.ReadFull(r, got)
		nw, ew := io.ReadFull(want, expected)
		if n != nw || err != ew || !bytes.Equal(got, expected) {
			t.Fatal(seek, got, expected, err, ew)
		}
	}
}

func TestMP3ScanUsesBatchedReads(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tone.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Repeat(data, 128)
	raw := &countedSeeker{Reader: bytes.NewReader(data)}
	direct, err := nativemp3.NewDecoder(raw)
	if err != nil {
		t.Fatal(err)
	}
	batched := &countedSeeker{Reader: bytes.NewReader(data)}
	buffered, err := nativemp3.NewDecoder(newBufferedSeeker(batched))
	if err != nil {
		t.Fatal(err)
	}
	if buffered.Len() != direct.Len() || batched.reads >= raw.reads/10 {
		t.Fatalf("length %d/%d; raw reads %d, buffered reads %d", buffered.Len(), direct.Len(), raw.reads, batched.reads)
	}
	t.Logf("MP3 opening: %d -> %d source reads", raw.reads, batched.reads)
}

func TestPCMEndSeekAndBatchedRefill(t *testing.T) {
	for _, path := range []string{writeWAV(t, 48000, 1), writeWAV(t, 44100, 1), filepath.Join("testdata", "tone.mp3"), filepath.Join("testdata", "tone.flac"), filepath.Join("testdata", "tone.ogg")} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			d, format, err := Decode(path)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			r := newPCM(d, format)
			buf := make([]byte, playerBufferBytes)
			if n, err := r.Read(buf); n != 2048*bytesPerFrame || err != nil {
				t.Fatal("refill batch", n, err)
			}
			if _, err := r.Seek(0, io.SeekEnd); err != nil {
				t.Fatal(err)
			}
			if n, err := r.Read(buf); n != 0 || err != io.EOF {
				t.Fatal("seek to EOF", n, err)
			}
			if _, err := r.Seek(0, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			if n, err := r.Read(buf); n == 0 || err != nil {
				t.Fatal("seek back after EOF", n, err)
			}
		})
	}
}

// A concatenated fixture models the frame-header scan of a longer MP3 without
// adding a large binary fixture to the repository.
func BenchmarkMP3Open(b *testing.B) {
	data, err := os.ReadFile(filepath.Join("testdata", "tone.mp3"))
	if err != nil {
		b.Fatal(err)
	}
	path := filepath.Join(b.TempDir(), "long.mp3")
	if err = os.WriteFile(path, bytes.Repeat(data, 128), 0600); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		d, _, err := Decode(path)
		if err != nil {
			b.Fatal(err)
		}
		d.Close()
	}
}
