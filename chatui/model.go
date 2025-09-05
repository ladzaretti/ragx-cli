package chatui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ladzaretti/ragx/llm"
	"github.com/ladzaretti/ragx/types"
	"github.com/ladzaretti/ragx/vecdb"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	listWidth      = 24
	textareaHeight = 2
	statusBarLines = 1

	reasoningStartTag = "<think>"
	reasoningEndTag   = "</think>"
)

// model is the bubbletea model that drives the chat interface.
type model struct {
	// ui components

	viewport        viewport.Model
	textarea        textarea.Model
	spinner         spinner.Model
	thinkingSpinner spinner.Model
	modelList       list.Model

	// chat session

	providers types.Providers
	vecdb     *vecdb.VectorDB
	llmConfig LLMConfig

	historyBuilder   strings.Builder
	responseBuilder  strings.Builder
	reasoningBuilder strings.Builder

	// focus management

	currentFocus focus
	prefixActive bool

	// state

	loading       bool
	reasoning     bool
	reasoningDone bool
	reasoningShow bool
	asciiShow     bool
	selectedModel string
	contextUsed   llm.ContextUsage
	cancel        context.CancelFunc // cancel for the in-flight LLM request
	lastErr       string             // shown in footer when non-empty

	// layout

	width         int
	height        int
	listWidth     int
	legendHeight  int
	legendWrapped string
	statusHeight  int
	statusWrapped string
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

// LLMConfig contains high-level LLM settings for the RAG pipeline.
type LLMConfig struct {
	Models             []types.ModelConfig // Models lists optional per model metadata.
	DefaultModel       string              // DefaultModel is the model used for chat/generation when none is specified.
	UserPromptTmpl     string              // UserPromptTmpl is a go template used to build the user query + context.
	EmbeddingModel     string              // EmbeddingModel is the model used to produce embeddings.
	RetrievalTopK      int                 // RetrievalTopK is the number of results to fetch from the vector DB for RAG. Use 0 to disable retrieval.
	DefaultContext     int                 // DefaultContext is the fallback maximum context length (in tokens).
	DefaultTemperature *float64            // DefaultTemperature is the fallback sampling temperature.
}

// New creates a new [model].
func New(providers types.Providers, vecdb *vecdb.VectorDB, llmConfig LLMConfig) *model {
	ta := textarea.New()
	ta.Placeholder = "Ask anything\n(Press Ctrl+S to submit)"
	ta.Focus()
	ta.Prompt = ""
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.SetHeight(textareaHeight)
	ta.ShowLineNumbers = false
	ta.FocusedStyle.Base = lipgloss.NewStyle().
		PaddingTop(0).
		PaddingBottom(0).
		BorderTop(true).
		BorderForeground(lipgloss.Color(mochaSurface0))

	spinnerPlain := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(spinnerStyle),
	)

	spinnerThinking := spinner.New(spinner.WithSpinner(spinner.Spinner{
		Frames: []string{
			"thinking",
			"thinking.",
			"thinking..",
			"thinking...",
		},
		FPS: time.Second / 4,
	}))

	items := make([]list.Item, 0, 32)
	longest := 0

	selectedIndex, selectedModel := 0, llmConfig.DefaultModel
	for i, p := range providers {
		for j, m := range p.AvailableModels {
			if l := lipgloss.Width(m); l > longest {
				longest = l
			}

			if m == selectedModel {
				selectedIndex = i + j
			}

			items = append(items, listItem(m))
		}
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
		providers:       providers,
		vecdb:           vecdb,
		llmConfig:       llmConfig,
		selectedModel:   selectedModel,
		viewport:        viewport.New(0, 0),
		modelList:       lm,
		listWidth:       lw,
		textarea:        ta,
		spinner:         spinnerPlain,
		thinkingSpinner: spinnerThinking,
		reasoningShow:   false,
		asciiShow:       true,
		legendHeight:    1,
		currentFocus:    focusTextarea,
	}
}

func (*model) Init() tea.Cmd { return textinput.Blink }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:cyclop,gocognit
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m.updateLayout(msg)

	case tea.BlurMsg:
		m.textarea.Blur()

	case tea.FocusMsg:
		if m.currentFocus == focusTextarea {
			m.focus(focusTextarea)
		}

	case spinner.TickMsg:
		var plainCmd, thinkingCmd tea.Cmd

		m.spinner, plainCmd = m.spinner.Update(msg)
		m.thinkingSpinner, thinkingCmd = m.thinkingSpinner.Update(msg)

		cmds := []tea.Cmd{}

		if m.loading {
			cmds = append(cmds, plainCmd)
		}

		if m.reasoning {
			cmds = append(cmds, thinkingCmd)
		}

		if len(cmds) > 0 {
			return m, tea.Batch(cmds...)
		}

		return m, nil

	case ragErr:
		m.loading = false
		m.lastErr = strings.ToUpper(msg.err.Error())
		m.updateViewport()

		return m, nil
	case ragReady:
		return m, waitChunk(msg.ch)

	case streamChunk:
		if m.loading { // first chunk has arrived
			prefix := llmPrefixStyle.Render("llm(" + m.selectedModel + "): ")
			m.ensureHistoryNewline()
			m.writeHistory(prefix)
		}

		m.loading = false

		if msg.Err != nil {
			m.reasoning, m.reasoningDone = false, false

			switch {
			case errors.Is(msg.Err, io.EOF):
				if m.cancel != nil {
					m.cancel()
					m.cancel = nil
				}

				provider, err := m.providers.ProviderFor(m.selectedModel)
				if err != nil {
					m.lastErr = strings.ToUpper(msg.Err.Error())
					m.contextUsed = llm.ContextUsage{}
				} else {
					m.contextUsed = provider.Session.ContextUsed()
				}

				m.writeHistory(m.responseBuilder.String())
				m.responseBuilder.Reset()
			default:
				m.lastErr = strings.ToUpper(msg.Err.Error())
				m.reasoningBuilder.Reset()
			}

			return m, nil
		}

		reasoningStarted := false

		switch strings.TrimSpace(msg.Content) {
		case reasoningStartTag:
			m.reasoning, reasoningStarted = true, true
		case reasoningEndTag:
			m.reasoning, m.reasoningDone = false, true
			m.reasoningBuilder.Reset()
		default:
			// discard whitespaces-only chunk after reasoning is done.
			if m.reasoningDone && strings.TrimSpace(msg.Content) == "" {
				m.reasoningDone = false
				return m, waitChunk(msg.ch)
			}

			m.writeResponseChunk(msg.Content)
			m.updateViewport()
		}

		if m.currentFocus != focusViewport {
			m.viewport.GotoBottom()
		}

		cmds := []tea.Cmd{waitChunk(msg.ch)}

		if reasoningStarted {
			cmds = append(cmds, m.thinkingSpinner.Tick)
		}

		return m, tea.Batch(cmds...)
	}

	// bubble internal updates

	return m.defaultUpdate(msg)
}

func (m *model) defaultUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		taCmd tea.Cmd
		vpCmd tea.Cmd
		mlCmd tea.Cmd
	)

	m.textarea, taCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)
	m.modelList, mlCmd = m.modelList.Update(msg)

	cmds := []tea.Cmd{vpCmd, taCmd, mlCmd}

	return m, tea.Batch(cmds...)
}

func (m *model) View() string {
	left := []string{m.viewport.View()}
	if m.asciiShow {
		left = append([]string{asciiComponentView}, left...)
	}

	main := lipgloss.JoinVertical(lipgloss.Left, left...)

	modeLabel, legendItemStyle := m.currentFocus.String(), m.currentFocus.style()
	if m.prefixActive {
		modeLabel, legendItemStyle = "prefix", prefixStatusStyle
	}

	footerItems := []string{
		legendItemStyle.Render(strings.ToUpper(modeLabel)),
	}

	if m.lastErr != "" {
		footerItems = append(footerItems, errorStatusStyle.Render(m.lastErr))
	} else {
		var (
			context          = cmp.Or(m.contextUsed.Max, m.llmConfig.DefaultContext)
			used, percentage = m.contextUsed.Used, 0
		)

		if context > 0 {
			percentage = min((used*100)/context, 100)
		}

		footerItems = append(footerItems,
			truncate(selectedModelStatusStyle, m.selectedModel, 28),
			truncate(embedSelectedModelStatusStyle, m.llmConfig.EmbeddingModel, 22),
			contextStatusStyle.Render(fmt.Sprintf("Ctx %d%%", percentage)),
		)
	}

	m.statusWrapped = barStyle.Width(m.width).
		Render(lipgloss.JoinHorizontal(lipgloss.Left, footerItems...))

	var b strings.Builder

	b.WriteString(main)
	b.WriteString("\n")

	if m.loading {
		b.WriteString(m.spinner.View())
	}

	b.WriteString("\n")
	b.WriteString(m.textarea.View())
	b.WriteString("\n")
	b.WriteString(m.legendWrapped)
	b.WriteString("\n")
	b.WriteString(m.statusWrapped)

	if m.currentFocus == focusModelList {
		return m.renderModelPopup()
	}

	return b.String()
}

type layoutMsg = tea.WindowSizeMsg

// handleKey routes key events based on focus.
func (m *model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) { //nolint:cyclop
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "ctrl+n":
		m.historyBuilder.Reset()
		m.viewport.SetContent("")
		m.contextUsed.Used = 0
		m.focus(focusTextarea)

		return m, textinput.Blink

	case "ctrl+a":
		m.prefixActive = !m.prefixActive

		if m.prefixActive {
			m.refreshLegend()
			m.textarea.Blur()
		} else {
			m.focus(focusTextarea)
		}

		m, cmd := m.updateLayout(layoutMsg{
			Width:  m.width,
			Height: m.height,
		})

		return m, tea.Batch(cmd, textinput.Blink)

	case "esc": //nolint:goconst
		if m.prefixActive {
			m.prefixActive = false

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

	if m.prefixActive {
		m.refreshLegend()
		return m.handlePrefixKey(k.String())
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
var prefixMap = map[string]func(*model) (tea.Model, tea.Cmd){
	"q": func(m *model) (tea.Model, tea.Cmd) { return m, tea.Quit },
	"h": func(m *model) (tea.Model, tea.Cmd) { m.focus(focusViewport); return m, nil },
	"m": func(m *model) (tea.Model, tea.Cmd) { m.focus(focusModelList); return m, nil },
	"a": func(m *model) (tea.Model, tea.Cmd) {
		m.asciiShow = !m.asciiShow
		m.focus(focusTextarea)

		resize := func() tea.Msg {
			return tea.WindowSizeMsg{Width: m.width, Height: m.height}
		}

		return m, tea.Batch(textinput.Blink, resize)
	},
	"r": func(m *model) (tea.Model, tea.Cmd) {
		m.reasoningShow = !m.reasoningShow
		m.focus(focusTextarea)
		return m, textinput.Blink
	},
	"l": func(m *model) (tea.Model, tea.Cmd) {
		m.historyBuilder.Reset()
		m.viewport.SetContent("")
		m.focus(focusTextarea)
		return m, textinput.Blink
	},
}

func (m *model) handlePrefixKey(k string) (tea.Model, tea.Cmd) {
	m.prefixActive = false
	if f, ok := prefixMap[k]; ok {
		return f(m)
	}

	// Unknown prefix key — return to textarea
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
	default:
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
func (m *model) sendPrompt(q string) (tea.Model, tea.Cmd) {
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
	m.writeHistory(userPrefixStyle.Render("you:") + " " + q + "\n")
	m.updateViewport()

	m.textarea.Reset()
	m.viewport.GotoBottom()

	return m, tea.Batch(m.spinner.Tick, m.startRAGCmd(ctx, q))
}

func (m *model) updateLayout(w tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	vpWidth := max(w.Width, 1)
	m.viewport.Width = w.Width

	m.textarea.SetWidth(vpWidth)
	m.textarea.SetHeight(textareaHeight)

	m.refreshLegend()

	m.legendHeight = lipgloss.Height(m.legendWrapped)
	m.statusHeight = lipgloss.Height(m.statusWrapped)

	reserved := statusBarLines
	if m.asciiShow {
		reserved += asciiLines
	}

	availHeight := w.Height - m.textarea.Height() - m.legendHeight - m.statusHeight - reserved

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
		var block string

		if m.reasoningShow {
			block = reasoningTextStyle.Render(m.reasoningBuilder.String())
		} else {
			block = reasoningSpinnerStyle.Render(m.thinkingSpinner.View())
		}

		if block != "" {
			view += "\n" + block + "\n"
		}
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
	case m.prefixActive:
		return lipgloss.JoinHorizontal(lipgloss.Left,
			legendItem("H", "HISTORY"), divider,
			legendItem("R", m.reasoningLegendLabel()), divider,
			legendItem("M", "CHANGE MODEL"), divider,
			legendItem("L", "CLEAR"), divider,
			legendItem("A", m.asciiLegendLabel()), divider,
			legendItem("Q", "QUIT"), divider,
			legendItem("ESC", "CANCEL"),
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
			legendItem("^A", "PREFIX"), divider,
			legendItem("^N", "NEW CHAT"), divider,
			legendItem("^C", "QUIT"),
		)
	}
}

func (m *model) renderModelPopup() string {
	w, h := m.width, m.height

	listW := clamp(30, 54, w-12)
	listH := clamp(8, 16, h-8)

	m.modelList.SetSize(listW, listH)

	modal := modalFrameStyle.Render(m.modelList.View())

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

func clamp(minV, maxV, v int) int {
	if v < minV {
		return minV
	}

	if v > maxV {
		return maxV
	}

	return v
}

func truncate(style lipgloss.Style, s string, maxl int) string {
	if maxl > 0 && len(s) > maxl {
		if maxl <= 1 {
			return style.Render("...")
		}

		s = s[:maxl-1] + "..."
	}

	return style.Render(s)
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

func (m *model) reasoningLegendLabel() string {
	if m.reasoningShow {
		return "HIDE REASONING"
	}

	return "SHOW REASONING"
}

func (m *model) asciiLegendLabel() string {
	if m.reasoningShow {
		return "HIDE ASCII"
	}

	return "SHOW ASCII"
}
