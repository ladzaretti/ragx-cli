package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"

	"github.com/ladzaretti/ragrat/llm"
)

const (
	localhost    = "http://localhost:11434/v1"
	model        = "hf.co/unsloth/DeepSeek-R1-0528-Qwen3-8B-GGUF:Q4_K_XL"
	embedding    = "nomic-embed-text:v1.5"
	systemPrompt = `
**Your Role and Instructions:**
You are an AI assistant that answers questions using text from a provided CONTEXT. Follow these rules precisely:

1.  Your answers must be based exclusively on the information found within the CONTEXT section of the user's message.
2.  Do not use any of your pre-existing knowledge or any information from outside the provided CONTEXT.
3.  Directly address the user's QUESTION. Do not add conversational fluff or extraneous details.
4.  If the CONTEXT does not contain the information needed to answer the QUESTION, you must respond with the exact phrase: "The provided context does not contain the information to answer this question."
5.  Answer in concise, clear English.
`
)

func main() {
	client := llm.New(
		llm.WithBaseURL(localhost),
	)

	slog.Info("starting chat",
		"model", model,
		"baseUrl", localhost,
	)

	ctx := context.Background()

	models, err := client.ListModels(ctx)
	if err != nil {
		panic(err)
	}

	if !slices.Contains(models, model) {
		slog.Error("llm model not available", "model", model)
		os.Exit(1)
	}

	if !slices.Contains(models, embedding) {
		slog.Error("embedding model not available", "model", model)
		os.Exit(1)
	}

	slog.Info("available models", "models", models)

	retrieve := "Ramat Gan was founded in 1921 as a satellite city of Tel Aviv. It is home to one of the world's major diamond exchanges."
	question := "When was Tel aviv founded?"

	vector, err := client.Embedding(ctx, embedding, question)
	if err != nil {
		panic(err)
	}

	slog.Info("embedding vector created", "dims", len(vector))

	request := llm.NewRagParamsBuilder(model, systemPrompt, retrieve, question)

	if err := client.ChatStreaming(ctx, os.Stdout, request.Build()); err != nil {
		fmt.Printf("chat error: %v", err)
	}
}
