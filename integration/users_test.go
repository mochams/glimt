package integration

import (
	"testing"
	"time"

	gl "github.com/mochams/glimt"
)

// Models

type User struct {
	ID        int
	Name      string
	Email     string
	Status    string
	Age       int
	CreatedAt time.Time
	DeletedAt *time.Time
}

// Helpers

func insertUser(t *testing.T, name, email, status string, age int) int {
	t.Helper()
	sql, args := testState.registry.MustGet("insertUser").Build()
	args = append([]any{name, email, status, age}, args...)

	var id int
	err := testState.db.QueryRow(sql, name, email, status, age).Scan(&id)
	if err != nil {
		t.Fatalf("insertUser: %v", err)
	}
	_ = args
	return id
}

func scanUser(t *testing.T, rows interface{ Scan(...any) error }) User {
	t.Helper()
	var u User
	if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Status, &u.Age, &u.CreatedAt, &u.DeletedAt); err != nil {
		t.Fatalf("scanUser: %v", err)
	}
	return u
}

func countRows(t *testing.T, sql string, args ...any) int {
	t.Helper()
	rows, err := testState.db.Query(sql, args...)
	if err != nil {
		t.Fatalf("countRows query: %v", err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("countRows scan: %v", err)
	}
	return n
}

func cleanUsers(t *testing.T) {
	t.Helper()
	sql, _ := testState.registry.Query("DELETE FROM users").Build()
	if _, err := testState.db.Exec(sql); err != nil {
		t.Fatalf("cleanUsers: %v", err)
	}
}

// Tests

func TestUser_Insert(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	id := insertUser(t, "Alice", "alice@example.com", "active", 30)
	if id == 0 {
		t.Error("expected non-zero id after insert")
	}
}

func TestUser_GetByID(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	id := insertUser(t, "Alice", "alice@example.com", "active", 30)

	sql, args := testState.registry.MustGet("listUsers").Where(gl.Eq("id", id)).Build()
	row := testState.db.QueryRow(sql, args...)
	u := scanUser(t, row)

	if u.ID != id {
		t.Errorf("ID: got %d, want %d", u.ID, id)
	}
	if u.Name != "Alice" {
		t.Errorf("Name: got %q, want %q", u.Name, "Alice")
	}
	if u.Email != "alice@example.com" {
		t.Errorf("Email: got %q, want %q", u.Email, "alice@example.com")
	}
}

func TestUser_GetByEmail(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)

	sql, args := testState.registry.MustGet("listUsers").Where(gl.Eq("email", "alice@example.com")).Build()
	row := testState.db.QueryRow(sql, args...)
	u := scanUser(t, row)

	if u.Email != "alice@example.com" {
		t.Errorf("Email: got %q, want %q", u.Email, "alice@example.com")
	}
}

func TestUser_ListAll(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "active", 25)
	insertUser(t, "Charlie", "charlie@example.com", "inactive", 40)

	sql, args := testState.registry.MustGet("listUsers").Build()
	n := countRows(t, sql, args...)

	if n != 3 {
		t.Errorf("count: got %d, want 3", n)
	}
}

func TestUser_FilterByStatus(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "active", 25)
	insertUser(t, "Charlie", "charlie@example.com", "inactive", 40)

	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.Eq("status", "active")).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestUser_FilterByAgeRange(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 17)
	insertUser(t, "Bob", "bob@example.com", "active", 25)
	insertUser(t, "Charlie", "charlie@example.com", "active", 66)

	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.Between("age", 18, 65)).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestUser_FilterByAgeRangeExclusive(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 18)
	insertUser(t, "Bob", "bob@example.com", "active", 25)
	insertUser(t, "Charlie", "charlie@example.com", "active", 65)

	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.RangeOpen("age", 18, 65)).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestUser_FilterByMultipleStatuses(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "inactive", 25)
	insertUser(t, "Charlie", "charlie@example.com", "suspended", 40)

	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.In("status", "active", "inactive")).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestUser_ExcludeStatus(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "inactive", 25)
	insertUser(t, "Charlie", "charlie@example.com", "suspended", 40)

	sql, args := testState.registry.MustGet("listUsers").
		Exclude(gl.Eq("status", "suspended")).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestUser_CompoundFilter(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "active", 17)
	insertUser(t, "Charlie", "charlie@example.com", "inactive", 30)

	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.And(
			gl.Eq("status", "active"),
			gl.Gte("age", 18),
		)).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestUser_OrFilter(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "inactive", 25)
	insertUser(t, "Charlie", "charlie@example.com", "suspended", 40)

	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.Or(
			gl.Eq("status", "active"),
			gl.Eq("status", "inactive"),
		)).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestUser_ChainedWhere(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "active", 17)
	insertUser(t, "Charlie", "charlie@example.com", "inactive", 30)

	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.Eq("status", "active")).
		Where(gl.Gte("age", 18)).
		Where(gl.Null("deleted_at")).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestUser_Pagination(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "active", 25)
	insertUser(t, "Charlie", "charlie@example.com", "active", 40)
	insertUser(t, "Dave", "dave@example.com", "active", 35)
	insertUser(t, "Eve", "eve@example.com", "active", 28)

	sql, args := testState.registry.MustGet("listUsers").
		OrderBy("age ASC").
		Limit(2).
		Offset(2).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestUser_NotFilter(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "inactive", 25)

	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.Not(gl.Eq("status", "inactive"))).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestUser_LikeFilter(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice Smith", "alice@example.com", "active", 30)
	insertUser(t, "Bob Jones", "bob@example.com", "active", 25)
	insertUser(t, "Alice Cooper", "acooper@example.com", "active", 40)

	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.Like("name", "Alice%")).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestUser_NotInFilter(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "inactive", 25)
	insertUser(t, "Charlie", "charlie@example.com", "suspended", 40)

	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.NotIn("status", "inactive", "suspended")).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}
