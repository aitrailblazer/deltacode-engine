// Package discovery ranks bounded Go symbols against natural-language intent.
// It is deterministic, read-only, and grants no execution authority.
package discovery

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/aitrailblazer/deltacode-engine/pkg/workspace"
)

const (
	maximumIntentBytes = 16 << 10
	maximumResults     = 100
)

// Candidate is a repository-root-validated symbol match.
type Candidate struct {
	Path    string `json:"path"`
	Symbol  string `json:"symbol"`
	Kind    string `json:"kind"`
	Line    int    `json:"line"`
	EndLine int    `json:"end_line"`
	Score   int    `json:"score"`
	Reason  string `json:"reason"`
}

type tokenSet map[string]int

// Discover parses bounded Go files and deterministically ranks their functions
// and methods against intent.
func Discover(root, intent string, tracked []string, limit int) ([]Candidate, error) {
	if strings.TrimSpace(intent) == "" || len(intent) > maximumIntentBytes {
		return nil, fmt.Errorf("discovery intent is invalid")
	}
	if limit <= 0 || limit > maximumResults {
		return nil, fmt.Errorf("discovery result limit is invalid")
	}
	if len(tracked) == 0 || len(tracked) > workspace.MaxFiles {
		return nil, fmt.Errorf("discovery source set is invalid")
	}
	canonicalRoot, err := workspace.CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	query := terms(intent)
	if len(query) == 0 {
		return nil, fmt.Errorf("discovery intent has no searchable terms")
	}

	var candidates []Candidate
	for _, relative := range tracked {
		if strings.ToLower(filepath.Ext(relative)) != ".go" {
			continue
		}
		path, err := workspace.ResolveFile(canonicalRoot, relative)
		if err != nil {
			return nil, err
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		set := token.NewFileSet()
		file, err := parser.ParseFile(set, path, source, parser.ParseComments)
		if err != nil {
			continue
		}
		pathTerms := terms(relative)
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name == nil {
				continue
			}
			candidate := scoreFunction(set, source, filepath.ToSlash(relative), function, query, pathTerms)
			if candidate.Score <= 0 {
				continue
			}
			if strings.HasSuffix(relative, "_test.go") {
				candidate.Score -= 250
			}
			if candidate.Score <= 0 {
				continue
			}
			candidates = append(candidates, candidate)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].Path != candidates[j].Path {
			return candidates[i].Path < candidates[j].Path
		}
		if candidates[i].Line != candidates[j].Line {
			return candidates[i].Line < candidates[j].Line
		}
		return candidates[i].Symbol < candidates[j].Symbol
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

func scoreFunction(set *token.FileSet, source []byte, path string, function *ast.FuncDecl, query, pathTerms tokenSet) Candidate {
	nameTerms := terms(function.Name.Name)
	docTerms := tokenSet{}
	if function.Doc != nil {
		docTerms = terms(function.Doc.Text())
	}
	signatureEnd := set.Position(function.Type.End()).Offset
	start := set.Position(function.Pos()).Offset
	end := set.Position(function.End()).Offset
	if start < 0 || start > len(source) {
		start = 0
	}
	if signatureEnd < start || signatureEnd > len(source) {
		signatureEnd = start
	}
	if end < signatureEnd || end > len(source) {
		end = signatureEnd
	}
	signatureTerms := terms(string(source[start:signatureEnd]))
	bodyTerms := terms(string(source[signatureEnd:end]))
	receiverTerms := tokenSet{}
	if function.Recv != nil {
		for _, field := range function.Recv.List {
			receiverTerms.addAll(terms(exprText(set, source, field.Type)))
		}
	}

	score := 0
	var reasons []string
	for term, queryFrequency := range query {
		multiplier := min(queryFrequency, 2)
		termScore := 0
		if nameTerms[term] > 0 {
			termScore += 60
		}
		if receiverTerms[term] > 0 {
			termScore += 12
		}
		if matchesTerm(pathTerms, term) {
			termScore += 45
		}
		if docTerms[term] > 0 {
			termScore += 8
		}
		if signatureTerms[term] > 0 {
			termScore += 6
		}
		if bodyTerms[term] > 0 {
			termScore += min(bodyTerms[term], 2)
		}
		if termScore > 0 {
			score += termScore * multiplier
			reasons = append(reasons, term)
		}
	}
	if covered(query, nameTerms) {
		score += 50
	}
	sort.Strings(reasons)
	if len(reasons) > 6 {
		reasons = reasons[:6]
	}
	kind := "function"
	if function.Recv != nil {
		kind = "method"
	}
	return Candidate{
		Path: path, Symbol: function.Name.Name, Kind: kind,
		Line: set.Position(function.Pos()).Line, EndLine: set.Position(function.End()).Line,
		Score: score, Reason: "matched " + strings.Join(reasons, ","),
	}
}

func exprText(set *token.FileSet, source []byte, expression ast.Expr) string {
	start := set.Position(expression.Pos()).Offset
	end := set.Position(expression.End()).Offset
	if start < 0 || end < start || end > len(source) {
		return ""
	}
	return string(source[start:end])
}

func covered(query, candidate tokenSet) bool {
	matched := 0
	for term := range query {
		if candidate[term] > 0 {
			matched++
		}
	}
	return matched >= 2 && matched == len(query)
}

var tokenPattern = regexp.MustCompile(`[a-z][a-z0-9]*`)

func terms(value string) tokenSet {
	normalized := splitIdentifier(value)
	output := tokenSet{}
	for _, term := range tokenPattern.FindAllString(strings.ToLower(normalized), -1) {
		if len(term) < 2 || stopwords[term] {
			continue
		}
		output[stem(term)]++
	}
	return output
}

func stem(term string) string {
	switch {
	case len(term) > 4 && strings.HasSuffix(term, "ies"):
		return strings.TrimSuffix(term, "ies") + "y"
	case len(term) > 5 && strings.HasSuffix(term, "ing"):
		base := strings.TrimSuffix(term, "ing")
		if len(base) > 2 && base[len(base)-1] == base[len(base)-2] {
			base = base[:len(base)-1]
		}
		return base
	case len(term) > 3 && strings.HasSuffix(term, "s") && !strings.HasSuffix(term, "ss"):
		return strings.TrimSuffix(term, "s")
	default:
		return term
	}
}

func matchesTerm(candidate tokenSet, query string) bool {
	if candidate[query] > 0 {
		return true
	}
	if len(query) < 4 {
		return false
	}
	for term := range candidate {
		if len(term) >= 4 && (strings.Contains(term, query) || strings.Contains(query, term)) {
			return true
		}
	}
	return false
}

func splitIdentifier(value string) string {
	var output strings.Builder
	var previous rune
	for index, current := range value {
		if index > 0 && unicode.IsUpper(current) && (unicode.IsLower(previous) || unicode.IsDigit(previous)) {
			output.WriteByte(' ')
		}
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			output.WriteRune(current)
		} else {
			output.WriteByte(' ')
		}
		previous = current
	}
	return output.String()
}

func (set tokenSet) addAll(other tokenSet) {
	for term, count := range other {
		set[term] += count
	}
}

var stopwords = map[string]bool{
	"a": true, "an": true, "and": true, "as": true, "at": true, "before": true,
	"for": true, "from": true, "in": true, "into": true, "is": true, "it": true,
	"of": true, "on": true, "or": true, "the": true, "through": true, "to": true,
	"using": true, "while": true, "with": true, "without": true,
}
