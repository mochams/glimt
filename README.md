# glimt — Go Enhanced SQL

**glimt** is a lightweight SQL toolkit for Go that lets you:

* keep SQL in `.sql` files
* load queries by name
* compose dynamic predicates safely at runtime

**SQL with runtime composition**.

---

## glimt?

SQL is often kept in dedicated `.sql` files rather than embedded directly in code. This approach keeps queries readable easier to review, and easier to maintain as they grow in complexity.

Libraries like **yesql** popularized this pattern: write SQL in SQL, load it by name, and execute it from your application.

However, real-world APIs often need to **dynamically respond to request parameters**:

* optional filters
* search queries
* pagination
* conditional predicates

This usually leads to manually assembling SQL strings in code.

**glimt bridges this gap.**

You can:

* keep **base queries in `.sql` files**
* load them by name
* safely **compose dynamic predicates at runtime**

```sql
-- :name listUsers
SELECT * FROM users
```

```go
sql, args := reg.MustGet("listUsers").
    Where(glimt.And(
        glimt.Eq("status", "active"),
        glimt.Gt("age", 18),
    )).
    OrderBy("created_at DESC").
    Limit(20).
    Build()
```

This keeps your **core SQL declarative**, while allowing **flexible runtime filtering**.

---

## Features

* Named SQL queries loaded from `.sql` files
* Composable predicates (`And`, `Or`, `Eq`, `In`, `Between`, etc.)
* Multi-dialect support

  * Postgres `$1`
  * MySQL / SQLite `?`
  * SQL Server `@p1`
  * Oracle `:1`

* Full clause support

  * `WHERE`
  * `GROUP BY`
  * `HAVING`
  * `ORDER BY`
  * `LIMIT`
  * `OFFSET`

* Works with `fs.FS` and `embed.FS`
* Zero external dependencies

---

## Installation

```bash
go get github.com/mochams/glimt
```

---

## Quick Start

### Ad-hoc queries

```go
reg := glimt.NewRegistry(glimt.DialectPostgres)

sql, args := reg.Query("SELECT * FROM users").
    Where(glimt.And(
        glimt.Eq("status", "active"),
        glimt.Gt("age", 18),
    )).
    OrderBy("created_at DESC").
    Limit(20).
    Offset(0).
    Build()
```

Result:

```sql
SELECT * FROM users
WHERE status = $1 AND age > $2
ORDER BY created_at DESC
LIMIT $3 OFFSET $4
```

Args:

```txt
["active", 18, 20, 0]
```

---

### Using SQL files

Write queries in `.sql` files:

```sql
-- :name listUsers
SELECT * FROM users

-- :name getUserByID
SELECT * FROM users WHERE id = ?

-- :name listActiveOrders
SELECT *
FROM orders
WHERE status = ?
AND deleted_at IS NULL
```

Load and use them:

```go
reg := glimt.NewRegistry(glimt.DialectPostgres)

if err := reg.Load("queries/users.sql"); err != nil {
    log.Fatal(err)
}

query := reg.MustGet("listUsers")

sql, args := query.
    Where(glimt.Eq("role", "admin")).
    OrderBy("created_at DESC").
    Limit(10).
    Build()
```

---

### Embedded SQL files

Works with Go's `embed`.

```go
//go:embed queries
var sqlFiles embed.FS

reg := glimt.NewRegistry(glimt.DialectPostgres)

if err := reg.WalkFS(sqlFiles, "queries"); err != nil {
    log.Fatal(err)
}
```

---

## Query Methods

| Method             | Description                                                   |
| ------------------ | ------------------------------------------------------------- |
| `Where(pred)`      | Add a WHERE condition. Multiple calls are ANDed together      |
| `Exclude(pred)`    | Add a negated WHERE condition — equivalent to `AND NOT (...)` |
| `GroupBy(cols...)` | Add a GROUP BY clause                                         |
| `Having(pred)`     | Add a HAVING condition                                        |
| `OrderBy(cols...)` | Add an ORDER BY clause                                        |
| `Limit(n)`         | Add a LIMIT clause                                            |
| `Offset(n)`        | Add an OFFSET clause                                          |
| `Build()`          | Finalize and return the SQL string and arguments              |

---

## Predicates

### Simple conditions

```go
glimt.Cond("age > ?", 30)

glimt.Eq("status", "active")
glimt.Neq("status", "banned")

glimt.Lt("age", 18)
glimt.Lte("age", 18)

glimt.Gt("age", 65)
glimt.Gte("age", 65)
```

---

### Set conditions

```go
glimt.In("status", "active", "pending")

glimt.NotIn("status", "banned", "deleted")
```

---

### Logical combinators

```go
glimt.And(
    glimt.Eq("status", "active"),
    glimt.Gt("age", 18),
)

glimt.Or(
    glimt.Eq("role", "admin"),
    glimt.Eq("role", "mod"),
)

glimt.Not(
    glimt.Eq("status", "banned"),
)
```

They compose naturally:

```go
glimt.And(
    glimt.Eq("status", "active"),
    glimt.Or(
        glimt.Eq("role", "admin"),
        glimt.Eq("role", "mod"),
    ),
)
```

---

## SQL Dialects

glimt writes placeholders automatically.

| Database           | Placeholder Style | Example      |
| ------------------ | ----------------- | ------------ |
| **Postgres**       | `$n`              | `$1, $2, $3` |
| **MySQL / SQLite** | `?`               | `?, ?, ?`    |
| **SQL Server**     | `@pN`             | `@p1, @p2`   |
| **Oracle**         | `:N`              | `:1, :2`     |

Example:

```go
reg := glimt.NewRegistry(glimt.DialectSQLServer)

reg.Query("SELECT * FROM users").
    Where(glimt.Eq("id", 1)).
    Build()
```

---

## Registry

```go
reg := glimt.NewRegistry(glimt.DialectPostgres)

// load single file
reg.Load("queries/users.sql")

// load all .sql files in a directory
reg.LoadDir("queries")

// load from fs.FS
reg.LoadFS(fsys, "queries/users.sql")

// walk directory in fs.FS
reg.WalkFS(fsys, "queries")
```

Get queries:

```go
q, err := reg.Get("listUsers")

q := reg.MustGet("listUsers")

q := reg.Query("Select * FROM users")
```

---

## SQL File Format

**Queries are defined using `-- :name` annotations**

```sql
-- :name listUsers
SELECT * FROM users

-- :name getUserByID
SELECT * FROM users WHERE id = ?

-- :name deleteUser
DELETE FROM users WHERE id = ?
```

**Queries with `existing WHERE` clauses**

The builder appends `WHERE` to the base SQL. If the query already has an
outer `WHERE`, the result is invalid SQL. For fully static queries this is
fine — just don't call `.Where()`:

```sql
-- :name listAdults
SELECT *
FROM users u
LEFT JOIN user_profile up ON up.user_id = u.id
WHERE u.age > 18
```

```go
sql, args := reg.MustGet("listAdults").Limit(10).Build()
```

However, a static `WHERE` limits reuse. The same base query cannot serve
both adults and kids without duplication. The better approach is to keep
the base query clean and push the condition to the builder:

```sql
-- :name listUsers
SELECT *
FROM users u
LEFT JOIN user_profile up ON up.user_id = u.id
```

```go
adults := reg.MustGet("listUsers").
    Where(glimt.Gt("u.age", 18)).
    Limit(10).
    Build()

kids := reg.MustGet("listUsers").
    Where(glimt.Lt("u.age", 18)).
    Limit(10).
    Build()
```

One base query, two use cases, no duplication. This is the pattern glimt
is designed around — write SQL once, filter dynamically

**Rules:**

* Use `?` as the placeholder in all SQL files regardless of target dialect —
  glimt writes them to the correct format at `Build()` time
* Query names must be unique within a file and across all loaded files
* Empty query bodies are ignored at load time
* Do not write an outer `WHERE` clause if you intend to call `.Where()` —
  the builder appends `WHERE pred`, not `AND pred`
* `WHERE` clauses inside subqueries, CTEs, and `EXISTS` expressions are
  unaffected by the builder and are always safe

---

## Inspiration

glimt is inspired by [**yesql**](https://github.com/krisajenkins/yesql), a Clojure library by Kris Jenkins that encourages writing SQL in SQL rather than embedding it in application code.

glimt extends the idea with **composable predicates**, making it easier to build dynamic queries for APIs and search endpoints.

---
