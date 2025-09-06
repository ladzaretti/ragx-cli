package main

import (
	"os"

	"github.com/ladzaretti/ragx-cli/cli"
	"github.com/ladzaretti/ragx-cli/clierror"
	"github.com/ladzaretti/ragx-cli/genericclioptions"
)

func main() {
	clierror.SetName("ragx")

	io := genericclioptions.NewDefaultIOStreams()
	rag := cli.NewDefaultRAGCommand(io, os.Args[1:])

	if err := rag.Execute(); err != nil {
		io.Errorf("%v\n", err)
		os.Exit(1)
	}
}
