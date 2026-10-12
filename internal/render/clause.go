package render

import (
	"strings"

	"github.com/mochams/glimt/internal/syntax"
)

// clauseKind is a clause of the top-level statement that composition can
// touch.
type clauseKind uint8

// Clause kinds, in the order their keywords are listed in clauseKeyword.
const (
	clauseWhere clauseKind = iota
	clauseOrder
	clauseLimit
	clauseOffset
	clauseFetch
	clauseLock
	numClauses
)

// clauseKeyword is the keyword that starts each clause. The writer writes
// it, and clauseIntro, made from it, is what Render writes for a clause it
// adds or replaces, so the two can't disagree.
var clauseKeyword = [numClauses]string{
	clauseWhere: "WHERE", clauseOrder: "ORDER BY", clauseLimit: "LIMIT", clauseOffset: "OFFSET",
	clauseFetch: "FETCH", clauseLock: "FOR",
}

// clauseIntro is each clause's keyword with a space on each side.
var clauseIntro = func() (intro [numClauses]string) {
	for k, kw := range clauseKeyword {
		intro[k] = " " + kw + " "
	}

	return intro
}()

// clause is where one composable clause sits in a template, as indexes into
// its slots. A clause the query has is cut three times, at start, body and
// end: " WHERE " lies between start and body, and the clause's own SQL
// between body and end. A clause it lacks is cut once, where it would go,
// and start, body and end are that one slot.
type clause struct {
	start  int  // the slot before the clause, or where it would go; -1 when there is none
	body   int  // the slot where the clause's body begins
	end    int  // the slot after the clause
	absent bool // the query lacks the clause; start is where it would go
	params bool // the clause holds a param, so replacing it is refused
}

// has reports whether the query has the clause, with cuts around it.
func (c *clause) has() bool {
	return c.start >= 0 && !c.absent
}

// noClauses is a template's clauses before the writer cuts any: none can be
// composed.
var noClauses = func() (cs [numClauses]clause) {
	for k := range cs {
		cs[k] = clause{start: -1, body: -1, end: -1}
	}

	return cs
}()

// statementKind says which compositions a template's statement allows.
type statementKind uint8

// Statement kinds.
const (
	fixedStatement  statementKind = iota // INSERT, MERGE or a raw statement: none
	selectStatement                      // a SELECT: WHERE, ORDER BY, LIMIT, OFFSET
	queryStatement                       // another query, such as a UNION: ORDER BY, LIMIT, OFFSET
	dmlStatement                         // UPDATE or DELETE: WHERE
)

// isQuery reports whether the statement is a query, which can take ORDER BY,
// LIMIT and OFFSET and be counted.
func (k statementKind) isQuery() bool {
	return k == selectStatement || k == queryStatement
}

// takesWhere reports whether a WHERE condition can be added to the statement.
func (k statementKind) takesWhere() bool {
	return k == selectStatement || k == dmlStatement
}

// cut ends the current part with a slot at an edge of clause k, and returns
// the slot's index.
func (w *writer) cut(k clauseKind) int {
	w.cuts = append(w.cuts, w.b.Len())
	w.slots = append(w.slots, slot{param: -1, clause: k})

	return len(w.slots) - 1
}

// composable writes clause k with its body e, cut at its start, body and
// end, or cuts once where it would go when e is nil.
func (w *writer) composable(k clauseKind, e *syntax.Expr) {
	c := &w.clauses[k]

	if e == nil {
		c.start = w.cut(k)
		c.body, c.end, c.absent = c.start, c.start, true

		return
	}

	c.start = w.cut(k)
	w.word(clauseKeyword[k])
	w.text(" ")
	c.body = w.cut(k)
	w.span(e.Span)
	c.end = w.cut(k)
}

// where writes the top-level WHERE clause, which composition can extend, or
// cuts where one would go. WHERE CURRENT OF is written without cuts: nothing
// can be ANDed with a cursor position.
func (w *writer) where(e *syntax.Expr) {
	if e != nil && w.isCurrentOf(e) {
		w.clause("WHERE", e)

		return
	}

	w.composable(clauseWhere, e)
}

// isCurrentOf reports whether a WHERE body is CURRENT OF cursor.
func (w *writer) isCurrentOf(e *syntax.Expr) bool {
	if e.To-e.From < 2 {
		return false
	}

	text := func(i int32) string { return w.p.Tokens[i].Text(w.p.Src) }

	return strings.EqualFold(text(e.From), "current") && strings.EqualFold(text(e.From+1), "of")
}

// topTail writes the ORDER BY, LIMIT, OFFSET, FETCH and locking clauses of
// the top-level query, cut for composition, and cuts where each one the
// statement lacks would go. A clause inside parentheses that belongs to the
// statement, in w.inner, was cut where it is, so it isn't added again.
// Postgres wants LIMIT (or FETCH) and OFFSET side by side within one level,
// so a missing one goes right after the one the statement has, at its level,
// and both go right after ORDER BY when it has neither. A missing FETCH or
// locking clause gets no cut: neither is ever added.
func (w *writer) topTail(q *syntax.Query) {
	own := tailExprs(q)
	has := func(k clauseKind) bool { return own[k] != nil || w.inner[k] != nil }

	if own[clauseOrder] != nil || !has(clauseOrder) {
		w.composable(clauseOrder, q.OrderBy)
	}

	limit, offset := has(clauseLimit) || has(clauseFetch), has(clauseOffset)

	if !limit && !offset {
		w.composable(clauseLimit, nil)
		w.composable(clauseOffset, nil)
	}

	for _, c := range tailClauses(q) {
		w.composable(c.kind, c.expr)

		if partner, ok := missingPartner(c.kind, limit, offset); ok {
			w.composable(partner, nil)
		}
	}
}

// missingPartner returns the clause that goes right after tail clause k when
// the query lacks it: LIMIT after an OFFSET when the query has no LIMIT or
// FETCH, and OFFSET after a LIMIT or FETCH when it has no OFFSET.
func missingPartner(k clauseKind, limit, offset bool) (clauseKind, bool) {
	switch {
	case k == clauseOffset && !limit:
		return clauseLimit, true
	case (k == clauseLimit || k == clauseFetch) && !offset:
		return clauseOffset, true
	}

	return 0, false
}
