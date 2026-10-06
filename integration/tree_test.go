package integration

import "testing"

func TestPGTreeIgnoresPositions(t *testing.T) {
	a, err := pgTree("SELECT a FROM t WHERE b = 1")
	if err != nil {
		t.Fatal(err)
	}

	b, err := pgTree("select   a\nfrom t where (b = 1)")
	if err != nil {
		t.Fatal(err)
	}

	if a != b {
		t.Errorf("trees differ:\n%s\n%s", a, b)
	}

	if c, _ := pgTree("SELECT a FROM t WHERE b = 2"); c == a {
		t.Error("different statements give the same tree")
	}
}

func TestStatementKind(t *testing.T) {
	tests := []struct {
		sql, want string
	}{
		{"SELECT 1", "SelectStmt"},
		{"VALUES (1)", "SelectStmt"},
		{"INSERT INTO t VALUES (1)", "InsertStmt"},
		{"UPDATE t SET a = 1", "UpdateStmt"},
		{"DELETE FROM t", "DeleteStmt"},
		{"MERGE INTO t USING s ON true WHEN MATCHED THEN DELETE", "MergeStmt"},
		{"CREATE TABLE t (a int)", "other"},
	}

	for _, tt := range tests {
		if got, err := statementKind(tt.sql); err != nil || got != tt.want {
			t.Errorf("statementKind(%q) = %q, %v; want %q", tt.sql, got, err, tt.want)
		}
	}

	if _, err := statementKind("SELECT 1; SELECT 2"); err == nil {
		t.Error("two statements: want an error")
	}

	if _, err := statementKind("SELEC 1"); err == nil {
		t.Error("invalid SQL: want an error")
	}
}
