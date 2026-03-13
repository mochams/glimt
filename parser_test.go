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
			name: "duplicate query name within file",
			input: `-- :name listUsers
SELECT * FROM users

-- :name listUsers
SELECT * FROM admins`,
			wantErr: true,
		},
		{
			name: "regular SQL comment in body is preserved",
			input: `-- :name listUsers
-- fetch all active users
SELECT * FROM users WHERE status = ?`,
			want: map[string]string{
				"listUsers": "-- fetch all active users\nSELECT * FROM users WHERE status = ?",
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
			name:  "query before first name ignored",
			input: "SELECT * FROM ignored;\n-- :name Valid\nSELECT 42;",
			want: map[string]string{
				"Valid": "SELECT 42",
			},
		},
		{
			name:    "empty query",
			input:   "-- :name Empty\n\n-- :name Real\nSELECT 1;",
			wantErr: true,
		},
		{
			name:    "query name with spaces is rejected",
			input:   "-- :name create UsersTable\nSELECT 1",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parse(strings.NewReader(tt.input))

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
