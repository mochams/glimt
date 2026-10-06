package glimt_test

import (
	"embed"
	"errors"
	"fmt"

	"github.com/mochams/glimt"
)

//go:embed testdata/queries
var queries embed.FS

func Example() {
	reg, err := glimt.Load(queries, "testdata/queries")
	if err != nil {
		panic(err)
	}

	sql, args, err := reg.Get("ordersByIDs").Build(glimt.Args{"ids": []int{4, 8}, "org": 1})
	if err != nil {
		panic(err)
	}

	fmt.Println(sql)
	fmt.Println(args)
	// Output:
	// SELECT id, total FROM orders WHERE id IN ($1, $2) AND org_id = $3
	// [4 8 1]
}

func ExampleRegistry_Names() {
	reg, err := glimt.Load(queries, "testdata/queries")
	if err != nil {
		panic(err)
	}

	fmt.Println(reg.Names())
	// Output: [cancelOrder createUsersTable getUser listOrders ordersByIDs]
}

func ExampleBuilder() {
	reg, err := glimt.Load(queries, "testdata/queries")
	if err != nil {
		panic(err)
	}

	status, page := "paid", 2

	b := reg.Get("listOrders").Bind(glimt.Args{"org": 1}).
		Where(glimt.Eq("o.status", status), glimt.If(false, glimt.Gt("o.total", 100))).
		OrderBy(glimt.Desc("o.total")).ThenBy(glimt.Asc("o.id")).
		Limit(20).Offset(20 * (page - 1))

	sql, args, err := b.Build()
	if err != nil {
		panic(err)
	}

	fmt.Println(sql)
	fmt.Println(args)

	total, totalArgs, err := b.BuildCount()
	if err != nil {
		panic(err)
	}

	fmt.Println(total)
	fmt.Println(totalArgs)
	// Output:
	// SELECT o.id, o.total, o.status FROM orders o WHERE (o.org_id = $1) AND (o.status = $2) ORDER BY o.total DESC, o.id LIMIT $3 OFFSET $4
	// [1 paid 20 20]
	// SELECT count(*) FROM (SELECT o.id, o.total, o.status FROM orders o WHERE (o.org_id = $1) AND (o.status = $2)) AS t
	// [1 paid]
}

func ExampleIf() {
	reg, err := glimt.Load(queries, "testdata/queries")
	if err != nil {
		panic(err)
	}

	// Two requests: one filters by status, the other doesn't.
	for _, status := range []string{"paid", ""} {
		sql, args, err := reg.Get("listOrders").Bind(glimt.Args{"org": 1}).
			Where(glimt.If(status != "", glimt.Eq("o.status", status))).
			Build()
		if err != nil {
			panic(err)
		}

		fmt.Println(sql)
		fmt.Println(args)
	}
	// Output:
	// SELECT o.id, o.total, o.status FROM orders o WHERE (o.org_id = $1) AND (o.status = $2) ORDER BY o.created_at DESC, o.id
	// [1 paid]
	// SELECT o.id, o.total, o.status FROM orders o WHERE o.org_id = $1 ORDER BY o.created_at DESC, o.id
	// [1]
}

func ExampleColumns() {
	reg, err := glimt.Load(queries, "testdata/queries")
	if err != nil {
		panic(err)
	}

	// The columns a request may sort by, checked once at startup.
	sortable := glimt.Columns{"created": "o.created_at", "total": "o.total"}
	if err := sortable.Check(); err != nil {
		panic(err)
	}

	// From the request, say ?sort=total&desc=1 and then ?sort=password_hash.
	for _, field := range []string{"total", "password_hash"} {
		sql, _, err := reg.Get("listOrders").Bind(glimt.Args{"org": 1}).
			OrderBy(sortable.Order(field, true)).Build()

		switch {
		case errors.Is(err, glimt.ErrUnknownField):
			fmt.Println("400 Bad Request:", err)
		case err != nil:
			panic(err)
		default:
			fmt.Println(sql)
		}
	}
	// Output:
	// SELECT o.id, o.total, o.status FROM orders o WHERE o.org_id = $1 ORDER BY o.total DESC
	// 400 Bad Request: glimt: query "listOrders": unknown field "password_hash"
}
