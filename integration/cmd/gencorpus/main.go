// Command gencorpus writes every SQL statement found in glimt's own tests to
// standard output, one JSON string per line, for the oracle corpus.
//
// Usage, from the integration directory:
//
//	go run ./cmd/gencorpus ../internal > testdata/glimt.jsonl
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// starters are the words a statement can begin with.
var starters = map[string]bool{
	"SELECT": true, "WITH": true, "INSERT": true, "UPDATE": true, "DELETE": true, "MERGE": true,
	"VALUES": true, "TABLE": true, "CREATE": true, "CALL": true, "(": true,
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gencorpus <dir>")
		os.Exit(2)
	}

	found, err := collect(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)

	for _, s := range found {
		if err := enc.Encode(s); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

// collect returns the distinct statements in the _test.go files under dir, sorted.
func collect(dir string) ([]string, error) {
	seen := map[string]bool{}

	//nolint:gosec // walks the caller's own source tree
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}

		for _, s := range stringConstants(file) {
			if isStatement(s) {
				seen[s] = true
			}
		}

		return nil
	})

	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}

	slices.Sort(out)

	return out, err
}

// stringConstants returns the value of each maximal constant string
// expression in file: a literal or a concatenation of literals.
func stringConstants(file *ast.File) []string {
	var out []string

	ast.Inspect(file, func(n ast.Node) bool {
		e, ok := n.(ast.Expr)
		if !ok {
			return true
		}

		v := evaluate(e)
		if v == nil {
			return true
		}

		out = append(out, constant.StringVal(v))

		return false
	})

	return out
}

// evaluate returns the value of a constant string expression, or nil.
func evaluate(e ast.Expr) constant.Value {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			return constant.MakeFromLiteral(e.Value, e.Kind, 0)
		}
	case *ast.ParenExpr:
		return evaluate(e.X)
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return nil
		}

		x, y := evaluate(e.X), evaluate(e.Y)
		if x != nil && y != nil {
			return constant.BinaryOp(x, token.ADD, y)
		}
	}

	return nil
}

// isStatement reports whether s looks like a SQL statement. A string that
// starts with a newline is a test's expected output, not SQL.
func isStatement(s string) bool {
	if strings.HasPrefix(s, "\n") {
		return false
	}

	fields := strings.Fields(s)

	return len(fields) > 0 && (starters[strings.ToUpper(fields[0])] || strings.HasPrefix(fields[0], "("))
}
