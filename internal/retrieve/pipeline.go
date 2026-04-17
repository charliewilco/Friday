package retrieve

import (
	"context"
	"fmt"
	"strings"

	"github.com/charliewilco/friday/internal/config"
	"github.com/charliewilco/friday/internal/ollama"
	"github.com/charliewilco/friday/internal/store"
)

const PromptTemplate = `You are Friday, a research assistant that answers questions using only the user's own writing. You have access to the following excerpts from their personal markdown corpus. Each excerpt is labeled with its source path and heading location.

When answering:
- Ground your answer in the excerpts. If the excerpts don't contain an answer, say so plainly.
- Quote sparingly. Prefer summarizing in the user's own framing.
- If the user is asking whether they've written about a topic before, cite the most relevant excerpts explicitly.
- Do not invent content. Do not speculate about what the user might have meant.

--- EXCERPTS ---

%s

--- QUESTION ---

%s
`

type Pipeline struct {
	Config config.Loaded
	Store  *store.Store
	Client *ollama.Client
}

type Result struct {
	Prompt   string
	Sources  []store.SearchResult
	Response string
}

func (p Pipeline) Sources(ctx context.Context, query string, topK int) ([]store.SearchResult, error) {
	vectors, err := p.Client.Embed(ctx, p.Config.Config.Models.Embeddings, []string{query})
	if err != nil {
		return nil, fmt.Errorf("failed embedding query: %w", err)
	}
	if len(vectors) == 0 {
		return nil, fmt.Errorf("embedding model returned no vector for query")
	}
	return p.Store.Search(ctx, vectors[0], topK, p.Config.Config.Retrieval.SimilarityThreshold)
}

func (p Pipeline) Ask(ctx context.Context, model, query string, topK int, onToken func(string)) (Result, error) {
	sources, err := p.Sources(ctx, query, topK)
	if err != nil {
		return Result{}, err
	}
	if len(sources) == 0 {
		return Result{Sources: nil}, nil
	}

	prompt := buildPrompt(query, sources)
	response, err := p.Client.Chat(ctx, model, []ollama.ChatMessage{
		{
			Role:    "system",
			Content: "You are Friday. Answer only from the supplied excerpts.",
		},
		{
			Role:    "user",
			Content: prompt,
		},
	}, onToken)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Prompt:   prompt,
		Sources:  sources,
		Response: response,
	}, nil
}

func buildPrompt(query string, sources []store.SearchResult) string {
	var excerpts strings.Builder
	for idx, source := range sources {
		fmt.Fprintf(&excerpts, "[%d] %s — %s\n%s\n\n", idx+1, source.Path, source.HeadingPath, source.Content)
	}
	return fmt.Sprintf(PromptTemplate, strings.TrimSpace(excerpts.String()), query)
}
