// Package audio streams files through bounded PCM buffers, with at most two
// playback streams during a crossfade.
package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	nativeaudio "github.com/egoist/mygo/audio"
	nativemp3 "github.com/egoist/mygo/audio/mp3"
	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/vorbis"
)

const OutputRate = 48000
const bytesPerFrame = 8 // stereo float32, preserving >16-bit source precision

func Decode(path string) (beep.StreamSeekCloser, beep.Format, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, beep.Format{}, err
	}
	var s beep.StreamSeekCloser
	var format beep.Format
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		var d nativeaudio.Decoder
		d, err = nativemp3.NewDecoder(f)
		if err == nil {
			s, format = adaptNative(f, d, 2)
		}
	case ".flac":
		s, format, err = decodeFLAC(f)
	case ".ogg", ".oga":
		s, format, err = vorbis.Decode(f)
	case ".wav":
		var d nativeaudio.Decoder
		var precision int
		precision, err = wavPrecision(f)
		if err == nil {
			_, err = f.Seek(0, io.SeekStart)
		}
		if err == nil {
			d, _, err = nativeaudio.NewDecoder(f)
		}
		if err == nil {
			s, format = adaptNative(f, d, precision)
		}
	default:
		err = fmt.Errorf("unsupported format: %s", filepath.Ext(path))
	}
	if err != nil {
		f.Close()
		return nil, beep.Format{}, err
	}
	if format.NumChannels < 1 || format.NumChannels > 2 || format.SampleRate <= 0 {
		s.Close()
		return nil, beep.Format{}, fmt.Errorf("only mono and stereo audio are supported")
	}
	return s, format, nil
}

// MyGo's native WAV/MP3 decoders batch reads and retain their scratch buffers.
// Adapt just the decoder; Beep's real-time resampler and Oto own the output.
type nativeAdapter struct {
	file *os.File
	d    nativeaudio.Decoder
	buf  [1024]float32
	pos  int
	err  error
}

func adaptNative(f *os.File, d nativeaudio.Decoder, precision int) (beep.StreamSeekCloser, beep.Format) {
	return &nativeAdapter{file: f, d: d}, beep.Format{SampleRate: beep.SampleRate(d.SampleRate()), NumChannels: d.Channels(), Precision: precision}
}
func (d *nativeAdapter) Stream(samples [][2]float64) (int, bool) {
	if d.err != nil {
		return 0, false
	}
	ch := d.d.Channels()
	if ch < 1 || ch > 2 {
		return 0, false
	}
	n, err := d.d.Read(d.buf[:min(len(samples), len(d.buf)/ch)*ch])
	frames := n / ch
	for i := 0; i < frames; i++ {
		samples[i][0] = float64(d.buf[i*ch])
		samples[i][1] = samples[i][0]
		if ch == 2 {
			samples[i][1] = float64(d.buf[i*ch+1])
		}
	}
	d.pos += frames
	if err != io.EOF {
		d.err = err
	}
	return frames, frames > 0
}
func (d *nativeAdapter) Err() error    { return d.err }
func (d *nativeAdapter) Len() int      { return int(d.d.Len()) }
func (d *nativeAdapter) Position() int { return d.pos }
func (d *nativeAdapter) Seek(p int) error {
	if err := d.d.SetPosition(int64(p)); err != nil {
		return err
	}
	d.pos = p
	d.err = nil
	return nil
}
func (d *nativeAdapter) Close() error { return d.file.Close() }

func wavPrecision(f *os.File) (int, error) {
	var head [12]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return 0, err
	}
	if string(head[:4]) != "RIFF" || string(head[8:]) != "WAVE" {
		return 0, fmt.Errorf("invalid WAV header")
	}
	for chunks := 0; chunks < 1024; chunks++ {
		var h [8]byte
		if _, err := io.ReadFull(f, h[:]); err != nil {
			return 0, err
		}
		size := int64(binary.LittleEndian.Uint32(h[4:]))
		if string(h[:4]) == "fmt " {
			var b [16]byte
			if size < 16 {
				return 0, fmt.Errorf("invalid WAV format")
			}
			if _, err := io.ReadFull(f, b[:]); err != nil {
				return 0, err
			}
			return int(binary.LittleEndian.Uint16(b[14:])) / 8, nil
		}
		if _, err := f.Seek(size+size%2, io.SeekCurrent); err != nil {
			return 0, err
		}
	}
	return 0, fmt.Errorf("WAV has too many metadata chunks")
}

// pcmReader's decoder and resampler are only touched by Oto's reader, or by
// Seek while Oto has stopped reading. Position is shared through an atomic.
type pcmReader struct {
	decoder beep.StreamSeekCloser
	format  beep.Format
	stream  beep.Streamer
	samples [512][2]float64
	frames  atomic.Int64
}

func newPCM(s beep.StreamSeekCloser, f beep.Format) *pcmReader {
	r := &pcmReader{decoder: s, format: f}
	r.reset()
	return r
}

func (r *pcmReader) reset() {
	r.stream = r.decoder
	if r.format.SampleRate != OutputRate {
		// Quality 4 is Beep's recommended real-time, good-quality setting.
		r.stream = beep.Resample(4, r.format.SampleRate, OutputRate, r.decoder)
	}
}

func (r *pcmReader) Read(p []byte) (int, error) {
	count := min(len(p)/bytesPerFrame, len(r.samples))
	if count == 0 {
		return 0, io.ErrShortBuffer
	}
	n, _ := r.stream.Stream(r.samples[:count])
	for i, sample := range r.samples[:n] {
		for ch, value := range sample {
			// Bound resampler overshoot; never boost gain or add an effect.
			value = max(-1, min(1, value))
			binary.LittleEndian.PutUint32(p[i*8+ch*4:], math.Float32bits(float32(value)))
		}
	}
	r.frames.Add(int64(n))
	if err := r.stream.Err(); err != nil {
		return n * 8, err
	}
	if n == 0 {
		return 0, io.EOF
	}
	return n * 8, nil
}

func (r *pcmReader) Seek(offset int64, whence int) (int64, error) {
	if r.decoder.Len() < 0 {
		return 0, fmt.Errorf("duration is unknown; seeking is unavailable")
	}
	end := int64(r.decoder.Len()) * OutputRate / int64(r.format.SampleRate)
	frame := offset / bytesPerFrame
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		frame += r.frames.Load()
	case io.SeekEnd:
		frame += end
	default:
		return 0, fmt.Errorf("invalid seek mode")
	}
	frame = max(0, min(end, frame))
	sourceFrame := frame * int64(r.format.SampleRate) / OutputRate
	if err := r.decoder.Seek(int(sourceFrame)); err != nil {
		return 0, err
	}
	r.reset() // Drop resampler history so seeking cannot replay old samples.
	r.frames.Store(frame)
	return frame * bytesPerFrame, nil
}

func (r *pcmReader) duration() time.Duration { return r.format.SampleRate.D(max(0, r.decoder.Len())) }
