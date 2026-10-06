package render

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/mochams/glimt/internal/syntax"
)

// Template is a query compiled at load time, ready to render per request.
// It is immutable and safe for concurrent use.
type Template struct {
	sql      string   // the complete SQL, when expands is false
	expands  bool     // some param is written as IN (:name)
	parts    []string // the SQL around the slots: parts[i] comes before slots[i]
	slots    []slot   // params and clause edges, in order
	names    []string // distinct param names, in order of first use
	uses     []use    // how each name is used
	kept     []use    // how each name is used outside the clauses a count drops; nil unless a query
	partsLen int      // total length of parts

	kind    statementKind      // the compositions the statement allows
	clauses [numClauses]clause // where the composable clauses are
}

// use records how a param name is used: as a single value, in IN (:name), or both.
type use uint8

const (
	useScalar use = 1 << iota
	useExpand
)

// maxArgs is how many bind parameters Postgres accepts in one statement.
const maxArgs = 65535

// Compile writes the statement of p as SQL and prepares it for Render.
func Compile(p *syntax.Parsed) (*Template, error) {
	w, err := write(p)
	if err != nil {
		return nil, err
	}

	if len(w.names) > maxArgs {
		return nil, fmt.Errorf("%d params is more than the %d Postgres accepts", len(w.names), maxArgs)
	}

	t := &Template{
		parts: w.parts, slots: w.slots, names: w.names, uses: make([]use, len(w.names)),
		kind: w.kind, clauses: w.clauses,
	}

	for _, part := range t.parts {
		t.partsLen += len(part)
	}

	t.indexSlots()

	if !t.expands {
		t.sql = t.staticSQL()
	}

	return t, nil
}

// indexSlots records how each name is used, in the whole statement and in
// what a count keeps, and which composable clauses hold params.
func (t *Template) indexSlots() {
	for _, s := range t.slots {
		if s.isParam() {
			t.uses[s.param] |= s.use()
			t.expands = t.expands || s.expand
		}
	}

	for k := range t.clauses {
		c := &t.clauses[k]
		if c.has() {
			c.params = slices.ContainsFunc(t.slots[c.start:c.end], slot.isParam)
		}
	}

	if t.kind.isQuery() {
		t.kept = t.keptUses()
	}
}

// keptUses returns how each name is used outside the clauses a count drops.
// A name can be used both in a dropped clause and in one that stays, so this
// goes slot by slot. Counting is the only render that drops a clause holding
// a param: replacing one is refused.
func (t *Template) keptUses() []use {
	kept := make([]use, len(t.names))

	for i, s := range t.slots {
		if s.isParam() && !t.droppedByCount(i) {
			kept[s.param] |= s.use()
		}
	}

	return kept
}

// droppedByCount reports whether slot i lies in a clause a count drops.
func (t *Template) droppedByCount(i int) bool {
	for _, k := range countDrops {
		if c := &t.clauses[k]; c.has() && c.start < i && i < c.end {
			return true
		}
	}

	return false
}

// use returns how the slot uses its name.
func (s slot) use() use {
	if s.expand {
		return useExpand
	}

	return useScalar
}

// Names returns the distinct param names in the order Render takes their values.
func (t *Template) Names() []string {
	return slices.Clone(t.names)
}

// Render returns the SQL and its args for one value per name, in the order of
// Names. When no param expands, the SQL was built at load time and the args
// are values itself, so Render does not allocate; the caller must not change
// values while the args are in use.
func (t *Template) Render(values []any) (string, []any, error) {
	if len(values) != len(t.names) {
		return "", nil, fmt.Errorf("got %d values for %d params", len(values), len(t.names))
	}

	if !t.expands {
		return t.sql, values, nil
	}

	var r Run
	if err := t.Begin(&r, values, Plan{}); err != nil {
		return "", nil, err
	}

	r.Next() // nothing composed, so there is no hole: the walk runs to the end

	return r.Finish()
}

// staticSQL renders a template whose params don't expand, with a value of
// nil for each name. Numbering by first use then gives the name with index
// i the placeholder $i+1, whatever the values, so the SQL serves every
// Render.
func (t *Template) staticSQL() string {
	var r Run

	_ = t.Begin(&r, make([]any, len(t.names)), Plan{}) // with nothing expanding or composed, Begin can't fail
	r.Next()

	return r.o.b.String()
}

// argCount returns how many args values bind at most when each name is used
// as uses says, checking each expanded value. A name uses leaves out is not
// bound, so its value isn't checked.
func (t *Template) argCount(values []any, uses []use) (int, error) {
	total := 0

	for k, u := range uses {
		if u&useScalar != 0 {
			total++
		}

		if u&useExpand != 0 {
			n, err := listLen(values[k])
			if err != nil {
				return 0, fmt.Errorf(":%s %w", t.names[k], err)
			}

			total += n
		}
	}

	return total, checkArgCount(total)
}

// checkArgCount reports more args than Postgres accepts.
func checkArgCount(n int) error {
	if n > maxArgs {
		return fmt.Errorf("%d args is more than the %d Postgres accepts", n, maxArgs)
	}

	return nil
}

// numbering holds, for each use of each name, its first placeholder number
// and how many numbers it takes: 0 until the use is numbered.
type numbering []int

// index returns where the numbers for slot s's use of its name are kept.
func (nums numbering) index(s slot) int {
	i := 4 * int(s.param)
	if s.expand {
		i += 2
	}

	return i
}

// get returns the first number and count for slot s's use, or zero values.
func (nums numbering) get(s slot) (first, n int) {
	i := nums.index(s)

	return nums[i], nums[i+1]
}

// set records the first number and count for slot s's use.
func (nums numbering) set(s slot, first, n int) {
	i := nums.index(s)
	nums[i], nums[i+1] = first, n
}

// output is rendered SQL and its args, as they are written.
type output struct {
	b    strings.Builder
	args []any
	prev byte // the last byte written; 0 before any
}

// write writes SQL text.
func (o *output) write(s string) {
	if s != "" {
		o.b.WriteString(s)
		o.prev = s[len(s)-1]
	}
}

// placeholders writes n placeholders from $first on, separated by ", ". A
// space comes first when the last byte written is a word character, since
// Postgres would read "THEN$1" or "$1$2" as one token.
func (o *output) placeholders(first, n int) {
	if isWordByte(o.prev) {
		o.b.WriteByte(' ')
	}

	var buf [20]byte

	for i := range n {
		if i > 0 {
			o.b.WriteString(", ")
		}

		o.b.WriteByte('$')
		o.b.Write(strconv.AppendInt(buf[:0], int64(first+i), 10))
	}

	o.prev = '0'
}

// param writes the placeholders of a param slot, binding its value on first
// use and reusing its numbers after that.
func (o *output) param(nums numbering, s slot, values []any) {
	first, n := nums.get(s)
	if n == 0 {
		first = len(o.args) + 1

		if s.expand {
			o.args = appendList(o.args, values[s.param])
		} else {
			o.args = append(o.args, values[s.param])
		}

		n = len(o.args) - first + 1
		nums.set(s, first, n)
	}

	o.placeholders(first, n)
}

// isWordByte reports whether c can be part of a Postgres identifier or number.
func isWordByte(c byte) bool {
	return 'a' <= c|0x20 && c|0x20 <= 'z' || '0' <= c && c <= '9' || c == '_' || c == '$' || c >= 0x80
}
