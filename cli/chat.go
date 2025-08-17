package cli

import (
	"context"
	"errors"
	"io"

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
}

var _ genericclioptions.CmdOptions = &ChatOptions{}

// NewChatOptions initializes the options struct.
func NewChatOptions(stdio *genericclioptions.StdioOptions, llmOptions *llmOptions) *ChatOptions {
	return &ChatOptions{
		StdioOptions: stdio,
		llmOptions:   llmOptions,
	}
}

func (*ChatOptions) Complete() error { return nil }

func (*ChatOptions) Validate() error { return nil }

func (o *ChatOptions) Run(ctx context.Context, args ...string) error {
	if !o.Piped && len(args) == 0 {
		return ErrNoEmbedInput
	}

	if o.Piped && len(args) > 0 {
		return ErrConflictingEmbedInputs
	}

	var in io.Reader

	if o.Piped {
		in = o.In
	}

	err := o.embed(ctx, o.Logger, in, o.embeddingREs, args...)
	if err != nil {
		return errf("embed: %w", err)
	}

	var (
		tui = chatui.New(
			o.client,
			o.session,
			o.vectordb,
			o.embeddingConfig.TopK,
			o.availableModels,
			o.chatConfig.Model,
			o.embeddingConfig.EmbeddingModel,
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
	)

	cmd := &cobra.Command{
		Use:     "chat",
		Aliases: []string{"tui"},
		Short:   "Start the interactive terminal chat UI",
		Long:    "Launch a terminal interface for chatting with a local or remote LLM.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o, args...))
		},
	}

	return cmd
}
