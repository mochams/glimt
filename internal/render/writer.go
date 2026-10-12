package render

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/mochams/glimt/internal/syntax"
)

// slot is one cut in a template: a param occurrence, or an edge of a
// composable clause.
type slot struct {
	param  int32      // the param's index into the template's names, or -1 at a clause edge
	expand bool       // a param written as IN (:name)
	clause clauseKind // at a clause edge: which clause
}

// isParam reports whether the slot is a param rather than a clause edge.
func (s slot) isParam() bool {
	return s.param >= 0
}

// writer writes an AST as SQL into one buffer, noting where each param and
// clause edge cuts it.
type writer struct {
	p       *syntax.Parsed
	b       strings.Builder
	last    byte  // the last byte written; 0 before any
	cuts    []int // the offset in b of each slot
	slots   []slot
	names   []string                 // distinct param names, in order of first use
	index   map[string]int           // names by index, once there are too many to search
	kind    statementKind            // what the statement allows composition to add
	clauses [numClauses]clause       // where the top-level statement's composable clauses are
	inner   [numClauses]*syntax.Expr // tail clauses inside parentheses that belong to the top-level statement
	err     error                    // set when a node can't be written
}

// written is the SQL of a statement, cut at its params and clause edges.
type written struct {
	parts   []string // one more than slots: parts[i] comes before slots[i]
	slots   []slot
	names   []string
	kind    statementKind
	clauses [numClauses]clause
}

// maxSearchedNames is how many names nameIndex searches before it keeps a map.
const maxSearchedNames = 32

// write writes the statement of p, cut at its params and clause edges.
func write(p *syntax.Parsed) (*written, error) {
	n := len(p.Params) + 12 // the params, and a typical query's clause edges
	w := &writer{
		p: p, cuts: make([]int, 0, n), slots: make([]slot, 0, n), names: make([]string, 0, len(p.Params)),
		clauses: noClauses,
	}
	w.b.Grow(len(p.Src) + 16) // the SQL comes out about as long as the source
	w.statement(p.Stmt)

	if w.err != nil {
		return nil, w.err
	}

	return &written{parts: w.split(), slots: w.slots, names: w.names, kind: w.kind, clauses: w.clauses}, nil
}

// split cuts the written SQL at each param. The parts share one string.
func (w *writer) split() []string {
	sql := w.b.String()
	parts := make([]string, len(w.cuts)+1)

	prev := 0
	for i, cut := range w.cuts {
		parts[i] = sql[prev:cut]
		prev = cut
	}

	parts[len(w.cuts)] = sql[prev:]

	return parts
}

// statement writes a statement.
func (w *writer) statement(s syntax.Statement) {
	switch s := s.(type) {
	case *syntax.Query:
		w.query(s, true)
	case *syntax.Insert:
		w.insert(s)
	case *syntax.Update:
		w.update(s)
	case *syntax.Delete:
		w.delete(s)
	case *syntax.Merge:
		w.merge(s)
	case *syntax.Raw:
		w.span(s.Body.Span)
	default:
		w.fail(s)
	}
}

// query writes a query with its WITH clause and trailing clauses. The
// top-level query, top, is cut for composition; queries inside it aren't.
func (w *writer) query(q *syntax.Query, top bool) {
	w.with(q.With)

	if !top {
		w.setExpr(q.Body)
		w.tailClause(clauseOrder, q.OrderBy)
		w.tail(q)

		return
	}

	if s, ok := q.Body.(*syntax.Select); ok {
		w.kind = selectStatement
		w.selectCore(s, true)
	} else {
		w.kind = queryStatement
		w.inner = innerTail(q)
		w.setExpr(q.Body)
	}

	w.topTail(q)
}

// innerTail returns the tail clauses of the queries in parentheses that make
// up the body of the top-level query q, as in (SELECT … LIMIT 1) OFFSET 2.
// Postgres has no node for those parentheses: their tail clauses are the
// statement's own, so composition must find them there. A clause q has
// itself is left out; Postgres rejects a query that has it twice.
func innerTail(q *syntax.Query) (inner [numClauses]*syntax.Expr) {
	own := tailExprs(q)

	for p, ok := q.Body.(*syntax.ParenQuery); ok; p, ok = p.Query.Body.(*syntax.ParenQuery) {
		for k, e := range tailExprs(p.Query) {
			if e != nil && own[k] == nil && inner[k] == nil {
				inner[k] = e
			}
		}
	}

	return inner
}

// tailExprs returns q's ORDER BY, LIMIT, OFFSET, FETCH and locking clauses
// by clause kind; the rest are nil.
func tailExprs(q *syntax.Query) (tail [numClauses]*syntax.Expr) {
	tail[clauseOrder], tail[clauseLimit], tail[clauseOffset] = q.OrderBy, q.Limit, q.Offset
	tail[clauseFetch], tail[clauseLock] = q.Fetch, q.Locking

	return tail
}

// innerPartner cuts where the missing partner of tail clause k goes, when k
// is inside parentheses and belongs to the top-level statement. Postgres
// rejects LIMIT and OFFSET split across levels, as in (… LIMIT 1) OFFSET 2,
// so the partner goes inside, right after k.
func (w *writer) innerPartner(k clauseKind) {
	limit := w.inner[clauseLimit] != nil || w.inner[clauseFetch] != nil
	if partner, ok := missingPartner(k, limit, w.inner[clauseOffset] != nil); ok {
		w.composable(partner, nil)
	}
}

// tailClause writes tail clause k of a query inside the top-level one: cut
// for composition when it belongs to the top-level statement, as is otherwise.
func (w *writer) tailClause(k clauseKind, e *syntax.Expr) {
	if e != nil && e == w.inner[k] {
		w.composable(k, e)

		return
	}

	w.clause(clauseKeyword[k], e)
}

// tailClause is a LIMIT, OFFSET, FETCH or locking clause of a query.
type tailClause struct {
	kind clauseKind
	expr *syntax.Expr
}

// tail writes the LIMIT, OFFSET, FETCH and locking clauses of q in the order
// the source had them. Postgres takes LIMIT (or FETCH) and OFFSET in either
// order but side by side, with the locking clause before or after the pair.
// Keeping the source's order keeps them valid, and means each clause is
// followed by what followed it there, so it reads back the same: a label
// after AS, for one, depends on the word after it.
func (w *writer) tail(q *syntax.Query) {
	for _, c := range tailClauses(q) {
		w.tailClause(c.kind, c.expr)

		if c.expr == w.inner[c.kind] {
			w.innerPartner(c.kind)
		}
	}
}

// tailClauses returns the LIMIT, OFFSET, FETCH and locking clauses q has, in
// source order.
func tailClauses(q *syntax.Query) []tailClause {
	clauses := []tailClause{
		{clauseLimit, q.Limit},
		{clauseOffset, q.Offset},
		{clauseFetch, q.Fetch},
		{clauseLock, q.Locking},
	}

	present := clauses[:0]
	for _, c := range clauses {
		if c.expr != nil {
			present = append(present, c)
		}
	}

	slices.SortStableFunc(present, func(a, b tailClause) int { return cmp.Compare(a.expr.From, b.expr.From) })

	return present
}

// setExpr writes a query body.
func (w *writer) setExpr(e syntax.SetExpr) {
	switch e := e.(type) {
	case *syntax.Select:
		w.selectCore(e, false)
	case *syntax.SetOp:
		w.setOp(e)
	case *syntax.Values:
		w.values(e)
	case *syntax.TableQuery:
		w.word("TABLE")
		w.only(e.Only)
		w.span(e.Name.Span)
	case *syntax.ParenQuery:
		w.open()
		w.query(e.Query, false)
		w.close()
	default:
		w.fail(e)
	}
}

// selectCore writes a SELECT without its trailing clauses. The top-level
// one's WHERE is cut for composition.
func (w *writer) selectCore(s *syntax.Select, top bool) {
	w.word("SELECT")

	switch {
	case s.DistinctOn != nil:
		w.word("DISTINCT ON")
		w.open()
		w.span(s.DistinctOn.Span)
		w.close()
	case s.Distinct:
		w.word("DISTINCT")
	case s.All:
		w.word("ALL")
	}

	w.span(s.Columns.Span)
	w.clause("FROM", s.From)

	if top {
		w.where(s.Where)
	} else {
		w.clause("WHERE", s.Where)
	}

	w.clause("GROUP BY", s.GroupBy)
	w.clause("HAVING", s.Having)
	w.clause("WINDOW", s.Window)
}

// precedence returns how tightly e binds as a set operand: INTERSECT binds
// tighter than UNION and EXCEPT, and anything but a set operation tighter still.
func precedence(e syntax.SetExpr) int {
	op, ok := e.(*syntax.SetOp)

	switch {
	case !ok:
		return 3
	case op.Op == syntax.Intersect:
		return 2
	}

	return 1
}

// setOp writes a set operation. An operand is parenthesized when it would
// otherwise bind differently: set operations are left-associative.
func (w *writer) setOp(s *syntax.SetOp) {
	prec := precedence(s)

	w.operand(s.Left, precedence(s.Left) < prec)
	w.word(s.Op.String())

	if s.All {
		w.word("ALL")
	}

	w.operand(s.Right, precedence(s.Right) <= prec)
}

// operand writes a set operand, in parentheses when parens is set.
func (w *writer) operand(e syntax.SetExpr, parens bool) {
	if !parens {
		w.setExpr(e)

		return
	}

	w.open()
	w.setExpr(e)
	w.close()
}

// values writes a VALUES list.
func (w *writer) values(v *syntax.Values) {
	w.word("VALUES")

	for i, row := range v.Rows {
		if i > 0 {
			w.comma()
		}

		w.row(row)
	}
}

// row writes one parenthesized row of values.
func (w *writer) row(row []syntax.Expr) {
	w.open()

	for i := range row {
		if i > 0 {
			w.comma()
		}

		w.span(row[i].Span)
	}

	w.close()
}

// with writes a WITH clause, if present.
func (w *writer) with(with *syntax.With) {
	if with == nil {
		return
	}

	w.word("WITH")

	if with.Recursive {
		w.word("RECURSIVE")
	}

	for i, c := range with.CTEs {
		if i > 0 {
			w.comma()
		}

		w.cte(c)
	}
}

// cte writes one common table expression.
func (w *writer) cte(c *syntax.CTE) {
	w.ident(c.Name)

	if c.Columns != nil {
		w.identList(c.Columns)
	}

	w.word("AS")

	switch c.Materialized {
	case syntax.Materialized:
		w.word("MATERIALIZED")
	case syntax.NotMaterialized:
		w.word("NOT MATERIALIZED")
	}

	w.open()
	w.query(c.Body, false)
	w.close()
	w.clause("SEARCH", c.Search)
	w.clause("CYCLE", c.Cycle)
}

// insert writes an INSERT statement.
func (w *writer) insert(s *syntax.Insert) {
	w.with(s.With)
	w.word("INSERT INTO")
	w.table(s.Table, s.Alias)

	if s.Columns != nil {
		w.targetList(s.Columns)
	}

	w.overriding(s.Overriding)

	if s.DefaultValues {
		w.word("DEFAULT VALUES")
	} else {
		w.query(s.Source, false)
	}

	w.onConflict(s.OnConflict)
	w.clause("RETURNING", s.Returning)
}

// onConflict writes an ON CONFLICT clause, if present.
func (w *writer) onConflict(oc *syntax.OnConflict) {
	if oc == nil {
		return
	}

	w.word("ON CONFLICT")
	w.clause("", oc.Target)

	if oc.DoNothing {
		w.word("DO NOTHING")

		return
	}

	w.word("DO UPDATE SET")
	w.assignments(oc.Set)
	w.clause("WHERE", oc.Where)
}

// update writes an UPDATE statement.
func (w *writer) update(s *syntax.Update) {
	w.with(s.With)
	w.word("UPDATE")
	w.only(s.Only)
	w.table(s.Table, s.Alias)
	w.word("SET")
	w.assignments(s.Set)
	w.clause("FROM", s.From)
	w.kind = dmlStatement
	w.where(s.Where)
	w.clause("RETURNING", s.Returning)
}

// delete writes a DELETE statement.
func (w *writer) delete(s *syntax.Delete) {
	w.with(s.With)
	w.word("DELETE FROM")
	w.only(s.Only)
	w.table(s.Table, s.Alias)
	w.clause("USING", s.Using)
	w.kind = dmlStatement
	w.where(s.Where)
	w.clause("RETURNING", s.Returning)
}

// merge writes a MERGE statement.
func (w *writer) merge(s *syntax.Merge) {
	w.with(s.With)
	w.word("MERGE INTO")
	w.only(s.Only)
	w.table(s.Table, s.Alias)
	w.clause("USING", s.Using)
	w.clause("ON", s.On)

	for _, when := range s.When {
		w.mergeWhen(when)
	}

	w.clause("RETURNING", s.Returning)
}

// mergeWhen writes one WHEN clause of a MERGE.
func (w *writer) mergeWhen(m *syntax.MergeWhen) {
	w.word("WHEN")

	switch m.Match {
	case syntax.Matched:
		w.word("MATCHED")
	case syntax.NotMatched:
		w.word("NOT MATCHED")
	case syntax.NotMatchedBySource:
		w.word("NOT MATCHED BY SOURCE")
	default:
		w.fail(m)
	}

	w.clause("AND", m.Cond)
	w.word("THEN")
	w.mergeAction(m)
}

// mergeAction writes what a MERGE WHEN clause does.
func (w *writer) mergeAction(m *syntax.MergeWhen) {
	switch m.Action {
	case syntax.MergeDoNothing:
		w.word("DO NOTHING")
	case syntax.MergeDelete:
		w.word("DELETE")
	case syntax.MergeUpdate:
		w.word("UPDATE SET")
		w.assignments(m.Set)
	case syntax.MergeInsert:
		w.mergeInsert(m)
	default:
		w.fail(m)
	}
}

// mergeInsert writes the INSERT action of a MERGE WHEN clause.
func (w *writer) mergeInsert(m *syntax.MergeWhen) {
	w.word("INSERT")

	if m.Columns != nil {
		w.targetList(m.Columns)
	}

	w.overriding(m.Overriding)

	if m.DefaultValues {
		w.word("DEFAULT VALUES")

		return
	}

	w.word("VALUES")
	w.row(m.Values)
}

// overriding writes an OVERRIDING option, if present.
func (w *writer) overriding(o syntax.Overriding) {
	switch o {
	case syntax.OverridingSystem:
		w.word("OVERRIDING SYSTEM VALUE")
	case syntax.OverridingUser:
		w.word("OVERRIDING USER VALUE")
	}
}

// only writes ONLY before a target table when set.
func (w *writer) only(set bool) {
	if set {
		w.word("ONLY")
	}
}

// table writes a statement's target table and alias.
func (w *writer) table(name syntax.ObjectName, alias *syntax.Ident) {
	w.span(name.Span)

	if alias != nil {
		w.word("AS")
		w.ident(*alias)
	}
}

// assignments writes a SET list.
func (w *writer) assignments(list []syntax.Assignment) {
	for i := range list {
		if i > 0 {
			w.comma()
		}

		a := &list[i]
		if a.Tuple {
			w.targetList(a.Targets)
		} else {
			w.span(a.Targets[0].Span)
		}

		w.word("=")
		w.span(a.Value.Span)
	}
}

// targetList writes a parenthesized list of targets.
func (w *writer) targetList(list []syntax.Target) {
	w.open()

	for i := range list {
		if i > 0 {
			w.comma()
		}

		w.span(list[i].Span)
	}

	w.close()
}

// identList writes a parenthesized list of identifiers.
func (w *writer) identList(list []syntax.Ident) {
	w.open()

	for i, id := range list {
		if i > 0 {
			w.comma()
		}

		w.ident(id)
	}

	w.close()
}

// ident writes an identifier as written in the source, quotes included.
func (w *writer) ident(id syntax.Ident) {
	w.word(w.p.Tokens[id.Tok].Text(w.p.Src))
}

// clause writes keyword then e, when e is present. An empty keyword writes e alone.
func (w *writer) clause(keyword string, e *syntax.Expr) {
	if e == nil {
		return
	}

	if keyword != "" {
		w.word(keyword)
	}

	w.span(e.Span)
}

// span copies the tokens of s from the source, with a space wherever the
// source separated them. Params become slots.
func (w *writer) span(s syntax.Span) {
	for i := s.From; i < s.To; i++ {
		t := w.p.Tokens[i]

		switch {
		case i == s.From:
			w.space()
		case t.Sep == syntax.SepSpace:
			w.b.WriteByte(' ')
		}

		if t.Kind == syntax.PARAM {
			w.param(i)

			continue
		}

		w.text(t.Text(w.p.Src))
	}
}

// param ends the current part at the PARAM token tok and records its slot.
func (w *writer) param(tok int32) {
	i, found := slices.BinarySearchFunc(w.p.Params, tok, func(p *syntax.Param, tok int32) int {
		return cmp.Compare(p.Tok, tok)
	})
	if !found {
		w.err = fmt.Errorf("PARAM token %d is not a recorded param", tok)

		return
	}

	prm := w.p.Params[i]

	w.cuts = append(w.cuts, w.b.Len())
	w.slots = append(w.slots, slot{param: w.nameIndex(prm.Name), expand: prm.Mode == syntax.Expand})
	w.last = 'p' // a placeholder ends like a word
}

// nameIndex returns the index of name in w.names, adding it on first use.
// Few names are searched; past maxSearchedNames, a map takes over. Compile
// bounds the names at maxArgs, so an index fits in an int32.
func (w *writer) nameIndex(name string) int32 {
	if w.index != nil {
		if i, ok := w.index[name]; ok {
			return int32(i) //nolint:gosec // bounded by maxArgs
		}
	} else if i := slices.Index(w.names, name); i >= 0 {
		return int32(i) //nolint:gosec // bounded by maxArgs
	}

	w.names = append(w.names, name)
	i := len(w.names) - 1

	switch {
	case w.index != nil:
		w.index[name] = i
	case len(w.names) > maxSearchedNames:
		w.index = make(map[string]int, 2*len(w.names))
		for j, n := range w.names {
			w.index[n] = j
		}
	}

	return int32(i) //nolint:gosec // bounded by maxArgs
}

// word writes s, after a space unless it starts the text or follows "(".
func (w *writer) word(s string) {
	w.space()
	w.text(s)
}

// space writes the space due before a word or span.
func (w *writer) space() {
	if w.last != 0 && w.last != '(' && w.last != ' ' {
		w.b.WriteByte(' ')
	}
}

// open writes "(" as a word.
func (w *writer) open() {
	w.word("(")
}

// close writes ")" right after what precedes it.
func (w *writer) close() {
	w.text(")")
}

// comma writes "," right after what precedes it.
func (w *writer) comma() {
	w.text(",")
}

// text writes s as is.
func (w *writer) text(s string) {
	w.b.WriteString(s)
	w.last = s[len(s)-1]
}

// fail records that node n can't be written. Only the first failure is kept.
func (w *writer) fail(n any) {
	if w.err == nil {
		w.err = fmt.Errorf("cannot render %T", n)
	}
}
