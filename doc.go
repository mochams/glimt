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
//	if err := reg.LoadDir("queries/"); err != nil {
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
//	    glimt.Not(glimt.IsNull("deleted_at")),
//	)
//	// (status = ? AND (role = ? OR role = ?) AND NOT (deleted_at IS NULL))
//
// Available predicates: Cond, Eq, Neq, Gt, Gte, Lt, Lte, Like, ILike,
// IsNull, IsNotNull, In, NotIn, Between, RangeOpen, RangeClosedOpen,
// RangeOpenClosed, And, Or, Not.
//
// # Dialects
//
// Dialect is configured once on the registry and applied to all queries:
//
//	glimt.NewRegistry(glimt.DialectPostgres)   // $1, $2, ...
//	glimt.NewRegistry(glimt.DialectMySQL)      // ?, ?, ...
//	glimt.NewRegistry(glimt.DialectSQLServer)  // @p1, @p2, ...
//	glimt.NewRegistry(glimt.DialectOracle)     // :1, :2, ...
//
// # SQL File Format
//
// Files use '-- :name annotations'. A single file can contain
// multiple named queries. Use ? as the placeholder regardless of dialect —
// glimt rewrites them at build time.
//
//	-- :name listUsers
//	SELECT * FROM users
//
//	-- :name getUserByID
//	SELECT * FROM users WHERE id = ?
//
// Query names must be unique within a file and across all loaded files.
// Duplicates and empty query bodies are caught at load time.
//
// # Embedded Files
//
// The registry supports embedded SQL files via fs.FS and embed.FS:
//
//	//go:embed queries
//	var sqlFiles embed.FS
//
//	reg := glimt.NewRegistry(glimt.DialectPostgres)
//	if err := reg.WalkFS(sqlFiles, "queries"); err != nil {
//	    log.Fatal(err)
//	}
package glimt
