package prompt_test

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/ladzaretti/ragrat/cli/prompt"
	"github.com/ladzaretti/ragrat/vecdb"
)

func TestPrompt_BuildUserPrompt(t *testing.T) {
	testCases := []struct {
		name     string
		userTmpl string
		query    string
		chunks   []vecdb.SearchResult
		metaFn   prompt.MetaFunc
		want     string
		wantErr  string
	}{
		{
			name:  "no chunks",
			query: "foo bar",
			want: `USER QUERY:
foo bar

CONTEXT:
(no relevant chunks)`,
		},
		{
			name:  "single chunk without meta func",
			query: "foo",
			chunks: []vecdb.SearchResult{
				{Content: "  bar  "},
			},
			want: `USER QUERY:
foo

CONTEXT:
----
CHUNK id=0 source=unknown
TEXT: bar
----`,
		},
		{
			name:  "multiple chunks with meta func",
			query: "foo",
			chunks: []vecdb.SearchResult{
				{Content: "bar", Meta: meta("baz", 2)},
				{Content: "qux", Meta: meta("quux", 7)},
			},
			metaFn: prompt.DecodeMeta,
			want: `USER QUERY:
foo

CONTEXT:
----
CHUNK id=2 source=baz
TEXT: bar
----
CHUNK id=7 source=quux
TEXT: qux
----`,
		},
		{
			name:     "custom template override",
			userTmpl: "Q: {{.Query}}\nN: {{len .Chunks}}\nFirst: {{(index .Chunks 0).Source}}",
			query:    "foo",
			chunks: []vecdb.SearchResult{
				{Content: "bar", Meta: meta("baz", 2)},
				{Content: "qux", Meta: meta("quux", 7)},
			},
			metaFn: prompt.DecodeMeta,
			want: `Q: foo
N: 2
First: baz`,
		},
		{
			name:     "template parse error",
			query:    "foo",
			userTmpl: "{{",
			chunks:   nil,
			wantErr:  "template parse error: template: user_prompt:1: unclosed action",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			opts := []prompt.PromptOpt{}

			if tt.userTmpl != "" {
				opts = append(opts, prompt.WithUserPromptTmpl(tt.userTmpl))
			}

			got, err := prompt.BuildUserPrompt(tt.query, tt.chunks, tt.metaFn, opts...)
			if tt.wantErr != "" {
				if err == nil || tt.wantErr != err.Error() {
					t.Fatalf("want err: %q, got: %q", tt.wantErr, got)
				}
			}

			if err != nil && tt.wantErr == "" {
				t.Fatalf("unexpected error: %v", err)
			}

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("build user query content mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func meta(source string, index int) json.RawMessage {
	b, _ := json.Marshal(struct { //nolint:errchkjson
		Source string `json:"path,omitempty"`
		Index  int    `json:"index,omitempty"`
	}{Source: source, Index: index})

	return b
}
