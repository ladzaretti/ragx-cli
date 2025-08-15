package cli

import (
	"os"
	"sync"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type safeText struct {
	text string
	mu   sync.RWMutex // mu projects [stopSpinner.text]
}

func (t *safeText) store(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.text = text
}

func (t *safeText) load() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.text
}

type spinnerModel struct {
	spinner spinner.Model
	cancel  func()
	text    *safeText
}

func newSpinner(cancel func(), initialText string) (run func(), stop func()) {
	s := spinner.New()
	s.Spinner = spinner.Dot

	model := &spinnerModel{
		spinner: s,
		cancel:  cancel,
		text:    &safeText{text: initialText},
	}

	prog := tea.NewProgram(model, tea.WithOutput(os.Stderr))

	var (
		once sync.Once
		done = make(chan struct{})
	)

	stop = func() {
		once.Do(func() {
			prog.Quit()
			<-done
		})
	}

	run = func() {
		defer close(done)

		_, _ = prog.Run()
	}

	return run, stop
}

var _ tea.Model = &spinnerModel{}

func (m spinnerModel) Init() tea.Cmd { return m.spinner.Tick }

func (m spinnerModel) View() string { return m.spinner.View() + m.text.load() }

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if key.Type == tea.KeyCtrlC {
			if m.cancel != nil {
				defer m.cancel()
			}

			return m, tea.Quit
		}
	}

	var cmd tea.Cmd

	m.spinner, cmd = m.spinner.Update(msg)

	return m, cmd
}
