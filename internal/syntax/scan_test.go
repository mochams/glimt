package syntax

import (
	"strings"
	"testing"
)

// scanSQL runs scanExpr over src from its first token.
func scanSQL(tb testing.TB, src string, stop stopSet) (*parser, Expr, error) {
	tb.Helper()

	p := newParser(src, mustLex(tb, src))
	e, err := p.scanExpr(stop)

	return p, e, err
}

// spanText returns the source text a span covers.
func spanText(p *parser, s Span) string {
	if s.Empty() {
		return ""
	}

	return p.src[p.toks[s.From].Pos:p.toks[s.To-1].End]
}

func TestScanExprStops(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		stop   stopSet
		want   string // text of the scanned expression
		stopAt string // text of the token the scan stopped on
	}{
		{"clause keyword", "a = 1 WHERE b", clauseStops, "a = 1", "WHERE"},
		{"keyword inside case", "CASE WHEN a THEN b WHERE c END WHERE d", clauseStops,
			"CASE WHEN a THEN b WHERE c END", "WHERE"},
		{"nested case", "CASE WHEN a THEN CASE b WHEN 1 THEN 2 END END LIMIT 1", clauseStops,
			"CASE WHEN a THEN CASE b WHEN 1 THEN 2 END END", "LIMIT"},
		{"keyword inside parens", "f(a FROM b) FROM t", clauseStops, "f(a FROM b)", "FROM"},
		{"keyword inside brackets", "a[(SELECT 1) : 2] FROM t", clauseStops, "a[(SELECT 1) : 2]", "FROM"},
		{"is distinct from", "x IS DISTINCT FROM y FROM t", clauseStops, "x IS DISTINCT FROM y", "FROM"},
		{"within group", "f(x) WITHIN GROUP (ORDER BY x) GROUP BY y", clauseStops,
			"f(x) WITHIN GROUP (ORDER BY x)", "GROUP"},
		{"comma kept", "a, b WHERE c", clauseStops, "a, b", "WHERE"},
		{"comma stops", "a + 1, b", listStops, "a + 1", ","},
		{"comma in parens kept", "f(a, b), c", listStops, "f(a, b)", ","},
		{"on conflict", "a ON CONFLICT DO NOTHING", noStops, "a", "ON"},
		{"plain on", "a JOIN b ON a.x = b.x", noStops, "a JOIN b ON a.x = b.x", ""},
		{"unmatched close paren", "a) b", noStops, "a", ")"},
		{"semicolon", "a; b", noStops, "a", ";"},
		{"unmatched close bracket", "a] b", noStops, "a", "]"},
		{"empty", "WHERE a", clauseStops, "", "WHERE"},
		{"to end", "a b c", noStops, "a b c", ""},
		{"keyword label after AS", "count(*) AS order FROM t", clauseStops, "count(*) AS order", "FROM"},
		{"case label after AS", "a AS case, b AS end FROM t", clauseStops, "a AS case, b AS end", "FROM"},
		{"from label after AS", "a AS from, b AS group FROM t", clauseStops, "a AS from, b AS group", "FROM"},
		{"bare case labels", "0 case, t.x case, CASE WHEN a THEN 1 END case, f(y) case FROM t", clauseStops,
			"0 case, t.x case, CASE WHEN a THEN 1 END case, f(y) case", "FROM"},
		{"case after a word glimt doesn't know", "CASE WHEN a THEN 1 ELSE CASE WHEN b THEN 2 END END, x OR CASE WHEN c THEN d END FROM t",
			clauseStops, "CASE WHEN a THEN 1 ELSE CASE WHEN b THEN 2 END END, x OR CASE WHEN c THEN d END", "FROM"},
		{"case after an operator", "1 + CASE WHEN a THEN 2 END FROM t", clauseStops, "1 + CASE WHEN a THEN 2 END", "FROM"},
		{"deep nesting", "((((((((((a)))))))))) b", noStops, "((((((((((a)))))))))) b", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, e, err := scanSQL(t, tt.src, tt.stop)
			if err != nil {
				t.Fatalf("scanExpr(%q): %v", tt.src, err)
			}

			if got := spanText(p, e.Span); got != tt.want {
				t.Errorf("scanExpr(%q) = %q, want %q", tt.src, got, tt.want)
			}

			if got := p.tok().Text(p.src); got != tt.stopAt {
				t.Errorf("scanExpr(%q) stopped at %q, want %q", tt.src, got, tt.stopAt)
			}
		})
	}
}

func TestScanExprErrors(t *testing.T) {
	tests := []struct {
		src, want string
	}{
		{"(a; b)", `expected ")", found ";"`},
		{"a[1", `expected "]", found end of input`},
		{"f(a] b", `expected ")", found "]"`},
		{"(CASE WHEN x) END", `expected END, found ")"`},
		{"CASE WHEN (x END) END", `expected ")", found "END"`},
		{"[a)", `expected "]", found ")"`},
		{"CASE WHEN a THEN b) c", `expected END, found ")"`},
		{"x case", "expected END to close CASE, found end of input; " +
			"a column label named case is not supported unless written AS case"},
		{"a IN (SELECT b", `expected ")" after subquery, found end of input`},
		{"a IN (SELECT FROM WHERE)", `expected expression after FROM, found "WHERE"`},
	}

	for _, tt := range tests {
		_, _, err := scanSQL(t, tt.src, noStops)
		if err == nil || err.Error() != tt.want {
			t.Errorf("scanExpr(%q) error = %v, want %q", tt.src, err, tt.want)
		}
	}
}

func TestScanExprParamModes(t *testing.T) {
	tests := []struct {
		src  string
		want []ParamMode
	}{
		{"a IN (:x)", []ParamMode{Expand}},
		{"a NOT IN (:x)", []ParamMode{Expand}},
		{"a in ( :x )", []ParamMode{Expand}},
		{"a IN (:x, :y)", []ParamMode{Scalar, Scalar}},
		{"a IN ((:x))", []ParamMode{Scalar}},
		{"a IN (b, :x)", []ParamMode{Scalar}},
		{"a IN (:x::int)", []ParamMode{Scalar}},
		{"a = (:x)", []ParamMode{Scalar}},
		{"a = ANY(:x)", []ParamMode{Scalar}},
		{":x", []ParamMode{Scalar}},
	}

	for _, tt := range tests {
		_, e, err := scanSQL(t, tt.src, noStops)
		if err != nil {
			t.Fatalf("scanExpr(%q): %v", tt.src, err)
		}

		got := make([]ParamMode, len(e.Params))
		for i, prm := range e.Params {
			got[i] = prm.Mode
		}

		if len(got) != len(tt.want) || strings.Join(modeNames(got), ",") != strings.Join(modeNames(tt.want), ",") {
			t.Errorf("scanExpr(%q) modes = %v, want %v", tt.src, got, tt.want)
		}
	}
}

func modeNames(modes []ParamMode) []string {
	names := make([]string, len(modes))
	for i, m := range modes {
		names[i] = m.String()
	}

	return names
}

func TestScanExprSubqueries(t *testing.T) {
	src := "(SELECT 1) + (VALUES (:a)) + (WITH x AS (SELECT 1) SELECT :b) + (c) + ((SELECT 2))"

	p, e, err := scanSQL(t, src, noStops)
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"(SELECT 1)", "(VALUES (:a))", "(WITH x AS (SELECT 1) SELECT :b)", "(SELECT 2)"}
	if len(e.Subqueries) != len(want) {
		t.Fatalf("got %d subqueries, want %d", len(e.Subqueries), len(want))
	}

	for i, sub := range e.Subqueries {
		if got := spanText(p, sub.Span); got != want[i] {
			t.Errorf("subquery %d = %q, want %q", i, got, want[i])
		}

		if inner := (Span{From: sub.From + 1, To: sub.To - 1}); sub.Query.Span != inner {
			t.Errorf("subquery %d: query span %v, want %v", i, sub.Query.Span, inner)
		}
	}

	if e.Params != nil {
		t.Errorf("params inside subqueries leaked into the expression: %v", e.Params)
	}
}

func TestScanExprValuesColumn(t *testing.T) {
	_, e, err := scanSQL(t, "(values) + (VALUES (1))", noStops)
	if err != nil {
		t.Fatal(err)
	}

	if len(e.Subqueries) != 1 {
		t.Errorf("got %d subqueries, want 1: (values) is a column", len(e.Subqueries))
	}
}

// TestParseAllocsDoNotGrowWithTokens checks that parsing lexed tokens
// allocates nothing per token: a WHERE clause a hundred times longer costs
// the same allocations.
func TestParseAllocsDoNotGrowWithTokens(t *testing.T) {
	allocs := func(terms int) float64 {
		src := "SELECT a FROM t WHERE " + strings.Repeat("a = 1 AND ", terms) + "b = 2"
		toks := mustLex(t, src)

		return testing.AllocsPerRun(20, func() {
			if _, err := parseTokens(src, toks); err != nil {
				t.Fatal(err)
			}
		})
	}

	if small, large := allocs(1), allocs(100); small != large {
		t.Errorf("allocations grew with the token count: %v for 1 term, %v for 100", small, large)
	}
}
