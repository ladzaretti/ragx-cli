package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
	"github.com/ladzaretti/ragrat/llm"

	"github.com/spf13/cobra"
)

var Version = "0.0.0"

const (
	appName                  = "ragrat"
	envConfigPathKeyOverride = "RAGRAT_CONFIG_PATH"
	defaultBaseURL           = "http://localhost:11434/v1"
	defaultConfigName        = ".ragrat.toml"
	defaultLogFilename       = ".log"
	defaultLogLevel          = "info"
	defaultTemperature       = 0.7
	defaultChunkSize         = 500
	defaultTopK              = 4
)

// preRunPartialCommands are commands that require partial pre-run execution
// without client creation.
var preRunPartialCommands = []string{"config", "generate", "validate"}

type cleanupFunc func() error

type llmOptions struct {
	client        *llm.Client
	session       *llm.ChatSession
	models        []string
	selectedModel string
}

var _ genericclioptions.BaseOptions = &llmOptions{}

func (*llmOptions) Complete() error { return nil }

func (*llmOptions) Validate() error { return nil }

// DefaultRAGOptions is the base cli config shared across all ragrat subcommands.
type DefaultRAGOptions struct {
	*genericclioptions.StdioOptions

	configOptions *ConfigOptions
	llmOptions    *llmOptions

	cleanupFuncs []cleanupFunc
}

var _ genericclioptions.CmdOptions = &DefaultRAGOptions{}

// NewDefaultRAGOptions initializes the options struct.
func NewDefaultRAGOptions(iostreams *genericclioptions.IOStreams) *DefaultRAGOptions {
	stdio := &genericclioptions.StdioOptions{IOStreams: iostreams}

	return &DefaultRAGOptions{
		StdioOptions:  stdio,
		configOptions: NewConfigOptions(stdio),
		llmOptions:    &llmOptions{},
	}
}

func (o *DefaultRAGOptions) Complete() error {
	if err := o.StdioOptions.Complete(); err != nil {
		return err
	}

	if err := o.configOptions.Complete(); err != nil {
		return err
	}

	return o.llmOptions.Complete()
}

func (o *DefaultRAGOptions) Validate() error {
	if err := o.StdioOptions.Validate(); err != nil {
		return err
	}

	return o.configOptions.Validate()
}

func (o *DefaultRAGOptions) Run(ctx context.Context, args ...string) error {
	cmd := ""
	if len(args) == 1 {
		cmd = args[0]
	}

	if slices.Contains(preRunPartialCommands, cmd) {
		return nil
	}

	logger, err := o.initLogger()
	if err != nil {
		return err
	}

	o.Opts(genericclioptions.WithLogger(logger))

	if err := o.initLLM(logger); err != nil {
		return err
	}

	m, err := o.llmOptions.client.ListModels(ctx)
	if err != nil {
		return errf("llm list models: %v", err)
	}

	o.llmOptions.models = m

	return nil
}

func (o *DefaultRAGOptions) initLogger() (*slog.Logger, error) {
	dir := o.configOptions.resolved.Logging.Dir
	name := o.configOptions.resolved.Logging.Filename

	f, err := openLogFile(dir, name)
	if err != nil {
		return nil, errf("open log file: %v", err)
	}

	o.cleanupFuncs = append(o.cleanupFuncs, func() error { return f.Close() })

	level, _ := genericclioptions.ParseLevel(o.configOptions.resolved.Logging.Level)
	o.SetLevel(level)

	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level}))

	return logger, nil
}

func (o *DefaultRAGOptions) initLLM(logger *slog.Logger) error {
	opts := []llm.Option{
		llm.WithBaseURL(o.configOptions.resolved.LLM.BaseURL),
		llm.WithLogger(logger),
	}

	client, err := llm.NewClient(opts...)
	if err != nil {
		return errf("new client: %v", err)
	}

	model := o.configOptions.resolved.LLM.Model
	system := o.configOptions.fileConfig.Prompt.System

	session, err := llm.NewChat(client, system, model, llm.WithSessionLogger(logger))
	if err != nil {
		return errf("new chat session: %v", err)
	}

	o.llmOptions.client = client
	o.llmOptions.session = session
	o.llmOptions.selectedModel = model

	return nil
}

// NewDefaultRAGCommand creates the root cobra command.
func NewDefaultRAGCommand(iostreams *genericclioptions.IOStreams, args []string) *cobra.Command {
	o := NewDefaultRAGOptions(iostreams)

	cmd := &cobra.Command{
		Use:   "ragrat",
		Short: "",
		Long: `ragrat is a terminal based, self-hosted Retrieval-Augmented Generation (RAG) assistant.

It supports local and remote LLMs via OpenAI-compatible APIs.
Configuration is handled via flags or config files.`,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o, cmd.Name()))
		},
		PersistentPostRunE: func(_ *cobra.Command, _ []string) error {
			return clierror.Check(executeCleanup(o.cleanupFuncs))
		},
	}

	cmd.SetArgs(args)

	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.baseURL, "base-url", "u", "", "Override LLM base URL")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.model, "model", "m", "", "Override LLM model")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.configPath, "config", "c", "", "Path to config file")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.embeddingModel, "embedding-model", "e", "", "Override embedding model")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logDir, "log-dir", "d", "", "Override log directory")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logFilename, "log-file", "f", "", "Override log filename")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logLevel, "log-level", "l", "", "Set log level (debug, info, warn, error)")

	cmd.AddCommand(NewCmdTUI(o))
	cmd.AddCommand(NewCmdConfig(o))

	return cmd
}

// executeCleanup executes cleanup functions in reverse order,
// similar to defer statements.
//
// used functions are nilled out.
func executeCleanup(fs []cleanupFunc) error {
	var errs []error
	for i := len(fs) - 1; i >= 0; i-- {
		f := fs[i]
		if f == nil {
			continue
		}

		fs[i] = nil

		errs = append(errs, f())
	}

	return errors.Join(errs...)
}

func errf(format string, a ...any) error {
	return fmt.Errorf(format, a...)
}
