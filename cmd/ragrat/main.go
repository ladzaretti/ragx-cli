package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"

	"github.com/ladzaretti/ragrat/llm"
)

const (
	localhost = "http://localhost:11434/v1"
	// model        = "hf.co/unsloth/DeepSeek-R1-0528-Qwen3-8B-GGUF:Q4_K_XL"
	// model = "qwen3:8b"
	model = "llama3.1:8b"
	// model = "qwen2.5-coder:14b"
	// model     = "deepseek-r1:8b"
	embedding = "nomic-embed-text:v1.5"
	// systemPrompt = `
	// **Your Role and Instructions:**
	// You are an AI assistant that answers questions using text from a provided CONTEXT and by optionally calling tools.

	// - Use the CONTEXT as your primary source of truth.
	// - If the CONTEXT does not contain enough information, you may use available tools to gather the necessary data.
	// - Do not use your own pre-trained knowledge unless explicitly instructed to do so.
	// - Always respond directly to the QUESTION.
	// - If you cannot answer using the CONTEXT or tools, respond with: "The provided context does not contain the information to answer this question."
	// `
	systemPrompt = "You may use get_city_found_year to answer questions about a city's founding year."
	// systemPrompt = "You are an assistant that can call tools. When calling a function, respond using the OpenAI tool_call."
)

func main() {
	fmt.Println()
	client, err := llm.NewOpenAIClient(llm.WithBaseURL(localhost))
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()

	embedReq := llm.EmbedRequest{
		Model: embedding,
		Input: "embedding this chunk",
	}

	embedRes, err := client.Embed(ctx, embedReq)
	if err != nil {
		slog.Error("embedding error", "err", err)
	} else {
		slog.Info("embed success", "vector", embedRes.Vector())
	}

	embedBatchReq := llm.EmbedBatchRequest{
		Model: embedding,
		Input: []string{"embedding this chunk - batch 1", "embedding this chunk - batch 2"},
	}

	embedBatchRes, err := client.EmbedBatch(ctx, embedBatchReq)
	if err != nil {
		slog.Error("embedding error", "err", err)
	} else {
		slog.Info("embed success", "vectors", embedBatchRes.Vectors())
	}

	// Start a new chat with a system prompt and model
	chat := client.StartChat("You are a helpful assistant.", model)

	// Perform a single exchange
	resp, err := chat.Send(ctx, "What's the capital of France?")
	if err != nil {
		log.Fatalf("chat send failed: %v", err)
	}

	// Extract and print all candidate parts
	for _, c := range resp.Candidates() {
		for _, part := range c.Parts() {
			if text, ok := part.AsText(); ok {
				fmt.Println(">>", text)
			}
		}
	}

	stream, err := chat.SendStreaming(ctx, "Write a short poem about the sea.")
	if err != nil {
		log.Fatalf("streaming failed: %v", err)
	}
	var streamedText string
	var llmError error

	for response, err := range stream {
		if err != nil {
			slog.Error("error reading streaming LLM response")
			llmError = err
			break
		}
		if response == nil {
			break
		}

		if len(response.Candidates()) == 0 {
			slog.Error("No candidates in response")

			break
		}

		candidate := response.Candidates()[0]

		for _, part := range candidate.Parts() {
			if text, ok := part.AsText(); ok {
				streamedText += text
				fmt.Print(text)
			}
		}
	}

	if err != nil {
		slog.Error("error analyzing tool calls", "err", llmError)
	}

	resp, err = chat.Send(ctx, "can you repeat that ?")
	if err != nil {
		log.Fatalf("chat send failed: %v", err)
	}

	// Extract and print all candidate parts
	for _, c := range resp.Candidates() {
		for _, part := range c.Parts() {
			if text, ok := part.AsText(); ok {
				fmt.Println(">>", text)
			}
		}
	}

	fmt.Println() // final newline
}
