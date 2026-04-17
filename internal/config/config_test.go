package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAppliesDefaultsWarningsAndEnvOverride(t *testing.T) {
	t.Setenv("FRIDAY_OLLAMA_HOST", "http://example.test:11434")

	root := t.TempDir()
	contentDir := filepath.Join(root, "content")
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(root, FileName)
	if err := os.WriteFile(configPath, []byte(`
name = "demo"
unknown = "field"

[models]
chat = "chat-model"
embeddings = "embed-model"

[content]
paths = ["content"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if loaded.Config.Ollama.Host != "http://example.test:11434" {
		t.Fatalf("expected env override host, got %q", loaded.Config.Ollama.Host)
	}
	if loaded.Config.Content.MaxChunkTokens != 800 {
		t.Fatalf("expected default max chunk tokens, got %d", loaded.Config.Content.MaxChunkTokens)
	}
	if len(loaded.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(loaded.Warnings))
	}
}

func TestLoadRejectsMissingContentPath(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, FileName)
	if err := os.WriteFile(configPath, []byte(`
name = "demo"

[models]
chat = "chat-model"
embeddings = "embed-model"

[content]
paths = ["missing"]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(configPath); err == nil {
		t.Fatal("expected missing content path validation error")
	}
}
