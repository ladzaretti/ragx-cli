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

// Package-level env var storage (OpenAI env).
var (
	openAIAPIKey  string
	openAIAPIBase string
	openAIModel   string
)

// init reads and caches OpenAI environment variables:
//   - OPENAI_API_KEY, OPENAI_ENDPOINT, OPENAI_API_BASE, OPENAI_MODEL
//
// These serve as defaults; the model can be overridden by the Cobra --model flag.
// After loading env values, it registers the OpenAI provider factory.
func MustInitialize() {
	openAIAPIKey = os.Getenv("OPENAI_API_KEY")
	openAIAPIBase = os.Getenv("OPENAI_API_BASE")
	openAIModel = os.Getenv("OPENAI_MODEL")
}

// OpenAIClient implements the gollm.Client interface for OpenAI models.
type OpenAIClient struct {
	client openai.Client
}

// Ensure OpenAIClient implements the Client interface.
var _ Client = &OpenAIClient{}

type clientOptions struct {
	baseURL string
	apiKey  string

	// Extend with more options as needed
}

// Option is a functional option for configuring ClientOptions.
type Option func(*clientOptions)

func WithBaseURL(baseURL string) Option {
	return func(o *clientOptions) {
		o.baseURL = baseURL
	}
}

func WithAPIKey(key string) Option {
	return func(o *clientOptions) {
		o.apiKey = key
	}
}

// NewOpenAIClient creates a new client for interacting with OpenAI.
// Supports custom HTTP client (e.g., for skipping SSL verification).
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

	options := []option.RequestOption{}

	if len(c.baseURL) > 0 {
		options = append(options, option.WithBaseURL(baseURL))
	}

	if len(c.apiKey) > 0 {
		options = append(options, option.WithAPIKey(apiKey))
	}

	return &OpenAIClient{
		client: openai.NewClient(options...),
	}, nil
}

// Close cleans up any resources used by the client.
func (*OpenAIClient) Close() error {
	// No specific cleanup needed for the OpenAI client currently.
	return nil
}

// StartChat starts a new chat session.
func (c *OpenAIClient) StartChat(systemPrompt, model string) Chat { //nolint:ireturn
	selectedModel := getOpenAIModel(model)

	slog.Debug("Starting new chat session with model", "selectedModel", selectedModel)

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

// simpleCompletionResponse is a basic implementation of CompletionResponse.
type simpleCompletionResponse struct {
	content string
}

// Response returns the completion content.
func (r *simpleCompletionResponse) Response() string {
	return r.content
}

// UsageMetadata returns nil for now.
func (*simpleCompletionResponse) UsageMetadata() any {
	return nil
}

// GenerateCompletion sends a completion request to the OpenAI API.
func (c *OpenAIClient) GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error) { //nolint:ireturn
	slog.Info("OpenAI GenerateCompletion called with model", "model", req.Model)
	slog.Debug("Prompt for completion", "prompt", req.Prompt)

	completion, err := c.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: req.Model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(req.Prompt),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate OpenAI completion: %w", err)
	}

	// Check if there are choices and a message
	if len(completion.Choices) == 0 || completion.Choices[0].Message.Content == "" {
		return nil, errors.New("received an empty response from OpenAI")
	}

	// Return the content of the first choice
	resp := &simpleCompletionResponse{
		content: completion.Choices[0].Message.Content,
	}

	return resp, nil
}

// ListModels returns a slice of strings with model IDs.
// Note: This may not work with all OpenAI-compatible providers if they don't fully implement
// the Models.List endpoint or return data in a different format.
func (c *OpenAIClient) ListModels(ctx context.Context) ([]string, error) {
	res, err := c.client.Models.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("error listing models from OpenAI: %w", err)
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
		return nil, errors.New("no embedding data returned from API")
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

func (c *OpenAIClient) EmbedBatch(ctx context.Context, req EmbedBatchRequest) (EmbedBatchResponse, error) { //nolint:ireturn
	params := openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: req.Input},
		Model: req.Model,
	}

	slog.Info("Calling embedding batch API", "model", req.Model, "input_count", len(req.Input))

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

// getOpenAIModel returns the appropriate model based on configuration and explicitly provided model name.
func getOpenAIModel(model string) string {
	// If explicit model is provided, use it
	if model != "" {
		slog.Info("Using explicitly provided model", "model", model)
		return model
	}

	// Check configuration
	configModel := openAIModel
	if configModel != "" {
		slog.Info("Using model from config", "configModel", configModel)
		return configModel
	}

	// Default model as fallback
	slog.Info("No model specified, defaulting to gpt-4.1")

	return "gpt-4.1"
}
