// Package llm provides a lightweight openai api compatible client for
// streaming chat completions.
package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type (
	ChatParams   = openai.ChatCompletionNewParams
	MessageUnion = openai.ChatCompletionMessageParamUnion
	ChatModel    = openai.ChatModel
	Accumulator  = openai.ChatCompletionAccumulator
)

// Client wraps an openai api compatible client with optional accumulator tracking.
type Client struct {
	*config
	client *openai.Client
}

type config struct {
	baseURL     string
	apiKey      string
	accumulator bool
}

type Opt func(*config)

// WithBaseURL sets the API base URL (e.g. http://localhost:11434/v1 for Ollama).
func WithBaseURL(url string) Opt {
	return func(c *config) {
		c.baseURL = url
	}
}

func WithAPIKey(key string) Opt {
	return func(c *config) {
		c.apiKey = key
	}
}

// WithAccumulator enables or disables usage tracking via an accumulator.
func WithAccumulator(enabled bool) Opt {
	return func(c *config) {
		c.accumulator = enabled
	}
}

func New(configOpts ...Opt) *Client {
	c := &config{}

	for _, opt := range configOpts {
		opt(c)
	}

	opts := []option.RequestOption{}

	if len(c.baseURL) > 0 {
		opts = append(opts, option.WithBaseURL(c.baseURL))
	}

	if len(c.apiKey) > 0 {
		opts = append(opts, option.WithAPIKey(c.apiKey))
	}

	client := openai.NewClient(opts...)

	return &Client{
		client: &client,
		config: c,
	}
}

// ChatStreaming performs a streamed chat completion request
// and writes output to the given writer.
//
// If the accumulator is enabled, token usage stats will be printed at the end.
func (c *Client) ChatStreaming(ctx context.Context, w io.Writer, params ChatParams) error {
	stream := c.client.Chat.Completions.NewStreaming(ctx, params)
	defer func() {
		_ = stream.Close()
	}()

	var acc *Accumulator
	if c.accumulator {
		a := openai.ChatCompletionAccumulator{}
		acc = &a
	}

	for stream.Next() {
		chunk := stream.Current()

		if acc != nil {
			acc.AddChunk(chunk)
		}

		if len(chunk.Choices) > 0 {
			_, _ = fmt.Fprint(w, chunk.Choices[0].Delta.Content)
		}
	}

	if err := stream.Err(); err != nil {
		return fmt.Errorf("streaming chat failed: %w", err)
	}

	if acc != nil && acc.Usage.TotalTokens > 0 {
		slog.Info("chat completion finished", "tokens", acc.Usage.TotalTokens)
	}

	return nil
}

// ListModels returns the IDs of available models from the underlying api.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	list, err := c.client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	models := make([]string, 0, len(list.Data))

	for _, m := range list.Data {
		models = append(models, m.ID)
	}

	return models, nil
}

// Embedding returns the embedding vector for the given input and model.
func (c *Client) Embedding(ctx context.Context, model string, input string) ([]float64, error) {
	req := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(input)},
		Model: model,
	}

	res, err := c.client.Embeddings.New(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}

	if len(res.Data) == 0 {
		return nil, errors.New("no embedding data returned from API")
	}

	return res.Data[0].Embedding, nil
}
