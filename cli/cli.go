package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"

	"github.com/ladzaretti/ragrep/clierror"
	"github.com/ladzaretti/ragrep/genericclioptions"
	"github.com/ladzaretti/ragrep/types"
	"github.com/ladzaretti/ragrep/vecdb"

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
	appName                  = "ragrep"
	envConfigPathKeyOverride = "ragrep_CONFIG_PATH"
	defaultBaseURL           = "http://localhost:11434/v1"
	defaultConfigName        = ".ragrep.toml"
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

var defaultProvider = types.ProviderConfig{
	BaseURL: defaultBaseURL,
}

type cleanupFunc func() error

type step func(ctx context.Context, args ...string) error

// DefaultRAGOptions is the base cli config shared across all ragrep subcommands.
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

	o.llmOptions.llmConfig = o.configOptions.resolved.LLM
	o.llmOptions.promptConfig = *o.configOptions.resolved.Prompt
	o.llmOptions.embeddingConfig = *o.configOptions.resolved.Embedding
	o.llmOptions.embeddingREs = matchREs
	o.llmOptions.defaultContext = max(o.configOptions.flags.contextLength, 0)
	o.llmOptions.defaultTemperature = func(v float64) *float64 {
		if v == -1 {
			return nil
		}

		return &v
	}(o.configOptions.flags.temperature)

	return nil
}

func (o *DefaultRAGOptions) Validate() error {
	if err := o.StdioOptions.Validate(); err != nil {
		return err
	}

	if err := o.llmOptions.Validate(); err != nil {
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
		o.addStep(func(_ context.Context, _ ...string) error { return o.llmOptions.initProviders(o.Logger) })
		o.addStep(o.initLLMModels)
		o.addStep(func(_ context.Context, _ ...string) error { return validateSelectedModels(o.llmOptions) })
		o.addStep(o.initVecDim)
		o.addStep(o.initVecdb)
	case "list":
		o.addStep(func(_ context.Context, _ ...string) error { return o.initLogger() })
		o.addStep(func(_ context.Context, _ ...string) error { return o.llmOptions.initProviders(o.Logger) })
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

func (o *DefaultRAGOptions) initLLMModels(ctx context.Context, _ ...string) error {
	for _, p := range o.llmOptions.providers {
		m, err := p.Client.ListModels(ctx)
		if err != nil {
			return errf("llm list models: %v", err)
		}

		p.AvailableModels = m
	}

	return nil
}

func (o *DefaultRAGOptions) initVecDim(ctx context.Context, _ ...string) error {
	model := o.llmOptions.embeddingConfig.Model

	if model == "" {
		return ErrMissingEmbeddingModel
	}

	d, err := o.llmOptions.dimFor(ctx, model)
	if err != nil {
		return fmt.Errorf("init embedding dim: %w", err)
	}

	o.llmOptions.dim = d

	return nil
}

func (o *DefaultRAGOptions) initVecdb(_ context.Context, _ ...string) error {
	if o.llmOptions.dim == 0 {
		return ErrMissingDimension
	}

	v, err := vecdb.New(o.llmOptions.dim)
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
		Use:  "ragrep",
		Args: cobra.NoArgs,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		}, Short: "",
		Long: `ragrep is a terminal-first RAG assistant. 
Embed data, run retrieval, and query local or remote OpenAI API-compatible LLMs.`,
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

	cmd.PersistentFlags().Float64VarP(&o.configOptions.flags.temperature, "temp", "t", -1, "default sampling temperature (0.0-2.0)")
	cmd.PersistentFlags().IntVarP(&o.configOptions.flags.contextLength, "context", "x", -1, "default context length in tokens")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.model, "model", "m", "", "set LLM model")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.configPath, "config", "c", "", fmt.Sprintf("path to config file (default: ~/%s)", defaultConfigName))
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.embeddingModel, "embedding-model", "e", "", "set embedding model")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logDir, "log-dir", "d", "", "set log directory")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logFilename, "log-file", "f", "", "set log filename")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logLevel, "log-level", "l", "", "set log level (debug, info, warn, error)")
	cmd.PersistentFlags().StringSliceVarP(&o.matchPatterns, "match", "M", nil, "regex pattern(s) to match files (e.g. '^.*\\.md$', '(?i)\\.txt$')")

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
		"temp",
		"context",
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
		model          = o.configOptions.resolved.LLM.DefaultModel
		embeddingModel = o.configOptions.resolved.Embedding.Model
	)

	if model == "" {
		return ErrMissingLLMModel
	}

	if embeddingModel == "" {
		return ErrMissingEmbeddingModel
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

func validateSelectedModels(o *llmOptions, selected ...string) error {
	errs := make([]error, 0, len(selected))

	for _, model := range selected {
		_, err := o.providers.ProviderFor(model)
		if err != nil {
			errs = append(errs, errf("provider for %q: %w", model, err))
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
