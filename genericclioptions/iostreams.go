package genericclioptions

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
)

type IOStreams struct {
	in     FdReader
	out    io.Writer
	errOut io.Writer

	level slog.Level
}

// NewDefaultIOStreams returns the default IOStreams (using os.Stdin, os.Stdout, os.Stderr).
func NewDefaultIOStreams() *IOStreams {
	return &IOStreams{
		in:     os.Stdin,
		out:    os.Stdout,
		errOut: os.Stderr,
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
		in:     in,
		out:    out,
		errOut: errOut,
	}

	return
}

// NewTestIOStreamsDiscard returns IOStreams with mocked input,
// and discards both output and error output.
func NewTestIOStreamsDiscard(in *TestFdReader) *IOStreams {
	return &IOStreams{
		in:     in,
		out:    io.Discard,
		errOut: io.Discard,
	}
}

func (io *IOStreams) SetLevel(l slog.Level) {
	io.level = l
}

// Print writes a general, unprefixed message to the standard output stream.
func (io *IOStreams) Print(s string) {
	fmt.Fprint(io.out, s)
}

// Printf writes a general, unprefixed formatted message to the standard output stream.
func (io *IOStreams) Printf(format string, args ...any) {
	fmt.Fprintf(io.out, format, args...)
}

// Debugf writes formatted debug output to the error stream.
// if Verbose is enabled.
func (io *IOStreams) Debugf(format string, args ...any) {
	if io.level <= slog.LevelDebug {
		fmt.Fprintf(io.errOut, "DEBUG "+format, args...)
	}
}

// Infof writes a formatted message to the standard output stream.
func (io *IOStreams) Infof(format string, args ...any) {
	if io.level <= slog.LevelInfo {
		fmt.Fprintf(io.out, "INFO "+format, args...)
	}
}

// Warnf writes a formatted message to the standard output stream.
func (io *IOStreams) Warnf(format string, args ...any) {
	if io.level <= slog.LevelWarn {
		fmt.Fprintf(io.out, "WARN "+format, args...)
	}
}

// Errorf writes a formatted message to the error stream.
func (io *IOStreams) Errorf(format string, args ...any) {
	if io.level <= slog.LevelError {
		fmt.Fprintf(io.errOut, "WARN "+format, args...)
	}
}
