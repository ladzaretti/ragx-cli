package llm

import (
	"context"
	"fmt"
	"io"
	"iter"
)

// Client is the main LLM client interface.
type Client interface {
	io.Closer
	StartChat(systemPrompt, model string) Chat
	GenerateCompletion(ctx context.Context, req *CompletionRequest) (CompletionResponse, error)
	ListModels(ctx context.Context) ([]string, error)
	Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error)
	EmbedBatch(ctx context.Context, req EmbedBatchRequest) (EmbedBatchResponse, error)
}

// ChatResponseIterator is a streaming chat response from the LLM.
type ChatResponseIterator iter.Seq2[ChatResponse, error]

// Chat represents a multi-turn chat session with an LLM.
type Chat interface {
	Send(ctx context.Context, contents ...any) (ChatResponse, error)
	SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error)
	IsRetryableError(err error) bool
}

// CompletionRequest is a request to generate a text completion.
type CompletionRequest struct {
	Model  string `json:"model,omitempty"`
	Prompt string `json:"prompt,omitempty"`
}

// CompletionResponse is the result of a completion request.
type CompletionResponse interface {
	Response() string
	UsageMetadata() any
}

// ChatResponse represents a response from the LLM in a chat.
type ChatResponse interface {
	Candidates() []Candidate
	UsageMetadata() any
}

// Candidate is a possible response candidate from the LLM.
type Candidate interface {
	fmt.Stringer
	Parts() []Part
}

// Part is a segment of a candidate response (e.g. text).
type Part interface {
	AsText() (string, bool)
}

// EmbedRequest is a request to embed a single string input.
type EmbedRequest struct {
	Model string
	Input string
}

// EmbedResponse contains the embedding vector for a single input.
type EmbedResponse interface {
	Vector() []float64
	UsageMetadata() any
}

// EmbedBatchRequest is a request to embed multiple inputs.
type EmbedBatchRequest struct {
	Model string
	Input []string
}

// EmbedBatchResponse contains multiple embedding vectors.
type EmbedBatchResponse interface {
	Vectors() [][]float64
	UsageMetadata() any
}
