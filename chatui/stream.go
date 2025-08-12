package chatui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ladzaretti/ragrat/cli/prompt"
	"github.com/ladzaretti/ragrat/llm"
	"github.com/ladzaretti/ragrat/vecdb"
)

type chunk struct {
	err     error
	content string
}

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
		embModel = m.embeddingModel
		topK     = 10
	)

	return func() tea.Msg {
		q, err := client.Embed(ctx, llm.EmbedRequest{Input: query, Model: embModel})
		if err != nil {
			return ragErr{err}
		}

		hits, err := vdb.SearchKNN(toFloat32Slice(q.Vector), topK)
		if err != nil {
			return ragErr{err}
		}

		prompt := prompt.BuildUserPrompt(query, hits, decodeMeta)

		ch := sendStream(ctx, chat, llmModel, prompt)

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

// sendPrompt starts a streaming request and wires chunks back to [model.Update].
func sendStream(ctx context.Context, s *llm.ChatSession, model, prompt string) <-chan chunk {
	ch := make(chan chunk)

	go func() {
		defer close(ch)

		stream, err := s.SendStreaming(ctx, model, prompt)
		if err != nil {
			ch <- chunk{err: err}
			return
		}

		for res, err := range stream {
			if err != nil {
				ch <- chunk{err: fmt.Errorf("llm stream: %w", err)}
				return
			}

			ch <- chunk{content: res.Content}
		}

		ch <- chunk{err: io.EOF}
	}()

	return ch
}

func decodeMeta(raw json.RawMessage) (source string, id int) {
	meta, err := vecdb.DecodeMeta(raw)
	if err != nil {
		return
	}

	source, id = meta.Path, meta.Index

	return
}
