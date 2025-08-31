package main

import (
	"os"

	"github.com/ladzaretti/ragrep/cli"
	"github.com/ladzaretti/ragrep/clierror"
	"github.com/ladzaretti/ragrep/genericclioptions"
)

func main() {
	clierror.SetName("ragrep")

	io := genericclioptions.NewDefaultIOStreams()
	rag := cli.NewDefaultRAGCommand(io, os.Args[1:])

	if err := rag.Execute(); err != nil {
		io.Errorf("%v\n", err)
		os.Exit(1)
	}
}
