package integration

import (
	"slices"
	"testing"

	"github.com/mochams/glimt/internal/syntax"
)

func TestClauseSpans(t *testing.T) {
	tests := []struct {
		src  string
		want []string
	}{
		{"SELECT a FROM t WHERE b HAVING c LIMIT 1 OFFSET 2", []string{"b", "c", "1", "2"}},
		{"SELECT a FROM t LIMIT ALL OFFSET 5 ROWS", nil},
		{"DELETE FROM t WHERE CURRENT OF c", nil},
		{"SELECT a FROM t WHERE x IN (SELECT y FROM u WHERE z)", []string{"x IN (SELECT y FROM u WHERE z)", "z"}},
		{"WITH c AS (SELECT 1 WHERE p) UPDATE t SET a = (SELECT 2 WHERE q) WHERE r", []string{"p", "q", "r"}},
		{"INSERT INTO t SELECT 1 WHERE p ON CONFLICT DO UPDATE SET a = 1 WHERE q", []string{"p", "q"}},
		{"MERGE INTO t USING s ON p WHEN MATCHED AND q THEN DELETE", []string{"p", "q"}},
	}

	for _, tt := range tests {
		p, err := syntax.Parse(tt.src)
		if err != nil {
			t.Fatal(err)
		}

		var got []string
		for _, s := range clauseSpans(p) {
			got = append(got, tt.src[p.Tokens[s.From].Pos:p.Tokens[s.To-1].End])
		}

		slices.Sort(got)
		slices.Sort(tt.want)

		if !slices.Equal(got, tt.want) {
			t.Errorf("clauseSpans(%q) = %q, want %q", tt.src, got, tt.want)
		}
	}
}
