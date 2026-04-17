package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientEmbedModelExistsAndChat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]any{{"name": "chat-model"}, {"name": "embed-model"}},
			})
		case "/api/embed":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"embeddings": [][]float32{{1, 0, 0}},
			})
		case "/api/chat":
			fmt.Fprintln(w, `{"message":{"content":"hello"},"done":false}`)
			fmt.Fprintln(w, `{"message":{"content":" world"},"done":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL)
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
