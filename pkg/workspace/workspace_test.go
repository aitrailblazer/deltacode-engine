package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSourceFilesAndResolveFileStayInsideRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "pkg", "safe.go"), "package safe\n")
	writeFile(t, filepath.Join(root, "node_modules", "ignored.go"), "package ignored\n")
	outside := filepath.Join(t.TempDir(), "outside.go")
	writeFile(t, outside, "package outside\n")
	if err := os.Symlink(outside, filepath.Join(root, "linked.go")); err != nil {
		t.Fatal(err)
	}

	files, err := SourceFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "pkg/safe.go" {
		t.Fatalf("unexpected source inventory: %v", files)
	}
	if _, err := ResolveFile(root, "pkg/safe.go"); err != nil {
		t.Fatalf("safe path rejected: %v", err)
	}
	for _, path := range []string{"../outside.go", outside, "linked.go"} {
		if _, err := ResolveFile(root, path); err == nil {
			t.Fatalf("unsafe path accepted: %q", path)
		}
	}
}

func TestManifestSHA256IsDeterministicAndContentBound(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	writeFile(t, path, "package main\n")
	first, err := ManifestSHA256(root, []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ManifestSHA256(root, []string{"main.go"})
	if err != nil || second != first {
		t.Fatalf("manifest is not deterministic: first=%s second=%s err=%v", first, second, err)
	}
	writeFile(t, path, "package changed\n")
	third, err := ManifestSHA256(root, []string{"main.go"})
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("manifest did not change with source content")
	}
}

func TestCanonicalRootAcceptsRelativeDirectory(t *testing.T) {
	root, err := CanonicalRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(root) {
		t.Fatalf("canonical root is not absolute: %q", root)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
