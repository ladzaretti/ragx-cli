// Package llm provides a minimal OpenAI compatible client with chat,
// completion and embedding helpers.
//
// Requires Go 1.22+ for iter/cmp.
package llm

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

var (
	ErrNoModelSelected         = errors.New("no model specified")
	ErrEmptyCompletionResponse = errors.New("empty completion response")
	ErrNoEmbeddingReturned     = errors.New("no embedding returned")
)

// Client implements an open ai api compatible client.
type Client struct {
	config
	client openai.Client
}

type config struct {
	logger  *slog.Logger
	baseURL string
	apiKey  string
	model   string
}

// Option configures the OpenAI client.
type Option func(*config)

// WithBaseURL sets a custom API base URL.
func WithBaseURL(baseURL string) Option {
	return func(o *config) {
		o.baseURL = baseURL
	}
}

// WithAPIKey sets a custom API key.
func WithAPIKey(key string) Option {
	return func(o *config) {
		o.apiKey = key
	}
}

// WithModel sets a model to use.
func WithModel(model string) Option {
	return func(o *config) {
		o.model = model
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(o *config) {
		o.logger = logger
	}
}

// NewClient creates a new OpenAI client.
func NewClient(opts ...Option) (*Client, error) {
	c := &config{
		baseURL: os.Getenv("OPENAI_API_BASE"),
		apiKey:  os.Getenv("OPENAI_API_KEY"),
		model:   os.Getenv("OPENAI_MODEL"),
		logger:  slog.Default(),
	}

	for _, opt := range opts {
		opt(c)
	}

	options := []option.RequestOption{
		option.WithBaseURL(c.baseURL),
		option.WithAPIKey(c.apiKey),
	}

	return &Client{
		client: openai.NewClient(options...),
		config: *c,
	}, nil
}

// Close releases any resources (no-op for OpenAI).
func (*Client) Close() error {
	return nil
}

type CompletionRequest struct {
	Model  string
	Prompt string
}

// GenerateCompletion creates a single-turn completion from a prompt.
func (c *Client) GenerateCompletion(ctx context.Context, req CompletionRequest) (string, error) {
	model, err := c.selectModel(req.Model)
	if err != nil {
		return "", err
	}

	c.logger.Info("generate completion", "model", model)
	c.logger.Debug("prompt", "text", req.Prompt)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(req.Prompt),
		},
	})
	if err != nil {
		return "", err
	}

	if len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		return "", ErrEmptyCompletionResponse
	}

	return completion.Choices[0].Message.Content, nil
}

func (c *Client) selectModel(override string) (string, error) {
	if m := cmp.Or(override, c.model); m != "" {
		return m, nil
	}

	return "", ErrNoModelSelected
}

// ListModels returns available model IDs.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	res, err := c.client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	modelIDs := make([]string, 0, len(res.Data))
	for _, model := range res.Data {
		modelIDs = append(modelIDs, model.ID)
	}

	return modelIDs, nil
}

// EmbedRequest specifies a model and input string for embedding.
type EmbedRequest struct {
	Model string
	Input string
}

type EmbedResponse struct {
	Vector []float64
	Usage  *openai.CreateEmbeddingResponseUsage
}

// Embed returns the embedding for a single input.
func (c *Client) Embed(ctx context.Context, req EmbedRequest) (*EmbedResponse, error) {
	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(req.Input)},
		Model: req.Model,
	}

	c.logger.Info("embed request", "model", req.Model, "input_len", len(req.Input))

	res, err := c.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}

	if len(res.Data) == 0 {
		return nil, ErrNoEmbeddingReturned
	}

	return &EmbedResponse{
		Vector: res.Data[0].Embedding,
		Usage:  &res.Usage,
	}, nil
}

// EmbedBatchRequest contains multiple inputs to embed with a model.
type EmbedBatchRequest struct {
	Model string
	Input []string
}

type EmbedBatchResponse struct {
	Vectors [][]float64
	Usage   *openai.CreateEmbeddingResponseUsage
}

// EmbedBatch returns embeddings for multiple inputs.
func (c *Client) EmbedBatch(ctx context.Context, req EmbedBatchRequest) (*EmbedBatchResponse, error) {
	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: req.Input},
		Model: req.Model,
	}

	c.logger.Info("embed batch request", "model", req.Model, "input_count", len(req.Input))

	res, err := c.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("embedding batch request failed: %w", err)
	}

	if len(res.Data) != len(req.Input) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(req.Input), len(res.Data))
	}

	vectors := make([][]float64, len(res.Data))
	for i, e := range res.Data {
		vectors[i] = e.Embedding
	}

	return &EmbedBatchResponse{
		Vectors: vectors,
		Usage:   &res.Usage,
	}, nil
}

// ChatSession represents a single conversational context.
// Not thread safe, create a separate ChatSession per goroutine
// or protect calls with a mutex.
type ChatSession struct {
	logger  *slog.Logger
	client  openai.Client
	history []openai.ChatCompletionMessageParamUnion
	model   string
}

type SessionOpt func(*ChatSession)

func WithSessionLogger(logger *slog.Logger) SessionOpt {
	return func(c *ChatSession) {
		c.logger = logger
	}
}

// NewChat creates a new chat session with optional system prompt.
func NewChat(c *Client, systemPrompt, model string, opts ...SessionOpt) (*ChatSession, error) {
	session := &ChatSession{
		client: c.client,
		logger: slog.Default(),
	}

	for _, o := range opts {
		o(session)
	}

	logger := session.logger

	model, err := c.selectModel(model)
	if err != nil {
		return nil, err
	}

	logger.Info("start chat session", "model", model)

	history := []openai.ChatCompletionMessageParamUnion{}
	if systemPrompt != "" {
		history = append(history, openai.SystemMessage(systemPrompt))
	}

	session.history = history
	session.model = model

	return session, nil
}

// ChatResponseIterator is a streaming sequence of chat responses.
type ChatResponseIterator iter.Seq2[ChatResponse, error]

// ChatResponse is a non-streaming chat response.
type ChatResponse struct {
	Content string // assistant text
	Usage   any
}

func (s *ChatSession) selectModel(override string) (string, error) {
	if m := cmp.Or(override, s.model); m != "" {
		return m, nil
	}

	return "", ErrNoModelSelected
}

// Send sends user messages and returns a response.
// The assistant's reply is appended to the internal history.
func (s *ChatSession) Send(ctx context.Context, model string, contents ...string) (*ChatResponse, error) {
	model, err := s.selectModel(model)
	if err != nil {
		return nil, err
	}

	s.logger.Info("send chat turn", "model", model, "history_len", len(s.history))

	s.appendUserMessages(contents)

	chatReq := openai.ChatCompletionNewParams{
		Model:    model,
		Messages: s.history,
	}

	s.logger.Debug("chat request", "model", model, "message_count", len(chatReq.Messages))

	completion, err := s.client.Chat.Completions.New(ctx, chatReq)
	if err != nil {
		s.logger.Error("chat request failed", "err", err)
		return nil, err
	}

	s.logger.Debug("chat response", "id", completion.ID, "choices", len(completion.Choices))

	if len(completion.Choices) == 0 {
		return nil, ErrEmptyCompletionResponse
	}

	msg := completion.Choices[0].Message
	s.history = append(s.history, msg.ToParam())

	s.logger.Info("saved assistant message", "content_present", msg.Content != "")

	return &ChatResponse{
		Content: msg.Content,
		Usage:   completion.Usage,
	}, nil
}

// SendStreaming sends user messages and returns a streaming response iterator.
// The assistant's full reply is added to history after streaming completes.
func (s *ChatSession) SendStreaming(ctx context.Context, model string, contents ...string) (ChatResponseIterator, error) {
	model, err := s.selectModel(model)
	if err != nil {
		return nil, err
	}

	s.logger.Info("start streaming request", "model", model)

	s.appendUserMessages(contents)

	req := openai.ChatCompletionNewParams{
		Model:    model,
		Messages: s.history,
	}
	stream := s.client.Chat.Completions.NewStreaming(ctx, req)

	acc := openai.ChatCompletionAccumulator{}

	var buf strings.Builder

	return func(yield func(ChatResponse, error) bool) {
		defer func() {
			_ = stream.Close()
		}()

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if refusal, ok := acc.JustFinishedRefusal(); ok {
				yield(ChatResponse{}, fmt.Errorf("model refused: %v", refusal))
				return
			}

			if len(chunk.Choices) == 0 {
				continue
			}

			if delta := chunk.Choices[0].Delta.Content; delta != "" {
				buf.WriteString(delta)

				if !yield(ChatResponse{Content: delta}, nil) {
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				// rollback the orphaned user prompt.
				for i := len(s.history) - 1; i >= 0; i-- {
					if *s.history[i].GetRole() == "user" {
						s.history = s.history[:i]
						break
					}
				}
			}

			yield(ChatResponse{}, fmt.Errorf("stream error: %w", err))

			return
		}

		content := buf.String()
		if content != "" {
			param := openai.ChatCompletionMessage{Content: content, Role: "assistant"}.ToParam()
			s.history = append(s.history, param)
		}
	}, nil
}

// appendUserMessages appends user messages to the chat history.
func (s *ChatSession) appendUserMessages(msgs []string) {
	for _, msg := range msgs {
		s.history = append(s.history, openai.UserMessage(msg))
	}
}

// APIError wraps an HTTP error returned by the LLM provider.
type APIError struct {
	StatusCode int
	Message    string
	Err        error
}

func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("API Error: status=%d, message=%q, cause=%v", e.StatusCode, e.Message, e.Err)
	}

	return fmt.Sprintf("API Error: status=%d, message=%q", e.StatusCode, e.Message)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// IsRetryableError returns true if the error is retryable.
// It handles common HTTP codes and network timeouts.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusConflict,
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout:
			return true
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	return false
}
