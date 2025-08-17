package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ladzaretti/ragrat/cli/prompt"
	"github.com/ladzaretti/ragrat/clierror"
	"github.com/ladzaretti/ragrat/genericclioptions"
	"github.com/ladzaretti/ragrat/llm"

	"github.com/spf13/cobra"
)

type QueryOptions struct {
	*genericclioptions.StdioOptions
	llmOptions *llmOptions

	query string
}

var _ genericclioptions.CmdOptions = &QueryOptions{}

// NewQueryOptions initializes the options struct.
func NewQueryOptions(stdio *genericclioptions.StdioOptions, llmOptions *llmOptions) *QueryOptions {
	return &QueryOptions{
		StdioOptions: stdio,
		llmOptions:   llmOptions,
	}
}

func (*QueryOptions) Complete() error { return nil }

func (*QueryOptions) Validate() error { return nil }

func (o *QueryOptions) Run(ctx context.Context, args ...string) error {
	if !o.Piped && len(args) == 0 {
		return ErrNoEmbedInput
	}

	if o.Piped && len(args) > 0 {
		return ErrConflictingEmbedInputs
	}

	var in io.Reader

	if o.Piped {
		in = o.In
	}

	err := o.llmOptions.embed(ctx, o.Logger, in, o.llmOptions.embeddingREs, args...)
	if err != nil {
		return errf("embed: %w", err)
	}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	spinner := newSpinner(cancel, "")

	go spinner.run()

	defer spinner.stop()

	var (
		selectedModel  = o.llmOptions.chatConfig.Model
		embeddingModel = o.llmOptions.embeddingConfig.EmbeddingModel
		topK           = o.llmOptions.embeddingConfig.TopK
	)

	setStatus := spinner.sendStatusWithEllipsis

	setStatus("embedding query")

	q, err := o.llmOptions.client.Embed(ctx, llm.EmbedRequest{
		Input: o.query,
		Model: embeddingModel,
	})
	if err != nil {
		return err
	}

	setStatus(fmt.Sprintf("search knn (topK=%d)", topK))

	hits, err := o.llmOptions.vectordb.SearchKNN(toFloat32Slice(q.Vector), topK)
	if err != nil {
		return err
	}

	setStatus("sending to " + selectedModel)

	p := prompt.BuildUserPrompt(o.query, hits, prompt.DecodeMeta)
	ch := prompt.SendStream(ctx, o.llmOptions.session, selectedModel, p)

	if err := drainStream(ctx, ch, o.Print, setStatus, spinner.stop); err != nil {
		return fmt.Errorf("response stream: %w", err)
	}

	o.Print("\n")

	return nil
}

func drainStream(ctx context.Context, ch <-chan prompt.Chunk, printFunc func(string), setStatus func(string), stopSpinner func()) error {
	var (
		chunk         prompt.Chunk
		reasoning     = false
		reasoningDone = false
	)

	setStatus("processing")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case c, ok := <-ch:
			if !ok {
				return nil
			}

			chunk = c
		}

		if chunk.Err != nil {
			if errors.Is(chunk.Err, io.EOF) {
				return nil
			}

			return chunk.Err
		}

		switch strings.TrimSpace(chunk.Content) {
		case reasoningStartTag:
			setStatus("thinking")

			reasoning = true
		case reasoningEndTag:
			reasoning = false
			reasoningDone = true

			continue
		default:
		}

		if reasoning {
			continue
		}

		stopSpinner()

		if reasoningDone {
			reasoningDone = false

			if strings.TrimSpace(chunk.Content) == "" {
				continue
			}
		}

		printFunc(chunk.Content)
	}
}

// NewCmdQuery creates the <cmd> cobra command.
func NewCmdQuery(defaults *DefaultRAGOptions) *cobra.Command {
	o := NewQueryOptions(
		defaults.StdioOptions,
		defaults.llmOptions,
	)

	cmd := &cobra.Command{
		Use:   "query",
		Short: "",
		Long:  "",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmp.Or(
				clierror.Check(o.normalizeArgs(&args, cmd.ArgsLenAtDash())),
				clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o, args...)),
			)
		},
	}

	cmd.Flags().StringVarP(&o.query, "", "q", "", "Query to send to the LLM")

	return cmd
}

func (o *QueryOptions) normalizeArgs(args *[]string, argsBeforeDash int) error {
	norm, err := normalizeArgs(*args, argsBeforeDash, o.query)
	if err != nil {
		return err
	}

	*args, o.query = norm.args, norm.query

	return nil
}

type normalizeResult struct {
	query string
	args  []string
}

func normalizeArgs(src []string, argsBeforeDash int, q string) (res normalizeResult, _ error) {
	query := ""

	if argsBeforeDash == -1 { // no `--`
		res.args = append([]string(nil), src...)
	} else {
		res.args = src[:argsBeforeDash]
		after := src[argsBeforeDash:]

		if len(after) == 0 {
			return res, errf("missing query after --")
		}

		query = strings.TrimSpace(strings.Join(after, " "))
	}

	switch {
	case q != "":
		res.query = q
	case len(query) > 0:
		res.query = query
	case len(res.args) > 0:
		n := len(res.args)

		res.query = strings.TrimSpace(res.args[n-1])
		res.args = res.args[:n-1]
	default:
	}

	if res.query == "" {
		return res, errf("missing query: use -q or `-- <QUERY>`")
	}

	return res, nil
}
