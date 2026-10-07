package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pg "github.com/pganalyze/pg_query_go/v6"
)

// loadCorpus returns the statements in a corpus file. A .jsonl file holds
// one JSON string per line; a .sql file is split with Postgres's scanner.
func loadCorpus(path string) ([]string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // test corpus paths
	if err != nil {
		return nil, err
	}

	if strings.HasSuffix(path, ".jsonl") {
		return decodeLines(string(data))
	}

	return splitSQL(string(data)), nil
}

// decodeLines decodes one JSON string per non-empty line.
func decodeLines(data string) ([]string, error) {
	var out []string

	sc := bufio.NewScanner(strings.NewReader(data))
	sc.Buffer(nil, 1<<20)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		var s string
		if err := json.Unmarshal([]byte(line), &s); err != nil {
			return nil, fmt.Errorf("decoding %q: %w", line, err)
		}

		out = append(out, s)
	}

	return out, sc.Err()
}

// splitSQL splits a file of statements on ";" outside quotes and comments.
// A file Postgres's scanner can't read yields nothing.
func splitSQL(src string) []string {
	parts, err := pg.SplitWithScanner(src, true)
	if err != nil {
		return nil
	}

	out := parts[:0]
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}

	return out
}

// regressFiles returns the Postgres regression test files fetched by
// "make oracle-corpus", or none when they haven't been fetched.
func regressFiles() []string {
	files, _ := filepath.Glob(filepath.Join("testdata", "regress", "*.sql"))

	return files
}
