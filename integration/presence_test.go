package integration

import (
	"testing"

	"github.com/mochams/glimt/internal/syntax"
)

func TestShapes(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"SELECT a FROM t WHERE b GROUP BY c HAVING d WINDOW w AS () ORDER BY a LIMIT 1 OFFSET 2 FOR UPDATE",
			"select{fromClause,groupClause,havingClause,limitCount,limitOffset,lockingClause,sortClause,whereClause,windowClause}"},
		{"(WITH x AS (SELECT 1) SELECT 1 FROM t UNION SELECT 2) ORDER BY 1 LIMIT ALL",
			"select{limitCount,sortClause} cte(select{}) larg(select{fromClause}) rarg(select{})"},
		{"(SELECT a FROM t) FETCH FIRST 1 ROW ONLY", "select{fromClause,limitCount}"},
		{"TABLE t ORDER BY a", "select{fromClause,sortClause}"},
		{"VALUES (1) LIMIT 1", "select{limitCount}"},
		{"WITH c AS (SELECT 1) INSERT INTO t SELECT * FROM c RETURNING a",
			"insert{returningList} cte(select{}) source(select{fromClause})"},
		{"INSERT INTO t DEFAULT VALUES", "insert{}"},
		{"UPDATE t SET a = 1 FROM u WHERE b RETURNING c", "update{fromClause,returningList,whereClause}"},
		{"DELETE FROM t USING u WHERE b", "delete{usingClause,whereClause}"},
		{"MERGE INTO t USING s ON p WHEN MATCHED THEN DELETE", "merge{}"},
		{"SELECT a FROM t WHERE b IN (SELECT c FROM u WHERE d)", "select{fromClause,whereClause}"},
	}

	for _, tt := range tests {
		p, err := syntax.Parse(tt.src)
		if err != nil {
			t.Fatal(err)
		}

		if got := statementShape(p.Stmt).String(); got != tt.want {
			t.Errorf("glimt's shape of %q = %s\nwant %s", tt.src, got, tt.want)
		}

		tree, err := pgTree(tt.src)
		if err != nil {
			t.Fatal(err)
		}

		if f := checkClausePresence(p, tree); f != nil {
			t.Errorf("%q: %s", tt.src, f[0].msg)
		}
	}
}

func TestShapesRaw(t *testing.T) {
	p, err := syntax.Parse("CREATE TABLE t (a int)")
	if err != nil {
		t.Fatal(err)
	}

	if statementShape(p.Stmt) != nil || checkClausePresence(p, "") != nil {
		t.Error("a raw statement has a shape")
	}
}

// TestPresenceFindsFoldedClauses checks that the presence check reports a
// clause glimt read as part of the clause before it, which the clause check
// can't see.
func TestPresenceFindsFoldedClauses(t *testing.T) {
	src := "SELECT count(*) rows FROM t WHERE a = 1"

	p, err := syntax.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	tree, err := pgTree(src)
	if err != nil {
		t.Fatal(err)
	}

	sel := p.Stmt.(*syntax.Query).Body.(*syntax.Select)
	sel.From = nil // as glimt read it before the ROWS FROM fix

	if f := checkClausePresence(p, tree); len(f) != 1 || f[0].check != checkPresence {
		t.Errorf("checkClausePresence = %v, want one presence finding", f)
	}
}
