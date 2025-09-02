package prompt

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/ladzaretti/ragx/vecdb"
)

// DefaultSystemPrompt is the base, terminal-first system prompt for a ragx CLI.
const DefaultSystemPrompt = `# Identity
You are a terminal-first RAG assistant. You answer **only** from the provided CONTEXT.

# Instructions
- **Grounding**
  - Treat each CHUNK as the sole source of truth.
  - Chunks may be **TRUNCATED**; never infer missing parts. If you can't answer from CONTEXT, reply exactly: "I don't know based on the provided context.".
  - If chunks conflict, call it out and prefer the most relevant one.

- **Citations**
  - Cite in text with independent numbers by first appearance: "[1]", "[2]", ...
  - Numbers are **per CHUNK** (not per file). Re-using a CHUNK keeps its first number.
  - If you cited at least one CHUNK, append a Sources footer mapping each number to “(chunk <id>) <full source path>”.
  - If nothing was cited, do not include a Sources section.
  - List only sources you cited.

- **Output**
  - Human-readable Markdown optimized for terminals (short paragraphs, bullets).
  - Lead with the answer, then minimal rationale.
  - Quote minimally; otherwise summarize and cite.
  - If the reply would be long, give a tight summary and offer optional sections the user can request.

- **Safety & Scope**
  - No hallucinations: do not invent APIs/flags/file contents not present in CONTEXT.
  - If code/config looks truncated, avoid guessing.
  - If the query is ambiguous, ask **one** targeted clarifying question, then stop.
  - You cannot browse the web, read external files, or rely on memory.
  - If asked to “run” something, show **how**; don't claim you executed it.

# I/O Format
You will receive input like:

USER QUERY:
<question or command here>

CONTEXT:
----
CHUNK id=<id1> source=<path-or-url>
TEXT: <chunk text...>
----
CHUNK id=<id2> source=<path-or-url>
TEXT: <chunk text...>
----
(more chunks…)

# Examples

<user_query id="example-1">
USER QUERY:
How do I start the server?
</user_query>

<context id="example-1">
CONTEXT:
----
CHUNK id=2 source=README.md
TEXT: Run 'srv start --port 8080' to start the HTTP server. Requires Go 1.22+.
----
</context>
<assistant_response id="example-1">
- Start the server with: "srv start --port 8080". Requires Go 1.22+. [1]

Sources:
[1] (chunk 2) README.md
</assistant_response>

<user_query id="example-2">
USER QUERY:
What's the roadmap?
</user_query>

<context id="example-2">
CONTEXT:
(no relevant chunks)
</context>
<assistant_response id="example-2">
I don't know based on the provided context.
If you add the roadmap document or link, I can summarize it.
</assistant_response>
`

const DefaultUserPromptTmpl = `USER QUERY:
{{.Query}}

CONTEXT:
{{- if .Chunks }}
{{- range .Chunks }}
----
CHUNK id={{.ID}} source={{.Source}}
TEXT: {{.Content}}
{{- end }}
----
{{- else }}
(no relevant chunks)
{{- end }}`

type promptConfig struct {
	userTmpl string
}

type chunkView struct {
	ID      int
	Source  string
	Content string
}
type tmplData struct {
	Query  string
	Chunks []chunkView
}

type MetaFunc func(raw json.RawMessage) (source string, id int)

type PromptOpt func(*promptConfig)

func WithUserPromptTmpl(tmpl string) PromptOpt {
	return func(c *promptConfig) {
		c.userTmpl = tmpl
	}
}

// BuildUserPrompt renders the user prompt template.
// If no template is provided, [DefaultUserPromptTmpl] is used.
func BuildUserPrompt(query string, chunks []vecdb.SearchResult, metaFn MetaFunc, opts ...PromptOpt) (string, error) {
	c := &promptConfig{
		userTmpl: DefaultUserPromptTmpl,
	}

	for _, o := range opts {
		o(c)
	}

	td := tmplData{
		Query:  strings.TrimSpace(query),
		Chunks: make([]chunkView, 0, len(chunks)),
	}

	for i, ch := range chunks {
		src, id := "", 0
		if metaFn != nil {
			src, id = metaFn(ch.Meta)
		}

		src = cmp.Or(src, "unknown")
		id = cmp.Or(id, i)

		td.Chunks = append(td.Chunks, chunkView{
			ID:      id,
			Source:  src,
			Content: strings.TrimSpace(ch.Content),
		})
	}

	t, err := template.New("user_prompt").Parse(c.userTmpl)
	if err != nil {
		return "", fmt.Errorf("template parse error: %v", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, td); err != nil {
		return "", fmt.Errorf("template execution error: %v", err)
	}

	return buf.String(), nil
}
