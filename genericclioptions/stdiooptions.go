package genericclioptions

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// StdioOptions provides stdin-related CLI helpers,
// intended to be embedded in option structs.
type StdioOptions struct {
	*IOStreams

	nonInteractive bool
	level          slog.Level

	Logger *slog.Logger
}

var _ BaseOptions = &StdioOptions{}

type StdioOption func(*StdioOptions)

func WithIn(r FdReader) StdioOption {
	return func(o *StdioOptions) {
		if r != nil {
			o.in = r
		}
	}
}

func WithOut(w io.Writer) StdioOption {
	return func(o *StdioOptions) {
		if w != nil {
			o.out = w
		}
	}
}

func WithErr(w io.Writer) StdioOption {
	return func(o *StdioOptions) {
		if w != nil {
			o.errOut = w
		}
	}
}

func WithLogger(logger *slog.Logger) StdioOption {
	return func(o *StdioOptions) {
		if logger != nil {
			o.Logger = logger
		}
	}
}

func WithLevel(l slog.Level) StdioOption {
	return func(o *StdioOptions) {
		o.SetLevel(l)
		o.level = l
	}
}

func (o *StdioOptions) Opts(opts ...StdioOption) {
	for _, opt := range opts {
		opt(o)
	}
}

// NewStdioOptions creates a new StdioOptions with default streams and logger.
func NewStdioOptions() *StdioOptions {
	return &StdioOptions{
		IOStreams: NewDefaultIOStreams(),
		Logger:    slog.Default(),
	}
}

// Complete sets default values, e.g., enabling Stdin if piped input is detected.
func (o *StdioOptions) Complete() error {
	if !o.nonInteractive {
		fi, err := o.in.Stat()
		if err != nil {
			return fmt.Errorf("stat input: %v", err)
		}

		if !isatty(fi) {
			o.Logger.Debug("input is piped or redirected; Enabling non-interactive mode.\n")
			o.nonInteractive = true
		}
	}

	return nil
}

// Validate ensures the input mode (Stdin or interactive) is used appropriately.
func (o *StdioOptions) Validate() error {
	fi, err := o.in.Stat()
	if err != nil {
		return fmt.Errorf("stat input: %v", err)
	}

	if o.nonInteractive && isatty(fi) {
		return errors.New("non-interactive mode requires piped or redirected input")
	}

	return nil
}

func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return slog.LevelDebug, nil
	case "INFO":
		return slog.LevelInfo, nil
	case "WARN":
		return slog.LevelWarn, nil
	case "ERROR":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level: %q", s)
	}
}

func isatty(fi os.FileInfo) bool {
	return (fi.Mode() & os.ModeCharDevice) != 0
}
