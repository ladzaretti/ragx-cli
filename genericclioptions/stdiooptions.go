package genericclioptions

import (
	"errors"
	"fmt"
	"os"

	"github.com/ladzaretti/ragrat/clierror"
)

// StdioOptions provides stdin-related CLI helpers,
// intended to be embedded in option structs.
type StdioOptions struct {
	StdinIsPiped bool

	*IOStreams
}

var _ BaseOptions = &StdioOptions{}

// Complete sets default values, e.g., enabling Stdin if piped input is detected.
func (o *StdioOptions) Complete() error {
	if !o.StdinIsPiped {
		fi, err := o.In.Stat()
		if err != nil {
			return fmt.Errorf("stat input: %v", err)
		}

		if isPipedOrRedirected(fi) {
			o.Debugf("input is piped or redirected; Enabling non-interactive mode.\n")
			o.StdinIsPiped = true
		}
	}

	clierror.DebugMode(o.Verbose)

	return nil
}

// Validate ensures the input mode (Stdin or interactive) is used appropriately.
func (o *StdioOptions) Validate() error {
	fi, err := o.In.Stat()
	if err != nil {
		return fmt.Errorf("stat input: %v", err)
	}

	if o.StdinIsPiped && !isPipedOrRedirected(fi) {
		return errors.New("non-interactive mode requires piped or redirected input")
	}

	return nil
}

func isPipedOrRedirected(fi os.FileInfo) bool {
	return (fi.Mode() & os.ModeCharDevice) == 0
}
