package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const FileName = ".friday.toml"

type Config struct {
	Name      string          `toml:"name"`
	Models    ModelsConfig    `toml:"models"`
	Content   ContentConfig   `toml:"content"`
	Retrieval RetrievalConfig `toml:"retrieval"`
	Ollama    OllamaConfig    `toml:"ollama"`
}

type ModelsConfig struct {
	Chat       string `toml:"chat"`
	Embeddings string `toml:"embeddings"`
}

type ContentConfig struct {
	Paths              []string `toml:"paths"`
	Extensions         []string `toml:"extensions"`
	Ignore             []string `toml:"ignore"`
	MaxChunkTokens     int      `toml:"max_chunk_tokens"`
	MinChunkTokens     int      `toml:"min_chunk_tokens"`
	IncludeFrontmatter bool     `toml:"include_frontmatter"`
}

type RetrievalConfig struct {
	TopK                int     `toml:"top_k"`
	SimilarityThreshold float64 `toml:"similarity_threshold"`
}

type OllamaConfig struct {
	Host string `toml:"host"`
}

type Loaded struct {
	Config      Config
	Path        string
	ProjectRoot string
	Warnings    []string
}

func Default() Config {
	return Config{
		Models: ModelsConfig{
			Chat:       "gpt-oss:20b",
			Embeddings: "nomic-embed-text",
		},
		Content: ContentConfig{
			Extensions:         []string{".md", ".mdx"},
			Ignore:             []string{},
			MaxChunkTokens:     800,
			MinChunkTokens:     50,
			IncludeFrontmatter: true,
		},
		Retrieval: RetrievalConfig{
			TopK:                6,
			SimilarityThreshold: 0,
		},
		Ollama: OllamaConfig{
			Host: "http://localhost:11434",
		},
	}
}

func LoadFromDir(dir string) (Loaded, error) {
	path := filepath.Join(dir, FileName)
	return Load(path)
}

func Load(path string) (Loaded, error) {
	cfg := Default()
	meta, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return Loaded{}, fmt.Errorf("config error: failed parsing %s: %w", path, err)
	}

	loaded := Loaded{
		Config:      cfg,
		Path:        path,
		ProjectRoot: filepath.Dir(path),
	}

	for _, undecoded := range meta.Undecoded() {
		loaded.Warnings = append(loaded.Warnings, fmt.Sprintf("warning: unknown config field %q in %s", undecoded.String(), path))
	}

	if host := strings.TrimSpace(os.Getenv("FRIDAY_OLLAMA_HOST")); host != "" {
		loaded.Config.Ollama.Host = host
	}

	if err := loaded.Validate(); err != nil {
		return Loaded{}, err
	}

	return loaded, nil
}

func (l Loaded) Validate() error {
	if strings.TrimSpace(l.Config.Name) == "" {
		return errors.New("config error: missing required field 'name'. Add it to .friday.toml.")
	}
	if strings.TrimSpace(l.Config.Models.Chat) == "" {
		return errors.New("config error: missing required field 'models.chat'. Add it to .friday.toml.")
	}
	if strings.TrimSpace(l.Config.Models.Embeddings) == "" {
		return errors.New("config error: missing required field 'models.embeddings'. Add it to .friday.toml.")
	}
	if len(l.Config.Content.Paths) == 0 {
		return errors.New("config error: content.paths must contain at least one path. Add a markdown directory to .friday.toml.")
	}
	if l.Config.Content.MaxChunkTokens <= 0 {
		return fmt.Errorf("config error: content.max_chunk_tokens must be > 0, got %d. Update .friday.toml.", l.Config.Content.MaxChunkTokens)
	}
	if l.Config.Content.MinChunkTokens < 0 {
		return fmt.Errorf("config error: content.min_chunk_tokens must be >= 0, got %d. Update .friday.toml.", l.Config.Content.MinChunkTokens)
	}
	if l.Config.Content.MinChunkTokens > l.Config.Content.MaxChunkTokens {
		return fmt.Errorf("config error: min_chunk_tokens (%d) cannot exceed max_chunk_tokens (%d). Update .friday.toml.", l.Config.Content.MinChunkTokens, l.Config.Content.MaxChunkTokens)
	}
	if l.Config.Retrieval.TopK <= 0 {
		return fmt.Errorf("config error: retrieval.top_k must be > 0, got %d. Update .friday.toml.", l.Config.Retrieval.TopK)
	}

	for idx, rel := range l.Config.Content.Paths {
		abs := filepath.Join(l.ProjectRoot, rel)
		info, err := os.Stat(abs)
		if err != nil {
			return fmt.Errorf("config error: content.paths[%d] %q does not exist. Fix the path or create it.", idx, rel)
		}
		if !info.IsDir() {
			return fmt.Errorf("config error: content.paths[%d] %q is not a directory. Point it at a directory of markdown files.", idx, rel)
		}
	}

	return nil
}

func (l Loaded) ResolveContentPath(path string) string {
	return filepath.Join(l.ProjectRoot, path)
}

func (l Loaded) StateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to find home directory: %w", err)
	}
	return filepath.Join(home, ".friday", l.Config.Name), nil
}

func (l Loaded) DatabasePath() (string, error) {
	dir, err := l.StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "friday.db"), nil
}
