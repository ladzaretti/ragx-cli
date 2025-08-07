package main

import (
	"os"

	"github.com/ladzaretti/ragrat/cli"
	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
)

func main() {
	iostream := genericclioptions.NewDefaultIOStreams()
	rag := cli.NewDefaultRAGCommand(iostream, os.Args[1:])

	clierror.SetName("ragrat")

	_ = rag.Execute()
}
