package glimt

import (
	"testing"
)

func TestWritePlaceholders(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		dialect Dialect
		want    string
	}{
		// --- DialectMySQL (default / no rewrite) ---
		{
			name:    "mysql: no rewrite",
			sql:     "SELECT * FROM users WHERE id = ?",
			dialect: DialectMySQL,
			want:    "SELECT * FROM users WHERE id = ?",
		},
		{
			name:    "mysql: multiple placeholders unchanged",
			sql:     "SELECT * FROM users WHERE id = ? AND status = ?",
			dialect: DialectMySQL,
			want:    "SELECT * FROM users WHERE id = ? AND status = ?",
		},

		// --- DialectPostgres ---
		{
			name:    "postgres: no placeholders",
			sql:     "SELECT * FROM users",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users",
		},
		{
			name:    "postgres: single placeholder",
			sql:     "SELECT * FROM users WHERE id = ?",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users WHERE id = $1",
		},
		{
			name:    "postgres: multiple placeholders",
			sql:     "SELECT * FROM users WHERE id = ? AND status = ?",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users WHERE id = $1 AND status = $2",
		},
		{
			name:    "postgres: placeholder in IN clause",
			sql:     "SELECT * FROM users WHERE id IN (?, ?, ?)",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users WHERE id IN ($1, $2, $3)",
		},
		{
			name:    "postgres: placeholder in INSERT",
			sql:     "INSERT INTO users (name, email) VALUES (?, ?)",
			dialect: DialectPostgres,
			want:    "INSERT INTO users (name, email) VALUES ($1, $2)",
		},
		{
			name:    "postgres: placeholder in UPDATE",
			sql:     "UPDATE users SET name = ?, email = ? WHERE id = ?",
			dialect: DialectPostgres,
			want:    "UPDATE users SET name = $1, email = $2 WHERE id = $3",
		},

		// --- DialectSQLite (no rewrite) ---
		{
			name:    "sqlite: no rewrite",
			sql:     "SELECT * FROM users WHERE id = ?",
			dialect: DialectSQLite,
			want:    "SELECT * FROM users WHERE id = ?",
		},
		{
			name:    "sqlite: multiple placeholders unchanged",
			sql:     "SELECT * FROM users WHERE id = ? AND status = ?",
			dialect: DialectSQLite,
			want:    "SELECT * FROM users WHERE id = ? AND status = ?",
		},

		// --- string literals ---
		{
			name:    "? inside single quoted string is ignored",
			sql:     "SELECT * FROM users WHERE name = 'what?'",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users WHERE name = 'what?'",
		},
		{
			name:    "? outside string is rewritten, inside is not",
			sql:     "SELECT * FROM users WHERE name = 'what?' AND id = ?",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users WHERE name = 'what?' AND id = $1",
		},
		{
			name:    "escaped single quote inside string literal",
			sql:     "SELECT * FROM users WHERE name = 'it''s?'",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users WHERE name = 'it''s?'",
		},
		{
			name:    "escaped quote followed by real placeholder",
			sql:     "SELECT * FROM users WHERE name = 'it''s?' AND id = ?",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users WHERE name = 'it''s?' AND id = $1",
		},
		{
			name:    "empty string literal",
			sql:     "SELECT * FROM users WHERE name = '' AND id = ?",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users WHERE name = '' AND id = $1",
		},
		{
			name:    "multiple string literals",
			sql:     "SELECT * FROM users WHERE name = 'what?' OR label = 'huh?' AND id = ?",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users WHERE name = 'what?' OR label = 'huh?' AND id = $1",
		},

		// --- double quoted identifiers ---
		{
			name:    "? inside double quoted identifier is ignored",
			sql:     `SELECT * FROM "what?" WHERE id = ?`,
			dialect: DialectPostgres,
			want:    `SELECT * FROM "what?" WHERE id = $1`,
		},

		// --- ?? escape ---
		{
			name:    "postgres: ?? becomes a literal JSONB operator",
			sql:     "SELECT * FROM t WHERE data ?? 'k' AND id = ?",
			dialect: DialectPostgres,
			want:    "SELECT * FROM t WHERE data ? 'k' AND id = $1",
		},
		{
			name:    "postgres: ??| becomes ?|",
			sql:     "SELECT * FROM t WHERE data ??| array['a', 'b'] AND id = ?",
			dialect: DialectPostgres,
			want:    "SELECT * FROM t WHERE data ?| array['a', 'b'] AND id = $1",
		},
		{
			name:    "mysql: ?? becomes a literal question mark",
			sql:     "SELECT ?? AS q, ? AS p",
			dialect: DialectMySQL,
			want:    "SELECT ? AS q, ? AS p",
		},
		{
			name:    "sqlite: ?? inside a string literal is kept",
			sql:     "SELECT '??' WHERE id = ?",
			dialect: DialectSQLite,
			want:    "SELECT '??' WHERE id = ?",
		},

		// --- dialect-aware quoting ---
		{
			name:    "postgres: ? inside a dollar-quoted body is ignored",
			sql:     "SELECT $$it's ?$$, ?",
			dialect: DialectPostgres,
			want:    "SELECT $$it's ?$$, $1",
		},
		{
			name:    "postgres: E-string backslash escape",
			sql:     `SELECT E'it\'s?' AND id = ?`,
			dialect: DialectPostgres,
			want:    `SELECT E'it\'s?' AND id = $1`,
		},
		{
			name:    "mysql: backslash escape keeps ?? inside the literal",
			sql:     `SELECT 'it\'s ??', ??`,
			dialect: DialectMySQL,
			want:    `SELECT 'it\'s ??', ?`,
		},

		// --- edge cases ---
		{
			name:    "empty string",
			sql:     "",
			dialect: DialectPostgres,
			want:    "",
		},
		{
			name:    "no placeholders",
			sql:     "SELECT 1",
			dialect: DialectPostgres,
			want:    "SELECT 1",
		},
		{
			name:    "only a placeholder",
			sql:     "?",
			dialect: DialectPostgres,
			want:    "$1",
		},
		{
			name:    "native $1 in sql is passed through unchanged",
			sql:     "INSERT INTO users VALUES ($1, $2)",
			dialect: DialectPostgres,
			want:    "INSERT INTO users VALUES ($1, $2)",
		},
		{
			name:    "backtick quoted identifier ignored (MySQL style)",
			sql:     "SELECT `column?` FROM users WHERE id = ?",
			dialect: DialectMySQL,
			want:    "SELECT `column?` FROM users WHERE id = ?",
		},
		{
			name:    "placeholder at start of SQL",
			sql:     "? SELECT * FROM users",
			dialect: DialectPostgres,
			want:    "$1 SELECT * FROM users",
		},
		{
			name:    "placeholder at end of SQL",
			sql:     "SELECT * FROM users WHERE id = ?",
			dialect: DialectPostgres,
			want:    "SELECT * FROM users WHERE id = $1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := writePlaceholders(tt.sql, tt.dialect)
			if got != tt.want {
				t.Errorf("\ngot  %q\nwant %q", got, tt.want)
			}
		})
	}
}
