package syntax

import (
	"fmt"
	"math"
	"strings"
)

// lex splits the SQL of one query into tokens that meet the lexer contract
// described in the package documentation. It drops whitespace and comments,
// recording what separated each token in Token.Sep. Errors are *Error values.
func lex(src string) ([]Token, error) {
	if len(src) >= math.MaxInt32 {
		return nil, &Error{Msg: "source is too long"}
	}

	// Real queries run about 3.5 to 4.5 bytes per token, so this one allocation
	// usually holds them all.
	l := &lexer{src: src, toks: make([]Token, 0, len(src)/3+2)}
	if err := l.run(); err != nil {
		return nil, err
	}

	return l.toks, nil
}

// run lexes the whole source, ending with an EOF token.
func (l *lexer) run() error {
	for {
		if err := l.skipSeparators(); err != nil {
			return err
		}

		if l.pos == len(l.src) {
			break
		}

		if err := l.lexToken(); err != nil {
			return err
		}
	}

	l.emit(EOF, len(l.src), len(l.src))

	return nil
}

// lexer holds the state of one lex call.
type lexer struct {
	src      string
	pos      int     // byte offset of the next unread byte
	sep      Sep     // what separated the next token from the previous one
	brackets int     // how many "[" are open
	toks     []Token // tokens so far
	comments []int   // where each -- comment starts, when record is set
	record   bool    // record -- comments, for SplitFile
}

// skipSeparators skips whitespace and comments, noting them in l.sep.
func (l *lexer) skipSeparators() error {
	for l.pos < len(l.src) {
		c := l.src[l.pos]
		switch {
		case isSpace(c):
			l.sep = SepSpace
			l.pos++
		case c == '-' && l.peek(1) == '-':
			if l.record {
				l.comments = append(l.comments, l.pos)
			}

			l.sep = SepSpace
			l.pos = lineEnd(l.src, l.pos)
		case c == '/' && l.peek(1) == '*':
			end, ok := blockCommentEnd(l.src, l.pos)
			if !ok {
				return l.errorf(l.pos, "unterminated block comment")
			}

			l.sep = SepSpace
			l.pos = end
		default:
			return nil
		}
	}

	return nil
}

// lexToken lexes the token at the cursor, which is not a separator.
func (l *lexer) lexToken() error {
	switch c := l.src[l.pos]; {
	case c == '\'':
		return l.lexString(l.pos, l.pos)
	case c == '"':
		return l.lexQuotedIdent()
	case c == '$':
		return l.lexDollar()
	case isDigit(c) || c == '.' && isDigit(l.peek(1)):
		l.lexNumber()

		return nil
	case isIdentStart(c):
		return l.lexWord()
	}

	return l.lexSymbol()
}

// lexWord lexes an identifier or keyword at the cursor, or a string literal
// whose prefix the word turns out to be, such as E'…' or U&'…'.
func (l *lexer) lexWord() error {
	start := l.pos
	end := identEnd(l.src, start)
	word := l.src[start:end]

	switch next := byteAt(l.src, end); {
	case next == '\'' && isStringPrefix(word):
		return l.lexString(start, end)
	case word == "U" || word == "u":
		if next == '&' && byteAt(l.src, end+1) == '\'' {
			return l.lexString(start, end+1)
		}

		if next == '&' && byteAt(l.src, end+1) == '"' {
			return l.errorf(start, `U&"…" identifiers are not supported`)
		}
	}

	l.emit(IDENT, start, end)

	return nil
}

// lexSymbol lexes a param, an operator or punctuation at the cursor.
func (l *lexer) lexSymbol() error {
	c := l.src[l.pos]
	if c == ':' {
		return l.lexColon()
	}

	if k := punctuation(c); k != EOF {
		l.emit(k, l.pos, l.pos+1)

		return nil
	}

	if isOpChar(c) {
		l.emit(OP, l.pos, operatorEnd(l.src, l.pos))

		return nil
	}

	return l.errorf(l.pos, "unexpected character %q", string(c))
}

// lexColon lexes what starts with ":" at the cursor: the operators ::, :=
// and :, or a :name param, whose name must be ASCII letters, digits and _.
// Inside brackets, a ":" after a name, number or param is the array slice
// operator, so arr[lo:hi] holds no param.
func (l *lexer) lexColon() error {
	start := l.pos

	switch next := l.peek(1); {
	case next == ':' || next == '=':
		l.emit(OP, start, start+2)
	case l.atSliceBound():
		l.emit(OP, start, start+1) // arr[lo:hi]
	case isParamStart(next):
		end := paramEnd(l.src, start+1)
		if isIdentChar(byteAt(l.src, end)) {
			return l.errorf(start, "parameter %q: names may only hold ASCII letters, digits and _",
				l.src[start:identEnd(l.src, start+1)])
		}

		l.emit(PARAM, start, end)
	case next >= 0x80:
		return l.errorf(start, "parameter %q: names may only hold ASCII letters, digits and _",
			l.src[start:identEnd(l.src, start+1)])
	default:
		l.emit(OP, start, start+1)
	}

	return nil
}

// emit appends a token of kind k covering src[start:end] and moves the cursor to end.
func (l *lexer) emit(k Kind, start, end int) {
	t := Token{Pos: int32(start), End: int32(end), Kind: k, Sep: l.sep} //nolint:gosec // lex bounds len(src)
	if len(l.toks) == 0 {
		t.Sep = SepNone
	}

	switch {
	case k == LBRACK:
		l.brackets++
	case k == RBRACK && l.brackets > 0:
		l.brackets--
	}

	if k == IDENT && !l.after(DOT) {
		t.Kw = LookupKeyword(l.src[start:end]) // after a dot, every word is a field name
	}

	l.toks = append(l.toks, t)
	l.sep = SepNone
	l.pos = end
}

// after reports whether the last token emitted has kind k.
func (l *lexer) after(k Kind) bool {
	n := len(l.toks)

	return n > 0 && l.toks[n-1].Kind == k
}

// atSliceBound reports whether a ":" at the cursor separates array slice
// bounds: it is inside brackets and follows a complete operand, with or
// without space: a bare identifier that is not a keyword, a quoted
// identifier, a number, a param, ")" or "]". Outside brackets, and right
// after "[", ":name" is a param, so THEN:x and arr[:i] keep theirs.
func (l *lexer) atSliceBound() bool {
	n := len(l.toks)
	if n == 0 || l.brackets == 0 {
		return false
	}

	switch prev := l.toks[n-1]; prev.Kind {
	case IDENT:
		return prev.Kw == 0
	case QIDENT, NUMBER, PARAM, RPAREN, RBRACK:
		return true
	}

	return false
}

// errorf returns an *Error at byte offset at.
func (l *lexer) errorf(at int, format string, args ...any) error {
	return &Error{Pos: int32(at), Msg: fmt.Sprintf(format, args...)} //nolint:gosec // lex bounds len(src)
}

// peek returns the byte n positions after the cursor, or 0 past the end.
func (l *lexer) peek(n int) byte {
	return byteAt(l.src, l.pos+n)
}

// byteAt returns s[i], or 0 when i is past the end.
func byteAt(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}

	return 0
}

// punctuation returns the kind of punctuation byte c, or EOF if c is none.
func punctuation(c byte) Kind {
	switch c {
	case '(':
		return LPAREN
	case ')':
		return RPAREN
	case '[':
		return LBRACK
	case ']':
		return RBRACK
	case ',':
		return COMMA
	case ';':
		return SEMICOLON
	case '.':
		return DOT
	}

	return EOF
}

// operatorEnd returns the index just past the operator at s[i], following
// Postgres: an operator stops before "--" or "/*", and one of several bytes
// can't end in + or - unless it holds one of ~ ! @ # % ^ & | ` ?. So "=-1"
// is "=" then "-" then "1".
func operatorEnd(s string, i int) int {
	j := i + 1
	for j < len(s) && isOpChar(s[j]) && !startsComment(s, j) {
		j++
	}

	if strings.ContainsAny(s[i:j], "~!@#%^&|`?") {
		return j
	}

	for j-i > 1 && (s[j-1] == '+' || s[j-1] == '-') {
		j--
	}

	return j
}

// startsComment reports whether a comment starts at s[i].
func startsComment(s string, i int) bool {
	next := byteAt(s, i+1)

	return s[i] == '-' && next == '-' || s[i] == '/' && next == '*'
}

// lineEnd returns the index of the newline ending the line at s[i], or
// len(s). As in Postgres, "\r" ends a line as well as "\n".
func lineEnd(s string, i int) int {
	if n := strings.IndexAny(s[i:], "\n\r"); n >= 0 {
		return i + n
	}

	return len(s)
}

// blockCommentEnd returns the index just past the block comment opening at
// s[i]. Block comments nest, as in Postgres. ok is false when it is unterminated.
func blockCommentEnd(s string, i int) (end int, ok bool) {
	depth := 0
	for j := i; j+1 < len(s); j++ {
		switch {
		case s[j] == '/' && s[j+1] == '*':
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

// identEnd returns the index just past the identifier starting at s[i].
func identEnd(s string, i int) int {
	j := i + 1
	for j < len(s) && isIdentChar(s[j]) {
		j++
	}

	return j
}

// paramEnd returns the index just past the param name starting at s[i].
func paramEnd(s string, i int) int {
	j := i + 1
	for j < len(s) && (isParamStart(s[j]) || isDigit(s[j])) {
		j++
	}

	return j
}

// isSpace reports whether c is whitespace.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// isDigit reports whether c is a decimal digit.
func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

// isIdentStart reports whether c can start an identifier: a letter, _ or any
// non-ASCII byte, as in Postgres.
func isIdentStart(c byte) bool {
	return 'a' <= c|0x20 && c|0x20 <= 'z' || c == '_' || c >= 0x80
}

// isIdentChar reports whether c can continue an identifier.
func isIdentChar(c byte) bool {
	return isIdentStart(c) || isDigit(c) || c == '$'
}

// isParamStart reports whether c can start a param name: an ASCII letter or _.
func isParamStart(c byte) bool {
	return 'a' <= c|0x20 && c|0x20 <= 'z' || c == '_'
}

// isOpChar reports whether c can be part of an operator.
func isOpChar(c byte) bool {
	return strings.IndexByte("+-*/<>=~!@#%^&|`?", c) >= 0
}
