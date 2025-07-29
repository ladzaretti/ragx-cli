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

// Send sends the user message(s), appends to history, and gets the LLM response.
func (cs *openAIChatSession) Send(ctx context.Context, contents ...any) (ChatResponse, error) { //nolint:ireturn
	slog.Info("openAIChatSession.Send called", "model", cs.model, "history_len", len(cs.history))

	// Process and append messages to history
	if err := cs.addContentsToHistory(contents); err != nil {
		return nil, err
	}

	// Prepare and send API request
	chatReq := openai.ChatCompletionNewParams{
		Model:    cs.model,
		Messages: cs.history,
	}

	// Call the OpenAI API
	slog.Info("Sending request to OpenAI Chat API", "model", cs.model, "messages", len(chatReq.Messages))

	completion, err := cs.client.Chat.Completions.New(ctx, chatReq)
	if err != nil {
		// TODO: Check if error is retryable using cs.IsRetryableError
		slog.Error("OpenAI ChatCompletion API error", "err", err)
		return nil, fmt.Errorf("OpenAI chat completion failed: %w", err)
	}

	slog.Info("Received response from OpenAI Chat API", "id", completion.ID, "choices", len(completion.Choices))

	// Process the response
	if len(completion.Choices) == 0 {
		slog.Warn("Received response with no choices from OpenAI")
		return nil, errors.New("received empty response from OpenAI (no choices)")
	}

	// Add assistant's response (first choice) to history
	assistantMsg := completion.Choices[0].Message
	// Convert to param type before appending to history
	cs.history = append(cs.history, assistantMsg.ToParam())
	slog.Info("Added assistant message to history", "content_present", assistantMsg.Content != "")

	// Wrap the response
	resp := &openAIChatResponse{
		openaiCompletion: completion,
	}

	return resp, nil
}

// SendStreaming sends the user message(s) and returns an iterator for the LLM response stream.
func (cs *openAIChatSession) SendStreaming(ctx context.Context, contents ...any) (ChatResponseIterator, error) {
	slog.Info("Starting OpenAI streaming request", "model", cs.model)

	// Process and append messages to history
	if err := cs.addContentsToHistory(contents); err != nil {
		return nil, err
	}

	// Prepare and send API request
	chatReq := openai.ChatCompletionNewParams{
		Model:    cs.model,
		Messages: cs.history,
	}

	// Start the OpenAI streaming request
	slog.Info("Sending streaming request to OpenAI API",
		"model", cs.model,
		"messageCount", len(chatReq.Messages),
	)

	stream := cs.client.Chat.Completions.NewStreaming(ctx, chatReq)

	// Create an accumulator to track the full response
	acc := openai.ChatCompletionAccumulator{}

	// Create and return the stream iterator
	return func(yield func(ChatResponse, error) bool) {
		defer func() {
			_ = stream.Close()
		}()

		var (
			lastResponseChunk *openAIChatStreamResponse
			currentContent    strings.Builder
		)

		// Process stream chunks
		for stream.Next() {
			chunk := stream.Current()

			// Update the accumulator with the new chunk
			acc.AddChunk(chunk)

			// Handle content completion
			if _, ok := acc.JustFinishedContent(); ok {
				slog.Info("Content stream finished")
			}

			// Handle refusal completion
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

			// Only process content if there are choices and a delta
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" {
					currentContent.WriteString(delta.Content)
					streamResponse.content = delta.Content // Only set content if there's new content
				}
			}

			// Keep track of the last response for history
			lastResponseChunk = &openAIChatStreamResponse{
				streamChunk: chunk,
				accumulator: acc,
				content:     currentContent.String(), // Full accumulated content for history
			}

			// Only yield if there's actual content
			if streamResponse.content != "" {
				if !yield(streamResponse, nil) {
					return
				}
			}
		}

		// Check for errors after streaming completes
		if err := stream.Err(); err != nil {
			slog.Error("Error in OpenAI streaming", "err", err)
			yield(nil, fmt.Errorf("OpenAI streaming error: %w", err))

			return
		}

		// Update conversation history with the complete message
		if lastResponseChunk != nil {
			completeMessage := openai.ChatCompletionMessage{
				Content: lastResponseChunk.content,
				Role:    "assistant",
			}

			// Append the full assistant response to history
			cs.history = append(cs.history, completeMessage.ToParam())

			slog.Info("Added complete assistant message to history",
				"content_present", completeMessage.Content != "")
		}
	}, nil
}

// IsRetryableError determines if an error from the OpenAI API should be retried.
func (*openAIChatSession) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	return DefaultIsRetryableError(err)
}

// addContentsToHistory processes and appends user messages to chat history.
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
