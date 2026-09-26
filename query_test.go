package glimt

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// Helpers

func assertArgs(t *testing.T, gotArgs, wantArgs []any) {
	t.Helper()

	if len(gotArgs) != len(wantArgs) {
		t.Errorf("args length: got %d, want %d\ngot  %v\nwant %v",
			len(gotArgs), len(wantArgs), gotArgs, wantArgs)

		return
	}

	for i := range gotArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Errorf("arg[%d]: got %v, want %v", i, gotArgs[i], wantArgs[i])
		}
	}
}

func assertSQL(t *testing.T, got, want string) {
	t.Helper()

	if got != want {
		t.Errorf("SQL: got %q, want %q", got, want)
	}
}

// Tests

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
		{
			name:     "with explicit args",
			query:    NewQuery("INSERT INTO users VALUES (?, ?)", DialectPostgres).Args("doe", 42),
			wantSQL:  "INSERT INTO users VALUES ($1, $2)",
			wantArgs: []any{"doe", 42},
		},
		{
			name: "query with CTE",
			query: NewQuery("WITH active_users AS (SELECT * FROM users WHERE deleted_at IS NULL) SELECT * FROM active_users", DialectPostgres).
				Where(Eq("status", "active")).
				Where(Gt("age", 18)).
				OrderBy("created_at DESC").
				Limit(10),
			wantSQL:  "WITH active_users AS (SELECT * FROM users WHERE deleted_at IS NULL) SELECT * FROM active_users WHERE status = $1 AND age > $2 ORDER BY created_at DESC LIMIT $3",
			wantArgs: []any{"active", 18, 10},
		},

		{
			name:     "zero Query does not panic",
			query:    (&Query{}).Where(Eq("a", 1)),
			wantSQL:  " WHERE a = $1",
			wantArgs: []any{1},
		},

		// --- ORDER BY expressions ---
		{
			name: "OrderByExpr args follow WHERE args and precede LIMIT",
			query: NewQuery("SELECT * FROM docs", DialectPostgres).
				Where(Eq("org_id", 7)).
				OrderByExpr("ts_rank(search_vector, to_tsquery(?)) DESC", "cats").
				OrderBy("id").
				Limit(10),
			wantSQL:  "SELECT * FROM docs WHERE org_id = $1 ORDER BY ts_rank(search_vector, to_tsquery($2)) DESC, id LIMIT $3",
			wantArgs: []any{7, "cats", 10},
		},
		{
			name: "OrderBy and OrderByExpr keep call order",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				OrderBy("status").
				OrderByExpr("CASE WHEN id = ? THEN 0 ELSE 1 END", 42).
				OrderBy("name ASC", "id"),
			wantSQL:  "SELECT * FROM users ORDER BY status, CASE WHEN id = $1 THEN 0 ELSE 1 END, name ASC, id",
			wantArgs: []any{42},
		},
		{
			name: "OrderByExpr args follow HAVING args",
			query: NewQuery("SELECT status, COUNT(*) FROM users", DialectPostgres).
				GroupBy("status").
				Having(Gt("COUNT(*)", 1)).
				OrderByExpr("COUNT(*) > ? DESC", 10),
			wantSQL:  "SELECT status, COUNT(*) FROM users GROUP BY status HAVING COUNT(*) > $1 ORDER BY COUNT(*) > $2 DESC",
			wantArgs: []any{1, 10},
		},
		{
			name:    "blank order terms are ignored",
			query:   NewQuery("SELECT * FROM users", DialectPostgres).OrderBy("", "  ").OrderByExpr(" ", 1),
			wantSQL: "SELECT * FROM users",
		},
		{
			name:     "mysql OrderByExpr keeps ? placeholders",
			query:    NewQuery("SELECT * FROM users", DialectMySQL).OrderByExpr("FIELD(status, ?, ?)", "active", "idle"),
			wantSQL:  "SELECT * FROM users ORDER BY FIELD(status, ?, ?)",
			wantArgs: []any{"active", "idle"},
		},

		// --- subqueries ---
		{
			name: "InQuery numbers inner and outer placeholders once",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(Eq("status", "active")).
				Where(InQuery("id", NewQuery("SELECT user_id FROM trip_users", DialectPostgres).
					Where(Eq("trip_id", 42), Eq("role", "driver")))).
				Limit(10),
			wantSQL: "SELECT * FROM users WHERE status = $1 AND " +
				"id IN (SELECT user_id FROM trip_users WHERE trip_id = $2 AND role = $3) LIMIT $4",
			wantArgs: []any{"active", 42, "driver", 10},
		},
		{
			name: "InQuery keeps the inner query's Args and marker",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(InQuery("id", NewQuery("SELECT user_id FROM trip_users WHERE trip_id = ? /* :and */", DialectPostgres).
					Args(42).
					Where(Eq("role", "driver")))).
				Where(Eq("status", "active")),
			wantSQL:  "SELECT * FROM users WHERE id IN (SELECT user_id FROM trip_users WHERE trip_id = $1 AND role = $2) AND status = $3",
			wantArgs: []any{42, "driver", "active"},
		},
		{
			name: "Exists with a correlated subquery",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(Exists(NewQuery("SELECT 1 FROM orders o WHERE o.user_id = users.id /* :and */", DialectPostgres).
					Where(Eq("o.status", "paid")))),
			wantSQL:  "SELECT * FROM users WHERE EXISTS (SELECT 1 FROM orders o WHERE o.user_id = users.id AND o.status = $1)",
			wantArgs: []any{"paid"},
		},
		{
			name: "Not Exists",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(Not(Exists(NewQuery("SELECT 1 FROM bans b WHERE b.user_id = users.id", DialectPostgres)))),
			wantSQL: "SELECT * FROM users WHERE NOT (EXISTS (SELECT 1 FROM bans b WHERE b.user_id = users.id))",
		},
		{
			name: "subquery inside OR",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(Or(Eq("role", "admin"), InQuery("id", NewQuery("SELECT user_id FROM staff", DialectPostgres)))),
			wantSQL:  "SELECT * FROM users WHERE (role = $1 OR id IN (SELECT user_id FROM staff))",
			wantArgs: []any{"admin"},
		},
		{
			name: "nested subqueries",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(InQuery("id", NewQuery("SELECT user_id FROM trip_users", DialectPostgres).
					Where(InQuery("trip_id", NewQuery("SELECT id FROM trips", DialectPostgres).
						Where(Eq("city", "Nairobi")))))).
				Where(Eq("status", "active")),
			wantSQL: "SELECT * FROM users WHERE id IN (SELECT user_id FROM trip_users WHERE " +
				"trip_id IN (SELECT id FROM trips WHERE city = $1)) AND status = $2",
			wantArgs: []any{"Nairobi", "active"},
		},
		{
			name: "subquery with LIMIT",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(InQuery("id", NewQuery("SELECT user_id FROM logins", DialectPostgres).OrderBy("at DESC").Limit(5))).
				Limit(20),
			wantSQL:  "SELECT * FROM users WHERE id IN (SELECT user_id FROM logins ORDER BY at DESC LIMIT $1) LIMIT $2",
			wantArgs: []any{5, 20},
		},
		{
			name: "subquery ?? escape is converted once",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(InQuery("id", NewQuery("SELECT user_id FROM prefs WHERE data ?? 'beta'", DialectPostgres))).
				Where(Eq("status", "active")),
			wantSQL:  "SELECT * FROM users WHERE id IN (SELECT user_id FROM prefs WHERE data ? 'beta') AND status = $1",
			wantArgs: []any{"active"},
		},
		{
			name: "mysql subquery keeps ? placeholders",
			query: NewQuery("SELECT * FROM users", DialectMySQL).
				Where(InQuery("id", NewQuery("SELECT user_id FROM trip_users", DialectMySQL).Where(Eq("trip_id", 42)))),
			wantSQL:  "SELECT * FROM users WHERE id IN (SELECT user_id FROM trip_users WHERE trip_id = ?)",
			wantArgs: []any{42},
		},

		// --- pagination per dialect ---
		{
			name:     "postgres: offset without limit",
			query:    NewQuery("SELECT * FROM users", DialectPostgres).Offset(20),
			wantSQL:  "SELECT * FROM users OFFSET $1",
			wantArgs: []any{20},
		},
		{
			name:     "mysql: offset without limit gets the maximum limit",
			query:    NewQuery("SELECT * FROM users", DialectMySQL).Offset(20),
			wantSQL:  "SELECT * FROM users LIMIT 18446744073709551615 OFFSET ?",
			wantArgs: []any{20},
		},
		{
			name:     "sqlite: offset without limit gets LIMIT -1",
			query:    NewQuery("SELECT * FROM users", DialectSQLite).Offset(20),
			wantSQL:  "SELECT * FROM users LIMIT -1 OFFSET ?",
			wantArgs: []any{20},
		},
		{
			name:     "mysql: limit and offset",
			query:    NewQuery("SELECT * FROM users", DialectMySQL).Limit(10).Offset(20),
			wantSQL:  "SELECT * FROM users LIMIT ? OFFSET ?",
			wantArgs: []any{10, 20},
		},
		{
			name:     "sqlite: limit only",
			query:    NewQuery("SELECT * FROM users", DialectSQLite).Limit(10),
			wantSQL:  "SELECT * FROM users LIMIT ?",
			wantArgs: []any{10},
		},

		// --- clause markers ---
		{
			name: "where marker renders filters before GROUP BY",
			query: NewQuery("SELECT status, COUNT(*) FROM users /* :where */ GROUP BY status", DialectPostgres).
				Where(Gte("age", 18)),
			wantSQL:  "SELECT status, COUNT(*) FROM users WHERE age >= $1 GROUP BY status",
			wantArgs: []any{18},
		},
		{
			name:    "where marker renders nothing without filters",
			query:   NewQuery("SELECT status, COUNT(*) FROM users /* :where */ GROUP BY status", DialectPostgres),
			wantSQL: "SELECT status, COUNT(*) FROM users GROUP BY status",
		},
		{
			name: "and marker extends a fixed WHERE and interleaves args in order",
			query: NewQuery(
				"SELECT status, COUNT(*) FROM users WHERE org_id = ? /* :and */ GROUP BY status HAVING COUNT(*) > ?",
				DialectPostgres,
			).Args(7, 10).Where(Eq("role", "admin"), Gt("age", 18)),
			wantSQL:  "SELECT status, COUNT(*) FROM users WHERE org_id = $1 AND role = $2 AND age > $3 GROUP BY status HAVING COUNT(*) > $4",
			wantArgs: []any{7, "admin", 18, 10},
		},
		{
			name:     "and marker renders nothing without filters",
			query:    NewQuery("SELECT * FROM users WHERE org_id = ? /* :and */ ORDER BY id", DialectPostgres).Args(7),
			wantSQL:  "SELECT * FROM users WHERE org_id = $1 ORDER BY id",
			wantArgs: []any{7},
		},
		{
			name: "repeated markers render the filters at each one",
			query: NewQuery("SELECT id FROM a /* :where */ UNION ALL SELECT id FROM b /* :where */", DialectPostgres).
				Where(Eq("x", 1)),
			wantSQL:  "SELECT id FROM a WHERE x = $1 UNION ALL SELECT id FROM b WHERE x = $2",
			wantArgs: []any{1, 1},
		},
		{
			name: "order by and limit still append at the end",
			query: NewQuery("SELECT * FROM users WHERE deleted_at IS NULL /* :and */", DialectPostgres).
				Where(Eq("status", "active")).
				OrderBy("id").
				Limit(5),
			wantSQL:  "SELECT * FROM users WHERE deleted_at IS NULL AND status = $1 ORDER BY id LIMIT $2",
			wantArgs: []any{"active", 5},
		},
		{
			name: "args beyond the base placeholders are appended last",
			query: NewQuery("SELECT * FROM users WHERE org_id = ? /* :and */", DialectPostgres).
				Args(7, "extra").
				Where(Eq("status", "active")),
			wantSQL:  "SELECT * FROM users WHERE org_id = $1 AND status = $2",
			wantArgs: []any{7, "active", "extra"},
		},

		// --- empty-safe composition ---
		{
			name:    "empty AND omits WHERE",
			query:   NewQuery("SELECT * FROM users", DialectPostgres).Where(And()),
			wantSQL: "SELECT * FROM users",
		},
		{
			name:    "nil predicate omits WHERE",
			query:   NewQuery("SELECT * FROM users", DialectPostgres).Where(nil),
			wantSQL: "SELECT * FROM users",
		},
		{
			name:    "Where with no arguments omits WHERE",
			query:   NewQuery("SELECT * FROM users", DialectPostgres).Where(),
			wantSQL: "SELECT * FROM users",
		},
		{
			name:    "empty In matches nothing",
			query:   NewQuery("SELECT * FROM users", DialectPostgres).Where(In[string]("role")),
			wantSQL: "SELECT * FROM users WHERE 1=0",
		},
		{
			name: "variadic Where skips optional filters",
			query: NewQuery("SELECT * FROM users", DialectPostgres).Where(
				Eq("status", "active"),
				If(false, Eq("role", "admin")),
				Gt("age", 18),
			),
			wantSQL:  "SELECT * FROM users WHERE status = $1 AND age > $2",
			wantArgs: []any{"active", 18},
		},
		{
			name: "skipped filters keep placeholder numbering correct",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(And(), nil).
				Limit(10),
			wantSQL:  "SELECT * FROM users LIMIT $1",
			wantArgs: []any{10},
		},
		{
			name: "chained Cond with OR keeps precedence",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(Eq("org_id", 7)).
				Where(Cond("role = ? OR vip", "admin")),
			wantSQL:  "SELECT * FROM users WHERE org_id = $1 AND (role = $2 OR vip)",
			wantArgs: []any{7, "admin"},
		},
		{
			name: "chained Having calls accumulate",
			query: NewQuery("SELECT status, COUNT(*) FROM users", DialectPostgres).
				GroupBy("status").
				Having(Gt("COUNT(*)", 10)).
				Having(Lt("COUNT(*)", 100)),
			wantSQL:  "SELECT status, COUNT(*) FROM users GROUP BY status HAVING COUNT(*) > $1 AND COUNT(*) < $2",
			wantArgs: []any{10, 100},
		},
		{
			name: "empty Having omits HAVING",
			query: NewQuery("SELECT status, COUNT(*) FROM users", DialectPostgres).
				GroupBy("status").
				Having(nil, And()),
			wantSQL: "SELECT status, COUNT(*) FROM users GROUP BY status",
		},
		{
			name:    "Exclude of only empty predicates adds nothing",
			query:   NewQuery("SELECT * FROM users", DialectPostgres).Exclude(nil, And()),
			wantSQL: "SELECT * FROM users",
		},
		{
			name:    "Exclude of empty NotIn fails closed",
			query:   NewQuery("SELECT * FROM users", DialectPostgres).Exclude(NotIn[int]("org_id")),
			wantSQL: "SELECT * FROM users WHERE NOT (1=1)",
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

func TestQuery_subqueryRenderedAtBuild(t *testing.T) {
	inner := NewQuery("SELECT user_id FROM trip_users", DialectPostgres)
	outer := NewQuery("SELECT * FROM users", DialectPostgres).Where(InQuery("id", inner))

	// Filters added to the inner query before the outer Build are included.
	inner.Where(Eq("trip_id", 42))

	sql, args := outer.Build()
	assertSQL(t, sql, "SELECT * FROM users WHERE id IN (SELECT user_id FROM trip_users WHERE trip_id = $1)")
	assertArgs(t, args, []any{42})
}

func TestQuery_nilSubqueryPanics(t *testing.T) {
	tests := map[string]func(){
		"InQuery": func() { InQuery("id", nil) },
		"Exists":  func() { Exists(nil) },
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				want := "glimt: " + name + ": nil subquery"
				if got := recover(); got != want {
					t.Errorf("panic: got %v, want %q", got, want)
				}
			}()

			call()
		})
	}
}

func TestQuery_optionalSubqueryWithIf(t *testing.T) {
	tripID := 0 // not set in this request

	sql, _ := NewQuery("SELECT * FROM users", DialectPostgres).
		Where(If(tripID != 0, InQuery("id", NewQuery("SELECT user_id FROM trip_users", DialectPostgres).
			Where(Eq("trip_id", tripID))))).
		Build()
	assertSQL(t, sql, "SELECT * FROM users")
}

func TestParseSort(t *testing.T) {
	allowed := map[string]string{
		"created": "o.created_at",
		"total":   "o.total",
		"name":    "lower(u.name)",
	}

	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr string
	}{
		{name: "empty input", input: "", want: nil},
		{name: "blank input", input: "   ", want: nil},
		{name: "single ascending field", input: "total", want: []string{"o.total ASC"}},
		{name: "descending field", input: "-created", want: []string{"o.created_at DESC"}},
		{name: "explicit ascending field", input: "+total", want: []string{"o.total ASC"}},
		{name: "URL-decoded plus is a space", input: " total", want: []string{"o.total ASC"}},
		{
			name:  "multiple fields keep their order",
			input: "-created,name,total",
			want:  []string{"o.created_at DESC", "lower(u.name) ASC", "o.total ASC"},
		},
		{name: "spaces around fields", input: " -created , total ", want: []string{"o.created_at DESC", "o.total ASC"}},
		{name: "unknown field", input: "created,password", wantErr: `glimt: sort field "password": not allowed`},
		{name: "injection attempt", input: "total;DROP TABLE users", wantErr: `glimt: sort field "total;DROP TABLE users": not allowed`},
		{name: "raw column name is not allowed", input: "o.total", wantErr: `glimt: sort field "o.total": not allowed`},
		{name: "duplicate field", input: "total,-total", wantErr: `glimt: sort field "total": duplicate field`},
		{name: "empty field between commas", input: "total,,created", wantErr: `glimt: sort field "": empty field`},
		{name: "trailing comma", input: "total,", wantErr: `glimt: sort field "": empty field`},
		{name: "bare minus", input: "-", wantErr: `glimt: sort field "-": empty field`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSort(tt.input, allowed)

			if tt.wantErr != "" {
				var sortErr *SortError
				if !errors.As(err, &sortErr) {
					t.Fatalf("expected *SortError, got %v", err)
				}

				assertErrorContains(t, err, tt.wantErr)

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSort_withQuery(t *testing.T) {
	order, err := ParseSort("-created", map[string]string{"created": "created_at"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql, _ := NewQuery("SELECT * FROM orders", DialectPostgres).
		OrderBy(append(order, "id")...).
		Build()
	assertSQL(t, sql, "SELECT * FROM orders ORDER BY created_at DESC, id")
}

func TestQuery_BuildCount(t *testing.T) {
	tests := []struct {
		name     string
		query    *Query
		wantSQL  string
		wantArgs []any
	}{
		{
			name: "drops ORDER BY, LIMIT and OFFSET with their args",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(Eq("status", "active")).
				OrderByExpr("CASE WHEN id = ? THEN 0 ELSE 1 END", 42).
				OrderBy("id").
				Limit(10).
				Offset(20),
			wantSQL:  "SELECT COUNT(*) FROM (SELECT * FROM users WHERE status = $1) AS t",
			wantArgs: []any{"active"},
		},
		{
			name: "keeps a marker and its Args",
			query: NewQuery("SELECT * FROM users WHERE org_id = ? /* :and */", DialectPostgres).
				Args(7).
				Where(Eq("role", "admin")).
				Limit(5),
			wantSQL:  "SELECT COUNT(*) FROM (SELECT * FROM users WHERE org_id = $1 AND role = $2) AS t",
			wantArgs: []any{7, "admin"},
		},
		{
			name: "counts groups",
			query: NewQuery("SELECT status, COUNT(*) FROM users", DialectPostgres).
				GroupBy("status").
				Having(Gt("COUNT(*)", 1)).
				OrderBy("status"),
			wantSQL:  "SELECT COUNT(*) FROM (SELECT status, COUNT(*) FROM users GROUP BY status HAVING COUNT(*) > $1) AS t",
			wantArgs: []any{1},
		},
		{
			name: "keeps a subquery's own LIMIT, which is part of the filter",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(InQuery("id", NewQuery("SELECT user_id FROM logins", DialectPostgres).OrderBy("at DESC").Limit(5))).
				Limit(20),
			wantSQL:  "SELECT COUNT(*) FROM (SELECT * FROM users WHERE id IN (SELECT user_id FROM logins ORDER BY at DESC LIMIT $1)) AS t",
			wantArgs: []any{5},
		},
		{
			name: "keeps ORDER BY written in the base SQL",
			query: NewQuery("SELECT * FROM users /* :where */ ORDER BY name", DialectPostgres).
				Where(Eq("status", "active")),
			wantSQL:  "SELECT COUNT(*) FROM (SELECT * FROM users WHERE status = $1 ORDER BY name) AS t",
			wantArgs: []any{"active"},
		},
		{
			name:     "mysql keeps ? placeholders",
			query:    NewQuery("SELECT id FROM users", DialectMySQL).Where(Eq("status", "active")).Limit(10),
			wantSQL:  "SELECT COUNT(*) FROM (SELECT id FROM users WHERE status = ?) AS t",
			wantArgs: []any{"active"},
		},
		{
			name:    "zero Query does not panic",
			query:   &Query{},
			wantSQL: "SELECT COUNT(*) FROM () AS t",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotArgs := tt.query.BuildCount()

			assertSQL(t, gotSQL, tt.wantSQL)
			assertArgs(t, gotArgs, tt.wantArgs)
		})
	}
}

func TestQuery_BuildCountLeavesQueryIntact(t *testing.T) {
	q := NewQuery("SELECT * FROM users", DialectPostgres).Where(Eq("status", "active")).OrderBy("id").Limit(10)

	q.BuildCount()

	sql, args := q.Build()
	assertSQL(t, sql, "SELECT * FROM users WHERE status = $1 ORDER BY id LIMIT $2")
	assertArgs(t, args, []any{"active", 10})
}

func TestQuery_caseInsensitivePredicates(t *testing.T) {
	tests := []struct {
		name     string
		query    *Query
		wantSQL  string
		wantArgs []any
	}{
		{
			name:     "postgres IContains uses ILIKE",
			query:    NewQuery("SELECT * FROM users", DialectPostgres).Where(IContains("name", "50%")),
			wantSQL:  "SELECT * FROM users WHERE name ILIKE $1 ESCAPE '!'",
			wantArgs: []any{"%50!%%"},
		},
		{
			name:     "mysql IContains uses LIKE",
			query:    NewQuery("SELECT * FROM users", DialectMySQL).Where(IContains("name", "doe")),
			wantSQL:  "SELECT * FROM users WHERE name LIKE ? ESCAPE '!'",
			wantArgs: []any{"%doe%"},
		},
		{
			name:     "sqlite IStartsWith uses LIKE",
			query:    NewQuery("SELECT * FROM products", DialectSQLite).Where(IStartsWith("sku", "AB_")),
			wantSQL:  "SELECT * FROM products WHERE sku LIKE ? ESCAPE '!'",
			wantArgs: []any{"AB!_%"},
		},
		{
			name:     "postgres IEndsWith uses ILIKE",
			query:    NewQuery("SELECT * FROM users", DialectPostgres).Where(IEndsWith("email", "@Example.com")),
			wantSQL:  "SELECT * FROM users WHERE email ILIKE $1 ESCAPE '!'",
			wantArgs: []any{"%@Example.com"},
		},
		{
			name:     "postgres Contains stays case-sensitive",
			query:    NewQuery("SELECT * FROM users", DialectPostgres).Where(Contains("name", "doe")),
			wantSQL:  "SELECT * FROM users WHERE name LIKE $1 ESCAPE '!'",
			wantArgs: []any{"%doe%"},
		},
		{
			name:     "BuildCount keeps the dialect",
			query:    NewQuery("SELECT * FROM users", DialectPostgres).Where(IContains("name", "doe")),
			wantSQL:  "SELECT COUNT(*) FROM (SELECT * FROM users WHERE name ILIKE $1 ESCAPE '!') AS t",
			wantArgs: []any{"%doe%"},
		},
		{
			name: "subquery follows the outer query's dialect",
			query: NewQuery("SELECT * FROM users", DialectPostgres).
				Where(InQuery("id", NewQuery("SELECT user_id FROM notes", DialectMySQL).
					Where(IContains("body", "urgent")).
					Offset(5))),
			wantSQL:  "SELECT * FROM users WHERE id IN (SELECT user_id FROM notes WHERE body ILIKE $1 ESCAPE '!' OFFSET $2)",
			wantArgs: []any{"%urgent%", 5},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			build := tt.query.Build
			if strings.HasPrefix(tt.wantSQL, "SELECT COUNT(*)") {
				build = tt.query.BuildCount
			}

			gotSQL, gotArgs := build()
			assertSQL(t, gotSQL, tt.wantSQL)
			assertArgs(t, gotArgs, tt.wantArgs)
		})
	}
}
