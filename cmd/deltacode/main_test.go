package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aitrailblazer/deltacode-engine/pkg/contextpack"
	"github.com/aitrailblazer/deltacode-engine/pkg/discovery"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDiscoverAndContextCommands(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "lease", "cleanup.go"), `package lease

// TerminateDescendants stops surviving processes after lease expiration.
func TerminateDescendants() error { return nil }
`)

	var output bytes.Buffer
	if err := run(t.Context(), []string{
		"discover", "-root", root, "-query", "terminate descendants after lease expiration",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var candidates []discovery.Candidate
	if err := json.Unmarshal(output.Bytes(), &candidates); err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 || candidates[0].Symbol != "TerminateDescendants" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}

	output.Reset()
	if err := run(t.Context(), []string{
		"context", "-root", root, "-objective", "terminate descendants after lease expiration",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var packet contextpack.Packet
	if err := json.Unmarshal(output.Bytes(), &packet); err != nil {
		t.Fatal(err)
	}
	if packet.RepositoryName != filepath.Base(root) || len(packet.Slices) != 1 {
		t.Fatalf("unexpected context packet: %+v", packet)
	}
	if strings.Contains(output.String(), root) {
		t.Fatalf("context packet leaked absolute repository path %q", root)
	}
}

func TestSliceRejectsPathEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.go")
	writeFixture(t, outside, "package secret\n")

	var output bytes.Buffer
	err := run(t.Context(), []string{
		"slice", "-root", root, "-path", outside,
	}, &output)
	if err == nil || !strings.Contains(err.Error(), "repository-relative") {
		t.Fatalf("slice error = %v, want repository-relative rejection", err)
	}
}

func TestHelpAndCompatibilityAliases(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "lease", "cleanup.go"), `package lease
func TerminateDescendants() error { return nil }
`)
	var output bytes.Buffer
	if err := run(t.Context(), []string{"--help"}, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "deltacode discover") {
		t.Fatalf("unexpected help: %s", output.String())
	}

	output.Reset()
	if err := run(t.Context(), []string{
		"discover", "--root", root, "--intent", "terminate descendants", "--limit", "1",
	}, &output); err != nil {
		t.Fatal(err)
	}
	var candidates []discovery.Candidate
	if err := json.Unmarshal(output.Bytes(), &candidates); err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Symbol != "TerminateDescendants" {
		t.Fatalf("unexpected alias result: %+v", candidates)
	}
}

func TestMCPListsAndCallsRootBoundTools(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, "lease", "cleanup.go"), `package lease
func TerminateDescendants() error { return nil }
`)
	server := newMCPServer(root)
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = clientSession.Close()
		_ = serverSession.Wait()
	})

	tools, err := clientSession.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 3 {
		t.Fatalf("MCP tool count = %d, want 3", len(tools.Tools))
	}
	result, err := clientSession.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "discover_symbols",
		Arguments: map[string]any{
			"query": "terminate descendants",
			"top_k": 5,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) == 0 {
		t.Fatalf("unexpected MCP result: %+v", result)
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
