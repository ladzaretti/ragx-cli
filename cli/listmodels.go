package cli

import (
	"context"

	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
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
	for _, m := range o.availableModels {
		o.Print(m + "\n")
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
		Long:  "",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o))
		},
	}

	return cmd
}
