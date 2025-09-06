package types

import (
	"fmt"
	"slices"

	"github.com/ladzaretti/ragx-cli/llm"
)

type Provider struct {
	Client          *llm.Client
	Session         *llm.ChatSession
	AvailableModels []string
}

func (p *Provider) Supports(model string) bool { return slices.Contains(p.AvailableModels, model) }

type Providers []*Provider

func (o *Providers) ProviderFor(model string) (Provider, error) {
	for _, p := range *o {
		if p.Supports(model) {
			return *p, nil
		}
	}

	return Provider{}, fmt.Errorf("no provider found for: %q", model)
}
