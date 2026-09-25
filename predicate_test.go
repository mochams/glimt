package glimt

import "testing"

func TestPredicate(t *testing.T) {
	tests := []struct {
		name      string
		predicate Predicate
		wantSQL   string
		wantArgs  []any
	}{
		{
			name:      "simple condition",
			predicate: Cond("age > ?", 30),
			wantSQL:   "(age > ?)",
			wantArgs:  []any{30},
		},
		{
			name:      "empty AND",
			predicate: And(),
			wantSQL:   "",
			wantArgs:  nil,
		},
		{
			name:      "single AND",
			predicate: And(Gt("age", 30)),
			wantSQL:   "age > ?",
			wantArgs:  []any{30},
		},
		{
			name: "AND combination",
			predicate: And(
				Gt("age", 30),
				Eq("status", "active"),
			),
			wantSQL:  "(age > ? AND status = ?)",
			wantArgs: []any{30, "active"},
		},
		{
			name:      "empty OR",
			predicate: Or(),
			wantSQL:   "",
			wantArgs:  nil,
		},
		{
			name:      "single OR",
			predicate: Or(Lt("age", 18)),
			wantSQL:   "age < ?",
			wantArgs:  []any{18},
		},
		{
			name: "OR combination",
			predicate: Or(
				Lt("age", 18),
				Gt("age", 65),
			),
			wantSQL:  "(age < ? OR age > ?)",
			wantArgs: []any{18, 65},
		},
		{
			name:      "empty In matches nothing",
			predicate: In[int]("age"),
			wantSQL:   "1=0",
			wantArgs:  nil,
		},
		{
			name:      "In with values",
			predicate: In("age", 30, 40, 50),
			wantSQL:   "age IN (?, ?, ?)",
			wantArgs:  []any{30, 40, 50},
		},
		{
			name:      "empty NotIn matches everything",
			predicate: NotIn[int]("age"),
			wantSQL:   "1=1",
			wantArgs:  nil,
		},
		{
			name:      "NotIN with values",
			predicate: NotIn("age", 30, 40, 50),
			wantSQL:   "age NOT IN (?, ?, ?)",
			wantArgs:  []any{30, 40, 50},
		},
		{
			name:      "Eq",
			predicate: Eq("status", "active"),
			wantSQL:   "status = ?",
			wantArgs:  []any{"active"},
		},
		{
			name:      "Neq",
			predicate: Neq("status", "inactive"),
			wantSQL:   "status <> ?",
			wantArgs:  []any{"inactive"},
		},
		{
			name:      "Gt",
			predicate: Gt("age", 30),
			wantSQL:   "age > ?",
			wantArgs:  []any{30},
		},
		{
			name:      "Gte",
			predicate: Gte("age", 30),
			wantSQL:   "age >= ?",
			wantArgs:  []any{30},
		},
		{
			name:      "Lt",
			predicate: Lt("age", 65),
			wantSQL:   "age < ?",
			wantArgs:  []any{65},
		},
		{
			name:      "Lte",
			predicate: Lte("age", 65),
			wantSQL:   "age <= ?",
			wantArgs:  []any{65},
		},
		{
			name:      "Null",
			predicate: Null("deleted_at"),
			wantSQL:   "deleted_at IS NULL",
			wantArgs:  nil,
		},
		{
			name:      "NotNull",
			predicate: NotNull("email_verified_at"),
			wantSQL:   "email_verified_at IS NOT NULL",
			wantArgs:  nil,
		},
		{
			name:      "Like",
			predicate: Like("name", "%doe%"),
			wantSQL:   "name LIKE ?",
			wantArgs:  []any{"%doe%"},
		},
		{
			name:      "NotLike",
			predicate: NotLike("name", "%doe%"),
			wantSQL:   "name NOT LIKE ?",
			wantArgs:  []any{"%doe%"},
		},
		{
			name:      "ILike",
			predicate: ILike("name", "%doe%"),
			wantSQL:   "name ILIKE ?",
			wantArgs:  []any{"%doe%"},
		},
		{
			name:      "Between",
			predicate: Between("age", 18, 65),
			wantSQL:   "age BETWEEN ? AND ?",
			wantArgs:  []any{18, 65},
		},
		{
			name:      "NotBetween",
			predicate: NotBetween("age", 18, 65),
			wantSQL:   "age NOT BETWEEN ? AND ?",
			wantArgs:  []any{18, 65},
		},
		{
			name:      "RangeOpen",
			predicate: RangeOpen("age", 18, 65),
			wantSQL:   "age > ? AND age < ?",
			wantArgs:  []any{18, 65},
		},

		// --- escaped search ---
		{
			name:      "Contains escapes wildcards",
			predicate: Contains("name", "50%_off!"),
			wantSQL:   "name LIKE ? ESCAPE '!'",
			wantArgs:  []any{"%50!%!_off!!%"},
		},
		{
			name:      "Contains plain text",
			predicate: Contains("name", "doe"),
			wantSQL:   "name LIKE ? ESCAPE '!'",
			wantArgs:  []any{"%doe%"},
		},
		{
			name:      "StartsWith escapes wildcards",
			predicate: StartsWith("sku", "AB_"),
			wantSQL:   "sku LIKE ? ESCAPE '!'",
			wantArgs:  []any{"AB!_%"},
		},
		{
			name:      "EndsWith escapes wildcards",
			predicate: EndsWith("email", "%@example.com"),
			wantSQL:   "email LIKE ? ESCAPE '!'",
			wantArgs:  []any{"%!%@example.com"},
		},
		{
			name:      "backslash is not special",
			predicate: Contains("path", `C:\dir`),
			wantSQL:   "path LIKE ? ESCAPE '!'",
			wantArgs:  []any{`%C:\dir%`},
		},

		// --- generic In / NotIn ---
		{
			name:      "In spreads a typed int slice",
			predicate: In("id", []int{1, 2, 3}...),
			wantSQL:   "id IN (?, ?, ?)",
			wantArgs:  []any{1, 2, 3},
		},
		{
			name:      "In spreads a typed string slice",
			predicate: In("role", []string{"admin", "mod"}...),
			wantSQL:   "role IN (?, ?)",
			wantArgs:  []any{"admin", "mod"},
		},
		{
			name:      "In spreads an any slice",
			predicate: In("x", []any{1, "a"}...),
			wantSQL:   "x IN (?, ?)",
			wantArgs:  []any{1, "a"},
		},
		{
			name:      "In with an empty typed slice matches nothing",
			predicate: In("id", []int64{}...),
			wantSQL:   "1=0",
		},
		{
			name:      "NotIn spreads a typed slice",
			predicate: NotIn("status", []string{"banned", "deleted"}...),
			wantSQL:   "status NOT IN (?, ?)",
			wantArgs:  []any{"banned", "deleted"},
		},

		// --- empty-safe composition ---
		{
			name:      "Cond with OR is parenthesized",
			predicate: Cond("a = ? OR b = ?", 1, 2),
			wantSQL:   "(a = ? OR b = ?)",
			wantArgs:  []any{1, 2},
		},
		{
			name:      "blank Cond renders nothing and drops its args",
			predicate: And(Cond("  ", 1), Eq("a", 2)),
			wantSQL:   "a = ?",
			wantArgs:  []any{2},
		},
		{
			name:      "AND skips nil predicates",
			predicate: And(nil, Eq("a", 1), nil),
			wantSQL:   "a = ?",
			wantArgs:  []any{1},
		},
		{
			name:      "AND skips empty children and keeps the separator correct",
			predicate: And(And(), Eq("a", 1), Or(), Eq("b", 2)),
			wantSQL:   "(a = ? AND b = ?)",
			wantArgs:  []any{1, 2},
		},
		{
			name:      "AND with empty In keeps the literal",
			predicate: And(Eq("a", 1), In[int]("b")),
			wantSQL:   "(a = ? AND 1=0)",
			wantArgs:  []any{1},
		},
		{
			name:      "OR of only nil predicates renders nothing",
			predicate: Or(nil, nil),
			wantSQL:   "",
		},
		{
			name:      "nested empty groups render nothing",
			predicate: And(Or(And(), nil), And()),
			wantSQL:   "",
		},
		{
			name:      "NOT of empty renders nothing",
			predicate: Not(And()),
			wantSQL:   "",
		},
		{
			name:      "NOT of nil renders nothing",
			predicate: Not(nil),
			wantSQL:   "",
		},
		{
			name:      "NOT of empty NotIn fails closed",
			predicate: Not(NotIn[int]("org_id")),
			wantSQL:   "NOT (1=1)",
		},
		{
			name:      "If true keeps the predicate",
			predicate: If(true, Eq("a", 1)),
			wantSQL:   "a = ?",
			wantArgs:  []any{1},
		},
		{
			name:      "If false inside AND is skipped",
			predicate: And(If(false, Eq("a", 1)), Eq("b", 2)),
			wantSQL:   "b = ?",
			wantArgs:  []any{2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &sqlBuilder{}
			b.render(tt.predicate)
			assertSQL(t, b.string(), tt.wantSQL)
			assertArgs(t, b.args, tt.wantArgs)
		})
	}
}
