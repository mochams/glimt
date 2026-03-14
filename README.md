# Glimt

![GoDoc](https://pkg.go.dev/badge/github.com/mochams/glimt.svg)

**Glimt** is a lightweight SQL toolkit for Go that keeps queries in `.sql` files
while allowing **safe runtime composition of predicates**.

Many Go applications prefer writing SQL in `.sql` files instead of embedding
large queries directly in code. However APIs still need to dynamically add:

- filters
- search conditions
- pagination

Glimt keeps your **core SQL declarative**, while allowing **flexible runtime query composition**:

```sql
-- :name listUsers
SELECT * FROM users
```

```go
import gl "github.com/mochams/glimt"

reg := gl.NewRegistry(gl.DialectPostgres)

users := reg.MustGet("listUsers").Where(gl.Eq("status", "active"))
users.Where(gl.Eq("role", "admin"))
users.Limit(10)
users.Offset(2)

sql, args := users.Build()
```

Generated SQL (Postgres):

```sql
SELECT * FROM users
WHERE status = $1 AND role = $2
LIMIT $3 OFFSET $4
```

Generated Args:

```txt
["active", "admin", 10, 2]
```

Glimt lets you **write SQL once and compose predicates dynamically.**

Installation

```bash
go get github.com/mochams/glimt
```

Glimt lets you build queries directly in Go when needed

```go
reg := gl.NewRegistry(gl.DialectPostgres)

sql, args := reg.Query("SELECT * FROM users").
    Where(gl.And(
        gl.Eq("status", "active"),
        gl.Gt("age", 18),
    )).
    OrderBy("created_at DESC").
    Limit(20).
    Offset(0).
    Build()
```

Define queries in `.sql` files and load them by name.

```sql
-- :name listUsers
SELECT * FROM users

-- :name getUserByID
SELECT * FROM users WHERE id = ?
```

```go
reg := gl.NewRegistry(gl.DialectPostgres)

reg.Load("queries/users.sql")

sql, args := reg.MustGet("listUsers").
    Where(gl.Eq("role", "admin")).
    OrderBy("created_at DESC").
    Limit(10).
    Build()
```

Work seamlessly with Go's `embed`.

```go
//go:embed queries
var sqlFiles embed.FS

reg := gl.NewRegistry(gl.DialectPostgres)

reg.WalkFS(sqlFiles, "queries")
```

Glimt automatically writes placeholders for the target database.

| Database       | Example placeholders |
| -------------- | -------------------- |
| Postgres       | `$1, $2, $3`         |
| MySQL / SQLite | `?, ?, ?`            |
| SQL Server     | `@p1, @p2`           |
| Oracle         | `:1, :2`             |

SQL files should always use `?` placeholders. They are rewritten to the correct format at build time.

Queries are defined using `-- :name` annotations.

```sql
-- :name listUsers
SELECT * FROM users

-- :name deleteUser
DELETE FROM users WHERE id = ?
```

Query names must be unique across all loaded files.

For dynamic filtering, avoid writing a top-level `WHERE` clause in the base query.
Instead attach conditions through the builder:

```sql
-- :name listUsers
SELECT *
FROM users
```

```go
admins := reg.MustGet("listUsers").
    Where(gl.Eq("role", "admin")).
    Build()

guests := reg.MustGet("listUsers").
    Where(gl.Eq("role", "guest")).
    Build()
```

One base query, multiple use cases, no duplication.

Glimt aims to stay **SQL-first**, **Composable**, **Lightweight**, and **Dependency-free**

See the full example in [`examples`](examples).

Full API documentation is available at:

<https://pkg.go.dev/github.com/mochams/glimt>

Inspiration

Glimt is inspired by **Yesql**, a Clojure library by Kris Jenkins that
encourages writing SQL in SQL rather than embedding it in application code.

Glimt extends this idea with **composable predicates**, making it easier
to build dynamic queries for APIs and search endpoints.
