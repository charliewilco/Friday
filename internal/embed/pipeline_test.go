package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliewilco/friday/internal/config"
	"github.com/charliewilco/friday/internal/ollama"
	"github.com/charliewilco/friday/internal/store"
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

	server := newEmbedServer(3)
	defer server.Close()
	client := ollama.New(server.URL)
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
	server3 := newEmbedServer(3)
	defer server3.Close()
	if _, err := Run(ctx, cfg, db, ollama.New(server3.URL), Options{}); err != nil {
		t.Fatalf("initial Run() error = %v", err)
	}

	server4 := newEmbedServer(4)
	defer server4.Close()
	_, err = Run(ctx, cfg, db, ollama.New(server4.URL), Options{})
	if err == nil || !strings.Contains(err.Error(), "embedding dimension mismatch") {
		t.Fatalf("expected dimension mismatch error, got %v", err)
	}
}

func newEmbedServer(dim int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			http.NotFound(w, r)
			return
		}

		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		embeddings := make([][]float32, 0, len(req.Input))
		for _, input := range req.Input {
			vector := make([]float32, dim)
			if len(vector) > 0 {
				vector[0] = float32(len(input))
			}
			if len(vector) > 1 {
				vector[1] = float32(strings.Count(strings.ToLower(input), "swift") + 1)
			}
			for i := 2; i < dim; i++ {
				vector[i] = float32(i) / 10
			}
			embeddings = append(embeddings, vector)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": embeddings})
	}))
}
