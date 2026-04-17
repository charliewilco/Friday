package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	host       string
	httpClient *http.Client
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func New(host string) *Client {
	return &Client{
		host: strings.TrimRight(host, "/"),
		httpClient: &http.Client{
			Transport: http.DefaultTransport,
		},
	}
}

func (c *Client) Embed(ctx context.Context, model string, inputs []string) ([][]float32, error) {
	type request struct {
		Model string   `json:"model"`
		Input []string `json:"input"`
	}
	type response struct {
		Embeddings [][]float32 `json:"embeddings"`
	}

	var lastErr error
	for attempt, delay := range []time.Duration{0, time.Second, 3 * time.Second, 9 * time.Second} {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		resp, err := c.postJSON(timeoutContext(ctx, 30*time.Second), "/api/embed", request{
			Model: model,
			Input: inputs,
		})
		if err != nil {
			lastErr = err
			continue
		}

		var payload response
		err = decodeJSON(resp.Body, &payload)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if len(payload.Embeddings) != len(inputs) {
			return nil, fmt.Errorf("ollama embed returned %d embeddings for %d inputs", len(payload.Embeddings), len(inputs))
		}
		return payload.Embeddings, nil
	}

	return nil, fmt.Errorf("ollama embed failed after retries: %w", lastErr)
}

func (c *Client) EmbeddingDim(ctx context.Context, model string) (int, error) {
	vectors, err := c.Embed(ctx, model, []string{"probe"})
	if err != nil {
		return 0, err
	}
	if len(vectors) == 0 {
		return 0, fmt.Errorf("ollama embed returned no vectors for model %q", model)
	}
	return len(vectors[0]), nil
}

func (c *Client) Chat(ctx context.Context, model string, messages []ChatMessage, onToken func(string)) (string, error) {
	type request struct {
		Model    string        `json:"model"`
		Messages []ChatMessage `json:"messages"`
		Stream   bool          `json:"stream"`
	}
	type response struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Done bool `json:"done"`
	}

	resp, err := c.postJSON(ctx, "/api/chat", request{
		Model:    model,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)

	var output strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var event response
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return "", fmt.Errorf("failed parsing ollama chat stream: %w", err)
		}

		if delta := event.Message.Content; delta != "" {
			output.WriteString(delta)
			if onToken != nil {
				onToken(delta)
			}
		}
		if event.Done {
			return output.String(), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}
	return output.String(), nil
}

func (c *Client) Ping(ctx context.Context) error {
	resp, err := c.get(timeoutContext(ctx, 30*time.Second), "/api/tags")
	if err != nil {
		return fmt.Errorf("Ollama is not running. Start it with: ollama serve")
	}
	_ = resp.Body.Close()
	return nil
}

func (c *Client) ModelExists(ctx context.Context, model string) (bool, error) {
	type modelInfo struct {
		Name string `json:"name"`
	}
	type response struct {
		Models []modelInfo `json:"models"`
	}

	resp, err := c.get(timeoutContext(ctx, 30*time.Second), "/api/tags")
	if err != nil {
		return false, fmt.Errorf("failed to list Ollama models: %w", err)
	}
	defer resp.Body.Close()

	var payload response
	if err := decodeJSON(resp.Body, &payload); err != nil {
		return false, err
	}
	for _, item := range payload.Models {
		if item.Name == model {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) postJSON(ctx context.Context, path string, body any) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return nil, fmt.Errorf("ollama %s returned %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.host+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return nil, fmt.Errorf("ollama %s returned %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

func decodeJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("failed decoding ollama response: %w", err)
	}
	return nil
}

func timeoutContext(ctx context.Context, timeout time.Duration) context.Context {
	child, cancel := context.WithTimeout(ctx, timeout)
	go func() {
		<-child.Done()
		cancel()
	}()
	return child
}
