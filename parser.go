package glimt

import (
	"fmt"
	"strings"
	"unicode"
)

// parseFile splits the contents of a .sql file into named queries and
// sanitizes each body. Queries are introduced by "-- :name <name>" lines;
// text before the first annotation is ignored.
func parseFile(src string, dialect Dialect) (map[string]*template, error) {
	queries := make(map[string]*template)
	l := newLexer(src, dialect, true)

	var (
		name     string
		nameLine int
	)

	flush := func() error {
		tpl := l.take()
		if name == "" {
			return nil
		}

		if tpl.empty() {
			return lineErrorf(nameLine, "query %q has empty body", name)
		}

		if _, exists := queries[name]; exists {
			return lineErrorf(nameLine, "duplicate query name %q", name)
		}

		queries[name] = tpl

		return nil
	}

	l.onName = func(next string, line int) error {
		if !validName(next) {
			return lineErrorf(line, "invalid query name %q", next)
		}

		if err := flush(); err != nil {
			return err
		}

		name, nameLine = next, line

		return nil
	}

	if err := l.run(); err != nil {
		return nil, err
	}

	if err := flush(); err != nil {
		return nil, err
	}

	return queries, nil
}

// parseSQL sanitizes a single SQL statement. In strict mode, SQL that glimt
// would misread is an error; otherwise only unterminated quotes and comments are.
func parseSQL(src string, dialect Dialect, strict bool) (*template, error) {
	l := newLexer(src, dialect, strict)
	if err := l.run(); err != nil {
		return nil, err
	}

	return l.take(), nil
}

// notBlank returns false if a value is an empty string.
func notBlank(value string) bool {
	return strings.TrimSpace(value) != ""
}

// validName checks if a query name is valid (consists of letters, digits,
// and underscores, and does not start with a digit).
func validName(name string) bool {
	if !notBlank(name) {
		return false
	}

	for i, c := range name {
		if !unicode.IsLetter(c) && c != '_' && (i == 0 || !unicode.IsDigit(c)) {
			return false
		}
	}
	return true
}

// template is a sanitized SQL body, produced once when a query is loaded.
// Clause markers split it into parts: slots[i] sits between parts[i] and parts[i+1].
type template struct {
	parts  []string
	params []int // number of ? placeholders in each part
	slots  []slot
}

// empty reports whether the template has no SQL text.
func (t *template) empty() bool {
	for _, part := range t.parts {
		if part != "" {
			return false
		}
	}

	return true
}

// slot is a clause marker in a query, where the WHERE predicates are rendered.
type slot uint8

const (
	slotWhere slot = iota + 1 // /* :where */ renders " WHERE <preds>"
	slotAnd                   // /* :and */ renders " AND <preds>"
)

// slots maps marker names to slots.
var slots = map[string]slot{
	"where": slotWhere,
	"and":   slotAnd,
}

// lineError is a load-time error tied to a line of the source.
type lineError struct {
	line int
	msg  string
}

func (e *lineError) Error() string {
	return fmt.Sprintf("line %d: %s", e.line, e.msg)
}

func lineErrorf(line int, format string, args ...any) error {
	return &lineError{line: line, msg: fmt.Sprintf(format, args...)}
}

// lexer sanitizes SQL in a single pass. It strips comments, collapses
// whitespace outside literals and counts ? placeholders. String literals,
// quoted identifiers, dollar-quoted bodies and optimizer hints are copied verbatim.
//
// In strict mode (used when loading queries) it also handles "-- :name"
// annotations and rejects SQL that glimt would misread. In lenient mode
// (used for ad-hoc SQL) annotations are plain comments.
type lexer struct {
	src     string
	dialect Dialect
	strict  bool
	onName  func(name string, line int) error // "-- :name" handler; nil disallows annotations

	pos       int
	line      int
	lineStart bool // only whitespace and comments since the last newline

	out    []byte
	space  byte // whitespace to emit before the next token: 0, ' ' or '\n'
	lead   bool // emit leading whitespace too: the part follows a marker
	params int

	parts  []string
	counts []int
	slots  []slot
}

func newLexer(src string, dialect Dialect, strict bool) *lexer {
	return &lexer{src: src, dialect: dialect, strict: strict, line: 1, lineStart: true}
}

// run lexes the whole source.
func (l *lexer) run() error {
	for l.pos < len(l.src) {
		if err := l.step(); err != nil {
			return err
		}
	}

	return nil
}

// take returns the body collected so far and resets the output.
// Trailing semicolons are dropped so clauses can be appended.
func (l *lexer) take() *template {
	l.endPart()

	last := len(l.parts) - 1
	l.parts[last] = strings.TrimRight(l.parts[last], "; \n")

	tpl := &template{parts: l.parts, params: l.counts, slots: l.slots}
	l.parts, l.counts, l.slots = nil, nil, nil
	l.space, l.lead = 0, false

	return tpl
}

// endPart closes the current part of the body.
func (l *lexer) endPart() {
	l.parts = append(l.parts, string(l.out))
	l.counts = append(l.counts, l.params)
	l.out, l.params = l.out[:0], 0
}

// step lexes one token.
func (l *lexer) step() error {
	c := l.src[l.pos]

	switch {
	case isSpace(c):
		l.whitespace(c)
	case c == '-' && l.peek(1) == '-':
		return l.dashes()
	case c == '#' && l.dialect == DialectMySQL:
		l.skipLineComment()
	case c == '/' && l.peek(1) == '*':
		return l.blockComment()
	case c == '?':
		l.placeholder()
	default:
		return l.token()
	}

	return nil
}

// token copies quoted text or a single byte.
func (l *lexer) token() error {
	if end, ok := quotedEnd(l.src, l.pos, l.dialect); end > l.pos {
		if !ok {
			return lineErrorf(l.line, "unterminated quoted text")
		}

		l.emit(end)

		return nil
	}

	if l.strict && l.dialect == DialectPostgres && l.src[l.pos] == '$' && isDigit(l.peek(1)) && !l.afterIdent() {
		return lineErrorf(l.line, "native placeholder %q: use ? instead", l.native())
	}

	l.emit(l.pos + 1)

	return nil
}

// emit copies src[pos:end] to the output, preceded by any pending whitespace.
func (l *lexer) emit(end int) {
	if l.space != 0 && (len(l.out) > 0 || l.lead) {
		l.out = append(l.out, l.space)
	}

	s := l.src[l.pos:end]
	l.out = append(l.out, s...)
	l.space, l.lead = 0, false
	l.line += strings.Count(s, "\n")
	l.lineStart = false
	l.pos = end
}

// whitespace collapses a run of whitespace to one space, or one newline if the run has one.
func (l *lexer) whitespace(c byte) {
	switch {
	case c == '\n':
		l.space = '\n'
		l.line++
		l.lineStart = true
	case l.space == 0:
		l.space = ' '
	}

	l.pos++
}

// separate makes a removed comment act as whitespace, so "a/**/b" becomes "a b".
func (l *lexer) separate() {
	if l.space == 0 {
		l.space = ' '
	}
}

// placeholder counts a ? placeholder. "??" is an escaped, literal question mark
// (for Postgres JSONB operators) and is kept for the placeholder rewriter.
func (l *lexer) placeholder() {
	if l.peek(1) == '?' {
		l.emit(l.pos + 2)

		return
	}

	l.params++
	l.emit(l.pos + 1)
}

// dashes handles "--": an annotation at the start of a line, a line comment,
// or, on MySQL, two minus signs when no whitespace follows them.
func (l *lexer) dashes() error {
	if l.lineStart {
		if handled, err := l.annotation(); handled || err != nil {
			return err
		}
	}

	if l.dialect == DialectMySQL && l.peek(2) > ' ' {
		// Two minus signs, not a comment. Keep them apart so the output can
		// never read as a comment, for example once a following comment is removed.
		l.emit(l.pos + 1)
		l.space = ' '

		return nil
	}

	l.skipLineComment()

	return nil
}

// annotation handles a "-- :word" line comment and reports whether it did.
func (l *lexer) annotation() (bool, error) {
	text := strings.TrimLeft(l.restOfLine()[2:], " \t")

	word, arg, ok := cutAnnotation(text)
	if !ok {
		if l.strict && strings.HasPrefix(text, "name:") {
			return true, lineErrorf(l.line, "malformed annotation %q: use \"-- :name <name>\"", strings.TrimSpace(text))
		}

		return false, nil
	}

	if !l.strict {
		return false, nil
	}

	line := l.line
	l.skipLineComment()

	switch {
	case word != "name":
		return true, lineErrorf(line, "unknown annotation %q", ":"+word)
	case l.onName == nil:
		return true, lineErrorf(line, "unexpected annotation %q", ":name")
	default:
		return true, l.onName(strings.TrimSpace(arg), line)
	}
}

// blockComment removes a /* */ comment. Optimizer hints (/*+ */) and MySQL
// executable comments (/*! */) are kept. Postgres block comments nest.
// A comment holding only ":name" is a clause marker.
func (l *lexer) blockComment() error {
	end, ok := blockCommentEnd(l.src, l.pos, l.dialect == DialectPostgres)
	if !ok {
		return lineErrorf(l.line, "unterminated block comment")
	}

	if c := l.peek(2); c == '+' || c == '!' {
		l.emit(end)

		return nil
	}

	if name, ok := markerName(l.src[l.pos+2 : end-2]); ok {
		if handled, err := l.marker(name, end); handled || err != nil {
			return err
		}
	}

	l.line += strings.Count(l.src[l.pos:end], "\n")
	l.pos = end
	l.separate()

	return nil
}

// marker ends the current part at a clause marker and reports whether it did.
// In lenient mode an unknown marker is a plain comment.
func (l *lexer) marker(name string, end int) (bool, error) {
	s, known := slots[name]

	switch {
	case !known && l.strict:
		return true, lineErrorf(l.line, "unknown marker %q", "/* :"+name+" */")
	case !known:
		return false, nil
	}

	l.line += strings.Count(l.src[l.pos:end], "\n")
	l.pos = end
	l.endPart()
	l.slots = append(l.slots, s)

	// Keep the whitespace after the marker, or a space if there is none,
	// so the next part stays separated from the rendered clause.
	l.space, l.lead = ' ', true

	return true, nil
}

// markerName returns name when a block comment's content is exactly ":name".
func markerName(content string) (string, bool) {
	word, rest, ok := cutAnnotation(strings.TrimSpace(content))
	if !ok || rest != "" {
		return "", false
	}

	return word, true
}

// skipLineComment removes a comment up to, but not including, the newline.
func (l *lexer) skipLineComment() {
	l.pos += len(l.restOfLine())
	l.separate()
}

func (l *lexer) restOfLine() string {
	rest := l.src[l.pos:]
	if i := strings.IndexByte(rest, '\n'); i >= 0 {
		return rest[:i]
	}

	return rest
}

func (l *lexer) peek(n int) byte {
	if l.pos+n < len(l.src) {
		return l.src[l.pos+n]
	}

	return 0
}

func (l *lexer) afterIdent() bool {
	return l.pos > 0 && isIdentChar(l.src[l.pos-1])
}

// native returns the "$n" placeholder at pos.
func (l *lexer) native() string {
	end := l.pos + 1
	for end < len(l.src) && isDigit(l.src[end]) {
		end++
	}

	return l.src[l.pos:end]
}

// cutAnnotation splits ":word rest" into word and rest.
func cutAnnotation(text string) (word, rest string, ok bool) {
	if text == "" || text[0] != ':' {
		return "", "", false
	}

	end := 1
	for end < len(text) && (isIdentChar(text[end]) && text[end] != '$') {
		end++
	}

	if end == 1 || isDigit(text[1]) {
		return "", "", false
	}

	return text[1:end], text[end:], true
}

// quotedEnd returns the index just past the string literal, quoted identifier
// or dollar-quoted body that starts at s[i]. It returns i when nothing quoted
// starts there, and len(s) with ok false when the quoted text is unterminated.
func quotedEnd(s string, i int, dialect Dialect) (end int, ok bool) {
	switch c := s[i]; {
	case c == '\'':
		backslash := dialect == DialectMySQL || dialect == DialectPostgres && isEscapeString(s, i)

		return closeQuote(s, i, '\'', backslash)
	case c == '"':
		return closeQuote(s, i, '"', dialect == DialectMySQL)
	case c == '`' && dialect != DialectPostgres:
		return closeQuote(s, i, '`', false)
	case c == '$' && dialect == DialectPostgres:
		return dollarQuoteEnd(s, i)
	}

	return i, true
}

// closeQuote finds the end of the quoted text opening at s[i] with quote q.
// A doubled quote is an escaped quote. With backslash set, a backslash escapes the next byte.
func closeQuote(s string, i int, q byte, backslash bool) (int, bool) {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			if backslash {
				j++
			}
		case q:
			if j+1 < len(s) && s[j+1] == q {
				j++

				continue
			}

			return j + 1, true
		}
	}

	return len(s), false
}

// isEscapeString reports whether the quote at s[i] opens a Postgres E'...' string.
func isEscapeString(s string, i int) bool {
	return i > 0 && (s[i-1] == 'E' || s[i-1] == 'e') && (i < 2 || !isIdentChar(s[i-2]))
}

// dollarQuoteEnd returns the end of a Postgres $tag$...$tag$ body starting at s[i].
// It returns i when s[i] does not start a dollar quote (for example "$1").
func dollarQuoteEnd(s string, i int) (int, bool) {
	if i > 0 && isIdentChar(s[i-1]) {
		return i, true
	}

	j := i + 1
	for j < len(s) && s[j] != '$' {
		if !isIdentChar(s[j]) || j == i+1 && isDigit(s[j]) {
			return i, true
		}

		j++
	}

	if j >= len(s) {
		return i, true
	}

	tag := s[i : j+1]

	k := strings.Index(s[j+1:], tag)
	if k < 0 {
		return len(s), false
	}

	return j + 1 + k + len(tag), true
}

// blockCommentEnd returns the index just past the block comment starting at s[i].
func blockCommentEnd(s string, i int, nested bool) (int, bool) {
	depth := 0

	for j := i; j+1 < len(s); j++ {
		switch {
		case s[j] == '/' && s[j+1] == '*' && (nested || depth == 0):
			depth++
			j++
		case s[j] == '*' && s[j+1] == '/':
			depth--
			j++

			if depth == 0 {
				return j + 1, true
			}
		}
	}

	return len(s), false
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isIdentChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || isDigit(c) || c == '_' || c == '$' || c >= 0x80
}
