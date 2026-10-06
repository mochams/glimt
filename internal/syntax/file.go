package syntax

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// Chunk is one named query in a .sql file: the SQL after a "-- name: x"
// annotation, up to the next annotation or the end of the file.
type Chunk struct {
	Name   string // the name after "-- name:", as written
	Body   string // the query's SQL
	Offset int    // byte offset of Body in the file
	Line   int    // 1-based line of the annotation
	Empty  bool   // Body holds no SQL, only whitespace and comments
}

// annotationPrefix starts the text of an annotation comment.
const annotationPrefix = "name:"

// SplitFile splits the source of a .sql file at its "-- name: x"
// annotations. An annotation is a -- comment that starts a line. The lexer
// finds the comments, so one inside a string, quoted identifier, dollar
// quote or block comment is never an annotation. Only comments may come
// before the first annotation: SQL there, or in a file without one, is an
// error. So is a comment that looks like an annotation but isn't one, such
// as "-- Name: x" or "-- name : x". Errors are *Error values with offsets
// into src.
func SplitFile(src string) ([]Chunk, error) {
	if len(src) >= math.MaxInt32 {
		return nil, &Error{Msg: "source is too long"}
	}

	l := &lexer{src: src, toks: make([]Token, 0, len(src)/3+2), record: true}
	if err := l.run(); err != nil {
		return nil, err
	}

	var (
		chunks []Chunk
		starts []int // where each chunk's annotation starts
	)

	for _, at := range l.comments {
		if !startsLine(src, at) {
			continue
		}

		name, ok, err := annotation(src, at)
		if err != nil {
			return nil, err
		}

		if ok {
			chunks = append(chunks, Chunk{Name: name, Offset: lineEnd(src, at)})
			starts = append(starts, at)
		}
	}

	if err := checkLeading(l.toks, starts); err != nil {
		return nil, err
	}

	fillChunks(src, l.toks, chunks, starts)

	return chunks, nil
}

// checkLeading reports SQL before the first annotation, which starts at
// starts[0], or in a file with no annotation.
func checkLeading(toks []Token, starts []int) error {
	first := toks[0]

	switch {
	case first.Kind == EOF:
		return nil
	case len(starts) == 0:
		return &Error{Pos: first.Pos, Msg: `SQL without a "-- name:" annotation`}
	case int(first.Pos) < starts[0]:
		return &Error{Pos: first.Pos, Msg: `SQL before the first "-- name:" annotation`}
	}

	return nil
}

// fillChunks sets each chunk's body, line and emptiness. A body runs to the
// next annotation, or to the end of src.
func fillChunks(src string, toks []Token, chunks []Chunk, starts []int) {
	for i := range chunks {
		end := len(src)
		if i+1 < len(starts) {
			end = starts[i+1]
		}

		c := &chunks[i]
		c.Body = src[c.Offset:end]
		c.Line, _ = Position(src, starts[i])
		c.Empty = !hasToken(toks, c.Offset, end)
	}
}

// hasToken reports whether a token other than EOF starts in src[from:to].
func hasToken(toks []Token, from, to int) bool {
	i, _ := slices.BinarySearchFunc(toks, from, func(t Token, off int) int { return int(t.Pos) - off })

	return i < len(toks) && toks[i].Kind != EOF && int(toks[i].Pos) < to
}

// startsLine reports whether only spaces and tabs come before src[at] on its line.
func startsLine(src string, at int) bool {
	for i := at - 1; i >= 0; i-- {
		switch src[i] {
		case ' ', '\t':
		case '\n', '\r':
			return true
		default:
			return false
		}
	}

	return true
}

// annotation reads the -- comment at src[at:] as "-- name: x". ok is false
// for any other comment. A comment that starts with "name" and a colon, in
// any case and with any spaces between, but is not exactly one annotation,
// is an error.
func annotation(src string, at int) (name string, ok bool, err error) {
	text := strings.TrimSpace(src[at+2 : lineEnd(src, at)])
	if !nearAnnotation(text) {
		return "", false, nil
	}

	rest, found := strings.CutPrefix(text, annotationPrefix)

	name = strings.TrimSpace(rest)
	if !found || name == "" || strings.ContainsAny(name, " \t") {
		msg := "malformed annotation " + strconv.Quote("-- "+text) + `: want "-- name: <name>"`

		return "", false, &Error{Pos: int32(at), Msg: msg} //nolint:gosec // SplitFile bounds len(src)
	}

	return name, true, nil
}

// nearAnnotation reports whether a comment's text starts with "name" in any
// case, then spaces or tabs, then a colon: an annotation, or a near miss of
// one. "-- name of the customer" is neither.
func nearAnnotation(text string) bool {
	const word = "name"

	if len(text) < len(word) || !strings.EqualFold(text[:len(word)], word) {
		return false
	}

	rest := strings.TrimLeft(text[len(word):], " \t")

	return strings.HasPrefix(rest, ":")
}
