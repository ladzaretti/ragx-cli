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

	// v, err := embed.New(4, embed.WithPath("/tmp/embed.sqlite"))
	// if err != nil {
	// 	panic(err)
	// }

	// if err := v.Insert([]embed.Chunk{{Content: "blah", Vec: []float32{0.1, 0.5, 0.4, 0.5}, Meta: struct{ Meta string }{"yoyoy"}}}); err != nil {
	// 	panic(err)
	// }

	// results, err := v.SearchKNN([]float32{0.1, 0.5, 0.4, 0.5}, 2)
	// if err != nil {
	// 	panic(err)
	// }

	// for _, r := range results {
	// 	meta, _ := json.MarshalIndent(r.Meta, "", "  ")
	// 	fmt.Printf("RowID: %d\nContent: %s\nMeta: %s\nDistance: %.6f\n\n",
	// 		r.ID, r.Content, string(meta), r.Distance)
	// }

	// v.Close()

	clierror.SetName("ragrat")

	_ = rag.Execute()
}
