package integration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" driver

	"github.com/mochams/glimt"
	"github.com/mochams/glimt/internal/render"
	"github.com/mochams/glimt/internal/syntax"
)

// schema is the tables the live tests run against.
const schema = `
CREATE TABLE users (
	id       bigserial PRIMARY KEY,
	org      int NOT NULL,
	email    text NOT NULL UNIQUE,
	name     text,
	tags     text[] NOT NULL DEFAULT '{}',
	data     jsonb NOT NULL DEFAULT '{}',
	"offset" int NOT NULL DEFAULT 0
);
CREATE TABLE jobs (id bigserial PRIMARY KEY, state text NOT NULL DEFAULT 'queued');
CREATE TABLE stock (sku text PRIMARY KEY, qty int NOT NULL);
`

// liveDB connects to the Postgres in GLIMT_PG_DSN and creates the schema in a
// schema of its own, dropped when the test ends. It skips the test when
// GLIMT_PG_DSN is not set.
func liveDB(t *testing.T) *sql.Conn {
	t.Helper()

	dsn := os.Getenv("GLIMT_PG_DSN")
	if dsn == "" {
		t.Skip("GLIMT_PG_DSN is not set")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("glimt_test_%d", time.Now().UnixNano())
	for _, stmt := range []string{"CREATE SCHEMA " + name, "SET search_path TO " + name, schema} {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), "DROP SCHEMA "+name+" CASCADE")
		_ = conn.Close()
	})

	return conn
}

// renderSQL parses, compiles and renders src with the named values.
func renderSQL(t *testing.T, src string, named map[string]any) (string, []any) {
	t.Helper()

	p, err := syntax.Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q): %v", src, err)
	}

	tmpl, err := render.Compile(p)
	if err != nil {
		t.Fatalf("Compile(%q): %v", src, err)
	}

	values := make([]any, 0, len(named))
	for _, name := range tmpl.Names() {
		v, ok := named[name]
		if !ok {
			t.Fatalf("no value for :%s in %q", name, src)
		}

		values = append(values, v)
	}

	text, args, err := tmpl.Render(values)
	if err != nil {
		t.Fatalf("Render(%q): %v", src, err)
	}

	return text, args
}

// query renders src and returns the first column of each row as text.
func query(t *testing.T, q interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}, src string, named map[string]any,
) []string {
	t.Helper()

	text, args := renderSQL(t, src, named)

	rows, err := q.QueryContext(context.Background(), text, args...)
	if err != nil {
		t.Fatalf("%s %v: %v", text, args, err)
	}
	defer func() { _ = rows.Close() }()

	var out []string

	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}

		out = append(out, s.String)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", text, err)
	}

	return out
}

// expect fails the test unless got equals want.
func expect(t *testing.T, what string, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}

func TestPostgres(t *testing.T) {
	db := liveDB(t)

	ids := query(t, db, `INSERT INTO users (org, email, name, tags, data)
		VALUES (:org, :a, :a, '{x,y,z}', '{"vip": true}'), (:org, :b, 'b', '{}', '{}'), (2, :c, NULL, '{}', '{}')
		RETURNING id`, map[string]any{"org": 1, "a": "a@x", "b": "b@x", "c": "c@x"})
	expect(t, "inserted ids", ids, []string{"1", "2", "3"})

	t.Run("in list", func(t *testing.T) {
		got := query(t, db, "SELECT id FROM users WHERE email IN (:emails) AND org = :org ORDER BY id LIMIT :n",
			map[string]any{"emails": []string{"a@x", "b@x", "c@x"}, "org": 1, "n": 10})
		expect(t, "IN", got, []string{"1", "2"})

		got = query(t, db, "SELECT id FROM users WHERE id NOT IN (:ids) ORDER BY id", map[string]any{"ids": []int{1, 3}})
		expect(t, "NOT IN", got, []string{"2"})
	})

	t.Run("any array", func(t *testing.T) {
		got := query(t, db, "SELECT id FROM users WHERE id = ANY(:ids) ORDER BY id", map[string]any{"ids": []int64{2, 3}})
		expect(t, "= ANY", got, []string{"2", "3"})
	})

	t.Run("placeholder after a keyword", func(t *testing.T) {
		got := query(t, db, "SELECT CASE WHEN org = :org THEN:yes ELSE:no END FROM users ORDER BY id",
			map[string]any{"org": 1, "yes": "mine", "no": "theirs"})
		expect(t, "CASE", got, []string{"mine", "mine", "theirs"})
	})

	t.Run("lexical forms", func(t *testing.T) {
		expect(t, "continued string", query(t, db, "SELECT 'a'\n  'b'", nil), []string{"ab"})
		expect(t, "escape continuation", query(t, db, "SELECT E'a'\n'\\''", nil), []string{"a'"})
		expect(t, "keyword field", query(t, db, "SELECT u.offset FROM users u WHERE u.id = :id",
			map[string]any{"id": 1}), []string{"0"})
		expect(t, "slice", query(t, db, "SELECT u.tags[1:2] FROM users u WHERE u.id = :id",
			map[string]any{"id": 1}), []string{"{x,y}"})
		expect(t, "jsonb operator", query(t, db, "SELECT count(*) FROM users WHERE data ? :key",
			map[string]any{"key": "vip"}), []string{"1"})
	})

	t.Run("upsert", func(t *testing.T) {
		got := query(t, db, `INSERT INTO users (org, email, name) VALUES (:org, :email, :name)
			ON CONFLICT (email) DO UPDATE SET name = excluded.name RETURNING id`,
			map[string]any{"org": 1, "email": "b@x", "name": "renamed"})
		expect(t, "upsert", got, []string{"2"})
		expect(t, "name", query(t, db, "SELECT name FROM users WHERE id = 2", nil), []string{"renamed"})
	})

	t.Run("merge", func(t *testing.T) {
		query(t, db, "INSERT INTO stock VALUES ('a', 1), ('b', 5) RETURNING sku", nil)

		got := query(t, db, `MERGE INTO stock s USING (VALUES (:sku, :qty::int)) v(sku, qty) ON s.sku = v.sku
			WHEN MATCHED THEN UPDATE SET qty = s.qty + v.qty
			WHEN NOT MATCHED THEN INSERT (sku, qty) VALUES (v.sku, v.qty)
			WHEN NOT MATCHED BY SOURCE AND s.qty > :limit THEN DELETE
			RETURNING merge_action()`, map[string]any{"sku": "a", "qty": 2, "limit": 3})
		slices.Sort(got)
		expect(t, "merge actions", got, []string{"DELETE", "UPDATE"})
	})

	t.Run("skip locked", func(t *testing.T) {
		query(t, db, "INSERT INTO jobs (state) VALUES ('queued'), ('queued') RETURNING id", nil)

		tx, err := db.BeginTx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()

		got := query(t, tx, "SELECT id FROM jobs WHERE state = :state ORDER BY id LIMIT :n FOR UPDATE SKIP LOCKED",
			map[string]any{"state": "queued", "n": 1})
		expect(t, "claimed", got, []string{"1"})
	})
}

// composeQueries are the queries TestPostgresCompose loads through the public API.
var composeQueries = fstest.MapFS{"q.sql": &fstest.MapFile{Data: []byte(`
-- name: orgUsers
SELECT id FROM users WHERE org = :org

-- name: renameUsers
UPDATE users SET name = :name WHERE id = ANY(:ids) RETURNING id
`)}}

func TestPostgresCompose(t *testing.T) {
	db := liveDB(t)

	for i := range 6 {
		run(t, db, "INSERT INTO users (org, email, name) VALUES ($1, $2, $3)",
			1+i%2, fmt.Sprintf("u%d@x", i), []string{"ann", "bob", "cy"}[i%3])
	}

	reg, err := glimt.Load(composeQueries, ".")
	if err != nil {
		t.Fatal(err)
	}

	users := reg.Get("orgUsers").Bind(glimt.Args{"org": 1})

	t.Run("filters", func(t *testing.T) {
		expectBuilt(t, db, "In list and optional filter", users.
			Where(glimt.In("name", []string{"ann", "cy"}), glimt.If(false, glimt.Eq("name", "nobody"))).
			OrderBy(glimt.Asc("id")), []string{"1", "3"})
		expectBuilt(t, db, "Or group", users.
			Where(glimt.Or(glimt.Eq("name", "bob"), glimt.IStartsWith("email", "U1"))).OrderBy(glimt.Asc("id")),
			[]string{"5"})
	})

	t.Run("paging and count", func(t *testing.T) {
		page := users.OrderBy(glimt.Desc("name")).ThenBy(glimt.Asc("id")).Limit(2).Offset(1)
		expectBuilt(t, db, "page", page, []string{"5", "1"})

		sql, args, err := page.BuildCount()
		if err != nil {
			t.Fatal(err)
		}

		expect(t, "count", queryText(t, db, sql, args), []string{"3"})
	})

	t.Run("columns and pointers", func(t *testing.T) {
		sortable := glimt.Columns{"mail": "email", "id": "id"}
		if err := sortable.Check(); err != nil {
			t.Fatal(err)
		}

		minID := new(int64)
		*minID = 3

		expectBuilt(t, db, "sorted by an allowed field, filtered through a pointer", users.
			Where(glimt.If(minID != nil, glimt.Ge("id", minID))).
			OrderBy(sortable.Order("mail", true)), []string{"5", "3"})
	})

	t.Run("tenant filter on update", func(t *testing.T) {
		rename := reg.Get("renameUsers").Bind(glimt.Args{"name": "renamed", "ids": []int64{1, 2, 3}}).
			Where(glimt.Eq("org", 2))
		expectBuilt(t, db, "renamed", rename, []string{"2"})
	})
}

// run executes SQL that returns no rows.
func run(t *testing.T, db *sql.Conn, text string, args ...any) {
	t.Helper()

	if _, err := db.ExecContext(context.Background(), text, args...); err != nil {
		t.Fatalf("%s: %v", text, err)
	}
}

// expectBuilt builds b and checks the first column of its rows.
func expectBuilt(t *testing.T, db *sql.Conn, what string, b glimt.Builder, want []string) {
	t.Helper()

	text, args, err := b.Build()
	if err != nil {
		t.Fatal(err)
	}

	expect(t, what, queryText(t, db, text, args), want)
}

// queryText runs SQL and returns the first column of each row as text.
func queryText(t *testing.T, db *sql.Conn, text string, args []any) []string {
	t.Helper()

	rows, err := db.QueryContext(context.Background(), text, args...)
	if err != nil {
		t.Fatalf("%s %v: %v", text, args, err)
	}
	defer func() { _ = rows.Close() }()

	var out []string

	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}

		out = append(out, s.String)
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	return out
}
