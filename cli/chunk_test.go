package cli_test

import (
	"slices"
	"testing"

	"github.com/ladzaretti/ragrep/cli"
)

func TestChunkText(t *testing.T) {
	const (
		size    = 5
		overlap = 2 // step = 3
	)

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "basic overlap",
			input: "abcdefghij",
			want:  []string{"abcde", "defgh", "ghij"},
		},
		{
			name:  "tail shorter than size",
			input: "abcdefghi",
			want:  []string{"abcde", "defgh", "ghi"},
		},
		{
			name:  "short input",
			input: "abc",
			want:  []string{"abc"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name:  "unicode runes",
			input: "🍕🍕🍕🍕🍕🍕",
			want:  []string{"🍕🍕🍕🍕🍕", "🍕🍕🍕"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := cli.ChunkText(tt.input, size, overlap)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !slices.Equal(tt.want, got) {
				t.Errorf("want chunks: %#v, got:%#v", tt.want, got)
			}
		})
	}
}
