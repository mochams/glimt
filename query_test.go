package glimt

import (
	"testing"
)

func TestQueryBuild(t *testing.T) {
	tests := []struct {
		name     string
		query    *Query
		wantSQL  string
		wantArgs []any
	}{
		{
			name:    "base SQL only",
			query:   NewQuery("SELECT * FROM users", DialectPostgres),
			wantSQL: "SELECT * FROM users",
		},
		{
			name: "single where condition",
			query: NewQuery(
				"SELECT * FROM users",
				DialectPostgres,
			).Where(Eq("status", "active")),
			wantSQL:  "SELECT * FROM users WHERE status = $1",
			wantArgs: []any{"active"},
		},
		{
			name: "multiple where conditions",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(Eq("status", "active")).
				Where(Gt("age", 30)),
			wantSQL:  "SELECT * FROM users WHERE status = $1 AND age > $2",
			wantArgs: []any{"active", 30},
		},
		{
			name: "empty exclude clause",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Exclude(),
			wantSQL: "SELECT * FROM users",
		},
		{
			name: "single exclude clause",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Exclude(Eq("status", "inactive")),
			wantSQL:  "SELECT * FROM users WHERE NOT (status = $1)",
			wantArgs: []any{"inactive"},
		},
		{
			name: "exclude multiple conditions",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Exclude(
					Eq("status", "inactive"),
					Lt("age", 18),
				),
			wantSQL:  "SELECT * FROM users WHERE NOT ((status = $1 AND age < $2))",
			wantArgs: []any{"inactive", 18},
		},
		{
			name: "group by clause",
			query: NewQuery("SELECT status, COUNT(*) FROM users", DialectPostgres).
				GroupBy("status"),
			wantSQL: "SELECT status, COUNT(*) FROM users GROUP BY status",
		},
		{
			name: "having clause",
			query: NewQuery("SELECT status, COUNT(*) FROM users", DialectPostgres).
				GroupBy("status").
				Having(Gt("COUNT(*)", 10)),
			wantSQL:  "SELECT status, COUNT(*) FROM users GROUP BY status HAVING COUNT(*) > $1",
			wantArgs: []any{10},
		},
		{
			name: "order by clause",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				OrderBy("created_at DESC"),
			wantSQL: "SELECT * FROM users ORDER BY created_at DESC",
		},
		{
			name: "limit and offset",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Limit(10).
				Offset(20),
			wantSQL:  "SELECT * FROM users LIMIT $1 OFFSET $2",
			wantArgs: []any{10, 20},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotArgs := tt.query.Build()

			assertSQL(t, gotSQL, tt.wantSQL)
			assertArgs(t, gotArgs, tt.wantArgs)
		})
	}
}
