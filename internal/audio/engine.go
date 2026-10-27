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
	NearEnd                    bool // asks the queue for its next track once per playback/seek
	Position, Duration         time.Duration
	SampleRate, Bits, Channels int
	Error                      string
}

type command struct {
	kind     string
	path     string
	value    float64
	position time.Duration
	paused   bool
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
func (e *Engine) Restore(path string, position time.Duration, playing bool) bool {
	return e.send(command{kind: "load", path: path, position: max(0, position), paused: !playing})
}
func (e *Engine) Toggle()              { e.send(command{kind: "toggle"}) }
func (e *Engine) Stop()                { e.send(command{kind: "stop"}) }
func (e *Engine) Seek(d time.Duration) { e.send(command{kind: "seek", value: d.Seconds()}) }
func (e *Engine) Volume(v float64)     { e.send(command{kind: "volume", value: max(0, min(1, v))}) }
func (e *Engine) Snapshot() Status     { return *e.status.Load() }

// Crossfade sets the overlap duration. Zero disables it.
func (e *Engine) Crossfade(d time.Duration) {
	e.send(command{kind: "crossfade", value: max(0, min(12, d.Seconds()))})
}
func (e *Engine) Close() {
	e.once.Do(func() { e.closed.Store(true); e.commands <- command{kind: "close"} })
	<-e.done
}

func (e *Engine) run() {
	defer close(e.done)
	var device *oto.Context
	var player *oto.Player
	var source *pcmReader
	var outgoing *oto.Player
	var outgoingSource *pcmReader
	var crossfade, overlap time.Duration
	outgoingGain := 1.0
	nearEndSent := false
	var state Status
	volume := 1.0
	var tick <-chan time.Time
	timer := time.NewTicker(40 * time.Millisecond)
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
	releaseOutgoing := func() {
		if outgoing != nil {
			outgoing.PauseAndStopReading()
			_ = outgoing.Close()
			outgoingSource.decoder.Close()
			outgoing, outgoingSource = nil, nil
		}
		overlap = 0
	}
	applyVolume := func() {
		gain := 1.0
		if outgoing != nil {
			gain = fadeProgress(state.Position, overlap)
			outgoing.SetVolume(volume * outgoingGain * (1 - gain))
		}
		if player != nil {
			player.SetVolume(volume * gain)
		}
	}
	release := func() {
		releaseOutgoing()
		if player != nil {
			player.PauseAndStopReading()
			_ = player.Close()
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
				if player != nil {
					player.PauseAndStopReading()
				}
				publish(false)
				return
			case "load":
				publish(false)
				// Keep at most two bounded streaming players alive. A new skip
				// during a fade retires the oldest voice first.
				outgoingGain = 1
				if outgoing != nil {
					outgoingGain = fadeProgress(state.Position, overlap)
				}
				releaseOutgoing()
				if crossfade > 0 && !c.paused && c.position == 0 && player != nil && state.Playing && player.IsPlaying() {
					outgoing, outgoingSource = player, source
					overlap = min(crossfade, max(0, state.Duration-state.Position))
					player, source = nil, nil
				} else {
					release()
				}
				state = Status{Path: c.path}
				nearEndSent = false
				s, f, err := Decode(c.path)
				if err != nil {
					release()
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
				if c.position > 0 {
					position := c.position
					if state.Duration > 0 && position >= state.Duration {
						position = 0
					}
					if _, err = source.Seek(int64(position.Seconds()*OutputRate)*bytesPerFrame, io.SeekStart); err != nil {
						release()
						state.Error = fmt.Sprintf("restore: %v", err)
						publish(true)
						continue
					}
				}
				overlap = min(overlap, state.Duration/2)
				player = device.NewPlayer(source)
				player.SetBufferSize(OutputRate * bytesPerFrame / 8) // 125 ms, 48 KB
				applyVolume()
				if !c.paused {
					player.Play()
					state.Playing = true
					tick = timer.C
					timer.Reset(40 * time.Millisecond)
				} else {
					_ = device.Suspend()
				}
			case "toggle":
				if player == nil {
					continue
				}
				if state.Playing {
					player.PauseAndStopReading()
					if outgoing != nil {
						outgoing.PauseAndStopReading()
					}
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
					if outgoing != nil {
						outgoing.Play()
					}
					timer.Reset(40 * time.Millisecond)
					tick = timer.C
				}
				state.Playing = !state.Playing
			case "stop":
				release()
				state.Position = 0
				state.Ended = false
			case "volume":
				volume = c.value
				applyVolume()
			case "crossfade":
				crossfade = time.Duration(c.value * float64(time.Second))
				nearEndSent = false
				if crossfade == 0 {
					releaseOutgoing()
					applyVolume()
				}
			case "seek":
				if player == nil {
					continue
				}
				releaseOutgoing()
				applyVolume()
				nearEndSent = false
				state.NearEnd = false
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
				if outgoing != nil {
					applyVolume()
					if state.Position >= overlap || !outgoing.IsPlaying() || outgoing.Err() != nil {
						releaseOutgoing()
						applyVolume()
					}
				}
				// Cap the overlap on short tracks so each track has a solo
				// section, and never request the next while still fading in.
				if !nearEndSent && outgoing == nil && shouldCrossfade(state, crossfade) {
					nearEndSent = true
					state.NearEnd = true
					publish(true)
					state.NearEnd = false
				}
			}
		}
	}
}

func fadeProgress(position, duration time.Duration) float64 {
	if duration <= 0 {
		return 1
	}
	return max(0, min(1, float64(position)/float64(duration)))
}

func shouldCrossfade(s Status, duration time.Duration) bool {
	return duration > 0 && s.Playing && !s.Ended && s.Duration > 0 &&
		s.Position >= s.Duration-min(duration, s.Duration/2)
}
