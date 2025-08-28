package genericclioptions

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// MarkAllFlagsHidden hides all flags from the target's help output.
func MarkAllFlagsHidden(target *cobra.Command, excluded ...string) {
	f := target.HelpFunc()

	target.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if slices.Contains(excluded, f.Name) {
				return
			}

			f.Hidden = true
		})

		f(cmd, args)
	})
}

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

// ContainsAny reports whether str contains any of substrings.
func ContainsAny(str string, substrings ...string) bool {
	for _, substr := range substrings {
		if strings.Contains(str, substr) {
			return true
		}
	}

	return false
}

// RemoveLinesContaining removes any lines that contain any of substrings.
func RemoveLinesContaining(s string, substrings ...string) string {
	lines := strings.Split(s, "\n")
	out := lines[:0]

	for _, l := range lines {
		if ContainsAny(l, substrings...) {
			continue
		}

		out = append(out, l)
	}

	return strings.Join(out, "\n")
}
