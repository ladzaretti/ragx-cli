package prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ladzaretti/ragx-cli/llm"
	"github.com/ladzaretti/ragx-cli/vecdb"
)

type Chunk struct {
	Err     error
	Content string
}

// SendStream starts a streaming request and wires chunks back to [model.Update].
func SendStream(ctx context.Context, s *llm.ChatSession, req llm.ChatCompletionRequest) <-chan Chunk {
	ch := make(chan Chunk)

	go func() {
		defer close(ch)

		stream, err := s.SendStreaming(ctx, req)
		if err != nil {
			ch <- Chunk{Err: err}
			return
		}

		for res, err := range stream {
			if err != nil {
				ch <- Chunk{Err: fmt.Errorf("llm stream: %w", err)}
				return
			}

			ch <- Chunk{Content: res.Content}
		}

		ch <- Chunk{Err: io.EOF}
	}()

	return ch
}

func DecodeMeta(raw json.RawMessage) (source string, id int) {
	meta, err := vecdb.DecodeMeta(raw)
	if err != nil {
		return
	}

	source, id = meta.Source, meta.Index

	return
}
