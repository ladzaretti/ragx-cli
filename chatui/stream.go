package chatui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ladzaretti/ragrat/cli/prompt"
	"github.com/ladzaretti/ragrat/llm"
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
		client   = m.client
		chat     = m.chat
		vdb      = m.vecdb
		llmModel = m.selectedModel
		config   = m.config
	)

	return func() tea.Msg {
		q, err := client.Embed(ctx, llm.EmbedRequest{Input: query, Model: config.EmbeddingModel})
		if err != nil {
			return ragErr{err}
		}

		hits, err := vdb.SearchKNN(toFloat32Slice(q.Vector), config.TopK)
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

		ch := prompt.SendStream(ctx, chat, llmModel, p)

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
