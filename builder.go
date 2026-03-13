package glimt

import (
	"strings"
)

// sqlBuilder is a helper type for building SQL strings and collecting arguments.
type sqlBuilder struct {
	sql  strings.Builder
	args []any
}

// write appends a string to the SQL being built.
func (b *sqlBuilder) write(s string) {
	b.sql.WriteString(s)
}

// writeByte appends a single byte to the SQL being built.
func (b *sqlBuilder) writeByte(c byte) {
	b.sql.WriteByte(c)
}

// grow preallocates space in the SQL builder to optimize for building larger queries.
func (b *sqlBuilder) grow(n int) {
	b.sql.Grow(n)
}

// arg appends an argument to the list of arguments being collected.
func (b *sqlBuilder) arg(a any) {
	b.args = append(b.args, a)
}

// string returns the built SQL string.
func (b *sqlBuilder) string() string {
	return b.sql.String()
}

// reset clears the SQL builder and argument list for reuse.
func (b *sqlBuilder) reset() {
	b.sql.Reset()
	b.args = b.args[:0]
}
