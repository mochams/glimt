package glimt

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"testing"
)

// update rewrites the golden files from the current output.
var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// goldenQueries are the statements the golden test composes: every clause
// composition can touch, present and absent, in every order Postgres allows.
var goldenQueries = []struct {
	name string
	sql  string
	args Args
}{
	{"every clause", "SELECT DISTINCT a, count(*) FROM t WHERE b = :b GROUP BY a HAVING count(*) > :min " +
		"WINDOW w AS (PARTITION BY a) ORDER BY a LIMIT 10 OFFSET 5 FOR UPDATE", Args{"b": 1, "min": 2}},
	{"no clause", "SELECT 1", nil},
	{"from only", "SELECT a FROM t", nil},
	{"union", "SELECT a FROM t UNION SELECT a FROM u", nil},
	{"union with order", "SELECT a FROM t UNION ALL SELECT a FROM u ORDER BY 1 LIMIT 3", nil},
	{"values", "VALUES (1), (2)", nil},
	{"table", "TABLE t", nil},
	{"parenthesized", "(SELECT a FROM t WHERE p) ORDER BY a", nil},
	{"parenthesized with limit", "(SELECT a FROM t LIMIT 5)", nil},
	{"parenthesized with order and offset", "((SELECT a FROM t ORDER BY a) OFFSET 2 FOR UPDATE)", nil},
	{"parenthesized limit with order outside", "(SELECT a FROM t WHERE b = :b LIMIT :n) ORDER BY a", Args{"b": 1, "n": 2}},
	{"with", "WITH c AS (SELECT a FROM x WHERE y LIMIT 1) SELECT * FROM c WHERE z IN (SELECT 1 WHERE w)", nil},
	{"update with where", "UPDATE t SET a = :a WHERE id = :id RETURNING id", Args{"a": 1, "id": 2}},
	{"update without where", "UPDATE t SET a = 1 FROM s RETURNING a", nil},
	{"delete with where", "DELETE FROM t USING s WHERE x = :x", Args{"x": 1}},
	{"delete without where", "DELETE FROM t RETURNING id", nil},
	{"current of", "UPDATE t SET a = 1 WHERE CURRENT OF c", nil},
	{"insert", "INSERT INTO t (a) VALUES (:a) RETURNING a", Args{"a": 1}},
	{"fetch", "SELECT a FROM t ORDER BY a FETCH FIRST 5 ROWS ONLY", nil},
	{"offset and fetch", "SELECT a FROM t OFFSET 2 FETCH NEXT 5 ROWS WITH TIES", nil},
	{"for before limit", "SELECT a FROM t FOR UPDATE LIMIT 1", nil},
	{"for after limit", "SELECT a FROM t LIMIT 1 FOR SHARE SKIP LOCKED", nil},
	{"offset before limit", "SELECT a FROM t OFFSET 5 LIMIT 1", nil},
	{"limit only", "SELECT a FROM t LIMIT 1", nil},
	{"offset only", "SELECT a FROM t OFFSET 1", nil},
	{"params in the tail", "SELECT a FROM t WHERE b = :b ORDER BY f(:x) LIMIT :n OFFSET :o",
		Args{"b": 1, "x": 2, "n": 3, "o": 4}},
	{"param used in where and tail", "SELECT a FROM t WHERE b = :n ORDER BY a LIMIT :n", Args{"n": 7}},
	{"expanding param", "SELECT a FROM t WHERE id IN (:ids) AND org = :org ORDER BY a", Args{"ids": []int{1, 2}, "org": 3}},
	{"empty list in the tail", "SELECT id FROM users ORDER BY id IN (:ids)", Args{"ids": []int{}}},
}

// goldenOrders are the ways the golden test changes ORDER BY.
var goldenOrders = []string{"keep", "replace", "append"}

// TestGolden renders every golden query as written, composed every way the
// Builder can, and counted, and compares the SQL, args and errors with
// testdata/compose.golden, byte for byte. Run go test -run TestGolden
// -update to rewrite the file after an intended change.
func TestGolden(t *testing.T) {
	var files strings.Builder
	for _, q := range goldenQueries {
		fmt.Fprintf(&files, "-- name: %s\n%s;\n", strings.ReplaceAll(q.name, " ", "_"), q.sql)
	}

	reg := mustLoad(t, map[string]string{"golden.sql": files.String()})

	var out strings.Builder

	for _, q := range goldenQueries {
		query := reg.Get(strings.ReplaceAll(q.name, " ", "_"))

		sql, args, err := query.Build(q.args)
		writeGolden(&out, q.name+": as written", sql, args, err)

		for combo := range 2 * len(goldenOrders) * 2 * 2 * 2 {
			name, b := goldenBuilder(query.Bind(q.args), combo)
			if combo&1 != 0 {
				sql, args, err = b.BuildCount()
			} else {
				sql, args, err = b.Build()
			}

			writeGolden(&out, q.name+": "+name, sql, args, err)
		}
	}

	compareGolden(t, "testdata/compose.golden", out.String())
}

// goldenBuilder composes b as the bits of combo choose, and names the
// composition.
func goldenBuilder(b Builder, combo int) (string, Builder) {
	var name []string

	count := combo&1 != 0
	combo >>= 1

	if combo&1 != 0 {
		b = b.Where(Eq("z", "w"), In("y", []int{8, 9}))
		name = append(name, "where")
	}

	combo >>= 1

	switch order := goldenOrders[combo%len(goldenOrders)]; order {
	case "replace":
		b = b.OrderBy(Desc("z"))
		name = append(name, "order by")
	case "append":
		b = b.ThenBy(Asc("y").NullsLast())
		name = append(name, "then by")
	}

	combo /= len(goldenOrders)

	if combo&1 != 0 {
		b = b.Limit(10)
		name = append(name, "limit")
	}

	if combo&2 != 0 {
		b = b.Offset(20)
		name = append(name, "offset")
	}

	if count {
		name = append(name, "count")
	}

	if len(name) == 0 {
		return "bound", b
	}

	return strings.Join(name, ", "), b
}

// writeGolden writes one golden entry: its name, then the SQL and args, or
// the error.
func writeGolden(out *strings.Builder, name, sql string, args []any, err error) {
	fmt.Fprintf(out, "## %s\n", name)

	if err != nil {
		fmt.Fprintf(out, "error: %v\n\n", err)

		return
	}

	fmt.Fprintf(out, "%s\n%#v\n\n", sql, args)
}

// compareGolden compares got with the golden file at path, or rewrites the
// file with -update.
func compareGolden(t *testing.T, path, got string) {
	t.Helper()

	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v: run go test -run TestGolden -update to create it", err)
	}

	if got == string(want) {
		return
	}

	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(string(want), "\n")
	for i := range min(len(gotLines), len(wantLines)) {
		if gotLines[i] != wantLines[i] {
			t.Fatalf("%s differs at line %d:\ngot  %s\nwant %s", path, i+1, gotLines[i], wantLines[i])
		}
	}

	t.Fatalf("%s differs in length: got %d lines, want %d", path, len(gotLines), len(wantLines))
}
