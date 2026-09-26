package glimt

import (
	"fmt"
	"slices"
	"strings"
)

// Query represents a SQL Query being built,
// Includes the base SQL, WHERE clause, GROUP BY, HAVING, ORDER BY, LIMIT, OFFSET, and dialect.
// It provides methods for setting these components and building the final SQL string and arguments.
type Query struct {
	tpl     *template
	where   []Predicate
	groupBy []string
	having  []Predicate
	orderBy []orderTerm
	limit   *int
	offset  *int
	dialect Dialect
	args    []any
}

// orderTerm is one ORDER BY expression and the arguments of its placeholders.
type orderTerm struct {
	expr string
	args []any
}

// NewQuery creates a new Query from ad-hoc SQL for the given dialect.
// The SQL is sanitized like a loaded query, so comments and trailing
// semicolons cannot swallow or break the clauses glimt appends. This happens
// on every call; for SQL used on every request, prefer Registry.Add.
func NewQuery(sql string, dialect Dialect) *Query {
	tpl, err := parseSQL(sql, dialect, false)
	if err != nil {
		// Unterminated quotes or comments: keep the SQL as written so the
		// database reports the syntax error. The newline ends any trailing
		// line comment, which the lexer could not find, so it cannot swallow
		// the clauses appended after it.
		tpl = &template{parts: []string{strings.TrimSpace(sql) + "\n"}, params: []int{0}}
	}

	return &Query{tpl: tpl, dialect: dialect}
}

// Where adds predicates to the WHERE clause of the query, joined with AND.
// Chaining multiple Where calls also joins them with AND.
// Nil and empty predicates are skipped, and WHERE is omitted entirely when
// nothing renders. It returns the Query for chaining.
func (q *Query) Where(preds ...Predicate) *Query {
	q.where = append(q.where, preds...)

	return q
}

// Exclude adds a NOT condition to the WHERE clause for the given predicates.
// Multiple predicates are combined with AND before negation.
// Nothing is added when no predicate renders.
func (q *Query) Exclude(preds ...Predicate) *Query {
	return q.Where(Not(And(preds...)))
}

// GroupBy adds columns to the GROUP BY clause of the query.
// It returns the Query for chaining.
func (q *Query) GroupBy(cols ...string) *Query {
	q.groupBy = append(q.groupBy, cols...)

	return q
}

// Having adds predicates to the HAVING clause of the query, joined with AND.
// Like Where, chained calls accumulate, and nil or empty predicates are skipped.
// It returns the Query for chaining.
func (q *Query) Having(preds ...Predicate) *Query {
	q.having = append(q.having, preds...)

	return q
}

// OrderBy adds columns to the ORDER BY clause of the query.
// Blank columns are ignored. It returns the Query for chaining.
func (q *Query) OrderBy(cols ...string) *Query {
	q.orderBy = slices.Grow(q.orderBy, len(cols))

	for _, col := range cols {
		q.OrderByExpr(col)
	}

	return q
}

// OrderByExpr adds an ORDER BY expression with bound arguments, such as a
// relevance rank computed from a search term:
//
//	q.OrderByExpr("ts_rank(search_vector, plainto_tsquery(?)) DESC", term)
//
// Include the direction in expr, and pass request input only as args, never
// as part of expr. A blank expr is ignored. Terms from OrderBy and OrderByExpr
// render in the order they were added. It returns the Query for chaining.
//
// With SELECT DISTINCT, Postgres and MySQL require an ORDER BY expression to
// appear in the select list. Over a UNION, ORDER BY can only use output column names.
func (q *Query) OrderByExpr(expr string, args ...any) *Query {
	if strings.TrimSpace(expr) != "" {
		q.orderBy = append(q.orderBy, orderTerm{expr: expr, args: args})
	}

	return q
}

// Limit sets the LIMIT clause of the query to the given number.
// It returns the Query for chaining.
func (q *Query) Limit(n int) *Query {
	q.limit = &n

	return q
}

// Offset sets the OFFSET clause of the query to the given number.
// On MySQL and SQLite, an offset without a limit is rendered with the
// dialect's "no limit" form, since both reject OFFSET on its own.
// It returns the Query for chaining.
func (q *Query) Offset(n int) *Query {
	q.offset = &n

	return q
}

// Args sets the arguments for the query.
// Meaningful for queries with raw placeholders in the base SQL.
// Example: NewQuery("INSERT INTO users VALUES (?, ?)", DialectPostgres).Args("doe", 42)
// They are inserted first!
func (q *Query) Args(args ...any) *Query {
	q.args = append(q.args, args...)

	return q
}

// Build constructs the final SQL string and arguments for the query.
// It rewrites placeholders according to the specified dialect, and turns
// the "??" escape into a literal question mark.
func (q *Query) Build() (string, []any) {
	sql, args := q.RawBuild()

	return writePlaceholders(sql, q.dialect), args
}

// RawBuild constructs the raw SQL string with "?" placeholders and collects arguments.
// It does not rewrite placeholders for the dialect, and leaves "??" escapes as they are.
// Useful if you want to handle placeholders yourself.
func (q *Query) RawBuild() (string, []any) {
	b := q.newBuilder()
	q.build(b)

	return b.string(), b.args
}

// BuildCount constructs a query that counts the rows this query matches, for
// pagination totals:
//
//	SELECT COUNT(*) FROM (<query>) AS t
//
// The query's ORDER BY, LIMIT and OFFSET are left out, along with their
// arguments, so one filtered query yields both a page and its total. Calling
// it before or after Limit gives the same result. Placeholders are rewritten
// like Build.
//
// A query with GROUP BY counts groups. BuildCount is meant for SELECT queries.
// On MySQL, a SELECT * over a join can produce duplicate column names, which a
// derived table rejects ("Duplicate column name"); list the columns instead.
func (q *Query) BuildCount() (string, []any) {
	b := q.newBuilder()
	b.write("SELECT COUNT(*) FROM (")
	q.buildFiltered(b)
	b.write(") AS t")

	return writePlaceholders(b.string(), q.dialect), b.args
}

// newBuilder returns a builder for this query's dialect, sized for a typical query.
func (q *Query) newBuilder() *sqlBuilder {
	b := &sqlBuilder{dialect: q.dialect}
	b.args = make([]any, 0, 10)
	b.grow(256)

	return b
}

// build writes the query into b with "?" placeholders.
// Subquery predicates use it to render an inner query into the outer builder,
// so the outer Build numbers every placeholder once.
func (q *Query) build(b *sqlBuilder) {
	q.buildFiltered(b)
	q.buildOrderBy(b)
	q.buildPage(b)
}

// buildFiltered writes the base SQL, WHERE, GROUP BY and HAVING: everything
// that decides which rows match, without ordering or pagination.
func (q *Query) buildFiltered(b *sqlBuilder) {
	tpl := q.tpl
	if tpl == nil { // zero Query: no base SQL
		tpl = &template{parts: []string{""}, params: []int{0}}
	}

	q.buildBase(b, tpl)

	if len(tpl.slots) == 0 {
		q.buildWhere(b)
	}

	q.buildGroupBy(b)
	q.buildHaving(b)
}

// buildBase writes the base SQL. Args fill the placeholders of each part in
// order, and WHERE predicates are rendered at every clause marker. Args beyond
// the base placeholders are appended last; the driver reports the mismatch.
func (q *Query) buildBase(b *sqlBuilder, tpl *template) {
	args := q.args

	for i, part := range tpl.parts {
		b.write(part)

		n := min(tpl.params[i], len(args))
		b.args = append(b.args, args[:n]...)
		args = args[n:]

		if i < len(tpl.slots) {
			q.buildSlot(b, tpl.slots[i])
		}
	}

	b.args = append(b.args, args...)
}

// buildSlot renders the WHERE predicates at a clause marker.
func (q *Query) buildSlot(b *sqlBuilder, s slot) {
	switch s {
	case slotWhere:
		b.clause(" WHERE ", " AND ", q.where)
	case slotAnd:
		b.clause(" AND ", " AND ", q.where)
	}
}

// buildWhere adds the WHERE clause to the query if any predicate renders.
func (q *Query) buildWhere(b *sqlBuilder) {
	b.clause(" WHERE ", " AND ", q.where)
}

// buildGroupBy adds the GROUP BY clause to the query if any group by columns are set.
func (q *Query) buildGroupBy(b *sqlBuilder) {
	switch len(q.groupBy) {
	case 0: // nothing
	default:
		b.write(" GROUP BY ")

		for i := range len(q.groupBy) {
			if i > 0 {
				b.write(", ")
			}

			b.write(q.groupBy[i])
		}
	}
}

// buildHaving adds the HAVING clause to the query if any predicate renders.
func (q *Query) buildHaving(b *sqlBuilder) {
	b.clause(" HAVING ", " AND ", q.having)
}

// buildOrderBy adds the ORDER BY clause to the query if any term is set.
// Each term's args follow the WHERE, GROUP BY and HAVING args, in textual order.
func (q *Query) buildOrderBy(b *sqlBuilder) {
	for i, term := range q.orderBy {
		if i == 0 {
			b.write(" ORDER BY ")
		} else {
			b.write(", ")
		}

		b.write(term.expr)
		b.args = append(b.args, term.args...)
	}
}

// buildPage adds the LIMIT and OFFSET clauses to the query if they are set.
// MySQL and SQLite reject OFFSET without LIMIT, so an offset-only query gets
// the dialect's "no limit" form. The builder's dialect is used, so a subquery
// follows the query it is rendered into.
func (q *Query) buildPage(b *sqlBuilder) {
	switch {
	case q.limit != nil:
		b.write(" LIMIT ?")
		b.arg(*q.limit)
	case q.offset != nil && b.dialect == DialectMySQL:
		b.write(" LIMIT 18446744073709551615")
	case q.offset != nil && b.dialect == DialectSQLite:
		b.write(" LIMIT -1")
	}

	if q.offset != nil {
		b.write(" OFFSET ?")
		b.arg(*q.offset)
	}
}

// SortError reports an invalid sort parameter.
// Handlers can check for it with errors.As and respond with 400 Bad Request.
type SortError struct {
	Field  string // the field as the client sent it
	Reason string
}

func (e *SortError) Error() string {
	return fmt.Sprintf("glimt: sort field %q: %s", e.Field, e.Reason)
}

// ParseSort turns a client sort parameter such as "-created,name" into
// ORDER BY expressions, accepting only the fields in allowed.
//
// Fields are comma-separated. A leading "-" sorts descending and an optional
// leading "+" ascending. allowed maps each public field name to the SQL
// expression to sort by, without a direction:
//
//	order, err := glimt.ParseSort(r.URL.Query().Get("sort"), map[string]string{
//	    "created": "o.created_at",
//	    "total":   "o.total",
//	})
//	if err != nil {
//	    // respond with 400 Bad Request
//	}
//	q.OrderBy(append(order, "o.id")...)
//
// Unknown, duplicate and empty fields return a *SortError. An empty input
// returns no expressions. Because rows with equal sort values can come back in
// any order, append a unique column as a final tiebreaker when paginating.
//
// Only map values reach the SQL, so request input is never interpolated.
func ParseSort(input string, allowed map[string]string) ([]string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	fields := strings.Split(input, ",")
	order := make([]string, 0, len(fields))
	seen := make(map[string]bool, len(fields))

	for _, field := range fields {
		expr, name, err := sortExpr(strings.TrimSpace(field), allowed)
		if err != nil {
			return nil, err
		}

		if seen[name] {
			return nil, &SortError{Field: name, Reason: "duplicate field"}
		}

		seen[name] = true
		order = append(order, expr)
	}

	return order, nil
}

// sortExpr resolves one field, such as "-created", to an ORDER BY expression.
func sortExpr(field string, allowed map[string]string) (expr, name string, err error) {
	dir := " ASC"

	name = field
	if name != "" && (name[0] == '-' || name[0] == '+') {
		if name[0] == '-' {
			dir = " DESC"
		}

		name = strings.TrimSpace(name[1:])
	}

	if name == "" {
		return "", "", &SortError{Field: field, Reason: "empty field"}
	}

	col, ok := allowed[name]
	if !ok {
		return "", "", &SortError{Field: name, Reason: "not allowed"}
	}

	return col + dir, name, nil
}

// sqlBuilder is a helper type for building SQL strings and collecting arguments.
// It uses a byte slice so output can be rolled back when a predicate renders nothing.
// dialect lets predicates render dialect-specific SQL; subqueries share the
// outer query's builder, and so its dialect.
type sqlBuilder struct {
	buf     []byte
	args    []any
	dialect Dialect
}

// write appends a string to the SQL being built.
func (b *sqlBuilder) write(s string) {
	b.buf = append(b.buf, s...)
}

// writeByte appends a single byte to the SQL being built.
func (b *sqlBuilder) writeByte(c byte) {
	b.buf = append(b.buf, c)
}

// grow preallocates space in the SQL builder to optimize for building larger queries.
func (b *sqlBuilder) grow(n int) {
	b.buf = slices.Grow(b.buf, n)
}

// arg appends an argument to the list of arguments being collected.
func (b *sqlBuilder) arg(a any) {
	b.args = append(b.args, a)
}

// string returns the built SQL string.
func (b *sqlBuilder) string() string {
	return string(b.buf)
}

// reset clears the SQL builder and argument list for reuse.
func (b *sqlBuilder) reset() {
	b.buf = b.buf[:0]
	b.args = b.args[:0]
}

// render writes p and reports whether it produced any SQL.
// A nil predicate renders nothing. If p writes no SQL, any arguments it added are discarded.
func (b *sqlBuilder) render(p Predicate) bool {
	if p == nil {
		return false
	}

	start, argStart := len(b.buf), len(b.args)
	p(b)

	if len(b.buf) == start {
		b.args = b.args[:argStart]

		return false
	}

	return true
}

// join renders preds separated by sep, skipping predicates that render nothing.
// It returns the number of predicates rendered.
func (b *sqlBuilder) join(sep string, preds []Predicate) int {
	n := 0

	for _, p := range preds {
		mark := len(b.buf)
		if n > 0 {
			b.write(sep)
		}

		if !b.render(p) {
			b.buf = b.buf[:mark]

			continue
		}

		n++
	}

	return n
}

// group renders preds joined by sep inside parentheses.
// The parentheses are dropped when only one predicate renders, and nothing is written when none do.
func (b *sqlBuilder) group(sep string, preds []Predicate) {
	mark := len(b.buf)
	b.writeByte('(')

	switch b.join(sep, preds) {
	case 0:
		b.buf = b.buf[:mark]
	case 1:
		b.buf = slices.Delete(b.buf, mark, mark+1)
	default:
		b.writeByte(')')
	}
}

// clause renders preds joined by sep after prefix (e.g. " WHERE ").
// Nothing is written, not even the prefix, when no predicate renders.
func (b *sqlBuilder) clause(prefix, sep string, preds []Predicate) {
	mark := len(b.buf)
	b.write(prefix)

	if b.join(sep, preds) == 0 {
		b.buf = b.buf[:mark]
	}
}
