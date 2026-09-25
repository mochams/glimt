package glimt

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

var fuzzSeeds = []string{
	"SELECT * FROM t WHERE a = ? -- comment",
	"SELECT '--' /* x */ FROM t WHERE b = ?",
	"SELECT \"a?\", `b?` FROM t # mysql comment",
	"SELECT $$ it's ? $$, $tag$ -- $tag$, ?",
	"SELECT E'\\'?' , 'it''s?' , ?",
	"SELECT /*+ hint */ /*! mysql */ 1",
	"SELECT * FROM t /* :where */ GROUP BY a",
	"SELECT * FROM t WHERE a = ? /* :and */ ORDER BY b;",
	"SELECT data ?? 'k', data ??| array['a'] WHERE id = ?",
	"SELECT /* a /* nested */ b */ 1--1",
	"-- :name q\nSELECT 1",
	"SELECT 'unterminated",
	"SELECT 1 /* unterminated",
}

// FuzzLex checks that lexing never panics and that sanitized SQL is stable:
// lexing the output again yields the same template.
func FuzzLex(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s, uint8(0))
		f.Add(s, uint8(1))
		f.Add(s, uint8(2))
	}

	f.Fuzz(func(t *testing.T, src string, d uint8) {
		dialect := Dialect(d % 3)

		_, _ = parseSQL(src, dialect, true)
		_, _ = parseFile(src, dialect)

		tpl, err := parseSQL(src, dialect, false)
		if err != nil {
			return
		}

		if len(tpl.parts) != len(tpl.slots)+1 || len(tpl.params) != len(tpl.parts) {
			t.Fatalf("inconsistent template: %d parts, %d params, %d slots", len(tpl.parts), len(tpl.params), len(tpl.slots))
		}

		src2 := sourceOf(tpl)

		again, err := parseSQL(src2, dialect, false)
		if err != nil {
			t.Fatalf("re-lexing %q failed: %v", src2, err)
		}

		if !slices.Equal(again.parts, tpl.parts) || !slices.Equal(again.params, tpl.params) || !slices.Equal(again.slots, tpl.slots) {
			t.Fatalf("not stable:\nfirst  %q %v %v\nsecond %q %v %v",
				tpl.parts, tpl.params, tpl.slots, again.parts, again.params, again.slots)
		}
	})
}

// sourceOf turns a template back into SQL with its markers.
func sourceOf(tpl *template) string {
	var sb strings.Builder

	for i, part := range tpl.parts {
		sb.WriteString(part)

		if i < len(tpl.slots) {
			sb.WriteString(map[slot]string{slotWhere: " /* :where */", slotAnd: " /* :and */"}[tpl.slots[i]])
		}
	}

	return sb.String()
}

// FuzzWritePlaceholders checks that rewriting only replaces placeholders:
// every other byte is copied, and each "??" loses exactly one byte.
func FuzzWritePlaceholders(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s, uint8(0))
		f.Add(s, uint8(1))
	}

	f.Fuzz(func(t *testing.T, sql string, d uint8) {
		dialect := Dialect(d % 3)
		out := writePlaceholders(sql, dialect)

		placeholders, escapes := countPlaceholders(sql, dialect)

		want := len(sql) - escapes
		if dialect == DialectPostgres {
			for n := 1; n <= placeholders; n++ {
				want += len(strconv.Itoa(n)) // "?" becomes "$n"
			}
		}

		if len(out) != want {
			t.Fatalf("%q -> %q: length %d, want %d (%d placeholders, %d escapes)",
				sql, out, len(out), want, placeholders, escapes)
		}
	})
}

// countPlaceholders counts ? placeholders and ?? escapes outside quoted text.
func countPlaceholders(sql string, dialect Dialect) (placeholders, escapes int) {
	for i := 0; i < len(sql); {
		if end, _ := quotedEnd(sql, i, dialect); end > i {
			i = end

			continue
		}

		switch {
		case sql[i] != '?':
			i++
		case i+1 < len(sql) && sql[i+1] == '?':
			escapes++
			i += 2
		default:
			placeholders++
			i++
		}
	}

	return placeholders, escapes
}
