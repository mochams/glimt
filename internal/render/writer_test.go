package render

import (
	"slices"
	"strings"
	"testing"

	"github.com/mochams/glimt/internal/syntax"
)

// parseSQL parses src, failing the test on error.
func parseSQL(tb testing.TB, src string) *syntax.Parsed {
	tb.Helper()

	p, err := syntax.Parse(src)
	if err != nil {
		tb.Fatalf("Parse(%q): %v", src, err)
	}

	return p
}

// compileSQL parses and compiles src, failing the test on error.
func compileSQL(tb testing.TB, src string) *Template {
	tb.Helper()

	tmpl, err := Compile(parseSQL(tb, src))
	if err != nil {
		tb.Fatalf("Compile(%q): %v", src, err)
	}

	return tmpl
}

// named joins a template's parts with its params written back as :name,
// giving SQL that lexes and parses like the source.
func named(t *Template) string {
	var b strings.Builder
	for i, s := range t.slots {
		b.WriteString(t.parts[i])

		if s.isParam() {
			b.WriteString(":" + t.names[s.param])
		}
	}

	b.WriteString(t.parts[len(t.parts)-1])

	return b.String()
}

// checkRoundTrip verifies that writing src's AST loses nothing: the written
// SQL parses back to the same AST.
func checkRoundTrip(tb testing.TB, p *syntax.Parsed, tmpl *Template) {
	tb.Helper()

	sql := named(tmpl)

	back, err := syntax.Parse(sql)
	if err != nil {
		tb.Fatalf("written SQL %q does not parse: %v", sql, err)
	}

	if got, want := syntax.Dump(back), syntax.Dump(p); got != want {
		tb.Fatalf("round trip of %q through %q changed the AST:\n%s\nwant:\n%s", p.Src, sql, got, want)
	}
}

func TestWrite(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"canonical keywords", "select a, b from t where x = :x group by a having count(*) > 1 order by a desc limit :n offset 5",
			"SELECT a, b FROM t WHERE x = :x GROUP BY a HAVING count(*) > 1 ORDER BY a desc LIMIT :n OFFSET 5"},
		{"comments and spacing", "SELECT a,b -- note\n FROM t /* x */ WHERE  y=:y", "SELECT a,b FROM t WHERE y=:y"},
		{"tail clauses keep their order", "SELECT a FROM t OFFSET 10 LIMIT 5", "SELECT a FROM t OFFSET 10 LIMIT 5"},
		{"distinct on", "SELECT DISTINCT ON (a) a, b FROM t", "SELECT DISTINCT ON (a) a, b FROM t"},
		{"empty select list", "SELECT FROM t", "SELECT FROM t"},
		{"subquery kept as written", "SELECT * FROM t WHERE id IN (select id from u where k = :k)",
			"SELECT * FROM t WHERE id IN (select id from u where k = :k)"},
		{"continued string is copied as written", "SELECT 'ab'\n  'cd' FROM t", "SELECT 'ab'\n  'cd' FROM t"},
		{"keyword field names", "SELECT t.case FROM t WHERE t.offset > :n", "SELECT t.case FROM t WHERE t.offset > :n"},
		{"slice bounds hold no params", "SELECT a[lo:hi], a[:lo:hi] FROM t", "SELECT a[lo:hi], a[:lo:hi] FROM t"},
		{"union distinct", "SELECT 1 UNION DISTINCT SELECT 2", "SELECT 1 UNION SELECT 2"},
		{"set op precedence", "SELECT 1 UNION ALL SELECT 2 INTERSECT SELECT 3", "SELECT 1 UNION ALL SELECT 2 INTERSECT SELECT 3"},
		{"parenthesized operands", "(SELECT a FROM t LIMIT 1) UNION (SELECT b FROM u) ORDER BY 1",
			"(SELECT a FROM t LIMIT 1) UNION (SELECT b FROM u) ORDER BY 1"},
		{"values and table", "VALUES (1, :a), (2, :b) UNION TABLE t", "VALUES (1, :a), (2, :b) UNION TABLE t"},
		{"with", `WITH RECURSIVE r (n) AS NOT MATERIALIZED (SELECT 1), "M" AS MATERIALIZED (VALUES (2)) SELECT n FROM r`,
			`WITH RECURSIVE r (n) AS NOT MATERIALIZED (SELECT 1), "M" AS MATERIALIZED (VALUES (2)) SELECT n FROM r`},
		{"insert", "insert into app.t as o (a, tags[:i]) overriding system value values (:a, :t) " +
			"on conflict (a) where b do update set (c, d) = (select 1, 2), e = excluded.e where o.f returning a",
			"INSERT INTO app.t AS o (a, tags[:i]) OVERRIDING SYSTEM VALUE VALUES (:a, :t) " +
				"ON CONFLICT (a) where b DO UPDATE SET (c, d) = (select 1, 2), e = excluded.e WHERE o.f RETURNING a"},
		{"insert default values", "INSERT INTO t DEFAULT VALUES", "INSERT INTO t DEFAULT VALUES"},
		{"insert select on constraint", "INSERT INTO t SELECT * FROM s ON CONFLICT ON CONSTRAINT k DO NOTHING",
			"INSERT INTO t SELECT * FROM s ON CONFLICT ON CONSTRAINT k DO NOTHING"},
		{"update", "update t o set a = 1, (b) = (2) from s where s.id = o.id returning o.id",
			"UPDATE t AS o SET a = 1, (b) = (2) FROM s WHERE s.id = o.id RETURNING o.id"},
		{"delete", "delete from t as o using u where u.id = o.uid returning *",
			"DELETE FROM t AS o USING u WHERE u.id = o.uid RETURNING *"},
		{"merge", "merge into t a using s on a.id = s.id " +
			"when matched and s.gone then delete " +
			"when matched then update set v = s.v " +
			"when not matched by source then do nothing " +
			"when not matched then insert (id, v) values (s.id, :v) " +
			"when not matched then insert default values returning a.id",
			"MERGE INTO t AS a USING s ON a.id = s.id " +
				"WHEN MATCHED AND s.gone THEN DELETE " +
				"WHEN MATCHED THEN UPDATE SET v = s.v " +
				"WHEN NOT MATCHED BY SOURCE THEN DO NOTHING " +
				"WHEN NOT MATCHED THEN INSERT (id, v) VALUES (s.id, :v) " +
				"WHEN NOT MATCHED THEN INSERT DEFAULT VALUES RETURNING a.id"},
		{"raw", "create index i on t (a) where b = :b", "create index i on t (a) where b = :b"},
		{"locking, fetch and window", "select sum(a) over w from t window w as (partition by b) " +
			"for update skip locked offset 2 rows fetch first :n rows only",
			"SELECT sum(a) over w FROM t WINDOW w as (partition by b) FOR update skip locked OFFSET 2 rows FETCH first :n rows only"},
		{"only", "update only t set a = 1", "UPDATE ONLY t SET a = 1"},
		{"trailing semicolon", "SELECT 1;", "SELECT 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := parseSQL(t, tt.src)

			tmpl, err := Compile(p)
			if err != nil {
				t.Fatal(err)
			}

			if got := named(tmpl); got != tt.want {
				t.Errorf("written SQL:\n%s\nwant:\n%s", got, tt.want)
			}

			checkRoundTrip(t, p, tmpl)
		})
	}
}

// corpus covers every statement kind and clause the parser models.
var corpus = []string{
	"SELECT 1",
	"SELECT a, b FROM t WHERE x = :x GROUP BY a HAVING count(*) > 1 ORDER BY a DESC LIMIT :n OFFSET 5",
	"SELECT DISTINCT a FROM t",
	"SELECT ALL a FROM t",
	"SELECT ALL ALL",
	"SELECT * FROM a JOIN b ON a.id = b.id LEFT JOIN c USING (id) WHERE a.x = :x",
	"SELECT a IS DISTINCT FROM b, c IS NOT DISTINCT FROM d FROM t",
	"SELECT row_number() OVER (PARTITION BY a ORDER BY b), count(*) FILTER (WHERE c), EXTRACT(YEAR FROM d), " +
		"percentile_cont(0.5) WITHIN GROUP (ORDER BY e), CASE WHEN f THEN 'x' ELSE CASE g WHEN 1 THEN 'y' END END FROM t",
	"SELECT a[1], ARRAY[b, c] FROM t WHERE d = ANY(:ds)",
	"SELECT a, (SELECT max(b) FROM u WHERE u.a = t.a) AS m FROM t WHERE EXISTS (SELECT 1 FROM v WHERE v.k = :k)",
	"SELECT * FROM (SELECT a FROM t LIMIT :n) s, LATERAL (VALUES (s.a)) v(x)",
	"SELECT 1 EXCEPT SELECT 2 UNION SELECT 3",
	"SELECT 1 INTERSECT SELECT 2 UNION SELECT 3",
	"SELECT 1 INTERSECT (SELECT 2 UNION SELECT 3)",
	"TABLE app.orders ORDER BY id LIMIT :n",
	"WITH c AS (SELECT 1 UNION ALL SELECT 2) SELECT * FROM c",
	"WITH x AS (SELECT 1) UPDATE t SET a = 1",
	"INSERT INTO orders (user_id, total) VALUES (:user, :total), (:user, 0) RETURNING id",
	`INSERT INTO app."Orders" AS o VALUES (1)`,
	"INSERT INTO t (a) SELECT a FROM s WHERE b IN (:bs)",
	"INSERT INTO t (SELECT a FROM s)",
	"INSERT INTO t WITH x AS (SELECT 1) SELECT * FROM x",
	"INSERT INTO users (id, name) SELECT s.id, s.name FROM staging s JOIN batches b ON b.id = s.batch_id " +
		"WHERE s.active = true ON CONFLICT (id) DO UPDATE SET name = excluded.name RETURNING id",
	"INSERT INTO t SELECT 1 UNION SELECT 2 ON CONFLICT DO NOTHING",
	"INSERT INTO t (addr.city, tags[:i]) VALUES (:city, :tag)",
	"UPDATE orders SET status = :status WHERE id IN (:ids)",
	"UPDATE t SET a = coalesce(:a, a), b = DEFAULT, c = CASE WHEN d THEN 1 ELSE 2 END WHERE e",
	"UPDATE t SET (a, b) = (SELECT x, y FROM s WHERE s.id = t.id)",
	"UPDATE t SET addr.city = :city, tags[1] = 'a', data['k'][:i] = :v, grid[1:2] = :g",
	"UPDATE t SET a = s.a FROM s JOIN u ON u.id = s.uid WHERE s.id = t.id AND u.org = :org RETURNING t.id",
	"DELETE FROM orders o USING users u WHERE u.id = o.user_id AND o.id NOT IN (:keep) RETURNING o.id",
	"WITH old AS (SELECT id FROM t WHERE created < :cutoff) DELETE FROM t WHERE id IN (SELECT id FROM old)",
	"WITH src AS (SELECT * FROM staging WHERE batch = :batch) " +
		"MERGE INTO app.accounts AS a USING (SELECT * FROM src WHERE ok) AS s ON a.id = s.id " +
		"WHEN MATCHED AND s.deleted THEN DELETE " +
		"WHEN NOT MATCHED BY TARGET AND s.id IN (:ids) THEN INSERT (id, v) OVERRIDING SYSTEM VALUE VALUES (s.id, s.v) " +
		"RETURNING merge_action(), a.id",
	"CREATE TABLE t AS (SELECT a FROM s)",
	"SELECT id FROM jobs WHERE state = :s ORDER BY id LIMIT 10 FOR UPDATE OF jobs SKIP LOCKED",
	"SELECT a FROM t ORDER BY a OFFSET 5 ROWS FETCH NEXT :n ROWS WITH TIES",
	"SELECT sum(a) OVER w FROM t WINDOW w AS (PARTITION BY b)",
	"DELETE FROM ONLY t WHERE a",
	"MERGE INTO ONLY t USING s ON a WHEN MATCHED THEN DELETE",
	"CALL proc(:a)",
	"TABLE ONLY a UNION TABLE b *",
	"UPDATE t * SET a = 1",
	"SELECT * FROM ROWS FROM (f(1), g(2)) WITH ORDINALITY AS z(a, b, n)",
	"WITH RECURSIVE g (f) AS (SELECT 1 UNION ALL SELECT f FROM g) SEARCH BREADTH FIRST BY f SET seq CYCLE f SET c USING p TABLE g",
	"MERGE INTO t USING s1 JOIN s2 ON s1.id = s2.id ON t.id = s1.id WHEN MATCHED THEN DELETE",
	"SELECT 'a'\n  'b', E'it\\'s', $$x$$ FROM t",
}

func TestWriteRoundTrip(t *testing.T) {
	for _, src := range corpus {
		p := parseSQL(t, src)

		tmpl, err := Compile(p)
		if err != nil {
			t.Fatalf("Compile(%q): %v", src, err)
		}

		checkRoundTrip(t, p, tmpl)
	}
}

func TestWriteSlots(t *testing.T) {
	tmpl := compileSQL(t, "SELECT :a FROM t WHERE b IN (:bs) AND c = :a AND d = ANY(:bs)")

	var params []slot
	for _, s := range tmpl.slots {
		if s.isParam() {
			params = append(params, s)
		}
	}

	want := []slot{{param: 0}, {param: 1, expand: true}, {param: 0}, {param: 1}}
	if !slices.Equal(params, want) {
		t.Errorf("param slots = %+v, want %+v", params, want)
	}

	if got := strings.Join(tmpl.Names(), ","); got != "a,bs" {
		t.Errorf("Names = %s, want a,bs", got)
	}
}

func TestWriteParenthesizesBuiltTrees(t *testing.T) {
	// Trees the parser never builds, as composition might: each needs parens.
	p := parseSQL(t, "SELECT 1 UNION SELECT 2")
	one := p.Stmt.(*syntax.Query).Body.(*syntax.SetOp).Left
	two := p.Stmt.(*syntax.Query).Body.(*syntax.SetOp).Right

	tests := []struct {
		body syntax.SetExpr
		want string
	}{
		{&syntax.SetOp{Op: syntax.Union, Left: one, Right: &syntax.SetOp{Op: syntax.Except, Left: one, Right: two}},
			"SELECT 1 UNION (SELECT 1 EXCEPT SELECT 2)"},
		{&syntax.SetOp{Op: syntax.Intersect, Left: &syntax.SetOp{Op: syntax.Union, Left: one, Right: two}, Right: two},
			"(SELECT 1 UNION SELECT 2) INTERSECT SELECT 2"},
		{&syntax.SetOp{Op: syntax.Union, Left: &syntax.SetOp{Op: syntax.Union, Left: one, Right: two}, Right: two},
			"SELECT 1 UNION SELECT 2 UNION SELECT 2"},
	}

	for _, tt := range tests {
		built := *p
		built.Stmt = &syntax.Query{Body: tt.body}

		tmpl, err := Compile(&built)
		if err != nil {
			t.Fatal(err)
		}

		if got := named(tmpl); got != tt.want {
			t.Errorf("written SQL = %q, want %q", got, tt.want)
		}
	}
}

func TestWriteRejectsUnknownNodes(t *testing.T) {
	p := parseSQL(t, "SELECT 1")
	p.Stmt = nil

	if _, err := Compile(p); err == nil || err.Error() != "cannot render <nil>" {
		t.Errorf("Compile error = %v", err)
	}
}
