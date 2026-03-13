package glimt

// Predicate represents a SQL condition that evaluates to a boolean.
type Predicate func(*sqlBuilder)

// Cond creates a simple predicate with the given expression and arguments.
// Example usage: Cond("age > ?", 30)
// creates a predicate that builds "age > ?" with argument 30.
func Cond(expr string, args ...any) Predicate {
	return func(b *sqlBuilder) {
		b.write(expr)
		b.args = append(b.args, args...)
	}
}

// And combines multiple predicates with a logical AND.
// It handles zero, one, or multiple predicates appropriately.
// Example usage: And(CondX("age > ?", 30), CondX("status = ?", "active"))
// creates "(age > ? AND status = ?)" with arguments 30 and "active".
func And(preds ...Predicate) Predicate {
	return func(b *sqlBuilder) {
		if len(preds) == 0 {
			return
		}

		if len(preds) == 1 {
			preds[0](b)

			return
		}

		b.writeByte('(')

		for i := range preds {
			if i > 0 {
				b.write(" AND ")
			}

			preds[i](b)
		}

		b.writeByte(')')
	}
}

// Or combines multiple predicates with a logical OR.
// It handles zero, one, or multiple predicates appropriately.
// Example usage: Or(CondX("age < ?", 18), CondX("age > ?", 65))
// creates "(age < ? OR age > ?)" with arguments 18 and 65.
func Or(preds ...Predicate) Predicate {
	return func(b *sqlBuilder) {
		if len(preds) == 0 {
			return
		}

		if len(preds) == 1 {
			preds[0](b)

			return
		}

		b.writeByte('(')

		for i := range preds {
			if i > 0 {
				b.write(" OR ")
			}

			preds[i](b)
		}

		b.writeByte(')')
	}
}

// In creates a predicate for an IN clause with the specified column and values.
// Example usage: In("id", 1, 2, 3)
// creates "id IN (?, ?, ?)" with arguments 1, 2, and 3.
func In(col string, vals ...any) Predicate {
	return func(b *sqlBuilder) {
		if len(vals) == 0 {
			return
		}

		b.write(col)
		b.write(" IN (")

		for i := range vals {
			if i > 0 {
				b.write(", ")
			}

			b.writeByte('?')
			b.arg(vals[i])
		}

		b.writeByte(')')
	}
}

// NotIn creates a predicate for a NOT IN clause with the specified column and values.
// Example usage: NotIn("id", 1, 2, 3)
// creates "id NOT IN (?, ?, ?)" with arguments 1, 2, and 3.
func NotIn(col string, vals ...any) Predicate {
	return func(b *sqlBuilder) {
		if len(vals) == 0 {
			return
		}

		b.write(col)
		b.write(" NOT IN (")

		for i := range vals {
			if i > 0 {
				b.write(", ")
			}

			b.writeByte('?')
			b.arg(vals[i])
		}

		b.writeByte(')')
	}
}

// Not creates a predicate that negates the given predicate with a logical NOT.
// Example usage: Not(CondX("status = ?", "active"))
// creates "NOT (status = ?)" with argument "active".
func Not(pred Predicate) Predicate {
	return func(b *sqlBuilder) {
		b.write("NOT (")
		pred(b)
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
