// Package glimt runs Postgres queries kept in .sql files, and adds filters,
// sorting and paging to them per request.
//
// glimt parses every query when the program starts, so it knows where each
// query's WHERE, ORDER BY and LIMIT clauses are. At request time it binds
// the query's params and adds the request's conditions, order and paging in
// the right place. Values are always bound as args and never written into
// the SQL.
//
// # Writing queries
//
// Put each query after a "-- name:" line, and end it with ";":
//
//	-- name: listOrders
//	-- Orders of an organization, newest first.
//	SELECT o.id, o.total, o.status
//	FROM orders o
//	WHERE o.org_id = :org
//	ORDER BY o.created_at DESC;
//
//	-- name: ordersByIDs
//	SELECT id, total FROM orders WHERE id IN (:ids) AND org_id = :org;
//
// A query runs from its "-- name:" line to the next one. Names use ASCII
// letters, digits and _, can't start with a digit, and must be unique across
// all files. Comments in a query are dropped.
//
// glimt rejects a second statement inside one query. The ";" is how it can
// tell where the first one ends: without it, two UPDATE, INSERT or DELETE
// statements can run together and load as one query, because those words can
// also be column names.
//
// A line that looks like an annotation but isn't one, such as "-- Name: x",
// is an error, so a typo can't make a query disappear.
//
// # Loading
//
// Embed the files and call [Load] once, at startup:
//
//	//go:embed queries
//	var queries embed.FS
//
//	reg, err := glimt.Load(queries, "queries")
//	if err != nil {
//		log.Fatal(err)
//	}
//
// Load parses and compiles every query and reports one error per broken
// query, each a [*LoadError] with its file, line and column:
//
//	queries/orders.sql:9:13: glimt: expected expression after FROM, found "WHERE"
//
// [Registry.Get] returns a query and panics when there is none by that name.
// Call it for every query right after Load, so a missing one fails at
// startup. [Registry.Lookup] is for names chosen at run time.
//
// # Running a query
//
// [Query.Build] binds the query's :name params from [Args] and returns the
// SQL and args to pass to the driver:
//
//	sql, args, err := reg.Get("ordersByIDs").Build(glimt.Args{"ids": []int{4, 8}, "org": 7})
//	// SELECT id, total FROM orders WHERE id IN ($1, $2) AND org_id = $3
//	// args: [4 8 7]
//
//	rows, err := db.QueryContext(ctx, sql, args...)
//
// Args must hold a value for every param and nothing else. A name used
// twice reuses its placeholder. IN (:name) expands a slice into one
// placeholder per element; []byte, arrays and driver.Valuer types bind as
// one value. When list lengths vary a lot, = ANY(:name) binds the slice as
// one array and keeps the SQL the same for every length.
//
// # Composing per request
//
// [Query.Bind] sets the params and returns a [Builder], which adds
// conditions, ordering and paging. Each Builder method returns a new
// Builder and leaves the old one unchanged, so a base can be shared and
// extended per request:
//
//	b := reg.Get("listOrders").Bind(glimt.Args{"org": 7}).
//		Where(
//			glimt.Eq("o.status", "paid"),
//			glimt.If(minTotal > 0, glimt.Ge("o.total", minTotal)),
//		).
//		OrderBy(glimt.Desc("o.total")).
//		ThenBy(glimt.Asc("o.id")).
//		Limit(20).
//		Offset(40)
//
//	sql, args, err := b.Build()
//	// SELECT o.id, o.total, o.status FROM orders o
//	// WHERE (o.org_id = $1) AND (o.status = $2 AND o.total >= $3)
//	// ORDER BY o.total DESC, o.id LIMIT $4 OFFSET $5
//
//	total, totalArgs, err := b.BuildCount()
//	// SELECT count(*) FROM (SELECT o.id, o.total, o.status FROM orders o
//	// WHERE (o.org_id = $1) AND (o.status = $2 AND o.total >= $3)) AS t
//
// Added conditions are ANDed with the query's own WHERE as
// (original) AND (added), and never ORed, so a condition written in the
// query, such as a tenant filter, always holds. A condition added to a query
// with GROUP BY goes before the GROUP BY. Only the top-level statement is
// composed; subqueries and CTEs stay as written.
//
// [Builder.OrderBy] replaces the query's ORDER BY, and [Builder.ThenBy] adds
// terms after it, such as a unique tiebreaker for stable pages.
// [Builder.Limit] and [Builder.Offset] set LIMIT and OFFSET as bound args.
// [Builder.BuildCount] counts the rows of the composed query, without its
// ORDER BY, LIMIT, OFFSET, FETCH and locking clauses.
//
// Not every statement takes every change. A SELECT takes all of them. A
// UNION, VALUES or TABLE takes ordering, paging and counting, but not
// conditions. UPDATE and DELETE take conditions only. INSERT, MERGE and DDL
// take none. Asking for a change a statement can't take is an error that
// wraps [ErrNotComposable].
//
// # Conditions
//
// A [Pred] is one condition:
//
//   - comparisons: [Eq], [Ne], [Lt], [Le], [Gt], [Ge];
//   - lists: [In], [NotIn];
//   - NULL tests: [IsNull], [IsNotNull];
//   - patterns: [Like], [NotLike], [ILike], [NotILike];
//   - substrings, with % and _ in the text matching themselves: [Contains],
//     [StartsWith], [EndsWith], and [IContains], [IStartsWith], [IEndsWith]
//     to ignore case;
//   - groups: [And], [Or], [Not].
//
// [If] returns its condition only when a Go condition holds, which keeps
// optional request filters to one line each. The zero Pred adds nothing,
// and when every condition adds nothing, no WHERE is added:
//
//	b = b.Where(
//		glimt.If(req.Status != "", glimt.Eq("o.status", req.Status)),
//		glimt.If(len(req.IDs) > 0, glimt.In("o.id", req.IDs)),
//	)
//
// # Columns
//
// A column in a condition or order term is written into the SQL as given,
// so it must come from your code. It must be a plain column reference such
// as status, o.status or "Status"; anything else is an error that wraps
// [ErrBadColumn].
//
// When a request chooses the column, as with ?sort=total, map the names it
// may use to columns with [Columns]. Only the listed columns can be reached:
//
//	var sortable = glimt.Columns{"created": "o.created_at", "total": "o.total"}
//
//	b = b.OrderBy(sortable.Order(req.Sort, req.Desc))
//
// # Errors
//
// Every error message starts with "glimt:", after the file:line:col of a
// [*LoadError]. Errors from Build and BuildCount wrap a sentinel, such as
// [ErrMissingArg], [ErrEmptyList] or [ErrUnknownField], for errors.Is.
//
// # Concurrency
//
// A [Registry] and its queries never change after Load, and a Builder is a
// value. All of them are safe to share between goroutines.
package glimt
