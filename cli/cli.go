package cli

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
	"github.com/ladzaretti/ragrat/llm"
	"github.com/ladzaretti/ragrat/vecdb"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/spf13/cobra"
)

var Version = "0.0.0"

var (
	ErrMissingQuery          = errors.New("missing required --query flag")
	ErrMissingLLMModel       = errors.New("missing LLM model")
	ErrMissingEmbeddingModel = errors.New("missing embedding model")
	ErrMissingDimension      = errors.New("missing or invalid embedding dimension")
	ErrInvalidSelectedModel  = errors.New("selected model not found in available models")
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
	defaultChunkSize         = 600
	defaultOverlap           = 60
	defaultTopK              = 10
)

type cleanupFunc func() error

type llmOptions struct {
	client         *llm.Client
	session        *llm.ChatSession
	vectordb       *vecdb.VectorDB
	models         []string
	selectedModel  string
	embeddingModel string
}

var _ genericclioptions.BaseOptions = &llmOptions{}

func (*llmOptions) Complete() error { return nil }

func (*llmOptions) Validate() error { return nil }

type step func(ctx context.Context, args ...string) error

// DefaultRAGOptions is the base cli config shared across all ragrat subcommands.
type DefaultRAGOptions struct {
	*genericclioptions.StdioOptions

	configOptions *ConfigOptions
	llmOptions    *llmOptions

	cleanupFuncs  []cleanupFunc
	query         string
	matchPatterns []string
	matchREs      []*regexp.Regexp

	run []step
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

	o.matchREs = matchREs

	return nil
}

func (o *DefaultRAGOptions) Validate() error {
	if err := o.StdioOptions.Validate(); err != nil {
		return err
	}

	return o.configOptions.Validate()
}

func (o *DefaultRAGOptions) Run(ctx context.Context, args ...string) error {
	for _, s := range o.run {
		if err := s(ctx, args...); err != nil {
			return err
		}
	}

	return nil
}

func (o *DefaultRAGOptions) planFor(cmd *cobra.Command) {
	o.run = o.run[:0]

	switch cmd.CalledAs() {
	case "ragrat", "tui":
		o.add(o.prepareLLMEnvironment)
		o.add(o.embed)
	default:
	}
}

func (o *DefaultRAGOptions) add(rs step) {
	o.run = append(o.run, rs)
}

func (o *DefaultRAGOptions) prepareLLMEnvironment(ctx context.Context, _ ...string) error {
	logger, err := o.initLogger()
	if err != nil {
		return err
	}

	o.Opts(genericclioptions.WithLogger(logger))

	if err := validateQueryParams(o); err != nil {
		return err
	}

	if err := o.initLLM(logger); err != nil {
		return err
	}

	m, err := o.llmOptions.client.ListModels(ctx)
	if err != nil {
		return errf("llm list models: %v", err)
	}

	if err := validateSelectedModels(m, o.llmOptions.selectedModel, o.configOptions.resolved.Embedding.EmbeddingModel); err != nil {
		return err
	}

	v, err := vecdb.New(o.configOptions.resolved.Embedding.Dimensions)
	if err != nil {
		return errf("create vector database:%v", err)
	}

	o.llmOptions.models = m
	o.llmOptions.vectordb = v

	return nil
}

func (o *DefaultRAGOptions) embed(ctx context.Context, args ...string) error {
	defer func(start time.Time) {
		elapsed := time.Since(start)
		o.Infof("embedding took %s\n", elapsed.String())
	}(time.Now())

	discovered, err := discover(args, o.matchREs)
	if err != nil {
		return err
	}

	chunkedFiles, err := chunkFiles(ctx, o.IOStreams, discovered,
		o.configOptions.resolved.Embedding.ChunkSize,
		o.configOptions.resolved.Embedding.Overlap,
	)
	if err != nil {
		return err
	}

	o.Infof("discovered %d files, produced %d chunks\n", len(chunkedFiles), totalChunks(chunkedFiles))

	return o.embedAll(ctx, chunkedFiles)
}

func (o *DefaultRAGOptions) embedAll(ctx context.Context, chunkedFiles []*fileChunks) error {
	g, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(embedConcurrency)

	for _, cf := range chunkedFiles {
		if err := sem.Acquire(ctx, 1); err != nil {
			break
		}

		g.Go(func() error {
			defer sem.Release(1)
			return o.embedFile(ctx, cf)
		})
	}

	return g.Wait()
}

func (o *DefaultRAGOptions) embedFile(ctx context.Context, cf *fileChunks) error {
	n := len(cf.chunks)

	for i := 0; i < n; i += embedBatchSize {
		end := min(i+embedBatchSize, n)

		req := llm.EmbedBatchRequest{
			Input: cf.chunks[i:end],
			Model: o.configOptions.resolved.Embedding.EmbeddingModel,
		}

		res, err := o.llmOptions.client.EmbedBatch(ctx, req)
		if err != nil {
			return fmt.Errorf("embed batch [%d:%d]: %w", i, end, err)
		}

		if want, got := end-i, len(res.Vectors); want != got {
			return fmt.Errorf("embed batch [%d:%d]: want %d, got %d vectors",
				i, end, want, got)
		}

		embedded := make([]vecdb.Chunk, 0, len(res.Vectors))

		for j, vec := range res.Vectors {
			vecChunk := vecdb.Chunk{
				Content: cf.chunks[i+j],
				Vec:     toFloat32Slice(vec),
				Meta:    vecdb.Meta{Path: cf.path, Index: i + j},
			}
			embedded = append(embedded, vecChunk)
		}

		if err := o.llmOptions.vectordb.Insert(embedded); err != nil {
			return fmt.Errorf("vectordb insert %q [%d:%d]: %w", cf.path, i, end, err)
		}

		o.Infof(
			"embedded batch [%d:%d] of %d for %q\n",
			i, end, n, cf.path,
		)

		if end == n {
			break
		}
	}

	return nil
}

func toFloat32Slice(src []float64) (f32 []float32) {
	f32 = make([]float32, len(src))

	for i, v := range src {
		f32[i] = float32(v)
	}

	return f32
}

func (o *DefaultRAGOptions) RunQuery(_ context.Context, _ ...string) error {
	if o.query == "" {
		return ErrMissingQuery
	}

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

	temperature := o.configOptions.resolved.LLM.Temperature

	if temperature != 0.0 {
		opts = append(opts, llm.WithTemperature(temperature))
	}

	client, err := llm.NewClient(opts...)
	if err != nil {
		return errf("new client: %v", err)
	}

	var (
		model          = o.configOptions.resolved.LLM.Model
		embeddingModel = o.configOptions.resolved.Embedding.EmbeddingModel
		system         = o.configOptions.fileConfig.Prompt.System
	)

	sessionOpts := []llm.SessionOpt{
		llm.WithSessionLogger(logger),
	}

	if temperature != 0.0 {
		sessionOpts = append(sessionOpts, llm.WithSessionTemperature(temperature))
	}

	session, err := llm.NewChat(client, system, model, sessionOpts...)
	if err != nil {
		return errf("new chat session: %v", err)
	}

	o.llmOptions.client = client
	o.llmOptions.session = session
	o.llmOptions.selectedModel = model
	o.llmOptions.embeddingModel = embeddingModel

	return nil
}

// NewDefaultRAGCommand creates the root cobra command.
func NewDefaultRAGCommand(iostreams *genericclioptions.IOStreams, args []string) *cobra.Command {
	o := NewDefaultRAGOptions(iostreams)

	cmd := &cobra.Command{
		Use:   "ragrat [files]",
		Args:  cobra.MinimumNArgs(1),
		Short: "",
		Long: `ragrat is a terminal based, self-hosted Retrieval-Augmented Generation (RAG) assistant.

It supports local and remote LLMs via OpenAI-compatible APIs.
Configuration is handled via flags or config files.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			o.planFor(cmd)
			return clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o, args...))
		},
		PersistentPostRunE: func(_ *cobra.Command, _ []string) error {
			return clierror.Check(executeCleanup(o.cleanupFuncs))
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return clierror.Check(o.RunQuery(cmd.Context(), args...))
		},
	}

	cmd.SetArgs(args)

	cmd.Flags().StringVarP(&o.query, "query", "q", "", "Query to send to the LLM")

	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.baseURL, "base-url", "u", "", "Override LLM base URL")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.model, "model", "m", "", "Override LLM model")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.configPath, "config", "c", "", "Path to config file")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.embeddingModel, "embedding-model", "e", "", "Override embedding model")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logDir, "log-dir", "d", "", "Override log directory")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logFilename, "log-file", "f", "", "Override log filename")
	cmd.PersistentFlags().StringVarP(&o.configOptions.flags.logLevel, "log-level", "l", "", "Set log level (debug, info, warn, error)")
	cmd.PersistentFlags().StringSliceVarP(&o.matchPatterns, "match", "M", nil, "Glob pattern(s) for matching files (e.g. '*.md', 'data/*.txt')")
	cmd.PersistentFlags().IntVarP(&o.configOptions.flags.dimensions, "dim", "", 0, "Embedding vector dimension (must match embedding model output)")

	cmd.AddCommand(NewCmdTUI(o))
	cmd.AddCommand(NewCmdConfig(o))

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
