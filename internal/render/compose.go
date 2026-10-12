package render

import "fmt"

// OrderMode is what a composition does with a query's ORDER BY.
type OrderMode uint8

// Order modes.
const (
	OrderKeep    OrderMode = iota // leave ORDER BY as written
	OrderReplace                  // replace it, or add one
	OrderAppend                   // add terms after its own, or add one
)

// Plan is what a request adds to a query: plain data, given to Begin once.
// Render writes every keyword and separator; the caller writes only its own
// condition and order terms, at the holes Next stops at.
type Plan struct {
	Where  bool      // a condition, written at HoleWhere, is ANDed with the WHERE clause
	Order  OrderMode // what to do with ORDER BY; new terms are written at HoleOrder
	Limit  any       // the LIMIT value to bind, or nil to keep the query's LIMIT
	Offset any       // the OFFSET value to bind, or nil to keep the query's OFFSET
	Count  bool      // render SELECT count(*) over the query, without its tail clauses
}

// Composes reports whether p changes the query at all.
func (p *Plan) Composes() bool {
	return p.Where || p.Order != OrderKeep || p.Limit != nil || p.Offset != nil || p.Count
}

// Hole is where the caller writes its own SQL during a Run.
type Hole uint8

// Holes.
const (
	Done      Hole = iota // the render is complete: call Finish
	HoleWhere             // write the condition to AND with the WHERE clause
	HoleOrder             // write the ORDER BY terms, separated by ", "
)

// action is what a render does with one clause.
type action uint8

// Actions.
const (
	keep     action = iota // write the clause as compiled
	drop                   // leave it out
	replace                // write it anew in place of the query's, or add it
	extend                 // AND a condition with the query's WHERE, or add a WHERE
	appendTo               // add ORDER BY terms after the query's, or add an ORDER BY
)

// Run is one render in progress. It lives on the caller's stack: Begin
// starts it, Next writes SQL up to the next hole the caller must fill, and
// Finish returns the result once Next returns Done.
//
//	var r render.Run
//	if err := tmpl.Begin(&r, values, plan); err != nil { … }
//	for h := r.Next(); h != render.Done; h = r.Next() {
//		… write into r.Writer() …
//	}
//	sql, args, err := r.Finish()
type Run struct {
	t      *Template
	values []any
	o      output
	acts   [numClauses]action
	limit  any
	offset any
	i      int    // the next slot to walk
	owed   string // SQL due before the walk goes on, after the caller fills a hole
	count  bool
	done   bool
	nums   [16]int   // the numbering of a typical query
	more   numbering // the numbering, when nums is too small
}

// Begin starts a render of t with one value per name, in the order of
// Names, composed as p says. It reports a plan the statement can't take
// before anything is written.
func (t *Template) Begin(r *Run, values []any, p Plan) error {
	total, err := t.accept(values, &p)
	if err != nil {
		return err
	}

	*r = Run{}
	r.t, r.values, r.count = t, values, p.Count

	if p.Composes() {
		r.acts, r.limit, r.offset = t.actions(&p), p.Limit, p.Offset
	}

	extra := 0 // room for what the plan adds
	if p.Composes() {
		extra = 4
	}

	r.o.b.Grow(t.partsLen + 6*(len(t.slots)+total) + 16*extra)
	r.o.args = make([]any, 0, total+extra)

	if need := 4 * len(t.names); need > len(r.nums) {
		r.more = make(numbering, need)
	}

	if p.Count {
		r.o.write("SELECT count(*) FROM (")
	}

	return nil
}

// accept checks values and p before a render starts, and returns how many
// args the render binds at most. A count only checks the values of the
// params it keeps.
func (t *Template) accept(values []any, p *Plan) (int, error) {
	if len(values) != len(t.names) {
		return 0, fmt.Errorf("got %d values for %d params", len(values), len(t.names))
	}

	uses := t.uses
	if p.Count {
		if !t.kind.isQuery() {
			return 0, composeError("only a query can be counted")
		}

		uses = t.kept
	}

	total, err := t.argCount(values, uses)
	if err != nil {
		return 0, err
	}

	return total, t.check(p)
}

// Writer returns the writer for filling the hole Next stopped at.
func (r *Run) Writer() *Writer {
	return (*Writer)(r)
}

// Next writes the template up to the next hole the caller must fill, and
// returns it, or writes the rest and returns Done.
func (r *Run) Next() Hole {
	if r.done {
		return Done
	}

	t, nums := r.t, r.numbering()

	r.o.write(r.owed)
	r.owed = ""

	o := &r.o
	for i := r.i; i < len(t.slots); i++ {
		o.write(t.parts[i])

		switch s := t.slots[i]; {
		case s.isParam():
			o.param(nums, s, r.values)
		case r.acts[s.clause] == keep:
			// the clause is written as compiled
		default:
			r.i = i + 1

			h, ok := r.edge(i, s.clause)
			if ok {
				return h
			}

			i = r.i - 1 // edge may have jumped past a clause
		}
	}

	r.o.write(t.parts[len(t.parts)-1])

	if r.count {
		r.o.write(") AS t")
	}

	r.done = true

	return Done
}

// Finish returns the rendered SQL and its args.
func (r *Run) Finish() (string, []any, error) {
	return r.o.b.String(), r.o.args, checkArgCount(len(r.o.args))
}

// numbering returns the run's placeholder numbering.
func (r *Run) numbering() numbering {
	if r.more != nil {
		return r.more
	}

	return r.nums[:]
}

// edge acts on slot i, an edge of clause k. It returns the hole the caller
// fills here, if any.
func (r *Run) edge(i int, k clauseKind) (Hole, bool) {
	c := &r.t.clauses[k]

	switch r.acts[k] {
	case drop:
		if i == c.start {
			r.skip(c)
		}
	case replace:
		if i == c.start {
			r.o.write(clauseIntro[k])
			r.skip(c)

			return r.fill(k)
		}
	case extend:
		return r.extend(i, c)
	case appendTo:
		return r.appendTo(i, c)
	}

	return Done, false
}

// skip jumps over a clause the query has, so neither its SQL nor its params
// are written.
func (r *Run) skip(c *clause) {
	if !c.absent {
		r.i = c.end + 1
	}
}

// fill writes the body of replaced clause k: the bound value of LIMIT and
// OFFSET, or the caller's ORDER BY terms.
func (r *Run) fill(k clauseKind) (Hole, bool) {
	switch k {
	case clauseLimit:
		r.bind(r.limit)
	case clauseOffset:
		r.bind(r.offset)
	case clauseOrder:
		return HoleOrder, true
	}

	return Done, false
}

// bind writes a placeholder bound to v.
func (r *Run) bind(v any) {
	r.o.args = append(r.o.args, v)
	r.o.placeholders(len(r.o.args), 1)
}

// extend ANDs the caller's condition with the WHERE clause at slot i:
// (body) AND (condition), or a new WHERE condition.
func (r *Run) extend(i int, c *clause) (Hole, bool) {
	switch {
	case c.absent:
		r.o.write(clauseIntro[clauseWhere])

		return HoleWhere, true
	case i == c.body:
		r.o.write("(")
	case i == c.end:
		r.o.write(") AND (")
		r.owed = ")"

		return HoleWhere, true
	}

	return Done, false
}

// appendTo adds the caller's terms to ORDER BY at slot i, after the query's
// own, or as a new ORDER BY.
func (r *Run) appendTo(i int, c *clause) (Hole, bool) {
	switch {
	case c.absent:
		r.o.write(clauseIntro[clauseOrder])

		return HoleOrder, true
	case i == c.end:
		r.o.write(", ")

		return HoleOrder, true
	}

	return Done, false
}

// countDrops are the clauses a count leaves out.
var countDrops = [...]clauseKind{clauseOrder, clauseLimit, clauseOffset, clauseFetch, clauseLock}

// actions returns what a render following p does with each clause.
func (t *Template) actions(p *Plan) (acts [numClauses]action) {
	if p.Where {
		acts[clauseWhere] = extend
	}

	if p.Count {
		for _, k := range countDrops {
			acts[k] = drop
		}

		return acts
	}

	switch p.Order {
	case OrderReplace:
		acts[clauseOrder] = replace
	case OrderAppend:
		acts[clauseOrder] = appendTo
	}

	if p.Limit != nil {
		acts[clauseLimit] = replace
	}

	if p.Offset != nil {
		acts[clauseOffset] = replace
	}

	return acts
}

// Writer writes the caller's SQL into a hole of a Run: SQL text, and values
// it binds as args, numbered after the placeholders before them. It is the
// Run itself, seen only through these methods.
type Writer Run

// SQL writes SQL text as is. It must never hold request input.
func (w *Writer) SQL(s string) {
	w.o.write(s)
}

// Value writes a placeholder and binds v to it.
func (w *Writer) Value(v any) {
	w.o.args = append(w.o.args, v)
	w.o.placeholders(len(w.o.args), 1)
}

// List writes one placeholder per element of the list v, as IN (:name)
// does, and binds the elements. Its error, for a nil value, an empty list or
// a list of lists, has no subject: the caller names the list.
func (w *Writer) List(v any) error {
	if _, err := listLen(v); err != nil {
		return err
	}

	first := len(w.o.args) + 1
	w.o.args = appendList(w.o.args, v)
	w.o.placeholders(first, len(w.o.args)-first+1)

	return nil
}

// check reports a plan the statement can't take.
func (t *Template) check(p *Plan) error {
	if err := t.checkWhere(p); err != nil {
		return err
	}

	switch tail := p.Order != OrderKeep || p.Limit != nil || p.Offset != nil; {
	case tail && !t.kind.isQuery():
		return composeError("ORDER BY, LIMIT and OFFSET can only be added to a query")
	case p.Limit != nil && t.clauses[clauseFetch].has():
		return composeError("LIMIT can't be added to a query that uses FETCH")
	}

	if p.Count {
		return nil // the tail clauses are dropped, not replaced
	}

	return t.checkReplaced(p)
}

// checkWhere reports a WHERE condition the statement can't take.
func (t *Template) checkWhere(p *Plan) error {
	switch {
	case !p.Where:
		return nil
	case !t.kind.takesWhere():
		return composeError("a WHERE condition can only be added to a SELECT, UPDATE or DELETE")
	case t.clauses[clauseWhere].start < 0:
		return composeError("a WHERE condition can't be added to WHERE CURRENT OF")
	}

	return nil
}

// checkReplaced reports replacing a clause that holds a param: it's unclear
// whether the param or the composed value was meant.
func (t *Template) checkReplaced(p *Plan) error {
	held := func(k clauseKind) bool { return t.clauses[k].has() && t.clauses[k].params }

	switch {
	case p.Order == OrderReplace && held(clauseOrder):
		return composeError("can't replace an ORDER BY that holds a param")
	case p.Limit != nil && held(clauseLimit):
		return composeError("can't replace a LIMIT that holds a param")
	case p.Offset != nil && held(clauseOffset):
		return composeError("can't replace an OFFSET that holds a param")
	}

	return nil
}
