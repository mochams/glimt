package glimt

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// Helpers

func assertErrorContains(t *testing.T, err error, wantSubstring string) {
	t.Helper()

	if err == nil {
		t.Errorf("expected error containing %q, got nil", wantSubstring)

		return
	}

	if !strings.EqualFold(err.Error(), wantSubstring) {
		t.Errorf("unexpected error message: got %q, want substring %q", err.Error(), wantSubstring)
	}
}

// Tests

func TestRegistry_GetUnknownQuery(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	_, err := reg.Get("nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	assertErrorContains(t, err, `glimt: query not found: "nonexistent"`)
}

func TestRegistry_MustGetPanicsOnUnknown(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for unknown query, got none")
		}
	}()

	reg := NewRegistry(DialectPostgres)
	reg.MustGet("nonexistent")
}

func TestRegistry_AdHocQuery(t *testing.T) {
	reg := NewRegistry(DialectPostgres)
	sql, args := reg.Query("SELECT * FROM users").
		Where(Eq("status", "active")).
		Build()

	assertSQL(t, sql, "SELECT * FROM users WHERE status = $1")
	assertArgs(t, args, []any{"active"})
}

func TestRegistry_LoadFile(t *testing.T) {
	reg := NewRegistry(DialectPostgres)
	if err := reg.LoadFile("testdata/queries/users.sql"); err != nil {
		t.Fatalf("failed to load queries: %v", err)
	}

	gotSQL, err := reg.Get("insertUser")
	if err != nil {
		t.Fatalf("failed to get query: %v", err)
	}

	sql, _ := gotSQL.Build()
	wantSQL := "INSERT INTO users (name, email, status, age)\nVALUES ($1, $2, $3, $4)\nRETURNING id"
	assertSQL(t, sql, wantSQL)
}

func TestRegistry_LoadDuplicateFile(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFile("testdata/invalid_queries/duplicate_names.sql")
	if err == nil {
		t.Fatal("expected error for duplicate query names, got nil")
	}

	errMsg := "glimt: parse testdata/invalid_queries/duplicate_names.sql: line 4: duplicate query name \"listUsers\""
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadNonexistentFile(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFile("testdata/queries/nonexistent.sql")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}

	errMsg := "glimt: open testdata/queries/nonexistent.sql: open nonexistent.sql: no such file or directory"
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadMalformedAnnotation(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFile("testdata/invalid_queries/wrong_names.sql")

	errMsg := "glimt: parse testdata/invalid_queries/wrong_names.sql: line 1: " +
		"malformed annotation \"name: createProductsTable\": use \"-- :name <name>\""
	assertErrorContains(t, err, errMsg)

	if len(reg.Queries()) != 0 {
		t.Errorf("expected no queries loaded, got %d", len(reg.Queries()))
	}
}

func TestRegistry_LoadEmptyFile(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFile("testdata/queries/empty.sql")
	if err != nil {
		t.Fatal("unexpected error loading file:", err)
	}

	if len(reg.Queries()) != 0 {
		t.Errorf("expected no queries loaded, got %d", len(reg.Queries()))
	}
}

func TestRegistry_LoadDir(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.Load("testdata/queries")
	if err != nil {
		t.Fatalf("failed to load queries: %v", err)
	}

	expectedQueries := []string{
		"countUsersByStatus",
		"createOrdersTable",
		"createProductsTable",
		"createUsersTable",
		"dropOrdersTable",
		"dropProductsTable",
		"dropUsersTable",
		"insertOrder",
		"insertProduct",
		"insertUser",
		"listActiveUsers",
		"listOrders",
		"listProducts",
		"listUsers",
		"softDeleteOrder",
		"updateOrderStatus",
		"updateProductStock",
	}
	gotQueries := reg.Queries()

	if len(gotQueries) != len(expectedQueries) {
		t.Fatalf("expected %d queries, got %d", len(expectedQueries), len(gotQueries))
	}

	for i, want := range expectedQueries {
		if gotQueries[i] != want {
			t.Errorf("query[%d]: got %q, want %q", i, gotQueries[i], want)
		}
	}
}

func TestRegistry_LoadNonExistentFolder(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.Load("testdata/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}

	errMsg := "glimt: read dir testdata/nonexistent: stat testdata/nonexistent: no such file or directory"
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadEmptyDir(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.Load("testdata/empty_queries")
	if err != nil {
		t.Fatal("unexpected error loading empty directory:", err)
	}

	if len(reg.Queries()) != 0 {
		t.Errorf("expected no queries loaded, got %d", len(reg.Queries()))
	}
}

func TestRegistry_LoadInvalidQueries(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.Load("testdata/invalid_queries")
	if err == nil {
		t.Fatal("expected error for invalid queries, got nil")
	}

	errMsg := "glimt: parse testdata/invalid_queries/duplicate_names.sql: line 4: duplicate query name \"listUsers\""
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadFileFS(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFileFS(os.DirFS("testdata/queries"), "users.sql")
	if err != nil {
		t.Fatalf("failed to load queries from FS: %v", err)
	}

	gotSQL, err := reg.Get("insertUser")
	if err != nil {
		t.Fatalf("failed to get query: %v", err)
	}

	sql, _ := gotSQL.Build()
	wantSQL := "INSERT INTO users (name, email, status, age)\nVALUES ($1, $2, $3, $4)\nRETURNING id"
	assertSQL(t, sql, wantSQL)
}

func TestRegistry_LoadFileFSNonexistentFile(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFileFS(os.DirFS("testdata/queries"), "nonexistent.sql")
	if err == nil {
		t.Fatal("expected error for nonexistent file in FS, got nil")
	}

	errMsg := "glimt: open nonexistent.sql: open nonexistent.sql: no such file or directory"
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadFileFSDuplicate(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFileFS(os.DirFS("testdata/invalid_queries"), "duplicate_names.sql")
	if err == nil {
		t.Fatal("expected error for duplicate query names in FS, got nil")
	}

	errMsg := "glimt: parse duplicate_names.sql: line 4: duplicate query name \"listUsers\""
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadFS(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFS(os.DirFS("testdata/queries"), ".")
	if err != nil {
		t.Fatalf("failed to walk FS: %v", err)
	}

	expectedQueries := []string{
		"countUsersByStatus",
		"createOrdersTable",
		"createProductsTable",
		"createUsersTable",
		"dropOrdersTable",
		"dropProductsTable",
		"dropUsersTable",
		"insertOrder",
		"insertProduct",
		"insertUser",
		"listActiveUsers",
		"listOrders",
		"listProducts",
		"listUsers",
		"softDeleteOrder",
		"updateOrderStatus",
		"updateProductStock",
	}
	gotQueries := reg.Queries()

	if len(gotQueries) != len(expectedQueries) {
		t.Fatalf("expected %d queries, got %d", len(expectedQueries), len(gotQueries))
	}

	for i, want := range expectedQueries {
		if gotQueries[i] != want {
			t.Errorf("query[%d]: got %q, want %q", i, gotQueries[i], want)
		}
	}
}

func TestRegistry_LoadFSNonexistentDir(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFS(os.DirFS("testdata/queries"), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent directory in FS, got nil")
	}

	errMsg := "glimt: read dir nonexistent: stat nonexistent: no such file or directory"
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadFSInvalidQueries(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFS(os.DirFS("testdata/invalid_queries"), ".")
	if err == nil {
		t.Fatal("expected error for invalid queries in FS, got nil")
	}

	errMsg := "glimt: parse duplicate_names.sql: line 4: duplicate query name \"listUsers\""
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadFSEmptyDir(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFS(os.DirFS("testdata/empty_queries"), ".")
	if err != nil {
		t.Fatal("unexpected error walking empty directory in FS:", err)
	}

	if len(reg.Queries()) != 0 {
		t.Errorf("expected no queries loaded, got %d", len(reg.Queries()))
	}
}

func TestRegistry_Has(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFile("testdata/queries/users.sql")
	if err != nil {
		t.Fatalf("failed to load queries: %v", err)
	}

	if !reg.Has("insertUser") {
		t.Error("expected Has to return true for existing query, got false")
	}

	if reg.Has("nonexistent") {
		t.Error("expected Has to return false for unknown query, got true")
	}
}

func TestRegistry_Queries(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFile("testdata/queries/users.sql")
	if err != nil {
		t.Fatalf("failed to load queries: %v", err)
	}

	gotQueries := reg.Queries()
	expectedQueries := []string{
		"countUsersByStatus", "createUsersTable", "dropUsersTable", "insertUser", "listActiveUsers", "listUsers",
	}

	if len(gotQueries) != len(expectedQueries) {
		t.Fatalf("expected %d queries, got %d", len(expectedQueries), len(gotQueries))
	}

	for i, want := range expectedQueries {
		if gotQueries[i] != want {
			t.Errorf("query[%d]: got %q, want %q", i, gotQueries[i], want)
		}
	}
}

func TestRegistry_DynamicFiltering(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.Load("testdata/queries")
	if err != nil {
		t.Fatalf("failed to load queries: %v", err)
	}

	sql, args := reg.MustGet("listUsers").
		Where(Eq("status", "active")).
		Where(Gt("created_at", "2023-01-01")).
		OrderBy("created_at DESC").
		Limit(10).
		Build()

	wantSQL := "SELECT * FROM users WHERE status = $1 AND created_at > $2 ORDER BY created_at DESC LIMIT $3"
	assertSQL(t, sql, wantSQL)
	assertArgs(t, args, []any{"active", "2023-01-01", 10})
}

func TestRegistry_Add(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.Add("activeUsers", "SELECT * FROM users -- only active\nWHERE deleted_at IS NULL;")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sql, args := reg.MustGet("activeUsers").Limit(5).Build()
	assertSQL(t, sql, "SELECT * FROM users\nWHERE deleted_at IS NULL LIMIT $1")
	assertArgs(t, args, []any{5})
}

func TestRegistry_AddErrors(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		sql     string
		wantErr string
	}{
		{"invalid name", "bad name", "SELECT 1", `glimt: add: invalid query name "bad name"`},
		{"empty body", "empty", "  -- nothing here\n", "glimt: add empty: empty query body"},
		{"duplicate", "existing", "SELECT 2", `glimt: duplicate query name "existing" in Add`},
		{"native placeholder", "native", "SELECT * FROM t WHERE id = $1", `glimt: add native: line 1: native placeholder "$1": use ? instead`},
		{"annotation", "annotated", "-- :name other\nSELECT 1", `glimt: add annotated: line 1: unexpected annotation ":name"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := NewRegistry(DialectPostgres)
			if err := reg.Add("existing", "SELECT 1"); err != nil {
				t.Fatalf("setup: %v", err)
			}

			assertErrorContains(t, reg.Add(tt.query, tt.sql), tt.wantErr)
		})
	}
}

func TestRegistry_LoadIsAllOrNothing(t *testing.T) {
	reg := NewRegistry(DialectPostgres)
	if err := reg.Add("listUsers", "SELECT 1"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	err := reg.LoadFile("testdata/queries/users.sql")
	assertErrorContains(t, err, `glimt: duplicate query name "listUsers" in testdata/queries/users.sql`)

	if reg.Has("insertUser") {
		t.Error("a failed load must not add any of the file's queries")
	}
}

func TestRegistry_AdHocQueryIsSanitized(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	// Without sanitizing, the appended WHERE would land inside the line comment
	// and the owner filter would silently disappear.
	sql, args := reg.Query("SELECT * FROM docs -- every document").
		Where(Eq("owner_id", 7)).
		Build()
	assertSQL(t, sql, "SELECT * FROM docs WHERE owner_id = $1")
	assertArgs(t, args, []any{7})

	sql, _ = reg.Query("SELECT * FROM docs;").Where(Eq("owner_id", 7)).Build()
	assertSQL(t, sql, "SELECT * FROM docs WHERE owner_id = $1")
}

func TestRegistry_AdHocUnterminatedIsKeptAsWritten(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	// The database reports the syntax error; glimt does not guess.
	sql, _ := reg.Query("SELECT 'oops FROM docs").Build()
	assertSQL(t, sql, "SELECT 'oops FROM docs")
}
