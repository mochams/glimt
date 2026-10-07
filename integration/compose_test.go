package integration

import (
	"testing"

	"github.com/mochams/glimt/internal/syntax"
)

func TestCheckComposed(t *testing.T) {
	for _, src := range []string{
		"SELECT a FROM t WHERE b = 1 AND c = 2 ORDER BY a LIMIT 5",
		"SELECT a FROM t GROUP BY a HAVING count(*) > 1",
		"SELECT 1 UNION SELECT 2",
		"UPDATE t SET a = 1 WHERE b OR c RETURNING a",
		"DELETE FROM t USING s",
		"SELECT * FROM a, b FOR UPDATE OF a NOWAIT LIMIT 1",
		"SELECT collation for ('x')",
		"INSERT INTO t VALUES (1)",
		"UPDATE t SET a = 1 WHERE CURRENT OF c",
	} {
		p, err := syntax.Parse(src)
		if err != nil {
			t.Fatal(err)
		}

		for _, f := range checkComposed(p, src) {
			t.Errorf("%s: %s\n%q", f.check, f.msg, src)
		}
	}
}

func TestExpectedTreeFlattensAnd(t *testing.T) {
	want, err := expectedTree(checkComposeWhere, "SELECT 1 WHERE a AND b")
	if err != nil {
		t.Fatal(err)
	}

	got, err := normalizedTree("SELECT 1 WHERE (a AND b) AND (glimt_c IS NULL)")
	if err != nil {
		t.Fatal(err)
	}

	if got != want {
		t.Errorf("expected tree:\n%s\nPostgres:\n%s", want, got)
	}
}
