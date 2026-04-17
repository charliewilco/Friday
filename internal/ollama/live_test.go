package ollama

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestClientLiveOllama(t *testing.T) {
	if os.Getenv("FRIDAY_LIVE_OLLAMA") == "" {
		t.Skip("set FRIDAY_LIVE_OLLAMA=1 to run against a real Ollama instance")
	}

	host := withEnvDefault("FRIDAY_LIVE_OLLAMA_HOST", "http://127.0.0.1:11434")
	chatModel := withEnvDefault("FRIDAY_LIVE_OLLAMA_CHAT_MODEL", "smollm2:135m")
	embeddingModel := withEnvDefault("FRIDAY_LIVE_OLLAMA_EMBED_MODEL", "all-minilm")

	client := New(host)
	ctx := context.Background()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	for _, model := range []string{chatModel, embeddingModel} {
		ok, err := client.ModelExists(ctx, model)
		if err != nil {
			t.Fatalf("ModelExists(%q) error = %v", model, err)
		}
		if !ok {
			t.Fatalf("expected model %q to be available in live Ollama", model)
		}
	}

	vectors, err := client.Embed(ctx, embeddingModel, []string{"Friday indexes markdown", "Swift migration notes"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 2 || len(vectors[0]) == 0 {
		t.Fatalf("unexpected live embeddings shape: %#v", vectors)
	}

	var streamed strings.Builder
	output, err := client.Chat(ctx, chatModel, []ChatMessage{
		{Role: "user", Content: "Reply with exactly the word Friday."},
	}, func(token string) {
		streamed.WriteString(token)
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if strings.TrimSpace(output) == "" || strings.TrimSpace(streamed.String()) == "" {
		t.Fatalf("expected non-empty live chat output, got output=%q streamed=%q", output, streamed.String())
	}
}

func withEnvDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
