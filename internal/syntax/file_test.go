package syntax

import (
	"strings"
	"testing"
)

func TestSplitFile(t *testing.T) {
	src := strings.Join([]string{
		"-- leading comments are allowed",
		"-- name of the customer is in c.name: not an annotation",
		"",
		"-- name: listUsers",
		"-- Lists users.",
		"SELECT * FROM users WHERE org = :org;",
		"",
		"  --name:getUser   ",
		"SELECT * FROM users WHERE id = :id -- name: notAnAnnotation",
		"",
		"-- name: makeFunc",
		"CREATE FUNCTION f() RETURNS int AS $$",
		"-- name: insideDollarQuote",
		"SELECT 1",
		"$$ LANGUAGE sql;",
		"/*",
		"-- name: insideBlockComment",
		"*/",
		"-- name: empty",
		"-- only a comment",
	}, "\n")

	chunks, err := SplitFile(src)
	if err != nil {
		t.Fatal(err)
	}

	want := []struct {
		name  string
		line  int
		empty bool
		body  string // a substring of the body
	}{
		{"listUsers", 4, false, "SELECT * FROM users WHERE org = :org;"},
		{"getUser", 8, false, "-- name: notAnAnnotation"},
		{"makeFunc", 11, false, "-- name: insideBlockComment"},
		{"empty", 19, true, "-- only a comment"},
	}

	if len(chunks) != len(want) {
		t.Fatalf("got %d chunks, want %d: %+v", len(chunks), len(want), chunks)
	}

	for i, w := range want {
		c := chunks[i]
		if c.Name != w.name || c.Line != w.line || c.Empty != w.empty || !strings.Contains(c.Body, w.body) {
			t.Errorf("chunk %d = {%q line %d empty %v %q}, want {%q line %d empty %v containing %q}",
				i, c.Name, c.Line, c.Empty, c.Body, w.name, w.line, w.empty, w.body)
		}

		if src[c.Offset:c.Offset+len(c.Body)] != c.Body {
			t.Errorf("chunk %d: Offset %d does not locate its body", i, c.Offset)
		}
	}
}

func TestSplitFileLineEndings(t *testing.T) {
	chunks, err := SplitFile("-- name: a\r\nSELECT 1\r\n-- name: b\r\nSELECT 2\r\n")
	if err != nil {
		t.Fatal(err)
	}

	if len(chunks) != 2 || chunks[0].Name != "a" || chunks[1].Name != "b" || chunks[1].Line != 3 {
		t.Errorf("chunks = %+v", chunks)
	}

	if _, err := Parse(chunks[0].Body); err != nil {
		t.Errorf("Parse(%q): %v", chunks[0].Body, err)
	}
}

func TestSplitFileNone(t *testing.T) {
	for _, src := range []string{"", "-- only a comment\n", "/* a block */\n\n"} {
		chunks, err := SplitFile(src)
		if err != nil || chunks != nil {
			t.Errorf("SplitFile(%q) = %v, %v; want no chunks", src, chunks, err)
		}
	}
}

func TestSplitFileErrors(t *testing.T) {
	tests := []struct {
		src  string
		want string
		line int
	}{
		{"-- name: a\nSELECT 1\n-- name:\nSELECT 2", `malformed annotation "-- name:": want "-- name: <name>"`, 3},
		{"-- name: two words\nSELECT 1", `malformed annotation "-- name: two words": want "-- name: <name>"`, 1},
		{"-- name: a\nSELECT 'open", "unterminated string literal", 2},
		{"-- Name: first\nSELECT 1\n-- name: second\nSELECT 2",
			`malformed annotation "-- Name: first": want "-- name: <name>"`, 1},
		{"-- name: a\nSELECT 1\n-- name : b\nSELECT 2",
			`malformed annotation "-- name : b": want "-- name: <name>"`, 3},
		{"-- name: a\nSELECT 1\n--NAME: b\nSELECT 2",
			`malformed annotation "-- NAME: b": want "-- name: <name>"`, 3},
		{"SELECT 1\n-- name: a\nSELECT 2", `SQL before the first "-- name:" annotation`, 1},
		{"-- a comment\n\n  SELECT 1;\n-- name: a\nSELECT 2", `SQL before the first "-- name:" annotation`, 3},
		{"SELECT 1", `SQL without a "-- name:" annotation`, 1},
		{"-- just SQL\nSELECT 1; -- no annotations\n", `SQL without a "-- name:" annotation`, 2},
	}

	for _, tt := range tests {
		_, err := SplitFile(tt.src)

		serr, ok := err.(*Error)
		if !ok || serr.Error() != tt.want {
			t.Errorf("SplitFile(%q) error = %v, want %q", tt.src, err, tt.want)

			continue
		}

		if line, _ := serr.Position(tt.src); line != tt.line {
			t.Errorf("SplitFile(%q) error on line %d, want %d", tt.src, line, tt.line)
		}
	}
}

func TestPosition(t *testing.T) {
	src := "ab\ncd\nef"
	for _, tt := range []struct{ off, line, col int }{{0, 1, 1}, {4, 2, 2}, {8, 3, 3}, {99, 3, 3}, {-1, 1, 1}} {
		if line, col := Position(src, tt.off); line != tt.line || col != tt.col {
			t.Errorf("Position(%d) = %d:%d, want %d:%d", tt.off, line, col, tt.line, tt.col)
		}
	}
}
