package syntax

// parseQuery parses a query, including any leading WITH clause.
func (p *parser) parseQuery() (*Query, error) {
	start := p.pos

	var with *With
	if p.atKw(WITH) {
		w, err := p.parseWith()
		if err != nil {
			return nil, err
		}

		with = w
	}

	switch p.tok().Kw {
	case INSERT, UPDATE, DELETE, MERGE:
		return nil, p.unsupported("a data-modifying statement in WITH")
	}

	return p.parseQueryWith(start, with)
}

// parseQueryWith parses a query body and its trailing clauses. The query
// started at token start, and with is its already parsed WITH clause, if any.
func (p *parser) parseQueryWith(start int32, with *With) (*Query, error) {
	body, err := p.parseSetExpr(1)
	if err != nil {
		return nil, err
	}

	q := &Query{With: with, Body: body}
	if err := p.parseQueryTail(q); err != nil {
		return nil, err
	}

	q.Span = Span{From: start, To: p.pos}

	return q, nil
}

// setOpAt returns the set operation at token t and its precedence, or a zero
// precedence when t is not a set operation. INTERSECT binds tighter than
// UNION and EXCEPT, as in Postgres.
func setOpAt(t Token) (SetOpKind, int) {
	switch t.Kw {
	case UNION:
		return Union, 1
	case EXCEPT:
		return Except, 1
	case INTERSECT:
		return Intersect, 2
	}

	return 0, 0
}

// parseSetExpr parses set operations by precedence climbing. Operations
// binding looser than minPrec are left to the caller. All of them are left-associative.
func (p *parser) parseSetExpr(minPrec int) (SetExpr, error) {
	left, err := p.parseSetOperand()
	if err != nil {
		return nil, err
	}

	for {
		op, prec := setOpAt(p.tok())
		if prec == 0 || prec < minPrec {
			return left, nil
		}

		p.advance()

		all := p.acceptKw(ALL)
		if !all {
			p.acceptKw(DISTINCT) // UNION DISTINCT is plain UNION
		}

		right, err := p.parseSetExpr(prec + 1)
		if err != nil {
			return nil, err
		}

		span := Span{From: left.Bounds().From, To: right.Bounds().To}
		left = &SetOp{Span: span, Op: op, All: all, Left: left, Right: right}
	}
}

// parseSetOperand parses one operand of a set operation: a SELECT, a VALUES
// list or a parenthesized query.
func (p *parser) parseSetOperand() (SetExpr, error) {
	switch {
	case p.atKw(SELECT):
		return setExprOf(p.parseSelect())
	case p.atKw(VALUES):
		return setExprOf(p.parseValues())
	case p.atKw(TABLE):
		return setExprOf(p.parseTableQuery())
	case p.atKind(LPAREN):
		return setExprOf(p.parseParenQuery())
	}

	return nil, p.unexpected("SELECT, VALUES, TABLE or a parenthesized query")
}

// setExprOf converts a parse result to a SetExpr, so a failed parse returns a
// nil interface rather than an interface holding a nil pointer.
func setExprOf[T SetExpr](s T, err error) (SetExpr, error) {
	if err != nil {
		return nil, err
	}

	return s, nil
}

// parseTableQuery parses TABLE name.
func (p *parser) parseTableQuery() (*TableQuery, error) {
	tq := &TableQuery{Span: Span{From: p.pos}}
	p.advance() // TABLE
	tq.Only = p.acceptKw(ONLY)

	name, err := p.parseObjectName()
	if err != nil {
		return nil, err
	}

	p.acceptStar()

	tq.Name = name
	tq.To = p.pos

	return tq, nil
}

// parseParenQuery parses "( query )" used as a set operand.
func (p *parser) parseParenQuery() (*ParenQuery, error) {
	pq := &ParenQuery{Span: Span{From: p.pos}}
	p.advance() // (

	q, err := p.parseQuery()
	if err != nil {
		return nil, err
	}

	if err := p.expectKind(RPAREN); err != nil {
		return nil, err
	}

	pq.Query = q
	pq.To = p.pos

	return pq, nil
}

// parseSelect parses a SELECT core: everything up to, but not including, a
// set operation, ORDER BY, LIMIT or OFFSET.
func (p *parser) parseSelect() (*Select, error) {
	s := &Select{Span: Span{From: p.pos}}
	p.advance() // SELECT

	if err := p.parseDistinct(s); err != nil {
		return nil, err
	}

	cols, err := p.scanExpr(targetListStops)
	if err != nil {
		return nil, err
	}

	s.Columns = cols

	if p.atKw(INTO) {
		return nil, p.unsupported("SELECT INTO")
	}

	if err := p.parseSelectClauses(s); err != nil {
		return nil, err
	}

	s.To = p.pos

	return s, nil
}

// parseDistinct parses an optional ALL, DISTINCT or DISTINCT ON ( … ).
func (p *parser) parseDistinct(s *Select) error {
	if s.All = p.acceptKw(ALL); s.All || !p.acceptKw(DISTINCT) {
		return nil
	}

	s.Distinct = true
	if !p.acceptKw(ON) {
		return nil
	}

	if err := p.expectKind(LPAREN); err != nil {
		return err
	}

	on, err := p.requireExpr("expression after DISTINCT ON", noStops)
	if err != nil {
		return err
	}

	s.DistinctOn = &on

	return p.expectKind(RPAREN)
}

// parseSelectClauses parses the optional FROM, WHERE, GROUP BY, HAVING and
// WINDOW clauses of a SELECT core.
func (p *parser) parseSelectClauses(s *Select) error {
	var err error
	if p.atKw(FROM) {
		s.From, err = p.clause(fromStops, FROM)
	}

	if err == nil && p.atKw(WHERE) {
		s.Where, err = p.clause(clauseStops, WHERE)
	}

	if err == nil && p.atKw(GROUP) {
		s.GroupBy, err = p.clause(clauseStops, GROUP, BY)
	}

	if err == nil && p.atKw(HAVING) {
		s.Having, err = p.clause(clauseStops, HAVING)
	}

	if err == nil && p.atKw(WINDOW) {
		s.Window, err = p.clause(clauseStops, WINDOW)
	}

	return err
}

// parseValues parses a VALUES list: VALUES (…), (…).
func (p *parser) parseValues() (*Values, error) {
	v := &Values{Span: Span{From: p.pos}}
	p.advance() // VALUES

	for {
		row, err := p.parseRow()
		if err != nil {
			return nil, err
		}

		v.Rows = append(v.Rows, row)
		if !p.acceptKind(COMMA) {
			break
		}
	}

	v.To = p.pos

	return v, nil
}

// parseRow parses one parenthesized VALUES row, one Expr per column.
func (p *parser) parseRow() ([]Expr, error) {
	if err := p.expectKind(LPAREN); err != nil {
		return nil, err
	}

	var row []Expr
	for {
		e, err := p.requireExpr("value", listStops)
		if err != nil {
			return nil, err
		}

		row = append(row, e)
		if !p.acceptKind(COMMA) {
			break
		}
	}

	if err := p.expectKind(RPAREN); err != nil {
		return nil, err
	}

	return row, nil
}

// parseQueryTail parses a query's optional ORDER BY, then in any order its
// LIMIT or FETCH, OFFSET, and locking clauses, as Postgres allows.
func (p *parser) parseQueryTail(q *Query) error {
	var err error
	if p.atKw(ORDER) {
		q.OrderBy, err = p.clause(clauseStops, ORDER, BY)
	}

	for err == nil {
		var done bool
		if done, err = p.parseTailClause(q); done {
			return err
		}
	}

	return err
}

// parseTailClause parses one LIMIT, OFFSET, FETCH or locking clause into q.
// done reports that no such clause follows, or that q already has it.
func (p *parser) parseTailClause(q *Query) (done bool, err error) {
	kw := p.tok().Kw
	field := tailField(q, kw)

	switch {
	case field == nil || *field != nil:
		return true, nil
	case kw == LIMIT && q.Fetch != nil, kw == FETCH && q.Limit != nil:
		return true, p.errorf("LIMIT and FETCH can't be used together")
	}

	stop := clauseStops
	if kw == FOR {
		stop = lockingStops
	}

	*field, err = p.clause(stop, kw)

	return false, err
}

// tailField returns the field of q that holds the tail clause starting with
// kw, or nil when kw starts none.
func tailField(q *Query, kw Keyword) **Expr {
	switch kw {
	case LIMIT:
		return &q.Limit
	case OFFSET:
		return &q.Offset
	case FETCH:
		return &q.Fetch
	case FOR:
		return &q.Locking
	}

	return nil
}

// clause consumes the keywords that introduce a clause, such as GROUP BY,
// then scans the clause's expression, which must not be empty.
func (p *parser) clause(stop stopSet, intro ...Keyword) (*Expr, error) {
	for _, kw := range intro {
		if err := p.expectKw(kw); err != nil {
			return nil, err
		}
	}

	e, err := p.scanExpr(stop)
	if err != nil {
		return nil, err
	}

	if e.Empty() {
		return nil, p.unexpected("expression after " + joinKeywords(intro))
	}

	return &e, nil
}

// joinKeywords spells a keyword sequence such as GROUP BY.
func joinKeywords(list []Keyword) string {
	s := list[0].String()
	for _, kw := range list[1:] {
		s += " " + kw.String()
	}

	return s
}

// requireExpr scans an expression that must not be empty. what names the
// expected expression in the error message.
func (p *parser) requireExpr(what string, stop stopSet) (Expr, error) {
	e, err := p.scanExpr(stop)
	if err != nil {
		return Expr{}, err
	}

	if e.Empty() {
		return Expr{}, p.unexpected(what)
	}

	return e, nil
}
