package render

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mochams/glimt/internal/syntax"
)

// sprint prints a value for comparing args.
func sprint(v any) string {
	return fmt.Sprintf("%T:%v", v, v)
}

func TestRender(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		values   []any
		wantSQL  string
		wantArgs []any
	}{
		{"no params", "SELECT 1", []any{}, "SELECT 1", []any{}},
		{"scalars", "SELECT a FROM t WHERE b = :b AND c > :c", []any{1, "x"},
			"SELECT a FROM t WHERE b = $1 AND c > $2", []any{1, "x"}},
		{"repeated name reuses its number", "SELECT :a, :b WHERE x = :a", []any{1, 2},
			"SELECT $1, $2 WHERE x = $1", []any{1, 2}},
		{"expand", "SELECT * FROM t WHERE id IN (:ids) AND org = :org", []any{[]int{7, 8, 9}, 1},
			"SELECT * FROM t WHERE id IN ($1, $2, $3) AND org = $4", []any{7, 8, 9, 1}},
		{"expand after a scalar", "SELECT * FROM t WHERE org = :org AND id NOT IN (:ids)", []any{1, []string{"a", "b"}},
			"SELECT * FROM t WHERE org = $1 AND id NOT IN ($2, $3)", []any{1, "a", "b"}},
		{"expanded name reused", "SELECT * FROM t WHERE a IN (:ids) OR b IN (:ids)", []any{[]int{1, 2}},
			"SELECT * FROM t WHERE a IN ($1, $2) OR b IN ($1, $2)", []any{1, 2}},
		{"expanded and scalar uses numbered apart", "SELECT * FROM t WHERE a IN (:ids) OR b = ANY(:ids)",
			[]any{[]int{1, 2}}, "SELECT * FROM t WHERE a IN ($1, $2) OR b = ANY($3)", []any{1, 2, []int{1, 2}}},
		{"scalar in expand position", "SELECT * FROM t WHERE id IN (:id)", []any{5},
			"SELECT * FROM t WHERE id IN ($1)", []any{5}},
		{"byte slice in expand position", "SELECT * FROM t WHERE h IN (:h)", []any{[]byte("ab")},
			"SELECT * FROM t WHERE h IN ($1)", []any{[]byte("ab")}},
		{"params in every place", "WITH c AS (SELECT :a) MERGE INTO t USING (SELECT :b) s ON t.id = s.id " +
			"WHEN MATCHED AND t.x IN (:xs) THEN UPDATE SET v[:i] = :a " +
			"WHEN NOT MATCHED THEN INSERT (id) VALUES (:c) RETURNING :d",
			[]any{"a", "b", []int{1, 2}, 3, "c", "d"},
			"WITH c AS (SELECT $1) MERGE INTO t USING (SELECT $2) s ON t.id = s.id " +
				"WHEN MATCHED AND t.x IN ($3, $4) THEN UPDATE SET v[$5] = $1 " +
				"WHEN NOT MATCHED THEN INSERT (id) VALUES ($6) RETURNING $7",
			[]any{"a", "b", 1, 2, 3, "c", "d"}},
		{"params in subqueries", "SELECT * FROM t WHERE a IN (SELECT b FROM u WHERE c IN (:cs)) LIMIT :n",
			[]any{[]int{1}, 10}, "SELECT * FROM t WHERE a IN (SELECT b FROM u WHERE c IN ($1)) LIMIT $2", []any{1, 10}},
		{"placeholder after a keyword gets a space", "SELECT CASE WHEN a THEN:b ELSE:c END", []any{1, 2},
			"SELECT CASE WHEN a THEN $1 ELSE $2 END", []any{1, 2}},
		{"touching params get a space", "SELECT :a:b", []any{1, 2}, "SELECT $1 $2", []any{1, 2}},
		{"tail clauses keep their order", "SELECT a FROM t OFFSET :off LIMIT :lim", []any{5, 10},
			"SELECT a FROM t OFFSET $1 LIMIT $2", []any{5, 10}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl := compileSQL(t, tt.src)

			sql, args, err := tmpl.Render(tt.values)
			if err != nil {
				t.Fatal(err)
			}

			if sql != tt.wantSQL || !equalArgs(args, tt.wantArgs) {
				t.Errorf("Render =\n%s %v\nwant\n%s %v", sql, args, tt.wantSQL, tt.wantArgs)
			}

			checkNumbering(t, sql, args)
		})
	}
}

func TestRenderValuesOrder(t *testing.T) {
	tmpl := compileSQL(t, "SELECT a FROM t OFFSET :off LIMIT :lim")

	if got := tmpl.Names(); !slices.Equal(got, []string{"off", "lim"}) {
		t.Errorf("Names = %v, want [off lim]: values follow the written order", got)
	}

	tmpl.Names()[0] = "changed"

	if got := tmpl.Names()[0]; got != "off" {
		t.Errorf("Names shares the template's slice: got %q after a change", got)
	}
}

func TestRenderErrors(t *testing.T) {
	tests := []struct {
		src    string
		values []any
		want   string
	}{
		{"SELECT :a, :b", []any{1}, "got 1 values for 2 params"},
		{"SELECT 1 WHERE a IN (:ids)", []any{1, 2}, "got 2 values for 1 params"},
		{"SELECT 1 WHERE a IN (:ids)", []any{[]int{}}, ":ids is an empty list"},
		{"SELECT 1 WHERE a IN (:ids)", []any{nil}, ":ids is a nil value"},
		{"SELECT 1 WHERE a IN (:ids)", []any{[][]int{{1}}}, ":ids has elements that are lists, which IN (…) can't take"},
		{"SELECT 1 WHERE a IN (:ids)", []any{make([]int, 65536)}, "65536 args is more than the 65535 Postgres accepts"},
	}

	for _, tt := range tests {
		if _, _, err := compileSQL(t, tt.src).Render(tt.values); err == nil || err.Error() != tt.want {
			t.Errorf("Render(%q, %v) error = %v, want %q", tt.src, tt.values, err, tt.want)
		}
	}
}

func TestRenderScalarNil(t *testing.T) {
	sql, args, err := compileSQL(t, "SELECT * FROM t WHERE a = :a").Render([]any{nil})
	if err != nil || sql != "SELECT * FROM t WHERE a = $1" || len(args) != 1 || args[0] != nil {
		t.Errorf("Render = %q, %v, %v: a nil scalar binds NULL", sql, args, err)
	}
}

func TestRenderAllocations(t *testing.T) {
	static := compileSQL(t, benchQueries[1].src)
	expanding := compileSQL(t, "SELECT * FROM t WHERE org = :org AND id IN (:ids) AND s IN (:ss)")

	staticValues := []any{1, "a", "b", 2, 3}
	expandingValues := []any{1, []any{1, 2, 3}, []any{"a", "b"}}

	if n := testing.AllocsPerRun(50, func() { _, _, _ = static.Render(staticValues) }); n != 0 {
		t.Errorf("Render without expansion allocates %v times, want 0", n)
	}

	// The builder's buffer and the args; []any elements are already boxed.
	if n := testing.AllocsPerRun(50, func() { _, _, _ = expanding.Render(expandingValues) }); n != 2 {
		t.Errorf("Render with expansion allocates %v times, want 2", n)
	}
}

// placeholderRE matches a $n placeholder.
var placeholderRE = regexp.MustCompile(`\$(\d+)`)

// checkNumbering verifies that the placeholders in sql are exactly $1 to
// $len(args), each used at least once, and that none directly follows a
// word character, which would make Postgres read both as one identifier.
func checkNumbering(tb testing.TB, sql string, args []any) {
	tb.Helper()

	seen := make([]bool, len(args)+1)

	for _, loc := range placeholderRE.FindAllStringSubmatchIndex(sql, -1) {
		if loc[0] > 0 && isWordByte(sql[loc[0]-1]) {
			tb.Errorf("placeholder at %d in %q touches the word before it", loc[0], sql)
		}

		n, _ := strconv.Atoi(sql[loc[2]:loc[3]])
		if n < 1 || n > len(args) {
			tb.Errorf("placeholder $%d in %q is out of range for %d args", n, sql, len(args))

			continue
		}

		seen[n] = true
	}

	for n := 1; n <= len(args); n++ {
		if !seen[n] {
			tb.Errorf("arg %d is never used in %q", n, sql)
		}
	}
}

// fuzzVocab is the vocabulary FuzzRender builds statements from.
var fuzzVocab = strings.Fields(`SELECT FROM WHERE GROUP BY HAVING ORDER LIMIT OFFSET UNION INTERSECT
	EXCEPT ALL DISTINCT ON CONFLICT DO NOTHING UPDATE SET INSERT INTO VALUES DEFAULT DELETE USING
	RETURNING WITH RECURSIVE AS MATERIALIZED NOT IN IS CASE END CREATE MERGE WHEN THEN AND MATCHED
	TABLE OVERRIDING SYSTEM VALUE FETCH WINDOW ELSE a b t lo hi :p :q 1 'x' "Q" ( ) [ ] , . = + :: :`)

// fuzzSource maps each byte of data to a vocabulary word. The high bit glues
// a word to the one before it, so words can touch.
func fuzzSource(data []byte) string {
	var b strings.Builder
	for i, c := range data {
		if i > 0 && c&0x80 == 0 {
			b.WriteByte(' ')
		}

		b.WriteString(fuzzVocab[int(c&0x7f)%len(fuzzVocab)])
	}

	return b.String()
}

func FuzzRender(f *testing.F) {
	seeds := []string{
		"SELECT a FROM t WHERE a IN ( :p ) AND b = :q",
		"INSERT INTO t ( a ) VALUES ( :p ) ON CONFLICT ( a ) DO UPDATE SET a = :q RETURNING a",
		"SELECT 1 UNION ALL SELECT 2 INTERSECT SELECT :p",
		"UPDATE t SET a [ :p ] = :q WHERE b IN ( :p )",
		"MERGE INTO t USING a ON a = b WHEN MATCHED THEN DELETE",
	}
	for _, seed := range seeds {
		data := make([]byte, 0, len(seed))
		for _, w := range strings.Fields(seed) {
			data = append(data, byte(slices.Index(fuzzVocab, w)))
		}

		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		src := fuzzSource(data)

		p, err := syntax.Parse(src)
		if err != nil {
			return
		}

		tmpl, err := Compile(p)
		if err != nil {
			t.Fatalf("Compile(%q): %v", src, err)
		}

		checkRoundTrip(t, p, tmpl)

		values := make([]any, len(tmpl.names))
		for i := range values {
			values[i] = []int{i, i + 1}
		}

		sql, args, err := tmpl.Render(values)
		if err != nil {
			t.Fatalf("Render(%q): %v", src, err)
		}

		checkNumbering(t, sql, args)
	})
}

// benchQueries are statements for the benchmarks.
var benchQueries = []struct {
	name, src string
}{
	{"small", "SELECT id, name FROM users WHERE id = :id"},
	{"medium", "SELECT o.id, o.total, u.name FROM orders o JOIN users u ON u.id = o.user_id " +
		"WHERE o.org_id = :org AND o.status = :status AND o.kind = :kind AND o.created_at >= :since " +
		"ORDER BY o.created_at DESC, o.id LIMIT :limit"},
	{"expanding", "SELECT o.id, o.total FROM orders o WHERE o.org_id = :org AND o.status IN (:statuses) " +
		"AND o.id NOT IN (:skip) ORDER BY o.id LIMIT :limit"},
}

func TestCompileManyNames(t *testing.T) {
	var b strings.Builder

	b.WriteString("SELECT 1 WHERE")
	for i := range 40 {
		fmt.Fprintf(&b, " a = :p%d OR b = :p%d OR", i, i%7)
	}

	b.WriteString(" true")

	tmpl := compileSQL(t, b.String())
	if len(tmpl.names) != 40 {
		t.Fatalf("got %d names, want 40", len(tmpl.names))
	}

	for i, name := range tmpl.names {
		if name != fmt.Sprintf("p%d", i) {
			t.Errorf("name %d = %s", i, name)
		}
	}
}

func BenchmarkCompile(b *testing.B) {
	for _, q := range benchQueries {
		p := parseSQL(b, q.src)

		b.Run(q.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				if _, err := Compile(p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRender(b *testing.B) {
	values := map[string][]any{
		"small":      {42},
		"medium":     {1, "paid", "web", "2026-01-01", 50},
		"expanding":  {1, []string{"paid", "shipped", "done"}, []int{4, 8}, 50},
		"expand-100": {1, make([]int, 100), []int{4, 8}, 50},
	}

	queries := append(benchQueries, struct{ name, src string }{"expand-100", benchQueries[2].src})
	for _, q := range queries {
		tmpl := compileSQL(b, q.src)

		b.Run(q.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				if _, _, err := tmpl.Render(values[q.name]); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
