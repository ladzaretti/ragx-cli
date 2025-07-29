package llm

import (
	"context"
	"fmt"
	"io"
	"iter"
)

// Client is the main interface for interacting with an LLM backend.
type Client interface {
	io.Closer

	StartChat(systemPrompt, model string) Chat
	GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error)
	ListModels(ctx context.Context) ([]string, error)
	Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error)
	EmbedBatch(ctx context.Context, req EmbedBatchRequest) (EmbedBatchResponse, error)
}

// Chat represents a multi-turn conversation with the LLM.
type Chat interface {
	Send(ctx context.Context, contents ...any) (ChatResponse, error)
	SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error)
	IsRetryableError(err error) bool
}

// ChatResponseIterator is a streaming sequence of chat responses.
type ChatResponseIterator iter.Seq2[ChatResponse, error]

// CompletionRequest specifies a prompt and model for a completion call.
type CompletionRequest struct {
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt,omitempty"`
}

// CompletionResponse is the result of a single-shot completion call.
type CompletionResponse interface {
	Response() string
	UsageMetadata() any
}

// ChatResponse contains one or more candidates returned in a chat exchange.
type ChatResponse interface {
	Candidates() []Candidate
	UsageMetadata() any
}

// Candidate represents a possible LLM-generated response.
type Candidate interface {
	fmt.Stringer
	Parts() []Part
}

// Part is a fragment of a candidate, such as a text block or token.
type Part interface {
	AsText() (string, bool)
}

// EmbedRequest specifies a model and input string for embedding.
type EmbedRequest struct {
	Model string
	Input string
}

// EmbedResponse is a single embedding result.
type EmbedResponse interface {
	Vector() []float64
	UsageMetadata() any
}

// EmbedBatchRequest contains multiple inputs to embed with a model.
type EmbedBatchRequest struct {
	Model string
	Input []string
}

// EmbedBatchResponse is the result of a batch embedding call.
type EmbedBatchResponse interface {
	Vectors() [][]float64
	UsageMetadata() any
}
