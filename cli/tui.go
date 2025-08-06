package cli

import (
	"context"

	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
	"github.com/spf13/cobra"
)

type TUIOptions struct {
	*genericclioptions.StdioOptions

	opt any
}

var _ genericclioptions.CmdOptions = &TUIOptions{}

// NewTUIOptions initializes the options struct.
func NewTUIOptions(stdio *genericclioptions.StdioOptions) *TUIOptions {
	return &TUIOptions{
		StdioOptions: stdio,
	}
}

func (o *TUIOptions) Complete() error {
	return nil
}

func (o *TUIOptions) Validate() error {
	return nil
}

func (o *TUIOptions) Run(ctx context.Context, args ...string) error {
	return nil
}

// NewCmdTUI creates the <cmd> cobra command.
func NewCmdTUI(defaults *DefaultRAGOptions) *cobra.Command {
	o := NewTUIOptions(defaults.StdioOptions)

	cmd := &cobra.Command{
		Use:   "tui",
		Short: "",
		Long:  "",
		Run: func(cmd *cobra.Command, args []string) {
			clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o))
		},
	}

	return cmd
}
