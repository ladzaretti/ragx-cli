package chatui

import (
	"context"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ladzaretti/ragrat/llm"
)

type chunk struct {
	err     error
	content string
}

type streamChunk struct {
	chunk
	ch <-chan chunk
}

func waitChunk(ch <-chan chunk) tea.Cmd {
	return func() tea.Msg {
		c, ok := <-ch
		if !ok {
			return nil
		}

		return streamChunk{chunk: c, ch: ch}
	}
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
