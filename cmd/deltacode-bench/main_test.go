package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunProducesRankedReceipt(t *testing.T) {
	root := t.TempDir()
	source := `package lease

// TerminateDescendants stops surviving processes after lease expiration.
func TerminateDescendants() error { return nil }
`
	writeTestFile(t, filepath.Join(root, "lease", "cleanup.go"), source)

	input := suite{
		SchemaVersion: "deltacode.benchmark-suite.v1",
		Name:          "unit",
		Cases: []benchmarkCase{{
			ID: "CASE-001", Intent: "terminate descendants after lease expiration",
			ExpectedPath: "lease/cleanup.go", ExpectedSymbol: "TerminateDescendants",
		}},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	casesPath := filepath.Join(t.TempDir(), "cases.json")
	outputPath := filepath.Join(t.TempDir(), "result.json")
	writeTestFile(t, casesPath, string(data))

	if err := run(root, casesPath, 5, outputPath); err != nil {
		t.Fatal(err)
	}
	resultData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var result report
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatal(err)
	}
	if result.RecallAt1 != 1 || result.RecallAtK != 1 || result.SymbolExactAtK != 1 {
		t.Fatalf("unexpected metrics: %+v", result)
	}
	if result.RepositoryManifest == "" || result.SuiteSHA256 == "" {
		t.Fatalf("missing evidence hashes: %+v", result)
	}
}

func TestRunRejectsEmptySuite(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
	casesPath := filepath.Join(t.TempDir(), "cases.json")
	writeTestFile(t, casesPath, `{"schema_version":"deltacode.benchmark-suite.v1"}`)
	if err := run(root, casesPath, 5, ""); err == nil {
		t.Fatal("expected empty suite to fail")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
