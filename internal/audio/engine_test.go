package audio

import (
	"sync"
	"testing"
	"time"
)

func TestCommandCoalescingPreservesPlaybackOrder(t *testing.T) {
	e := &Engine{commandReady: make(chan struct{}, 1)}
	e.send(command{kind: "load", path: "first"})
	for i := range 1000 {
		if !e.send(command{kind: "seek", value: float64(i)}) {
			t.Fatal("slider filled the command queue")
		}
	}
	e.send(command{kind: "toggle"})
	e.send(command{kind: "load", path: "second"})
	e.send(command{kind: "seek", value: 42})
	for i := range 1000 {
		e.send(command{kind: "volume", value: float64(i) / 1000})
	}
	want := []command{{kind: "load", path: "first"}, {kind: "seek", value: 999}, {kind: "toggle"}, {kind: "load", path: "second"}, {kind: "seek", value: 42}, {kind: "volume", value: .999}}
	for _, c := range want {
		select {
		case <-e.commandReady:
			if got := e.takeCommand(); got != c {
				t.Fatalf("got %+v, want %+v", got, c)
			}
		default:
			t.Fatal("queued command lost its notification", c)
		}
	}
	select {
	case <-e.commandReady:
		t.Fatal("empty queue left a notification")
	default:
	}
}

func TestConcurrentCommandsAndClose(t *testing.T) {
	e := New(nil)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for i := range 1000 {
				e.Volume(float64(i%100) / 100)
				e.Seek(time.Duration(i) * time.Millisecond)
			}
		})
	}
	wg.Go(e.Close)
	wg.Wait()
	e.Close()
	if e.Load("after close") {
		t.Fatal("closed engine accepted work")
	}
}

func TestSnapshotInterpolation(t *testing.T) {
	now := time.Now()
	base := Status{Path: "song", Position: time.Second, Duration: 2 * time.Second, Playing: true}
	for _, tc := range []struct {
		name  string
		state Status
		age   time.Duration
		want  time.Duration
	}{
		{"between samples", base, 16 * time.Millisecond, 1016 * time.Millisecond},
		{"worker stalled", base, time.Second, time.Second + statusInterval},
		{"clock before sample", base, -time.Second, time.Second},
		{"paused", Status{Position: time.Second}, time.Second, time.Second},
		{"ended", Status{Position: time.Second, Playing: true, Ended: true}, time.Second, time.Second},
		{"error", Status{Position: time.Second, Playing: true, Error: "device"}, time.Second, time.Second},
		{"duration cap", Status{Position: 1990 * time.Millisecond, Duration: 2 * time.Second, Playing: true}, statusInterval, 2 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := playbackSnapshot{status: tc.state, at: now}
			if got := s.positionAt(now.Add(tc.age)); got.Position != tc.want || s.status != tc.state {
				t.Fatal(got, s.status)
			}
		})
	}
}
