package glimt

// Query represents a SQL Query being built,
// Includes the base SQL, WHERE clause, GROUP BY, HAVING, ORDER BY, LIMIT, OFFSET, and dialect.
// It provides methods for setting these components and building the final SQL string and arguments.
type Query struct {
	baseSQL string
	where   []Predicate
	groupBy []string
	having  Predicate
	orderBy []string
	limit   *int
	offset  *int
	dialect Dialect
}

// NewQuery creates a new Query with the given base SQL and default dialect.
func NewQuery(baseSQL string, dialect Dialect) *Query {
	return &Query{baseSQL: baseSQL, dialect: dialect}
}

// Where sets the WHERE clause of the query using the given Predicate.
// It returns the Query for chaining.
// Chaining multiple Where calls joins them with AND.
func (q *Query) Where(pred Predicate) *Query {
	q.where = append(q.where, pred)

	return q
}

// Exclude adds a NOT condition to the WHERE clause for the given predicates.
// Multiple predicates are combined with AND before negation.
func (q *Query) Exclude(preds ...Predicate) *Query {
	switch len(preds) {
	case 0:
		return q
	case 1:
		return q.Where(Not(preds[0]))
	default:
		return q.Where(Not(And(preds...)))
	}
}

// GroupBy adds columns to the GROUP BY clause of the query.
// It returns the Query for chaining.
func (q *Query) GroupBy(cols ...string) *Query {
	q.groupBy = append(q.groupBy, cols...)

	return q
}

// Having sets the HAVING clause of the query using the given Predicate.
// It returns the Query for chaining.
func (q *Query) Having(pred Predicate) *Query {
	q.having = pred

	return q
}

// OrderBy adds columns to the ORDER BY clause of the query.
// It returns the Query for chaining.
func (q *Query) OrderBy(cols ...string) *Query {
	q.orderBy = append(q.orderBy, cols...)

	return q
}

// Limit sets the LIMIT clause of the query to the given number.
// It returns the Query for chaining.
func (q *Query) Limit(n int) *Query {
	q.limit = &n

	return q
}

// Offset sets the OFFSET clause of the query to the given number.
// It returns the Query for chaining.
func (q *Query) Offset(n int) *Query {
	q.offset = &n

	return q
}

// Build constructs the final SQL string and arguments for the query.
// It rewrites placeholders according to the specified dialect.
func (q *Query) Build() (string, []any) {
	sql, args := q.RawBuild()

	return writePlaceholders(sql, q.dialect), args
}

// RawBuild constructs the raw SQL string with "?" placeholders and collects arguments.
// It does not rewrite placeholders for the dialect.
func (q *Query) RawBuild() (string, []any) {
	b := &sqlBuilder{}
	b.args = make([]any, 0, 10)
	b.grow(256)

	// Start with base SQL
	b.write(q.baseSQL)
	q.buildWhere(b)
	q.buildGroupBy(b)
	q.buildHaving(b)
	q.buildOrderBy(b)
	q.buildLimit(b)
	q.buildOffset(b)

	return b.string(), b.args
}

// buildWhere adds the WHERE clause to the query if any predicates are set.
func (q *Query) buildWhere(b *sqlBuilder) {
	switch len(q.where) {
	case 0: // nothing
	case 1:
		b.write(" WHERE ")
		q.where[0](b)
	default:
		b.write(" WHERE ")

		for i := range len(q.where) {
			if i > 0 {
				b.write(" AND ")
			}

			q.where[i](b)
		}
	}
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

// buildHaving adds the HAVING clause to the query if it is set.
func (q *Query) buildHaving(b *sqlBuilder) {
	if q.having != nil {
		b.write(" HAVING ")
		q.having(b)
	}
}

// buildOrderBy adds the ORDER BY clause to the query if any order by columns are set.
func (q *Query) buildOrderBy(b *sqlBuilder) {
	switch len(q.orderBy) {
	case 0: // nothing
	default:
		b.write(" ORDER BY ")

		for i := range len(q.orderBy) {
			if i > 0 {
				b.write(", ")
			}

			b.write(q.orderBy[i])
		}
	}
}

// buildLimit adds the LIMIT clause to the query if it is set.
func (q *Query) buildLimit(b *sqlBuilder) {
	if q.limit != nil {
		b.write(" LIMIT ?")
		b.arg(*q.limit)
	}
}

// buildOffset adds the OFFSET clause to the query if it is set.
func (q *Query) buildOffset(b *sqlBuilder) {
	if q.offset != nil {
		b.write(" OFFSET ?")
		b.arg(*q.offset)
	}
}
