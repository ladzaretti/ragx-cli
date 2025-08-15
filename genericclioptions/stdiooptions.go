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

	Logger *slog.Logger
	Piped  bool

	level slog.Level
}

var _ BaseOptions = &StdioOptions{}

type StdioOption func(*StdioOptions)

func WithIn(r FdReader) StdioOption {
	return func(o *StdioOptions) {
		if r != nil {
			o.In = r
		}
	}
}

func WithOut(w io.Writer) StdioOption {
	return func(o *StdioOptions) {
		if w != nil {
			o.Out = w
		}
	}
}

func WithErr(w io.Writer) StdioOption {
	return func(o *StdioOptions) {
		if w != nil {
			o.ErrOut = w
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
	if !o.Piped {
		fi, err := o.In.Stat()
		if err != nil {
			return fmt.Errorf("stat input: %v", err)
		}

		if !isatty(fi) {
			o.Piped = true
		}
	}

	return nil
}

// Validate ensures the input mode (Stdin or interactive) is used appropriately.
func (o *StdioOptions) Validate() error {
	fi, err := o.In.Stat()
	if err != nil {
		return fmt.Errorf("stat input: %v", err)
	}

	if o.Piped && isatty(fi) {
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
