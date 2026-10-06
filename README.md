# glimt

[![Go Reference](https://pkg.go.dev/badge/github.com/mochams/glimt.svg)](https://pkg.go.dev/github.com/mochams/glimt)

glimt keeps Postgres queries in `.sql` files and adds filters, sorting and paging to them per request.

It parses your SQL at startup, so it knows where each query's WHERE, ORDER BY and LIMIT are:

```sql
-- name: listOrders
SELECT o.id, o.total, o.status
FROM orders o
WHERE o.org_id = :org
ORDER BY o.created_at DESC;
```

```go
sql, args, err := reg.Get("listOrders").
    Bind(glimt.Args{"org": 7}).
    Where(glimt.Eq("o.status", "paid")).
    OrderBy(glimt.Desc("o.total")).
    Limit(20).
    Build()

// SELECT o.id, o.total, o.status FROM orders o
// WHERE (o.org_id = $1) AND (o.status = $2)
// ORDER BY o.total DESC LIMIT $3
// args: [7 paid 20]
```

Added conditions are ANDed with the query's own WHERE, so `o.org_id = :org` always holds. Values are always bound as args.

glimt supports Postgres only, needs Go 1.25+, and has no dependencies. It is before v1.0, so the API may still change.

```sh
go get github.com/mochams/glimt
```

## Writing queries

Put each query after a `-- name:` line and end it with `;`:

```sql
-- name: ordersByIDs
SELECT id, total FROM orders WHERE id IN (:ids) AND org_id = :org;
```

- A query runs from its `-- name:` line to the next one.
- Names use ASCII letters, digits and `_`, and must be unique across all files.
- The `;` lets glimt reject two statements in one query.
- A near-miss such as `-- Name: x` is an error, so a typo can't hide a query.

## Loading

```go
//go:embed queries
var queries embed.FS

reg, err := glimt.Load(queries, "queries")
if err != nil {
    log.Fatal(err)
}
```

`Load` parses every query and reports each broken one with its location:

```text
queries/orders.sql:9:13: glimt: expected expression after FROM, found "WHERE"
queries/orders.sql:14:1: glimt: duplicate query name "listOrders", first defined at queries/orders.sql:3
```

`reg.Get(name)` panics on an unknown name. Call it for every query right after `Load`, so a missing one fails at startup. Use `reg.Lookup(name)` for names chosen at run time.

## Running a query

```go
sql, args, err := reg.Get("ordersByIDs").Build(glimt.Args{"ids": []int{4, 8}, "org": 7})
// SELECT id, total FROM orders WHERE id IN ($1, $2) AND org_id = $3
// args: [4 8 7]

rows, err := db.QueryContext(ctx, sql, args...)
```

- `Args` must hold a value for every param and nothing else.
- `IN (:ids)` expands a slice into one placeholder per element. An empty slice is an error.
- `[]byte`, arrays and `driver.Valuer` types bind as one value.
- If list lengths vary a lot, write `= ANY(:ids)` to keep the SQL the same for every length.

## Composing per request

`Bind` returns a `Builder`. Each method returns a new `Builder`, so a base can be shared and extended per request.

```go
b := reg.Get("listOrders").Bind(glimt.Args{"org": 7}).
    Where(
        glimt.Eq("o.status", "paid"),
        glimt.If(minTotal > 0, glimt.Ge("o.total", minTotal)), // left out when false
    ).
    OrderBy(glimt.Desc("o.total")).
    ThenBy(glimt.Asc("o.id")).
    Limit(20).
    Offset(40)

sql, args, err := b.Build()
// ... WHERE (o.org_id = $1) AND (o.status = $2 AND o.total >= $3)
//     ORDER BY o.total DESC, o.id LIMIT $4 OFFSET $5

total, totalArgs, err := b.BuildCount()
// SELECT count(*) FROM (... WHERE (o.org_id = $1) AND (o.status = $2 AND o.total >= $3)) AS t
```

A condition goes before any GROUP BY, and an existing WHERE that uses OR is wrapped in parentheses. Subqueries and CTEs stay as written.

| Kind | Functions |
| --- | --- |
| Comparison | `Eq`, `Ne`, `Lt`, `Le`, `Gt`, `Ge` |
| List | `In`, `NotIn` |
| Null | `IsNull`, `IsNotNull` |
| Pattern | `Like`, `NotLike`, `ILike`, `NotILike` |
| Substring | `Contains`, `StartsWith`, `EndsWith`, and `IContains`, `IStartsWith`, `IEndsWith` |
| Grouping | `And`, `Or`, `Not` |
| Optional | `If` |

The substring functions match `%` and `_` literally. Use `IsNull` to test for NULL, since `Eq(col, nil)` is an error.

- `OrderBy` replaces the query's ORDER BY. `ThenBy` adds terms after it, such as a tiebreaker for stable pages.
- `Limit` and `Offset` are bound as args.
- `BuildCount` drops ORDER BY, LIMIT, OFFSET, FETCH and locking clauses.

Not every statement takes every change:

| Statement | `Where` | Order and paging | `BuildCount` |
| --- | --- | --- | --- |
| SELECT | yes | yes | yes |
| UNION, VALUES, TABLE | no | yes | yes |
| UPDATE, DELETE | yes | no | no |
| INSERT, MERGE, DDL | no | no | no |

## Columns from a request

Columns are written into the SQL as given, so never pass request input as one. When a request picks a column, list the allowed ones in a `Columns` map:

```go
var sortable = glimt.Columns{"created": "o.created_at", "total": "o.total"}

b = b.OrderBy(sortable.Order(req.Sort, req.Desc)) // unknown field: ErrUnknownField at Build
```

Call `sortable.Check()` at startup. A column must be a plain reference such as `status`, `o.status` or `"Status"`, and a reserved word like `order` must be quoted or qualified.

## Errors

Every message starts with `glimt:`. `Load` returns `*glimt.LoadError` values joined together. `Build` and `BuildCount` wrap a sentinel for `errors.Is`:

| Sentinel | Cause |
| --- | --- |
| `ErrMissingArg`, `ErrUnknownArg` | A param without a value, or a value without a param |
| `ErrEmptyList` | An empty list for `In`, `NotIn` or `IN (:name)` |
| `ErrNilValue` | An untyped `nil` in a comparison or as a list |
| `ErrBadColumn` | A column that isn't a plain column reference |
| `ErrUnknownField` | A field that isn't in a `Columns` |
| `ErrNotComposable` | A change the statement can't take |

## How glimt reads SQL

glimt reads a query's structure, its clauses, params and subqueries, and copies the rest as written. It parses SELECT, VALUES, TABLE, INSERT, UPDATE, DELETE and MERGE; DDL passes through.

It is tested against Postgres's own parser on the Postgres regression suite (about 45,000 statements), and CI runs its output against a real Postgres.

Some SQL is not supported. Each is an error when the queries load:

- `SELECT … INTO`. Use `CREATE TABLE … AS` instead.
- Statements that change data inside `WITH`, as in `WITH x AS (DELETE …)`.
- A column label named `case` without `AS`, as in `SELECT x case`. Write `x AS case`.
- A comment inside a string literal continued on the next line.

Two rules to know:

- Row values, as in `(a, b) IN (:pairs)`, aren't supported, and glimt doesn't catch them at load time. Pass one array per column instead: `(a, b) IN (SELECT * FROM unnest(:as::int[], :bs::int[]))`.
- In an array slice, `a[:hi]` reads `:hi` as a param. Write `a[: hi]` to slice up to a column named `hi`.

See [architecture.md](architecture.md) for how glimt works inside.

## Development

```sh
make test lint fuzz   # main module
make oracle-corpus    # fetch the Postgres regression suite
make oracle           # check against Postgres's parser; set GLIMT_PG_DSN for a live database
```

The Postgres checks live in the `integration` module, so the main module stays dependency-free.

## Inspiration

glimt is inspired by [Yesql](https://github.com/krisajenkins/yesql), a Clojure library by Kris Jenkins built on the idea that SQL is best written as SQL.
