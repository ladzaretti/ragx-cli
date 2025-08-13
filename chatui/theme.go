package chatui

import (
	"github.com/charmbracelet/lipgloss"
)

const ascii = `          ▗ 
▛▘▀▌▛▌▛▘▀▌▜▘
▌ █▌▙▌▌ █▌▐▖
    ▄▌      
`

var asciiComponentView = lipgloss.NewStyle().
	Foreground(lipgloss.Color(mochaLavender)). // or mochaBlue
	Bold(true).
	PaddingLeft(1).
	Render(ascii)

// catppuccin Mocha palette (hex codes).
const (
	mochaCrust    = "#11111b"
	mochaMantle   = "#181825"
	mochaSurface0 = "#313244"

	mochaText     = "#cdd6f4"
	mochaSubtext0 = "#a6adc8"
	mochaOverlay2 = "#9399b2"

	mochaRed      = "#f38ba8"
	mochaGreen    = "#a6e3a1"
	mochaYellow   = "#f9e2af"
	mochaBlue     = "#89b4fa"
	mochaMauve    = "#cba6f7"
	mochaLavender = "#b4befe"
	mochaPeach    = "#fab387"
	mochaTeal     = "#94e2d5"
)

var (
	keyStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaSubtext0)).Bold(true)
	dimStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaOverlay2))
	spinnerCol      = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaMauve))
	userPrefixStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaBlue)).Bold(true)
	llmPrefixStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaMauve)).Bold(true)
	reasoningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaSubtext0)).Italic(true)

	itemStyle         = lipgloss.NewStyle().Padding(0, 1)
	selectedItemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(mochaLavender)).Bold(true).Padding(0, 1)

	insertStatusStyle             = lipgloss.NewStyle().Background(lipgloss.Color(mochaMauve)).Foreground(lipgloss.Color(mochaCrust)).Bold(true).Padding(0, 1)
	leaderStatusStyle             = lipgloss.NewStyle().Background(lipgloss.Color(mochaLavender)).Foreground(lipgloss.Color(mochaCrust)).Bold(true).Padding(0, 1)
	historyStatusStyle            = lipgloss.NewStyle().Background(lipgloss.Color(mochaBlue)).Foreground(lipgloss.Color(mochaCrust)).Bold(true).Padding(0, 1)
	modelsStatusStyle             = lipgloss.NewStyle().Background(lipgloss.Color(mochaYellow)).Foreground(lipgloss.Color(mochaCrust)).Bold(true).Padding(0, 1)
	defaultStatusStyle            = lipgloss.NewStyle().Background(lipgloss.Color(mochaSurface0)).Foreground(lipgloss.Color(mochaText)).Bold(true).Padding(0, 1)
	errorStatusStyle              = lipgloss.NewStyle().Background(lipgloss.Color(mochaRed)).Foreground(lipgloss.Color(mochaCrust)).Bold(true).Padding(0, 1)
	selectedModelStatusStyle      = lipgloss.NewStyle().Background(lipgloss.Color(mochaPeach)).Foreground(lipgloss.Color(mochaCrust)).Bold(true).Padding(0, 1)
	embedSelectedModelStatusStyle = lipgloss.NewStyle().Background(lipgloss.Color(mochaTeal)).Foreground(lipgloss.Color(mochaCrust)).Bold(true).Padding(0, 1)

	barStyle = lipgloss.NewStyle().
			Background(lipgloss.Color(mochaMantle)).
			Foreground(lipgloss.Color(mochaText)).
			BorderTop(true).
			BorderForeground(lipgloss.Color(mochaSurface0))
)
