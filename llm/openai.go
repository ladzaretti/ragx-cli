package llm

import (
	"fmt"

	openai "github.com/openai/openai-go"
)

// openAIChatResponse implements ChatResponse for non-streaming chat.
type openAIChatResponse struct {
	openaiCompletion *openai.ChatCompletion
}

var _ ChatResponse = (*openAIChatResponse)(nil)

func (r *openAIChatResponse) UsageMetadata() any {
	if r.openaiCompletion != nil && r.openaiCompletion.Usage.TotalTokens > 0 {
		return r.openaiCompletion.Usage
	}

	return nil
}

func (r *openAIChatResponse) Candidates() []Candidate {
	if r.openaiCompletion == nil {
		return nil
	}

	candidates := make([]Candidate, len(r.openaiCompletion.Choices))

	for i, choice := range r.openaiCompletion.Choices {
		candidates[i] = &openAICandidate{openaiChoice: &choice}
	}

	return candidates
}

// openAICandidate implements Candidate for standard chat responses.
type openAICandidate struct {
	openaiChoice *openai.ChatCompletionChoice
}

var _ Candidate = (*openAICandidate)(nil)

func (c *openAICandidate) Parts() []Part {
	if c.openaiChoice == nil {
		return nil
	}

	var parts []Part

	if c.openaiChoice.Message.Content != "" {
		parts = append(parts, &openAIPart{content: c.openaiChoice.Message.Content})
	}

	return parts
}

func (c *openAICandidate) String() string {
	if c.openaiChoice == nil {
		return "<nil candidate>"
	}

	content := "<no content>"

	if c.openaiChoice.Message.Content != "" {
		content = c.openaiChoice.Message.Content
	}

	return fmt.Sprintf("Candidate(FinishReason: %s, Content: %q)",
		c.openaiChoice.FinishReason, content)
}

// openAIPart implements Part.
type openAIPart struct {
	content string
}

var _ Part = (*openAIPart)(nil)

func (p *openAIPart) AsText() (string, bool) {
	return p.content, p.content != ""
}

// openAIChatStreamResponse implements ChatResponse for streaming.
type openAIChatStreamResponse struct {
	streamChunk openai.ChatCompletionChunk
	accumulator openai.ChatCompletionAccumulator
	content     string
}

var _ ChatResponse = (*openAIChatStreamResponse)(nil)

func (r *openAIChatStreamResponse) UsageMetadata() any {
	if r.accumulator.Usage.TotalTokens > 0 {
		return r.accumulator.Usage
	}

	return nil
}

func (r *openAIChatStreamResponse) Candidates() []Candidate {
	if len(r.streamChunk.Choices) == 0 {
		return nil
	}

	candidates := make([]Candidate, len(r.streamChunk.Choices))

	for i, choice := range r.streamChunk.Choices {
		candidates[i] = &openAIStreamCandidate{
			streamChoice: choice,
			content:      r.content,
		}
	}

	return candidates
}

// openAIStreamCandidate implements Candidate for streamed deltas.
type openAIStreamCandidate struct {
	streamChoice openai.ChatCompletionChunkChoice
	content      string
}

var _ Candidate = (*openAIStreamCandidate)(nil)

func (c *openAIStreamCandidate) Parts() []Part {
	var parts []Part
	if c.content != "" {
		parts = append(parts, &openAIStreamPart{content: c.content})
	}

	return parts
}

func (c *openAIStreamCandidate) String() string {
	return fmt.Sprintf("StreamingCandidate(Content: %q)", c.content)
}

// openAIStreamPart implements Part.
type openAIStreamPart struct {
	content string
}

var _ Part = (*openAIStreamPart)(nil)

func (p *openAIStreamPart) AsText() (string, bool) {
	return p.content, p.content != ""
}
