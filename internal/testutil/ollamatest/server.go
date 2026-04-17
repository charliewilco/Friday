package ollamatest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type Options struct {
	ChatModel             string
	EmbeddingModel        string
	ChatResponse          string
	EmbeddingDim          int
	TransientEmbedFailure int
	EmbeddingForInput     func(string) []float32
}

type Server struct {
	*httptest.Server

	mu                    sync.Mutex
	chatModel             string
	embeddingModel        string
	chatResponse          string
	embeddingDim          int
	transientEmbedFailure int
	embeddingForInput     func(string) []float32
	embedRequests         int
	chatRequests          int
}

func New(tb testing.TB, opts Options) *Server {
	tb.Helper()

	server := &Server{
		chatModel:             withDefault(opts.ChatModel, "gpt-oss:20b"),
		embeddingModel:        withDefault(opts.EmbeddingModel, "nomic-embed-text"),
		chatResponse:          withDefault(opts.ChatResponse, "hello world"),
		embeddingDim:          max(2, opts.EmbeddingDim),
		transientEmbedFailure: opts.TransientEmbedFailure,
		embeddingForInput:     opts.EmbeddingForInput,
	}

	if server.embeddingForInput == nil {
		server.embeddingForInput = server.defaultEmbedding
	}

	server.Server = httptest.NewServer(http.HandlerFunc(server.handle))
	tb.Cleanup(server.Close)
	return server
}

func (s *Server) URLString() string {
	return s.URL
}

func (s *Server) EmbedRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.embedRequests
}

func (s *Server) ChatRequests() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chatRequests
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/tags":
		_ = json.NewEncoder(w).Encode(map[string]any{
			"models": []map[string]any{
				{"name": s.chatModel},
				{"name": s.embeddingModel},
			},
		})
	case "/api/embed":
		s.handleEmbed(w, r)
	case "/api/chat":
		s.handleChat(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleEmbed(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.embedRequests++
	requestNumber := s.embedRequests
	shouldFail := requestNumber <= s.transientEmbedFailure
	s.mu.Unlock()

	if shouldFail {
		http.Error(w, fmt.Sprintf("transient embed failure %d", requestNumber), http.StatusInternalServerError)
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
		embeddings = append(embeddings, s.embeddingForInput(input))
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": embeddings})
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.chatRequests++
	response := s.chatResponse
	s.mu.Unlock()

	chunks := splitResponse(response)
	for idx, chunk := range chunks {
		done := idx == len(chunks)-1
		fmt.Fprintf(w, "{\"message\":{\"content\":%q},\"done\":%t}\n", chunk, done)
	}
}

func (s *Server) defaultEmbedding(input string) []float32 {
	vector := make([]float32, s.embeddingDim)
	lower := strings.ToLower(input)
	if len(vector) > 0 {
		vector[0] = float32(strings.Count(lower, "swift") + strings.Count(lower, "migration") + 1)
	}
	if len(vector) > 1 {
		vector[1] = float32(len(strings.Fields(lower)) + 1)
	}
	for i := 2; i < len(vector); i++ {
		vector[i] = float32(i) / 10
	}
	return vector
}

func splitResponse(response string) []string {
	if response == "" {
		return []string{""}
	}
	parts := strings.Fields(response)
	if len(parts) <= 1 {
		return []string{response}
	}
	chunks := make([]string, 0, len(parts))
	for idx, part := range parts {
		if idx == 0 {
			chunks = append(chunks, part)
			continue
		}
		chunks = append(chunks, " "+part)
	}
	return chunks
}

func withDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
