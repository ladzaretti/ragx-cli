package cli

import (
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	defaultEllipsisDots = 3 // defaultEllipsisDots is the default maximum number of dots shown by the ellipsis.
	defaultEllipsisMod  = 2 // defaultEllipsisMod is the default number of ticks between ellipsis updates.
)

// ellipsis holds the state and timing configuration for an animated ellipsis.
type ellipsis struct {
	enabled   bool // enabled toggles the ellipsis animation on or off.
	mod       int  // mod is how many ticks must pass before the ellipsis advances.
	tickCount int  // tickCount counts ticks to decide when to advance.
	count     int  // count is the current number of dots to render.
}

func newEllipsis(mod int) *ellipsis { return &ellipsis{mod: mod} }

func (e *ellipsis) show(enabled bool) { e.enabled = enabled }

func (e *ellipsis) tick() {
	e.tickCount = (e.tickCount + 1) % e.mod
	if e.tickCount == 0 {
		e.count = (e.count + 1) % (defaultEllipsisDots + 1)
	}
}

func (e *ellipsis) String() string {
	if e == nil || !e.enabled {
		return ""
	}

	return strings.Repeat(".", e.count)
}

// updateStatusMsg is the message sent into the Bubble Tea
// event loop to change status.
type updateStatusMsg struct {
	text         string
	showEllipsis bool
}

// spinnerModel is the Bubble Tea model that renders the spinner and status suffix.
type spinnerModel struct {
	spinner  spinner.Model
	cancel   func()
	status   string
	ellipsis *ellipsis
}

var _ tea.Model = &spinnerModel{}

func (m spinnerModel) Init() tea.Cmd { return m.spinner.Tick }

func (m spinnerModel) View() string { return m.spinner.View() + m.status + m.ellipsis.String() }

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			if m.cancel != nil {
				defer m.cancel()
			}

			return m, tea.Quit
		}
	case updateStatusMsg:
		m.status = msg.text
		m.ellipsis.show(msg.showEllipsis)

		return m, nil
	}

	var cmd tea.Cmd

	m.ellipsis.tick()
	m.spinner, cmd = m.spinner.Update(msg)

	return m, cmd
}

// spinnerProg controls the lifecycle of the spinner Bubble Tea program.
type spinnerProg struct {
	prog *tea.Program
	once sync.Once
	done chan struct{}
}

func newSpinner(cancel func(), initialText string) *spinnerProg {
	sp := spinner.New()
	sp.Spinner = spinner.Dot

	model := spinnerModel{
		spinner:  sp,
		cancel:   cancel,
		status:   initialText,
		ellipsis: newEllipsis(defaultEllipsisMod),
	}

	prog := tea.NewProgram(model, tea.WithOutput(os.Stderr))

	return &spinnerProg{
		prog: prog,
		done: make(chan struct{}),
	}
}

// run starts the spinner program and blocks until the program exits.
func (s *spinnerProg) run() { defer close(s.done); _, _ = s.prog.Run() }

// stop quits the spinner program and waits for it to exit.
func (s *spinnerProg) stop() {
	s.once.Do(func() {
		s.prog.Quit()
		<-s.done
	})
}

func (s *spinnerProg) sendStatus(text string) { s.prog.Send(updateStatusMsg{text: text}) }

func (s *spinnerProg) sendStatusWithEllipsis(text string) {
	s.prog.Send(updateStatusMsg{text: text, showEllipsis: true})
}
