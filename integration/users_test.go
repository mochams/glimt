package integration

import (
	"strings"
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
	return insertID(t, "insertUser", name, email, status, age)
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

func TestUser_InTypedSlice(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "inactive", 25)
	insertUser(t, "Charlie", "charlie@example.com", "suspended", 40)

	statuses := []string{"active", "suspended"}
	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.In("status", statuses...)).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestUser_EmptyInMatchesNothing(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "active", 25)

	var allowed []int
	sql, args := testState.registry.MustGet("listUsers").
		Where(gl.In("id", allowed...)).
		Build()

	n := countRows(t, sql, args...)
	if n != 0 {
		t.Errorf("count: got %d, want 0", n)
	}
}

func TestUser_OptionalFiltersSkipped(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "inactive", 25)

	status, minAge := "", 0
	sql, args := testState.registry.MustGet("listUsers").
		Where(
			gl.If(status != "", gl.Eq("status", status)),
			gl.If(minAge > 0, gl.Gte("age", minAge)),
			gl.And(),
		).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestUser_ExcludeEmptyNotInFailsClosed(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)

	var denied []string
	sql, args := testState.registry.MustGet("listUsers").
		Exclude(gl.NotIn("status", denied...)).
		Build()

	n := countRows(t, sql, args...)
	if n != 0 {
		t.Errorf("count: got %d, want 0", n)
	}
}

func TestUser_MarkerKeepsFixedFilterAndGroupBy(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "active", 17)
	insertUser(t, "Charlie", "charlie@example.com", "inactive", 40)

	sql, args := testState.registry.MustGet("countUsersByStatus").
		Where(gl.Gte("age", 18)).
		OrderBy("status").
		Build()

	rows, err := testState.db.Query(sql, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	got := map[string]int{}
	for rows.Next() {
		var status string
		var total int
		if err := rows.Scan(&status, &total); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got[status] = total
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	if len(got) != 2 || got["active"] != 1 || got["inactive"] != 1 {
		t.Errorf("counts: got %v, want map[active:1 inactive:1]", got)
	}
}

func TestUser_ContainsMatchesWildcardsLiterally(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Promo 100%", "promo@example.com", "active", 30)
	insertUser(t, "Promo 1000", "promo1000@example.com", "active", 30)
	insertUser(t, "under_score", "under@example.com", "active", 30)
	insertUser(t, "underXscore", "underx@example.com", "active", 30)

	for _, tc := range []struct {
		pred gl.Predicate
		want int
	}{
		{gl.Contains("name", "100%"), 1},
		{gl.StartsWith("name", "under_"), 1},
		{gl.EndsWith("email", "@example.com"), 4},
	} {
		sql, args := testState.registry.MustGet("listUsers").Where(tc.pred).Build()
		if n := countRows(t, sql, args...); n != tc.want {
			t.Errorf("%s %v: got %d rows, want %d", sql, args, n, tc.want)
		}
	}
}

func TestUser_BuildCountMatchesRows(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "active", 25)
	insertUser(t, "Charlie", "charlie@example.com", "active", 40)
	insertUser(t, "Dave", "dave@example.com", "inactive", 35)

	q := testState.registry.MustGet("listUsers").
		Where(gl.Eq("status", "active")).
		OrderByExpr("CASE WHEN email = ? THEN 0 ELSE 1 END", "charlie@example.com").
		OrderBy("id").
		Limit(2).
		Offset(1)

	countSQL, countArgs := q.BuildCount()

	var total int
	if err := testState.db.QueryRow(countSQL, countArgs...).Scan(&total); err != nil {
		t.Fatalf("count: %v\n%s %v", err, countSQL, countArgs)
	}

	if total != 3 {
		t.Errorf("total: got %d, want 3", total)
	}

	pageSQL, pageArgs := q.Build()
	if n := countRows(t, pageSQL, pageArgs...); n != 2 {
		t.Errorf("page: got %d rows, want 2", n)
	}
}

func TestUser_OrderByExprPinsRowFirst(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice", "alice@example.com", "active", 30)
	insertUser(t, "Bob", "bob@example.com", "active", 25)
	pinned := insertUser(t, "Charlie", "charlie@example.com", "active", 40)

	sql, args := testState.registry.MustGet("listUsers").
		OrderByExpr("CASE WHEN id = ? THEN 0 ELSE 1 END", pinned).
		OrderBy("name").
		Build()

	rows, err := testState.db.Query(sql, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var order []string
	for rows.Next() {
		u := scanUser(t, rows)
		order = append(order, u.Name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	if want := []string{"Charlie", "Alice", "Bob"}; strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order: got %v, want %v", order, want)
	}
}

func TestUser_CaseInsensitiveSearch(t *testing.T) {
	t.Cleanup(func() { cleanUsers(t) })

	insertUser(t, "Alice Smith", "alice@example.com", "active", 30)
	insertUser(t, "ALICE Cooper", "acooper@EXAMPLE.com", "active", 40)
	insertUser(t, "Bob", "bob@example.com", "active", 25)

	for _, tc := range []struct {
		pred gl.Predicate
		want int
	}{
		{gl.IContains("name", "alice"), 2},
		{gl.IStartsWith("name", "ALICE"), 2},
		{gl.IEndsWith("email", "@example.COM"), 3},
	} {
		sql, args := testState.registry.MustGet("listUsers").Where(tc.pred).Build()
		if n := countRows(t, sql, args...); n != tc.want {
			t.Errorf("%s %v: got %d rows, want %d", sql, args, n, tc.want)
		}
	}
}
