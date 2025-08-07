package cli

import (
	"context"
	"errors"
	"slices"

	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
	"github.com/ladzaretti/ragrat/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var ErrInvalidModel = errors.New("invalid model selected")

type TUIOptions struct {
	*genericclioptions.StdioOptions
	*llmOptions
}

var _ genericclioptions.CmdOptions = &TUIOptions{}

// NewTUIOptions initializes the options struct.
func NewTUIOptions(stdio *genericclioptions.StdioOptions, llm *llmOptions) *TUIOptions {
	return &TUIOptions{
		StdioOptions: stdio,
		llmOptions:   llm,
	}
}

func (*TUIOptions) Complete() error { return nil }

func (o *TUIOptions) Validate() error {
	if !slices.Contains(o.models, o.selectedModel) {
		return errf("%w: %q", ErrInvalidModel, o.selectedModel)
	}

	return nil
}

func (o *TUIOptions) Run(_ context.Context, _ ...string) error {
	p := tea.NewProgram(model.New(o.session, o.models, o.selectedModel), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return errf("tui: %v\n", err)
	}

	return nil
}

// NewCmdTUI creates the <cmd> cobra command.
func NewCmdTUI(defaults *DefaultRAGOptions) *cobra.Command {
	o := NewTUIOptions(defaults.StdioOptions, defaults.llmOptions)

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Start the interactive terminal chat UI",
		Long:  "Launch a terminal interface for chatting with a local or remote LLM.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o))
		},
	}

	return cmd
}
