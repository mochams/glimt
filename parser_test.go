package glimt

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect // zero value is DialectPostgres
		input   string
		want    map[string]string
		wantErr string // exact error message; empty when no error is expected
	}{
		{
			name:  "empty file",
			input: "",
			want:  map[string]string{},
		},
		{
			name:  "file with no annotations",
			input: "SELECT * FROM users",
			want:  map[string]string{},
		},
		{
			name: "single query",
			input: `-- :name listUsers
SELECT * FROM users`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name: "single multiline query",
			input: `-- :name listUsers
SELECT *
FROM users
WHERE deleted_at IS NULL`,
			want: map[string]string{
				"listUsers": "SELECT *\nFROM users\nWHERE deleted_at IS NULL",
			},
		},
		{
			name: "multiple queries",
			input: `-- :name listUsers
SELECT * FROM users

-- :name getUserByID
SELECT * FROM users WHERE id = ?`,
			want: map[string]string{
				"listUsers":   "SELECT * FROM users",
				"getUserByID": "SELECT * FROM users WHERE id = ?",
			},
		},
		{
			name: "multiple multiline queries",
			input: `-- :name listUsers
SELECT *
FROM users
WHERE deleted_at IS NULL

-- :name listActiveOrders
SELECT *
FROM orders
WHERE status = ?
AND deleted_at IS NULL`,
			want: map[string]string{
				"listUsers":        "SELECT *\nFROM users\nWHERE deleted_at IS NULL",
				"listActiveOrders": "SELECT *\nFROM orders\nWHERE status = ?\nAND deleted_at IS NULL",
			},
		},
		{
			name: "blank lines between queries are ignored",
			input: `-- :name listUsers
SELECT * FROM users

-- :name listOrders
SELECT * FROM orders`,
			want: map[string]string{
				"listUsers":  "SELECT * FROM users",
				"listOrders": "SELECT * FROM orders",
			},
		},
		{
			name:  "query before first annotation is ignored",
			input: "SELECT * FROM ignored;\n-- :name Valid\nSELECT 42;",
			want: map[string]string{
				"Valid": "SELECT 42",
			},
		},
		{
			name: "annotation with extra whitespace",
			input: `   -- :name listUsers
SELECT * FROM users`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name: "query name with leading and trailing spaces",
			input: `-- :name   listUsers
SELECT * FROM users`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name:  "windows line endings",
			input: "-- :name GetUser\r\nSELECT * FROM users;\r\n",
			want: map[string]string{
				"GetUser": "SELECT * FROM users",
			},
		},
		{
			name: "trailing semicolon is stripped",
			input: `-- :name listUsers
SELECT * FROM users;`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name: "duplicate query name within file",
			input: `-- :name listUsers
SELECT * FROM users

-- :name listUsers
SELECT * FROM admins`,
			wantErr: "?",
		},
		{
			name:    "empty query body",
			input:   "-- :name Empty\n\n-- :name Real\nSELECT 1;",
			wantErr: "?",
		},
		{
			name:    "query name with spaces is rejected",
			input:   "-- :name create UsersTable\nSELECT 1",
			wantErr: "?",
		},
		{
			name: "standalone line comment before query is skipped",
			input: `-- :name listUsers
-- fetch all active users
SELECT * FROM users WHERE status = ?`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users WHERE status = ?",
			},
		},
		{
			name: "multiple standalone line comments are skipped",
			input: `-- :name listUsers
-- first comment
-- second comment
SELECT * FROM users`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name: "inline block comment is stripped",
			input: `-- :name listUsers
SELECT * FROM /* block comment */ users
WHERE status = ?`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users\nWHERE status = ?",
			},
		},
		{
			name: "multiple inline block comments on one line",
			input: `-- :name listUsers
SELECT /* col1 */ id, /* col2 */ name FROM users`,
			want: map[string]string{
				"listUsers": "SELECT id, name FROM users",
			},
		},
		{
			name: "block comment between every token",
			input: `-- :name listUsers
SELECT /* c1 */ * /* c2 */ FROM /* c3 */ users /* c4 */ WHERE /* c5 */ id /* c6 */ = /* c7 */ ?`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users WHERE id = ?",
			},
		},
		{
			name: "consecutive block comments no space between",
			input: `-- :name listUsers
SELECT /* a *//* b */ * FROM users`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name: "placeholder inside inline block comment is stripped",
			input: `-- :name listUsers
SELECT * FROM users /* WHERE status = ? */ WHERE id = ?`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users WHERE id = ?",
			},
		},
		{
			name: "multi-line block comment is skipped",
			input: `-- :name listUsers
/*
  fetch all active users
*/
SELECT * FROM users`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name: "multi-line block comment with SQL keywords inside is stripped",
			input: `-- :name listUsers
SELECT * FROM users
/*
  WHERE deleted_at IS NULL
  AND status = 'active'
*/
WHERE id = ?`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users\nWHERE id = ?",
			},
		},
		{
			name: "standalone one-line block comment between SQL lines",
			input: `-- :name listUsers
SELECT *
/* this is a comment */
FROM users
WHERE id = ?`,
			want: map[string]string{
				"listUsers": "SELECT *\nFROM users\nWHERE id = ?",
			},
		},
		{
			name: "inline line comment at end of line is stripped",
			input: `-- :name listUsers
SELECT * FROM users -- fetch all users`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name: "inline line comment with placeholder before it",
			input: `-- :name listUsers
SELECT * FROM users WHERE status = ? -- filter by status`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users WHERE status = ?",
			},
		},
		{
			name: "multiple lines each with inline line comment",
			input: `-- :name listUsers
SELECT * -- select all
FROM users -- from users table
WHERE status = ? -- filter by status`,
			want: map[string]string{
				"listUsers": "SELECT *\nFROM users\nWHERE status = ?",
			},
		},
		{
			name: "one-line block comment on its own line is skipped",
			input: `-- :name listUsers
/* one-line block comment */
SELECT * FROM users
WHERE id = ?`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users\nWHERE id = ?",
			},
		},
		{
			name: "block comment at start of line with SQL after",
			input: `-- :name listUsers
/* comment */ SELECT * FROM users`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name: "block comment at end of line",
			input: `-- :name listUsers
SELECT * FROM users /* trailing comment */`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name: "block comment and line comment on separate lines",
			input: `-- :name listUsers
/* block comment */
SELECT * FROM users -- line comment
WHERE status = ?`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users\nWHERE status = ?",
			},
		},
		{
			name: "block comment inline and line comment on same line",
			input: `-- :name listUsers
SELECT /* block */ * FROM users -- line comment`,
			want: map[string]string{
				"listUsers": "SELECT * FROM users",
			},
		},
		{
			name: "multiple queries with comments",
			input: `-- :name listUsers
/* fetch all users */
SELECT * FROM users -- active only

-- :name listProducts
SELECT /* all cols */ * FROM products`,
			want: map[string]string{
				"listUsers":    "SELECT * FROM users",
				"listProducts": "SELECT * FROM products",
			},
		},

		// --- quote-aware comment stripping ---
		{
			name:  "-- inside a string literal is kept",
			input: "-- :name q\nSELECT * FROM t WHERE note = '--not a comment'",
			want:  map[string]string{"q": "SELECT * FROM t WHERE note = '--not a comment'"},
		},
		{
			name:  "/* inside a string literal is kept",
			input: "-- :name q\nSELECT * FROM t WHERE path LIKE '/*%'\nAND id = ?",
			want:  map[string]string{"q": "SELECT * FROM t WHERE path LIKE '/*%'\nAND id = ?"},
		},
		{
			name:  "whitespace inside a string literal is preserved",
			input: "-- :name q\nSELECT * FROM t   WHERE name = 'a   b'",
			want:  map[string]string{"q": "SELECT * FROM t WHERE name = 'a   b'"},
		},
		{
			name:  "multi-line string literal is preserved",
			input: "-- :name q\nSELECT 'line one\n    line two' AS s",
			want:  map[string]string{"q": "SELECT 'line one\n    line two' AS s"},
		},
		{
			name:  "trailing semicolon inside a literal is kept",
			input: "-- :name q\nSELECT ';';",
			want:  map[string]string{"q": "SELECT ';'"},
		},
		{
			name:  "removed comment separates tokens",
			input: "-- :name q\nSELECT a/**/FROM t",
			want:  map[string]string{"q": "SELECT a FROM t"},
		},
		{
			name:  "optimizer hint is kept",
			input: "-- :name q\nSELECT /*+ INDEX(t idx) */ * FROM t",
			want:  map[string]string{"q": "SELECT /*+ INDEX(t idx) */ * FROM t"},
		},
		{
			name:    "mysql executable comment is kept",
			dialect: DialectMySQL,
			input:   "-- :name q\nSELECT /*!40001 SQL_NO_CACHE */ * FROM t",
			want:    map[string]string{"q": "SELECT /*!40001 SQL_NO_CACHE */ * FROM t"},
		},
		{
			name:  "double-quoted identifier with comment markers is kept",
			input: "-- :name q\nSELECT \"a--b\" FROM t",
			want:  map[string]string{"q": "SELECT \"a--b\" FROM t"},
		},

		// --- postgres ---
		{
			name:  "postgres: nested block comments",
			input: "-- :name q\nSELECT /* a /* b */ c */ 1",
			want:  map[string]string{"q": "SELECT 1"},
		},
		{
			name:  "postgres: dollar-quoted body is kept verbatim",
			input: "-- :name q\nCREATE FUNCTION f() RETURNS int AS $$\n  SELECT 1 -- inner comment\n$$ LANGUAGE sql",
			want:  map[string]string{"q": "CREATE FUNCTION f() RETURNS int AS $$\n  SELECT 1 -- inner comment\n$$ LANGUAGE sql"},
		},
		{
			name:  "postgres: tagged dollar quote",
			input: "-- :name q\nSELECT $fn$it's -- here$fn$",
			want:  map[string]string{"q": "SELECT $fn$it's -- here$fn$"},
		},
		{
			name:  "postgres: E-string backslash escape",
			input: "-- :name q\nSELECT E'it\\'s -- here' -- comment",
			want:  map[string]string{"q": "SELECT E'it\\'s -- here'"},
		},
		{
			name:  "postgres: backslash is literal in a standard string",
			input: "-- :name q\nSELECT 'C:\\' -- comment",
			want:  map[string]string{"q": "SELECT 'C:\\'"},
		},
		{
			name:  "postgres: # is an operator, not a comment",
			input: "-- :name q\nSELECT 5 # 3",
			want:  map[string]string{"q": "SELECT 5 # 3"},
		},
		{
			name:  "postgres: escaped ?? is kept for the rewriter",
			input: "-- :name q\nSELECT * FROM t WHERE data ?? 'k' AND id = ?",
			want:  map[string]string{"q": "SELECT * FROM t WHERE data ?? 'k' AND id = ?"},
		},
		{
			name:    "postgres: native placeholder is rejected",
			input:   "-- :name q\nSELECT * FROM t\nWHERE id = $1",
			wantErr: "line 3: native placeholder \"$1\": use ? instead",
		},

		// --- mysql ---
		{
			name:    "mysql: # starts a comment",
			dialect: DialectMySQL,
			input:   "-- :name q\nSELECT 1 # note",
			want:    map[string]string{"q": "SELECT 1"},
		},
		{
			name:    "mysql: -- without a following space is not a comment",
			dialect: DialectMySQL,
			input:   "-- :name q\nSELECT 1--1",
			want:    map[string]string{"q": "SELECT 1- -1"},
		},
		{
			name:    "mysql: backslash escape in a string",
			dialect: DialectMySQL,
			input:   "-- :name q\nSELECT 'it\\'s -- here' -- comment",
			want:    map[string]string{"q": "SELECT 'it\\'s -- here'"},
		},
		{
			name:    "mysql: block comments do not nest",
			dialect: DialectMySQL,
			input:   "-- :name q\nSELECT /* a /* b */ 1",
			want:    map[string]string{"q": "SELECT 1"},
		},
		{
			name:    "mysql: backtick identifier is kept",
			dialect: DialectMySQL,
			input:   "-- :name q\nSELECT `a--b` FROM t",
			want:    map[string]string{"q": "SELECT `a--b` FROM t"},
		},
		{
			name:    "sqlite: backtick identifier is kept",
			dialect: DialectSQLite,
			input:   "-- :name q\nSELECT `a--b` FROM t",
			want:    map[string]string{"q": "SELECT `a--b` FROM t"},
		},

		// --- annotations ---
		{
			name:  "annotation without a space after the dashes",
			input: "-- :name a\nSELECT 1\n--:name b\nSELECT 2",
			want:  map[string]string{"a": "SELECT 1", "b": "SELECT 2"},
		},
		{
			name:  "annotation separated by a tab",
			input: "--\t:name a\nSELECT 1",
			want:  map[string]string{"a": "SELECT 1"},
		},
		{
			name:  "annotation-like text after SQL is a plain comment",
			input: "-- :name a\nSELECT 1 -- :name b",
			want:  map[string]string{"a": "SELECT 1"},
		},
		{
			name:  "annotation inside a block comment is ignored",
			input: "-- :name a\nSELECT 1\n/*\n-- :name b\n*/",
			want:  map[string]string{"a": "SELECT 1"},
		},
		{
			name:    "unknown annotation is rejected",
			input:   "-- :name a\nSELECT 1\n-- :query b\nSELECT 2",
			wantErr: "line 3: unknown annotation \":query\"",
		},
		{
			name:    "sqlc-style annotation is rejected",
			input:   "-- :name a\nSELECT 1\n-- name: b\nSELECT 2",
			wantErr: "line 3: malformed annotation \"name: b\": use \"-- :name <name>\"",
		},

		// --- clause markers ---
		{
			name:  "where marker splits the body",
			input: "-- :name q\nSELECT * FROM t /* :where */ GROUP BY a",
			want:  map[string]string{"q": "SELECT * FROM t{where} GROUP BY a"},
		},
		{
			name:  "and marker after a fixed WHERE keeps the newline after it",
			input: "-- :name q\nSELECT * FROM t\nWHERE deleted_at IS NULL /* :and */\nORDER BY id",
			want:  map[string]string{"q": "SELECT * FROM t\nWHERE deleted_at IS NULL{and}\nORDER BY id"},
		},
		{
			name:  "marker without spaces or a following space",
			input: "-- :name q\nSELECT * FROM t/*:where*/GROUP BY a",
			want:  map[string]string{"q": "SELECT * FROM t{where} GROUP BY a"},
		},
		{
			name:  "marker at the end of the query",
			input: "-- :name q\nSELECT * FROM t WHERE a = 1 /* :and */;",
			want:  map[string]string{"q": "SELECT * FROM t WHERE a = 1{and}"},
		},
		{
			name:  "repeated markers",
			input: "-- :name q\nSELECT id FROM a /* :where */\nUNION ALL\nSELECT id FROM b /* :where */",
			want:  map[string]string{"q": "SELECT id FROM a{where}\nUNION ALL\nSELECT id FROM b{where}"},
		},
		{
			name:  "comment that is not only a marker is a plain comment",
			input: "-- :name q\nSELECT * FROM t /* :where filters go here */ GROUP BY a",
			want:  map[string]string{"q": "SELECT * FROM t GROUP BY a"},
		},
		{
			name:  "marker text inside a string literal is kept",
			input: "-- :name q\nSELECT '/* :where */' FROM t",
			want:  map[string]string{"q": "SELECT '/* :where */' FROM t"},
		},
		{
			name:    "unknown marker is rejected",
			input:   "-- :name q\nSELECT * FROM t\n/* :orderby */",
			wantErr: "line 3: unknown marker \"/* :orderby */\"",
		},
		{
			name:    "query with only a marker is empty",
			input:   "-- :name q\n/* :where */",
			wantErr: "line 1: query \"q\" has empty body",
		},

		// --- errors carry line numbers ---
		{
			name:    "duplicate name reports its line",
			input:   "-- :name a\nSELECT 1\n\n-- :name a\nSELECT 2",
			wantErr: "line 4: duplicate query name \"a\"",
		},
		{
			name:    "unterminated string literal",
			input:   "-- :name a\nSELECT 'oops\nFROM t",
			wantErr: "line 2: unterminated quoted text",
		},
		{
			name:    "unterminated block comment",
			input:   "-- :name a\nSELECT 1\n/* never closed\nFROM t",
			wantErr: "line 3: unterminated block comment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFile(tt.input, tt.dialect)

			if tt.wantErr != "" {
				assertParseError(t, err, tt.wantErr)

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			assertQueries(t, got, tt.want)
		})
	}
}

// renderTemplate joins a template's parts, showing clause markers as {where} and {and}.
func renderTemplate(tpl *template) string {
	var sb strings.Builder

	for i, part := range tpl.parts {
		sb.WriteString(part)

		if i < len(tpl.slots) {
			sb.WriteString(map[slot]string{slotWhere: "{where}", slotAnd: "{and}"}[tpl.slots[i]])
		}
	}

	return sb.String()
}

// sumParams returns the number of ? placeholders across all parts.
func sumParams(tpl *template) int {
	n := 0
	for _, c := range tpl.params {
		n += c
	}

	return n
}

// assertParseError checks err against want; "?" accepts any error.
func assertParseError(t *testing.T, err error, want string) {
	t.Helper()

	switch {
	case err == nil:
		t.Error("expected error, got nil")
	case want != "?" && err.Error() != want:
		t.Errorf("error:\ngot  %q\nwant %q", err.Error(), want)
	}
}

// assertQueries checks that got holds exactly the queries in want.
func assertQueries(t *testing.T, got map[string]*template, want map[string]string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("query count: got %d, want %d", len(got), len(want))
	}

	for name, wantSQL := range want {
		tpl, ok := got[name]
		if !ok {
			t.Errorf("query %q not found in result", name)

			continue
		}

		if gotSQL := renderTemplate(tpl); gotSQL != wantSQL {
			t.Errorf("query %q:\ngot  %q\nwant %q", name, gotSQL, wantSQL)
		}
	}
}

func TestParse_placeholderCount(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"none", "SELECT 1", 0},
		{"two placeholders", "SELECT * FROM t WHERE a = ? AND b = ?", 2},
		{"escaped ?? is not a placeholder", "SELECT * FROM t WHERE data ?? 'k' AND id = ?", 1},
		{"? inside a literal is not a placeholder", "SELECT '?' FROM t WHERE id = ?", 1},
		{"? inside a comment is not a placeholder", "SELECT * FROM t /* id = ? */ WHERE id = ?", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl, err := parseSQL(tt.input, DialectPostgres, true)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got := sumParams(tpl); got != tt.want {
				t.Errorf("params: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParse_longLine(t *testing.T) {
	// A single line longer than bufio.Scanner's 64KB default must still load.
	values := strings.Repeat("(1), ", 20000) + "(1)"
	input := "-- :name bulk\nINSERT INTO t (a) VALUES " + values

	got, err := parseFile(input, DialectPostgres)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want, gotSQL := "INSERT INTO t (a) VALUES "+values, renderTemplate(got["bulk"]); gotSQL != want {
		t.Errorf("long query was not preserved (got %d bytes, want %d)", len(gotSQL), len(want))
	}
}

func TestParseSQL_lenient(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"trailing line comment is stripped", "SELECT * FROM docs -- all docs", "SELECT * FROM docs"},
		{"trailing semicolon is stripped", "SELECT * FROM docs;", "SELECT * FROM docs"},
		{"annotations are plain comments", "-- :name x\nSELECT 1", "SELECT 1"},
		{"native placeholders are passed through", "SELECT * FROM t WHERE id = $1", "SELECT * FROM t WHERE id = $1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl, err := parseSQL(tt.input, DialectPostgres, false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got := renderTemplate(tpl); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParse_placeholdersPerPart(t *testing.T) {
	tpl, err := parseSQL("SELECT * FROM t WHERE org_id = ? /* :and */ GROUP BY a HAVING COUNT(*) > ? AND MAX(b) < ?", DialectPostgres, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(tpl.params) != 2 || tpl.params[0] != 1 || tpl.params[1] != 2 {
		t.Errorf("params per part: got %v, want [1 2]", tpl.params)
	}
}

func TestParseSQL_lenientUnknownMarker(t *testing.T) {
	tpl, err := parseSQL("SELECT * FROM t /* :orderby */ LIMIT 1", DialectPostgres, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := renderTemplate(tpl); got != "SELECT * FROM t LIMIT 1" {
		t.Errorf("got %q", got)
	}
}

func TestValidator_validName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid name", "getUserByID", true},
		{"name with spaces", "get user by id", false},
		{"name with special chars", "get-user-by-id!", false},
		{"empty name", "", false},
		{"name with only spaces", "   ", false},
		{"name with underscores", "get_user_by_id", true},
		{"name starting with underscore", "_getUserByID", true},
		{"name with leading digit", "9getUserByID", false},
		{"name with trailing digit", "getUserByID9", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validName(tt.input); got != tt.want {
				t.Errorf("validName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
