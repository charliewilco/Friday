# Friday

Friday is a local-first Go CLI for chatting with a personal markdown corpus through a local Ollama instance.

## Install

Friday currently uses `github.com/mattn/go-sqlite3`, so building requires cgo and a working C toolchain.

```bash
go install github.com/charliewilco/friday/cmd/friday@latest
```

## Quickstart

```bash
ollama pull gpt-oss:20b
ollama pull nomic-embed-text

cd /path/to/your/project
friday init
friday embed
friday run
```

One-shot usage:

```bash
friday ask "have I written about abstraction collapse before"
echo "summarize my 2024 writing about swift" | friday ask
```

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
