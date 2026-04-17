package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

type FileRecord struct {
	ID          int64
	Path        string
	MTimeUnix   int64
	SHA256      string
	Title       string
	LastIndexed int64
}

type ChunkRecord struct {
	ID          int64
	FileID      int64
	ChunkIndex  int
	HeadingPath string
	Content     string
	TokenCount  int
}

type SearchResult struct {
	ChunkID      int64
	Path         string
	Title        string
	HeadingPath  string
	Content      string
	Distance     float64
	Similarity   float64
	ChunkIndex   int
	EmbeddingLen int
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to apply schema: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) EnsureExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Friday state not found at %s. Run 'friday init' first", path)
		}
		return err
	}
	return nil
}

func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) GetMeta(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) SetEmbeddingDim(ctx context.Context, dim int) error {
	return s.SetMeta(ctx, "embedding_dim", strconv.Itoa(dim))
}

func (s *Store) EmbeddingDim(ctx context.Context) (int, bool, error) {
	value, ok, err := s.GetMeta(ctx, "embedding_dim")
	if err != nil || !ok {
		return 0, ok, err
	}
	dim, err := strconv.Atoi(value)
	if err != nil {
		return 0, false, fmt.Errorf("invalid stored embedding_dim %q: %w", value, err)
	}
	return dim, true, nil
}

func (s *Store) GetFileByPath(ctx context.Context, path string) (FileRecord, bool, error) {
	var file FileRecord
	err := s.db.QueryRowContext(ctx, `SELECT id, path, mtime_unix, sha256, COALESCE(title, ''), last_indexed FROM files WHERE path = ?`, path).
		Scan(&file.ID, &file.Path, &file.MTimeUnix, &file.SHA256, &file.Title, &file.LastIndexed)
	if errors.Is(err, sql.ErrNoRows) {
		return FileRecord{}, false, nil
	}
	if err != nil {
		return FileRecord{}, false, err
	}
	return file, true, nil
}

func (s *Store) UpsertFileWithChunks(ctx context.Context, file FileRecord, chunks []ChunkRecord, vectors [][]float32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx, `INSERT INTO files(path, mtime_unix, sha256, title, last_indexed) VALUES(?, ?, ?, ?, ?) ON CONFLICT(path) DO UPDATE SET mtime_unix = excluded.mtime_unix, sha256 = excluded.sha256, title = excluded.title, last_indexed = excluded.last_indexed`,
		file.Path, file.MTimeUnix, file.SHA256, emptyToNil(file.Title), file.LastIndexed)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil || id == 0 {
		rowErr := tx.QueryRowContext(ctx, `SELECT id FROM files WHERE path = ?`, file.Path).Scan(&id)
		if rowErr != nil {
			return rowErr
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM chunk_vectors WHERE chunk_id IN (SELECT id FROM chunks WHERE file_id = ?)`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE file_id = ?`, id); err != nil {
		return err
	}

	for idx, chunk := range chunks {
		res, err := tx.ExecContext(ctx, `INSERT INTO chunks(file_id, chunk_index, heading_path, content, token_count) VALUES(?, ?, ?, ?, ?)`,
			id, chunk.ChunkIndex, emptyToNil(chunk.HeadingPath), chunk.Content, chunk.TokenCount)
		if err != nil {
			return err
		}
		chunkID, err := res.LastInsertId()
		if err != nil {
			return err
		}

		vectorJSON, err := json.Marshal(vectors[idx])
		if err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO chunk_vectors(chunk_id, embedding_json) VALUES(?, ?)`, chunkID, string(vectorJSON)); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) DeleteMissingFiles(ctx context.Context, keepPaths map[string]struct{}) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path FROM files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var removed []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		if _, ok := keepPaths[path]; ok {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE path = ?`, path); err != nil {
			return nil, err
		}
		removed = append(removed, path)
	}
	return removed, rows.Err()
}

func (s *Store) CountStats(ctx context.Context) (files int, chunks int, err error) {
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM files`).Scan(&files); err != nil {
		return 0, 0, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM chunks`).Scan(&chunks); err != nil {
		return 0, 0, err
	}
	return files, chunks, nil
}

func (s *Store) LastEmbedAt(ctx context.Context) (time.Time, bool, error) {
	value, ok, err := s.GetMeta(ctx, "last_embed_at")
	if err != nil || !ok {
		return time.Time{}, ok, err
	}
	unix, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}, false, err
	}
	return time.Unix(unix, 0), true, nil
}

func (s *Store) Search(ctx context.Context, query []float32, limit int, threshold float64) ([]SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.chunk_index, COALESCE(c.heading_path, ''), c.content, f.path, COALESCE(f.title, ''), cv.embedding_json
		FROM chunks c
		JOIN files f ON f.id = c.file_id
		JOIN chunk_vectors cv ON cv.chunk_id = c.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]SearchResult, 0, limit)
	for rows.Next() {
		var result SearchResult
		var raw string
		if err := rows.Scan(&result.ChunkID, &result.ChunkIndex, &result.HeadingPath, &result.Content, &result.Path, &result.Title, &raw); err != nil {
			return nil, err
		}

		var vector []float32
		if err := json.Unmarshal([]byte(raw), &vector); err != nil {
			return nil, err
		}
		result.EmbeddingLen = len(vector)
		result.Distance = cosineDistance(query, vector)
		result.Similarity = 1 - result.Distance
		if threshold > 0 && result.Similarity < threshold {
			continue
		}
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Distance == results[j].Distance {
			return results[i].ChunkID < results[j].ChunkID
		}
		return results[i].Distance < results[j].Distance
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func cosineDistance(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 1
	}

	var dot, magA, magB float64
	for i := range a {
		af := float64(a[i])
		bf := float64(b[i])
		dot += af * bf
		magA += af * af
		magB += bf * bf
	}

	if magA == 0 || magB == 0 {
		return 1
	}

	sim := dot / (math.Sqrt(magA) * math.Sqrt(magB))
	return 1 - sim
}
