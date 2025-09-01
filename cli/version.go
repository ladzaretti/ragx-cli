package cli

import (
	"github.com/ladzaretti/ragx/genericclioptions"
	"github.com/spf13/cobra"
)

func newVersionCommand(defaults *DefaultRAGOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "version",
		Short:         "Show version",
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			defaults.Printf("%s\n", Version)
			return nil
		},
	}

	genericclioptions.MarkAllFlagsHidden(cmd, "help")

	return cmd
}
