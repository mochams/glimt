package glimt

import (
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
	if err == nil {
		t.Error("expected error for unknown query, got nil")
	}
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
	if err := reg.Load("testdata/queries/users.sql"); err != nil {
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

	err := reg.Load("testdata/invalid_queries/duplicate_names.sql")
	if err == nil {
		t.Fatal("expected error for duplicate query names, got nil")
	}

	errMsg := "glimt: parse testdata/invalid_queries/duplicate_names.sql: duplicate query name \"listUsers\""
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadNonexistentFile(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.Load("testdata/queries/nonexistent.sql")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}

	errMsg := "glimt: open testdata/queries/nonexistent.sql: open testdata/queries/nonexistent.sql: no such file or directory"
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadInvalidNaming(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.Load("testdata/invalid_queries/wrong_names.sql")
	if err != nil {
		t.Fatal("unexpected error loading file:", err)
	}

	if len(reg.Queries()) != 0 {
		t.Errorf("expected no queries loaded, got %d", len(reg.Queries()))
	}
}

func TestRegistry_LoadEmptyFile(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.Load("testdata/queries/empty.sql")
	if err != nil {
		t.Fatal("unexpected error loading file:", err)
	}

	if len(reg.Queries()) != 0 {
		t.Errorf("expected no queries loaded, got %d", len(reg.Queries()))
	}
}

func TestRegistry_LoadDir(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadDir("testdata/queries")
	if err != nil {
		t.Fatalf("failed to load queries: %v", err)
	}

	expectedQueries := []string{
		"createOrdersTable",
		"createProductsTable",
		"createUsersTable",
		"dropOrdersTable",
		"dropProductsTable",
		"dropUsersTable",
		"insertOrder",
		"insertProduct",
		"insertUser",
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

	err := reg.LoadDir("testdata/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}

	errMsg := "glimt: read dir testdata/nonexistent: open testdata/nonexistent: no such file or directory"
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadEmptyDir(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadDir("testdata/empty_queries")
	if err != nil {
		t.Fatal("unexpected error loading empty directory:", err)
	}

	if len(reg.Queries()) != 0 {
		t.Errorf("expected no queries loaded, got %d", len(reg.Queries()))
	}
}

func TestRegistry_LoadInvalidQueries(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadDir("testdata/invalid_queries")
	if err == nil {
		t.Fatal("expected error for nonexistent directory, got nil")
	}

	errMsg := "glimt: parse testdata/invalid_queries/duplicate_names.sql: duplicate query name \"listUsers\""
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadFS(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFS(os.DirFS("testdata/queries"), "users.sql")
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

func TestRegistry_LoadFSNonexistentFile(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFS(os.DirFS("testdata/queries"), "nonexistent.sql")
	if err == nil {
		t.Fatal("expected error for nonexistent file in FS, got nil")
	}

	errMsg := "glimt: open nonexistent.sql: open nonexistent.sql: no such file or directory"
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_LoadFSDuplicate(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.LoadFS(os.DirFS("testdata/invalid_queries"), "duplicate_names.sql")
	if err == nil {
		t.Fatal("expected error for duplicate query names in FS, got nil")
	}

	errMsg := "glimt: parse duplicate_names.sql: duplicate query name \"listUsers\""
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_WalkFS(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.WalkFS(os.DirFS("testdata/queries"), ".")
	if err != nil {
		t.Fatalf("failed to walk FS: %v", err)
	}

	expectedQueries := []string{
		"createOrdersTable",
		"createProductsTable",
		"createUsersTable",
		"dropOrdersTable",
		"dropProductsTable",
		"dropUsersTable",
		"insertOrder",
		"insertProduct",
		"insertUser",
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

func TestRegistry_WalkFSNonexistentDir(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.WalkFS(os.DirFS("testdata/queries"), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent directory in FS, got nil")
	}

	errMsg := "glimt: read dir nonexistent: open nonexistent: no such file or directory"
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_WalkFSInvalidQueries(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.WalkFS(os.DirFS("testdata/invalid_queries"), ".")
	if err == nil {
		t.Fatal("expected error for invalid queries in FS, got nil")
	}

	errMsg := "glimt: parse duplicate_names.sql: duplicate query name \"listUsers\""
	assertErrorContains(t, err, errMsg)
}

func TestRegistry_WalkFSEmptyDir(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.WalkFS(os.DirFS("testdata/empty_queries"), ".")
	if err != nil {
		t.Fatal("unexpected error walking empty directory in FS:", err)
	}

	if len(reg.Queries()) != 0 {
		t.Errorf("expected no queries loaded, got %d", len(reg.Queries()))
	}
}

func TestRegistry_Has(t *testing.T) {
	reg := NewRegistry(DialectPostgres)

	err := reg.Load("testdata/queries/users.sql")
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

	err := reg.Load("testdata/queries/users.sql")
	if err != nil {
		t.Fatalf("failed to load queries: %v", err)
	}

	gotQueries := reg.Queries()
	expectedQueries := []string{"createUsersTable", "dropUsersTable", "insertUser", "listUsers"}

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

	err := reg.LoadDir("testdata/queries")
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
