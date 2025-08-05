package model

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ladzaretti/ragrat/llm"
)

const (
	listWidth     = 24
	textareaHight = 2
	extraLines    = 2 // spinner + status bar
)

var (
	// https://catppuccin.com/palette/

	// Base/background layers
	mochaCrust    = "#11111b"
	mochaMantle   = "#181825"
	mochaSurface0 = "#313244"

	// Text and UI detail
	mochaText     = "#cdd6f4"
	mochaSubtext0 = "#a6adc8"
	mochaOverlay2 = "#9399b2"

	// Semantic accent colors
	mochaRed      = "#f38ba8"
	mochaGreen    = "#a6e3a1"
	mochaYellow   = "#f9e2af"
	mochaBlue     = "#89b4fa"
	mochaMauve    = "#cba6f7"
	mochaLavender = "#b4befe"

	keyStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaSubtext0)).Bold(true)
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaOverlay2))
	boldStyle  = lipgloss.NewStyle().Bold(true)
	spinnerCol = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaMauve))

	itemStyle         = lipgloss.NewStyle().Padding(0, 1)
	selectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(mochaLavender)).
				Bold(true).
				Padding(0, 1)

	// status bar related style.

	insertStatusStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(mochaGreen)).
				Foreground(lipgloss.Color(mochaCrust)).
				Bold(true).Padding(0, 1)

	leaderStatusStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(mochaMauve)).
				Foreground(lipgloss.Color(mochaCrust)).
				Bold(true).Padding(0, 1)

	historyStatusStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(mochaBlue)). // pick your preferred color
				Foreground(lipgloss.Color(mochaCrust)).
				Bold(true).Padding(0, 1)

	modelsStatusStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(mochaYellow)). // or mochaLavender
				Foreground(lipgloss.Color(mochaCrust)).
				Bold(true).Padding(0, 1)

	defaultStatusStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(mochaSurface0)).
				Foreground(lipgloss.Color(mochaText)).
				Bold(true).
				Padding(0, 1)

	errorStatusStyle = lipgloss.NewStyle().
				Background(lipgloss.Color(mochaRed)).
				Foreground(lipgloss.Color(mochaCrust)).
				Bold(true).Padding(0, 1)

	barStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(mochaMantle)).
			Foreground(lipgloss.Color(mochaText)).
			Padding(0, 0).
			BorderTop(true).
			BorderForeground(lipgloss.Color(mochaSurface0))
)

// listItem wraps a model name so it can satisfy list.Item.
type listItem string

func (i listItem) Title() string       { return string(i) }
func (i listItem) FilterValue() string { return string(i) }
func (listItem) Description() string   { return "" }

type simpleDelegate struct{}

func (simpleDelegate) Height() int                         { return 1 }
func (simpleDelegate) Spacing() int                        { return 0 }
func (simpleDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

const (
	leadSel   = "│ "
	leadUnsel = "  "
)

func (simpleDelegate) Render(w io.Writer, m list.Model, index int, it list.Item) {
	li, ok := it.(listItem)
	if !ok {
		return
	}

	name := string(li)

	prefix := leadUnsel
	style := itemStyle

	if index == m.Index() {
		prefix = leadSel
		style = selectedItemStyle
	}

	_, _ = fmt.Fprint(w, style.Render(prefix+name))
}

// model is the bubbletea model that drives the chat interface.
type model struct {
	// ui components

	viewport  viewport.Model
	textarea  textarea.Model
	spinner   spinner.Model
	modelList list.Model

	// chat session

	chat           *llm.ChatSession
	selectedModel  string
	historyBuilder strings.Builder

	// focus management
	currentFocus focus
	leaderActive bool

	// state

	loading bool
	cancel  context.CancelFunc // cancel for the in-flight LLM request
	lastErr string             // shown in footer when non-empty

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
func New(chat *llm.ChatSession, models []string) *model {
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

	for _, m := range models {
		if l := lipgloss.Width(m); l > longest {
			longest = l
		}

		items = append(items, listItem(m))
	}

	// ensure we have enough width to show the longest model name, capped at 40.
	lw := max(listWidth, min(longest+2, 40))

	lm := list.New(items, simpleDelegate{}, lw, 10)
	lm.Title = "MODEL SELECT"
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
		selectedModel: models[0],
		legendHeight:  1,
		currentFocus:  focusTextarea,
	}
}

func (*model) Init() tea.Cmd { return textinput.Blink }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.loading = false

		if msg.err != nil {
			m.lastErr = msg.err.Error()
			return m, nil
		}

		m.writeHistory(msg.content)

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

func (m *model) ensureHistoryNewline() {
	if m.historyBuilder.Len() == 0 {
		return
	}

	if strings.HasSuffix(m.historyBuilder.String(), "\n") {
		return
	}

	m.historyBuilder.WriteByte('\n')
}

// writeHistory appends to builder and refreshes viewport.
func (m *model) writeHistory(s string) {
	m.historyBuilder.WriteString(s)

	wrapped := lipgloss.NewStyle().
		Width(m.viewport.Width).
		Render(m.historyBuilder.String())

	m.viewport.SetContent(wrapped)
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

func (m *model) refreshLegend() {
	m.legendWrapped = lipgloss.NewStyle().
		Width(m.width).
		Render(m.legend())
}

//nolint:unparam
var leaderMap = map[string]func(*model) (tea.Model, tea.Cmd){
	"q": func(m *model) (tea.Model, tea.Cmd) { return m, tea.Quit },
	"h": func(m *model) (tea.Model, tea.Cmd) { m.focus(focusViewport); return m, nil },
	"m": func(m *model) (tea.Model, tea.Cmd) { m.focus(focusModelList); return m, nil },
	"l": func(m *model) (tea.Model, tea.Cmd) {
		m.historyBuilder = strings.Builder{}
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
			m.historyBuilder.WriteByte('\n')
		}

		return m, nil

	default:
	}

	var cmd tea.Cmd

	m.textarea, cmd = m.textarea.Update(k)

	return m, cmd
}

type chunk struct {
	err     error
	content string
}

type streamChunk struct {
	chunk
	ch <-chan chunk
}

func waitChunk(ch <-chan chunk) tea.Cmd {
	return func() tea.Msg {
		c, ok := <-ch
		if !ok {
			return nil
		}

		return streamChunk{chunk: c, ch: ch}
	}
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
	m.writeHistory(boldStyle.Render("you:") + " " + p + "\n")

	// buffered to avoid blocking OpenAI iterator
	ch := make(chan chunk)

	go func() {
		defer close(ch)

		stream, err := m.chat.SendStreaming(ctx, m.selectedModel, p)
		if err != nil {
			ch <- chunk{err: err}
			return
		}

		for res, err := range stream {
			if err != nil {
				ch <- chunk{err: err}
				return
			}
			ch <- chunk{content: res.Content}
		}
	}()

	m.textarea.Reset()

	m.writeHistory(boldStyle.Render("llm(" + m.selectedModel + "): "))

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

func (m *model) View() string {
	mainArea := lipgloss.JoinHorizontal(lipgloss.Top, m.viewport.View(), m.modelList.View())

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
