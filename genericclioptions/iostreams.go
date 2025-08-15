package genericclioptions

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
)

type IOStreams struct {
	In     FdReader
	Out    io.Writer
	ErrOut io.Writer

	level slog.Level
}

// NewDefaultIOStreams returns the default IOStreams (using os.Stdin, os.Stdout, os.Stderr).
func NewDefaultIOStreams() *IOStreams {
	return &IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
	}
}

// NewTestIOStreamsWithMockInput returns IOStreams with mock input,
// a [TestFdReader] and out and error buffers for unit tests.
//
//nolint:revive
func NewTestIOStreams(r *TestFdReader) (iostream *IOStreams, in *TestFdReader, out *bytes.Buffer, errOut *bytes.Buffer) {
	in = r
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}

	iostream = &IOStreams{
		In:     in,
		Out:    out,
		ErrOut: errOut,
	}

	return
}

// NewTestIOStreamsDiscard returns IOStreams with mocked input,
// and discards both output and error output.
func NewTestIOStreamsDiscard(in *TestFdReader) *IOStreams {
	return &IOStreams{
		In:     in,
		Out:    io.Discard,
		ErrOut: io.Discard,
	}
}

func (io *IOStreams) SetLevel(l slog.Level) {
	io.level = l
}

// Print writes a general, unprefixed message to the standard output stream.
func (io *IOStreams) Print(s string) {
	fmt.Fprint(io.Out, s)
}

// Printf writes a general, unprefixed formatted message to the standard output stream.
func (io *IOStreams) Printf(format string, args ...any) {
	fmt.Fprintf(io.Out, format, args...)
}

// Debugf writes formatted debug output to the error stream.
// if Verbose is enabled.
func (io *IOStreams) Debugf(format string, args ...any) {
	if io.level <= slog.LevelDebug {
		fmt.Fprintf(io.ErrOut, "DEBUG "+format, args...)
	}
}

// Infof writes a formatted message to the standard output stream.
func (io *IOStreams) Infof(format string, args ...any) {
	if io.level <= slog.LevelInfo {
		fmt.Fprintf(io.Out, "INFO "+format, args...)
	}
}

// Warnf writes a formatted message to the standard output stream.
func (io *IOStreams) Warnf(format string, args ...any) {
	if io.level <= slog.LevelWarn {
		fmt.Fprintf(io.Out, "WARN "+format, args...)
	}
}

// Errorf writes a formatted message to the error stream.
func (io *IOStreams) Errorf(format string, args ...any) {
	if io.level <= slog.LevelError {
		fmt.Fprintf(io.ErrOut, "WARN "+format, args...)
	}
}
