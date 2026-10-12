package syntax

// Stop sets for the clauses of a MERGE.
var (
	mergeSourceStops = stopSet{kws: kws(ON, WHEN), joins: true}             // USING source, maybe a join
	mergeOnStops     = stopSet{kws: clauseKeywords.with(WHEN)}              // ON condition
	mergeCondStops   = stopSet{kws: kws(THEN)}                              // WHEN … AND condition
	mergeSetStops    = stopSet{kws: clauseKeywords.with(WHEN), comma: true} // UPDATE SET values
)

// parseMerge parses a MERGE statement with the cursor on MERGE. The
// statement started at token start, and with is its WITH clause, if any.
func (p *parser) parseMerge(start int32, with *With) (*Merge, error) {
	m := &Merge{Span: Span{From: start}, With: with}
	p.advance() // MERGE

	if err := p.expectKw(INTO); err != nil {
		return nil, err
	}

	m.Only = p.acceptKw(ONLY)

	var err error
	if m.Table, m.Alias, err = p.parseTable(false); err != nil {
		return nil, err
	}

	if m.Using, err = p.clause(mergeSourceStops, USING); err != nil {
		return nil, err
	}

	if m.On, err = p.clause(mergeOnStops, ON); err != nil {
		return nil, err
	}

	if err := p.parseMergeTail(m); err != nil {
		return nil, err
	}

	m.To = p.pos

	return m, nil
}

// parseMergeTail parses one or more WHEN clauses and an optional RETURNING.
func (p *parser) parseMergeTail(m *Merge) error {
	if !p.atKw(WHEN) {
		return p.unexpected("WHEN")
	}

	for p.atKw(WHEN) {
		w, err := p.parseMergeWhen()
		if err != nil {
			return err
		}

		m.When = append(m.When, w)
	}

	var err error
	if p.atKw(RETURNING) {
		m.Returning, err = p.parseReturning()
	}

	return err
}

// parseMergeWhen parses WHEN [NOT] MATCHED [BY …] [AND cond] THEN action.
func (p *parser) parseMergeWhen() (*MergeWhen, error) {
	w := &MergeWhen{Span: Span{From: p.pos}}
	p.advance() // WHEN

	var err error
	if w.Match, err = p.parseMergeMatch(); err != nil {
		return nil, err
	}

	if p.acceptKw(AND) {
		cond, err := p.requireExpr("condition after AND", mergeCondStops)
		if err != nil {
			return nil, err
		}

		w.Cond = &cond
	}

	if err := p.expectKw(THEN); err != nil {
		return nil, err
	}

	if err := p.parseMergeAction(w); err != nil {
		return nil, err
	}

	w.To = p.pos

	return w, nil
}

// parseMergeMatch parses MATCHED, NOT MATCHED [BY TARGET] or NOT MATCHED BY SOURCE.
func (p *parser) parseMergeMatch() (MergeMatch, error) {
	if p.acceptKw(MATCHED) {
		return Matched, nil
	}

	if err := p.expectKw(NOT); err != nil {
		return 0, err
	}

	if err := p.expectKw(MATCHED); err != nil {
		return 0, err
	}

	if !p.acceptKw(BY) {
		return NotMatched, nil
	}

	if p.acceptKw(SOURCE) {
		return NotMatchedBySource, nil
	}

	return NotMatched, p.expectKw(TARGET)
}

// parseMergeAction parses a WHEN clause's action. INSERT is only allowed for
// rows not matched in the target, and UPDATE and DELETE only for the others.
func (p *parser) parseMergeAction(w *MergeWhen) error {
	inserting := w.Match == NotMatched

	switch {
	case p.acceptKw(DO):
		w.Action = MergeDoNothing

		return p.expectKw(NOTHING)
	case inserting && p.acceptKw(INSERT):
		w.Action = MergeInsert

		return p.parseMergeInsert(w)
	case !inserting && p.acceptKw(UPDATE):
		w.Action = MergeUpdate

		return p.parseMergeUpdate(w)
	case !inserting && p.acceptKw(DELETE):
		w.Action = MergeDelete

		return nil
	case inserting:
		return p.unexpected("INSERT or DO NOTHING")
	}

	return p.unexpected("UPDATE, DELETE or DO NOTHING")
}

// parseMergeUpdate parses SET … after UPDATE.
func (p *parser) parseMergeUpdate(w *MergeWhen) error {
	if err := p.expectKw(SET); err != nil {
		return err
	}

	var err error
	w.Set, err = p.parseAssignments(mergeSetStops)

	return err
}

// parseMergeInsert parses [(cols)] [OVERRIDING …] VALUES (…) | DEFAULT VALUES after INSERT.
func (p *parser) parseMergeInsert(w *MergeWhen) error {
	var err error
	if p.atKind(LPAREN) {
		if w.Columns, err = p.parseTargetList(); err != nil {
			return err
		}
	}

	if w.Overriding, err = p.parseOverriding(); err != nil {
		return err
	}

	if p.acceptKw(DEFAULT) {
		w.DefaultValues = true

		return p.expectKw(VALUES)
	}

	if err := p.expectKw(VALUES); err != nil {
		return err
	}

	w.Values, err = p.parseRow()

	return err
}
