package syntax

// parseUpdate parses an UPDATE statement with the cursor on UPDATE. The
// statement started at token start, and with is its WITH clause, if any.
func (p *parser) parseUpdate(start int32, with *With) (*Update, error) {
	u := &Update{Span: Span{From: start}, With: with}
	p.advance() // UPDATE

	u.Only = p.acceptKw(ONLY)

	var err error
	if u.Table, u.Alias, err = p.parseTable(false); err != nil {
		return nil, err
	}

	if err := p.expectKw(SET); err != nil {
		return nil, err
	}

	if u.Set, err = p.parseAssignments(clauseItemStops); err != nil {
		return nil, err
	}

	if err := p.parseUpdateClauses(u); err != nil {
		return nil, err
	}

	u.To = p.pos

	return u, nil
}

// parseUpdateClauses parses the optional FROM, WHERE and RETURNING clauses of an UPDATE.
func (p *parser) parseUpdateClauses(u *Update) error {
	var err error
	if p.atKw(FROM) {
		u.From, err = p.clause(fromStops, FROM)
	}

	if err == nil && p.atKw(WHERE) {
		u.Where, err = p.clause(clauseStops, WHERE)
	}

	if err == nil && p.atKw(RETURNING) {
		u.Returning, err = p.parseReturning()
	}

	return err
}

// parseAssignments parses a comma-separated SET list. stop ends each value.
func (p *parser) parseAssignments(stop stopSet) ([]Assignment, error) {
	var list []Assignment
	for {
		a, err := p.parseAssignment(stop)
		if err != nil {
			return nil, err
		}

		list = append(list, a)
		if !p.acceptKind(COMMA) {
			break
		}
	}

	return list, nil
}

// parseAssignment parses one SET item: target = value, or (a, b) = value.
func (p *parser) parseAssignment(stop stopSet) (Assignment, error) {
	a := Assignment{Span: Span{From: p.pos}}

	var err error
	if a.Tuple = p.atKind(LPAREN); a.Tuple {
		a.Targets, err = p.parseTargetList()
	} else {
		a.Targets, err = p.parseSingleTarget()
	}

	if err != nil {
		return Assignment{}, err
	}

	if t := p.tok(); t.Kind != OP || t.Text(p.src) != "=" {
		return Assignment{}, p.unexpected(`"="`)
	}

	p.advance()

	if a.Value, err = p.requireExpr("value after =", stop); err != nil {
		return Assignment{}, err
	}

	a.To = p.pos

	return a, nil
}

// parseSingleTarget parses the lone target of a col = value assignment.
func (p *parser) parseSingleTarget() ([]Target, error) {
	t, err := p.parseTarget()
	if err != nil {
		return nil, err
	}

	return []Target{t}, nil
}

// parseTargetList parses a parenthesized, comma-separated list of targets.
func (p *parser) parseTargetList() ([]Target, error) {
	if err := p.expectKind(LPAREN); err != nil {
		return nil, err
	}

	var list []Target
	for {
		t, err := p.parseTarget()
		if err != nil {
			return nil, err
		}

		list = append(list, t)
		if !p.acceptKind(COMMA) {
			break
		}
	}

	if err := p.expectKind(RPAREN); err != nil {
		return nil, err
	}

	return list, nil
}

// parseTarget parses a column and its optional path of fields and subscripts.
func (p *parser) parseTarget() (Target, error) {
	t := Target{Span: Span{From: p.pos}}

	var err error
	if t.Column, err = p.parseIdent("column name"); err != nil {
		return Target{}, err
	}

	if p.atKind(DOT) || p.atKind(LBRACK) {
		path, err := p.parsePath()
		if err != nil {
			return Target{}, err
		}

		t.Path = &path
	}

	t.To = p.pos

	return t, nil
}

// parsePath parses a chain of .field and [subscript] after a target column.
// Subscripts are light expressions, so their params and subqueries are recorded.
func (p *parser) parsePath() (Expr, error) {
	path := Expr{Span: Span{From: p.pos}}

	for {
		var err error

		switch {
		case p.acceptKind(DOT):
			_, err = p.parseIdent("field name")
		case p.acceptKind(LBRACK):
			err = p.parseSubscript(&path)
		default:
			path.To = p.pos

			return path, nil
		}

		if err != nil {
			return Expr{}, err
		}
	}
}

// parseSubscript parses the inside of a subscript and its closing "]", with
// the cursor after "[". It adds the subscript's params and subqueries to path.
func (p *parser) parseSubscript(path *Expr) error {
	sub, err := p.requireExpr("subscript", noStops)
	if err != nil {
		return err
	}

	path.Params = append(path.Params, sub.Params...)
	path.Subqueries = append(path.Subqueries, sub.Subqueries...)

	return p.expectKind(RBRACK)
}

// parseReturning parses a RETURNING clause, which ends the statement.
func (p *parser) parseReturning() (*Expr, error) {
	return p.clause(targetListStops, RETURNING)
}
