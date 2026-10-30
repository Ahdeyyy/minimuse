package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/tphakala/go-m4a/aacm4a"
)

// The codec retains a few AAC frames; compressed sample tables provide random
// access. Keep the edit-list presentation window, excluding priming and padding.
type m4aDecoder struct {
	file        *os.File
	d           *aacm4a.Decoder
	format      beep.Format
	length, pos int
	err         error
	buf         [4096]byte
}

func decodeM4A(f *os.File) (beep.StreamSeekCloser, beep.Format, error) {
	d, info, err := aacm4a.NewSeekableDecoder(f)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("M4A (AAC-LC): %w", err)
	}
	format := beep.Format{SampleRate: beep.SampleRate(info.SampleRate), NumChannels: info.Channels, Precision: 2}
	if info.SampleRate <= 0 || info.Channels < 1 || info.Channels > 2 || info.Duration <= 0 {
		return nil, beep.Format{}, fmt.Errorf("M4A requires mono or stereo audio with a valid duration")
	}
	// Duration is rounded down to nanoseconds by the container reader.
	length := int(info.Duration/time.Second)*info.SampleRate + int((int64(info.Duration%time.Second)*int64(info.SampleRate)+int64(time.Second)-1)/int64(time.Second))
	s := &m4aDecoder{file: f, d: d, format: format, length: length}
	if err := s.Seek(0); err != nil {
		return nil, beep.Format{}, err
	}
	return s, format, nil
}

func (s *m4aDecoder) Stream(samples [][2]float64) (int, bool) {
	if s.err != nil || s.pos == s.length {
		return 0, false
	}
	stride := s.format.NumChannels * 2
	frames := min(len(samples), len(s.buf)/stride, s.length-s.pos)
	n, err := io.ReadFull(s.d, s.buf[:frames*stride])
	frames = n / stride
	for i := 0; i < frames; i++ {
		j := i * stride
		samples[i][0] = float64(int16(binary.LittleEndian.Uint16(s.buf[j:]))) / 32768
		samples[i][1] = samples[i][0]
		if stride == 4 {
			samples[i][1] = float64(int16(binary.LittleEndian.Uint16(s.buf[j+2:]))) / 32768
		}
	}
	s.pos += frames
	if err != nil {
		s.err = fmt.Errorf("M4A ended before its presentation window: %w", err)
	}
	return frames, frames > 0
}

func (s *m4aDecoder) Len() int      { return s.length }
func (s *m4aDecoder) Position() int { return s.pos }
func (s *m4aDecoder) Err() error    { return s.err }
func (s *m4aDecoder) Close() error  { return s.file.Close() }
func (s *m4aDecoder) Seek(p int) error {
	if p < 0 || p > s.length {
		return fmt.Errorf("M4A seek out of range: %d", p)
	}
	if p != s.length {
		// Round up so conversion back to media ticks selects exactly p.
		target := time.Duration((int64(p)*int64(time.Second) + int64(s.format.SampleRate) - 1) / int64(s.format.SampleRate))
		if err := s.d.SeekTime(target); err != nil {
			return err
		}
	}
	s.pos, s.err = p, nil
	return nil
}
