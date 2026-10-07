package syntax

// parseDelete parses a DELETE statement with the cursor on DELETE. The
// statement started at token start, and with is its WITH clause, if any.
func (p *parser) parseDelete(start int32, with *With) (*Delete, error) {
	d := &Delete{Span: Span{From: start}, With: with}
	p.advance() // DELETE

	if err := p.expectKw(FROM); err != nil {
		return nil, err
	}

	d.Only = p.acceptKw(ONLY)

	var err error
	if d.Table, d.Alias, err = p.parseTable(false); err != nil {
		return nil, err
	}

	if err := p.parseDeleteClauses(d); err != nil {
		return nil, err
	}

	d.To = p.pos

	return d, nil
}

// parseDeleteClauses parses the optional USING, WHERE and RETURNING clauses of a DELETE.
func (p *parser) parseDeleteClauses(d *Delete) error {
	var err error
	if p.atKw(USING) {
		d.Using, err = p.clause(fromStops, USING)
	}

	if err == nil && p.atKw(WHERE) {
		d.Where, err = p.clause(clauseStops, WHERE)
	}

	if err == nil && p.atKw(RETURNING) {
		d.Returning, err = p.parseReturning()
	}

	return err
}
