// Command deltacode-bench runs a frozen intent-to-symbol retrieval suite.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sort"
	"time"

	"github.com/aitrailblazer/deltacode-engine/pkg/discovery"
	"github.com/aitrailblazer/deltacode-engine/pkg/workspace"
)

type suite struct {
	SchemaVersion string `json:"schema_version"`
	Name          string `json:"name"`
	Repository    struct {
		URL        string `json:"url"`
		BaseCommit string `json:"base_commit"`
		State      string `json:"state"`
	} `json:"repository"`
	Cases []benchmarkCase `json:"cases"`
}

type benchmarkCase struct {
	ID             string `json:"id"`
	Intent         string `json:"intent"`
	ExpectedPath   string `json:"expected_path"`
	ExpectedSymbol string `json:"expected_symbol"`
}

type caseResult struct {
	Case              benchmarkCase         `json:"case"`
	DurationMicros    int64                 `json:"duration_microseconds"`
	ExpectedPathRank  int                   `json:"expected_path_rank"`
	ExpectedExactRank int                   `json:"expected_exact_rank"`
	Candidates        []discovery.Candidate `json:"candidates"`
}

type report struct {
	SchemaVersion        string       `json:"schema_version"`
	SuiteName            string       `json:"suite_name"`
	RepositoryURL        string       `json:"repository_url,omitempty"`
	RepositoryBaseCommit string       `json:"repository_base_commit,omitempty"`
	RepositoryState      string       `json:"repository_state,omitempty"`
	RepositoryManifest   string       `json:"repository_manifest_sha256"`
	GeneratedUTC         string       `json:"generated_utc"`
	GoVersion            string       `json:"go_version"`
	GOOS                 string       `json:"goos"`
	GOARCH               string       `json:"goarch"`
	CaseCount            int          `json:"case_count"`
	TopK                 int          `json:"top_k"`
	RecallAt1            float64      `json:"recall_at_1"`
	RecallAtK            float64      `json:"recall_at_k"`
	SymbolExactAtK       float64      `json:"symbol_exact_at_k"`
	P50Micros            int64        `json:"p50_microseconds"`
	P95Micros            int64        `json:"p95_microseconds"`
	SuiteSHA256          string       `json:"suite_sha256"`
	Results              []caseResult `json:"results"`
}

func main() {
	root := flag.String("root", "", "checked-out repository root")
	casesPath := flag.String("cases", "", "frozen benchmark suite JSON")
	topK := flag.Int("top-k", 5, "retrieval cutoff")
	output := flag.String("out", "", "optional report path; defaults to stdout")
	flag.Parse()

	if err := run(*root, *casesPath, *topK, *output); err != nil {
		fmt.Fprintln(os.Stderr, "deltacode-bench:", err)
		os.Exit(1)
	}
}

func run(root, casesPath string, topK int, output string) error {
	if root == "" || casesPath == "" {
		return fmt.Errorf("-root and -cases are required")
	}
	if topK <= 0 || topK > 100 {
		return fmt.Errorf("-top-k must be between 1 and 100")
	}
	data, err := os.ReadFile(casesPath)
	if err != nil {
		return fmt.Errorf("read suite: %w", err)
	}
	var input suite
	if err := json.Unmarshal(data, &input); err != nil {
		return fmt.Errorf("decode suite: %w", err)
	}
	if input.SchemaVersion == "" || len(input.Cases) == 0 {
		return fmt.Errorf("suite must declare schema_version and at least one case")
	}
	files, err := workspace.SourceFiles(root)
	if err != nil {
		return err
	}
	manifest, err := workspace.ManifestSHA256(root, files)
	if err != nil {
		return err
	}
	result := report{
		SchemaVersion: "deltacode.benchmark-report.v1", SuiteName: input.Name,
		RepositoryURL: input.Repository.URL, RepositoryBaseCommit: input.Repository.BaseCommit,
		RepositoryState: input.Repository.State, RepositoryManifest: manifest,
		GeneratedUTC: time.Now().UTC().Format(time.RFC3339),
		GoVersion:    runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		CaseCount: len(input.Cases), TopK: topK,
		SuiteSHA256: hash(data),
	}
	durations := make([]int64, 0, len(input.Cases))
	pathAt1, pathAtK, exactAtK := 0, 0, 0
	for _, item := range input.Cases {
		start := time.Now()
		candidates, err := discovery.Discover(root, item.Intent, files, topK)
		if err != nil {
			return fmt.Errorf("case %s: %w", item.ID, err)
		}
		elapsed := time.Since(start).Microseconds()
		entry := caseResult{
			Case: item, DurationMicros: elapsed, ExpectedPathRank: -1,
			ExpectedExactRank: -1, Candidates: candidates,
		}
		for index, candidate := range candidates {
			rank := index + 1
			if entry.ExpectedPathRank < 0 && candidate.Path == item.ExpectedPath {
				entry.ExpectedPathRank = rank
			}
			if entry.ExpectedExactRank < 0 && candidate.Path == item.ExpectedPath &&
				candidate.Symbol == item.ExpectedSymbol {
				entry.ExpectedExactRank = rank
			}
		}
		if entry.ExpectedPathRank == 1 {
			pathAt1++
		}
		if entry.ExpectedPathRank > 0 {
			pathAtK++
		}
		if entry.ExpectedExactRank > 0 {
			exactAtK++
		}
		durations = append(durations, elapsed)
		result.Results = append(result.Results, entry)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	count := float64(len(input.Cases))
	result.RecallAt1 = float64(pathAt1) / count
	result.RecallAtK = float64(pathAtK) / count
	result.SymbolExactAtK = float64(exactAtK) / count
	result.P50Micros = percentile(durations, 0.50)
	result.P95Micros = percentile(durations, 0.95)

	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	if output == "" {
		_, err = os.Stdout.Write(encoded)
		return err
	}
	return os.WriteFile(output, encoded, 0o644)
}

func percentile(values []int64, quantile float64) int64 {
	if len(values) == 0 {
		return 0
	}
	index := int(float64(len(values)-1) * quantile)
	return values[index]
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
