package cli

import (
	"context"
	"errors"
	"io"

	"github.com/ladzaretti/ragx/chatui"
	"github.com/ladzaretti/ragx/clierror"
	"github.com/ladzaretti/ragx/genericclioptions"

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
		config = chatui.LLMConfig{
			Models:             o.llmConfig.Models,
			DefaultModel:       o.llmConfig.DefaultModel,
			UserPromptTmpl:     o.promptConfig.UserPromptTmpl,
			EmbeddingModel:     o.embeddingConfig.Model,
			RetrievalTopK:      o.embeddingConfig.TopK,
			DefaultTemperature: o.defaultTemperature,
			DefaultContext:     o.defaultContext,
		}
		tui = chatui.New(o.providers, o.vectordb, config)
		p   = tea.NewProgram(tui,
			tea.WithAltScreen(),
			tea.WithReportFocus(),
		)
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
		Use:     "chat [flags] [path]...",
		Aliases: []string{"tui"},
		Short:   "Start the interactive terminal chat UI",
		Long: `Embed content from one or more paths (files or directories) or from stdin, 
then launches an interactive TUI for chatting with the LLM. Directories are walked recursively.

When paths are provided, files are included if they match any -M/--match regex (full path).
If no -M filter is given, all files under the provided paths are embedded.`,
		Example: `  # embed all .go files in current dir and start the TUI
  ragx chat . -M '\.go$'

  # embed multiple paths (markdown and txt) and start the TUI
  ragx chat ./docs ./src -M '(?i)\.(md|txt)$'

  # embed stdin and start the TUI
  cat readme.md | ragx chat`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o, args...))
		},
	}

	return cmd
}
