package embed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charliewilco/friday/internal/config"
	"github.com/charliewilco/friday/internal/corpus"
	"github.com/charliewilco/friday/internal/ollama"
	"github.com/charliewilco/friday/internal/store"
)

type Result struct {
	IndexedFiles   int
	UnchangedFiles int
	RemovedFiles   int
	TotalFiles     int
	TotalChunks    int
	Duration       time.Duration
}

type Options struct {
	Force   bool
	DryRun  bool
	Verbose bool
	Stderr  *os.File
}

func Run(ctx context.Context, cfg config.Loaded, db *store.Store, client *ollama.Client, opts Options) (Result, error) {
	start := time.Now()
	files, err := corpus.WalkMarkdownFiles(cfg)
	if err != nil {
		return Result{}, err
	}

	dim, err := client.EmbeddingDim(ctx, cfg.Config.Models.Embeddings)
	if err != nil {
		return Result{}, fmt.Errorf("failed probing embedding model %q: %w", cfg.Config.Models.Embeddings, err)
	}

	storedDim, ok, err := db.EmbeddingDim(ctx)
	if err != nil {
		return Result{}, err
	}
	if ok && storedDim != dim {
		return Result{}, fmt.Errorf("embedding dimension mismatch: stored=%d current=%d. Run 'friday reset' and re-embed", storedDim, dim)
	}
	if !ok && !opts.DryRun {
		if err := db.SetEmbeddingDim(ctx, dim); err != nil {
			return Result{}, err
		}
	}

	keepPaths := make(map[string]struct{}, len(files))
	result := Result{TotalFiles: len(files)}

	for _, relPath := range files {
		keepPaths[relPath] = struct{}{}
		absPath := filepath.Join(cfg.ProjectRoot, relPath)
		input, err := os.ReadFile(absPath)
		if err != nil {
			return Result{}, fmt.Errorf("failed reading %s: %w", relPath, err)
		}

		sum := sha256.Sum256(input)
		hash := hex.EncodeToString(sum[:])
		record, found, err := db.GetFileByPath(ctx, relPath)
		if err != nil {
			return Result{}, err
		}
		if found && record.SHA256 == hash && !opts.Force {
			result.UnchangedFiles++
			if opts.Verbose {
				fmt.Fprintf(opts.Stderr, "unchanged: %s\n", relPath)
			}
			continue
		}

		chunks, title, err := corpus.ChunkMarkdown(relPath, input, cfg.Config.Content)
		if err != nil {
			return Result{}, fmt.Errorf("failed chunking %s: %w", relPath, err)
		}

		if opts.DryRun {
			result.IndexedFiles++
			if opts.Verbose || !found || record.SHA256 != hash {
				fmt.Fprintf(opts.Stderr, "indexed: %s (%d chunks)\n", relPath, len(chunks))
			}
			continue
		}

		vectors, err := embedChunks(ctx, client, cfg.Config.Models.Embeddings, chunks)
		if err != nil {
			return Result{}, fmt.Errorf("failed embedding %s: %w", relPath, err)
		}

		chunkRecords := make([]store.ChunkRecord, 0, len(chunks))
		for idx, chunk := range chunks {
			chunkRecords = append(chunkRecords, store.ChunkRecord{
				ChunkIndex:  idx,
				HeadingPath: chunk.HeadingPath,
				Content:     chunk.Content,
				TokenCount:  chunk.TokenCount,
			})
		}

		info, err := os.Stat(absPath)
		if err != nil {
			return Result{}, err
		}

		err = db.UpsertFileWithChunks(ctx, store.FileRecord{
			Path:        relPath,
			MTimeUnix:   info.ModTime().Unix(),
			SHA256:      hash,
			Title:       title,
			LastIndexed: time.Now().Unix(),
		}, chunkRecords, vectors)
		if err != nil {
			return Result{}, err
		}

		result.IndexedFiles++
		fmt.Fprintf(opts.Stderr, "indexed: %s (%d chunks)\n", relPath, len(chunks))
	}

	removed, err := db.DeleteMissingFiles(ctx, keepPaths)
	if err != nil {
		return Result{}, err
	}
	result.RemovedFiles = len(removed)
	for _, path := range removed {
		fmt.Fprintf(opts.Stderr, "removed: %s\n", path)
	}

	if !opts.DryRun {
		if err := db.SetMeta(ctx, "last_embed_at", fmt.Sprint(time.Now().Unix())); err != nil {
			return Result{}, err
		}
		if err := db.SetMeta(ctx, "embedding_model", cfg.Config.Models.Embeddings); err != nil {
			return Result{}, err
		}
	}

	_, result.TotalChunks, err = db.CountStats(ctx)
	if err != nil {
		return Result{}, err
	}
	result.Duration = time.Since(start)
	return result, nil
}

func embedChunks(ctx context.Context, client *ollama.Client, model string, chunks []corpus.Chunk) ([][]float32, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	const batchSize = 32
	var vectors [][]float32
	for start := 0; start < len(chunks); start += batchSize {
		end := min(start+batchSize, len(chunks))
		inputs := make([]string, 0, end-start)
		for _, chunk := range chunks[start:end] {
			inputs = append(inputs, chunk.Content)
		}
		batch, err := client.Embed(ctx, model, inputs)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batch...)
	}
	return vectors, nil
}
