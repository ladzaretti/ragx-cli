package main

import (
	"os"

	"github.com/ladzaretti/ragrat/cli"
	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
)

func main() {
	clierror.SetName("ragrat")

	io := genericclioptions.NewDefaultIOStreams()
	rag := cli.NewDefaultRAGCommand(io, os.Args[1:])

	if err := rag.Execute(); err != nil {
		io.Errorf("%v\n", err)
		os.Exit(1)
	}
}
