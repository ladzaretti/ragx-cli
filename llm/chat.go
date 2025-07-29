package llm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	openai "github.com/openai/openai-go"
)

type openAIChatSession struct {
	client  openai.Client
	history []openai.ChatCompletionMessageParamUnion
	model   string
}

var _ Chat = (*openAIChatSession)(nil)

// Send sends user messages and returns a response.
// The assistant's reply is appended to the internal history.
func (cs *openAIChatSession) Send(ctx context.Context, contents ...any) (ChatResponse, error) { //nolint:ireturn
	slog.Info("openAIChatSession.Send called", "model", cs.model, "history_len", len(cs.history))

	if err := cs.addContentsToHistory(contents); err != nil {
		return nil, err
	}

	chatReq := openai.ChatCompletionNewParams{
		Model:    cs.model,
		Messages: cs.history,
	}

	slog.Info("Sending request to OpenAI Chat API", "model", cs.model, "messages", len(chatReq.Messages))

	completion, err := cs.client.Chat.Completions.New(ctx, chatReq)
	if err != nil {
		slog.Error("OpenAI ChatCompletion API error", "err", err)
		return nil, fmt.Errorf("OpenAI chat completion failed: %w", err)
	}

	slog.Info("Received response from OpenAI Chat API", "id", completion.ID, "choices", len(completion.Choices))

	if len(completion.Choices) == 0 {
		slog.Warn("Received response with no choices from OpenAI")
		return nil, errors.New("received empty response from OpenAI (no choices)")
	}

	assistantMsg := completion.Choices[0].Message
	cs.history = append(cs.history, assistantMsg.ToParam())

	slog.Info("Added assistant message to history", "content_present", assistantMsg.Content != "")

	return &openAIChatResponse{openaiCompletion: completion}, nil
}

// SendStreaming sends user messages and returns a streaming response iterator.
// The assistant's full reply is added to history after streaming completes.
func (cs *openAIChatSession) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	slog.Info("Starting OpenAI streaming request", "model", cs.model)

	if err := cs.addContentsToHistory(contents); err != nil {
		return nil, err
	}

	chatReq := openai.ChatCompletionNewParams{
		Model:    cs.model,
		Messages: cs.history,
	}

	slog.Info("Sending streaming request to OpenAI API",
		"model", cs.model,
		"messageCount", len(chatReq.Messages),
	)

	stream := cs.client.Chat.Completions.NewStreaming(ctx, chatReq)
	acc := openai.ChatCompletionAccumulator{}

	return func(yield func(ChatResponse, error) bool) {
		defer func() { _ = stream.Close() }()

		var (
			lastResponseChunk *openAIChatStreamResponse
			currentContent    strings.Builder
		)

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if _, ok := acc.JustFinishedContent(); ok {
				slog.Info("Content stream finished")
			}

			if refusal, ok := acc.JustFinishedRefusal(); ok {
				slog.Info("Refusal stream finished", "refusal", refusal)
				yield(nil, fmt.Errorf("model refused to respond: %v", refusal))

				return
			}

			streamResponse := &openAIChatStreamResponse{
				streamChunk: chunk,
				accumulator: acc,
				content:     "",
			}

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" {
					currentContent.WriteString(delta.Content)
					streamResponse.content = delta.Content
				}
			}

			lastResponseChunk = &openAIChatStreamResponse{
				streamChunk: chunk,
				accumulator: acc,
				content:     currentContent.String(),
			}

			if streamResponse.content != "" {
				if !yield(streamResponse, nil) {
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			slog.Error("Error in OpenAI streaming", "err", err)
			yield(nil, fmt.Errorf("OpenAI streaming error: %w", err))

			return
		}

		if lastResponseChunk != nil {
			completeMessage := openai.ChatCompletionMessage{
				Content: lastResponseChunk.content,
				Role:    "assistant",
			}
			cs.history = append(cs.history, completeMessage.ToParam())

			slog.Info("Added complete assistant message to history",
				"content_present", completeMessage.Content != "")
		}
	}, nil
}

// IsRetryableError returns true if the error is retryable.
func (*openAIChatSession) IsRetryableError(err error) bool {
	return err != nil && DefaultIsRetryableError(err)
}

// addContentsToHistory appends user messages to the chat history.
func (cs *openAIChatSession) addContentsToHistory(contents []any) error {
	for _, content := range contents {
		switch c := content.(type) {
		case string:
			slog.Info("Adding user message to history", "content", c)
			cs.history = append(cs.history, openai.UserMessage(c))
		default:
			slog.Warn("Unhandled content type", "type", fmt.Sprintf("%T", content))
			return fmt.Errorf("unhandled content type: %T", content)
		}
	}

	return nil
}
