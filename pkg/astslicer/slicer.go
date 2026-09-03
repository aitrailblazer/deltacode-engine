package astslicer

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SliceReport represents the deterministic receipt and metrics of an AST slice.
type SliceReport struct {
	FilePath         string   `json:"file_path"`
	Language         string   `json:"language"`
	TargetSymbol     string   `json:"target_symbol"`
	OriginalLines    int      `json:"original_lines"`
	SlicedLines      int      `json:"sliced_lines"`
	OriginalBytes    int      `json:"original_bytes"`
	SlicedBytes      int      `json:"sliced_bytes"`
	LineReduction    float64  `json:"line_reduction_percent"`
	ByteReduction    float64  `json:"byte_reduction_percent"`
	ElapsedMicros    int64    `json:"elapsed_microseconds"`
	SourceSHA256     string   `json:"source_sha256"`
	SlicedSHA256     string   `json:"sliced_sha256"`
	RetainedEntities []string `json:"retained_entities"`
	SlicedSource     string   `json:"sliced_source"`
}

// SliceFile detects the language by file extension and executes structural AST slicing.
func SliceFile(filePath string, targetSymbol string) (*SliceReport, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".go":
		return SliceGoFile(filePath, targetSymbol)
	case ".py":
		return SlicePythonFile(filePath, targetSymbol)
	default:
		return nil, fmt.Errorf("unsupported file extension %q (supported: .go, .py)", ext)
	}
}

// SliceGoFile parses a Go file, preserves all structs, interfaces, and signatures,
// but folds the bodies of functions that do not match the target symbol.
func SliceGoFile(filePath string, targetSymbol string) (*SliceReport, error) {
	start := time.Now()
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading source file: %w", err)
	}

	srcHash := sha256.Sum256(src)
	origBytes := len(src)
	origLines := strings.Count(string(src), "\n") + 1

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing go AST: %w", err)
	}

	var retained []string
	targetLower := strings.ToLower(targetSymbol)

	for _, decl := range node.Decls {
		fn, isFn := decl.(*ast.FuncDecl)
		if !isFn {
			if gen, isGen := decl.(*ast.GenDecl); isGen {
				for _, spec := range gen.Specs {
					if ts, isType := spec.(*ast.TypeSpec); isType {
						retained = append(retained, "type:"+ts.Name.Name)
					}
				}
			}
			continue
		}

		fnName := fn.Name.Name
		matches := targetSymbol == "" || strings.Contains(strings.ToLower(fnName), targetLower)

		if matches {
			retained = append(retained, "func_full:"+fnName)
		} else {
			startLine := fset.Position(fn.Pos()).Line
			endLine := fset.Position(fn.End()).Line
			omittedMsg := fmt.Sprintf("// ... folded %d lines (lines %d-%d) ...", endLine-startLine, startLine, endLine)

			fn.Body = &ast.BlockStmt{
				List: []ast.Stmt{
					&ast.ExprStmt{
						X: &ast.BasicLit{
							Kind:  token.STRING,
							Value: fmt.Sprintf("%q", omittedMsg),
						},
					},
				},
			}
			retained = append(retained, "func_signature_only:"+fnName)
		}
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, node); err != nil {
		return nil, fmt.Errorf("formatting sliced AST: %w", err)
	}

	slicedSrc := buf.String()
	slicedBytes := len(slicedSrc)
	slicedLines := strings.Count(slicedSrc, "\n") + 1
	slicedHash := sha256.Sum256([]byte(slicedSrc))

	elapsed := time.Since(start).Microseconds()
	lineRed := 0.0
	if origLines > 0 {
		lineRed = (1.0 - float64(slicedLines)/float64(origLines)) * 100.0
	}
	byteRed := 0.0
	if origBytes > 0 {
		byteRed = (1.0 - float64(slicedBytes)/float64(origBytes)) * 100.0
	}

	return &SliceReport{
		FilePath:         filePath,
		Language:         "go",
		TargetSymbol:     targetSymbol,
		OriginalLines:    origLines,
		SlicedLines:      slicedLines,
		OriginalBytes:    origBytes,
		SlicedBytes:      slicedBytes,
		LineReduction:    lineRed,
		ByteReduction:    byteRed,
		ElapsedMicros:    elapsed,
		SourceSHA256:     hex.EncodeToString(srcHash[:]),
		SlicedSHA256:     hex.EncodeToString(slicedHash[:]),
		RetainedEntities: retained,
		SlicedSource:     slicedSrc,
	}, nil
}

var (
	pyDefRegex   = regexp.MustCompile(`^(\s*)(?:async\s+)?def\s+([a-zA-Z0-9_]+)\s*\(`)
	pyClassRegex = regexp.MustCompile(`^(\s*)class\s+([a-zA-Z0-9_]+)`)
)

// SlicePythonFile parses a Python file, preserves classes and the target function,
// but folds non-target function bodies into minimal 1-line comments and pass statements.
func SlicePythonFile(filePath string, targetSymbol string) (*SliceReport, error) {
	start := time.Now()
	src, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading python source file: %w", err)
	}

	srcHash := sha256.Sum256(src)
	origBytes := len(src)
	origLines := strings.Count(string(src), "\n") + 1

	targetLower := strings.ToLower(targetSymbol)
	var retained []string

	scanner := bufio.NewScanner(bytes.NewReader(src))
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning python source: %w", err)
	}

	var outputLines []string
	i := 0
	for i < len(lines) {
		line := lines[i]

		if classMatch := pyClassRegex.FindStringSubmatch(line); classMatch != nil {
			className := classMatch[2]
			retained = append(retained, "class:"+className)
			outputLines = append(outputLines, line)
			i++
			continue
		}

		if defMatch := pyDefRegex.FindStringSubmatch(line); defMatch != nil {
			indent := defMatch[1]
			baseIndentLen := len(indent)
			funcName := defMatch[2]
			matches := targetSymbol == "" || strings.Contains(strings.ToLower(funcName), targetLower)

			for i < len(lines) && !strings.Contains(lines[i], "):") && !strings.HasSuffix(strings.TrimSpace(lines[i]), ":") {
				outputLines = append(outputLines, lines[i])
				i++
			}
			if i < len(lines) {
				outputLines = append(outputLines, lines[i])
				i++
			}

			bodyStart := i
			for i < len(lines) {
				trimmed := strings.TrimSpace(lines[i])
				if trimmed == "" {
					i++
					continue
				}
				curIndentLen := len(lines[i]) - len(strings.TrimLeft(lines[i], " \t"))
				if curIndentLen <= baseIndentLen {
					break
				}
				i++
			}
			bodyEnd := i

			if matches {
				retained = append(retained, "func_full:"+funcName)
				for k := bodyStart; k < bodyEnd; k++ {
					outputLines = append(outputLines, lines[k])
				}
			} else {
				retained = append(retained, "func_signature_only:"+funcName)
				bodyCount := bodyEnd - bodyStart
				foldIndent := indent + "    "
				outputLines = append(outputLines, fmt.Sprintf("%s\"\"\"... folded %d lines (lines %d-%d) ...\"\"\"", foldIndent, bodyCount, bodyStart+1, bodyEnd))
				outputLines = append(outputLines, fmt.Sprintf("%spass", foldIndent))
			}
			continue
		}

		outputLines = append(outputLines, line)
		i++
	}

	slicedSrc := strings.Join(outputLines, "\n") + "\n"
	slicedBytes := len(slicedSrc)
	slicedLines := len(outputLines)
	slicedHash := sha256.Sum256([]byte(slicedSrc))

	elapsed := time.Since(start).Microseconds()
	lineRed := 0.0
	if origLines > 0 {
		lineRed = (1.0 - float64(slicedLines)/float64(origLines)) * 100.0
	}
	byteRed := 0.0
	if origBytes > 0 {
		byteRed = (1.0 - float64(slicedBytes)/float64(origBytes)) * 100.0
	}

	return &SliceReport{
		FilePath:         filePath,
		Language:         "python",
		TargetSymbol:     targetSymbol,
		OriginalLines:    origLines,
		SlicedLines:      slicedLines,
		OriginalBytes:    origBytes,
		SlicedBytes:      slicedBytes,
		LineReduction:    lineRed,
		ByteReduction:    byteRed,
		ElapsedMicros:    elapsed,
		SourceSHA256:     hex.EncodeToString(srcHash[:]),
		SlicedSHA256:     hex.EncodeToString(slicedHash[:]),
		RetainedEntities: retained,
		SlicedSource:     slicedSrc,
	}, nil
}
