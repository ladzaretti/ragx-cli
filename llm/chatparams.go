package llm

import (
	"fmt"

	"github.com/openai/openai-go"
)

// ChatParamsBuilder defines an interface for building
// openai api style chat request parameters.
type ChatParamsBuilder interface {
	Build() ChatParams
}

// RagParamsBuilder constructs a basic request with a system prompt, context, and question.
type RagParamsBuilder struct {
	model        string
	systemPrompt string
	context      string
	question     string
}

var _ ChatParamsBuilder = &RagParamsBuilder{}

func NewRagParamsBuilder(model, systemPrompt, context, question string) *RagParamsBuilder {
	return &RagParamsBuilder{
		model:        model,
		systemPrompt: systemPrompt,
		context:      context,
		question:     question,
	}
}

// Build constructs the chat completion parameters using
// the configured system prompt, context, and question.
func (b *RagParamsBuilder) Build() ChatParams {
	finalPrompt := fmt.Sprintf(`
CONTEXT:
%s
---
QUESTION:
%s
`, b.context, b.question)

	messages := []MessageUnion{
		openai.SystemMessage(b.systemPrompt),
		openai.UserMessage(finalPrompt),
	}

	return ChatParams{
		Model:    b.model,
		Messages: messages,
	}
}
