package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
	"github.com/ladzaretti/ragrat/llm"
	"github.com/ladzaretti/ragrat/vecdb"

	"github.com/spf13/cobra"
)

var Version = "0.0.0"

var (
	ErrMissingQuery           = errors.New("missing required --query flag")
	ErrMissingLLMModel        = errors.New("missing LLM model")
	ErrMissingEmbeddingModel  = errors.New("missing embedding model")
	ErrMissingDimension       = errors.New("missing or invalid embedding dimension")
	ErrInvalidSelectedModel   = errors.New("selected model not found in available models")
	ErrNoEmbedInput           = errors.New("no input provided for embedding")
	ErrConflictingEmbedInputs = errors.New("cannot embed from both piped input and file arguments")
)

const (
	embedConcurrency = 8
	embedBatchSize   = 64
)

const (
	appName                  = "ragrat"
	envConfigPathKeyOverride = "RAGRAT_CONFIG_PATH"
	defaultBaseURL           = "http://localhost:11434/v1"
	defaultConfigName        = ".ragrat.toml"
	defaultLogFilename       = ".log"
	defaultLogLevel          = "info"
	defaultChunkSize         = 2000
	defaultOverlap           = 200
	defaultTopK              = 20
)

const (
	reasoningStartTag = "<think>"
	reasoningEndTag   = "</think>"
)

type cleanupFunc func() error

type step func(ctx context.Context, args ...string) error

// DefaultRAGOptions is the base cli config shared across all ragrat subcommands.
type DefaultRAGOptions struct {
	*genericclioptions.StdioOptions

	configOptions *configOptions
	llmOptions    *llmOptions

	cleanupFuncs  []cleanupFunc
	matchPatterns []string

	steps []step
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

	if err := o.llmOptions.Complete(); err != nil {
		return err
	}

	return o.complete()
}

func (o *DefaultRAGOptions) complete() error { //nolint:revive
	matchREs, err := compileREs(o.matchPatterns...)
	if err != nil {
		return err
	}

	o.llmOptions.chatConfig = o.configOptions.resolved.LLM
	o.llmOptions.promptConfig = *o.configOptions.resolved.Prompt
	o.llmOptions.embeddingConfig = *o.configOptions.resolved.Embedding
	o.llmOptions.embeddingREs = matchREs

	return nil
}

func (o *DefaultRAGOptions) Validate() error {
	if err := o.StdioOptions.Validate(); err != nil {
		return err
	}

	return o.configOptions.Validate()
}

func (o *DefaultRAGOptions) Run(ctx context.Context, args ...string) error {
	for _, s := range o.steps {
		if err := s(ctx, args...); err != nil {
			return err
		}
	}

	return nil
}

func (o *DefaultRAGOptions) planFor(cmd *cobra.Command) {
	o.steps = o.steps[:0]

	switch cmd.CalledAs() {
	case "query", "chat", "tui":
		o.addStep(func(_ context.Context, _ ...string) error { return o.initLogger() })
		o.addStep(func(_ context.Context, _ ...string) error { return validateQueryParams(o) })
		o.addStep(func(_ context.Context, _ ...string) error { return o.initClient() })
		o.addStep(o.initLLMModels)
		o.addStep(func(_ context.Context, _ ...string) error {
			return validateSelectedModels(
				o.llmOptions.availableModels,
				o.llmOptions.chatConfig.Model,
				o.llmOptions.embeddingConfig.EmbeddingModel,
			)
		})
		o.addStep(func(_ context.Context, _ ...string) error { return o.initSession(o.Logger) })
		o.addStep(o.initVecdb)
	case "list":
		o.addStep(func(_ context.Context, _ ...string) error { return o.initLogger() })
		o.addStep(func(_ context.Context, _ ...string) error { return o.initClient() })
		o.addStep(o.initLLMModels)
	default:
	}
}

func (o *DefaultRAGOptions) addStep(s step) {
	o.steps = append(o.steps, s)
}

func (o *DefaultRAGOptions) initLogger() error {
	dir := o.configOptions.resolved.Logging.Dir
	name := o.configOptions.resolved.Logging.Filename

	f, err := openLogFile(dir, name)
	if err != nil {
		return errf("open log file: %v", err)
	}

	o.cleanupFuncs = append(o.cleanupFuncs, func() error { return f.Close() })

	level, _ := genericclioptions.ParseLevel(o.configOptions.resolved.Logging.Level)
	o.SetLevel(level)

	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level}))

	o.Opts(genericclioptions.WithLogger(logger))

	return nil
}

func (o *DefaultRAGOptions) initClient() error {
	opts := []llm.Option{
		llm.WithBaseURL(o.llmOptions.chatConfig.BaseURL),
		llm.WithLogger(o.Logger),
	}

	temperature := o.llmOptions.chatConfig.Temperature

	if temperature != 0.0 {
		opts = append(opts, llm.WithTemperature(temperature))
	}

	client, err := llm.NewClient(opts...)
	if err != nil {
		return errf("new client: %v", err)
	}

	o.llmOptions.client = client

	return nil
}

func (o *DefaultRAGOptions) initSession(logger *slog.Logger) error {
	var (
		model       = o.llmOptions.chatConfig.Model
		temperature = o.llmOptions.chatConfig.Temperature
		system      = o.llmOptions.promptConfig.System
	)

	sessionOpts := []llm.SessionOpt{
		llm.WithSessionLogger(logger),
	}

	if temperature != 0.0 {
		sessionOpts = append(sessionOpts, llm.WithSessionTemperature(temperature))
	}

	session, err := llm.NewChat(o.llmOptions.client, system, model, sessionOpts...)
	if err != nil {
		return errf("new chat session: %v", err)
	}

	o.llmOptions.session = session

	return nil
}

func (o *DefaultRAGOptions) initLLMModels(ctx context.Context, _ ...string) error {
	m, err := o.llmOptions.client.ListModels(ctx)
	if err != nil {
		return errf("llm list models: %v", err)
	}

	o.llmOptions.availableModels = m

	return nil
}

func (o *DefaultRAGOptions) initVecdb(_ context.Context, _ ...string) error {
	v, err := vecdb.New(o.llmOptions.embeddingConfig.Dimensions)
	if err != nil {
		return errf("create vector database:%v", err)
	}

	o.llmOptions.vectordb = v

	return nil
}

// NewDefaultRAGCommand creates the root cobra command.
func NewDefaultRAGCommand(iostreams *genericclioptions.IOStreams, args []string) *cobra.Command {
	o := NewDefaultRAGOptions(iostreams)

	cmd := &cobra.Command{
		Use:  "ragrat",
		Args: cobra.NoArgs,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		}, Short: "",
		Long: `ragrat is a terminal based, self-hosted Retrieval-Augmented Generation (RAG) assistant.

It supports local and remote LLMs via OpenAI-compatible APIs.
Configuration is handled via flags or config files.`,
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			o.planFor(cmd)
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o, args...))
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
	cmd.PersistentFlags().StringSliceVarP(&o.matchPatterns, "match", "M", nil, "Glob pattern(s) for matching files (e.g. '*.md', 'data/*.txt')")
	cmd.PersistentFlags().IntVarP(&o.configOptions.flags.dimensions, "dim", "", 0, "Embedding vector dimension (must match embedding model output)")

	hiddenFlags := []string{
		"base-url",
		"config",
		"dim",
		"embedding-model",
		"log-dir",
		"log-file",
		"log-level",
		"match",
		"model",
	}

	genericclioptions.MarkFlagsHidden(cmd, hiddenFlags...)

	cmd.AddCommand(NewCmdChat(o))
	cmd.AddCommand(NewCmdQuery(o))
	cmd.AddCommand(NewCmdConfig(o))
	cmd.AddCommand(NewCmdListModels(o))
	cmd.AddCommand(newVersionCommand(o))

	return cmd
}

func validateQueryParams(o *DefaultRAGOptions) error {
	var (
		model          = o.configOptions.resolved.LLM.Model
		embeddingModel = o.configOptions.resolved.Embedding.EmbeddingModel
		dim            = o.configOptions.resolved.Embedding.Dimensions
	)

	if model == "" {
		return ErrMissingLLMModel
	}

	if embeddingModel == "" {
		return ErrMissingEmbeddingModel
	}

	if dim == 0 {
		return ErrMissingDimension
	}

	errs := make([]error, 0, len(o.matchPatterns))

	for _, p := range o.matchPatterns {
		_, err := filepath.Match(p, "")
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func validateSelectedModels(models []string, selected ...string) error {
	errs := make([]error, 0, len(selected))

	for _, s := range selected {
		if !slices.Contains(models, s) {
			errs = append(errs, fmt.Errorf("%w: %q", ErrInvalidSelectedModel, s))
		}
	}

	return errors.Join(errs...)
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

func compileREs(exprs ...string) ([]*regexp.Regexp, error) {
	var (
		matchREs = make([]*regexp.Regexp, 0, len(exprs))
		errs     = make([]error, 0, len(exprs))
	)

	for _, expr := range exprs {
		re, err := regexp.Compile(expr)
		if err != nil {
			errs = append(errs, fmt.Errorf("invalid --match regex %q: %w", expr, err))
			continue
		}

		matchREs = append(matchREs, re)
	}

	return matchREs, errors.Join(errs...)
}

func errf(format string, a ...any) error {
	return fmt.Errorf(format, a...)
}
