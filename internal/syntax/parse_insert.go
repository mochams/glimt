package syntax

// parseInsert parses an INSERT statement with the cursor on INSERT. The
// statement started at token start, and with is its WITH clause, if any.
func (p *parser) parseInsert(start int32, with *With) (*Insert, error) {
	ins := &Insert{Span: Span{From: start}, With: with}
	p.advance() // INSERT

	if err := p.expectKw(INTO); err != nil {
		return nil, err
	}

	if err := p.parseInsertTarget(ins); err != nil {
		return nil, err
	}

	if err := p.parseInsertSource(ins); err != nil {
		return nil, err
	}

	var err error
	if p.atKw(ON) {
		ins.OnConflict, err = p.parseOnConflict()
	}

	if err == nil && p.atKw(RETURNING) {
		ins.Returning, err = p.parseReturning()
	}

	if err != nil {
		return nil, err
	}

	ins.To = p.pos

	return ins, nil
}

// parseInsertTarget parses the table, the optional AS alias, the optional
// column list and the optional OVERRIDING clause. A "(" that opens a query
// starts the source, not a column list.
func (p *parser) parseInsertTarget(ins *Insert) error {
	var err error
	if ins.Table, ins.Alias, err = p.parseTable(true); err != nil {
		return err
	}

	if p.atKind(LPAREN) && !p.startsQuery(p.pos+1) && p.peek(1).Kind != LPAREN {
		if ins.Columns, err = p.parseTargetList(); err != nil {
			return err
		}
	}

	ins.Overriding, err = p.parseOverriding()

	return err
}

// parseOverriding parses an optional OVERRIDING { SYSTEM | USER } VALUE.
func (p *parser) parseOverriding() (Overriding, error) {
	if !p.acceptKw(OVERRIDING) {
		return OverridingNone, nil
	}

	var o Overriding

	switch {
	case p.acceptKw(SYSTEM):
		o = OverridingSystem
	case p.acceptKw(USER):
		o = OverridingUser
	default:
		return OverridingNone, p.unexpected("SYSTEM or USER")
	}

	return o, p.expectKw(VALUE)
}

// parseInsertSource parses DEFAULT VALUES or the query that supplies the rows.
func (p *parser) parseInsertSource(ins *Insert) error {
	if p.acceptKw(DEFAULT) {
		ins.DefaultValues = true

		return p.expectKw(VALUES)
	}

	if !p.atQuery() {
		return p.unexpected("VALUES, a query or DEFAULT VALUES")
	}

	var err error
	ins.Source, err = p.parseQuery()

	return err
}

// parseOnConflict parses ON CONFLICT [target] DO NOTHING | DO UPDATE SET … [WHERE …].
func (p *parser) parseOnConflict() (*OnConflict, error) {
	oc := &OnConflict{Span: Span{From: p.pos}}
	p.advance() // ON

	if err := p.expectKw(CONFLICT); err != nil {
		return nil, err
	}

	if !p.atKw(DO) {
		target, err := p.requireExpr("conflict target or DO", targetStops)
		if err != nil {
			return nil, err
		}

		oc.Target = &target
	}

	if err := p.expectKw(DO); err != nil {
		return nil, err
	}

	if err := p.parseConflictAction(oc); err != nil {
		return nil, err
	}

	oc.To = p.pos

	return oc, nil
}

// parseConflictAction parses what follows DO: NOTHING, or UPDATE SET … [WHERE …].
func (p *parser) parseConflictAction(oc *OnConflict) error {
	if p.acceptKw(NOTHING) {
		oc.DoNothing = true

		return nil
	}

	if err := p.expectKw(UPDATE); err != nil {
		return err
	}

	if err := p.expectKw(SET); err != nil {
		return err
	}

	var err error
	if oc.Set, err = p.parseAssignments(clauseItemStops); err != nil {
		return err
	}

	if p.atKw(WHERE) {
		oc.Where, err = p.clause(clauseStops, WHERE)
	}

	return err
}
