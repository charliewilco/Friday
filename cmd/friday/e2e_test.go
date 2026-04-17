package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charliewilco/friday/internal/commands"
)

func TestCLIInitEmbedAsk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{
					{"name": "gpt-oss:20b"},
					{"name": "nomic-embed-text"},
				},
			})
		case "/api/embed":
			var req struct {
				Input []string `json:"input"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode embed request: %v", err)
			}
			embeddings := make([][]float32, 0, len(req.Input))
			for _, input := range req.Input {
				switch {
				case strings.Contains(strings.ToLower(input), "swift"):
					embeddings = append(embeddings, []float32{1, 0})
				default:
					embeddings = append(embeddings, []float32{0, 1})
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": embeddings})
		case "/api/chat":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{\"message\":{\"content\":\"You wrote about Swift migration.\"},\"done\":true}\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("FRIDAY_OLLAMA_HOST", server.URL)

	contentDir := filepath.Join(root, "project", "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "post.md"), []byte("# Swift\nI wrote about Swift migration.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contentDir, "other.md"), []byte("# Other\nUnrelated note.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	previousWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(root, "project")
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(previousWD)

	run := func(args ...string) string {
		t.Helper()
		cmd := commands.NewRootCommand()
		var stdout bytes.Buffer
		cmd.SetOut(&stdout)
		cmd.SetErr(&stdout)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, stdout.String())
		}
		return stdout.String()
	}

	initOutput := run("init", "--yes")
	if !strings.Contains(initOutput, "Run 'friday embed'") {
		t.Fatalf("unexpected init output: %s", initOutput)
	}

	embedOutput := run("embed")
	if !strings.Contains(embedOutput, "embed complete") {
		t.Fatalf("unexpected embed output: %s", embedOutput)
	}

	askOutput := run("ask", "what did i write about swift")
	if !strings.Contains(askOutput, "You wrote about Swift migration.") {
		t.Fatalf("unexpected ask output: %s", askOutput)
	}
	if !strings.Contains(askOutput, "Sources:") {
		t.Fatalf("expected citations in ask output: %s", askOutput)
	}
}
