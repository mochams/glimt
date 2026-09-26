package glimt_test

import (
	"errors"
	"fmt"
	"testing/fstest"

	"github.com/mochams/glimt"
)

// A list endpoint: optional filters from the request, a fixed tenant filter
// kept in SQL, client-chosen sorting, and the total for pagination.
func Example() {
	reg := glimt.NewRegistry(glimt.DialectPostgres)

	files := fstest.MapFS{
		"queries/orders.sql": {Data: []byte(`
-- :name listOrders
SELECT * FROM orders
WHERE org_id = ? /* :and */
`)},
	}
	if err := reg.LoadFS(files, "queries"); err != nil {
		panic(err)
	}

	// Values parsed from the request; empty ones are skipped.
	orgID, status, ids, search := 7, "paid", []int{3, 5}, ""

	var where []glimt.Predicate
	if status != "" {
		where = append(where, glimt.Eq("status", status))
	}

	if len(ids) > 0 {
		where = append(where, glimt.In("id", ids...))
	}

	if search != "" {
		where = append(where, glimt.IContains("note", search))
	}

	order, err := glimt.ParseSort("-created", map[string]string{"created": "created_at"})
	if err != nil {
		panic(err) // respond with 400 Bad Request
	}

	q := reg.MustGet("listOrders").
		Args(orgID).
		Where(where...).
		OrderBy(append(order, "id")...)

	page, pageArgs := q.Limit(20).Offset(40).Build()
	total, totalArgs := q.BuildCount()

	fmt.Println(page)
	fmt.Println(pageArgs)
	fmt.Println(total)
	fmt.Println(totalArgs)
	// Output:
	// SELECT * FROM orders
	// WHERE org_id = $1 AND status = $2 AND id IN ($3, $4) ORDER BY created_at DESC, id LIMIT $5 OFFSET $6
	// [7 paid 3 5 20 40]
	// SELECT COUNT(*) FROM (SELECT * FROM orders
	// WHERE org_id = $1 AND status = $2 AND id IN ($3, $4)) AS t
	// [7 paid 3 5]
}

func ExampleQuery_Where() {
	reg := glimt.NewRegistry(glimt.DialectPostgres)

	role := "" // not set in this request

	sql, args := reg.Query("SELECT * FROM users").
		Where(
			glimt.Eq("status", "active"),
			glimt.If(role != "", glimt.Eq("role", role)),
		).
		Build()

	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// SELECT * FROM users WHERE status = $1
	// [active]
}

func ExampleIn() {
	allowed := []int{} // this user may see no organizations

	sql, _ := glimt.NewQuery("SELECT * FROM orgs", glimt.DialectPostgres).
		Where(glimt.In("id", allowed...)).
		Build()

	fmt.Println(sql)
	// Output:
	// SELECT * FROM orgs WHERE 1=0
}

func ExampleContains() {
	sql, args := glimt.NewQuery("SELECT * FROM products", glimt.DialectMySQL).
		Where(glimt.Contains("name", "50%")).
		Build()

	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// SELECT * FROM products WHERE name LIKE ? ESCAPE '!'
	// [%50!%%]
}

func ExampleQuery_OrderByExpr() {
	pinnedID := 42 // for example, the row the client just created

	sql, args := glimt.NewQuery("SELECT * FROM orders", glimt.DialectPostgres).
		Where(glimt.Eq("status", "paid")).
		OrderByExpr("CASE WHEN id = ? THEN 0 ELSE 1 END", pinnedID).
		OrderBy("created_at DESC").
		Build()

	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// SELECT * FROM orders WHERE status = $1 ORDER BY CASE WHEN id = $2 THEN 0 ELSE 1 END, created_at DESC
	// [paid 42]
}

func ExampleQuery_BuildCount() {
	q := glimt.NewQuery("SELECT id, total FROM orders", glimt.DialectPostgres).
		Where(glimt.Eq("status", "paid")).
		OrderBy("created_at DESC").
		Limit(20).
		Offset(40)

	sql, args := q.BuildCount()

	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// SELECT COUNT(*) FROM (SELECT id, total FROM orders WHERE status = $1) AS t
	// [paid]
}

func ExampleIContains() {
	for _, dialect := range []glimt.Dialect{glimt.DialectPostgres, glimt.DialectMySQL} {
		sql, args := glimt.NewQuery("SELECT * FROM users", dialect).
			Where(glimt.IContains("name", "doe")).
			Build()

		fmt.Println(sql, args)
	}
	// Output:
	// SELECT * FROM users WHERE name ILIKE $1 ESCAPE '!' [%doe%]
	// SELECT * FROM users WHERE name LIKE ? ESCAPE '!' [%doe%]
}

func ExampleParseSort() {
	allowed := map[string]string{"created": "o.created_at", "total": "o.total"}

	order, err := glimt.ParseSort("-created,total", allowed)
	fmt.Println(order, err)

	_, err = glimt.ParseSort("password", allowed)

	var sortErr *glimt.SortError
	fmt.Println(errors.As(err, &sortErr), err)
	// Output:
	// [o.created_at DESC o.total ASC] <nil>
	// true glimt: sort field "password": not allowed
}

// Both the outer query and the subquery are named queries.
func ExampleInQuery() {
	reg := glimt.NewRegistry(glimt.DialectPostgres)

	files := fstest.MapFS{"queries.sql": {Data: []byte(`
-- :name listUsers
SELECT * FROM users

-- :name tripUserIDs
SELECT user_id FROM trip_users
`)}}
	if err := reg.LoadFS(files, "."); err != nil {
		panic(err)
	}

	drivers := reg.MustGet("tripUserIDs").
		Where(glimt.Eq("trip_id", 42), glimt.Eq("role", "driver"))

	sql, args := reg.MustGet("listUsers").
		Where(glimt.Eq("status", "active"), glimt.InQuery("id", drivers)).
		Build()

	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// SELECT * FROM users WHERE status = $1 AND id IN (SELECT user_id FROM trip_users WHERE trip_id = $2 AND role = $3)
	// [active 42 driver]
}

func ExampleExists() {
	reg := glimt.NewRegistry(glimt.DialectPostgres)

	if err := reg.Add("userOrders", "SELECT 1 FROM orders o WHERE o.user_id = users.id /* :and */"); err != nil {
		panic(err)
	}

	sql, args := reg.Query("SELECT * FROM users").
		Where(glimt.Not(glimt.Exists(reg.MustGet("userOrders").Where(glimt.Eq("o.status", "paid"))))).
		Build()

	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// SELECT * FROM users WHERE NOT (EXISTS (SELECT 1 FROM orders o WHERE o.user_id = users.id AND o.status = $1))
	// [paid]
}

func ExampleRegistry_Add() {
	reg := glimt.NewRegistry(glimt.DialectPostgres)

	err := reg.Add("countByStatus", `
		SELECT status, COUNT(*) FROM users
		WHERE deleted_at IS NULL /* :and */
		GROUP BY status`)
	if err != nil {
		panic(err)
	}

	sql, args := reg.MustGet("countByStatus").Where(glimt.Gte("age", 18)).Build()

	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// SELECT status, COUNT(*) FROM users
	// WHERE deleted_at IS NULL AND age >= $1
	// GROUP BY status
	// [18]
}
