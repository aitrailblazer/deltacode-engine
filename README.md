# DeltaCode Engine

> Deterministic task-to-symbol discovery and bounded AST context for coding agents.

[![Go](https://img.shields.io/badge/Go-1.25.13+-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-Apache--2.0-green.svg)](LICENSE)

DeltaCode turns a coding objective into a small, inspectable context packet. It
uses repository paths, Go symbols, signatures, receivers, comments, and function
bodies—not embeddings—to rank likely implementation sites. It then returns
bounded Go or Python AST slices with source hashes.

Version 0.1 deliberately has:

- no vector database;
- no embedding model or model download;
- no persistent index or indexing daemon;
- no network access in discovery and slicing;
- no code-execution authority.

The MCP transport uses the official Go MCP SDK. The retrieval and slicing core
uses the Go standard library.

## Why no vector database?

On one frozen 25-case private Go-repository suite, the structural ranker reached
92% path Recall@5. The two tested `zvec-grep` model configurations reached 40% and
48% on their best routes. That is a result for one repository and one frozen
task set—not a universal claim that AST retrieval beats vector retrieval.

The underlying repository and task definitions are private, so this historical
result is not independently reproducible. A cross-repository public benchmark
is required before broader comparative claims.

| Frozen private suite (25 cases) | Recall@1 | Recall@5 | Exact symbol@5 | p95 |
|---|---:|---:|---:|---:|
| DeltaCode structural ranker, current replay | 72% | **92%** | **80%** | 169 ms |
| `zvec-grep` + Jina, vector route | 8% | 48% | 0% | 401 ms |
| `zvec-grep` + Potion, best path route | 8% | 40% | 16% | 687 ms |

The aggregate receipt and its limitations are in
[`benchmarks/private-go-summary.json`](benchmarks/private-go-summary.json).

## Build and test

```bash
go install github.com/aitrailblazer/deltacode-engine/cmd/deltacode@v0.1.0

# Or build from a checkout:
go test ./...
go build -trimpath -o ./bin/deltacode ./cmd/deltacode
```

## CLI

```bash
# Rank likely Go symbols for a task.
./bin/deltacode discover \
  -root /path/to/repository \
  -query "reject workspace paths that escape through symlinks" \
  -top-k 5

# Slice one file. The path must remain inside the declared root.
./bin/deltacode slice \
  -root /path/to/repository \
  -path pkg/workspace/workspace.go \
  -target ResolveFile

# Produce a model-independent context packet.
./bin/deltacode context \
  -root /path/to/repository \
  -objective "reject workspace paths that escape through symlinks" \
  -top-k 5
```

## MCP

Running `deltacode` with no arguments starts the stdio MCP server bound to the
current directory. Set the root explicitly with `deltacode mcp -root /repo`.
The server exposes:

- `discover_symbols`
- `slice_symbol`
- `context_for_task`

Example configuration:

```json
{
  "mcpServers": {
    "deltacode": {
      "command": "/absolute/path/to/deltacode",
      "args": ["mcp", "-root", "/absolute/path/to/repository"]
    }
  }
}
```

Every file request is resolved under the process-configured repository root.
Absolute paths, `..` escapes, symlinks, non-regular files, oversized files, and
unsupported source types fail closed.

## Run a frozen suite

```bash
go run ./cmd/deltacode-bench \
  -root /path/to/checked-out-repository \
  -cases /path/to/cases.json \
  -top-k 5 \
  -out result.json
```

The report binds the case-file hash, source manifest hash, runtime, per-case
rankings, and aggregate metrics. Benchmark cases must be frozen before tuning.

## Scope

The discovery ranker currently indexes Go functions and methods. The slicer
supports Go and Python. DeltaCode proposes context; compilers, tests, policy
controllers, and human review retain authority over edits and execution.

The earlier Model2Vec/bbolt prototype is preserved on the
`experimental/vector` branch and is not part of the v0.1 release.

## License

Apache License 2.0.
