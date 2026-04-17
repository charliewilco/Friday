package embed

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliewilco/friday/internal/config"
	"github.com/charliewilco/friday/internal/ollama"
	"github.com/charliewilco/friday/internal/store"
	"github.com/charliewilco/friday/internal/testutil/ollamatest"
)

func TestRunInitializesMetaIsIdempotentAndRemovesDeletedFiles(t *testing.T) {
	root := t.TempDir()
	contentDir := filepath.Join(root, "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "a.md"), []byte("# Alpha\nhello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "b.md"), []byte("# Beta\nswift migration notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Loaded{
		Config:      config.Default(),
		Path:        filepath.Join(root, config.FileName),
		ProjectRoot: root,
	}
	cfg.Config.Name = "demo"
	cfg.Config.Content.Paths = []string{"content"}

	db, err := store.Open(filepath.Join(t.TempDir(), "friday.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	server := ollamatest.New(t, ollamatest.Options{EmbeddingDim: 3})
	client := ollama.New(server.URLString())
	ctx := context.Background()
	var output bytes.Buffer

	first, err := Run(ctx, cfg, db, client, Options{Stderr: &output})
	if err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if first.IndexedFiles != 2 || first.UnchangedFiles != 0 || first.RemovedFiles != 0 {
		t.Fatalf("unexpected first result: %+v", first)
	}
	if !strings.Contains(output.String(), "indexed: content/a.md") || !strings.Contains(output.String(), "indexed: content/b.md") {
		t.Fatalf("expected per-file indexing output, got %q", output.String())
	}

	for _, key := range []string{"schema_version", "project_name", "created_at", "last_embed_at", "embedding_model", "embedding_dim"} {
		if value, ok, err := db.GetMeta(ctx, key); err != nil || !ok || value == "" {
			t.Fatalf("expected meta %q, got value=%q ok=%v err=%v", key, value, ok, err)
		}
	}

	output.Reset()
	second, err := Run(ctx, cfg, db, client, Options{Stderr: &output})
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if second.IndexedFiles != 0 || second.UnchangedFiles != 2 || second.RemovedFiles != 0 {
		t.Fatalf("unexpected second result: %+v", second)
	}
	if !strings.Contains(output.String(), "unchanged: content/a.md") || !strings.Contains(output.String(), "unchanged: content/b.md") {
		t.Fatalf("expected unchanged per-file output, got %q", output.String())
	}

	if err := os.Remove(filepath.Join(contentDir, "b.md")); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	third, err := Run(ctx, cfg, db, client, Options{Stderr: &output})
	if err != nil {
		t.Fatalf("third Run() error = %v", err)
	}
	if third.IndexedFiles != 0 || third.UnchangedFiles != 1 || third.RemovedFiles != 1 {
		t.Fatalf("unexpected third result: %+v", third)
	}
	if !strings.Contains(output.String(), "removed: content/b.md") {
		t.Fatalf("expected removed output, got %q", output.String())
	}

	files, _, err := db.CountStats(ctx)
	if err != nil {
		t.Fatalf("CountStats() error = %v", err)
	}
	if files != 1 {
		t.Fatalf("expected 1 file after removal, got %d", files)
	}
}

func TestRunRejectsEmbeddingDimensionMismatch(t *testing.T) {
	root := t.TempDir()
	contentDir := filepath.Join(root, "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "a.md"), []byte("# Alpha\nhello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Loaded{
		Config:      config.Default(),
		Path:        filepath.Join(root, config.FileName),
		ProjectRoot: root,
	}
	cfg.Config.Name = "demo"
	cfg.Config.Content.Paths = []string{"content"}

	db, err := store.Open(filepath.Join(t.TempDir(), "friday.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	server3 := ollamatest.New(t, ollamatest.Options{EmbeddingDim: 3})
	if _, err := Run(ctx, cfg, db, ollama.New(server3.URLString()), Options{}); err != nil {
		t.Fatalf("initial Run() error = %v", err)
	}

	server4 := ollamatest.New(t, ollamatest.Options{EmbeddingDim: 4})
	_, err = Run(ctx, cfg, db, ollama.New(server4.URLString()), Options{})
	if err == nil || !strings.Contains(err.Error(), "embedding dimension mismatch") {
		t.Fatalf("expected dimension mismatch error, got %v", err)
	}
}
