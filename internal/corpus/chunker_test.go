package corpus

import (
	"strings"
	"testing"

	"github.com/charliewilco/friday/internal/config"
)

func TestChunkMarkdownBuildsHeadingPathsAndFrontmatterContext(t *testing.T) {
	input := []byte(`---
title: Friday
tags:
  - go
---
# Intro
This is the intro.

## Details
This is a more detailed section with enough words to survive chunking.
`)

	chunks, title, err := ChunkMarkdown("notes/test.md", input, config.Default().Content)
	if err != nil {
		t.Fatalf("ChunkMarkdown() error = %v", err)
	}

	if title != "Friday" {
		t.Fatalf("expected title Friday, got %q", title)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	if !strings.Contains(chunks[0].Content, "tags: go") {
		t.Fatalf("expected frontmatter context in chunk content, got %q", chunks[0].Content)
	}
	foundNested := false
	for _, chunk := range chunks {
		if strings.Contains(chunk.HeadingPath, "Intro > Details") {
			foundNested = true
		}
	}
	if !foundNested {
		t.Fatalf("expected nested heading path in chunks: %#v", chunks)
	}
}
