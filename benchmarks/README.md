# Benchmarks

`cmd/deltacode-bench` runs frozen intent-to-symbol suites and emits a
content-addressed JSON receipt.

## Historical qualification

The initial design decision used a 25-case suite from a private Go repository.
The aggregate result is published in
[`private-go-summary.json`](private-go-summary.json). The task
definitions and source snapshot are not public because they expose private
repository structure.

That result is useful as a bounded engineering observation, not as a publicly
reproducible or universal comparison. A clean, commit-pinned, multi-repository
public suite is planned.

## Run your own suite

Create a JSON file:

```json
{
  "schema_version": "deltacode.benchmark-suite.v1",
  "name": "my frozen suite",
  "repository": {
    "url": "https://github.com/example/project",
    "base_commit": "full-commit-sha",
    "state": "clean"
  },
  "cases": [
    {
      "id": "CASE-001",
      "intent": "reject paths that escape through symlinks",
      "expected_path": "internal/workspace/path.go",
      "expected_symbol": "Resolve"
    }
  ]
}
```

Then run:

```bash
go run ./cmd/deltacode-bench \
  -root /path/to/checkout \
  -cases cases.json \
  -top-k 5 \
  -out result.json
```
