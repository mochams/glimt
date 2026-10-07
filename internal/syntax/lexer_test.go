package syntax

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

// mustLex lexes src, failing the test on error.
func mustLex(tb testing.TB, src string) []Token {
	tb.Helper()

	toks, err := lex(src)
	if err != nil {
		tb.Fatal(err)
	}

	return toks
}

// lexCase is one table-driven lexer test. want lists each token but EOF as "KIND text".
type lexCase struct {
	name string
	src  string
	want []string
}

// runLexCases lexes each case's source and compares the tokens. Every
// successful lex must also hold the lexer properties.
func runLexCases(t *testing.T, cases []lexCase) {
	t.Helper()

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			toks, err := lex(tc.src)
			if err != nil {
				t.Fatalf("lex(%q): %v", tc.src, err)
			}

			checkLexed(t, tc.src, toks)

			if got := describeTokens(tc.src, toks); !slices.Equal(got, tc.want) {
				t.Errorf("lex(%q) =\n%q\nwant\n%q", tc.src, got, tc.want)
			}
		})
	}
}

// describeTokens renders each token but EOF as "KIND text".
func describeTokens(src string, toks []Token) []string {
	out := make([]string, 0, len(toks))
	for _, t := range toks[:len(toks)-1] {
		out = append(out, fmt.Sprintf("%v %s", t.Kind, t.Text(src)))
	}

	return out
}

// lexError is one table-driven lexer error test, with the 1-based column of the error.
type lexError struct {
	src string
	msg string
	col int
}

// runLexErrors checks that each source fails to lex with the given message at the given column.
func runLexErrors(t *testing.T, cases []lexError) {
	t.Helper()

	for _, tc := range cases {
		_, err := lex(tc.src)

		var lerr *Error
		if !errors.As(err, &lerr) {
			t.Errorf("lex(%q) error = %v, want *Error %q", tc.src, err, tc.msg)

			continue
		}

		if _, col := lerr.Position(tc.src); lerr.Error() != tc.msg || col != tc.col {
			t.Errorf("lex(%q) error = %q at column %d, want %q at column %d", tc.src, lerr, col, tc.msg, tc.col)
		}
	}
}

// checkLexed verifies the lexer properties for a successful lex: the stream
// meets the lexer contract, and the text between tokens is only whitespace
// and comments, so nothing was dropped that should have been kept.
func checkLexed(tb testing.TB, src string, toks []Token) {
	tb.Helper()

	checkContract(tb, src, toks)

	prev := 0
	for _, t := range toks {
		gap := src[prev:t.Pos]
		if rest, err := lex(gap); err != nil || len(rest) != 1 {
			tb.Errorf("lex(%q): gap %q before %q holds tokens or fails: %v", src, gap, t.Text(src), err)
		}

		prev = int(t.End)
	}
}

// checkContract verifies that toks meets the lexer contract Parse relies on:
// tokens in order within src, exactly one EOF at the end, keywords only on
// identifiers, known separators, and well-formed param and quoted-identifier text.
func checkContract(tb testing.TB, src string, toks []Token) {
	tb.Helper()

	if len(toks) == 0 || toks[len(toks)-1].Kind != EOF {
		tb.Fatalf("lex(%q): the stream does not end with EOF", src)
	}

	prevEnd := int32(0)
	for i, t := range toks {
		if msg := contractFault(src, t, prevEnd, i == len(toks)-1); msg != "" {
			tb.Fatalf("lex(%q): token %d %+v %s", src, i, t, msg)
		}

		prevEnd = t.End
	}
}

// contractFault returns how token t breaks the lexer contract, or "".
func contractFault(src string, t Token, prevEnd int32, last bool) string {
	switch {
	case t.Pos < prevEnd || t.End < t.Pos || int(t.End) > len(src):
		return "is out of order or out of bounds"
	case (t.Kind == EOF) != last:
		return "breaks the rule that exactly one EOF ends the stream"
	case t.Kind > DOT || t.Sep > SepSpace:
		return "has an unknown kind or separator"
	case t.Kw >= numKeywords || t.Kw != 0 && t.Kind != IDENT:
		return "has a keyword but is not an identifier"
	}

	return textFault(t.Text(src), t.Kind)
}

// textFault returns how a token's text breaks the lexer contract, or "".
func textFault(text string, k Kind) string {
	switch {
	case k != EOF && text == "":
		return "is empty"
	case k == PARAM && (len(text) < 2 || text[0] != ':'):
		return `is a param but not ":name"`
	case k == QIDENT && (len(text) < 2 || text[0] != '"' || text[len(text)-1] != '"'):
		return "is a quoted identifier without its quotes"
	}

	return ""
}

func TestLexTokens(t *testing.T) {
	runLexCases(t, []lexCase{
		{"statement", "SELECT id, name FROM users WHERE id = :id;", []string{
			"IDENT SELECT", "IDENT id", ", ,", "IDENT name", "IDENT FROM", "IDENT users",
			"IDENT WHERE", "IDENT id", "OP =", "PARAM :id", "; ;",
		}},
		{"punctuation", "f(a[1]).b", []string{"IDENT f", "( (", "IDENT a", "[ [", "NUMBER 1", "] ]", ") )", ". .", "IDENT b"}},
		{"identifiers", "_x a$b héllo t1", []string{"IDENT _x", "IDENT a$b", "IDENT héllo", "IDENT t1"}},
		{"comments", "SELECT -- note\n a /* x */ , /* multi\nline */ b -- end", []string{
			"IDENT SELECT", "IDENT a", ", ,", "IDENT b",
		}},
		{"nested block comment", "/* a /* b */ c */ x", []string{"IDENT x"}},
		{"carriage return ends a line comment", "a -- c\rb", []string{"IDENT a", "IDENT b"}},
		{"comment as separator", "a/**/b", []string{"IDENT a", "IDENT b"}},
		{"params", ":id :_a1 :A_9", []string{"PARAM :id", "PARAM :_a1", "PARAM :A_9"}},
		{"colon operators", "x::int a := 1 b[1:2] c[: 3]", []string{
			"IDENT x", "OP ::", "IDENT int", "IDENT a", "OP :=", "NUMBER 1",
			"IDENT b", "[ [", "NUMBER 1", "OP :", "NUMBER 2", "] ]", "IDENT c", "[ [", "OP :", "NUMBER 3", "] ]",
		}},
		{"cast then param", "a::text = :b", []string{"IDENT a", "OP ::", "IDENT text", "OP =", "PARAM :b"}},
		{"trailing minus is its own operator", "a=-1", []string{"IDENT a", "OP =", "OP -", "NUMBER 1"}},
		{"trailing plus and minus are trimmed", "a+-b", []string{"IDENT a", "OP +", "OP -", "IDENT b"}},
		{"special character keeps the minus", "a @- b", []string{"IDENT a", "OP @-", "IDENT b"}},
		{"multi-character operators", "a <> b != c ->> 'k' @> :p || d <= e => f", []string{
			"IDENT a", "OP <>", "IDENT b", "OP !=", "IDENT c", "OP ->>", "STRING 'k'", "OP @>",
			"PARAM :p", "OP ||", "IDENT d", "OP <=", "IDENT e", "OP =>", "IDENT f",
		}},
		{"jsonb question operators", "j ? 'a' AND j ?| :ks", []string{
			"IDENT j", "OP ?", "STRING 'a'", "IDENT AND", "IDENT j", "OP ?|", "PARAM :ks",
		}},
		{"operator stops before a comment", "a=--x\n1", []string{"IDENT a", "OP =", "NUMBER 1"}},
		{"operator stops before a block comment", "a*/* x */b", []string{"IDENT a", "OP *", "IDENT b"}},
		{"star", "count(*)", []string{"IDENT count", "( (", "OP *", ") )"}},
		{"empty", "  -- nothing\n", []string{}},
		{"keyword after a dot is a field name", "t.offset t . case public.order x.in", []string{
			"IDENT t", ". .", "IDENT offset", "IDENT t", ". .", "IDENT case", "IDENT public", ". .", "IDENT order",
			"IDENT x", ". .", "IDENT in",
		}},
		{"slice bounds", "a[lo:hi] a[1:n] a[:lo:hi] a[\"q\":2] a[lo :hi] a[f(x):y] a[:i]", []string{
			"IDENT a", "[ [", "IDENT lo", "OP :", "IDENT hi", "] ]",
			"IDENT a", "[ [", "NUMBER 1", "OP :", "IDENT n", "] ]",
			"IDENT a", "[ [", "PARAM :lo", "OP :", "IDENT hi", "] ]",
			"IDENT a", "[ [", `QIDENT "q"`, "OP :", "NUMBER 2", "] ]",
			"IDENT a", "[ [", "IDENT lo", "OP :", "IDENT hi", "] ]",
			"IDENT a", "[ [", "IDENT f", "( (", "IDENT x", ") )", "OP :", "IDENT y", "] ]",
			"IDENT a", "[ [", "PARAM :i", "] ]",
		}},
		{"a param after a word or a space", "THEN:b ELSE:c lo:d a[x] :e x::int", []string{
			"IDENT THEN", "PARAM :b", "IDENT ELSE", "PARAM :c", "IDENT lo", "PARAM :d",
			"IDENT a", "[ [", "IDENT x", "] ]", "PARAM :e", "IDENT x", "OP ::", "IDENT int",
		}},
	})
}

func TestLexKeywordsAndSeparators(t *testing.T) {
	src := "  select\tA /* c */ from\n\nt -- x\n;"

	toks, err := lex(src)
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		kw  Keyword
		sep Sep
	}{
		{SELECT, SepNone}, // the first token never has a separator
		{0, SepSpace},
		{FROM, SepSpace},
		{0, SepSpace},
		{0, SepSpace}, // ;
		{0, SepNone},  // EOF
	}

	if len(toks) != len(want) {
		t.Fatalf("got %d tokens, want %d", len(toks), len(want))
	}

	for i, w := range want {
		if toks[i].Kw != w.kw || toks[i].Sep != w.sep {
			t.Errorf("token %d %q = {%v %v}, want {%v %v}", i, toks[i].Text(src), toks[i].Kw, toks[i].Sep, w.kw, w.sep)
		}
	}
}

func TestLexFieldNamesHaveNoKeyword(t *testing.T) {
	src := "t.offset . case"

	toks := mustLex(t, src)
	for _, i := range []int{2, 4} {
		if toks[i].Kw != 0 {
			t.Errorf("token %q after a dot has keyword %v", toks[i].Text(src), toks[i].Kw)
		}
	}
}

func TestLexErrors(t *testing.T) {
	runLexErrors(t, []lexError{
		{"SELECT /* open", "unterminated block comment", 8},
		{"SELECT /* a /* b */", "unterminated block comment", 8},
		{"WHERE a = :a$b", `parameter ":a$b": names may only hold ASCII letters, digits and _`, 11},
		{"WHERE a = :héllo", `parameter ":héllo": names may only hold ASCII letters, digits and _`, 11},
		{"WHERE a = :é", `parameter ":é": names may only hold ASCII letters, digits and _`, 11},
		{`SELECT U&"d\0061t"`, `U&"…" identifiers are not supported`, 8},
		{"SELECT {a}", `unexpected character "{"`, 8},
		{`SELECT a \ b`, `unexpected character "\\"`, 10},
	})
}

func TestLexErrorLine(t *testing.T) {
	src := "SELECT a\nFROM t\nWHERE b = 'open"

	_, err := lex(src)

	var lerr *Error
	if !errors.As(err, &lerr) {
		t.Fatalf("Lex error = %v, want *Error", err)
	}

	if line, col := lerr.Position(src); line != 3 || col != 11 {
		t.Errorf("Position = %d:%d, want 3:11", line, col)
	}
}

func TestLexFeedsParse(t *testing.T) {
	for _, q := range benchQueries {
		toks, err := lex(q.sql)
		if err != nil {
			t.Fatalf("lex(%s): %v", q.name, err)
		}

		checkLexed(t, q.sql, toks)

		p, err := Parse(q.sql)
		if err != nil {
			t.Fatalf("Parse(%s): %v", q.name, err)
		}

		checkInvariants(t, p)
	}
}

// TestLexAllocatesOnce checks that lexing a typical query allocates only the
// token slice, once: nothing per token.
func TestLexAllocatesOnce(t *testing.T) {
	for _, q := range benchQueries {
		allocs := testing.AllocsPerRun(20, func() {
			if _, err := lex(q.sql); err != nil {
				t.Fatal(err)
			}
		})

		if allocs != 1 {
			t.Errorf("lex(%s) allocates %v times, want 1", q.name, allocs)
		}
	}
}

func FuzzLex(f *testing.F) {
	for _, seed := range []string{
		"SELECT a, b::int FROM t WHERE c IN (:ids) AND d = E'x\\'y' -- c\n",
		"/* a /* b */ */ $f$ body $f$ 'it''s' \"Q\"\"x\" 1.5e-3 0x1F a=-1 j->>'k' ?| @>",
		"'unterminated",
		"$1",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, src string) {
		toks, err := lex(src)
		if err != nil {
			var lerr *Error
			if !errors.As(err, &lerr) || int(lerr.Pos) > len(src) {
				t.Fatalf("lex(%q) error %v is not an *Error within the source", src, err)
			}

			return
		}

		checkLexed(t, src, toks)
	})
}

func BenchmarkLex(b *testing.B) {
	for _, q := range benchQueries {
		b.Run(q.name, func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				if _, err := lex(q.sql); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
