package syntax

import "testing"

func TestLexLiterals(t *testing.T) {
	runLexCases(t, []lexCase{
		{"string", `'abc'`, []string{`STRING 'abc'`}},
		{"doubled quote", `'it''s'`, []string{`STRING 'it''s'`}},
		{"backslash is plain in a standard string", `'a\' b`, []string{`STRING 'a\'`, "IDENT b"}},
		{"escape string", `E'a\'b'`, []string{`STRING E'a\'b'`}},
		{"lower-case escape string", `e'\\' x`, []string{`STRING e'\\'`, "IDENT x"}},
		{"bit strings", `B'0101' X'1F'`, []string{`STRING B'0101'`, `STRING X'1F'`}},
		{"national string is two tokens", `N'x'`, []string{"IDENT N", `STRING 'x'`}},
		{"unicode string", `U&'d\0061t' u&'x'`, []string{`STRING U&'d\0061t'`, `STRING u&'x'`}},
		{"prefix needs to touch the quote", `E 'x'`, []string{"IDENT E", `STRING 'x'`}},
		{"word is not a prefix", `xyz'a'`, []string{"IDENT xyz", `STRING 'a'`}},
		{"continued string is one token", "'ab'\n  'cd' x", []string{"STRING 'ab'\n  'cd'", "IDENT x"}},
		{"continuation keeps escapes", "E'a'\n'\\''", []string{"STRING E'a'\n'\\''"}},
		{"continuation needs a newline", "'ab' 'cd'", []string{`STRING 'ab'`, `STRING 'cd'`}},
		{"block comment ends a continuation", "'a' /* c */\n'b'", []string{`STRING 'a'`, `STRING 'b'`}},
		{"dollar quotes don't continue", "$$a$$\n'b'", []string{`STRING $$a$$`, `STRING 'b'`}},
		{"dollar quote", `$$it's$$`, []string{`STRING $$it's$$`}},
		{"tagged dollar quote", `$fn$ a $x$ b $fn$ c`, []string{`STRING $fn$ a $x$ b $fn$`, "IDENT c"}},
		{"empty dollar body", `$_t1$$_t1$`, []string{`STRING $_t1$$_t1$`}},
		{"quoted identifier", `"Name" "a""b"`, []string{`QIDENT "Name"`, `QIDENT "a""b"`}},
		{"integers and decimals", "42 1.5 .5 1.", []string{"NUMBER 42", "NUMBER 1.5", "NUMBER .5", "NUMBER 1."}},
		{"exponents", "1e10 1.5E-3 2e+1", []string{"NUMBER 1e10", "NUMBER 1.5E-3", "NUMBER 2e+1"}},
		{"bare e is not an exponent", "1e x", []string{"NUMBER 1", "IDENT e", "IDENT x"}},
		{"prefixed integers", "0x1F 0o17 0b101 0XaB", []string{"NUMBER 0x1F", "NUMBER 0o17", "NUMBER 0b101", "NUMBER 0XaB"}},
		{"prefix without digits", "0x", []string{"NUMBER 0", "IDENT x"}},
		{"separators", "1_000_000 0x_FF", []string{"NUMBER 1_000_000", "NUMBER 0", "IDENT x_FF"}},
		{"two decimals", "1.5.2", []string{"NUMBER 1.5", "NUMBER .2"}},
	})
}

func TestLexLiteralErrors(t *testing.T) {
	runLexErrors(t, []lexError{
		{`SELECT 'abc`, "unterminated string literal", 8},
		{`SELECT E'abc\'`, "unterminated string literal", 8},
		{`SELECT "abc`, "unterminated quoted identifier", 8},
		{`SELECT ""`, "zero-length quoted identifier", 8},
		{"SELECT 'a' -- c\n'b'", "a comment inside a continued string literal is not supported", 11},
		{"SELECT 'a'\n'b", "unterminated string literal", 8},
		{`SELECT $$abc`, "unterminated dollar-quoted string", 8},
		{`SELECT $t$abc$x$`, "unterminated dollar-quoted string", 8},
		{"WHERE id = $12", "positional parameter $12: use :name", 12},
		{"SELECT $ 1", `unexpected character "$"`, 8},
		{"SELECT $a b$", `unexpected character "$"`, 8},
	})
}

func TestQuoteEnd(t *testing.T) {
	tests := []struct {
		s       string
		escapes bool
		end     int
		ok      bool
	}{
		{`'ab'`, false, 4, true},
		{`'a''b'`, false, 6, true},
		{`'a\'b'`, false, 4, true},
		{`'a\'b'`, true, 6, true},
		{`'ab`, false, 3, false},
		{`'a\`, true, 3, false},
	}

	for _, tt := range tests {
		if end, ok := quoteEnd(tt.s, 0, '\'', tt.escapes); end != tt.end || ok != tt.ok {
			t.Errorf("quoteEnd(%q, %v) = %d, %v, want %d, %v", tt.s, tt.escapes, end, ok, tt.end, tt.ok)
		}
	}
}

func TestDollarTagEnd(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"$$", 2},
		{"$tag$", 5},
		{"$_1$", 4},
		{"$1$", -1},
		{"$a b$", -1},
		{"$abc", -1},
		{"$", -1},
	}

	for _, tt := range tests {
		if got := dollarTagEnd(tt.s, 0); got != tt.want {
			t.Errorf("dollarTagEnd(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}
