# Changelog

All notable changes to glimt are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Before 1.0.0, minor releases may contain breaking changes; they are listed under
**Upgrading** in each release.

## [0.4.0] - Unreleased

This release makes dynamic queries for API endpoints safe to compose: optional
filters, search, client-chosen sorting, pagination and subqueries. Named queries
can now keep fixed clauses in SQL.

### Upgrading from 0.3.0

- `DialectSQLServer` and `DialectOracle` are removed. glimt supports Postgres,
  MySQL and SQLite.
- `In` and `NotIn` are generic. Spread typed slices directly:
  `In("id", ids...)`. A list of mixed types no longer compiles; spread a `[]any`
  instead. A call with no values needs a type argument: `In[int]("id")`.
- `Cond` wraps its expression in parentheses. Tests that compare exact SQL
  strings need updating.
- `Having` adds to earlier `Having` calls instead of replacing them.
- Some `.sql` files that loaded before now fail to load, with the line number.
  This happens for:
  - an unknown `-- :word` annotation, or the `-- name: x` style used by other
    tools (write `-- :name x`);
  - unterminated quotes or block comments;
  - native `$1` placeholders on Postgres (write `?`).
- Error messages changed. Load errors show the full path and a line number.
  `Get` errors wrap `ErrNotFound`; check them with `errors.Is` instead of
  comparing strings.

### Added

- `If(cond, p)` for inline optional filters. `Where` and `Having` accept several
  predicates at once.
- Clause markers. In a query, `/* :where */` renders ` WHERE <predicates>` and
  `/* :and */` renders ` AND <predicates>`. A named query can keep a fixed
  `WHERE`, `GROUP BY`, `UNION` or `RETURNING` in SQL. The markers are comments,
  so the file still runs as-is in a database client.
- `InQuery(col, q)` and `Exists(q)` for subqueries. Both queries can be named,
  and all placeholders are numbered once. They panic on a nil query.
- `ParseSort` turns a client sort parameter such as `-created,total` into
  `ORDER BY` expressions, and only accepts allowlisted fields. Invalid input
  returns a `*SortError`.
- `Contains`, `StartsWith` and `EndsWith` for search. They escape `%` and `_` in
  user input so it matches literally.
- `Registry.Add(name, sql)` registers SQL defined in Go code, sanitized once like
  a loaded file.
- `ErrNotFound`, returned (wrapped) by `Registry.Get`.
- `??` writes a literal `?`, for example for the Postgres JSONB `?`, `?|` and
  `?&` operators.
- Runnable examples on pkg.go.dev.

### Changed

- Nil and empty predicates render nothing. `WHERE` and `HAVING` are left out
  when no predicate applies.
- `In` with an empty list renders `1=0` and matches no rows, so an access filter
  such as `In("org_id", allowed...)` never widens to every row. `NotIn` with an
  empty list renders `1=1`.
- All SQL is sanitized once, when it is loaded or added: comments are removed,
  whitespace is normalized and placeholders are counted. `Get` does no string
  work.
- Comment stripping understands string literals, quoted identifiers, Postgres
  dollar-quoted bodies and nested comments, and MySQL `#` comments and backslash
  escapes. Whitespace inside literals is preserved. Optimizer hints (`/*+ */`)
  and MySQL `/*! */` comments are kept.
- Annotations allow flexible spacing, so `--:name x` works.
- On MySQL, two minus signs that are not a comment are written apart:
  `1--1` becomes `1- -1`, which MySQL evaluates the same way.
- A load that fails adds none of the file's queries.
- Building queries is faster.

### Removed

- `DialectSQLServer` and `DialectOracle`.

### Fixed

- Empty predicates no longer produce invalid SQL such as `WHERE ` or `( AND )`,
  and a nil predicate no longer panics.
- `In("id", ids)` with a typed slice bound the whole slice as a single value.
- A `Cond` containing `OR` combined with other filters with the wrong
  precedence.
- The parser no longer corrupts SQL. It used to cut `--` and `/*` inside string
  literals, delete the rest of a query after an unclosed `/*`, and join tokens
  around a removed comment (`a/**/b`).
- A typo such as `--:name` no longer merges one query into the one before it.
- `OFFSET` without `LIMIT` works on MySQL and SQLite.
- Query files with lines longer than 64 KB load.

### Security

- Ad-hoc SQL passed to `Registry.Query` or `NewQuery` is now sanitized. A
  trailing `-- comment` used to swallow the clauses glimt appends, so filters
  such as an owner check were silently dropped and every row was returned.

[0.4.0]: https://github.com/mochams/glimt/compare/v0.3.0...HEAD
