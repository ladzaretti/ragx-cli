package genericclioptions

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// MarkFlagsHidden hides the given flags from the target's help output.
func MarkFlagsHidden(target *cobra.Command, hidden ...string) {
	f := target.HelpFunc()

	target.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == target {
			for _, n := range hidden {
				flag := cmd.Flags().Lookup(n)
				if flag != nil {
					flag.Hidden = true
				}
			}
		}

		f(cmd, args)
	})
}

func RejectDisallowedFlags(cmd *cobra.Command, disallowed ...string) error {
	for _, name := range disallowed {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("flag --%s is not allowed with '%s' command", name, cmd.Name())
		}
	}

	return nil
}

func StringContains(str string, substrings ...string) bool {
	for _, substr := range substrings {
		if strings.Contains(str, substr) {
			return true
		}
	}

	return false
}
