package app

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"sbx/internal/tui"
)

func TestRunParallelLimitsConcurrencyAndReportsFailure(t *testing.T) {
	var running, peak, ran atomic.Int32
	step := func(fail bool) tui.CreationStep {
		return tui.CreationStep{Run: func() error {
			ran.Add(1)
			now := running.Add(1)
			for {
				old := peak.Load()
				if now <= old || peak.CompareAndSwap(old, now) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			running.Add(-1)
			if fail {
				return errors.New("clone failed")
			}
			return nil
		}}
	}
	steps := []tui.CreationStep{step(false), step(false), step(false)}
	if err := runParallel(steps, 2); err != nil || ran.Load() != 3 || peak.Load() != 2 {
		t.Fatalf("err=%v ran=%d peak=%d", err, ran.Load(), peak.Load())
	}
	ran.Store(0)
	steps = []tui.CreationStep{step(true), step(false), step(false), step(false)}
	if err := runParallel(steps, 1); err == nil || ran.Load() != 1 {
		t.Fatalf("err=%v ran=%d", err, ran.Load())
	}
}
