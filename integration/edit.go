package integration

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/mochams/glimt/internal/syntax"
)

// edit replaces src[from:to] with text. An edit with from == to inserts text.
type edit struct {
	from, to int
	text     string
}

// applyEdits returns src with the edits applied. Edits must not overlap; at
// the same position, an insertion goes before a replacement.
func applyEdits(src string, edits []edit) string {
	slices.SortStableFunc(edits, func(a, b edit) int {
		if c := cmp.Compare(a.from, b.from); c != 0 {
			return c
		}

		return cmp.Compare(a.to-a.from, b.to-b.from)
	})

	var b strings.Builder

	prev := 0
	for _, e := range edits {
		b.WriteString(src[prev:e.from])
		b.WriteString(e.text)
		prev = e.to
	}

	b.WriteString(src[prev:])

	return b.String()
}

// paramEdits replaces each :name param of p with its Postgres placeholder,
// numbered as glimt's renderer numbers them when every value is a single
// value: by first use of each name, separately for IN (:name) and other
// uses. A placeholder gets a space when it would touch the word before it.
func paramEdits(p *syntax.Parsed) []edit {
	type use struct {
		name   string
		expand bool
	}

	numbers := map[use]int{}
	edits := make([]edit, 0, len(p.Params))

	for _, prm := range p.Params {
		u := use{prm.Name, prm.Mode == syntax.Expand}

		n, ok := numbers[u]
		if !ok {
			n = len(numbers) + 1
			numbers[u] = n
		}

		t := p.Tokens[prm.Tok]
		text := "$" + strconv.Itoa(n)

		if t.Pos > 0 && isWordByte(p.Src[t.Pos-1]) {
			text = " " + text
		}

		edits = append(edits, edit{from: int(t.Pos), to: int(t.End), text: text})
	}

	return edits
}

// parenEdits wraps each span in parentheses.
func parenEdits(p *syntax.Parsed, spans []syntax.Span) []edit {
	edits := make([]edit, 0, 2*len(spans))
	for _, s := range spans {
		from, to := int(p.Tokens[s.From].Pos), int(p.Tokens[s.To-1].End)
		edits = append(edits, edit{from: from, to: from, text: "("}, edit{from: to, to: to, text: ")"})
	}

	return edits
}

// isWordByte reports whether c can be part of a Postgres identifier or number.
func isWordByte(c byte) bool {
	return 'a' <= c|0x20 && c|0x20 <= 'z' || '0' <= c && c <= '9' || c == '_' || c == '$' || c >= 0x80
}
