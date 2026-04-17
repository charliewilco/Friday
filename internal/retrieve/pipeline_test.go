package retrieve

import (
	"strings"
	"testing"

	"github.com/charliewilco/friday/internal/store"
)

func TestBuildPromptIncludesOrderedSources(t *testing.T) {
	query := "what have i written about swift?"
	sources := []store.SearchResult{
		{
			Path:        "notes/swift.md",
			HeadingPath: "Swift > Migration",
			Content:     "I wrote about moving from TypeScript to Swift.",
		},
		{
			Path:        "notes/mac.md",
			HeadingPath: "macOS",
			Content:     "I also wrote about AppKit.",
		},
	}

	prompt := buildPrompt(query, sources)

	for _, want := range []string{
		"[1] notes/swift.md — Swift > Migration",
		"I wrote about moving from TypeScript to Swift.",
		"[2] notes/mac.md — macOS",
		"I also wrote about AppKit.",
		query,
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}
}
