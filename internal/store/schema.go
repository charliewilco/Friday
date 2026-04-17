package store

const schemaSQL = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS files (
	id INTEGER PRIMARY KEY,
	path TEXT UNIQUE NOT NULL,
	mtime_unix INTEGER NOT NULL,
	sha256 TEXT NOT NULL,
	title TEXT,
	last_indexed INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS files_path_idx ON files(path);

CREATE TABLE IF NOT EXISTS chunks (
	id INTEGER PRIMARY KEY,
	file_id INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
	chunk_index INTEGER NOT NULL,
	heading_path TEXT,
	content TEXT NOT NULL,
	token_count INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS chunks_file_idx ON chunks(file_id);

CREATE TABLE IF NOT EXISTS chunk_vectors (
	chunk_id INTEGER PRIMARY KEY REFERENCES chunks(id) ON DELETE CASCADE,
	embedding_json TEXT NOT NULL
);
`
