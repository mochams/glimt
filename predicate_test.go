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
			wantSQL:   "age > ?",
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
			predicate: And(Cond("age > ?", 30)),
			wantSQL:   "age > ?",
			wantArgs:  []any{30},
		},
		{
			name: "AND combination",
			predicate: And(
				Cond("age > ?", 30),
				Cond("status = ?", "active"),
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
			predicate: Or(Cond("age < ?", 18)),
			wantSQL:   "age < ?",
			wantArgs:  []any{18},
		},
		{
			name: "OR combination",
			predicate: Or(
				Cond("age < ?", 18),
				Cond("age > ?", 65),
			),
			wantSQL:  "(age < ? OR age > ?)",
			wantArgs: []any{18, 65},
		},
		{
			name:      "empty In",
			predicate: In("age"),
			wantSQL:   "",
			wantArgs:  nil,
		},
		{
			name:      "In with values",
			predicate: In("age", 30, 40, 50),
			wantSQL:   "age IN (?, ?, ?)",
			wantArgs:  []any{30, 40, 50},
		},
		{
			name:      "empty NotIN",
			predicate: NotIn("age"),
			wantSQL:   "",
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &sqlBuilder{}
			tt.predicate(b)
			assertSQL(t, b.string(), tt.wantSQL)
			assertArgs(t, b.args, tt.wantArgs)
		})
	}
}
