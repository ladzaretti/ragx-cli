package cli

import (
	"context"
	"strings"

	"github.com/ladzaretti/ragx/clierror"
	"github.com/ladzaretti/ragx/genericclioptions"
	"github.com/spf13/cobra"
)

type ListModelsOptions struct {
	*genericclioptions.StdioOptions
	*llmOptions
}

var _ genericclioptions.CmdOptions = &ListModelsOptions{}

// NewListModelsOptions initializes the options struct.
func NewListModelsOptions(stdio *genericclioptions.StdioOptions, llmOptions *llmOptions) *ListModelsOptions {
	return &ListModelsOptions{
		StdioOptions: stdio,
		llmOptions:   llmOptions,
	}
}

func (*ListModelsOptions) Complete() error { return nil }

func (*ListModelsOptions) Validate() error { return nil }

func (o *ListModelsOptions) Run(_ context.Context, _ ...string) error {
	for i, p := range o.providers {
		baseURL, models := o.llmConfig.Providers[i].BaseURL, p.AvailableModels

		if i != 0 {
			o.Print("\n") // space out providers
		}

		out := strings.Join(
			append([]string{baseURL}, models...),
			"\n\t",
		)

		o.Print(out + "\n")
	}

	return nil
}

// NewCmdListModels creates the ListModels cobra command.
func NewCmdListModels(defaults *DefaultRAGOptions) *cobra.Command {
	o := NewListModelsOptions(
		defaults.StdioOptions,
		defaults.llmOptions,
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available models",
		Long:  "List all models available to ragx from the configured LLM backend.",
		Example: `# list models from the active backend
  ragx list

  # list models using a specific config file
  ragx list --config ~/.ragx.toml`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o))
		},
	}

	hiddenFlags := []string{
		"dim",
		"embedding-model",
		"match",
		"model",
		"temp",
		"context",
	}

	genericclioptions.MarkFlagsHidden(cmd, hiddenFlags...)

	return cmd
}
