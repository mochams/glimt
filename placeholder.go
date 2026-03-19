package glimt

import (
	"strconv"
	"strings"
)

// Dialect represents the SQL dialect to use for building queries.
// It determines how placeholders are formatted (e.g., "?" for MySQL, "$1" for Postgres).
type Dialect int

// Constants for supported SQL dialects.
const (
	DialectPostgres  Dialect = iota // $1, $2
	DialectMySQL                    // ?, ?
	DialectSQLite                   // ?, ?  (same as MySQL)
	DialectSQLServer                // @p1,	@p2
	DialectOracle                   // :1, :2
)

// writePlaceholders rewrites '?' placeholders in the SQL string to the appropriate format for the given dialect,
func writePlaceholders(sql string, dialect Dialect) string {
	if !strings.Contains(sql, "?") || dialect == DialectMySQL || dialect == DialectSQLite {
		return sql
	}

	var buf strings.Builder
	buf.Grow(len(sql))
	processPlaceholders(&buf, sql, dialect)
	return buf.String()
}

// processPlaceholders scans the SQL string and rewrites '?' placeholders
// according to the dialect. Skips placeholders inside string literals.
func processPlaceholders(buf *strings.Builder, sql string, dialect Dialect) {
	n := 1
	i := 0
	for i < len(sql) {
		switch sql[i] {
		case '\'':
			i = consumeSingleQuote(buf, sql, i)
		case '"':
			i = consumeDoubleQuote(buf, sql, i)
		case '?':
			writePlaceholder(buf, dialect, n)
			n++
			i++
		default:
			buf.WriteByte(sql[i])
			i++
		}
	}
}

// consumeSingleQuote appends chars from a single-quoted string literal to the buffer, handling escaped single quotes.
func consumeSingleQuote(buf *strings.Builder, sql string, i int) int {
	buf.WriteByte('\'')
	i++
	for i < len(sql) {
		buf.WriteByte(sql[i])
		if sql[i] == '\'' {
			i++
			if i < len(sql) && sql[i] == '\'' {
				buf.WriteByte('\'')
				i++
				continue
			}
			break
		}
		i++
	}
	return i
}

// consumeDoubleQuote appends chars from a double-quoted string literal to the buffer, handling escaped double quotes.
func consumeDoubleQuote(buf *strings.Builder, sql string, i int) int {
	buf.WriteByte('"')
	i++
	for i < len(sql) {
		buf.WriteByte(sql[i])
		if sql[i] == '"' {
			i++
			break
		}
		i++
	}
	return i
}

// writePlaceholder appends the appropriate placeholder for the given dialect and parameter index to the buffer.
func writePlaceholder(buf *strings.Builder, dialect Dialect, n int) {
	var tmp [20]byte
	switch dialect {
	case DialectPostgres:
		buf.WriteByte('$')
		buf.Write(strconv.AppendInt(tmp[:0], int64(n), 10))
	case DialectSQLServer:
		buf.WriteString("@p")
		buf.Write(strconv.AppendInt(tmp[:0], int64(n), 10))
	case DialectOracle:
		buf.WriteByte(':')
		buf.Write(strconv.AppendInt(tmp[:0], int64(n), 10))
	default:
		buf.WriteByte('?')
	}
}
