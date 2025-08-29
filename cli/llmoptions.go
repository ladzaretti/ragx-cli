package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"time"

	"github.com/ladzaretti/ragrat/cli/types"
	"github.com/ladzaretti/ragrat/genericclioptions"
	"github.com/ladzaretti/ragrat/llm"
	"github.com/ladzaretti/ragrat/vecdb"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

type llmOptions struct {
	llmConfig       LLMConfig
	promptConfig    PromptConfig
	embeddingConfig EmbeddingConfig

	providers    types.Providers
	vectordb     *vecdb.VectorDB
	dim          int
	embeddingREs []*regexp.Regexp
}

var _ genericclioptions.BaseOptions = &llmOptions{}

func (*llmOptions) Complete() error { return nil }

func (*llmOptions) Validate() error { return nil }

func (o *llmOptions) initProviders(logger *slog.Logger) error {
	o.providers = make([]*types.Provider, 0, len(o.llmConfig.Providers))

	for _, c := range o.llmConfig.Providers {
		client, err := createClient(logger, c)
		if err != nil {
			return err
		}

		session, err := createSession(logger, client, c.Temperature, o.promptConfig.System)
		if err != nil {
			return err
		}

		p := &types.Provider{
			Client:  client,
			Session: session,
		}

		o.providers = append(o.providers, p)
	}

	return nil
}

func (o *llmOptions) dimFor(ctx context.Context, embeddingModel string) (int, error) {
	provider, err := o.providers.ProviderFor(embeddingModel)
	if err != nil {
		return 0, fmt.Errorf("provider for: %w", err)
	}

	req := llm.EmbedRequest{
		Input: "",
		Model: embeddingModel,
	}

	res, err := provider.Client.Embed(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("dim: %w", err)
	}

	return len(res.Vector), nil
}

func (o *llmOptions) embed(ctx context.Context, logger *slog.Logger, r io.Reader, matchREs []*regexp.Regexp, args ...string) error {
	ctx, cancel := context.WithCancel(ctx)

	spinner := newSpinner(cancel, "")

	go spinner.run()

	defer spinner.stop()

	switch {
	case r != nil:
		return o.embedInput(ctx, logger, spinner.sendStatusWithEllipsis, r)
	case len(args) > 0:
		return o.discoverAndEmbed(ctx, logger, spinner.display, spinner.setStatus, matchREs, args...)
	default:
	}

	return nil
}

func (o *llmOptions) embedInput(ctx context.Context, logger *slog.Logger, sendStatus func(string), r io.Reader) error {
	bs, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read piped input: %w", err)
	}

	chunks, err := ChunkText(string(bs),
		o.embeddingConfig.ChunkSize,
		o.embeddingConfig.Overlap,
	)
	if err != nil {
		return fmt.Errorf("chunk piped input: %w", err)
	}

	dataChunks := &dataChunks{
		source: "piped-data",
		chunks: chunks,
	}

	sendStatus("embedding piped data")

	if err := o.embedData(ctx, logger, dataChunks); err != nil {
		return fmt.Errorf("embed piped input: %w", err)
	}

	return nil
}

func (o *llmOptions) discoverAndEmbed(ctx context.Context, logger *slog.Logger, display func(text string), status func(string), matchREs []*regexp.Regexp, args ...string) error {
	defer func(start time.Time) {
		elapsed := time.Since(start)
		logger.Debug("embedding total duration", "duration", elapsed)
	}(time.Now())

	discovered, err := discover(args, matchREs)
	if err != nil {
		return err
	}

	chunkedFiles, err := chunkFiles(ctx, display, discovered,
		o.embeddingConfig.ChunkSize,
		o.embeddingConfig.Overlap,
	)
	if err != nil {
		return err
	}

	logger.Debug("discovered files", "files", len(chunkedFiles), "chunks", totalChunks(chunkedFiles))

	return o.embedAll(ctx, logger, status, chunkedFiles)
}

func (o *llmOptions) embedAll(ctx context.Context, logger *slog.Logger, sendStatus func(string), chunkedFiles []*dataChunks) error {
	g, ctx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(embedConcurrency)

	for i, cf := range chunkedFiles {
		if err := sem.Acquire(ctx, 1); err != nil {
			break
		}

		g.Go(func() error {
			defer sem.Release(1)
			sendStatus(fmt.Sprintf("embedding [%d/%d] %s", i+1, len(chunkedFiles), cf.source))

			return o.embedData(ctx, logger, cf)
		})
	}

	return g.Wait()
}

func (o *llmOptions) embedData(ctx context.Context, logger *slog.Logger, cf *dataChunks) error {
	n := len(cf.chunks)
	embeddingModel := o.embeddingConfig.Model

	provider, err := o.providers.ProviderFor(embeddingModel)
	if err != nil {
		return fmt.Errorf("provider for: %w", err)
	}

	for i := 0; i < n; i += embedBatchSize {
		end := min(i+embedBatchSize, n)

		req := llm.EmbedBatchRequest{
			Input: cf.chunks[i:end],
			Model: o.embeddingConfig.Model,
		}

		res, err := provider.Client.EmbedBatch(ctx, req)
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
				Meta:    vecdb.Meta{Source: cf.source, Index: i + j},
			}
			embedded = append(embedded, vecChunk)
		}

		if err := o.vectordb.Insert(embedded); err != nil {
			return fmt.Errorf("vectordb insert %q [%d:%d]: %w", cf.source, i, end, err)
		}

		logger.Debug("embedded batch", "range", fmt.Sprintf("[%d:%d]", i, end), "total", n, "source", cf.source)

		if end == n {
			break
		}
	}

	return nil
}

func createClient(logger *slog.Logger, c ProviderConfig) (*llm.Client, error) {
	opts := []llm.Option{
		llm.WithBaseURL(c.BaseURL),
		llm.WithLogger(logger),
		llm.WithTemperature(c.Temperature),
	}

	client, err := llm.NewClient(opts...)
	if err != nil {
		return nil, errf("new client: %v", err)
	}

	return client, nil
}

func createSession(logger *slog.Logger, client *llm.Client, temperature *float64, systemPrompt string) (*llm.ChatSession, error) {
	sessionOpts := []llm.SessionOpt{
		llm.WithSessionLogger(logger),
		llm.WithSessionTemperature(temperature),
	}

	session, err := llm.NewChat(client, systemPrompt, sessionOpts...)
	if err != nil {
		return nil, errf("new chat session: %v", err)
	}

	return session, nil
}

func toFloat32Slice(src []float64) (f32 []float32) {
	f32 = make([]float32, len(src))

	for i, v := range src {
		f32[i] = float32(v)
	}

	return f32
}
