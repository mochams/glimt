package syntax

// Token is one lexical token. It holds byte offsets into the source rather
// than the text itself, so a token stream allocates no substrings.
type Token struct {
	Pos, End int32   // byte range of the token's text: src[Pos:End]
	Kw       Keyword // the keyword an IDENT spells, or 0
	Kind     Kind    // lexical class
	Sep      Sep     // what separated it from the previous token in the source
}

// Text returns the token's text in src.
func (t Token) Text(src string) string {
	return src[t.Pos:t.End]
}

// Sep records whether anything separated a token from the one before it. The
// separator itself, whitespace or comments, is not kept.
type Sep uint8

// Separators.
const (
	SepNone  Sep = iota // the tokens touch, or the token is the first
	SepSpace            // whitespace or comments
)

// Kind is the lexical class of a token.
type Kind uint8

// Token kinds. The zero Kind is EOF, so a zero Token marks the end of input.
const (
	EOF       Kind = iota // end of input, always the last token
	IDENT                 // unquoted identifier or keyword
	QIDENT                // quoted identifier, quotes included: "Name"
	STRING                // string literal of any form: '…', E'…', $tag$…$tag$
	NUMBER                // numeric literal
	PARAM                 // named parameter: :name
	OP                    // operator: =, <>, ::, ->>, …
	LPAREN                // (
	RPAREN                // )
	LBRACK                // [
	RBRACK                // ]
	COMMA                 // ,
	SEMICOLON             // ;
	DOT                   // .
)

var kindNames = [...]string{
	EOF:       "EOF",
	IDENT:     "IDENT",
	QIDENT:    "QIDENT",
	STRING:    "STRING",
	NUMBER:    "NUMBER",
	PARAM:     "PARAM",
	OP:        "OP",
	LPAREN:    "(",
	RPAREN:    ")",
	LBRACK:    "[",
	RBRACK:    "]",
	COMMA:     ",",
	SEMICOLON: ";",
	DOT:       ".",
}

// String returns the name of the kind, or the punctuation it stands for.
func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}

	return "Kind(?)"
}

// Keyword identifies a keyword the parser needs. Only IDENT tokens carry one.
// The zero Keyword means the token is not a keyword.
type Keyword uint8

// Keywords the parser recognizes. Words outside this set are plain identifiers
// to the parser, even when Postgres treats them as keywords.
const (
	_ Keyword = iota
	ALL
	ALTER
	AND
	AS
	BY
	CALL
	CASE
	COMMENT
	CONFLICT
	CONSTRAINT
	CREATE
	CROSS
	CYCLE
	DEFAULT
	DELETE
	DISTINCT
	DO
	DROP
	END
	EXCEPT
	FETCH
	FOR
	FROM
	GRANT
	GROUP
	HAVING
	IN
	INSERT
	INTERSECT
	INTO
	IS
	JOIN
	LATERAL
	LIMIT
	MATCHED
	MATERIALIZED
	MERGE
	NATURAL
	NOT
	NOTHING
	OFFSET
	ON
	ONLY
	ORDER
	OVERRIDING
	RECURSIVE
	RETURNING
	REVOKE
	ROWS
	SEARCH
	SELECT
	SET
	SOURCE
	SYSTEM
	TABLE
	TARGET
	THEN
	TRUNCATE
	UNION
	UPDATE
	USER
	USING
	VALUE
	VALUES
	WHEN
	WHERE
	WINDOW
	WITH
	WITHIN
	numKeywords
)

var keywordNames = [...]string{
	ALL:          "ALL",
	ALTER:        "ALTER",
	AND:          "AND",
	AS:           "AS",
	BY:           "BY",
	CALL:         "CALL",
	CASE:         "CASE",
	COMMENT:      "COMMENT",
	CONFLICT:     "CONFLICT",
	CONSTRAINT:   "CONSTRAINT",
	CREATE:       "CREATE",
	CROSS:        "CROSS",
	CYCLE:        "CYCLE",
	DEFAULT:      "DEFAULT",
	DELETE:       "DELETE",
	DISTINCT:     "DISTINCT",
	DO:           "DO",
	DROP:         "DROP",
	END:          "END",
	EXCEPT:       "EXCEPT",
	FETCH:        "FETCH",
	FOR:          "FOR",
	FROM:         "FROM",
	GRANT:        "GRANT",
	GROUP:        "GROUP",
	HAVING:       "HAVING",
	IN:           "IN",
	INSERT:       "INSERT",
	INTERSECT:    "INTERSECT",
	INTO:         "INTO",
	IS:           "IS",
	JOIN:         "JOIN",
	LATERAL:      "LATERAL",
	LIMIT:        "LIMIT",
	MATCHED:      "MATCHED",
	MATERIALIZED: "MATERIALIZED",
	MERGE:        "MERGE",
	NATURAL:      "NATURAL",
	NOT:          "NOT",
	NOTHING:      "NOTHING",
	OFFSET:       "OFFSET",
	ON:           "ON",
	ONLY:         "ONLY",
	ORDER:        "ORDER",
	OVERRIDING:   "OVERRIDING",
	RECURSIVE:    "RECURSIVE",
	RETURNING:    "RETURNING",
	REVOKE:       "REVOKE",
	ROWS:         "ROWS",
	SEARCH:       "SEARCH",
	SELECT:       "SELECT",
	SET:          "SET",
	SOURCE:       "SOURCE",
	SYSTEM:       "SYSTEM",
	TABLE:        "TABLE",
	TARGET:       "TARGET",
	THEN:         "THEN",
	TRUNCATE:     "TRUNCATE",
	UNION:        "UNION",
	UPDATE:       "UPDATE",
	USER:         "USER",
	USING:        "USING",
	VALUE:        "VALUE",
	VALUES:       "VALUES",
	WHEN:         "WHEN",
	WHERE:        "WHERE",
	WINDOW:       "WINDOW",
	WITH:         "WITH",
	WITHIN:       "WITHIN",
}

// String returns the keyword in upper case.
func (k Keyword) String() string {
	if k > 0 && k < numKeywords {
		return keywordNames[k]
	}

	return "Keyword(?)"
}

// KeywordCategory is how Postgres restricts a keyword's use as a name, as in
// its src/include/parser/kwlist.h.
type KeywordCategory uint8

// Keyword categories, from least to most restricted.
const (
	UnreservedKeyword   KeywordCategory = iota + 1 // usable as any name
	ColNameKeyword                                 // usable as a column or table name, not a function or type
	TypeFuncNameKeyword                            // usable as a function or type name, not a column
	ReservedKeyword                                // usable only as a column label after AS, or quoted
)

// keywordCategories holds each keyword's Postgres category.
var keywordCategories = [...]KeywordCategory{
	ALL:          ReservedKeyword,
	ALTER:        UnreservedKeyword,
	AND:          ReservedKeyword,
	AS:           ReservedKeyword,
	BY:           UnreservedKeyword,
	CALL:         UnreservedKeyword,
	CASE:         ReservedKeyword,
	COMMENT:      UnreservedKeyword,
	CONFLICT:     UnreservedKeyword,
	CONSTRAINT:   ReservedKeyword,
	CREATE:       ReservedKeyword,
	CROSS:        TypeFuncNameKeyword,
	CYCLE:        UnreservedKeyword,
	DEFAULT:      ReservedKeyword,
	DELETE:       UnreservedKeyword,
	DISTINCT:     ReservedKeyword,
	DO:           ReservedKeyword,
	DROP:         UnreservedKeyword,
	END:          ReservedKeyword,
	EXCEPT:       ReservedKeyword,
	FETCH:        ReservedKeyword,
	FOR:          ReservedKeyword,
	FROM:         ReservedKeyword,
	GRANT:        ReservedKeyword,
	GROUP:        ReservedKeyword,
	HAVING:       ReservedKeyword,
	IN:           ReservedKeyword,
	INSERT:       UnreservedKeyword,
	INTERSECT:    ReservedKeyword,
	INTO:         ReservedKeyword,
	IS:           TypeFuncNameKeyword,
	JOIN:         TypeFuncNameKeyword,
	LATERAL:      ReservedKeyword,
	LIMIT:        ReservedKeyword,
	MATCHED:      UnreservedKeyword,
	MATERIALIZED: UnreservedKeyword,
	MERGE:        UnreservedKeyword,
	NATURAL:      TypeFuncNameKeyword,
	NOT:          ReservedKeyword,
	NOTHING:      UnreservedKeyword,
	OFFSET:       ReservedKeyword,
	ON:           ReservedKeyword,
	ONLY:         ReservedKeyword,
	ORDER:        ReservedKeyword,
	OVERRIDING:   UnreservedKeyword,
	RECURSIVE:    UnreservedKeyword,
	RETURNING:    ReservedKeyword,
	REVOKE:       UnreservedKeyword,
	ROWS:         UnreservedKeyword,
	SEARCH:       UnreservedKeyword,
	SELECT:       ReservedKeyword,
	SET:          UnreservedKeyword,
	SOURCE:       UnreservedKeyword,
	SYSTEM:       UnreservedKeyword,
	TABLE:        ReservedKeyword,
	TARGET:       UnreservedKeyword,
	THEN:         ReservedKeyword,
	TRUNCATE:     UnreservedKeyword,
	UNION:        ReservedKeyword,
	UPDATE:       UnreservedKeyword,
	USER:         ReservedKeyword,
	USING:        ReservedKeyword,
	VALUE:        UnreservedKeyword,
	VALUES:       ColNameKeyword,
	WHEN:         ReservedKeyword,
	WHERE:        ReservedKeyword,
	WINDOW:       ReservedKeyword,
	WITH:         ReservedKeyword,
	WITHIN:       UnreservedKeyword,
}

// Category returns k's Postgres keyword category, or 0 when k is not a keyword.
func (k Keyword) Category() KeywordCategory {
	if k == 0 || k >= numKeywords {
		return 0
	}

	return keywordCategories[k]
}

// reserved reports whether Postgres reserves k. Every clause terminator is
// reserved, so a column name can never end a clause by accident.
func (k Keyword) reserved() bool {
	return k.Category() == ReservedKeyword
}

// colID reports whether k can name a column, table or alias without quotes,
// as Postgres's ColId allows: unreserved and column-name keywords.
func (k Keyword) colID() bool {
	c := k.Category()

	return c == UnreservedKeyword || c == ColNameKeyword
}

// maxKeywordLen is the length of the longest keyword, MATERIALIZED.
const maxKeywordLen = 12

// keywordsByLen groups the keywords by the length of their names, for LookupKeyword.
var keywordsByLen = func() (byLen [maxKeywordLen + 1][]Keyword) {
	for k := Keyword(1); k < numKeywords; k++ {
		n := len(keywordNames[k])
		byLen[n] = append(byLen[n], k)
	}

	return byLen
}()

// LookupKeyword returns the keyword text spells, ignoring case, or 0 if text is
// not a keyword. It does not allocate.
func LookupKeyword(text string) Keyword {
	if len(text) < 2 || len(text) > maxKeywordLen {
		return 0
	}

	for _, k := range keywordsByLen[len(text)] {
		if equalFoldUpper(text, keywordNames[k]) {
			return k
		}
	}

	return 0
}

// equalFoldUpper reports whether text equals upper, an upper-case ASCII
// letters-only name of the same length, ignoring case.
func equalFoldUpper(text, upper string) bool {
	for i := range len(text) {
		if c := text[i]; c != upper[i] && c != upper[i]+('a'-'A') {
			return false
		}
	}

	return true
}

// kwSet is a set of keywords, one bit per Keyword.
type kwSet [2]uint64

// This fails to compile if the keywords outgrow kwSet.
var _ [128 - numKeywords]struct{}

// kws returns the set holding the given keywords.
func kws(list ...Keyword) kwSet {
	var s kwSet

	return s.with(list...)
}

// with returns s with the given keywords added.
func (s kwSet) with(list ...Keyword) kwSet {
	for _, k := range list {
		s[k>>6] |= 1 << (k & 63)
	}

	return s
}

// without returns s with the given keywords removed.
func (s kwSet) without(list ...Keyword) kwSet {
	for _, k := range list {
		s[k>>6] &^= 1 << (k & 63)
	}

	return s
}

// has reports whether k is in s. The zero Keyword is never in a set.
func (s kwSet) has(k Keyword) bool {
	return k != 0 && k < numKeywords && s[k>>6]&(1<<(k&63)) != 0
}
