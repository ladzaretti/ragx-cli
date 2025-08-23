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

	query  string
	dryRun bool
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
		selectedModel  = o.llmOptions.llmConfig.DefaultModel
		embeddingModel = o.llmOptions.embeddingConfig.EmbeddingModel
		topK           = o.llmOptions.embeddingConfig.TopK
	)

	provider, err := o.llmOptions.providers.ProviderFor(embeddingModel)
	if err != nil {
		return fmt.Errorf("provider for: %w", err)
	}

	setStatus := spinner.sendStatusWithEllipsis

	setStatus("embedding query")

	q, err := provider.Client.Embed(ctx, llm.EmbedRequest{
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

	opts := []prompt.PromptOpt{
		prompt.WithUserPromptTmpl(o.llmOptions.promptConfig.UserPromptTmpl),
	}

	p, err := prompt.BuildUserPrompt(o.query, hits, prompt.DecodeMeta, opts...)
	if err != nil {
		return errf("build user prompt: %w", err)
	}

	if o.dryRun {
		spinner.stop()
		o.Print(p + "\n")

		return nil
	}

	ch := prompt.SendStream(ctx, provider.Session, selectedModel, p)

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
		Use:     "query [flags] [path]... [--] <query>",
		Aliases: []string{"q"},
		Short:   "Embed data from paths or stdin and query the LLM",
		Long: `Embeds content from one or more paths (files or directories) or from stdin.
Directories are walked recursively.

Query is required and can be provided in the following precedence:
  1) with --query/-q
  2) after a flag terminator (--)
  3) as the last positional argument

When paths are provided, files are included if they match any -M/--match regex (full path).
If no -M filter is given, all files under the provided paths are embedded.`,
		Example: `  # embed all .go files in current dir and query via --query/-q
  ragrat query . -M '\.go$' -q "<query>"

  # embed a single file and provide query after flag terminator --
  ragrat query readme.md -- "<query>"

  # embed stdin and provide query as the last positional argument
  cat readme.md | ragrat query "<query>"

  # embed multiple paths with filter
  ragrat query docs src -M '(?i)\.(md|txt)$' -q "<query>"`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmp.Or(
				clierror.Check(o.normalizeArgs(&args, cmd.ArgsLenAtDash())),
				clierror.Check(genericclioptions.ExecuteCommand(cmd.Context(), o, args...)),
			)
		},
	}

	cmd.Flags().StringVarP(&o.query, "query", "q", "", "set query text (can also be given positionally)")
	cmd.Flags().BoolVarP(&o.dryRun, "dry-run", "", false, "print retrieval plan and the final prompt without calling the LLM")

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
