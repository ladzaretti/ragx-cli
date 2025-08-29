package chatui

import (
	"context"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ladzaretti/ragrat/cli/prompt"
	"github.com/ladzaretti/ragrat/llm"
	"github.com/ladzaretti/ragrat/types"
)

type chunk = prompt.Chunk

type streamChunk struct {
	chunk
	ch <-chan chunk
}

type ragReady struct{ ch <-chan chunk }

type ragErr struct{ err error }

func waitChunk(ch <-chan chunk) tea.Cmd {
	return func() tea.Msg {
		c, ok := <-ch
		if !ok {
			return nil
		}

		return streamChunk{chunk: c, ch: ch}
	}
}

func (m *model) startRAGCmd(ctx context.Context, query string) tea.Cmd {
	var (
		vdb      = m.vecdb
		llmModel = m.selectedModel
		config   = m.llmConfig
	)

	provider, err := m.providers.ProviderFor(m.selectedModel)
	if err != nil {
		return func() tea.Msg { return ragErr{err} }
	}

	return func() tea.Msg {
		q, err := provider.Client.Embed(ctx, llm.EmbedRequest{Input: query, Model: config.EmbeddingModel})
		if err != nil {
			return ragErr{err}
		}

		hits, err := vdb.SearchKNN(toFloat32Slice(q.Vector), config.RetrievalTopK)
		if err != nil {
			return ragErr{err}
		}

		opts := []prompt.PromptOpt{
			prompt.WithUserPromptTmpl(config.UserPromptTmpl),
		}

		p, err := prompt.BuildUserPrompt(query, hits, prompt.DecodeMeta, opts...)
		if err != nil {
			return ragErr{err}
		}

		var (
			temperature   *float64
			contextLength int
		)

		i := slices.IndexFunc(
			m.llmConfig.Models,
			func(m types.ModelConfig) bool { return m.ID == llmModel },
		)
		if i != -1 {
			temperature = m.llmConfig.Models[i].Temperature
			contextLength = m.llmConfig.Models[i].Context
		}

		req := llm.ChatCompletionRequest{
			Model:         llmModel,
			Temperature:   temperature,
			ContextLength: contextLength,
			Prompt:        p,
		}

		ch := prompt.SendStream(ctx, provider.Session, req)

		return ragReady{ch: ch}
	}
}

// tiny helper if you don’t already have it in this package:
func toFloat32Slice(src []float64) []float32 {
	dst := make([]float32, len(src))
	for i, v := range src {
		dst[i] = float32(v)
	}

	return dst
}
