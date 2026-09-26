package glimt

import "strings"

// Predicate represents a SQL condition that evaluates to a boolean.
//
// A nil Predicate is valid and renders nothing: Where, Having, And, Or and Not
// skip it. This makes optional filters safe to compose (see If).
type Predicate func(*sqlBuilder)

// Cond creates a predicate from a raw SQL expression and its arguments.
// The expression is wrapped in parentheses so it composes safely with other predicates.
// A blank expression renders nothing.
// Example usage: Cond("age > ? OR vip", 30)
// creates "(age > ? OR vip)" with argument 30.
func Cond(expr string, args ...any) Predicate {
	if strings.TrimSpace(expr) == "" {
		return nil
	}

	return func(b *sqlBuilder) {
		b.writeByte('(')
		b.write(expr)
		b.writeByte(')')
		b.args = append(b.args, args...)
	}
}

// If returns p when cond is true, and nil otherwise.
// Because nil predicates render nothing, If expresses an optional filter inline.
// Example usage: If(status != "", Eq("status", status))
//
// The arguments of p are evaluated even when cond is false, so avoid dereferencing
// pointers inside If. Appending to a []Predicate and calling Where(preds...) avoids this.
func If(cond bool, p Predicate) Predicate {
	if cond {
		return p
	}

	return nil
}

// And combines multiple predicates with a logical AND.
// Nil and empty predicates are skipped; the result is parenthesized only when
// two or more predicates render, and renders nothing when none do.
// Example usage: And(Gt("age", 30), Eq("status", "active"))
// creates "(age > ? AND status = ?)" with arguments 30 and "active".
func And(preds ...Predicate) Predicate {
	return func(b *sqlBuilder) {
		b.group(" AND ", preds)
	}
}

// Or combines multiple predicates with a logical OR.
// Nil and empty predicates are skipped; the result is parenthesized only when
// two or more predicates render, and renders nothing when none do.
// Example usage: Or(Lt("age", 18), Gt("age", 65))
// creates "(age < ? OR age > ?)" with arguments 18 and 65.
func Or(preds ...Predicate) Predicate {
	return func(b *sqlBuilder) {
		b.group(" OR ", preds)
	}
}

// In creates a predicate for an IN clause with the specified column and values.
// Values can be listed or spread from a typed slice: In("id", ids...).
// An empty value list renders "1=0", which matches no rows, so a filter such as
// In("org_id", allowed...) never widens to every row when allowed is empty.
// Example usage: In("id", 1, 2, 3)
// creates "id IN (?, ?, ?)" with arguments 1, 2, and 3.
//
// For long lists on Postgres, Cond("id = ANY(?)", ids) binds a single array
// argument instead of one placeholder per value.
func In[T any](col string, vals ...T) Predicate {
	return func(b *sqlBuilder) {
		if len(vals) == 0 {
			b.write("1=0")

			return
		}

		writeList(b, col, " IN (", vals)
	}
}

// NotIn creates a predicate for a NOT IN clause with the specified column and values.
// Values can be listed or spread from a typed slice: NotIn("id", ids...).
// An empty value list renders "1=1", which matches every row.
// Example usage: NotIn("id", 1, 2, 3)
// creates "id NOT IN (?, ?, ?)" with arguments 1, 2, and 3.
func NotIn[T any](col string, vals ...T) Predicate {
	return func(b *sqlBuilder) {
		if len(vals) == 0 {
			b.write("1=1")

			return
		}

		writeList(b, col, " NOT IN (", vals)
	}
}

// writeList writes "col<op>?, ?, ...)" and appends vals as arguments.
func writeList[T any](b *sqlBuilder, col, op string, vals []T) {
	b.write(col)
	b.write(op)

	for i := range vals {
		if i > 0 {
			b.write(", ")
		}

		b.writeByte('?')
		b.arg(vals[i])
	}

	b.writeByte(')')
}

// InQuery creates a predicate for an IN clause with a subquery.
// The subquery keeps its own Args, filters and clause markers. It is rendered
// when the outer query is built, and its placeholders are numbered together
// with the outer query's, so both can come from the registry:
//
//	InQuery("id", reg.MustGet("tripUserIDs").Where(Eq("trip_id", 42)))
//
// creates "id IN (SELECT user_id FROM trip_users WHERE trip_id = ?)" with argument 42.
//
// For "not in", prefer Not(Exists(...)): NOT IN matches no rows as soon as
// the subquery returns a NULL.
//
// InQuery panics if q is nil. A nil subquery is a programming error, such as
// an ignored error from Registry.Get; for an optional subquery filter, use If.
func InQuery(col string, q *Query) Predicate {
	if q == nil {
		panic("glimt: InQuery: nil subquery")
	}

	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" IN (")
		q.build(b)
		b.writeByte(')')
	}
}

// Exists creates a predicate for an EXISTS clause with a subquery.
// Like InQuery, the subquery is rendered when the outer query is built and its
// placeholders are numbered with the outer query's. Correlate it with the
// outer query in its SQL, for example "SELECT 1 FROM orders o WHERE o.user_id = users.id".
// Example usage: Exists(reg.MustGet("userOrders").Where(Eq("o.status", "paid")))
// creates "EXISTS (SELECT 1 FROM orders o WHERE o.user_id = users.id AND o.status = ?)".
//
// Exists panics if q is nil, for the same reason as InQuery.
func Exists(q *Query) Predicate {
	if q == nil {
		panic("glimt: Exists: nil subquery")
	}

	return func(b *sqlBuilder) {
		b.write("EXISTS (")
		q.build(b)
		b.writeByte(')')
	}
}

// Not creates a predicate that negates the given predicate with a logical NOT.
// It renders nothing when pred is nil or renders nothing.
// Example usage: Not(Eq("status", "active"))
// creates "NOT (status = ?)" with argument "active".
func Not(pred Predicate) Predicate {
	return func(b *sqlBuilder) {
		mark := len(b.buf)
		b.write("NOT (")

		if !b.render(pred) {
			b.buf = b.buf[:mark]

			return
		}

		b.writeByte(')')
	}
}

// Eq creates a predicate for an equality condition between a column and a value.
// Example usage: Eq("name", "Alice")
// creates "name = ?" with argument "Alice".
func Eq(col string, val any) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" = ?")
		b.arg(val)
	}
}

// Neq creates a predicate for an inequality condition between a column and a value.
// Example usage: Neq("status", "inactive")
// creates "status <> ?" with argument "inactive".
func Neq(col string, val any) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" <> ?")
		b.arg(val)
	}
}

// Gt creates a predicate for a greater-than condition between a column and a value.
// Example usage: Gt("age", 30)
// creates "age > ?" with argument 30.
func Gt(col string, val any) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" > ?")
		b.arg(val)
	}
}

// Gte creates a predicate for a greater-than-or-equal condition between a column and a value.
// Example usage: Gte("age", 18)
// creates "age >= ?" with argument 18.
func Gte(col string, val any) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" >= ?")
		b.arg(val)
	}
}

// Lt creates a predicate for a less-than condition between a column and a value.
// Example usage: Lt("age", 65)
// creates "age < ?" with argument 65.
func Lt(col string, val any) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" < ?")
		b.arg(val)
	}
}

// Lte creates a predicate for a less-than-or-equal condition between a column and a value.
// Example usage: Lte("age", 65)
// creates "age <= ?" with argument 65.
func Lte(col string, val any) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" <= ?")
		b.arg(val)
	}
}

// Null creates a predicate for an IS NULL condition.
// Example usage: Null("deleted_at")
// creates "deleted_at IS NULL".
func Null(col string) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" IS NULL")
	}
}

// NotNull creates a predicate for an IS NOT NULL condition.
// Example usage: NotNull("deleted_at")
// creates "deleted_at IS NOT NULL".
func NotNull(col string) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" IS NOT NULL")
	}
}

// Like creates a predicate for a LIKE condition.
// Example usage: Like("name", "%john%")
// creates "name LIKE ?" with argument "%john%".
func Like(col, pattern string) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" LIKE ?")
		b.arg(pattern)
	}
}

// NotLike creates a predicate for a NOT LIKE condition.
// Example usage: NotLike("name", "%john%")
// creates "name NOT LIKE ?" with argument "%john%".
func NotLike(col, pattern string) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" NOT LIKE ?")
		b.arg(pattern)
	}
}

// ILike creates a predicate for a case-insensitive ILIKE condition (Postgres only).
// Example usage: ILike("name", "%john%")
// creates "name ILIKE ?" with argument "%john%".
func ILike(col, pattern string) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" ILIKE ?")
		b.arg(pattern)
	}
}

// likeEscaper escapes LIKE wildcards with '!'. Unlike a backslash, '!' needs no
// escaping inside SQL string literals on any supported dialect.
var likeEscaper = strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")

// Contains creates a predicate matching rows where col contains s.
// Wildcards in s match literally, so user input such as "50%" is safe to pass.
// Example usage: Contains("name", "50%")
// creates "name LIKE ? ESCAPE '!'" with argument "%50!%%".
//
// LIKE is case-sensitive on Postgres; for case-insensitive search, use
// IContains. An empty s matches every non-NULL value.
func Contains(col, s string) Predicate {
	return likeEscaped(col, "%"+likeEscaper.Replace(s)+"%", false)
}

// StartsWith creates a predicate matching rows where col starts with s.
// Wildcards in s match literally.
// Example usage: StartsWith("sku", "AB_")
// creates "sku LIKE ? ESCAPE '!'" with argument "AB!_%".
func StartsWith(col, s string) Predicate {
	return likeEscaped(col, likeEscaper.Replace(s)+"%", false)
}

// EndsWith creates a predicate matching rows where col ends with s.
// Wildcards in s match literally.
// Example usage: EndsWith("email", "@example.com")
// creates "email LIKE ? ESCAPE '!'" with argument "%@example.com".
func EndsWith(col, s string) Predicate {
	return likeEscaped(col, "%"+likeEscaper.Replace(s), false)
}

// IContains creates a case-insensitive Contains. Wildcards in s match literally.
// Example usage: IContains("name", "doe")
// creates "name ILIKE ? ESCAPE '!'" on Postgres and "name LIKE ? ESCAPE '!'"
// on MySQL and SQLite, with argument "%doe%".
//
// Postgres uses ILIKE, which a pg_trgm GIN index can serve. On MySQL and
// SQLite, LIKE already ignores case in the common case: MySQL follows the
// column's collation (the default _ci collations ignore case, and _ai_ci ones
// also ignore accents; _bin and _cs collations do not), and SQLite ignores
// case for ASCII letters only.
func IContains(col, s string) Predicate {
	return likeEscaped(col, "%"+likeEscaper.Replace(s)+"%", true)
}

// IStartsWith creates a case-insensitive StartsWith. Wildcards in s match literally.
// It renders like IContains; see IContains for how each dialect handles case.
// Example usage: IStartsWith("sku", "ab_")
// creates "sku ILIKE ? ESCAPE '!'" on Postgres with argument "ab!_%".
func IStartsWith(col, s string) Predicate {
	return likeEscaped(col, likeEscaper.Replace(s)+"%", true)
}

// IEndsWith creates a case-insensitive EndsWith. Wildcards in s match literally.
// It renders like IContains; see IContains for how each dialect handles case.
// Example usage: IEndsWith("email", "@Example.com")
// creates "email ILIKE ? ESCAPE '!'" on Postgres with argument "%@Example.com".
func IEndsWith(col, s string) Predicate {
	return likeEscaped(col, "%"+likeEscaper.Replace(s), true)
}

// likeEscaped writes "col LIKE ? ESCAPE '!'" with the escaped pattern as its
// argument. With ignoreCase, Postgres gets ILIKE; MySQL and SQLite keep LIKE,
// which ignores case under their defaults.
func likeEscaped(col, pattern string, ignoreCase bool) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)

		if ignoreCase && b.dialect == DialectPostgres {
			b.write(" ILIKE ? ESCAPE '!'")
		} else {
			b.write(" LIKE ? ESCAPE '!'")
		}

		b.arg(pattern)
	}
}

// Between creates a predicate for a BETWEEN condition with the specified column and range values.
// Example usage: Between("age", 18, 65)
// creates "age BETWEEN ? AND ?" with arguments 18 and 65.
func Between(col string, low, high any) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" BETWEEN ? AND ?")
		b.arg(low)
		b.arg(high)
	}
}

// RangeOpen creates a predicate for an open range condition with the specified column and range values.
// Example usage: RangeOpen("age", 18, 65)
// creates "age > ? AND age < ?" with arguments 18 and 65.
func RangeOpen(col string, low, high any) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" > ? AND ")
		b.write(col)
		b.write(" < ?")
		b.arg(low)
		b.arg(high)
	}
}

// NotBetween creates a predicate for a NOT BETWEEN condition with the specified column and range values.
// Example usage: NotBetween("age", 18, 65)
// creates "age NOT BETWEEN ? AND ?" with arguments 18 and 65.
func NotBetween(col string, low, high any) Predicate {
	return func(b *sqlBuilder) {
		b.write(col)
		b.write(" NOT BETWEEN ? AND ?")
		b.arg(low)
		b.arg(high)
	}
}
