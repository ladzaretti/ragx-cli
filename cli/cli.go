package cli

import (
	"context"

	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
	"github.com/spf13/cobra"
)

type DefaultRAGOptions struct {
	*genericclioptions.StdioOptions

	configOptions *ConfigOptions
}

var _ genericclioptions.CmdOptions = &DefaultRAGOptions{}

// NewDefaultRAGOptions initializes the options struct.
func NewDefaultRAGOptions(iostreams *genericclioptions.IOStreams) *DefaultRAGOptions {
	stdio := &genericclioptions.StdioOptions{IOStreams: iostreams}

	return &DefaultRAGOptions{
		StdioOptions:  stdio,
		configOptions: NewConfigOptions(stdio),
	}
}

func (o *DefaultRAGOptions) Complete() error {
	if err := o.StdioOptions.Complete(); err != nil {
		return err
	}

	if err := o.configOptions.Complete(); err != nil {
		return err
	}

	return o.complete()
}

func (o *DefaultRAGOptions) complete() error { return nil }

func (o *DefaultRAGOptions) Validate() error {
	return nil
}

func (o *DefaultRAGOptions) Run(ctx context.Context, args ...string) error {
	return nil
}

// NewDefaultRAGCommand creates the <cmd> cobra command.
func NewDefaultRAGCommand(iostreams *genericclioptions.IOStreams, args []string) *cobra.Command {
	o := NewDefaultRAGOptions(iostreams)

	cmd := &cobra.Command{
		Use:   "ragrat",
		Short: "",
		Long:  "",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o))
		},
	}

	cmd.SetArgs(args)

	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.baseURL, "base-url", "u", "", "Override LLM base URL")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.model, "model", "m", "", "Override LLM model")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logDir, "log-dir", "d", "", "Override log directory")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.configPath, "config", "c", "", "Path to config file")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logFilename, "log-file", "f", "", "Override log filename")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.embeddingModel, "embedding-model", "e", "", "Override embedding model")

	cmd.AddCommand(NewCmdTUI(o))
	cmd.AddCommand(NewCmdConfig(o))

	return cmd
}
