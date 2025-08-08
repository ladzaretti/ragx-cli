package chatui

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/ladzaretti/ragrat/llm"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	listWidth     = 24
	textareaHight = 2
	extraLines    = 7 // 5L ascii + 1L spinner + 1L status bar

	reasoningStartTag = "<think>"
	reasoningEndTag   = "</think>"
)

// model is the bubbletea model that drives the chat interface.
type model struct {
	// ui components

	viewport  viewport.Model
	textarea  textarea.Model
	spinner   spinner.Model
	modelList list.Model

	// chat session

	chat             *llm.ChatSession
	selectedModel    string
	historyBuilder   strings.Builder
	responseBuilder  strings.Builder
	reasoningBuilder strings.Builder

	// focus management
	currentFocus focus
	leaderActive bool

	// state

	loading       bool
	reasoning     bool
	reasoningDone bool
	cancel        context.CancelFunc // cancel for the in-flight LLM request
	lastErr       string             // shown in footer when non-empty

	// layout

	width         int
	listWidth     int
	legendHeight  int
	legendWrapped string
}

// focus is the current component in focus.
type focus int

const (
	_ focus = iota
	focusTextarea
	focusViewport
	focusModelList
)

func (f focus) String() string {
	switch f {
	case focusViewport:
		return "history"
	case focusModelList:
		return "models"
	case focusTextarea:
		return "insert"
	default:
	}

	return ""
}

func (f focus) style() lipgloss.Style {
	switch f {
	case focusTextarea:
		return insertStatusStyle
	case focusViewport:
		return historyStatusStyle
	case focusModelList:
		return modelsStatusStyle
	default:
		return defaultStatusStyle
	}
}

func (m *model) focus(f focus) {
	m.currentFocus = f

	m.refreshLegend()

	if f == focusTextarea {
		m.textarea.Focus()
		return
	}

	m.textarea.Blur()
}

// New creates a new [model].
func New(chat *llm.ChatSession, models []string, selectedModel string) *model {
	ta := textarea.New()
	ta.Placeholder = "Ask anything\n(Press Ctrl+S to submit)"
	ta.Focus()
	ta.Prompt = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.SetHeight(textareaHight)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.Base = lipgloss.NewStyle().
		PaddingTop(0).
		PaddingBottom(0).
		BorderTop(true).
		BorderForeground(lipgloss.Color(mochaSurface0))

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = spinnerCol

	items := make([]list.Item, 0, len(models))
	longest := 0

	selectedIndex := 0
	for i, m := range models {
		if l := lipgloss.Width(m); l > longest {
			longest = l
		}

		if m == selectedModel {
			selectedIndex = i
		}

		items = append(items, listItem(m))
	}

	// ensure we have enough width to show the longest model name, capped at 40.
	lw := max(listWidth, min(longest+2, 40))

	lm := list.New(items, simpleDelegate{}, lw, 10)
	lm.Title = "MODEL SELECT"
	lm.Select(selectedIndex)
	lm.SetFilteringEnabled(false)
	lm.SetShowStatusBar(false)
	lm.SetShowHelp(false)
	lm.Styles.Title = lipgloss.NewStyle().
		PaddingLeft(1).
		PaddingRight(1).
		Foreground(lipgloss.Color(mochaLavender)).
		Background(lipgloss.Color(mochaSurface0))

	return &model{
		chat:          chat,
		viewport:      viewport.New(0, 0),
		modelList:     lm,
		listWidth:     lw,
		textarea:      ta,
		spinner:       sp,
		selectedModel: models[selectedIndex],
		legendHeight:  1,
		currentFocus:  focusTextarea,
	}
}

func (*model) Init() tea.Cmd { return textinput.Blink }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:cyclop
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m.resize(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)

		if m.loading {
			return m, cmd
		}

		return m, nil

	case streamChunk:
		if m.loading { // first chunk has arrived
			prefix := llmPrefixStyle.Render("llm(" + m.selectedModel + "): ")
			m.ensureHistoryNewline()
			m.writeHistory(prefix)
		}

		m.loading = false

		if msg.err != nil {
			m.reasoning, m.reasoningDone = false, false

			switch {
			case errors.Is(msg.err, io.EOF):
				if m.cancel != nil {
					m.cancel()
					m.cancel = nil
				}

				m.writeHistory(m.responseBuilder.String())
				m.responseBuilder.Reset()
			default:
				m.lastErr = strings.ToUpper(msg.err.Error())
				m.reasoningBuilder.Reset()
			}

			return m, nil
		}

		switch strings.TrimSpace(msg.content) {
		case reasoningStartTag:
			m.reasoning = true
		case reasoningEndTag:
			m.reasoning, m.reasoningDone = false, true
			m.reasoningBuilder.Reset()
		default:
			if m.reasoningDone && strings.TrimSpace(msg.content) == "" {
				m.reasoningDone = false
				return m, waitChunk(msg.ch)
			}

			m.writeResponseChunk(msg.content)
			m.updateViewport()
		}

		if m.currentFocus != focusViewport {
			m.viewport.GotoBottom()
		}

		return m, waitChunk(msg.ch)
	}

	// bubble internal updates

	var (
		taCmd tea.Cmd
		vpCmd tea.Cmd
		mlCmd tea.Cmd
	)

	m.textarea, taCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.modelList, mlCmd = m.modelList.Update(msg)

	cmds := []tea.Cmd{vpCmd, taCmd, mlCmd}
	if m.loading {
		cmds = append(cmds, m.spinner.Tick)
	}

	return m, tea.Batch(cmds...)
}

func (m *model) View() string {
	leftSide := lipgloss.JoinVertical(lipgloss.Left,
		asciiComponentView, // or asciiComponent{}.View()
		m.viewport.View(),
	)

	mainArea := lipgloss.JoinHorizontal(lipgloss.Top, leftSide, m.modelList.View())

	modeLabel, legendItemStyle := m.currentFocus.String(), m.currentFocus.style()
	if m.leaderActive {
		modeLabel, legendItemStyle = "leader", leaderStatusStyle
	}

	footerContent := lipgloss.JoinHorizontal(
		lipgloss.Left,
		legendItemStyle.Render(strings.ToUpper(modeLabel)),
	)

	if m.lastErr != "" {
		footerContent = lipgloss.JoinHorizontal(lipgloss.Left, footerContent, errorStatusStyle.Render(m.lastErr))
	}

	status := barStyle.Width(m.viewport.Width + m.modelList.Width()).Render(footerContent)

	var b strings.Builder

	b.WriteString(mainArea)
	b.WriteString("\n")

	if m.loading {
		b.WriteString(m.spinner.View())
	}

	b.WriteString("\n")
	b.WriteString(m.textarea.View())
	b.WriteString("\n")
	b.WriteString(m.legendWrapped)
	b.WriteString("\n")
	b.WriteString(status)

	return b.String()
}

// handleKey routes key events based on focus.
func (m *model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "ctrl+a":
		m.leaderActive = !m.leaderActive

		m.refreshLegend()

		if m.leaderActive {
			m.legendWrapped = lipgloss.NewStyle().Width(m.width).Render(m.legend())
			m.textarea.Blur()

			return m, nil
		}

		m.focus(focusTextarea)

		return m, textinput.Blink

	case "esc": //nolint:goconst
		if m.leaderActive {
			m.leaderActive = false

			m.refreshLegend()
			m.focus(focusTextarea)

			return m, textinput.Blink
		}

	case "ctrl+s":
		if m.loading {
			return m, nil
		}

		prompt := strings.TrimSpace(m.textarea.Value())
		if prompt == "" {
			return m, nil
		}

		return m.sendPrompt(prompt)

	default:
	}

	if m.leaderActive {
		m.refreshLegend()
		return m.handleLeaderKey(k.String())
	}

	switch m.currentFocus {
	case focusViewport:
		return m.handleViewport(k)
	case focusModelList:
		return m.handleModelList(k)
	case focusTextarea:
		return m.handleTextarea(k)
	default:
	}

	return m, nil
}

//nolint:unparam
var leaderMap = map[string]func(*model) (tea.Model, tea.Cmd){
	"q": func(m *model) (tea.Model, tea.Cmd) { return m, tea.Quit },
	"h": func(m *model) (tea.Model, tea.Cmd) { m.focus(focusViewport); return m, nil },
	"m": func(m *model) (tea.Model, tea.Cmd) { m.focus(focusModelList); return m, nil },
	"l": func(m *model) (tea.Model, tea.Cmd) {
		m.historyBuilder.Reset()
		m.viewport.SetContent("")
		m.focus(focusTextarea)
		return m, textinput.Blink
	},
}

func (m *model) handleLeaderKey(k string) (tea.Model, tea.Cmd) {
	m.leaderActive = false
	if f, ok := leaderMap[k]; ok {
		return f(m)
	}

	// Unknown leader key — return to textarea
	if m.currentFocus == focusTextarea {
		m.focus(focusTextarea)

		return m, textinput.Blink
	}

	return m, nil
}

func (m *model) handleViewport(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.focus(focusTextarea)

		return m, textinput.Blink

	default:
	}

	var cmd tea.Cmd

	m.viewport, cmd = m.viewport.Update(k)

	return m, cmd
}

func (m *model) handleModelList(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "enter":
		if it, ok := m.modelList.SelectedItem().(listItem); ok {
			m.selectedModel = string(it)
		}

		m.focus(focusTextarea)

		return m, textinput.Blink
	}

	var cmd tea.Cmd

	m.modelList, cmd = m.modelList.Update(k)

	return m, cmd
}

func (m *model) handleTextarea(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		if m.cancel != nil {
			m.cancel()
			m.cancel = nil
			m.loading = false
			m.ensureHistoryNewline()
		}

		return m, nil

	default:
	}

	var cmd tea.Cmd

	m.textarea, cmd = m.textarea.Update(k)

	return m, cmd
}

// sendPrompt starts a streaming request and wires chunks back to Update.
func (m *model) sendPrompt(p string) (tea.Model, tea.Cmd) {
	// cancel previous request if exists
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	m.loading = true
	m.lastErr = ""

	m.ensureHistoryNewline()
	m.writeHistory(userPrefixStyle.Render("you:") + " " + p + "\n")
	m.updateViewport()

	ch := sendStream(ctx, m.chat, m.selectedModel, p)

	m.textarea.Reset()

	m.viewport.GotoBottom()

	return m, tea.Batch(m.spinner.Tick, waitChunk(ch))
}

func (m *model) resize(w tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	vpWidth := max(w.Width-m.listWidth, 1)
	m.viewport.Width = vpWidth

	m.textarea.SetWidth(vpWidth)
	m.textarea.SetHeight(textareaHight)

	m.refreshLegend()

	m.legendHeight = lipgloss.Height(m.legendWrapped)

	availHeight := w.Height - m.textarea.Height() - m.legendHeight - extraLines

	m.viewport.Height = max(availHeight, 1)
	m.modelList.SetSize(m.listWidth, availHeight)

	wrapped := lipgloss.NewStyle().Width(m.viewport.Width).Render(m.historyBuilder.String())

	m.viewport.SetContent(wrapped)

	return m, nil
}

func (m *model) updateViewport() {
	view := m.historyBuilder.String()

	if m.responseBuilder.Len() > 0 {
		view += m.responseBuilder.String()
	}

	if m.reasoningBuilder.Len() > 0 {
		reasoning := m.reasoningBuilder.String()
		view += "\n" + reasoningStyle.Render(reasoning) + "\n"
	}

	wrapped := lipgloss.NewStyle().
		Width(m.viewport.Width).
		Render(view)

	m.viewport.SetContent(wrapped)
}

func (m *model) refreshLegend() {
	m.legendWrapped = lipgloss.NewStyle().
		Width(m.width).
		Render(m.legend())
}

func (m *model) legend() string {
	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color(mochaOverlay2)).
		Render(" • ")

	legendItem := func(k, label string) string {
		return lipgloss.JoinHorizontal(lipgloss.Left,
			keyStyle.Render(k),
			dimStyle.Render(" "+label),
		)
	}

	switch {
	case m.leaderActive:
		return lipgloss.JoinHorizontal(lipgloss.Left,
			legendItem("H", "HISTORY"), divider,
			legendItem("M", "CHANGE MODEL"), divider,
			legendItem("L", "CLEAR CHAT"), divider,
			legendItem("Q", "QUIT"), divider,
			legendItem("ESC", "CANCEL LEADER"),
		)

	case m.currentFocus == focusModelList:
		return lipgloss.JoinHorizontal(lipgloss.Left,
			legendItem("▲/K ▼/J", "SCROLL"), divider,
			legendItem("ENTER", "SELECT MODEL"), divider,
			legendItem("ESC", "CANCEL"),
		)

	case m.currentFocus == focusViewport:
		return lipgloss.JoinHorizontal(lipgloss.Left,
			legendItem("▲/K ▼/J", "SCROLL"), divider,
			legendItem("ESC", "BACK"),
		)

	default:
		return lipgloss.JoinHorizontal(lipgloss.Left,
			legendItem("^S", "SEND"), divider,
			legendItem("ESC", "CANCEL"), divider,
			legendItem("^A", "LEADER MODE"), divider,
			legendItem("^C", "QUIT"),
		)
	}
}

func (m *model) ensureHistoryNewline() {
	if m.historyBuilder.Len() == 0 {
		return
	}

	if strings.HasSuffix(m.historyBuilder.String(), "\n") {
		return
	}

	m.historyBuilder.WriteByte('\n')
}

func (m *model) writeHistory(s string) {
	m.historyBuilder.WriteString(s)
}

func (m *model) writeResponseChunk(s string) {
	if m.reasoning {
		m.reasoningBuilder.WriteString(s)
	} else {
		m.responseBuilder.WriteString(s)
	}
}
