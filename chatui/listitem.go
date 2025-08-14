package chatui

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// listItem wraps a model name so it can satisfy [list.Item].
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

	fmt.Fprint(w, style.Render(prefix+name))
}
