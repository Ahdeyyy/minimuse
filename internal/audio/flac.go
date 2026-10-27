package audio

import (
	"bufio"
	"fmt"
	"io"

	"github.com/gopxl/beep/v2"
	"github.com/mewkiz/flac/frame"
	"github.com/mewkiz/flac/meta"
)

// flacDecoder seeks by bisecting compressed byte offsets and checking frame
// headers and CRCs. The upstream seek implementation decodes the entire song
// on the first seek when no embedded seek table exists. This needs neither a
// full-file scan nor a full-song PCM cache, even on the first seek.
type flacDecoder struct {
	file         io.ReadSeekCloser
	r            *bufio.Reader
	info         *meta.StreamInfo
	start, end   int64
	frame        *frame.Frame
	inFrame, pos int
	err          error
}

func decodeFLAC(file io.ReadSeekCloser) (beep.StreamSeekCloser, beep.Format, error) {
	var signature [4]byte
	if _, err := io.ReadFull(file, signature[:]); err != nil {
		return nil, beep.Format{}, err
	}
	if string(signature[:3]) == "ID3" {
		var tag [6]byte
		if _, err := io.ReadFull(file, tag[:]); err != nil {
			return nil, beep.Format{}, err
		}
		size := int64(tag[2]&127)<<21 | int64(tag[3]&127)<<14 | int64(tag[4]&127)<<7 | int64(tag[5]&127)
		if _, err := file.Seek(size, io.SeekCurrent); err != nil {
			return nil, beep.Format{}, err
		}
		if _, err := io.ReadFull(file, signature[:]); err != nil {
			return nil, beep.Format{}, err
		}
	}
	if string(signature[:]) != "fLaC" {
		return nil, beep.Format{}, fmt.Errorf("invalid FLAC signature")
	}
	block, err := meta.Parse(file)
	if err != nil {
		return nil, beep.Format{}, err
	}
	info, ok := block.Body.(*meta.StreamInfo)
	if !ok || info.NChannels < 1 || info.NChannels > 2 || info.SampleRate == 0 || info.BlockSizeMax == 0 {
		return nil, beep.Format{}, fmt.Errorf("invalid or unsupported FLAC stream info")
	}
	for !block.IsLast {
		block, err = meta.New(file)
		if err != nil {
			return nil, beep.Format{}, err
		}
		if _, err = file.Seek(block.Length, io.SeekCurrent); err != nil {
			return nil, beep.Format{}, err
		}
	}
	start, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, beep.Format{}, err
	}
	end, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, beep.Format{}, err
	}
	d := &flacDecoder{file: file, r: bufio.NewReaderSize(file, 32*1024), info: info, start: start, end: end}
	if err = d.reset(start); err == nil {
		d.frame, err = d.readFrame()
	}
	if err != nil {
		return nil, beep.Format{}, err
	}
	return d, beep.Format{SampleRate: beep.SampleRate(info.SampleRate), NumChannels: int(info.NChannels), Precision: int(info.BitsPerSample+7) / 8}, nil
}

func (d *flacDecoder) reset(offset int64) error {
	if _, err := d.file.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	d.r.Reset(d.file)
	return nil
}

func (d *flacDecoder) offset() (int64, error) {
	p, err := d.file.Seek(0, io.SeekCurrent)
	return p - int64(d.r.Buffered()), err
}

func (d *flacDecoder) readFrame() (*frame.Frame, error) {
	f, err := frame.New(d.r)
	if err != nil {
		return nil, err
	}
	if f.BitsPerSample == 0 {
		f.BitsPerSample = d.info.BitsPerSample
	}
	if f.SampleRate == 0 {
		f.SampleRate = d.info.SampleRate
	}
	if f.Channels.Count() != int(d.info.NChannels) || f.BitsPerSample != d.info.BitsPerSample ||
		f.SampleRate != d.info.SampleRate || f.BlockSize > d.info.BlockSizeMax {
		return nil, fmt.Errorf("FLAC frame does not match stream info")
	}
	if err = f.Parse(); err != nil {
		return nil, err
	}
	return f, nil
}

func (d *flacDecoder) frameStart(f *frame.Frame) int {
	if f.HasFixedBlockSize {
		// The last fixed-size frame can be shorter than the preceding frames.
		return int(f.Num) * int(d.info.BlockSizeMax)
	}
	return int(f.Num)
}

func (d *flacDecoder) Stream(samples [][2]float64) (n int, ok bool) {
	if d.err != nil {
		return 0, false
	}
	scale := 1 / float64(int64(1)<<(d.info.BitsPerSample-1))
	for n < len(samples) {
		if d.frame == nil {
			var err error
			d.frame, err = d.readFrame()
			if err != nil {
				if err != io.EOF {
					d.err = err
				}
				return n, n > 0
			}
			d.inFrame = 0
		}
		count := min(len(samples)-n, int(d.frame.BlockSize)-d.inFrame)
		left := d.frame.Subframes[0].Samples[d.inFrame:]
		right := left
		if d.info.NChannels == 2 {
			right = d.frame.Subframes[1].Samples[d.inFrame:]
		}
		for i := range count {
			samples[n+i] = [2]float64{float64(left[i]) * scale, float64(right[i]) * scale}
		}
		n += count
		d.pos += count
		d.inFrame += count
		if d.inFrame == int(d.frame.BlockSize) {
			d.frame = nil
		}
	}
	return n, n > 0
}

// findFrame locates the next complete, CRC-checked frame after an arbitrary
// byte offset. False sync patterns in compressed samples are skipped.
func (d *flacDecoder) findFrame(offset, limit int64) (*frame.Frame, error) {
	if err := d.reset(offset); err != nil {
		return nil, err
	}
	for offset < limit {
		sync, err := d.r.Peek(2)
		if err != nil {
			return nil, err
		}
		if sync[0] == 0xff && sync[1]&0xfe == 0xf8 {
			if f, err := d.readFrame(); err == nil {
				return f, nil
			}
			if err := d.reset(offset + 1); err != nil {
				return nil, err
			}
		} else if _, err := d.r.ReadByte(); err != nil {
			return nil, err
		}
		offset++
	}
	return nil, io.EOF
}

func (d *flacDecoder) Seek(p int) error {
	if p < 0 || d.Len() > 0 && p > d.Len() {
		return fmt.Errorf("FLAC seek position out of range: %d", p)
	}
	if d.Len() > 0 && p == d.Len() {
		if err := d.reset(d.end); err != nil {
			return err
		}
		d.frame, d.inFrame, d.pos, d.err = nil, 0, p, nil
		return nil
	}
	low, high := d.start, d.end
	for low < high {
		probe := low + (high-low)/2
		f, err := d.findFrame(probe, high)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			high = probe
			continue
		}
		if err != nil {
			return err
		}
		start := d.frameStart(f)
		switch {
		case p < start:
			high = probe
		case p >= start+int(f.BlockSize):
			low, err = d.offset()
			if err != nil {
				return err
			}
		default:
			d.frame, d.inFrame, d.pos, d.err = f, p-start, p, nil
			return nil
		}
	}
	return fmt.Errorf("FLAC frame for sample %d was not found", p)
}

func (d *flacDecoder) Err() error    { return d.err }
func (d *flacDecoder) Len() int      { return int(d.info.NSamples) }
func (d *flacDecoder) Position() int { return d.pos }
func (d *flacDecoder) Close() error  { return d.file.Close() }
