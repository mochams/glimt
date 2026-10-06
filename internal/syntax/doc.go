// Package syntax holds glimt's lexer, tokens, structural parser and AST for
// Postgres. [Parse] lexes one query's SQL and parses it into an AST, and
// [SplitFile] splits a .sql file into its "-- name:" queries.
//
// # Scope
//
// The parser is structural, not semantic. It answers where things are:
//   - the statement kind: SELECT, VALUES, TABLE, INSERT, UPDATE, DELETE, MERGE,
//     or a Raw statement such as DDL;
//   - where each top-level clause (WHERE, ORDER BY, LIMIT, …) starts and ends;
//   - whether an INSERT takes its rows from a query;
//   - where the named params (:name) and parenthesized subqueries are, and
//     which params are written as IN (:name) and so expand to a list;
//   - the structure of UNION, INTERSECT and EXCEPT.
//
// It never answers what things mean. Expressions are kept as token spans and
// are never checked for types, columns or functions. JOINs are not modelled:
// FROM is one opaque span that joins pass through untouched. Anything glimt
// doesn't support is a "… is not supported" error, never a silent misparse.
//
// The AST is plain data. Composing queries and rendering SQL happen in later
// stages, which must not mutate a parsed AST because it is shared once cached.
// See architecture.md at the repository root.
//
// # Lexer contract
//
// The lexer follows Postgres's scanner, assuming standard_conforming_strings
// is on. The parser relies on these guarantees, which the lexer's fuzz test
// enforces:
//   - the stream ends with exactly one EOF token;
//   - tokens are in source order and don't overlap, and src[Pos:End] is each token's text;
//   - comments are gone, and Sep records whether whitespace or comments
//     separated a token from the one before it;
//   - a string literal continued on another line, as Postgres allows, is one token;
//   - a PARAM token's text is ":name" with an ASCII name; "::" is an OP token,
//     and so is a ":" inside brackets after a name, number or param, as in arr[lo:hi];
//   - positional placeholders such as $1 were rejected;
//   - strings and quoted identifiers are complete, and a QIDENT's text keeps its quotes;
//   - keywords are IDENT tokens with Kw set, and an IDENT right after "." never
//     has one, since a field name can be any word.
//
// # Known limits
//
//   - A clause ends at "ON CONFLICT", except for the condition of a JOIN in
//     a FROM, USING or MERGE source, so elsewhere a condition can't start
//     with a column named conflict; quote it. Misplaced, it is an error,
//     never a misparse.
//   - USING doesn't end a clause, since it is valid inside JOIN … USING and
//     ORDER BY … USING. Invalid SQL such as "WHERE a USING s" stays in the span
//     and fails in Postgres.
//   - Expand marks how a param is written, not its type. The renderer binds a
//     slice's elements, and rejects lists of lists, so row values such as
//     (a, b) IN (:pairs) are not supported yet.
//   - In a slice, a[:hi] reads :hi as a param; write a[: hi] for a column bound.
//   - A column label named case after a plain name, as in SELECT x case, must
//     be written AS case; glimt reads a bare CASE there as a CASE expression.
//   - A comment inside a continued string literal is not supported.
//   - SELECT INTO, data-modifying statements inside WITH, and a query in
//     parentheses followed by RETURNING, as in JSON_ARRAY(SELECT … RETURNING
//     jsonb), are not supported yet.
//   - A "*" after a table name, which includes the tables inheriting from it,
//     is accepted and dropped: it is Postgres's default.
package syntax
