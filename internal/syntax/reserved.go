package syntax

// reservedWords are the words Postgres never reads as a bare column name:
// the RESERVED_KEYWORD and TYPE_FUNC_NAME_KEYWORD entries of its
// src/include/parser/kwlist.h, as of Postgres 17. The integration module
// checks the list against kwlist.h. It is longer than the parser's keyword
// table, which holds only the keywords the parser needs.
var reservedWords = [...]string{
	"all", "analyse", "analyze", "and", "any", "array", "as", "asc", "asymmetric", "authorization",
	"binary", "both", "case", "cast", "check", "collate", "collation", "column", "concurrently",
	"constraint", "create", "cross", "current_catalog", "current_date", "current_role",
	"current_schema", "current_time", "current_timestamp", "current_user", "default", "deferrable",
	"desc", "distinct", "do", "else", "end", "except", "false", "fetch", "for", "foreign", "freeze",
	"from", "full", "grant", "group", "having", "ilike", "in", "initially", "inner", "intersect",
	"into", "is", "isnull", "join", "lateral", "leading", "left", "like", "limit", "localtime",
	"localtimestamp", "natural", "not", "notnull", "null", "offset", "on", "only", "or", "order",
	"outer", "overlaps", "placing", "primary", "references", "returning", "right", "select",
	"session_user", "similar", "some", "symmetric", "system_user", "table", "tablesample", "then", "to",
	"trailing", "true", "union", "unique", "user", "using", "variadic", "verbose", "when", "where",
	"window", "with",
}

// maxReservedLen is the length of the longest reserved word, current_timestamp.
const maxReservedLen = 17

// reservedByLen groups the reserved words by length, for ReservedWord.
var reservedByLen = func() (byLen [maxReservedLen + 1][]string) {
	for _, w := range reservedWords {
		byLen[len(w)] = append(byLen[len(w)], w)
	}

	return byLen
}()

// ReservedWord reports whether Postgres reserves word, in any case, so that
// it can't name a column without quotes: true for words such as order, null,
// true, current_date, to and left.
func ReservedWord(word string) bool {
	if len(word) < 2 || len(word) > maxReservedLen {
		return false
	}

	for _, w := range reservedByLen[len(word)] {
		if equalFoldLower(word, w) {
			return true
		}
	}

	return false
}

// equalFoldLower reports whether text equals lower, a lower-case ASCII name
// of the same length, ignoring case.
func equalFoldLower(text, lower string) bool {
	for i := range len(text) {
		c := text[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}

		if c != lower[i] {
			return false
		}
	}

	return true
}
