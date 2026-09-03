// Package contextpack compiles model-independent repository context.
package contextpack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aitrailblazer/deltacode-engine/pkg/astslicer"
	"github.com/aitrailblazer/deltacode-engine/pkg/discovery"
	"github.com/aitrailblazer/deltacode-engine/pkg/workspace"
)

const (
	SchemaVersion         = "deltacode.context.v1"
	maximumContextBytes   = 256 << 10
	maximumContextResults = 10
)

// Slice binds a structural source excerpt to its repository-relative path.
type Slice struct {
	Path             string   `json:"path"`
	Symbol           string   `json:"symbol"`
	Language         string   `json:"language"`
	OriginalBytes    int      `json:"original_bytes"`
	SlicedBytes      int      `json:"sliced_bytes"`
	ByteReduction    float64  `json:"byte_reduction_percent"`
	SourceSHA256     string   `json:"source_sha256"`
	SlicedSHA256     string   `json:"sliced_sha256"`
	RetainedEntities []string `json:"retained_entities"`
	Source           string   `json:"source"`
}

// Packet is an immutable, read-only context bundle for any coding model.
type Packet struct {
	SchemaVersion      string                `json:"schema_version"`
	Objective          string                `json:"objective"`
	RepositoryName     string                `json:"repository_name"`
	RepositoryManifest string                `json:"repository_manifest_sha256"`
	Candidates         []discovery.Candidate `json:"candidates"`
	RelatedTests       []string              `json:"related_tests"`
	Slices             []Slice               `json:"slices"`
	OmittedCandidates  int                   `json:"omitted_candidates"`
	EstimatedTokens    int                   `json:"estimated_tokens"`
	ContextSHA256      string                `json:"context_sha256"`
}

// Build discovers symbols and creates bounded AST slices for the best unique
// files. It performs no network, model, write, edit, or command operation.
func Build(root, objective string, topK int) (*Packet, error) {
	if topK <= 0 || topK > maximumContextResults {
		return nil, fmt.Errorf("context result limit must be between 1 and %d", maximumContextResults)
	}
	canonicalRoot, err := workspace.CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	files, err := workspace.SourceFiles(canonicalRoot)
	if err != nil {
		return nil, err
	}
	manifest, err := workspace.ManifestSHA256(canonicalRoot, files)
	if err != nil {
		return nil, err
	}
	candidates, err := discovery.Discover(canonicalRoot, objective, files, topK)
	if err != nil {
		return nil, err
	}

	packet := &Packet{
		SchemaVersion:      SchemaVersion,
		Objective:          objective,
		RepositoryName:     filepath.Base(canonicalRoot),
		RepositoryManifest: manifest,
		Candidates:         candidates,
		RelatedTests:       relatedTests(files, candidates),
	}
	seenPaths := make(map[string]bool)
	totalBytes := 0
	for _, candidate := range candidates {
		if strings.HasSuffix(candidate.Path, "_test.go") {
			continue
		}
		if seenPaths[candidate.Path] {
			continue
		}
		seenPaths[candidate.Path] = true
		path, err := workspace.ResolveFile(canonicalRoot, candidate.Path)
		if err != nil {
			return nil, err
		}
		report, err := astslicer.SliceFile(path, candidate.Symbol)
		if err != nil {
			continue
		}
		if totalBytes+report.SlicedBytes > maximumContextBytes {
			packet.OmittedCandidates++
			continue
		}
		totalBytes += report.SlicedBytes
		packet.Slices = append(packet.Slices, Slice{
			Path: candidate.Path, Symbol: candidate.Symbol, Language: report.Language,
			OriginalBytes: report.OriginalBytes, SlicedBytes: report.SlicedBytes,
			ByteReduction: report.ByteReduction, SourceSHA256: report.SourceSHA256,
			SlicedSHA256: report.SlicedSHA256, RetainedEntities: report.RetainedEntities,
			Source: report.SlicedSource,
		})
	}
	packet.EstimatedTokens = (totalBytes + 3) / 4
	digest, err := packetDigest(packet)
	if err != nil {
		return nil, err
	}
	packet.ContextSHA256 = digest
	return packet, nil
}

func relatedTests(files []string, candidates []discovery.Candidate) []string {
	packageDirs := make(map[string]bool)
	for _, candidate := range candidates {
		packageDirs[filepath.ToSlash(filepath.Dir(candidate.Path))] = true
	}
	var tests []string
	for _, path := range files {
		if !strings.HasSuffix(path, "_test.go") {
			continue
		}
		if packageDirs[filepath.ToSlash(filepath.Dir(path))] {
			tests = append(tests, path)
		}
	}
	sort.Strings(tests)
	return tests
}

func packetDigest(packet *Packet) (string, error) {
	copy := *packet
	copy.ContextSHA256 = ""
	data, err := json.Marshal(copy)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
