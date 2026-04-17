package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreUpsertAndSearch(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "friday.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.UpsertFileWithChunks(ctx, FileRecord{
		Path:        "content/test.md",
		MTimeUnix:   time.Now().Unix(),
		SHA256:      "abc",
		Title:       "Test",
		LastIndexed: time.Now().Unix(),
	}, []ChunkRecord{
		{ChunkIndex: 0, HeadingPath: "Intro", Content: "hello world", TokenCount: 3},
		{ChunkIndex: 1, HeadingPath: "Details", Content: "different text", TokenCount: 3},
	}, [][]float32{
		{1, 0, 0},
		{0, 1, 0},
	}); err != nil {
		t.Fatalf("UpsertFileWithChunks() error = %v", err)
	}

	results, err := db.Search(ctx, []float32{1, 0, 0}, 1, 0)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].HeadingPath != "Intro" {
		t.Fatalf("expected Intro result, got %q", results[0].HeadingPath)
	}
}
