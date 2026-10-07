package glimt

import (
	"fmt"

	"github.com/mochams/glimt/internal/syntax"
)

// checkColumn reports whether col is a column reference glimt can write as
// is: name(.name)*, each part a bare identifier or a "quoted" one. The first
// part, when bare, must not be a word Postgres reserves, such as order or
// null: quote it, or qualify a lone one (t.order). Parts after a dot may be
// any word. Columns come from code, never from a request; see Columns.
func checkColumn(col string) error {
	i, parts := 0, 0
	for i < len(col) {
		if parts > 0 {
			if col[i] != '.' {
				return badColumn(col)
			}

			i++
		}

		end, ok := partEnd(col, i)
		if !ok {
			return badColumn(col)
		}

		if parts == 0 && syntax.ReservedWord(col[i:end]) {
			return reservedColumn(col, col[i:end], end == len(col))
		}

		i = end
		parts++
	}

	if parts == 0 {
		return badColumn(col)
	}

	return nil
}

// badColumn reports a string that is not a column reference.
func badColumn(col string) error {
	return fmt.Errorf("%q is %w", col, ErrBadColumn)
}

// reservedColumn reports a column whose first part is the reserved word,
// unquoted; alone tells whether it is the only part.
func reservedColumn(col, word string, alone bool) error {
	fix := "quote it"
	if alone {
		fix = "quote it or qualify it"
	}

	return fmt.Errorf("%q is %w: %s is a reserved word, so %s", col, ErrBadColumn, word, fix)
}

// partEnd returns the end of the name part starting at col[i]: a quoted
// identifier, or a bare one. ok is false when no valid part starts there.
func partEnd(col string, i int) (end int, ok bool) {
	if i < len(col) && col[i] == '"' {
		return quotedEnd(col, i)
	}

	j := i
	for j < len(col) && isIdentByte(col[j], j == i) {
		j++
	}

	return j, j > i
}

// quotedEnd returns the end of the quoted identifier opening at col[i].
// Doubled quotes escape a quote; an empty name or a NUL is invalid.
func quotedEnd(col string, i int) (end int, ok bool) {
	for j := i + 1; j < len(col); j++ {
		switch col[j] {
		case 0:
			return 0, false
		case '"':
			if j+1 < len(col) && col[j+1] == '"' {
				j++

				continue
			}

			return j + 1, j > i+1
		}
	}

	return 0, false
}

// isIdentByte reports whether c can be part of a bare identifier; first
// tells whether it would be the first byte.
func isIdentByte(c byte, first bool) bool {
	letter := 'a' <= c|0x20 && c|0x20 <= 'z' || c == '_' || c >= 0x80
	if first {
		return letter
	}

	return letter || '0' <= c && c <= '9' || c == '$'
}
