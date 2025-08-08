package clierror

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	DefaultErrorExitCode = 1
)

var (
	// errHandler is the function used to handle cli errors.
	errHandler = FatalErrHandler

	// errWriter is used to output cli error messages.
	errWriter io.Writer = os.Stderr

	// fprintf is the function used to format and print errors.
	fprintf = fmt.Fprintf

	// name is the name of the root cli command in use.
	name string
)

// SetErrorHandler overrides the default [FatalErrHandler] error handler.
func SetErrorHandler(f func(string, int)) {
	errHandler = f
}

// ResetErrorHandler restores the default error handler.
func ResetErrorHandler() {
	errHandler = FatalErrHandler
}

// SetErrWriter overrides the default error output writer [os.Stderr].
func SetErrWriter(w io.Writer) {
	errWriter = w
}

// ResetErrWriter restores the default error output writer to [os.Stderr].
func ResetErrWriter() {
	errWriter = os.Stderr
}

// SetDefaultFprintf sets the default function used to print errors.
func SetDefaultFprintf(f func(w io.Writer, format string, a ...any) (n int, err error)) {
	fprintf = f
}

// SetName for the cli in use.
func SetName(s string) {
	name = s
}

// FatalErrHandler prints the message provided and then exits with the given code.
func FatalErrHandler(msg string, code int) {
	printError(msg)

	//nolint:revive // Intentional exit after fatal error.
	os.Exit(code)
}

func PrintErrHandler(msg string, _ int) {
	printError(msg)
}

func printError(msg string) {
	if len(msg) == 0 {
		return
	}

	// add newline if needed
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}

	fprintf(errWriter, msg) //nolint:errcheck
}

// ErrExit may be passed to CheckError to instruct it to output nothing but exit with
// status code 1.
var ErrExit = errors.New("exit")

// Check prints a user-friendly error message and invokes the configured error handler.
//
// When the [FatalErrHandler] is used, the program will exit before this function returns.
func Check(err error) error {
	check(err, errHandler)
	return err
}

//nolint:revive
func check(err error, handleErr func(string, int)) {
	if err == nil {
		return
	}

	switch {
	case errors.Is(err, ErrExit):
		handleErr("", DefaultErrorExitCode)
	default:
		msg, ok := StandardErrorMessage(err)
		if !ok {
			msg = err.Error()
			if !strings.HasPrefix(msg, name+": ") {
				msg = name + ": " + msg
			}
		}

		handleErr(msg, DefaultErrorExitCode)
	}
}

func StandardErrorMessage(_ error) (string, bool) {
	return "", false
}
