# Contributing

1. Open an issue describing the behavior and acceptance criteria.
2. Keep changes narrow and preserve the read-only, root-bounded trust boundary.
3. Add or update tests.
4. Run:

```bash
gofmt -w .
go test ./...
go vet ./...
go test -race ./...
go mod verify
```

Performance and retrieval claims must include a frozen case manifest, raw
machine-readable result, repository revision, and environment metadata.
