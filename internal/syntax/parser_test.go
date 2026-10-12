package syntax

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// parseSQL parses src.
func parseSQL(tb testing.TB, src string) (*Parsed, error) {
	tb.Helper()

	return Parse(src)
}

// parseCase is one table-driven parser test. want is the Dump output, or the
// error message when err is set. A leading newline in want is ignored.
type parseCase struct {
	name string
	sql  string
	want string
	err  bool
}

// runParseCases runs each case as a subtest. Every successful parse must also
// hold the structural invariants.
func runParseCases(t *testing.T, cases []parseCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := parseSQL(t, tc.sql)
			want := strings.TrimPrefix(tc.want, "\n")

			if tc.err {
				if err == nil {
					t.Fatalf("Parse(%q) succeeded, want error %q\n%s", tc.sql, want, Dump(p))
				}

				if err.Error() != want {
					t.Fatalf("Parse(%q) error = %q, want %q", tc.sql, err, want)
				}

				return
			}

			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.sql, err)
			}

			checkInvariants(t, p)

			if got := Dump(p); got != want {
				t.Errorf("Parse(%q) dump:\n%s\nwant:\n%s", tc.sql, got, want)
			}
		})
	}
}

// checkInvariants verifies the structural invariants every successful parse
// must hold: the statement covers the whole input but a trailing ";", child
// spans nest inside their parents in source order, and every PARAM token is
// recorded exactly once.
func checkInvariants(tb testing.TB, p *Parsed) {
	tb.Helper()

	start, end := int32(0), int32(len(p.Tokens)-1)
	for start < end && p.Tokens[start].Kind == SEMICOLON {
		start++
	}

	for end > start && p.Tokens[end-1].Kind == SEMICOLON {
		end--
	}

	if got := p.Stmt.Bounds(); got != (Span{From: start, To: end}) {
		tb.Errorf("statement span = %v, want %v", got, Span{From: start, To: end})
	}

	checkTree(tb, p, p.Stmt)
	checkParams(tb, p)
}

// checkTree verifies that n's children lie inside n, in order and without
// overlapping, then checks each child.
func checkTree(tb testing.TB, p *Parsed, n Node) {
	tb.Helper()

	parent := n.Bounds()
	prev := parent.From

	// Source order, since LIMIT and OFFSET may come in either order.
	kids := children(n)
	slices.SortStableFunc(kids, func(a, b Node) int { return int(a.Bounds().From - b.Bounds().From) })

	for _, c := range kids {
		s := c.Bounds()
		if !parent.Contains(s) || s.From < prev {
			tb.Errorf("%T %v: child %T %v is out of place", n, parent, c, s)
		}

		prev = s.To
		checkTree(tb, p, c)
	}

	if e, ok := n.(*Expr); ok {
		checkExprParams(tb, p, e)
	}
}

// checkExprParams verifies that e's params are PARAM tokens inside e and
// outside its subqueries.
func checkExprParams(tb testing.TB, p *Parsed, e *Expr) {
	tb.Helper()

	for _, prm := range e.Params {
		tok := Span{From: prm.Tok, To: prm.Tok + 1}
		if !e.Contains(tok) || p.Tokens[prm.Tok].Kind != PARAM {
			tb.Errorf("param %q at %d is not a PARAM token in %v", prm.Name, prm.Tok, e.Span)
		}

		for _, sub := range e.Subqueries {
			if sub.Contains(tok) {
				tb.Errorf("param %q at %d belongs to a subquery, not %v", prm.Name, prm.Tok, e.Span)
			}
		}
	}
}

// checkParams verifies Parsed.Params against a flat scan of the PARAM tokens.
func checkParams(tb testing.TB, p *Parsed) {
	tb.Helper()

	var toks []int32

	for i, t := range p.Tokens {
		if t.Kind == PARAM {
			toks = append(toks, int32(i))
		}
	}

	got := make([]int32, len(p.Params))
	for i, prm := range p.Params {
		got[i] = prm.Tok
		if want := p.Tokens[prm.Tok].Text(p.Src)[1:]; prm.Name != want {
			tb.Errorf("param at %d named %q, want %q", prm.Tok, prm.Name, want)
		}
	}

	if !slices.Equal(got, toks) {
		tb.Errorf("Params at tokens %v, want %v", got, toks)
	}
}

// tokNode is a single token seen as a node, for identifiers in checkTree.
type tokNode int32

func (t tokNode) Bounds() Span {
	return Span{From: int32(t), To: int32(t) + 1}
}

// kids collects the children of a node, skipping absent ones.
type kids []Node

func (k *kids) add(n Node) {
	*k = append(*k, n)
}

func (k *kids) expr(e *Expr) {
	if e != nil {
		k.add(e)
	}
}

func (k *kids) with(w *With) {
	if w != nil {
		k.add(w)
	}
}

func (k *kids) alias(a *Ident) {
	if a != nil {
		k.add(tokNode(a.Tok))
	}
}

func (k *kids) idents(list []Ident) {
	for _, id := range list {
		k.add(tokNode(id.Tok))
	}
}

func (k *kids) targets(list []Target) {
	for i := range list {
		k.add(&list[i])
	}
}

func (k *kids) assignments(list []Assignment) {
	for i := range list {
		k.add(&list[i])
	}
}

func (k *kids) rows(rows [][]Expr) {
	for _, row := range rows {
		for i := range row {
			k.add(&row[i])
		}
	}
}

// children returns n's child nodes in source order.
func children(n Node) []Node {
	var k kids

	switch n := n.(type) {
	case *Query:
		k.with(n.With)
		k.add(n.Body)
		k.expr(n.OrderBy)
		k.expr(n.Limit)
		k.expr(n.Offset)
	case *Select:
		k.expr(n.DistinctOn)
		k.expr(&n.Columns)
		k.expr(n.From)
		k.expr(n.Where)
		k.expr(n.GroupBy)
		k.expr(n.Having)
	case *SetOp:
		k.add(n.Left)
		k.add(n.Right)
	case *Values:
		k.rows(n.Rows)
	case *ParenQuery:
		k.add(n.Query)
	case *Expr:
		for _, sub := range n.Subqueries {
			k.add(sub)
		}
	case *Subquery:
		k.add(n.Query)
	default:
		return dmlChildren(n)
	}

	return k
}

// dmlChildren returns the children of the nodes children doesn't handle.
func dmlChildren(n Node) []Node {
	var k kids

	switch n := n.(type) {
	case *With:
		for _, c := range n.CTEs {
			k.add(c)
		}
	case *CTE:
		k.add(tokNode(n.Name.Tok))
		k.idents(n.Columns)
		k.add(n.Body)
		k.expr(n.Search)
		k.expr(n.Cycle)
	case *Insert:
		k.with(n.With)
		k.add(&n.Table)
		k.alias(n.Alias)
		k.targets(n.Columns)
		k.insertSource(n)
		k.expr(n.Returning)
	case *OnConflict:
		k.expr(n.Target)
		k.assignments(n.Set)
		k.expr(n.Where)
	case *Update:
		k.update(n)
	case *Delete:
		k.with(n.With)
		k.add(&n.Table)
		k.alias(n.Alias)
		k.expr(n.Using)
		k.expr(n.Where)
		k.expr(n.Returning)
	case *Merge:
		k.merge(n)
	default:
		return leafChildren(n)
	}

	return k
}

// leafChildren returns the children of the smallest nodes.
func leafChildren(n Node) []Node {
	var k kids

	switch n := n.(type) {
	case *Assignment:
		k.targets(n.Targets)
		k.add(&n.Value)
	case *Target:
		k.add(tokNode(n.Column.Tok))
		k.expr(n.Path)
	case *MergeWhen:
		k.expr(n.Cond)
		k.assignments(n.Set)
		k.targets(n.Columns)
		k.rows([][]Expr{n.Values})
	case *Raw:
		k.add(&n.Body)
	case *TableQuery:
		k.add(&n.Name)
	case *ObjectName:
		k.idents(n.Parts)
	}

	return k
}

func (k *kids) insertSource(n *Insert) {
	if n.Source != nil {
		k.add(n.Source)
	}

	if n.OnConflict != nil {
		k.add(n.OnConflict)
	}
}

func (k *kids) merge(n *Merge) {
	k.with(n.With)
	k.add(&n.Table)
	k.alias(n.Alias)
	k.expr(n.Using)
	k.expr(n.On)

	for _, w := range n.When {
		k.add(w)
	}

	k.expr(n.Returning)
}

func (k *kids) update(n *Update) {
	k.with(n.With)
	k.add(&n.Table)
	k.alias(n.Alias)
	k.assignments(n.Set)
	k.expr(n.From)
	k.expr(n.Where)
	k.expr(n.Returning)
}

func TestParseStatements(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "semicolons around", sql: ";; SELECT 1; ;;", want: `
query
  select
    columns: 1
`},
		{name: "raw create", sql: "CREATE INDEX i ON t (a) WHERE b = :b", want: `
raw: CREATE INDEX i ON t (a) WHERE b = :b
params: b
`},
		{name: "raw drop", sql: "drop table if exists orders", want: `
raw: drop table if exists orders
`},
		{name: "raw with subquery", sql: "CREATE TABLE t AS (SELECT a FROM s)", want: `
raw: CREATE TABLE t AS (SELECT a FROM s)
  subquery
    query
      select
        columns: a
        from: s
`},
		{name: "with before update", sql: "WITH x AS (SELECT 1) UPDATE t SET a = 1", want: `
update
  with
    cte x
      query
        select
          columns: 1
  table: t
  set: a = 1
`},
		{name: "with recursive and options", sql: "WITH RECURSIVE r (n) AS NOT MATERIALIZED (SELECT 1), " +
			"m AS MATERIALIZED (VALUES (2)) SELECT n FROM r", want: `
query
  with recursive
    cte r (n) not materialized
      query
        select
          columns: 1
    cte m materialized
      query
        values
          row: [2]
  select
    columns: n
    from: r
`},
		{name: "search and cycle", sql: "WITH RECURSIVE g (f, t) AS (SELECT 1, 2 UNION ALL SELECT f, t FROM g) " +
			"SEARCH DEPTH FIRST BY f, t SET seq CYCLE f, t SET is_cycle TO true DEFAULT false USING path, " +
			"h AS (SELECT 1) SELECT * FROM g", want: `
query
  with recursive
    cte g (f, t)
      query
        union all
          select
            columns: 1, 2
          select
            columns: f, t
            from: g
      search: DEPTH FIRST BY f, t SET seq
      cycle: f, t SET is_cycle TO true DEFAULT false USING path
    cte h
      query
        select
          columns: 1
  select
    columns: *
    from: g
`},
		{name: "cte named recursive", sql: "WITH recursive AS (SELECT 1), r2 AS (SELECT 2) TABLE recursive", want: `
query
  with
    cte recursive
      query
        select
          columns: 1
    cte r2
      query
        select
          columns: 2
  table recursive
`},
		{name: "quoted cte name", sql: `WITH "My""Cte" AS (SELECT 1) SELECT 2`, want: `
query
  with
    cte "My""Cte"
      query
        select
          columns: 1
  select
    columns: 2
`},
	})
}

func TestParseErrors(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "unknown statement", sql: "INSET INTO t VALUES (1)", err: true,
			want: `expected a statement, found "INSET"`},
		{name: "empty", sql: "", err: true, want: "expected a statement, found end of input"},
		{name: "two statements", sql: "SELECT 1; SELECT 2", err: true,
			want: `a second statement starts at "SELECT": is a "-- name:" annotation missing or misspelled?`},
		{name: "two selects run together", sql: "SELECT a FROM t WHERE b = 1\nSELECT 2", err: true,
			want: `a second statement starts at "SELECT": is a "-- name:" annotation missing or misspelled?`},
		{name: "select after a select list", sql: "SELECT 1\nSELECT 2", err: true,
			want: `a second statement starts at "SELECT": is a "-- name:" annotation missing or misspelled?`},
		{name: "table after a clause", sql: "SELECT a FROM t ORDER BY a\nTABLE u", err: true,
			want: `a second statement starts at "TABLE": is a "-- name:" annotation missing or misspelled?`},
		{name: "update after a semicolon", sql: "DELETE FROM t;\nUPDATE t SET a = 1", err: true,
			want: `a second statement starts at "UPDATE": is a "-- name:" annotation missing or misspelled?`},
		{name: "raw after a semicolon", sql: "SELECT 1; DROP TABLE t", err: true,
			want: `a second statement starts at "DROP": is a "-- name:" annotation missing or misspelled?`},
		{name: "select after a bare label", sql: "SELECT 1 x\nSELECT 2", err: true,
			want: `a second statement starts at "SELECT": is a "-- name:" annotation missing or misspelled?`},
		{name: "select after returning", sql: "DELETE FROM t RETURNING id\nSELECT a FROM t", err: true,
			want: `a second statement starts at "SELECT": is a "-- name:" annotation missing or misspelled?`},
		{name: "select star after a select list", sql: "SELECT 1\nSELECT * FROM t", err: true,
			want: `a second statement starts at "SELECT": is a "-- name:" annotation missing or misspelled?`},
		{name: "junk after a statement", sql: "SELECT 1; x", err: true,
			want: `expected end of statement, found "x"`},
		{name: "with without statement", sql: "WITH x AS (SELECT 1) CREATE TABLE y", err: true,
			want: `expected SELECT, VALUES, TABLE, INSERT, UPDATE, DELETE or MERGE, found "CREATE"`},
		{name: "dml in cte", sql: "WITH d AS (DELETE FROM t RETURNING id) SELECT * FROM d", err: true,
			want: `a data-modifying statement in WITH is not supported`},
		{name: "dml after a nested with", sql: "WITH x AS (WITH y AS (SELECT 1) DELETE FROM t RETURNING a) SELECT 1", err: true,
			want: "a data-modifying statement in WITH is not supported"},
		{name: "search without set", sql: "WITH x AS (SELECT 1) SEARCH DEPTH FIRST BY a SELECT 1", err: true,
			want: `expected SET, found end of input`},
		{name: "cte missing as", sql: "WITH x (SELECT 1) SELECT 2", err: true,
			want: `expected column name, found "SELECT"`},
		{name: "reserved cte name", sql: "WITH select AS (SELECT 1) SELECT 2", err: true,
			want: `expected CTE name, found "select"`},
		{name: "unclosed paren", sql: "SELECT (1 FROM t", err: true,
			want: `expected ")", found end of input`},
		{name: "unmatched bracket", sql: "SELECT a] FROM t", err: true,
			want: `expected end of statement, found "]"`},
		{name: "unclosed bracket", sql: "SELECT a[1 FROM t", err: true,
			want: `expected "]", found end of input`},
		{name: "case without end", sql: "SELECT CASE WHEN a THEN 1 FROM t", err: true,
			want: "expected END to close CASE, found end of input; " +
				"a column label named case is not supported unless written AS case"},
		{name: "stray close paren", sql: "SELECT 1)", err: true,
			want: `expected end of statement, found ")"`},
		{name: "raw stray close paren", sql: "DROP TABLE t)", err: true,
			want: `expected end of statement, found ")"`},
	})
}

// TestKeywordsAsNames checks every keyword Postgres accepts as a name, as an
// alias and as a column, so a new keyword can't quietly stop being one.
func TestKeywordsAsNames(t *testing.T) {
	templates := []string{
		"UPDATE t %[1]s SET %[1]s = 1 WHERE x",
		"DELETE FROM t %[1]s WHERE x",
		"INSERT INTO t AS %[1]s (%[1]s) VALUES (1)",
		"MERGE INTO t %[1]s USING s ON x WHEN MATCHED THEN UPDATE SET %[1]s = 1",
		"WITH %[1]s (%[1]s) AS (SELECT 1) TABLE %[1]s",
	}

	for k := Keyword(1); k < numKeywords; k++ {
		if !k.colID() {
			continue
		}

		name := strings.ToLower(k.String())

		for i, tmpl := range templates {
			if k == SET && (i < 2 || i == 3) {
				continue // SET can't be a bare alias
			}

			src := fmt.Sprintf(tmpl, name)

			p, err := parseSQL(t, src)
			if err != nil {
				t.Errorf("Parse(%q): %v", src, err)

				continue
			}

			checkInvariants(t, p)
		}
	}
}

func TestParseAliases(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "unreserved merge alias", sql: "MERGE INTO c target USING s ON c.id = s.id WHEN MATCHED THEN DELETE", want: `
merge
  table: c alias: target
  using: s
  on: c.id = s.id
  when matched
    delete
`},
		{name: "unreserved update alias", sql: "UPDATE a source SET x = 1", want: `
update
  table: a alias: source
  set: x = 1
`},
		{name: "unreserved delete alias", sql: "DELETE FROM t value WHERE x", want: `
delete
  table: t alias: value
  where: x
`},
		{name: "set is not an alias", sql: "UPDATE t SET set = 1", want: `
update
  table: t
  set: set = 1
`},
		{name: "reserved alias needs quotes", sql: "UPDATE t user SET x = 1", err: true,
			want: `expected SET, found "user"`},
		{name: "call", sql: "CALL proc(:a, 2)", want: `
raw: CALL proc(:a, 2)
params: a
`},
	})
}

func TestErrorPosition(t *testing.T) {
	src := "SELECT a\nFROM t\nWHERE ]"

	_, err := parseSQL(t, src)

	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("Parse error = %v, want *Error", err)
	}

	if line, col := perr.Position(src); line != 3 || col != 7 {
		t.Errorf("Position = %d:%d, want 3:7", line, col)
	}
}

func TestParseParams(t *testing.T) {
	p, err := parseSQL(t, "SELECT :a, :b FROM t WHERE x = :a AND y IN (SELECT z FROM u WHERE w = :c)")
	if err != nil {
		t.Fatal(err)
	}

	checkInvariants(t, p)

	names := make([]string, len(p.Params))
	for i, prm := range p.Params {
		names[i] = prm.Name
	}

	if want := []string{"a", "b", "a", "c"}; !slices.Equal(names, want) {
		t.Errorf("params = %v, want %v", names, want)
	}
}

func TestParseWithoutParams(t *testing.T) {
	p, err := parseSQL(t, "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}

	if p.Params != nil {
		t.Errorf("Params = %v, want nil", p.Params)
	}
}

// fuzzVocab is the vocabulary FuzzParse builds token streams from.
var fuzzVocab = strings.Fields(`SELECT FROM WHERE GROUP BY HAVING ORDER LIMIT OFFSET UNION INTERSECT
	EXCEPT ALL DISTINCT ON CONFLICT DO NOTHING UPDATE SET INSERT INTO VALUES DEFAULT DELETE USING
	RETURNING WITH RECURSIVE AS MATERIALIZED NOT IN IS CASE END CREATE FOR ONLY MERGE WHEN THEN AND
	MATCHED BY SOURCE TARGET TABLE OVERRIDING SYSTEM VALUE INTO FETCH WINDOW CALL ELSE ROWS SEARCH CYCLE
	JOIN CROSS NATURAL a b t lo hi
	:p :q 1 'x' "Q" ( ) [ ] , ; . = + :: :`)

// fuzzSQL maps each byte of data to a vocabulary word. The high bit glues
// the word to the one before it, so words can touch: lo:hi, t.offset.
func fuzzSQL(data []byte) string {
	var b strings.Builder
	for i, c := range data {
		if i > 0 && c&0x80 == 0 {
			b.WriteByte(' ')
		}

		b.WriteString(fuzzVocab[int(c&0x7f)%len(fuzzVocab)])
	}

	return b.String()
}

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"SELECT a FROM t WHERE a IN ( :p )",
		"INSERT INTO t ( a ) SELECT a FROM t WHERE a = :p ON CONFLICT ( a ) DO UPDATE SET a = :q RETURNING a",
		"WITH t AS ( SELECT 1 ) SELECT a FROM t UNION ALL SELECT b FROM t ORDER BY a LIMIT :p",
		"UPDATE t SET a = CASE a END WHERE b = ( SELECT 1 )",
		"DELETE FROM t USING a WHERE b IN ( SELECT a FROM t ) RETURNING a",
		"MERGE INTO t USING ( SELECT a FROM b ) ON a = b WHEN MATCHED AND a THEN UPDATE SET a [ :p ] = 1 " +
			"WHEN NOT MATCHED THEN INSERT ( a . b ) OVERRIDING SYSTEM VALUE VALUES ( :q ) RETURNING a",
		"TABLE t UNION TABLE a",
	} {
		data := make([]byte, 0, len(seed))
		for _, w := range strings.Fields(seed) {
			data = append(data, byte(slices.Index(fuzzVocab, w)))
		}

		f.Add(data)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		src := fuzzSQL(data)

		p, err := parseSQL(t, src)
		if err != nil {
			var perr *Error
			if !errors.As(err, &perr) {
				t.Fatalf("Parse(%q) error %v is not *Error", src, err)
			}

			return
		}

		checkInvariants(t, p)
	})
}

// benchQueries are statements of increasing size for BenchmarkParse.
var benchQueries = []struct {
	name, sql string
}{
	{"small", "SELECT id, name FROM users WHERE id = :id"},
	{"medium", "SELECT o.id, o.total, u.name FROM orders o JOIN users u ON u.id = o.user_id " +
		"WHERE o.org_id = :org AND o.status IN (:statuses) AND o.created_at >= :since " +
		"ORDER BY o.created_at DESC, o.id LIMIT :limit OFFSET :offset"},
	{"large", "WITH recent AS (SELECT user_id, sum(total) AS spent FROM orders " +
		"WHERE created_at >= :since GROUP BY user_id HAVING sum(total) > :min) " +
		"INSERT INTO vip (user_id, spent, tier) SELECT r.user_id, r.spent, " +
		"CASE WHEN r.spent > :gold THEN 'gold' ELSE 'silver' END FROM recent r " +
		"JOIN users u ON u.id = r.user_id WHERE u.active AND u.id NOT IN (SELECT user_id FROM banned) " +
		"UNION ALL SELECT user_id, 0, 'new' FROM signups WHERE created_at >= :since " +
		"ON CONFLICT (user_id) DO UPDATE SET spent = excluded.spent, tier = excluded.tier " +
		"WHERE vip.spent < excluded.spent RETURNING user_id, tier"},
}

func BenchmarkParse(b *testing.B) {
	for _, q := range benchQueries {
		b.Run(q.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				if _, err := Parse(q.sql); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
