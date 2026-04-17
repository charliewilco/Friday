package ollama

import (
	"context"
	"strings"
	"testing"

	"github.com/charliewilco/friday/internal/testutil/ollamatest"
)

func TestClientEmbedModelExistsAndChat(t *testing.T) {
	server := ollamatest.New(t, ollamatest.Options{
		ChatModel:      "chat-model",
		EmbeddingModel: "embed-model",
		ChatResponse:   "hello world",
		EmbeddingDim:   3,
	})

	client := New(server.URLString())
	ctx := context.Background()

	ok, err := client.ModelExists(ctx, "chat-model")
	if err != nil {
		t.Fatalf("ModelExists() error = %v", err)
	}
	if !ok {
		t.Fatal("expected model to exist")
	}

	vectors, err := client.Embed(ctx, "embed-model", []string{"test"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 3 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}

	var streamed string
	output, err := client.Chat(ctx, "chat-model", []ChatMessage{{Role: "user", Content: "hello"}}, func(token string) {
		streamed += token
	})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if output != "hello world" || streamed != "hello world" {
		t.Fatalf("unexpected chat output %q / %q", output, streamed)
	}
}

func TestClientEmbedRetriesTransientFailure(t *testing.T) {
	server := ollamatest.New(t, ollamatest.Options{
		EmbeddingModel:        "embed-model",
		TransientEmbedFailure: 2,
		EmbeddingDim:          4,
	})

	client := New(server.URLString())
	vectors, err := client.Embed(context.Background(), "embed-model", []string{"retry me"})
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 4 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
	if server.EmbedRequests() != 3 {
		t.Fatalf("expected 3 embed attempts, got %d", server.EmbedRequests())
	}
}

func TestClientPingReportsHelpfulError(t *testing.T) {
	client := New("http://127.0.0.1:9")
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected Ping() to fail")
	}
	if !strings.Contains(err.Error(), "ollama serve") {
		t.Fatalf("expected helpful Ping() error, got %v", err)
	}
}
