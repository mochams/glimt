// Package glimt provides a lightweight SQL query builder for Go with support
// for named queries and composable dynamic predicates.
//
// # Overview
//
// Named queries live in .sql files and are loaded into a registry at startup.
// At runtime, queries are retrieved by name and extended with composable
// predicates for dynamic filtering — including WHERE conditions, GROUP BY,
// ORDER BY, LIMIT, and OFFSET — before being built into a parameterized SQL
// string that is safe against injection.
//
// The library has three core components:
//
//   - Registry — loads and caches named queries from .sql files
//   - Query — a chainable builder for attaching dynamic clauses
//   - Predicates — composable conditions for WHERE and HAVING clauses
//
// # Quick Start
//
// Write SQL in .sql files using annotations:
//
//	-- :name listUsers
//	SELECT * FROM users
//
//	-- :name getUserByID
//	SELECT * FROM users WHERE id = ?
//
// Load at startup and query at runtime:
//
//	reg := glimt.NewRegistry(glimt.DialectPostgres)
//	if err := reg.Load("queries/"); err != nil {
//	    log.Fatal(err)
//	}
//
//	sql, args := reg.MustGet("listUsers").
//	    Where(glimt.And(
//	        glimt.Eq("status", "active"),
//	        glimt.Gt("age", 18),
//	    )).
//	    OrderBy("created_at DESC").
//	    Limit(10).
//	    Build()
//
//	db.QueryContext(ctx, sql, args...)
//
// Ad-hoc queries are supported through the registry as well:
//
//	sql, args := reg.Query("SELECT * FROM orders").
//	    Where(glimt.Eq("user_id", userID)).
//	    Build()
//
// # SQL Injection Safety
//
// glimt never interpolates values into SQL strings. Every value passed to a
// predicate becomes a bound argument. Placeholders are rewritten to the correct
// dialect format at build time:
//
//	reg.Query("SELECT * FROM users").
//	    Where(glimt.Eq("name", "robert'); DROP TABLE users;--")).
//	    Build()
//	// SELECT * FROM users WHERE name = $1
//	// args: ["robert'); DROP TABLE users;--"]
//
// Column names, Cond expressions, GroupBy and OrderBy are written into the SQL
// as given. Never pass request input to them; use ParseSort for client-chosen
// sorting.
//
// # Sorting
//
// ParseSort turns a client sort parameter such as "-created,total" into
// ORDER BY expressions, accepting only allowlisted fields:
//
//	order, err := glimt.ParseSort(r.URL.Query().Get("sort"), map[string]string{
//	    "created": "o.created_at",
//	    "total":   "o.total",
//	})
//	if err != nil {
//	    // *glimt.SortError: respond with 400 Bad Request
//	}
//
//	q.OrderBy(append(order, "o.id")...) // unique tiebreaker for stable pages
//
// # Predicates
//
// Predicates are composable conditions that can be combined with And, Or, and Not:
//
//	glimt.And(
//	    glimt.Eq("status", "active"),
//	    glimt.Or(
//	        glimt.Eq("role", "admin"),
//	        glimt.Eq("role", "mod"),
//	    ),
//	    glimt.Not(glimt.Null("deleted_at")),
//	)
//	// (status = ? AND (role = ? OR role = ?) AND NOT (deleted_at IS NULL))
//
// Available predicates: Cond, Eq, Neq, Gt, Gte, Lt, Lte, Like, NotLike,
// ILike, Contains, StartsWith, EndsWith, Null, NotNull, In, NotIn, InQuery,
// Exists, Between, NotBetween, RangeOpen, And, Or, Not, If.
//
// For search boxes, Contains, StartsWith and EndsWith escape the LIKE
// wildcards % and _ in user input, so they match literally:
//
//	glimt.Or(glimt.Contains("name", q), glimt.Contains("email", q))
//	// (name LIKE ? ESCAPE '!' OR email LIKE ? ESCAPE '!')
//
// Cond wraps its raw expression in parentheses, so a condition containing OR
// keeps its meaning when combined with other predicates.
//
// # Optional Filters
//
// A nil predicate renders nothing, and so does an And, Or or Not with nothing
// to render. WHERE and HAVING are omitted when no predicate renders. This lets
// API handlers collect only the filters a request actually uses:
//
//	var where []glimt.Predicate
//	if f.Status != "" {
//	    where = append(where, glimt.Eq("status", f.Status))
//	}
//	if len(f.Roles) > 0 {
//	    where = append(where, glimt.In("role", f.Roles...))
//	}
//
//	sql, args := reg.MustGet("listUsers").Where(where...).Build()
//
// The same slice can be reused for a matching count query. If expresses a
// single optional filter inline:
//
//	q.Where(glimt.If(f.Status != "", glimt.Eq("status", f.Status)))
//
// The arguments to If are evaluated even when the condition is false, so do
// not dereference pointers inside it.
//
// In with an empty list renders 1=0 and matches no rows, so a filter such as
// In("org_id", allowed...) cannot widen to every row. NotIn with an empty list
// renders 1=1.
//
// Predicates follow SQL NULL semantics. A row whose column is NULL matches
// neither Eq nor Neq, so Not and Exclude drop it too, and NotIn with a nil in
// its list matches no rows. Use Null and NotNull to match NULL explicitly.
//
// # Dialects
//
// Dialect is configured once on the registry and applied to all queries:
//
//	glimt.NewRegistry(glimt.DialectPostgres)  // $1, $2, ...
//	glimt.NewRegistry(glimt.DialectMySQL)     // ?, ?, ...
//	glimt.NewRegistry(glimt.DialectSQLite)    // ?, ?, ...
//
// # Loading Queries
//
// The registry provides four methods for loading SQL files:
//
//	// load all .sql files recursively from a directory
//	reg.Load("queries/")
//
//	// load all .sql files recursively from an fs.FS
//	reg.LoadFS(sqlFiles, "queries")
//
//	// load a single file by path
//	reg.LoadFile("queries/users.sql")
//
//	// load a single file from an fs.FS
//	reg.LoadFileFS(sqlFiles, "queries/users.sql")
//
// SQL defined in Go code can be registered the same way with Add:
//
//	err := reg.Add("activeUsers", "SELECT * FROM users WHERE deleted_at IS NULL")
//
// Every query is sanitized once, when it is loaded or added, so Get does no
// string work. Ad-hoc SQL passed to Query is sanitized the same way on every
// call; prefer Add for SQL used on every request.
//
// Load or add queries at startup. After that, a Registry is safe for
// concurrent use by multiple goroutines.
//
// # SQL File Format
//
// Files use '-- :name annotations'. A single file can contain multiple named
// queries. Use ? as the placeholder regardless of dialect — glimt writes
// them at build time.
//
//	-- :name listUsers
//	SELECT * FROM users
//
//	-- :name getUserByID
//	SELECT * FROM users WHERE id = ?
//
// Annotations must be on their own line. Spacing is flexible ("--:name x"
// works), but any other "-- :word" annotation, or the "-- name: x" style used
// by other tools, is a load error rather than a silent comment.
//
// SQL comments are stripped at load time: line comments (--, and # on MySQL)
// and block comments (/* */) are removed before storing the query. Stripping
// understands string literals, quoted identifiers and Postgres dollar-quoted
// bodies, so text such as '--' or '/*' inside them is kept, and whitespace
// inside literals is preserved. Optimizer hints (/*+ */) and MySQL executable
// comments (/*! */) are kept.
//
// To write a literal question mark outside a string, such as the Postgres
// JSONB ? operator, escape it as ??:
//
//	SELECT * FROM products WHERE attributes ?? 'color'
//
// On Postgres, use ? rather than native $1 placeholders; mixing the two would
// number parameters twice, so $1 in a loaded query is a load error.
//
// Query names must be unique within a file and across all loaded files.
// Duplicates, empty query bodies, unterminated quotes and unterminated
// comments are caught at load time, and errors include the line number.
//
// # Embedded Files
//
// The registry supports embedded SQL files via fs.FS and embed.FS:
//
//	//go:embed queries
//	var sqlFiles embed.FS
//
//	reg := glimt.NewRegistry(glimt.DialectPostgres)
//	if err := reg.LoadFS(sqlFiles, "queries"); err != nil {
//	    log.Fatal(err)
//	}
//
// # Subqueries
//
// InQuery and Exists embed another query, so both queries can be named in
// .sql files:
//
//	-- :name listUsers
//	SELECT * FROM users
//
//	-- :name tripUserIDs
//	SELECT user_id FROM trip_users
//
//	sql, args := reg.MustGet("listUsers").
//	    Where(glimt.InQuery("id", reg.MustGet("tripUserIDs").Where(glimt.Eq("trip_id", tripID)))).
//	    Build()
//	// SELECT * FROM users WHERE id IN (SELECT user_id FROM trip_users WHERE trip_id = $1)
//
// The subquery keeps its own Args, filters and clause markers. It is rendered
// when the outer query is built, and all placeholders are numbered once. For
// "not in", prefer Not(Exists(...)): NOT IN matches no rows as soon as the
// subquery returns a NULL. MySQL does not allow LIMIT inside an IN subquery.
//
// InQuery and Exists panic when given a nil query, which is always a
// programming error. For a subquery filter that only applies sometimes, use If.
//
// # Clause Markers
//
// By default, WHERE, GROUP BY, HAVING, ORDER BY, LIMIT and OFFSET are appended
// to the end of the query. To keep a fixed WHERE, a GROUP BY, a UNION or a
// RETURNING clause in the SQL file, mark where the Where predicates go:
//
//	-- :name countOrders
//	SELECT status, COUNT(*) FROM orders
//	WHERE org_id = ? AND deleted_at IS NULL /* :and */
//	GROUP BY status
//
//	sql, args := reg.MustGet("countOrders").
//	    Args(orgID).
//	    Where(glimt.Eq("region", region)).
//	    Build()
//	// ... WHERE org_id = $1 AND deleted_at IS NULL AND region = $2 GROUP BY status
//
// Two markers are available:
//
//   - /* :where */ renders " WHERE <predicates>"
//   - /* :and */ renders " AND <predicates>", after a fixed WHERE
//
// Both render nothing when no predicate applies. Markers are SQL comments, so
// the file still runs as-is in psql, the mysql client or sqlite3. A marker can
// appear more than once (for example in each branch of a UNION); the predicates
// are rendered at each one. Args fill the ? placeholders of the base SQL in
// order, around the rendered predicates. GroupBy, Having, OrderBy, Limit and
// Offset are still appended at the end.
//
// A fixed condition before /* :and */ that uses OR must be parenthesized:
// WHERE (a OR b) /* :and */. Unknown markers are a load error.
package glimt
