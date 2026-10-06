package integration

import (
	"testing"

	"github.com/mochams/glimt/internal/syntax"
)

func TestApplyEdits(t *testing.T) {
	src := "abcdef"
	got := applyEdits(src, []edit{
		{from: 4, to: 6, text: "X"},
		{from: 1, to: 1, text: "("},
		{from: 1, to: 2, text: "B"},
		{from: 3, to: 3, text: ")"},
	})

	if want := "a(Bc)dX"; got != want {
		t.Errorf("applyEdits = %q, want %q", got, want)
	}
}

func TestParamEdits(t *testing.T) {
	tests := []struct {
		src, want string
	}{
		{"SELECT :a, :b WHERE x = :a", "SELECT $1, $2 WHERE x = $1"},
		{"SELECT 1 WHERE a IN (:ids) OR b = ANY(:ids)", "SELECT 1 WHERE a IN ($1) OR b = ANY($2)"},
		{"SELECT CASE WHEN a THEN:b END", "SELECT CASE WHEN a THEN $1 END"},
		{"SELECT :a:b", "SELECT $1 $2"},
	}

	for _, tt := range tests {
		p, err := syntax.Parse(tt.src)
		if err != nil {
			t.Fatal(err)
		}

		if got := applyEdits(tt.src, paramEdits(p)); got != tt.want {
			t.Errorf("params of %q = %q, want %q", tt.src, got, tt.want)
		}
	}
}

func TestParenEdits(t *testing.T) {
	src := "SELECT a FROM t WHERE b = 1 LIMIT 5"

	p, err := syntax.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	got := applyEdits(src, parenEdits(p, clauseSpans(p)))
	if want := "SELECT a FROM t WHERE (b = 1) LIMIT (5)"; got != want {
		t.Errorf("parens = %q, want %q", got, want)
	}
}
