package syntax

import "strings"

// stopSet tells scanExpr where a clause ends. Besides these, a clause always
// ends at ";", at EOF, at a ")" or "]" it did not open, and at ON CONFLICT.
type stopSet struct {
	kws   kwSet // keywords that end the clause
	comma bool  // a comma ends the clause
	joins bool  // the clause is a join tree: an ON or USING that belongs to a JOIN doesn't end it
	// labels marks a target list, where SELECT or TABLE right after an item's
	// expression may be its column label rather than the next statement.
	labels bool
}

// clauseKeywords are the keywords that start a clause, and SELECT and TABLE,
// which start a statement. Every clause ends at any of them, so a clause out
// of order, or a second statement, is an error rather than part of the
// clause before it. All of them are reserved in Postgres, so none can be a
// column name. SELECT and TABLE appear inside a clause only in parentheses,
// or as a column label in a target list; see atBareLabel.
var clauseKeywords = kws(
	INTO, FROM, WHERE, GROUP, HAVING, WINDOW, ORDER, LIMIT, OFFSET, FOR, FETCH,
	UNION, INTERSECT, EXCEPT, RETURNING, SELECT, TABLE,
)

// Stop sets for scanExpr.
var (
	noStops         = stopSet{}                                  // ends only at ";", EOF, ")" or "]"
	listStops       = stopSet{comma: true}                       // an item of a parenthesized list
	clauseStops     = stopSet{kws: clauseKeywords}               // a clause body
	clauseItemStops = stopSet{kws: clauseKeywords, comma: true}  // an item of a clause list, such as SET
	fromStops       = stopSet{kws: clauseKeywords, joins: true}  // FROM or USING, a join tree
	targetListStops = stopSet{kws: clauseKeywords, labels: true} // a select list or RETURNING list
	targetStops     = stopSet{kws: kws(DO)}                      // an ON CONFLICT target, which may hold WHERE
	lockingStops    = stopSet{kws: clauseKeywords.without(FOR)}  // locking clauses: FOR … FOR … is one span
)

// nesting is the stack of brackets and CASE expressions a scan is inside.
// The first levels live inline, so ordinary expressions don't allocate.
// Terminators only count when the stack is empty.
type nesting struct {
	inline [8]Kind // open LPAREN, LBRACK, or IDENT for CASE
	more   []Kind  // levels past the inline ones
	n      int

	joins   int  // JOINs at depth zero still waiting for their ON or USING
	natural bool // a NATURAL was seen, so the next JOIN takes no condition
}

// push opens a level.
func (s *nesting) push(k Kind) {
	if s.n < len(s.inline) {
		s.inline[s.n] = k
	} else {
		s.more = append(s.more, k)
	}

	s.n++
}

// at returns open level i, counting from the outermost.
func (s *nesting) at(i int) Kind {
	if i < len(s.inline) {
		return s.inline[i]
	}

	return s.more[i-len(s.inline)]
}

// top returns the innermost open level, or EOF when none is open.
func (s *nesting) top() Kind {
	if s.n == 0 {
		return EOF
	}

	return s.at(s.n - 1)
}

// pop closes the innermost level.
func (s *nesting) pop() {
	s.n--
	if s.n >= len(s.inline) {
		s.more = s.more[:s.n-len(s.inline)]
	}
}

// openCase reports whether a CASE is open at any level.
func (s *nesting) openCase() bool {
	for i := range s.n {
		if s.at(i) == IDENT {
			return true
		}
	}

	return false
}

// opener returns the kind that closer kind k closes.
func opener(k Kind) Kind {
	if k == RBRACK {
		return LBRACK
	}

	return LPAREN
}

// closer names what closes level k, for error messages.
func closer(k Kind) string {
	switch k {
	case LPAREN:
		return `")"`
	case LBRACK:
		return `"]"`
	}

	return "END"
}

// track updates the stack for token t, whose keyword is ignored when it is a
// column label after AS. A closer must match the innermost open level.
func (p *parser) track(depth *nesting, t Token, label bool) error {
	switch {
	case t.Kind == LPAREN, t.Kind == LBRACK:
		depth.push(t.Kind)
	case t.Kind == RPAREN, t.Kind == RBRACK:
		if depth.top() != opener(t.Kind) {
			return p.unexpected(closer(depth.top()))
		}

		depth.pop()
	case label:
	case t.Kw == CASE:
		depth.push(IDENT)
	case t.Kw == END && depth.top() == IDENT:
		depth.pop()
	case t.Kw == END && depth.openCase():
		return p.unexpected(closer(depth.top()))
	}

	return nil
}

// scanExpr scans an expression from the current token up to the first
// terminator in stop, leaving the cursor on the terminator. It records the
// params it passes and parses each parenthesized query it meets as a
// subquery. The returned Expr may be empty.
func (p *parser) scanExpr(stop stopSet) (Expr, error) {
	e := Expr{Span: Span{From: p.pos}}

	var depth nesting
	for !p.scanDone(stop, e.From, &depth) {
		if err := p.scanToken(&e, &depth, stop); err != nil {
			return Expr{}, err
		}
	}

	if err := p.checkClosed(&depth); err != nil {
		return Expr{}, err
	}

	e.To = p.pos

	return e, nil
}

// scanToken consumes the token at the cursor as part of e. A "(" that opens a
// query is consumed with the whole subquery.
func (p *parser) scanToken(e *Expr, depth *nesting, stop stopSet) error {
	t := p.tok()
	if stop.joins && depth.n == 0 {
		p.trackJoin(depth, t)
	}

	switch {
	case t.Kind == PARAM:
		e.Params = append(e.Params, p.newParam())
	case t.Kind == LPAREN && p.startsQuery(p.pos+1):
		sub, err := p.parseSubquery()
		if err != nil {
			return err
		}

		e.Subqueries = append(e.Subqueries, sub)

		return nil
	}

	if err := p.track(depth, t, p.atLabel(e.From) || p.atBareCase(e.From)); err != nil {
		return err
	}

	p.advance()

	return nil
}

// trackJoin counts the JOINs at depth zero of a join tree that still need a
// condition. CROSS and NATURAL joins take none; an ON or USING satisfies one.
func (p *parser) trackJoin(depth *nesting, t Token) {
	switch t.Kw {
	case NATURAL:
		depth.natural = true
	case JOIN:
		if depth.natural || p.pos > 0 && p.toks[p.pos-1].Kw == CROSS {
			depth.natural = false
		} else {
			depth.joins++
		}
	case ON, USING:
		if depth.joins > 0 {
			depth.joins--
		}
	}
}

// checkClosed reports a bracket or CASE that was still open when the scan stopped.
func (p *parser) checkClosed(depth *nesting) error {
	switch {
	case depth.n == 0:
		return nil
	case depth.top() == IDENT:
		return p.errorf("expected END to close CASE, found %s; "+
			"a column label named case is not supported unless written AS case", p.describe())
	}

	return p.unexpected(closer(depth.top()))
}

// scanDone reports whether a scan that started at token start stops at the
// cursor. ";" and EOF stop it at any depth, and a ")" or "]" the scan did
// not open stops it. The stop set only applies outside every level.
func (p *parser) scanDone(stop stopSet, start int32, depth *nesting) bool {
	switch t := p.tok(); {
	case t.Kind == EOF, t.Kind == SEMICOLON:
		return true
	case depth.n > 0:
		return false
	case t.Kind == RPAREN, t.Kind == RBRACK:
		return true
	}

	if stop.joins && depth.joins > 0 && (p.atKw(ON) || p.atKw(USING)) {
		return false // the condition of a JOIN in the join tree
	}

	return p.atTerminator(stop, start)
}

// atTerminator reports whether the token at the cursor ends a clause that
// started at token start, given that the scan is outside every level.
func (p *parser) atTerminator(stop stopSet, start int32) bool {
	t := p.tok()
	switch {
	case t.Kind == COMMA:
		return stop.comma
	case p.inExpression(start, stop):
		return false
	case t.Kw == ON && p.peek(1).Kw == CONFLICT:
		return true
	case stop.labels && p.atBareLabel(start):
		return false
	}

	return stop.kws.has(t.Kw)
}

// inExpression reports whether the keyword at the cursor belongs to the
// expression being scanned from token start, though it could end a clause
// elsewhere: a column label after AS, FROM in IS [NOT] DISTINCT FROM or in
// the ROWS FROM (…) table function of a join tree, or GROUP in WITHIN GROUP
// (ORDER BY …). stop is the scan's stop set.
func (p *parser) inExpression(start int32, stop stopSet) bool {
	if p.pos == start {
		return false
	}

	if p.atLabel(start) {
		return true
	}

	if p.atKw(FOR) {
		return !p.atLocking() // COLLATION FOR (…)
	}

	switch prev := p.toks[p.pos-1].Kw; p.tok().Kw {
	case FROM:
		return prev == DISTINCT && p.atDistinctFrom() || prev == ROWS && stop.joins && p.atRowsFrom()
	case GROUP:
		return prev == WITHIN
	}

	return false
}

// atDistinctFrom reports whether the FROM at the cursor, after DISTINCT, is
// part of IS [NOT] DISTINCT FROM. After anything but IS or NOT, DISTINCT is
// a column label, as in SELECT a distinct FROM t, and the FROM starts the
// FROM clause.
func (p *parser) atDistinctFrom() bool {
	if p.pos < 2 {
		return false
	}

	before := p.toks[p.pos-2].Kw

	return before == IS || before == NOT
}

// atRowsFrom reports whether the FROM at the cursor, after ROWS, in a join
// tree, is part of the ROWS FROM (…) table function: the ROWS starts a FROM
// item, after FROM, a comma, JOIN or LATERAL, and a "(" follows. The caller
// checks the join tree: only a FROM clause, USING or MERGE source holds FROM
// items, so elsewhere ROWS is a column or its label, as in SELECT 0, rows
// FROM t or SELECT count(*) rows FROM t, and the FROM starts the FROM
// clause. The token before ROWS may be outside the scan, as FROM is.
func (p *parser) atRowsFrom() bool {
	if p.pos < 2 || p.peek(1).Kind != LPAREN {
		return false
	}

	before := p.toks[p.pos-2]

	return before.Kind == COMMA || before.Kw == FROM || before.Kw == JOIN || before.Kw == LATERAL
}

// atBareLabel reports whether a SELECT or TABLE at the cursor is a column
// label written without AS, as in SELECT 0 select: of the words that end a
// clause, Postgres takes only these two as bare labels. A label follows the
// item's expression and comes right before the end of the item: a comma, a
// ")", the end of the statement or a clause keyword. Before anything else,
// as in SELECT 1 SELECT 2, the word starts a second statement.
func (p *parser) atBareLabel(start int32) bool {
	if !p.atKw(SELECT) && !p.atKw(TABLE) || p.pos == start || p.toks[p.pos-1].Kind == COMMA {
		return false
	}

	next := p.peek(1)
	switch next.Kind {
	case COMMA, RPAREN, SEMICOLON, EOF:
		return true
	}

	return clauseKeywords.has(next.Kw) && next.Kw != SELECT && next.Kw != TABLE
}

// atBareCase reports whether a CASE at the cursor is a column label written
// without AS, as in SELECT 0 case: it directly follows a token that can only
// end an operand, where no expression can start. A bare word isn't one, since
// it may be a keyword glimt doesn't know, such as ELSE or OR.
func (p *parser) atBareCase(start int32) bool {
	if !p.atKw(CASE) || p.pos == start {
		return false
	}

	switch prev := p.toks[p.pos-1]; prev.Kind {
	case NUMBER, STRING, QIDENT, PARAM, RPAREN, RBRACK:
		return true
	case IDENT:
		qualified := p.pos-2 >= start && p.toks[p.pos-2].Kind == DOT // t.x case

		return prev.Kw == END || qualified
	}

	return false
}

// atLocking reports whether the FOR at the cursor starts a locking clause:
// FOR UPDATE, FOR NO KEY UPDATE, FOR SHARE or FOR KEY SHARE. Any other FOR
// belongs to an expression, as in COLLATION FOR (x).
func (p *parser) atLocking() bool {
	next := p.peek(1)
	if next.Kw == UPDATE {
		return true
	}

	word := next.Text(p.src)

	return next.Kind == IDENT && (strings.EqualFold(word, "share") || strings.EqualFold(word, "no") ||
		strings.EqualFold(word, "key"))
}

// atLabel reports whether the token at the cursor directly follows AS within
// a scan that started at token start. Postgres takes any word there as a
// column label, keywords included.
func (p *parser) atLabel(start int32) bool {
	return p.pos > start && p.toks[p.pos-1].Kw == AS && p.tok().Kind == IDENT
}

// startsQuery reports whether token i begins a query, which makes the "("
// before it open a subquery. VALUES can also be a column name, so it only
// starts a query when a row follows it.
func (p *parser) startsQuery(i int32) bool {
	switch p.tokAt(i).Kw {
	case SELECT, WITH, TABLE:
		return true
	case VALUES:
		return p.tokAt(i+1).Kind == LPAREN
	}

	return false
}

// parseSubquery parses "( query )" with the cursor on "(".
func (p *parser) parseSubquery() (*Subquery, error) {
	from := p.pos
	p.advance() // (

	q, err := p.parseQuery()
	if err != nil {
		return nil, err
	}

	if p.atKw(RETURNING) {
		return nil, p.unsupported("RETURNING after a query in parentheses, as in JSON_ARRAY(SELECT … RETURNING …),")
	}

	if p.tok().Kind != RPAREN {
		return nil, p.unexpected(`")" after subquery`)
	}

	p.advance() // )

	return &Subquery{Span: Span{From: from, To: p.pos}, Query: q}, nil
}

// newParam records the PARAM token at the cursor. Params come from one
// pre-sized slab, so recording them doesn't allocate.
func (p *parser) newParam() *Param {
	t := p.tok()
	prm := Param{Name: p.src[t.Pos+1 : t.End], Tok: p.pos, Mode: p.paramMode(p.pos)}

	if len(p.params) == cap(p.params) {
		return &prm // unreachable: the slab holds every PARAM token
	}

	p.params = append(p.params, prm)

	return &p.params[len(p.params)-1]
}

// paramMode returns Expand for a param written exactly as IN ( :name ), and
// Scalar otherwise.
func (p *parser) paramMode(i int32) ParamMode {
	if i >= 2 && p.toks[i-2].Kw == IN && p.toks[i-1].Kind == LPAREN && p.toks[i+1].Kind == RPAREN {
		return Expand
	}

	return Scalar
}
