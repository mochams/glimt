package syntax

import "testing"

// Every node kind satisfies the interfaces the parser returns it through.
var (
	_ Statement = (*Query)(nil)
	_ Statement = (*Insert)(nil)
	_ Statement = (*Update)(nil)
	_ Statement = (*Delete)(nil)
	_ Statement = (*Merge)(nil)
	_ Statement = (*Raw)(nil)
	_ SetExpr   = (*Select)(nil)
	_ SetExpr   = (*SetOp)(nil)
	_ SetExpr   = (*Values)(nil)
	_ SetExpr   = (*TableQuery)(nil)
	_ SetExpr   = (*ParenQuery)(nil)
	_ Node      = (*Expr)(nil)
	_ Node      = (*With)(nil)
	_ Node      = (*CTE)(nil)
	_ Node      = (*OnConflict)(nil)
	_ Node      = (*Assignment)(nil)
	_ Node      = (*Subquery)(nil)
	_ Node      = (*ObjectName)(nil)
	_ Node      = (*Target)(nil)
	_ Node      = (*MergeWhen)(nil)
)

func TestSpan(t *testing.T) {
	s := Span{From: 2, To: 5}

	tests := []struct {
		name      string
		got, want bool
	}{
		{"non-empty", s.Empty(), false},
		{"empty", Span{From: 3, To: 3}.Empty(), true},
		{"contains itself", s.Contains(s), true},
		{"contains inner", s.Contains(Span{From: 3, To: 4}), true},
		{"contains empty inner", s.Contains(Span{From: 5, To: 5}), true},
		{"overlaps start", s.Contains(Span{From: 1, To: 3}), false},
		{"overlaps end", s.Contains(Span{From: 4, To: 6}), false},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, tt.got, tt.want)
		}
	}

	if s.Bounds() != s {
		t.Errorf("Bounds() = %v, want %v", s.Bounds(), s)
	}
}

func TestEnumStrings(t *testing.T) {
	tests := []struct {
		got, want string
	}{
		{Union.String(), "UNION"},
		{Intersect.String(), "INTERSECT"},
		{Except.String(), "EXCEPT"},
		{SetOpKind(0).String(), "SetOpKind(?)"},
		{Scalar.String(), "scalar"},
		{Expand.String(), "expand"},
		{Matched.String(), "matched"},
		{NotMatched.String(), "not matched"},
		{NotMatchedBySource.String(), "not matched by source"},
		{MergeMatch(0).String(), "MergeMatch(?)"},
		{MergeDoNothing.String(), "do nothing"},
		{MergeUpdate.String(), "update"},
		{MergeDelete.String(), "delete"},
		{MergeInsert.String(), "insert"},
		{MergeAction(0).String(), "MergeAction(?)"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestIdentNames(t *testing.T) {
	p, err := parseSQL(t, `UPDATE "My""Table" AS plain SET "Col" = 1`)
	if err != nil {
		t.Fatal(err)
	}

	u := p.Stmt.(*Update)

	tests := []struct {
		id     Ident
		name   string
		quoted bool
	}{
		{u.Table.Parts[0], `My"Table`, true},
		{*u.Alias, "plain", false},
		{u.Set[0].Targets[0].Column, "Col", true},
	}

	for _, tt := range tests {
		if tt.id.Name != tt.name || tt.id.Quoted != tt.quoted {
			t.Errorf("ident = {%q %v}, want {%q %v}", tt.id.Name, tt.id.Quoted, tt.name, tt.quoted)
		}
	}
}
