package contextpack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildProducesDeterministicRootBoundContext(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "lease", "cleanup.go"), `package lease

// TerminateDescendants stops surviving processes after lease expiration.
func TerminateDescendants() error { return nil }

func unrelated() {}
`)
	write(t, filepath.Join(root, "lease", "cleanup_test.go"), `package lease
func TestTerminateDescendants() {}
`)

	first, err := Build(root, "terminate descendants after lease expiration", 5)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(root, "terminate descendants after lease expiration", 5)
	if err != nil {
		t.Fatal(err)
	}
	if first.ContextSHA256 != second.ContextSHA256 {
		t.Fatalf("context is not deterministic: %s != %s", first.ContextSHA256, second.ContextSHA256)
	}
	if first.RepositoryName != filepath.Base(root) {
		t.Fatalf("repository name = %q, want %q", first.RepositoryName, filepath.Base(root))
	}
	if len(first.Candidates) == 0 || first.Candidates[0].Symbol != "TerminateDescendants" {
		t.Fatalf("unexpected candidates: %+v", first.Candidates)
	}
	if len(first.Slices) != 1 || first.Slices[0].Path != "lease/cleanup.go" {
		t.Fatalf("unexpected slices: %+v", first.Slices)
	}
	if len(first.RelatedTests) != 1 || first.RelatedTests[0] != "lease/cleanup_test.go" {
		t.Fatalf("unexpected related tests: %+v", first.RelatedTests)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
