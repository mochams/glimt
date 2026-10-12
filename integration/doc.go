// Package integration checks glimt against Postgres. It is a separate module,
// so the main module keeps zero dependencies; it may import glimt's internal
// packages because its import path is under the main module's.
//
// # The oracle
//
// The oracle runs every statement of a corpus through glimt and compares the
// result with Postgres's own parser, through pg_query_go:
//   - tokens: glimt's token boundaries and keyword categories match
//     Postgres's scanner, for statements without params;
//   - render: Postgres parses glimt's rendered SQL to the same tree as the
//     source with each :name written as its placeholder;
//   - clauses: wrapping each clause body glimt found (WHERE, HAVING, LIMIT,
//     OFFSET, MERGE conditions, ON CONFLICT … WHERE) in parentheses leaves
//     Postgres's tree unchanged, so glimt cut each clause where Postgres does;
//   - presence: each clause (FROM, WHERE, GROUP BY, ORDER BY, LIMIT, …) is
//     in glimt's AST exactly when it is in Postgres's tree, so no clause was
//     folded into the one before it;
//   - accept: glimt accepts every SELECT, INSERT, UPDATE, DELETE and MERGE
//     Postgres accepts, unless it reports the construct as not supported;
//   - params: glimt finds no :name in SQL Postgres accepts as it is, apart
//     from the documented a[:hi].
//
// TestReservedWords and TestKeywordCategories check glimt's keywords against
// Postgres's src/include/parser/kwlist.h, fetched by make oracle-corpus.
//
// The corpus is every SQL string in glimt's tests (testdata/glimt.jsonl,
// from make corpus), real application queries (testdata/queries.sql), and
// Postgres's regression suite (testdata/regress, from make oracle-corpus).
//
// # Live Postgres
//
// With GLIMT_PG_DSN set, TestPostgres runs rendered SQL against a real
// database, in a schema of its own that it drops afterwards.
package integration
