package store

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

var sqliteVecOnce sync.Once

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
	ChunkID     int64
	Path        string
	Title       string
	HeadingPath string
	Content     string
	Distance    float64
	Similarity  float64
	ChunkIndex  int
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	enableSQLiteVec()

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)

	store := &Store{db: db}
	if err := store.validateVectorSupport(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to apply schema: %w", err)
	}

	return store, nil
}

func enableSQLiteVec() {
	sqliteVecOnce.Do(func() {
		sqlite_vec.Auto()
	})
}

func (s *Store) validateVectorSupport() error {
	var version string
	if err := s.db.QueryRow(`SELECT vec_version()`).Scan(&version); err != nil {
		return fmt.Errorf("failed to initialize sqlite-vec support: %w. Rebuild Friday with CGO enabled so sqlite-vec can be linked into the binary", err)
	}
	return nil
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

func (s *Store) InitializeProject(ctx context.Context, projectName string, now time.Time) error {
	if err := s.SetMeta(ctx, "schema_version", SchemaVersion); err != nil {
		return err
	}
	if err := s.SetMeta(ctx, "project_name", projectName); err != nil {
		return err
	}
	return s.SetMetaIfMissing(ctx, "created_at", strconv.FormatInt(now.Unix(), 10))
}

func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) SetMetaIfMissing(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO meta(key, value) VALUES(?, ?)`, key, value)
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

func (s *Store) EnsureVectorTable(ctx context.Context, dim int) error {
	var createSQL string
	err := s.db.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE name = 'chunk_vectors'`).Scan(&createSQL)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.ExecContext(ctx, vectorTableSQL(dim))
		if err != nil {
			return fmt.Errorf("failed to create sqlite-vec chunk_vectors table: %w", err)
		}
		return nil
	}
	if err != nil {
		return err
	}

	lowerSQL := strings.ToLower(createSQL)
	if !strings.Contains(lowerSQL, "using vec0") {
		return errors.New("Friday state uses a legacy embedding store. Run 'friday reset' and re-embed to rebuild with sqlite-vec")
	}
	if !strings.Contains(lowerSQL, fmt.Sprintf("float[%d]", dim)) {
		return fmt.Errorf("sqlite-vec table dimension mismatch for chunk_vectors: expected float[%d]. Run 'friday reset' and re-embed", dim)
	}
	if !strings.Contains(lowerSQL, "distance_metric=cosine") {
		return errors.New("sqlite-vec chunk_vectors table is missing cosine distance configuration. Run 'friday reset' and re-embed")
	}
	return nil
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

func (s *Store) UpsertFileWithChunks(ctx context.Context, file FileRecord, chunks []ChunkRecord, vectors [][]float32) (err error) {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("chunk/vector count mismatch: %d chunks, %d vectors", len(chunks), len(vectors))
	}

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

		vectorBlob, err := sqlite_vec.SerializeFloat32(vectors[idx])
		if err != nil {
			return fmt.Errorf("failed serializing vector for chunk %d: %w", idx, err)
		}

		if _, err := tx.ExecContext(ctx, `INSERT INTO chunk_vectors(chunk_id, embedding) VALUES(?, ?)`, chunkID, vectorBlob); err != nil {
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

	var allPaths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			_ = rows.Close()
			return nil, err
		}
		allPaths = append(allPaths, path)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	var removed []string
	for _, path := range allPaths {
		if _, ok := keepPaths[path]; ok {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE path = ?`, path); err != nil {
			return nil, err
		}
		removed = append(removed, path)
	}

	return removed, nil
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
	if limit <= 0 {
		return nil, fmt.Errorf("search limit must be > 0, got %d", limit)
	}

	queryBlob, err := sqlite_vec.SerializeFloat32(query)
	if err != nil {
		return nil, fmt.Errorf("failed serializing query vector: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		WITH knn AS (
			SELECT
				chunk_id,
				distance
			FROM chunk_vectors
			WHERE embedding MATCH ?
				AND k = ?
		)
		SELECT
			c.id,
			c.chunk_index,
			COALESCE(c.heading_path, ''),
			c.content,
			f.path,
			COALESCE(f.title, ''),
			knn.distance
		FROM knn
		JOIN chunks c ON c.id = knn.chunk_id
		JOIN files f ON f.id = c.file_id
		WHERE (? <= 0 OR (1 - knn.distance) >= ?)
		ORDER BY knn.distance ASC, c.id ASC
	`, queryBlob, limit, threshold, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]SearchResult, 0, limit)
	for rows.Next() {
		var result SearchResult
		if err := rows.Scan(&result.ChunkID, &result.ChunkIndex, &result.HeadingPath, &result.Content, &result.Path, &result.Title, &result.Distance); err != nil {
			return nil, err
		}
		result.Similarity = 1 - result.Distance
		results = append(results, result)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}
