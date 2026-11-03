package tui

import (
	"fmt"
	"io"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type CreationStep struct {
	Name string
	Run  func() error
}

// CreationParallelism is how many steps run at once.
const CreationParallelism = 4

type stepState int

const (
	stepQueued stepState = iota
	stepRunning
	stepComplete
	stepFailed
)

type creationDone struct {
	index int
	err   error
}
type creationTick time.Time

type creation struct {
	workspace string
	steps     []CreationStep
	states    []stepState
	running   int
	complete  int
	frame     int
	err       error
	initial   tea.Cmd
}

// newCreation starts the first steps here because Init cannot keep changes
// to the model.
func newCreation(workspace string, steps []CreationStep) creation {
	m, initial := creation{workspace: workspace, steps: steps, states: make([]stepState, len(steps))}.start()
	m.initial = initial
	return m
}

func (m creation) run(index int) tea.Cmd {
	step := m.steps[index]
	return func() tea.Msg { return creationDone{index, step.Run()} }
}

// start launches queued steps up to the parallelism limit. After a failure
// nothing new starts; running steps are left to finish.
func (m creation) start() (creation, tea.Cmd) {
	var commands []tea.Cmd
	for i := range m.steps {
		if m.err != nil || m.running == CreationParallelism {
			break
		}
		if m.states[i] == stepQueued {
			m.states[i] = stepRunning
			m.running++
			commands = append(commands, m.run(i))
		}
	}
	return m, tea.Batch(commands...)
}

func creationPulse() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return creationTick(t) })
}

func (m creation) Init() tea.Cmd {
	return tea.Batch(m.initial, creationPulse())
}

func (m creation) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case creationDone:
		m.running--
		if msg.err != nil {
			m.states[msg.index] = stepFailed
			if m.err == nil {
				m.err = msg.err
			}
		} else {
			m.states[msg.index] = stepComplete
			m.complete++
		}
		if m.running == 0 && (m.err != nil || m.complete == len(m.steps)) {
			return m, tea.Quit
		}
		return m.start()
	case creationTick:
		m.frame++
		return m, creationPulse()
	}
	return m, nil
}

func (m creation) View() tea.View {
	var view strings.Builder
	fmt.Fprintf(&view, "Creating %s\n\nCloning projects (%d/%d complete)\n\n", m.workspace, m.complete, len(m.steps))
	for i, step := range m.steps {
		status := "queued"
		switch m.states[i] {
		case stepRunning:
			status = string("|/-\\"[m.frame%4]) + " cloning"
		case stepComplete:
			status = "complete"
		case stepFailed:
			status = "failed"
		}
		fmt.Fprintf(&view, "  %s  %s\n", step.Name, status)
	}
	if m.err != nil {
		fmt.Fprintf(&view, "\n%s\n", m.err)
	}
	return tea.NewView(view.String())
}

// RunCreation leaves a completed project list in the terminal before setup continues.
// Input stays with subprocesses; cancellation comes from the caller's context.
func RunCreation(out io.Writer, workspace string, steps []CreationStep) error {
	if len(steps) == 0 {
		return nil
	}
	p := tea.NewProgram(newCreation(workspace, steps), tea.WithInput(nil), tea.WithOutput(out), tea.WithoutSignalHandler())
	result, err := p.Run()
	if err != nil {
		return err
	}
	return result.(creation).err
}
