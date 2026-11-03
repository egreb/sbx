package tui

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCreationRunsStepsTogetherAndStopsAfterFailure(t *testing.T) {
	steps := []CreationStep{
		{Name: "api", Run: func() error { return nil }},
		{Name: "web", Run: func() error { return errors.New("clone failed") }},
	}
	for i := 0; i < CreationParallelism; i++ {
		steps = append(steps, CreationStep{Name: fmt.Sprintf("extra%d", i), Run: func() error { return nil }})
	}
	m := newCreation("test", steps)
	view := m.View().Content
	if strings.Count(view, "cloning") != CreationParallelism || !strings.Contains(view, "extra3  queued") {
		t.Fatal(view)
	}
	next, _ := m.Update(creationDone{index: 0})
	m = next.(creation)
	if view := m.View().Content; !strings.Contains(view, "api  complete") || !strings.Contains(view, "1/6 complete") || !strings.Contains(view, "extra2  | cloning") {
		t.Fatal(view)
	}
	next, _ = m.Update(creationDone{index: 1, err: errors.New("clone failed")})
	m = next.(creation)
	if view := m.View().Content; m.err == nil || !strings.Contains(view, "web  failed") || !strings.Contains(view, "extra3  queued") {
		t.Fatal(view)
	}
	for _, index := range []int{2, 3, 4} {
		next, _ = m.Update(creationDone{index: index})
		m = next.(creation)
	}
	if m.running != 0 || m.states[5] != stepQueued {
		t.Fatalf("unexpected state: %#v", m)
	}
}

func TestRunCreationCompletesSteps(t *testing.T) {
	var output bytes.Buffer
	var count atomic.Int32
	err := RunCreation(&output, "example", []CreationStep{
		{Name: "api", Run: func() error { count.Add(1); return nil }},
		{Name: "web", Run: func() error { count.Add(1); return nil }},
	})
	if err != nil || count.Load() != 2 {
		t.Fatalf("count=%d err=%v", count.Load(), err)
	}
}
