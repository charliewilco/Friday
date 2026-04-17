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
	if err := db.InitializeProject(ctx, "demo", time.Now()); err != nil {
		t.Fatalf("InitializeProject() error = %v", err)
	}
	if err := db.SetEmbeddingDim(ctx, 3); err != nil {
		t.Fatalf("SetEmbeddingDim() error = %v", err)
	}
	if err := db.EnsureVectorTable(ctx, 3); err != nil {
		t.Fatalf("EnsureVectorTable() error = %v", err)
	}

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

func TestStoreSearchAppliesThresholdAndOrdering(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "friday.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.InitializeProject(ctx, "demo", time.Now()); err != nil {
		t.Fatalf("InitializeProject() error = %v", err)
	}
	if err := db.SetEmbeddingDim(ctx, 2); err != nil {
		t.Fatalf("SetEmbeddingDim() error = %v", err)
	}
	if err := db.EnsureVectorTable(ctx, 2); err != nil {
		t.Fatalf("EnsureVectorTable() error = %v", err)
	}

	now := time.Now().Unix()
	if err := db.UpsertFileWithChunks(ctx, FileRecord{
		Path:        "content/a.md",
		MTimeUnix:   now,
		SHA256:      "a",
		Title:       "A",
		LastIndexed: now,
	}, []ChunkRecord{
		{ChunkIndex: 0, HeadingPath: "Alpha", Content: "alpha", TokenCount: 1},
		{ChunkIndex: 1, HeadingPath: "Alpha 2", Content: "alpha again", TokenCount: 2},
		{ChunkIndex: 2, HeadingPath: "Beta", Content: "beta", TokenCount: 1},
	}, [][]float32{
		{1, 0},
		{1, 0},
		{0, 1},
	}); err != nil {
		t.Fatalf("UpsertFileWithChunks() error = %v", err)
	}

	results, err := db.Search(ctx, []float32{1, 0}, 3, 0.99)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 high-similarity results, got %d", len(results))
	}
	if results[0].ChunkID >= results[1].ChunkID {
		t.Fatalf("expected deterministic ordering by chunk id for identical distances, got %d then %d", results[0].ChunkID, results[1].ChunkID)
	}
}
