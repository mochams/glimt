package glimt

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
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
			wantErr: true,
		},
		{
			name:    "empty query body",
			input:   "-- :name Empty\n\n-- :name Real\nSELECT 1;",
			wantErr: true,
		},
		{
			name:    "query name with spaces is rejected",
			input:   "-- :name create UsersTable\nSELECT 1",
			wantErr: true,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_parser := newParser()
			err := _parser.parse(strings.NewReader(tt.input))
			got := _parser.queries

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("query count: got %d, want %d", len(got), len(tt.want))
			}

			for name, wantSQL := range tt.want {
				gotSQL, ok := got[name]
				if !ok {
					t.Errorf("query %q not found in result", name)

					continue
				}

				if gotSQL != wantSQL {
					t.Errorf("query %q:\ngot  %q\nwant %q", name, gotSQL, wantSQL)
				}
			}
		})
	}
}
