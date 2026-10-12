package syntax

import "strings"

// lexString lexes a string literal whose opening quote is at quote. The
// literal starts at start, which is before quote when it has a prefix such
// as E, B, X, N or U&. Only E'…' strings treat a backslash as an escape.
// As in Postgres, a literal continues in another quoted part after
// whitespace that holds a newline, and the parts form one token.
func (l *lexer) lexString(start, quote int) error {
	escapes := quote-start == 1 && (l.src[start] == 'E' || l.src[start] == 'e')

	end, ok := quoteEnd(l.src, quote, '\'', escapes)
	for ok {
		next, comment := continuation(l.src, end)
		if next < 0 {
			break
		}

		if comment {
			return l.errorf(end, "a comment inside a continued string literal is not supported")
		}

		end, ok = quoteEnd(l.src, next, '\'', escapes)
	}

	if !ok {
		return l.errorf(start, "unterminated string literal")
	}

	l.emit(STRING, start, end)

	return nil
}

// continuation returns the index of the quote that continues a string
// literal ending at s[i], or -1. As in Postgres, the quote must follow
// whitespace and -- comments that hold at least one newline. comment reports
// whether a -- comment came between the parts.
func continuation(s string, i int) (next int, comment bool) {
	newline := false

	for i < len(s) {
		switch c := s[i]; {
		case c == '\n' || c == '\r':
			newline = true
			i++
		case isSpace(c):
			i++
		case c == '-' && byteAt(s, i+1) == '-':
			comment = true
			i = lineEnd(s, i)
		case c == '\'' && newline:
			return i, comment
		default:
			return -1, false
		}
	}

	return -1, false
}

// lexQuotedIdent lexes a "quoted identifier" at the cursor.
func (l *lexer) lexQuotedIdent() error {
	start := l.pos

	end, ok := quoteEnd(l.src, start, '"', false)
	switch {
	case !ok:
		return l.errorf(start, "unterminated quoted identifier")
	case end-start == 2:
		return l.errorf(start, "zero-length quoted identifier")
	}

	l.emit(QIDENT, start, end)

	return nil
}

// lexDollar lexes what starts with "$" at the cursor: a dollar-quoted
// string. A positional placeholder such as $1 is an error.
func (l *lexer) lexDollar() error {
	start := l.pos

	if isDigit(l.peek(1)) {
		end := digitsEnd(l.src, start+1)

		return l.errorf(start, "positional parameter %s: use :name", l.src[start:end])
	}

	tagEnd := dollarTagEnd(l.src, start)
	if tagEnd < 0 {
		return l.errorf(start, `unexpected character "$"`)
	}

	tag := l.src[start:tagEnd]

	i := strings.Index(l.src[tagEnd:], tag)
	if i < 0 {
		return l.errorf(start, "unterminated dollar-quoted string")
	}

	l.emit(STRING, start, tagEnd+i+len(tag))

	return nil
}

// lexNumber lexes a numeric literal at the cursor.
func (l *lexer) lexNumber() {
	l.emit(NUMBER, l.pos, numberEnd(l.src, l.pos))
}

// isStringPrefix reports whether word, directly followed by a quote, prefixes
// a string literal: E'…' (escapes), B'…' (bits) or X'…' (hex bits). As in
// Postgres, N'…' is two tokens, N and the string.
func isStringPrefix(word string) bool {
	if len(word) != 1 {
		return false
	}

	switch word[0] | 0x20 {
	case 'e', 'b', 'x':
		return true
	}

	return false
}

// quoteEnd returns the index just past the quoted text opening with quote q
// at s[i]. A doubled quote is an escaped quote, and with escapes set a
// backslash escapes the next byte. ok is false when the text is unterminated.
func quoteEnd(s string, i int, q byte, escapes bool) (end int, ok bool) {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			if escapes {
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

// dollarTagEnd returns the index just past the opening tag of a dollar quote
// at s[i]: $$ or $tag$, where tag is an identifier that doesn't start with a
// digit and doesn't contain "$". It returns -1 when no tag starts at s[i].
func dollarTagEnd(s string, i int) int {
	for j := i + 1; j < len(s); j++ {
		switch c := s[j]; {
		case c == '$':
			return j + 1
		case !isIdentChar(c) || j == i+1 && isDigit(c):
			return -1
		}
	}

	return -1
}

// numberEnd returns the index just past the numeric literal at s[i]: an
// integer or decimal with an optional exponent, a 0x, 0o or 0b integer, any
// of them with _ separators. The digits are not validated; Postgres does that.
func numberEnd(s string, i int) int {
	if s[i] == '0' && i+2 < len(s) && strings.IndexByte("xXoObB", s[i+1]) >= 0 && isHexDigit(s[i+2]) {
		j := i + 2
		for j < len(s) && (isHexDigit(s[j]) || s[j] == '_') {
			j++
		}

		return j
	}

	j := digitsEnd(s, i)
	if j < len(s) && s[j] == '.' {
		j = digitsEnd(s, j+1)
	}

	return exponentEnd(s, j)
}

// exponentEnd returns the index just past an exponent such as e10 or E-3 at
// s[i], or i when no exponent starts there.
func exponentEnd(s string, i int) int {
	if i >= len(s) || s[i]|0x20 != 'e' {
		return i
	}

	j := i + 1
	if j < len(s) && (s[j] == '+' || s[j] == '-') {
		j++
	}

	if j >= len(s) || !isDigit(s[j]) {
		return i
	}

	return digitsEnd(s, j)
}

// digitsEnd returns the index of the first byte from i on that is neither a
// digit nor an _ separator.
func digitsEnd(s string, i int) int {
	for i < len(s) && (isDigit(s[i]) || s[i] == '_') {
		i++
	}

	return i
}

// isHexDigit reports whether c is a hexadecimal digit.
func isHexDigit(c byte) bool {
	return isDigit(c) || 'a' <= c|0x20 && c|0x20 <= 'f'
}
