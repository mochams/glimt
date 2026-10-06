package syntax

import (
	"fmt"
	"strconv"
	"strings"
)

// Error is a parse error.
type Error struct {
	Pos int32 // byte offset in the source of the token where parsing failed
	Msg string
}

// Error returns the message. The glimt package adds its prefix and the
// error's location.
func (e *Error) Error() string {
	return e.Msg
}

// Position returns the 1-based line and byte column of e in src.
func (e *Error) Position(src string) (line, col int) {
	return Position(src, int(e.Pos))
}

// Position returns the 1-based line and byte column of byte offset off in src.
func Position(src string, off int) (line, col int) {
	before := src[:min(max(off, 0), len(src))]
	line = 1 + strings.Count(before, "\n")
	col = len(before) - strings.LastIndexByte(before, '\n')

	return line, col
}

// rawStarts are the leading keywords of statements glimt passes through as *Raw.
var rawStarts = kws(CREATE, ALTER, DROP, TRUNCATE, COMMENT, GRANT, REVOKE, CALL)

// statementStarts are the leading keywords of statements glimt models.
var statementStarts = kws(SELECT, VALUES, TABLE, WITH, INSERT, UPDATE, DELETE, MERGE)

// Parse lexes and parses the single statement in src. Semicolons around it,
// the empty statements Postgres ignores, are allowed. Errors are *Error values.
func Parse(src string) (*Parsed, error) {
	toks, err := lex(src)
	if err != nil {
		return nil, err
	}

	return parseTokens(src, toks)
}

// parseTokens parses the statement in toks, the tokens lex returned for src.
func parseTokens(src string, toks []Token) (*Parsed, error) {
	p := newParser(src, toks)

	for p.acceptKind(SEMICOLON) { // empty statements, which Postgres ignores
	}

	stmt, err := p.parseStatement()
	if err != nil {
		return nil, err
	}

	if err := p.parseEnd(); err != nil {
		return nil, err
	}

	return p.result(stmt), nil
}

// parser holds the state of one Parse call: the input and a cursor into it.
type parser struct {
	src    string
	toks   []Token
	pos    int32   // index of the current token
	last   int32   // index of the EOF token
	params []Param // slab holding every param, in source order
}

// newParser returns a parser positioned on the first token. toks must be
// non-empty and shorter than math.MaxInt32.
func newParser(src string, toks []Token) *parser {
	n := 0
	for i := range toks {
		if toks[i].Kind == PARAM {
			n++
		}
	}

	return &parser{
		src:    src,
		toks:   toks,
		last:   int32(len(toks) - 1), //nolint:gosec // Parse bounds len(toks) by math.MaxInt32
		params: make([]Param, 0, n),
	}
}

// parseStatement parses one statement, including any leading WITH clause.
func (p *parser) parseStatement() (Statement, error) {
	start := p.pos

	var with *With
	if p.atKw(WITH) {
		w, err := p.parseWith()
		if err != nil {
			return nil, err
		}

		with = w
	}

	return p.parseStatementBody(start, with)
}

// parseStatementBody parses the statement after its WITH clause, if any,
// choosing the statement kind by its first token.
func (p *parser) parseStatementBody(start int32, with *With) (Statement, error) {
	switch {
	case p.atKw(INSERT):
		return stmtOf(p.parseInsert(start, with))
	case p.atKw(UPDATE):
		return stmtOf(p.parseUpdate(start, with))
	case p.atKw(DELETE):
		return stmtOf(p.parseDelete(start, with))
	case p.atKw(MERGE):
		return stmtOf(p.parseMerge(start, with))
	case p.atQuery():
		return stmtOf(p.parseQueryWith(start, with))
	case with != nil:
		return nil, p.unexpected("SELECT, VALUES, TABLE, INSERT, UPDATE, DELETE or MERGE")
	case rawStarts.has(p.tok().Kw):
		return stmtOf(p.parseRaw())
	}

	return nil, p.unexpected("a statement")
}

// stmtOf converts a parse result to a Statement, so a failed parse returns a
// nil interface rather than an interface holding a nil pointer.
func stmtOf[T Statement](s T, err error) (Statement, error) {
	if err != nil {
		return nil, err
	}

	return s, nil
}

// parseEnd accepts trailing ";"s, the empty statements Postgres ignores, and
// requires the end of input after them. A statement's first keyword there
// means two statements ran together, as when a "-- name:" line is missing.
func (p *parser) parseEnd() error {
	for p.acceptKind(SEMICOLON) {
	}

	switch kw := p.tok().Kw; {
	case p.atKind(EOF):
		return nil
	case statementStarts.has(kw) || rawStarts.has(kw):
		return p.errorf(`a second statement starts at %s: is a "-- name:" annotation missing or misspelled?`, p.describe())
	}

	return p.unexpected("end of statement")
}

// parseRaw parses a statement glimt doesn't model, up to its end.
func (p *parser) parseRaw() (*Raw, error) {
	body, err := p.scanExpr(noStops)
	if err != nil {
		return nil, err
	}

	return &Raw{Span: body.Span, Body: body}, nil
}

// result assembles the Parsed value for stmt.
func (p *parser) result(stmt Statement) *Parsed {
	r := &Parsed{Src: p.src, Tokens: p.toks, Stmt: stmt}
	if len(p.params) == 0 {
		return r
	}

	r.Params = make([]*Param, len(p.params))
	for i := range p.params {
		r.Params[i] = &p.params[i]
	}

	return r
}

// parseWith parses a WITH clause with the cursor on WITH.
func (p *parser) parseWith() (*With, error) {
	w := &With{Span: Span{From: p.pos}}
	p.advance() // WITH

	// RECURSIVE is the flag only when a CTE name follows: in WITH recursive
	// AS (…), it is the name.
	if next := p.peek(1); p.atKw(RECURSIVE) && next.Kw != AS && next.Kind != LPAREN {
		w.Recursive = true
		p.advance()
	}

	for {
		cte, err := p.parseCTE()
		if err != nil {
			return nil, err
		}

		w.CTEs = append(w.CTEs, cte)
		if !p.acceptKind(COMMA) {
			break
		}
	}

	w.To = p.pos

	return w, nil
}

// parseCTE parses one common table expression: name [(cols)] AS [[NOT] MATERIALIZED] (query).
func (p *parser) parseCTE() (*CTE, error) {
	c := &CTE{Span: Span{From: p.pos}}

	var err error
	if c.Name, err = p.parseIdent("CTE name"); err != nil {
		return nil, err
	}

	if p.atKind(LPAREN) {
		if c.Columns, err = p.parseIdentList("column name"); err != nil {
			return nil, err
		}
	}

	if err := p.expectKw(AS); err != nil {
		return nil, err
	}

	c.Materialized = p.parseMaterialization()

	if c.Body, err = p.parseCTEBody(); err != nil {
		return nil, err
	}

	if c.Search, err = p.parseCTEOption(SEARCH, SET); err != nil {
		return nil, err
	}

	if c.Cycle, err = p.parseCTEOption(CYCLE, USING); err != nil {
		return nil, err
	}

	c.To = p.pos

	return c, nil
}

// parseCTEOption parses an optional SEARCH … SET col or CYCLE … USING col:
// the option keyword kw, a span up to the keyword last, and the column after
// it. The returned span covers everything after kw.
func (p *parser) parseCTEOption(kw, last Keyword) (*Expr, error) {
	if !p.acceptKw(kw) {
		return nil, nil
	}

	e, err := p.requireExpr("expression after "+kw.String(), stopSet{kws: kws(last)})
	if err != nil {
		return nil, err
	}

	if err := p.expectKw(last); err != nil {
		return nil, err
	}

	if _, err := p.parseIdent("column name"); err != nil {
		return nil, err
	}

	e.To = p.pos

	return &e, nil
}

// parseMaterialization parses an optional [NOT] MATERIALIZED.
func (p *parser) parseMaterialization() Materialization {
	switch {
	case p.acceptKw(MATERIALIZED):
		return Materialized
	case p.atKw(NOT) && p.peek(1).Kw == MATERIALIZED:
		p.advance()
		p.advance()

		return NotMaterialized
	}

	return MaterializeDefault
}

// parseCTEBody parses a CTE's parenthesized query. Data-modifying bodies are not supported.
func (p *parser) parseCTEBody() (*Query, error) {
	if err := p.expectKind(LPAREN); err != nil {
		return nil, err
	}

	switch p.tok().Kw {
	case INSERT, UPDATE, DELETE, MERGE:
		return nil, p.unsupported("a data-modifying statement in WITH")
	}

	q, err := p.parseQuery()
	if err != nil {
		return nil, err
	}

	if err := p.expectKind(RPAREN); err != nil {
		return nil, err
	}

	return q, nil
}

// parseIdent parses a quoted identifier, a bare one, or a keyword Postgres
// accepts as a name (see Keyword.colID). what names the identifier in errors.
func (p *parser) parseIdent(what string) (Ident, error) {
	t := p.tok()
	switch {
	case t.Kind == QIDENT:
		id := Ident{Name: unquoteIdent(t.Text(p.src)), Quoted: true, Tok: p.pos}
		p.advance()

		return id, nil
	case t.Kind == IDENT && (t.Kw == 0 || t.Kw.colID()):
		id := Ident{Name: t.Text(p.src), Tok: p.pos}
		p.advance()

		return id, nil
	}

	return Ident{}, p.unexpected(what)
}

// unquoteIdent strips the quotes from a quoted identifier and undoubles its
// inner quotes. It only allocates when the identifier holds a doubled quote.
func unquoteIdent(s string) string {
	s = s[1 : len(s)-1]
	if !strings.Contains(s, `""`) {
		return s
	}

	return strings.ReplaceAll(s, `""`, `"`)
}

// parseIdentList parses a parenthesized, comma-separated list of identifiers.
func (p *parser) parseIdentList(what string) ([]Ident, error) {
	if err := p.expectKind(LPAREN); err != nil {
		return nil, err
	}

	var list []Ident
	for {
		id, err := p.parseIdent(what)
		if err != nil {
			return nil, err
		}

		list = append(list, id)
		if !p.acceptKind(COMMA) {
			break
		}
	}

	if err := p.expectKind(RPAREN); err != nil {
		return nil, err
	}

	return list, nil
}

// parseObjectName parses a possibly qualified name: a[.b[.c]].
func (p *parser) parseObjectName() (ObjectName, error) {
	name := ObjectName{Span: Span{From: p.pos}}
	for {
		id, err := p.parseIdent("table name")
		if err != nil {
			return ObjectName{}, err
		}

		name.Parts = append(name.Parts, id)
		if !p.acceptKind(DOT) {
			break
		}
	}

	name.To = p.pos

	return name, nil
}

// parseTable parses a statement's target table and optional alias. With
// needAS, an alias must follow AS, as in INSERT. Without it AS is optional,
// and a bare alias is any name parseIdent accepts except SET, the only such
// keyword that can follow a target table.
func (p *parser) parseTable(needAS bool) (ObjectName, *Ident, error) {
	if p.atKw(ONLY) {
		return ObjectName{}, nil, p.unsupported("ONLY")
	}

	name, err := p.parseObjectName()
	if err != nil {
		return ObjectName{}, nil, err
	}

	if !needAS {
		p.acceptStar()
	}

	alias, err := p.parseAlias(needAS)
	if err != nil {
		return ObjectName{}, nil, err
	}

	return name, alias, nil
}

// acceptStar skips the "*" Postgres allows after a table name to include the
// tables that inherit from it. That is the default, so nothing records it.
func (p *parser) acceptStar() {
	if t := p.tok(); t.Kind == OP && t.Text(p.src) == "*" {
		p.advance()
	}
}

// parseAlias parses an optional table alias. See parseTable.
func (p *parser) parseAlias(needAS bool) (*Ident, error) {
	t := p.tok()
	bare := t.Kind == QIDENT || t.Kind == IDENT && (t.Kw == 0 || t.Kw.colID()) && t.Kw != SET

	if !p.acceptKw(AS) && (needAS || !bare) {
		return nil, nil
	}

	id, err := p.parseIdent("alias")
	if err != nil {
		return nil, err
	}

	return &id, nil
}

// atQuery reports whether the cursor is at the start of a query.
func (p *parser) atQuery() bool {
	return p.atKind(LPAREN) || p.startsQuery(p.pos)
}

// tok returns the token at the cursor.
func (p *parser) tok() Token {
	return p.toks[p.pos]
}

// tokAt returns token i, or EOF when i is past the end.
func (p *parser) tokAt(i int32) Token {
	if i < 0 || i >= p.last {
		return p.toks[p.last]
	}

	return p.toks[i]
}

// peek returns the token n positions after the cursor.
func (p *parser) peek(n int32) Token {
	return p.tokAt(p.pos + n)
}

// advance moves the cursor to the next token. It never moves past EOF.
func (p *parser) advance() {
	if p.pos < p.last {
		p.pos++
	}
}

// atKw reports whether the token at the cursor is keyword kw.
func (p *parser) atKw(kw Keyword) bool {
	return p.tok().Kw == kw
}

// atKind reports whether the token at the cursor has kind k.
func (p *parser) atKind(k Kind) bool {
	return p.tok().Kind == k
}

// acceptKw consumes the token at the cursor if it is keyword kw.
func (p *parser) acceptKw(kw Keyword) bool {
	if !p.atKw(kw) {
		return false
	}

	p.advance()

	return true
}

// acceptKind consumes the token at the cursor if it has kind k.
func (p *parser) acceptKind(k Kind) bool {
	if !p.atKind(k) {
		return false
	}

	p.advance()

	return true
}

// expectKw consumes keyword kw or reports that it is missing.
func (p *parser) expectKw(kw Keyword) error {
	if !p.acceptKw(kw) {
		return p.unexpected(kw.String())
	}

	return nil
}

// expectKind consumes a token of kind k or reports that it is missing.
func (p *parser) expectKind(k Kind) error {
	if !p.acceptKind(k) {
		return p.unexpected(strconv.Quote(k.String()))
	}

	return nil
}

// unexpected reports that want was expected at the cursor.
func (p *parser) unexpected(want string) error {
	return p.errorf("expected %s, found %s", want, p.describe())
}

// unsupported reports a construct glimt does not support.
func (p *parser) unsupported(what string) error {
	return p.errorf("%s is not supported", what)
}

// errorf returns an *Error at the token at the cursor.
func (p *parser) errorf(format string, args ...any) error {
	return &Error{Pos: p.tok().Pos, Msg: fmt.Sprintf(format, args...)}
}

// describe names the token at the cursor for an error message.
func (p *parser) describe() string {
	t := p.tok()
	if t.Kind == EOF {
		return "end of input"
	}

	return strconv.Quote(t.Text(p.src))
}
