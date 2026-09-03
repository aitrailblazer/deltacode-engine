package astslicer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSliceGoFilePreservesTargetAndTypes(t *testing.T) {
	path := writeSource(t, "example.go", `package example

type Config struct { Retries int }

func Unrelated(cfg Config) int {
	total := 0
	for i := 0; i < cfg.Retries; i++ { total += i }
	return total
}

func Target(name string) string {
	return "hello " + name
}
`)
	report, err := SliceFile(path, "Target")
	if err != nil {
		t.Fatal(err)
	}
	if report.Language != "go" || !strings.Contains(report.SlicedSource, "type Config struct") {
		t.Fatalf("type was not preserved: %+v", report)
	}
	if !strings.Contains(report.SlicedSource, `return "hello " + name`) {
		t.Fatal("target body was not preserved")
	}
	if strings.Contains(report.SlicedSource, "total += i") {
		t.Fatal("unrelated body was not folded")
	}
	if report.SourceSHA256 == "" || report.SlicedSHA256 == "" {
		t.Fatal("missing content hashes")
	}
}

func TestSlicePythonFilePreservesTargetAndClass(t *testing.T) {
	path := writeSource(t, "example.py", `class Worker:
    def start(self):
        print("start")

    def target(self, task):
        return task

def helper():
    return 42
`)
	report, err := SliceFile(path, "target")
	if err != nil {
		t.Fatal(err)
	}
	if report.Language != "python" || !strings.Contains(report.SlicedSource, "class Worker:") {
		t.Fatalf("class was not preserved: %+v", report)
	}
	if !strings.Contains(report.SlicedSource, "return task") {
		t.Fatal("target body was not preserved")
	}
	if strings.Contains(report.SlicedSource, `print("start")`) ||
		strings.Contains(report.SlicedSource, "return 42") {
		t.Fatal("unrelated body was not folded")
	}
}

func TestSliceFileRejectsUnsupportedAndInvalidInput(t *testing.T) {
	unsupported := writeSource(t, "example.txt", "text")
	if _, err := SliceFile(unsupported, ""); err == nil {
		t.Fatal("unsupported extension accepted")
	}
	invalid := writeSource(t, "invalid.go", "package broken\nfunc {")
	if _, err := SliceFile(invalid, ""); err == nil {
		t.Fatal("invalid Go source accepted")
	}
}

func writeSource(t *testing.T, name, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
