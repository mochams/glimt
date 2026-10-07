package glimt

import (
	"errors"
	"slices"
	"sync"
	"testing"
)

// builderQueries are the queries the builder tests compose.
var builderQueries = map[string]string{"q.sql": `
-- name: list
SELECT o.id FROM orders o WHERE o.org = :org ORDER BY o.created_at DESC
-- name: page
SELECT id FROM orders WHERE org = :org ORDER BY id LIMIT :n
-- name: both
SELECT id FROM a UNION SELECT id FROM b
-- name: cancel
UPDATE orders SET status = 'cancelled' WHERE id = :id RETURNING id
-- name: add
INSERT INTO orders (org) VALUES (:org)
`}

func TestBuilder(t *testing.T) {
	reg := mustLoad(t, builderQueries)

	tests := []struct {
		name string
		b    Builder
		sql  string
		args []any
	}{
		{"filter, sort and page", reg.Get("list").Bind(Args{"org": 1}).
			Where(Eq("o.status", "paid"), In("o.id", []int{5, 6})).
			OrderBy(Asc("o.total")).ThenBy(Asc("o.id")).Limit(10).Offset(20),
			"SELECT o.id FROM orders o WHERE (o.org = $1) AND (o.status = $2 AND o.id IN ($3, $4)) " +
				"ORDER BY o.total, o.id LIMIT $5 OFFSET $6", []any{1, "paid", 5, 6, 10, 20}},
		{"tiebreak after the query's order", reg.Get("list").Bind(Args{"org": 1}).ThenBy(Asc("o.id")),
			"SELECT o.id FROM orders o WHERE o.org = $1 ORDER BY o.created_at DESC, o.id", []any{1}},
		{"order and limit on a union", reg.Get("both").Bind(nil).OrderBy(Desc("id")).Limit(5),
			"SELECT id FROM a UNION SELECT id FROM b ORDER BY id DESC LIMIT $1", []any{5}},
		{"tenant filter on an update", reg.Get("cancel").Bind(Args{"id": 9}).Where(Eq("org", 2)),
			"UPDATE orders SET status = 'cancelled' WHERE (id = $1) AND (org = $2) RETURNING id", []any{9, 2}},
		{"nothing added", reg.Get("list").Bind(Args{"org": 1}),
			"SELECT o.id FROM orders o WHERE o.org = $1 ORDER BY o.created_at DESC", []any{1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := tt.b.Build()
			if err != nil {
				t.Fatal(err)
			}

			if sql != tt.sql || !slices.Equal(args, tt.args) {
				t.Errorf("Build =\n%s %v\nwant\n%s %v", sql, args, tt.sql, tt.args)
			}
		})
	}
}

func TestBuilderIsAValue(t *testing.T) {
	reg := mustLoad(t, builderQueries)

	base := reg.Get("list").Bind(Args{"org": 1}).Where(Eq("a", 1))
	paid := base.Where(Eq("s", "paid"))
	open := base.Where(Eq("s", "open"))

	for _, tt := range []struct {
		b    Builder
		args []any
	}{{base, []any{1, 1}}, {paid, []any{1, 1, "paid"}}, {open, []any{1, 1, "open"}}} {
		if _, args, err := tt.b.Build(); err != nil || !slices.Equal(args, tt.args) {
			t.Errorf("args = %v, %v; want %v", args, err, tt.args)
		}
	}
}

func TestBuildCount(t *testing.T) {
	reg := mustLoad(t, builderQueries)

	sql, args, err := reg.Get("page").Bind(Args{"org": 1, "n": 50}).Where(Eq("status", "paid")).Offset(100).BuildCount()
	if err != nil {
		t.Fatal(err)
	}

	if want := "SELECT count(*) FROM (SELECT id FROM orders WHERE (org = $1) AND (status = $2)) AS t"; sql != want {
		t.Errorf("sql = %q, want %q", sql, want)
	}

	if want := []any{1, "paid"}; !slices.Equal(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}
}

func TestBuilderErrors(t *testing.T) {
	reg := mustLoad(t, builderQueries)

	tests := []struct {
		b    Builder
		want string
	}{
		{reg.Get("both").Bind(nil).Where(Eq("a", 1)),
			`glimt: query "both": a WHERE condition can only be added to a SELECT, UPDATE or DELETE`},
		{reg.Get("add").Bind(Args{"org": 1}).Where(Eq("a", 1)),
			`glimt: query "add": a WHERE condition can only be added to a SELECT, UPDATE or DELETE`},
		{reg.Get("cancel").Bind(Args{"id": 1}).Limit(1),
			`glimt: query "cancel": ORDER BY, LIMIT and OFFSET can only be added to a query`},
		{reg.Get("page").Bind(Args{"org": 1, "n": 5}).Limit(10),
			`glimt: query "page": can't replace a LIMIT that holds a param`},
		{reg.Get("list").Bind(Args{"org": 1}).Limit(-1), `glimt: query "list": negative limit -1`},
		{reg.Get("list").Bind(Args{"org": 1}).Offset(-2).Limit(-1), `glimt: query "list": negative offset -2`},
		{reg.Get("list").Bind(nil), `glimt: query "list": no value for :org`},
		{Builder{}, "glimt: Builder not made by Query.Bind"},
	}

	for _, tt := range tests {
		if _, _, err := tt.b.Build(); errText(err) != tt.want {
			t.Errorf("Build error = %q, want %q", errText(err), tt.want)
		}
	}

	if _, _, err := reg.Get("cancel").Bind(Args{"id": 1}).BuildCount(); err == nil {
		t.Error("BuildCount on an UPDATE succeeded")
	}
}

// TestBuilderConcurrent composes one shared query from many goroutines; run
// with -race, it checks that composing never writes to the cached query.
func TestBuilderConcurrent(t *testing.T) {
	reg := mustLoad(t, builderQueries)
	q := reg.Get("list")

	want, _, err := q.Bind(Args{"org": 1}).Where(Eq("a", 0)).Limit(1).Build()
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				if got, _, err := q.Bind(Args{"org": 1}).Where(Eq("a", 0)).Limit(1).Build(); err != nil || got != want {
					t.Errorf("Build = %q, %v; want %q", got, err, want)
				}
			}
		})
	}

	wg.Wait()

	if again, _, err := q.Build(Args{"org": 1}); err != nil || again != "SELECT o.id FROM orders o WHERE o.org = $1 ORDER BY o.created_at DESC" {
		t.Errorf("the cached query changed: %q, %v", again, err)
	}
}

func TestBuilderAllocations(t *testing.T) {
	reg := mustLoad(t, builderQueries)
	q, args := reg.Get("list"), Args{"org": 1}

	n := testing.AllocsPerRun(100, func() {
		_, _, _ = q.Bind(args).Where(Eq("o.status", "paid")).OrderBy(Desc("o.total")).Limit(50).Build()
	})

	// The param values, made by Bind, the SQL buffer and the args. The
	// Builder and the render state stay on the stack.
	if n > 3 {
		t.Errorf("composing allocates %v times, want at most 3", n)
	}

	if n := testing.AllocsPerRun(100, func() {
		_, _, _ = q.Bind(args).Where(Eq("o.status", "paid")).BuildCount()
	}); n > 3 {
		t.Errorf("counting allocates %v times, want at most 3", n)
	}

	// The param values, made by Bind, and their copy, the args.
	if n := testing.AllocsPerRun(100, func() {
		_, _, _ = q.Bind(args).Where(If(false, Eq("o.status", "paid"))).Build()
	}); n > 2 {
		t.Errorf("Bind and Build with nothing composed allocate %v times, want at most 2", n)
	}

	b := q.Bind(args)
	if n := testing.AllocsPerRun(100, func() { _, _, _ = b.Build() }); n > 1 {
		t.Errorf("Build with nothing composed allocates %v times, want at most 1", n)
	}
}

func TestBuildCountIgnoresDroppedParams(t *testing.T) {
	reg := mustLoad(t, map[string]string{"q.sql": "-- name: q\nSELECT id FROM users ORDER BY id IN (:ids);"})
	b := reg.Get("q").Bind(Args{"ids": []int{}})

	sql, args, err := b.BuildCount()
	if want := "SELECT count(*) FROM (SELECT id FROM users) AS t"; sql != want || len(args) != 0 || err != nil {
		t.Errorf("BuildCount = %q, %v, %v; want %q", sql, args, err, want)
	}

	if _, _, err := b.Build(); !errors.Is(err, ErrEmptyList) {
		t.Errorf("Build error = %v, want ErrEmptyList", err)
	}
}

func TestBuildDoesNotShareArgs(t *testing.T) {
	reg := mustLoad(t, builderQueries)
	base := reg.Get("list").Bind(Args{"org": 1})

	_, args, err := base.Build()
	if err != nil {
		t.Fatal(err)
	}

	args[0] = 2 // a caller may reuse its args

	if _, again, _ := base.Build(); again[0] != 1 {
		t.Errorf("a second Build binds %v: the args of the first are shared with the Builder", again[0])
	}
}

func TestBindCopiesArgs(t *testing.T) {
	reg := mustLoad(t, builderQueries)
	args := Args{"org": 1}
	base := reg.Get("list").Bind(args)

	args["org"] = 2

	if _, got, _ := base.Build(); got[0] != 1 {
		t.Errorf("Build binds %v after the Args changed, want the value at Bind", got[0])
	}
}

func BenchmarkBuild(b *testing.B) {
	reg, err := Load(mapFS(builderQueries), ".")
	if err != nil {
		b.Fatal(err)
	}

	q, args := reg.Get("list"), Args{"org": 1}

	b.Run("as written", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			if _, _, err := q.Build(args); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("bound, nothing composed", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			if _, _, err := q.Bind(args).Where(If(false, Eq("o.status", "paid"))).Build(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("composed", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			if _, _, err := q.Bind(args).Where(Eq("o.status", "paid")).OrderBy(Desc("o.total")).Limit(50).Build(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("count", func(b *testing.B) {
		b.ReportAllocs()

		for b.Loop() {
			if _, _, err := q.Bind(args).Where(Eq("o.status", "paid")).BuildCount(); err != nil {
				b.Fatal(err)
			}
		}
	})
}
