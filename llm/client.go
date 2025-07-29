package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

var (
	openAIAPIKey  string
	openAIAPIBase string
	openAIModel   string
)

// InitEnv loads OpenAI-related environment variables.
// These act as fallbacks for client options.
func InitEnv() {
	openAIAPIKey = os.Getenv("OPENAI_API_KEY")
	openAIAPIBase = os.Getenv("OPENAI_API_BASE")
	openAIModel = os.Getenv("OPENAI_MODEL")
}

// OpenAIClient implements Client using the OpenAI API.
type OpenAIClient struct {
	client openai.Client
}

var _ Client = &OpenAIClient{}

type clientOptions struct {
	baseURL string
	apiKey  string
}

// Option configures the OpenAI client.
type Option func(*clientOptions)

// WithBaseURL sets a custom API base URL.
func WithBaseURL(baseURL string) Option {
	return func(o *clientOptions) {
		o.baseURL = baseURL
	}
}

// WithAPIKey sets a custom API key.
func WithAPIKey(key string) Option {
	return func(o *clientOptions) {
		o.apiKey = key
	}
}

// NewOpenAIClient creates a new OpenAI client.
func NewOpenAIClient(opts ...Option) (*OpenAIClient, error) {
	c := &clientOptions{}
	for _, opt := range opts {
		opt(c)
	}

	baseURL := c.baseURL
	if baseURL == "" {
		baseURL = openAIAPIBase
	}

	apiKey := c.apiKey
	if apiKey == "" {
		apiKey = openAIAPIKey
	}

	var options []option.RequestOption
	if baseURL != "" {
		options = append(options, option.WithBaseURL(baseURL))
	}

	if apiKey != "" {
		options = append(options, option.WithAPIKey(apiKey))
	}

	return &OpenAIClient{
		client: openai.NewClient(options...),
	}, nil
}

// Close releases any resources (no-op for OpenAI).
func (*OpenAIClient) Close() error {
	return nil
}

// StartChat creates a new chat session with optional system prompt.
func (c *OpenAIClient) StartChat(systemPrompt, model string) Chat { //nolint:ireturn
	selectedModel := getOpenAIModel(model)

	slog.Debug("Starting new chat session", "model", selectedModel)

	history := []openai.ChatCompletionMessageParamUnion{}
	if systemPrompt != "" {
		history = append(history, openai.SystemMessage(systemPrompt))
	}

	return &openAIChatSession{
		client:  c.client,
		history: history,
		model:   selectedModel,
	}
}

type simpleCompletionResponse struct {
	content string
}

func (r *simpleCompletionResponse) Response() string {
	return r.content
}

func (*simpleCompletionResponse) UsageMetadata() any {
	return nil
}

// GenerateCompletion creates a single-turn completion from a prompt.
func (c *OpenAIClient) GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error) { //nolint:ireturn
	slog.Info("Generating completion", "model", req.Model)
	slog.Debug("Prompt", "text", req.Prompt)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: req.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(req.Prompt),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("completion failed: %w", err)
	}

	if len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		return nil, errors.New("empty completion response")
	}

	return &simpleCompletionResponse{
		content: completion.Choices[0].Message.Content,
	}, nil
}

// ListModels returns available model IDs.
func (c *OpenAIClient) ListModels(ctx context.Context) ([]string, error) {
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

type openAIEmbedResponse struct {
	vector []float64
	usage  *openai.CreateEmbeddingResponseUsage
}

func (r *openAIEmbedResponse) Vector() []float64 {
	return r.vector
}

func (r *openAIEmbedResponse) UsageMetadata() any {
	return r.usage
}

// Embed returns the embedding for a single input.
func (c *OpenAIClient) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) { //nolint:ireturn
	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfString: openai.String(req.Input)},
		Model: req.Model,
	}

	slog.Info("Calling embedding API", "model", req.Model, "input_len", len(req.Input))

	res, err := c.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}

	if len(res.Data) == 0 {
		return nil, errors.New("no embedding returned")
	}

	return &openAIEmbedResponse{
		vector: res.Data[0].Embedding,
		usage:  &res.Usage,
	}, nil
}

type openAIEmbedBatchResponse struct {
	vectors [][]float64
	usage   *openai.CreateEmbeddingResponseUsage
}

func (r *openAIEmbedBatchResponse) Vectors() [][]float64 {
	return r.vectors
}

func (r *openAIEmbedBatchResponse) UsageMetadata() any {
	return r.usage
}

// EmbedBatch returns embeddings for multiple inputs.
func (c *OpenAIClient) EmbedBatch(ctx context.Context, req EmbedBatchRequest) (EmbedBatchResponse, error) { //nolint:ireturn
	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: req.Input},
		Model: req.Model,
	}

	slog.Info("Calling batch embedding API", "model", req.Model, "input_count", len(req.Input))

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

	return &openAIEmbedBatchResponse{
		vectors: vectors,
		usage:   &res.Usage,
	}, nil
}

// getOpenAIModel selects a model from env or fallback.
func getOpenAIModel(model string) string {
	if model != "" {
		slog.Info("Using model from request", "model", model)
		return model
	}

	if openAIModel != "" {
		slog.Info("Using model from env", "model", openAIModel)
		return openAIModel
	}

	slog.Info("No model specified, defaulting to gpt-4.1")

	return "gpt-4.1"
}
