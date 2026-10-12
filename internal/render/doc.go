// Package render turns a parsed query into SQL with Postgres placeholders
// and the args to bind to them.
//
// # Compile once, render per request
//
// [Compile] runs at load time, once per query. It writes the AST as SQL and
// cuts that SQL at each param, giving a [Template]. [Template.Render] runs
// per request. It only numbers the params and binds the values, and for a
// query whose params don't expand it returns a SQL string built at load time
// without allocating.
//
// The SQL is written from the AST. Clause keywords are written in upper
// case, aliases always get AS, and expressions, subqueries included, are
// copied token by token from the source, with one space wherever the source
// had whitespace or comments. A placeholder that would touch the word before
// it, as in THEN:x, gets a space too, since Postgres would read "THEN$1" as
// one identifier.
//
// # Composition
//
// Compile also cuts the top-level statement at its composable clauses:
// WHERE, ORDER BY, LIMIT, OFFSET, FETCH and the locking clause. A clause the
// query has is cut at its start, where its body begins and at its end; a
// clause it lacks is cut once, where it would go. Postgres has no node for
// parentheses around a whole query, so in (SELECT … LIMIT 1) the LIMIT is
// the statement's, and it is cut where it is.
//
// A composed render is a [Run] the caller steps through. [Template.Begin]
// takes a [Plan], plain data saying what the request adds, and refuses a
// plan the statement can't take before anything is written. [Run.Next]
// writes the template, and for each clause does what the plan says: keeps
// it, jumps over it, writes it anew, or extends it. It stops only at a
// [Hole] where the caller writes its own SQL, the condition to AND with the
// WHERE clause or the ORDER BY terms, through [Run.Writer]. Render writes
// every keyword and separator itself, binds LIMIT and OFFSET values from
// the plan, and ANDs a condition as (original) AND (added). Counting drops
// the tail clauses and wraps the query in SELECT count(*). A clause that is
// jumped over leaves its params unbound, and composed values are numbered
// after the query's own.
//
// Run is a struct, not an interface the caller implements, so it and the
// caller's state stay on the stack. Render, for a query used as written, is
// the same walk with an empty plan.
//
// # Params
//
// Render takes one value per distinct param name, in the order of
// [Template.Names]. Params are numbered $1, $2, … in order of first use, and a
// name used again reuses its number.
//
// A param written as IN (:name) expands to one placeholder per element when
// its value is a slice, so IN (:ids) with three ids becomes IN ($1, $2, $3).
// A []byte, an array such as a [16]byte UUID, and any driver.Valuer are not
// lists: they bind as one value. Expanding an empty list or a nil value is an
// error, and so are list elements that are themselves lists. A name used both
// in IN (:name) and elsewhere is numbered separately for each use, since one
// binds the elements and the other the whole value.
//
// Postgres accepts at most 65,535 args in one statement; Render reports an
// error past that instead of letting the server fail.
//
// # IN lists and statement caches
//
// Expanding IN (:ids) gives different SQL text for each list length, and a
// driver that caches prepared statements by SQL text, as pgx does, keeps one
// statement per length. Where lengths vary widely, write = ANY(:ids) instead:
// it binds the whole slice as one array and gives the same text every time.
// pgx binds Go slices as arrays directly; lib/pq needs pq.Array.
package render
