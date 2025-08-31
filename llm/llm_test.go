package llm_test

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/ladzaretti/ragrep/llm"

	openai "github.com/openai/openai-go/v2"
)

type countMsgs struct{}

func (countMsgs) Count(msgs ...llm.ChatMessage) int { return len(msgs) }

func sys() llm.ChatMessage { return openai.SystemMessage("") }

func user() llm.ChatMessage { return openai.UserMessage("") }

func asst() llm.ChatMessage { return openai.AssistantMessage("") }

func TestTruncateHistory(t *testing.T) {
	type testCase struct {
		name         string
		contextLimit int
		tc           llm.TokenCounter
		history      []llm.ChatMessage
		want         []llm.ChatMessage
	}

	s, u1, a1, u2, a2, u3, a3 := sys(), user(), asst(), user(), asst(), user(), asst()
	tc := countMsgs{}

	tests := []testCase{
		{
			name:         "fits unchanged",
			history:      []llm.ChatMessage{s, u1, a1, u2, a2, u3, a3},
			tc:           tc,
			contextLimit: 7,
			want:         []llm.ChatMessage{s, u1, a1, u2, a2, u3, a3},
		},
		{
			name:         "drops oldest pair",
			history:      []llm.ChatMessage{s, u1, a1, u2, a2},
			tc:           tc,
			contextLimit: 3,
			want:         []llm.ChatMessage{s, u2, a2},
		},
		{
			name:         "drops leading non user-asst pair",
			history:      []llm.ChatMessage{s, a1, u2, a2},
			tc:           tc,
			contextLimit: 3,
			want:         []llm.ChatMessage{s, u2, a2},
		},
		{
			name:         "drops oldest pair (no system)",
			history:      []llm.ChatMessage{a1, a1, u2, a2, u3, a3},
			tc:           tc,
			contextLimit: 4,
			want:         []llm.ChatMessage{u2, a2, u3, a3},
		},
		{
			name:         "keeps only system",
			history:      []llm.ChatMessage{s, a1, u2, a2, u3, a3},
			tc:           tc,
			contextLimit: 1,
			want:         []llm.ChatMessage{s},
		},
		{
			name:         "exact limit unchanged",
			history:      []llm.ChatMessage{s, u1, a1},
			tc:           tc,
			contextLimit: 3,
			want:         []llm.ChatMessage{s, u1, a1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llm.TruncateHistory(tt.tc, tt.history, tt.contextLimit)

			if diff := cmp.Diff(tt.want, got, cmp.Comparer(compareMsgs)); diff != "" {
				t.Errorf("truncate history (-want +got):\n%s", diff)
			}
		})
	}
}

func compareMsgs(a, b llm.ChatMessage) bool {
	switch {
	case a.OfSystem != nil || b.OfSystem != nil:
		return a.OfSystem != nil && b.OfSystem != nil &&
			a.OfSystem == b.OfSystem

	case a.OfUser != nil || b.OfUser != nil:
		return a.OfUser != nil && b.OfUser != nil &&
			a.OfUser == b.OfUser

	case a.OfAssistant != nil || b.OfAssistant != nil:
		return a.OfAssistant != nil && b.OfAssistant != nil &&
			a.OfAssistant == b.OfAssistant

	default:
		return false
	}
}
