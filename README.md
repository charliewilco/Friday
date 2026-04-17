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
