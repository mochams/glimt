package integration

import (
	"strings"

	"github.com/mochams/glimt/internal/syntax"
)

// clauseSpans returns the single-expression clause bodies of p's statement,
// subqueries included, that can be wrapped in parentheses without changing
// what Postgres parses: WHERE, HAVING, LIMIT, OFFSET, a MERGE's ON and WHEN
// … AND conditions, and an ON CONFLICT DO UPDATE's WHERE.
func clauseSpans(p *syntax.Parsed) []syntax.Span {
	c := &collector{p: p}
	c.statement(p.Stmt)

	return c.spans
}

// collector walks an AST, collecting clause spans.
type collector struct {
	p     *syntax.Parsed
	spans []syntax.Span
}

// statement collects from a statement.
func (c *collector) statement(s syntax.Statement) {
	switch s := s.(type) {
	case *syntax.Query:
		c.query(s)
	case *syntax.Insert:
		c.with(s.With)
		c.exprs(s.Returning)

		if s.Source != nil {
			c.query(s.Source)
		}

		if s.OnConflict != nil {
			c.exprs(s.OnConflict.Target)
			c.assignments(s.OnConflict.Set)
			c.clause(s.OnConflict.Where)
		}
	case *syntax.Update:
		c.with(s.With)
		c.assignments(s.Set)
		c.exprs(s.From, s.Returning)
		c.clause(s.Where)
	case *syntax.Delete:
		c.with(s.With)
		c.exprs(s.Using, s.Returning)
		c.clause(s.Where)
	case *syntax.Merge:
		c.merge(s)
	}
}

// merge collects from a MERGE.
func (c *collector) merge(m *syntax.Merge) {
	c.with(m.With)
	c.exprs(m.Using, m.Returning)
	c.clause(m.On)

	for _, w := range m.When {
		c.clause(w.Cond)
		c.assignments(w.Set)

		for i := range w.Values {
			c.exprs(&w.Values[i])
		}
	}
}

// query collects from a query and its trailing clauses.
func (c *collector) query(q *syntax.Query) {
	c.with(q.With)
	c.setExpr(q.Body)
	c.exprs(q.OrderBy, q.Fetch, q.Locking)

	if q.Limit != nil && !c.isOnly(q.Limit, "all") {
		c.clause(q.Limit)
	} else {
		c.exprs(q.Limit)
	}

	if q.Offset != nil && !c.endsWithRows(q.Offset) {
		c.clause(q.Offset)
	} else {
		c.exprs(q.Offset)
	}
}

// setExpr collects from a query body.
func (c *collector) setExpr(e syntax.SetExpr) {
	switch e := e.(type) {
	case *syntax.Select:
		c.exprs(e.DistinctOn, &e.Columns, e.From, e.GroupBy, e.Window)
		c.clause(e.Where)
		c.clause(e.Having)
	case *syntax.SetOp:
		c.setExpr(e.Left)
		c.setExpr(e.Right)
	case *syntax.ParenQuery:
		c.query(e.Query)
	case *syntax.Values:
		for _, row := range e.Rows {
			for i := range row {
				c.exprs(&row[i])
			}
		}
	}
}

// with collects from the bodies of a WITH clause.
func (c *collector) with(w *syntax.With) {
	if w == nil {
		return
	}

	for _, cte := range w.CTEs {
		c.query(cte.Body)
		c.exprs(cte.Search, cte.Cycle)
	}
}

// assignments collects from the values of a SET list.
func (c *collector) assignments(list []syntax.Assignment) {
	for i := range list {
		c.exprs(&list[i].Value)
	}
}

// clause records e as a clause span, unless it is a WHERE CURRENT OF, and
// collects from its subqueries.
func (c *collector) clause(e *syntax.Expr) {
	if e == nil {
		return
	}

	if !c.startsWith(e, "current", "of") {
		c.spans = append(c.spans, e.Span)
	}

	c.exprs(e)
}

// exprs collects from the subqueries of each expression.
func (c *collector) exprs(list ...*syntax.Expr) {
	for _, e := range list {
		if e == nil {
			continue
		}

		for _, sub := range e.Subqueries {
			c.query(sub.Query)
		}
	}
}

// isOnly reports whether e is the single word w, ignoring case.
func (c *collector) isOnly(e *syntax.Expr, w string) bool {
	return e.To-e.From == 1 && strings.EqualFold(c.text(e.From), w)
}

// endsWithRows reports whether e ends in ROW or ROWS, as OFFSET n ROWS does.
func (c *collector) endsWithRows(e *syntax.Expr) bool {
	last := c.text(e.To - 1)

	return strings.EqualFold(last, "row") || strings.EqualFold(last, "rows")
}

// startsWith reports whether e starts with the given words, ignoring case.
func (c *collector) startsWith(e *syntax.Expr, words ...string) bool {
	if int(e.To-e.From) < len(words) {
		return false
	}

	for i, w := range words {
		if !strings.EqualFold(c.text(e.From+int32(i)), w) { //nolint:gosec // i < len(words), a handful
			return false
		}
	}

	return true
}

// text returns the text of token i.
func (c *collector) text(i int32) string {
	return c.p.Tokens[i].Text(c.p.Src)
}
