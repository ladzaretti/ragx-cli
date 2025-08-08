package main

import (
	"os"

	"github.com/ladzaretti/ragrat/cli"
	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
)

// TODO: system prompt
// TODO1: user prompt template with context
// TODO3: embedding in sqlite

func main() {
	iostream := genericclioptions.NewDefaultIOStreams()
	rag := cli.NewDefaultRAGCommand(iostream, os.Args[1:])

	clierror.SetName("ragrat")

	_ = rag.Execute()
}
