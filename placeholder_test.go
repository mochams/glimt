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

		// --- DialectSQLServer ---
		{
			name:    "sqlserver: single placeholder",
			sql:     "SELECT * FROM users WHERE id = ?",
			dialect: DialectSQLServer,
			want:    "SELECT * FROM users WHERE id = @p1",
		},
		{
			name:    "sqlserver: multiple placeholders",
			sql:     "SELECT * FROM users WHERE id = ? AND status = ?",
			dialect: DialectSQLServer,
			want:    "SELECT * FROM users WHERE id = @p1 AND status = @p2",
		},

		// --- DialectOracle ---
		{
			name:    "oracle: single placeholder",
			sql:     "SELECT * FROM users WHERE id = ?",
			dialect: DialectOracle,
			want:    "SELECT * FROM users WHERE id = :1",
		},
		{
			name:    "oracle: multiple placeholders",
			sql:     "SELECT * FROM users WHERE id = ? AND status = ?",
			dialect: DialectOracle,
			want:    "SELECT * FROM users WHERE id = :1 AND status = :2",
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
			name:    "sqlserver: placeholder inside single-quoted string ignored",
			sql:     "SELECT * FROM users WHERE name = 'what?' AND id = ?",
			dialect: DialectSQLServer,
			want:    "SELECT * FROM users WHERE name = 'what?' AND id = @p1",
		},
		{
			name:    "oracle: placeholder inside single-quoted string ignored",
			sql:     "SELECT * FROM users WHERE name = 'what?' AND id = ?",
			dialect: DialectOracle,
			want:    "SELECT * FROM users WHERE name = 'what?' AND id = :1",
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
