package syntax

import "strings"

// Dump returns an indented, stable description of p's AST for tests and
// debugging. Each span is shown as its tokens' text, with one space wherever
// the source had whitespace or a comment. Dump is not a renderer: its output
// is not SQL.
func Dump(p *Parsed) string {
	d := dumper{p: p}
	d.stmt(0, p.Stmt)
	d.params()

	return d.b.String()
}

// dumper writes a Dump into one buffer.
type dumper struct {
	p *Parsed
	b strings.Builder
}

// stmt dumps a statement.
func (d *dumper) stmt(depth int, s Statement) {
	switch s := s.(type) {
	case *Query:
		d.query(depth, s)
	case *Insert:
		d.insert(depth, s)
	case *Update:
		d.update(depth, s)
	case *Delete:
		d.delete(depth, s)
	case *Merge:
		d.merge(depth, s)
	case *Raw:
		d.expr(depth, "raw", &s.Body)
	}
}

// query dumps a query and its trailing clauses.
func (d *dumper) query(depth int, q *Query) {
	d.label(depth, "query")
	d.with(depth+1, q.With)
	d.setExpr(depth+1, q.Body)
	d.expr(depth+1, "order by", q.OrderBy)
	d.expr(depth+1, "limit", q.Limit)
	d.expr(depth+1, "offset", q.Offset)
	d.expr(depth+1, "fetch", q.Fetch)
	d.expr(depth+1, "for", q.Locking)
}

// setExpr dumps a query body.
func (d *dumper) setExpr(depth int, e SetExpr) {
	switch e := e.(type) {
	case *Select:
		d.selectCore(depth, e)
	case *SetOp:
		d.setOp(depth, e)
	case *Values:
		d.values(depth, e)
	case *TableQuery:
		d.indent(depth)
		d.b.WriteString("table ")

		if e.Only {
			d.b.WriteString("only ")
		}

		d.text(e.Name.Span)
		d.b.WriteByte('\n')
	case *ParenQuery:
		d.label(depth, "paren")
		d.query(depth+1, e.Query)
	}
}

// selectCore dumps a SELECT core.
func (d *dumper) selectCore(depth int, s *Select) {
	switch {
	case s.DistinctOn != nil:
		d.label(depth, "select distinct on")
		d.expr(depth+1, "on", s.DistinctOn)
	case s.Distinct:
		d.label(depth, "select distinct")
	case s.All:
		d.label(depth, "select all")
	default:
		d.label(depth, "select")
	}

	d.expr(depth+1, "columns", &s.Columns)
	d.expr(depth+1, "from", s.From)
	d.expr(depth+1, "where", s.Where)
	d.expr(depth+1, "group by", s.GroupBy)
	d.expr(depth+1, "having", s.Having)
	d.expr(depth+1, "window", s.Window)
}

// setOp dumps a set operation and both operands.
func (d *dumper) setOp(depth int, s *SetOp) {
	name := strings.ToLower(s.Op.String())
	if s.All {
		name += " all"
	}

	d.label(depth, name)
	d.setExpr(depth+1, s.Left)
	d.setExpr(depth+1, s.Right)
}

// values dumps a VALUES list, one line per row.
func (d *dumper) values(depth int, v *Values) {
	d.label(depth, "values")

	for _, row := range v.Rows {
		d.row(depth+1, row)
	}
}

// row dumps one VALUES row and the subqueries in it.
func (d *dumper) row(depth int, row []Expr) {
	d.indent(depth)
	d.b.WriteString("row:")

	for i := range row {
		d.b.WriteString(" [")
		d.text(row[i].Span)
		d.b.WriteByte(']')
	}

	d.b.WriteByte('\n')

	for i := range row {
		d.subqueries(depth+1, &row[i])
	}
}

// with dumps a WITH clause, if present.
func (d *dumper) with(depth int, w *With) {
	if w == nil {
		return
	}

	if w.Recursive {
		d.label(depth, "with recursive")
	} else {
		d.label(depth, "with")
	}

	for _, c := range w.CTEs {
		d.cte(depth+1, c)
	}
}

// cte dumps one common table expression.
func (d *dumper) cte(depth int, c *CTE) {
	d.indent(depth)
	d.b.WriteString("cte ")
	d.b.WriteString(d.p.Tokens[c.Name.Tok].Text(d.p.Src))
	d.identList(c.Columns)

	switch c.Materialized {
	case Materialized:
		d.b.WriteString(" materialized")
	case NotMaterialized:
		d.b.WriteString(" not materialized")
	}

	d.b.WriteByte('\n')
	d.query(depth+1, c.Body)
	d.expr(depth+1, "search", c.Search)
	d.expr(depth+1, "cycle", c.Cycle)
}

// insert dumps an INSERT statement.
func (d *dumper) insert(depth int, s *Insert) {
	d.label(depth, "insert")
	d.with(depth+1, s.With)
	d.target(depth+1, s.Table, s.Alias, false)

	d.targets(depth+1, s.Columns)
	d.overriding(depth+1, s.Overriding)

	if s.DefaultValues {
		d.label(depth+1, "default values")
	} else {
		d.query(depth+1, s.Source)
	}

	d.onConflict(depth+1, s.OnConflict)
	d.expr(depth+1, "returning", s.Returning)
}

// onConflict dumps an ON CONFLICT clause, if present.
func (d *dumper) onConflict(depth int, oc *OnConflict) {
	if oc == nil {
		return
	}

	d.label(depth, "on conflict")
	d.expr(depth+1, "target", oc.Target)

	if oc.DoNothing {
		d.label(depth+1, "do nothing")

		return
	}

	d.label(depth+1, "do update")
	d.assignments(depth+2, oc.Set)
	d.expr(depth+2, "where", oc.Where)
}

// update dumps an UPDATE statement.
func (d *dumper) update(depth int, s *Update) {
	d.label(depth, "update")
	d.with(depth+1, s.With)
	d.target(depth+1, s.Table, s.Alias, s.Only)
	d.assignments(depth+1, s.Set)
	d.expr(depth+1, "from", s.From)
	d.expr(depth+1, "where", s.Where)
	d.expr(depth+1, "returning", s.Returning)
}

// delete dumps a DELETE statement.
func (d *dumper) delete(depth int, s *Delete) {
	d.label(depth, "delete")
	d.with(depth+1, s.With)
	d.target(depth+1, s.Table, s.Alias, s.Only)
	d.expr(depth+1, "using", s.Using)
	d.expr(depth+1, "where", s.Where)
	d.expr(depth+1, "returning", s.Returning)
}

// merge dumps a MERGE statement.
func (d *dumper) merge(depth int, s *Merge) {
	d.label(depth, "merge")
	d.with(depth+1, s.With)
	d.target(depth+1, s.Table, s.Alias, s.Only)
	d.expr(depth+1, "using", s.Using)
	d.expr(depth+1, "on", s.On)

	for _, w := range s.When {
		d.mergeWhen(depth+1, w)
	}

	d.expr(depth+1, "returning", s.Returning)
}

// mergeWhen dumps one WHEN clause of a MERGE.
func (d *dumper) mergeWhen(depth int, w *MergeWhen) {
	d.label(depth, "when "+w.Match.String())
	d.expr(depth+1, "and", w.Cond)
	d.label(depth+1, w.Action.String())

	switch w.Action {
	case MergeUpdate:
		d.assignments(depth+2, w.Set)
	case MergeInsert:
		d.targets(depth+2, w.Columns)
		d.overriding(depth+2, w.Overriding)
		d.mergeValues(depth+2, w)
	}
}

// mergeValues dumps the row a MERGE INSERT adds.
func (d *dumper) mergeValues(depth int, w *MergeWhen) {
	if w.DefaultValues {
		d.label(depth, "default values")

		return
	}

	d.row(depth, w.Values)
}

// targets dumps an INSERT column list, if present.
func (d *dumper) targets(depth int, list []Target) {
	if list == nil {
		return
	}

	d.indent(depth)
	d.b.WriteString("columns: (")

	for i := range list {
		if i > 0 {
			d.b.WriteString(", ")
		}

		d.text(list[i].Span)
	}

	d.b.WriteString(")\n")

	for i := range list {
		if list[i].Path != nil {
			d.subqueries(depth+1, list[i].Path)
		}
	}
}

// overriding dumps an OVERRIDING option, if present.
func (d *dumper) overriding(depth int, o Overriding) {
	switch o {
	case OverridingSystem:
		d.label(depth, "overriding system value")
	case OverridingUser:
		d.label(depth, "overriding user value")
	}
}

// target dumps a statement's table, marked when ONLY is set, and its alias.
func (d *dumper) target(depth int, table ObjectName, alias *Ident, only bool) {
	d.indent(depth)
	d.b.WriteString("table: ")

	if only {
		d.b.WriteString("only ")
	}

	d.text(table.Span)

	if alias != nil {
		d.b.WriteString(" alias: ")
		d.b.WriteString(d.p.Tokens[alias.Tok].Text(d.p.Src))
	}

	d.b.WriteByte('\n')
}

// assignments dumps a SET list, one line per assignment: its targets, then
// "=", then its value.
func (d *dumper) assignments(depth int, list []Assignment) {
	for i := range list {
		a := &list[i]

		d.indent(depth)
		d.b.WriteString("set: ")

		if a.Tuple {
			d.b.WriteByte('(')
		}

		for j := range a.Targets {
			if j > 0 {
				d.b.WriteString(", ")
			}

			d.text(a.Targets[j].Span)
		}

		if a.Tuple {
			d.b.WriteByte(')')
		}

		d.b.WriteString(" = ")
		d.text(a.Value.Span)
		d.b.WriteByte('\n')
		d.subqueries(depth+1, &a.Value)
	}
}

// expr dumps a labelled expression and its subqueries. A nil e is skipped.
func (d *dumper) expr(depth int, label string, e *Expr) {
	if e == nil {
		return
	}

	d.indent(depth)
	d.b.WriteString(label)
	d.b.WriteByte(':')

	if !e.Empty() {
		d.b.WriteByte(' ')
		d.text(e.Span)
	}

	d.b.WriteByte('\n')
	d.subqueries(depth+1, e)
}

// subqueries dumps the subqueries of e.
func (d *dumper) subqueries(depth int, e *Expr) {
	for _, sub := range e.Subqueries {
		d.label(depth, "subquery")
		d.query(depth+1, sub.Query)
	}
}

// identList writes " (a, b)" for a non-empty list of identifiers.
func (d *dumper) identList(list []Ident) {
	if len(list) == 0 {
		return
	}

	d.b.WriteString(" (")

	for i, id := range list {
		if i > 0 {
			d.b.WriteString(", ")
		}

		d.b.WriteString(d.p.Tokens[id.Tok].Text(d.p.Src))
	}

	d.b.WriteByte(')')
}

// params writes the closing params line: each name, with "*" marking Expand.
func (d *dumper) params() {
	if len(d.p.Params) == 0 {
		return
	}

	d.b.WriteString("params:")

	for _, prm := range d.p.Params {
		d.b.WriteByte(' ')
		d.b.WriteString(prm.Name)

		if prm.Mode == Expand {
			d.b.WriteByte('*')
		}
	}

	d.b.WriteByte('\n')
}

// text writes the tokens of s, with one space wherever the source separated them.
func (d *dumper) text(s Span) {
	for i := s.From; i < s.To; i++ {
		t := d.p.Tokens[i]
		if i > s.From && t.Sep != SepNone {
			d.b.WriteByte(' ')
		}

		d.b.WriteString(t.Text(d.p.Src))
	}
}

// label writes a line holding only a node label.
func (d *dumper) label(depth int, label string) {
	d.indent(depth)
	d.b.WriteString(label)
	d.b.WriteByte('\n')
}

// indent writes two spaces per depth level.
func (d *dumper) indent(depth int) {
	for range depth {
		d.b.WriteString("  ")
	}
}
