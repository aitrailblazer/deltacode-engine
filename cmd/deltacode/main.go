package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aitrailblazer/deltacode-engine/pkg/astslicer"
	"github.com/aitrailblazer/deltacode-engine/pkg/contextpack"
	"github.com/aitrailblazer/deltacode-engine/pkg/discovery"
	"github.com/aitrailblazer/deltacode-engine/pkg/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "0.1.0"

type discoverArgs struct {
	Query string `json:"query"`
	TopK  int    `json:"top_k,omitempty"`
}

type sliceArgs struct {
	Path         string `json:"path"`
	TargetSymbol string `json:"target_symbol,omitempty"`
}

type contextArgs struct {
	Objective string `json:"objective"`
	TopK      int    `json:"top_k,omitempty"`
}

type sliceResult struct {
	FilePath             string   `json:"file_path"`
	Language             string   `json:"language"`
	TargetSymbol         string   `json:"target_symbol,omitempty"`
	OriginalLines        int      `json:"original_lines"`
	SlicedLines          int      `json:"sliced_lines"`
	OriginalBytes        int      `json:"original_bytes"`
	SlicedBytes          int      `json:"sliced_bytes"`
	LineReductionPercent float64  `json:"line_reduction_percent"`
	ByteReductionPercent float64  `json:"byte_reduction_percent"`
	ElapsedMicroseconds  int64    `json:"elapsed_microseconds"`
	SourceSHA256         string   `json:"source_sha256"`
	SlicedSHA256         string   `json:"sliced_sha256"`
	RetainedEntities     []string `json:"retained_entities"`
	SlicedSource         string   `json:"sliced_source"`
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "deltacode:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	if len(args) == 0 {
		return runMCP(ctx, ".")
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		writeUsage(output)
		return nil
	}
	if args[0] == "mcp" {
		flags := flag.NewFlagSet("mcp", flag.ContinueOnError)
		flags.SetOutput(output)
		root := flags.String("root", ".", "fixed repository root")
		if err := flags.Parse(args[1:]); err != nil {
			if err == flag.ErrHelp {
				return nil
			}
			return err
		}
		return runMCP(ctx, *root)
	}
	switch args[0] {
	case "discover":
		return runDiscover(args[1:], output)
	case "slice":
		return runSlice(args[1:], output)
	case "context":
		return runContext(args[1:], output)
	case "version":
		_, err := fmt.Fprintln(output, version)
		return err
	default:
		return fmt.Errorf("unknown command %q; run deltacode --help", args[0])
	}
}

func runDiscover(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("discover", flag.ContinueOnError)
	flags.SetOutput(output)
	root := flags.String("root", ".", "repository root")
	query := flags.String("query", "", "natural-language coding objective")
	intent := flags.String("intent", "", "alias for -query")
	topK := flags.Int("top-k", 5, "number of ranked symbols")
	limit := flags.Int("limit", 0, "alias for -top-k")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *query == "" {
		*query = *intent
	}
	if *limit != 0 {
		*topK = *limit
	}
	files, err := workspace.SourceFiles(*root)
	if err != nil {
		return err
	}
	candidates, err := discovery.Discover(*root, *query, files, *topK)
	if err != nil {
		return err
	}
	return writeJSON(output, candidates)
}

func runSlice(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("slice", flag.ContinueOnError)
	flags.SetOutput(output)
	root := flags.String("root", ".", "repository root")
	path := flags.String("path", "", "repository-relative Go or Python source path")
	target := flags.String("target", "", "target function, method, or class")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	result, err := sliceWithinRoot(*root, *path, *target)
	if err != nil {
		return err
	}
	return writeJSON(output, result)
}

func runContext(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("context", flag.ContinueOnError)
	flags.SetOutput(output)
	root := flags.String("root", ".", "repository root")
	objective := flags.String("objective", "", "natural-language coding objective")
	query := flags.String("query", "", "alias for -objective")
	topK := flags.Int("top-k", 5, "maximum ranked symbols")
	limit := flags.Int("limit", 0, "alias for -top-k")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *objective == "" {
		*objective = *query
	}
	if *limit != 0 {
		*topK = *limit
	}
	packet, err := contextpack.Build(*root, *objective, *topK)
	if err != nil {
		return err
	}
	return writeJSON(output, packet)
}

func runMCP(ctx context.Context, root string) error {
	canonicalRoot, err := workspace.CanonicalRoot(root)
	if err != nil {
		return err
	}
	server := newMCPServer(canonicalRoot)
	return server.Run(ctx, &mcp.StdioTransport{})
}

func newMCPServer(canonicalRoot string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "deltacode-engine",
		Version: version,
	}, nil)

	addDiscoverTool(server, canonicalRoot)
	addSliceTool(server, canonicalRoot)
	addContextTool(server, canonicalRoot)
	return server
}

func addDiscoverTool(server *mcp.Server, root string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "discover_symbols",
		Description: "Deterministically rank Go functions and methods for a coding objective without embeddings, a database, or network access.",
		InputSchema: objectSchema(map[string]any{
			"query": stringProperty("Natural-language coding objective"),
			"top_k": integerProperty("Maximum ranked symbols (1-100)"),
		}, "query"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args discoverArgs) (*mcp.CallToolResult, any, error) {
		topK := args.TopK
		if topK == 0 {
			topK = 5
		}
		files, err := workspace.SourceFiles(root)
		if err != nil {
			return nil, nil, err
		}
		result, err := discovery.Discover(root, args.Query, files, topK)
		if err != nil {
			return nil, nil, err
		}
		return toolResult(result)
	})
}

func addSliceTool(server *mcp.Server, root string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "slice_symbol",
		Description: "Return a bounded AST slice for one repository-relative Go or Python file. Paths cannot escape the declared repository root.",
		InputSchema: objectSchema(map[string]any{
			"path":          stringProperty("Repository-relative source path"),
			"target_symbol": stringProperty("Optional symbol to retain in full"),
		}, "path"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args sliceArgs) (*mcp.CallToolResult, any, error) {
		result, err := sliceWithinRoot(root, args.Path, args.TargetSymbol)
		if err != nil {
			return nil, nil, err
		}
		return toolResult(result)
	})
}

func addContextTool(server *mcp.Server, root string) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "context_for_task",
		Description: "Compile a deterministic, model-independent context packet from ranked Go symbols, related tests, and bounded AST slices.",
		InputSchema: objectSchema(map[string]any{
			"objective": stringProperty("Natural-language coding objective"),
			"top_k":     integerProperty("Maximum ranked symbols (1-10)"),
		}, "objective"),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args contextArgs) (*mcp.CallToolResult, any, error) {
		topK := args.TopK
		if topK == 0 {
			topK = 5
		}
		result, err := contextpack.Build(root, args.Objective, topK)
		if err != nil {
			return nil, nil, err
		}
		return toolResult(result)
	})
}

func sliceWithinRoot(root, relative, target string) (*sliceResult, error) {
	path, err := workspace.ResolveFile(root, relative)
	if err != nil {
		return nil, err
	}
	report, err := astslicer.SliceFile(path, target)
	if err != nil {
		return nil, fmt.Errorf("slice source: %w", err)
	}
	return &sliceResult{
		FilePath: filepathForResult(relative), Language: report.Language,
		TargetSymbol: report.TargetSymbol, OriginalLines: report.OriginalLines,
		SlicedLines: report.SlicedLines, OriginalBytes: report.OriginalBytes,
		SlicedBytes: report.SlicedBytes, LineReductionPercent: report.LineReduction,
		ByteReductionPercent: report.ByteReduction, ElapsedMicroseconds: report.ElapsedMicros,
		SourceSHA256: report.SourceSHA256, SlicedSHA256: report.SlicedSHA256,
		RetainedEntities: report.RetainedEntities, SlicedSource: report.SlicedSource,
	}, nil
}

func filepathForResult(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func toolResult(value any) (*mcp.CallToolResult, any, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}, value, nil
}

func writeJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerProperty(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func writeUsage(output io.Writer) {
	fmt.Fprintln(output, `DeltaCode Engine

Usage:
  deltacode discover -root REPO -query OBJECTIVE [-top-k 5]
  deltacode slice    -root REPO -path FILE [-target SYMBOL]
  deltacode context  -root REPO -objective OBJECTIVE [-top-k 5]
  deltacode mcp      -root REPO
  deltacode version

Aliases:
  discover: -intent for -query; -limit for -top-k
  context:  -query for -objective; -limit for -top-k`)
}
