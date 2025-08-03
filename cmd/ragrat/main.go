package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ladzaretti/ragrat/llm"
	"github.com/ladzaretti/ragrat/model"
)

const appName = "ragrat"

func defaultLogDir() (string, error) {
	if stateDir, ok := os.LookupEnv("XDG_STATE_HOME"); ok {
		return filepath.Join(stateDir, appName), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, ".local", "state", appName), nil
}

func defaultLogFile() (*os.File, error) {
	logDir, err := defaultLogDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(logDir, 0o750); err != nil {
		return nil, err
	}

	var (
		filename = filepath.Join(logDir, ".log")
		flag     = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	)

	return os.OpenFile(filepath.Clean(filename), flag, 0o600) //nolint:gosec // internal filename
}

func main() {
	f, err := defaultLogFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "open ragrat log: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(f, nil))

	client, err := llm.NewClient(
		llm.WithBaseURL("http://localhost:11434/v1"),
		llm.WithLogger(logger),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "llm client: %v\n", err)
		os.Exit(1)
	}

	models, err := client.ListModels(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "llm list models: %v\n", err)
		os.Exit(1)
	}

	session, err := llm.NewChat(client, "", models[0], llm.WithSessionLogger(logger))
	if err != nil {
		fmt.Fprintf(os.Stderr, "llm session: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(model.New(session, models), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "ragrat-tui: %v\n", err)
		os.Exit(1)
	}
}
