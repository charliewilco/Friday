# Friday

Friday is a local-first Go CLI for chatting with a personal markdown corpus through a local Ollama instance.

## Install

Friday uses `github.com/mattn/go-sqlite3` together with the official [`sqlite-vec` Go CGO bindings](https://alexgarcia.xyz/sqlite-vec/go.html), so building requires:

- Go 1.22+
- cgo enabled
- a working C toolchain

`sqlite-vec` is compiled and linked into the binary during the build, so you do not need to install a separate `.dylib`/`.so` at runtime. The first build is slower because the vector extension is compiled from source.

```bash
go install github.com/charliewilco/friday/cmd/friday@latest
```

If Friday starts but fails while opening the database with a `sqlite-vec` initialization error, rebuild with cgo enabled and confirm your local toolchain can compile CGO dependencies.

## Quickstart

```bash
ollama pull gpt-oss:20b
ollama pull nomic-embed-text

cd /path/to/your/project
friday init
friday embed
friday run
```

`friday embed` prints one line per file as it indexes, skips unchanged files on repeat runs, and removes entries for files that no longer exist.

One-shot usage:

```bash
friday ask "have I written about abstraction collapse before"
echo "summarize my 2024 writing about swift" | friday ask
```

## Commands

- `friday init` bootstraps `.friday.toml`, confirms detected content paths, and creates the project state directory under `~/.friday/<name>/`.
- `friday embed` indexes markdown files into SQLite, prints one line per file, and keeps the vector store in sync with file additions, edits, and deletions.
- `friday run` opens the interactive REPL.
- `friday ask` runs the same retrieval pipeline once for shell-friendly usage.
- `friday reset` removes the derived Friday state for the current project.

## Config

Friday reads `.friday.toml` from the current working directory:

```toml
name = "charlies-site"

[models]
chat = "gpt-oss:20b"
embeddings = "nomic-embed-text"

[content]
paths = ["content"]
extensions = [".md", ".mdx"]
ignore = []
max_chunk_tokens = 800
min_chunk_tokens = 50
include_frontmatter = true

[retrieval]
top_k = 6
similarity_threshold = 0.0

[ollama]
host = "http://localhost:11434"
```

`FRIDAY_OLLAMA_HOST` overrides `ollama.host`.

## Notes

- Friday stores derived state in `~/.friday/<project-name>/friday.db`.
- Changing to an embedding model with a different output dimension requires `friday reset` before re-embedding.
- `friday run` supports `:help`, `:stats`, `:sources <query>`, `:k <n>`, and `:q`.

## Development

Run the full local check suite with:

```bash
just check
```

The test strategy has two layers:

- Most unit and integration tests use mocked Ollama HTTP servers so they run quickly and deterministically with no external dependencies.
- A separate live smoke test can target a real Ollama instance:

```bash
FRIDAY_LIVE_OLLAMA=1 \
FRIDAY_LIVE_OLLAMA_HOST=http://127.0.0.1:11434 \
FRIDAY_LIVE_OLLAMA_CHAT_MODEL=smollm2:135m \
FRIDAY_LIVE_OLLAMA_EMBED_MODEL=all-minilm \
go test ./internal/ollama -run TestClientLiveOllama -v
```

Those model choices are intentionally small so the live CI job stays practical.
