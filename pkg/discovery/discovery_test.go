package discovery

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRanksFunctionAndMethodDeterministically(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "artifact", "store.go"), `package artifact

// Put stores a bounded content-addressed artifact after redaction.
func (s *Store) Put(content []byte) error { return s.redact(content) }

func unrelated() {}
`)
	tracked := []string{"artifact/store.go"}
	got, err := Discover(root, "store a bounded content-addressed artifact with redaction", tracked, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].Path != "artifact/store.go" || got[0].Symbol != "Put" || got[0].Kind != "method" {
		t.Fatalf("unexpected result: %+v", got)
	}
	again, err := Discover(root, "store a bounded content-addressed artifact with redaction", tracked, 5)
	if err != nil || len(again) != len(got) || again[0] != got[0] {
		t.Fatalf("result is not deterministic: first=%+v second=%+v err=%v", got, again, err)
	}
}

func TestDiscoverRejectsEscapesSymlinksAndInvalidBounds(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.go")
	write(t, outside, "package outside\nfunc Escape() {}\n")
	if err := os.Symlink(outside, filepath.Join(root, "linked.go")); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		intent  string
		tracked []string
		limit   int
	}{
		{"parent escape", "find escape", []string{"../outside.go"}, 5},
		{"absolute", "find escape", []string{outside}, 5},
		{"symlink", "find escape", []string{"linked.go"}, 5},
		{"empty intent", "", []string{"linked.go"}, 5},
		{"invalid limit", "find escape", []string{"linked.go"}, 0},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if _, err := Discover(root, item.intent, item.tracked, item.limit); err == nil {
				t.Fatal("expected fail-closed error")
			}
		})
	}
}

func write(t *testing.T, path, value string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}
