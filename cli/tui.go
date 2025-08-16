package cli

import (
	"context"
	"errors"

	"github.com/ladzaretti/ragrat/chatui"
	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var ErrInvalidModel = errors.New("invalid model selected")

type ChatOptions struct {
	*genericclioptions.StdioOptions
	*llmOptions

	config *configOptions
}

var _ genericclioptions.CmdOptions = &ChatOptions{}

// NewChatOptions initializes the options struct.
func NewChatOptions(stdio *genericclioptions.StdioOptions, llm *llmOptions, config *configOptions) *ChatOptions {
	return &ChatOptions{
		StdioOptions: stdio,
		llmOptions:   llm,
		config:       config,
	}
}

func (*ChatOptions) Complete() error { return nil }

func (*ChatOptions) Validate() error { return nil }

func (o *ChatOptions) Run(_ context.Context, _ ...string) error {
	var (
		tui = chatui.New(
			o.client,
			o.session,
			o.vectordb,
			o.config.resolved.Embedding.TopK,
			o.models,
			o.config.resolved.LLM.Model,
			o.config.resolved.Embedding.EmbeddingModel,
		)
		p = tea.NewProgram(tui, tea.WithAltScreen())
	)

	if _, err := p.Run(); err != nil {
		return errf("chatui: %v\n", err)
	}

	return nil
}

// NewCmdChat creates the <cmd> cobra command.
func NewCmdChat(defaults *DefaultRAGOptions) *cobra.Command {
	o := NewChatOptions(
		defaults.StdioOptions,
		defaults.llmOptions,
		defaults.configOptions,
	)

	cmd := &cobra.Command{
		Use:     "chat",
		Aliases: []string{"tui"},
		Short:   "Start the interactive terminal chat UI",
		Long:    "Launch a terminal interface for chatting with a local or remote LLM.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o))
		},
	}

	return cmd
}
