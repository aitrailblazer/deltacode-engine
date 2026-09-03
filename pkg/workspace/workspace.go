// Package workspace provides bounded, read-only repository traversal.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MaxFiles      = 10_000
	MaxSourceSize = 1 << 20
)

var skippedDirectories = map[string]bool{
	".git": true, ".hg": true, ".svn": true,
	".cache": true, ".zvec-grep": true, "DerivedData": true,
	"artifacts": true, "node_modules": true, "qa_evidence": true,
	"vendor": true, "__pycache__": true,
}

// CanonicalRoot resolves root and verifies that it names a directory.
func CanonicalRoot(root string) (string, error) {
	if !filepath.IsAbs(root) {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return "", fmt.Errorf("make workspace root absolute: %w", err)
		}
		root = absolute
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat workspace root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace root is not a directory")
	}
	return filepath.Clean(resolved), nil
}

// SourceFiles returns a deterministic repository-relative list of bounded Go
// and Python source files. Symlinks are never followed.
func SourceFiles(root string) ([]string, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	files := make([]string, 0, 256)
	err = filepath.WalkDir(canonical, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == canonical {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if skippedDirectories[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		extension := strings.ToLower(filepath.Ext(entry.Name()))
		if extension != ".go" && extension != ".py" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > MaxSourceSize {
			return nil
		}
		relative, err := filepath.Rel(canonical, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		if len(files) > MaxFiles {
			return fmt.Errorf("workspace contains more than %d bounded source files", MaxFiles)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("workspace contains no bounded Go or Python source files")
	}
	return files, nil
}

// ResolveFile validates a repository-relative regular file and returns its
// canonical path. It rejects symlinks and root escapes.
func ResolveFile(root, relative string) (string, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return "", err
	}
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("source path must be repository-relative")
	}
	candidate := filepath.Clean(filepath.Join(canonical, filepath.FromSlash(relative)))
	if !Within(canonical, candidate) {
		return "", fmt.Errorf("source path escaped workspace root")
	}
	info, err := os.Lstat(candidate)
	if err != nil {
		return "", fmt.Errorf("lstat source path: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("source path is not a regular file")
	}
	if info.Size() > MaxSourceSize {
		return "", fmt.Errorf("source file exceeds %d bytes", MaxSourceSize)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve source path: %w", err)
	}
	if !Within(canonical, resolved) {
		return "", fmt.Errorf("resolved source path escaped workspace root")
	}
	return resolved, nil
}

// ManifestSHA256 binds a deterministic source-file inventory and content.
func ManifestSHA256(root string, relativePaths []string) (string, error) {
	hash := sha256.New()
	for _, relative := range relativePaths {
		path, err := ResolveFile(root, relative)
		if err != nil {
			return "", err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(relative))
		hash.Write([]byte{0})
		hash.Write(content)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Within reports whether candidate is root or a descendant of root.
func Within(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
