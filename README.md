# Glimt

![GoDoc](https://pkg.go.dev/badge/github.com/mochams/glimt.svg)

**Glimt** is a lightweight SQL toolkit for Go that keeps queries in `.sql` files
while allowing **safe runtime composition of predicates**.

Many applications prefer writing SQL in `.sql` files instead of embedding
large queries directly in code. However, APIs still need to dynamically add:

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

sql, args := reg.MustGet("listUsers").
    Where(gl.Eq("status", "active"), gl.Eq("role", "admin")).
    Limit(10).
    Offset(2).
    Build()
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

Define queries in `.sql` files and load them by name.

```sql
-- :name listUsers
SELECT * FROM users
```

```go
reg := gl.NewRegistry(gl.DialectPostgres)

reg.LoadFile("queries/users.sql")

admins, args := reg.MustGet("listUsers").
    Where(gl.Eq("role", "admin")).
    Limit(10).
    Build()

user, args := reg.MustGet("listUsers").
    Where(gl.Eq("id", userID)).
    Limit(1).
    Build()
```

Work seamlessly with Go's `embed`.

```go
//go:embed queries
var sqlFiles embed.FS

reg := gl.NewRegistry(gl.DialectPostgres)

reg.LoadFS(sqlFiles, "queries")
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

For SQL defined in Go that runs on every request, register it once at startup
so it is sanitized once, like queries loaded from files:

```go
if err := reg.Add("activeUsers", "SELECT * FROM users WHERE deleted_at IS NULL"); err != nil {
    log.Fatal(err)
}
```

Queries are defined using `-- :name` annotations.

```sql
-- :name listUsers
SELECT * FROM users

-- :name deleteUser
DELETE FROM users WHERE id = ?
```

Query names must be unique across all loaded files.

By default, dynamic clauses are appended to the end of the base query, so a
query without a fixed `WHERE` can be filtered directly:

```sql
-- :name listUsers
SELECT * FROM users
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

For API endpoints, apply only the filters a request uses. `gl.If` skips a
filter when its condition is false, and `WHERE` is omitted when no filter applies:

```go
where := []gl.Predicate{
    gl.If(status != "", gl.Eq("status", status)),
    gl.If(len(roles) > 0, gl.In("role", roles...)),
    // IContains ignores case and escapes % and _ in user input, so they match literally.
    gl.If(search != "", gl.Or(gl.IContains("name", search), gl.IContains("email", search))),
}

sql, args := reg.MustGet("listUsers").Where(where...).Limit(20).Build()
```

Use a plain `if` for filters that dereference a pointer:

```go
if req.MinAge != nil {
    where = append(where, gl.Gte("age", *req.MinAge))
}
```

To keep a fixed `WHERE`, `GROUP BY`, `UNION` or `RETURNING` in the SQL file,
mark where the filters go. Markers are comments, so the file still runs as-is
in psql:

```sql
-- :name countOrders
SELECT status, COUNT(*) FROM orders
WHERE org_id = ? AND deleted_at IS NULL /* :and */
GROUP BY status
```

```go
sql, args := reg.MustGet("countOrders").
    Args(orgID).
    Where(where...).
    Build()
// SELECT status, COUNT(*) FROM orders
// WHERE org_id = $1 AND deleted_at IS NULL AND status = $2
// GROUP BY status
```

Use `/* :and */` after a fixed `WHERE`, and `/* :where */` where there is none.
Both render nothing when no filter applies.

Subqueries can be named too. `InQuery` and `Exists` embed one query in another,
and all placeholders are numbered once:

```sql
-- :name listUsers
SELECT * FROM users

-- :name tripUserIDs
SELECT user_id FROM trip_users
```

```go
sql, args := reg.MustGet("listUsers").
    Where(gl.InQuery("id", reg.MustGet("tripUserIDs").Where(gl.Eq("trip_id", tripID)))).
    Where(gl.Eq("status", "active")).
    Build()
// SELECT * FROM users
// WHERE id IN (SELECT user_id FROM trip_users WHERE trip_id = $1) AND status = $2
```

For "not in", prefer `gl.Not(gl.Exists(...))`, which is not tripped up by NULLs.

Let clients choose the sort order without passing request input into SQL.
Only allowlisted fields are accepted; a leading `-` sorts descending:

```go
order, err := gl.ParseSort(r.URL.Query().Get("sort"), map[string]string{
    "created": "o.created_at",
    "total":   "o.total",
})
if err != nil {
    http.Error(w, err.Error(), http.StatusBadRequest)
    return
}

q.OrderBy(append(order, "o.id")...) // "-created" → ORDER BY o.created_at DESC, o.id
```

For an ordering that needs a value, such as pinning one row to the top, use
`OrderByExpr` and pass the value as an argument:

```go
q.OrderByExpr("CASE WHEN o.id = ? THEN 0 ELSE 1 END", pinnedID)
```

One filtered query gives both a page and its total. `BuildCount` leaves out
`ORDER BY`, `LIMIT` and `OFFSET`:

```go
q := reg.MustGet("listOrders").Args(orgID).Where(where...).OrderBy(order...)

page, pageArgs := q.Limit(20).Offset(40).Build()
total, totalArgs := q.BuildCount() // SELECT COUNT(*) FROM (...) AS t
```

Glimt automatically writes placeholders for the target database.

<table style="width: 100%;">
  <tr>
    <th>Database</th>
    <th>Placeholders</th>
  </tr>
  <tr>
    <td>Postgres</td>
    <td>$1, $2, $3</td>
  </tr>
  <tr>
    <td>MySQL, SQLite</td>
    <td>?, ?, ?</td>
  </tr>
</table>

SQL files should always use `?` placeholders. They are rewritten to the correct format at build time.
Write `??` for a literal question mark, such as the Postgres JSONB `?` operator.

Comments are stripped once, when queries are loaded. Stripping understands string
literals and quoted identifiers, so `'--'` inside a literal is kept.

Glimt aims to stay **SQL-first**, **Composable**, **Lightweight**, and **Dependency-free**

Release notes and upgrade steps are in [CHANGELOG.md](CHANGELOG.md).

Full API documentation, with runnable examples, is available at:

<https://pkg.go.dev/github.com/mochams/glimt>

Inspiration

Glimt is inspired by **Yesql**, a Clojure library by Kris Jenkins that
encourages writing SQL in SQL rather than embedding it in application code.

Glimt extends this idea with **composable predicates**, making it easier
to build dynamic queries for APIs and search endpoints.
