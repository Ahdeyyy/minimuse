package audio

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/oto/v3"
)

type Status struct {
	Path                       string
	Playing                    bool
	Ended                      bool
	Position, Duration         time.Duration
	SampleRate, Bits, Channels int
	Error                      string
}

type command struct {
	kind  string
	path  string
	value float64
}

// Engine serializes all device and decoder operations on one worker. The UI
// never decodes, seeks, opens a sound device, or waits for a disk read.
type Engine struct {
	commands chan command
	done     chan struct{}
	closed   atomic.Bool
	onChange func(Status)
	status   atomic.Pointer[Status]
	once     sync.Once
}

func New(onChange func(Status)) *Engine {
	e := &Engine{commands: make(chan command, 64), done: make(chan struct{}), onChange: onChange}
	e.status.Store(&Status{})
	go e.run()
	return e
}

func (e *Engine) send(c command) bool {
	if e.closed.Load() {
		return false
	}
	select {
	case e.commands <- c:
		return true
	default:
		return false
	}
}
func (e *Engine) Load(path string) bool { return e.send(command{kind: "load", path: path}) }
func (e *Engine) Toggle()               { e.send(command{kind: "toggle"}) }
func (e *Engine) Stop()                 { e.send(command{kind: "stop"}) }
func (e *Engine) Seek(d time.Duration)  { e.send(command{kind: "seek", value: d.Seconds()}) }
func (e *Engine) Volume(v float64)      { e.send(command{kind: "volume", value: max(0, min(1, v))}) }
func (e *Engine) Snapshot() Status      { return *e.status.Load() }
func (e *Engine) Close() {
	e.once.Do(func() { e.closed.Store(true); e.commands <- command{kind: "close"} })
	<-e.done
}

func (e *Engine) run() {
	defer close(e.done)
	var device *oto.Context
	var player *oto.Player
	var source *pcmReader
	var state Status
	volume := 1.0
	var tick <-chan time.Time
	timer := time.NewTicker(250 * time.Millisecond)
	timer.Stop()
	defer timer.Stop()
	publish := func(notify bool) {
		if source != nil && player != nil {
			frames := source.frames.Load() - int64(player.BufferedSize()/bytesPerFrame)
			state.Position = time.Duration(max(0, frames)) * time.Second / OutputRate
			if state.Duration > 0 {
				state.Position = min(state.Duration, state.Position)
			}
		}
		s := state
		e.status.Store(&s)
		if notify && e.onChange != nil {
			e.onChange(s)
		}
	}
	release := func() {
		if player != nil {
			player.PauseAndStopReading()
			player = nil
		}
		if source != nil {
			source.decoder.Close()
			source = nil
		}
		if device != nil {
			_ = device.Suspend()
		}
		state.Playing = false
		timer.Stop()
		tick = nil
	}
	defer release()
	for {
		select {
		case c := <-e.commands:
			switch c.kind {
			case "close":
				return
			case "load":
				release()
				state = Status{Path: c.path}
				s, f, err := Decode(c.path)
				if err != nil {
					state.Error = err.Error()
					publish(true)
					continue
				}
				source = newPCM(s, f)
				if device == nil {
					var ready chan struct{}
					device, ready, err = oto.NewContext(&oto.NewContextOptions{SampleRate: OutputRate, ChannelCount: 2, Format: oto.FormatFloat32LE, BufferSize: 60 * time.Millisecond, ApplicationName: "MiniMuse"})
					if err == nil {
						<-ready
						err = device.Err()
					}
				}
				if err == nil {
					err = device.Resume()
				}
				if err != nil {
					release()
					state.Error = fmt.Sprintf("audio device: %v", err)
					publish(true)
					continue
				}
				state.Duration, state.SampleRate, state.Bits, state.Channels = source.duration(), int(f.SampleRate), f.Precision*8, f.NumChannels
				player = device.NewPlayer(source)
				player.SetBufferSize(OutputRate * bytesPerFrame / 8) // 125 ms, 48 KB
				player.SetVolume(volume)
				player.Play()
				state.Playing = true
				tick = timer.C
				timer.Reset(250 * time.Millisecond)
			case "toggle":
				if player == nil {
					continue
				}
				if state.Playing {
					player.PauseAndStopReading()
					_ = device.Suspend()
					timer.Stop()
					tick = nil
				} else {
					if err := device.Resume(); err != nil {
						state.Error = err.Error()
						publish(true)
						continue
					}
					player.Play()
					timer.Reset(250 * time.Millisecond)
					tick = timer.C
				}
				state.Playing = !state.Playing
			case "stop":
				release()
				state.Position = 0
				state.Ended = false
			case "volume":
				volume = c.value
				if player != nil {
					player.SetVolume(volume)
				}
			case "seek":
				if player == nil {
					continue
				}
				_, err := player.Seek(int64(c.value*OutputRate)*bytesPerFrame, io.SeekStart)
				if err != nil {
					state.Error = fmt.Sprintf("seek: %v", err)
				}
			}
			publish(c.kind != "volume")
		case <-tick:
			if player == nil {
				continue
			}
			if err := player.Err(); err != nil {
				release()
				state.Error = err.Error()
				publish(true)
				continue
			}
			if !player.IsPlaying() {
				release()
				state.Position = state.Duration
				state.Ended = true
				publish(true)
			} else {
				publish(false)
			}
		}
	}
}
