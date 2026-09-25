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
	DialectPostgres Dialect = iota // $1, $2
	DialectMySQL                   // ?, ?
	DialectSQLite                  // ?, ?  (same as MySQL)
)

// writePlaceholders rewrites '?' placeholders in the SQL string to the appropriate format for the given dialect.
// Only Postgres needs rewriting; MySQL and SQLite use '?' natively.
// In every dialect, "??" outside quoted text becomes a literal '?' (for example the Postgres JSONB '?' operator).
func writePlaceholders(sql string, dialect Dialect) string {
	marker := "??"
	if dialect == DialectPostgres {
		marker = "?"
	}

	if !strings.Contains(sql, marker) {
		return sql
	}

	var buf strings.Builder
	buf.Grow(len(sql) + 8)
	processPlaceholders(&buf, sql, dialect)

	return buf.String()
}

// processPlaceholders scans the SQL string and rewrites '?' placeholders,
// copying string literals and quoted identifiers unchanged.
func processPlaceholders(buf *strings.Builder, sql string, dialect Dialect) {
	var tmp [20]byte

	n := 1
	for i := 0; i < len(sql); {
		// Copy plain text up to the next byte that may start a quote or placeholder.
		next := strings.IndexAny(sql[i:], "?'\"`$")
		if next < 0 {
			buf.WriteString(sql[i:])

			return
		}

		buf.WriteString(sql[i : i+next])
		i += next

		if end, _ := quotedEnd(sql, i, dialect); end > i {
			buf.WriteString(sql[i:end])
			i = end

			continue
		}

		switch {
		case sql[i] != '?':
			buf.WriteByte(sql[i])
			i++
		case i+1 < len(sql) && sql[i+1] == '?':
			buf.WriteByte('?')
			i += 2
		case dialect == DialectPostgres:
			buf.WriteByte('$')
			buf.Write(strconv.AppendInt(tmp[:0], int64(n), 10))
			n++
			i++
		default:
			buf.WriteByte('?')
			i++
		}
	}
}
