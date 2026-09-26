package integration

import (
	"testing"
	"time"

	gl "github.com/mochams/glimt"
)

// Models

type Order struct {
	ID        int
	UserID    int
	ProductID int
	Quantity  int
	Total     float64
	Status    string
	CreatedAt time.Time
	DeletedAt *time.Time
}

// Helpers

func insertOrder(t *testing.T, userID, productID, quantity int, total float64) int {
	t.Helper()
	return insertID(t, "insertOrder", userID, productID, quantity, total)
}

func scanOrder(t *testing.T, rows interface{ Scan(...any) error }) Order {
	t.Helper()
	var o Order
	if err := rows.Scan(&o.ID, &o.UserID, &o.ProductID, &o.Quantity, &o.Total, &o.Status, &o.CreatedAt, &o.DeletedAt); err != nil {
		t.Fatalf("scanOrder: %v", err)
	}
	return o
}

func cleanOrders(t *testing.T) {
	t.Helper()
	if _, err := testState.db.Exec("DELETE FROM orders"); err != nil {
		t.Fatalf("cleanOrders: %v", err)
	}
}

func cleanAll(t *testing.T) {
	t.Helper()
	cleanOrders(t)
	cleanProducts(t)
	cleanUsers(t)
}

// Tests

func TestOrder_Insert(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	id := insertOrder(t, userID, productID, 1, 999.99)
	if id == 0 {
		t.Error("expected non-zero id after insert")
	}
}

func TestOrder_GetByID(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	id := insertOrder(t, userID, productID, 2, 1999.98)

	sql, args := testState.registry.MustGet("listOrders").Where(gl.Eq("id", id)).Build()
	row := testState.db.QueryRow(sql, args...)
	o := scanOrder(t, row)

	if o.ID != id {
		t.Errorf("ID: got %d, want %d", o.ID, id)
	}
	if o.UserID != userID {
		t.Errorf("UserID: got %d, want %d", o.UserID, userID)
	}
	if o.ProductID != productID {
		t.Errorf("ProductID: got %d, want %d", o.ProductID, productID)
	}
	if o.Quantity != 2 {
		t.Errorf("Quantity: got %d, want %d", o.Quantity, 2)
	}
	if o.Total != 1999.98 {
		t.Errorf("Total: got %f, want %f", o.Total, 1999.98)
	}
}

func TestOrder_ListAll(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	insertOrder(t, userID, productID, 1, 999.99)
	insertOrder(t, userID, productID, 2, 1999.98)
	insertOrder(t, userID, productID, 3, 2999.97)

	sql, args := testState.registry.MustGet("listOrders").Build()
	n := countRows(t, sql, args...)

	if n != 3 {
		t.Errorf("count: got %d, want 3", n)
	}
}

func TestOrder_ListByUser(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	user1 := insertUser(t, "Alice", "alice@example.com", "active", 30)
	user2 := insertUser(t, "Bob", "bob@example.com", "active", 25)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	insertOrder(t, user1, productID, 1, 999.99)
	insertOrder(t, user1, productID, 2, 1999.98)
	insertOrder(t, user2, productID, 1, 999.99)

	sql, args := testState.registry.MustGet("listOrders").Where(gl.Eq("user_id", user1)).Build()
	n := countRows(t, sql, args...)

	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestOrder_FilterByStatus(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	id1 := insertOrder(t, userID, productID, 1, 999.99)
	insertOrder(t, userID, productID, 2, 1999.98)
	insertOrder(t, userID, productID, 3, 2999.97)

	// update one to completed
	sql, args := testState.registry.MustGet("updateOrderStatus").Args("completed", id1).Build()
	if _, err := testState.db.Exec(sql, args...); err != nil {
		t.Fatalf("updateOrderStatus: %v", err)
	}

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Where(gl.Eq("status", "pending")).
		Build()

	n := countRows(t, listSQL, listArgs...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestOrder_FilterByTotalRange(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	insertOrder(t, userID, productID, 1, 50.00)
	insertOrder(t, userID, productID, 2, 500.00)
	insertOrder(t, userID, productID, 3, 1500.00)
	insertOrder(t, userID, productID, 4, 3000.00)

	sql, args := testState.registry.MustGet("listOrders").
		Where(gl.Between("total", 100.00, 2000.00)).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestOrder_FilterByTotalRangeExclusive(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	insertOrder(t, userID, productID, 1, 100.00)
	insertOrder(t, userID, productID, 2, 500.00)
	insertOrder(t, userID, productID, 3, 2000.00)

	sql, args := testState.registry.MustGet("listOrders").
		Where(gl.RangeOpen("total", 100.00, 2000.00)).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestOrder_FilterByMultipleStatuses(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	id1 := insertOrder(t, userID, productID, 1, 999.99)
	id2 := insertOrder(t, userID, productID, 2, 1999.98)
	insertOrder(t, userID, productID, 3, 2999.97)

	updateCompletedSQL, updateCompletedArgs := testState.registry.MustGet("updateOrderStatus").
		Args("completed", id1).
		Build()
	updateCancelledSQL, updateCancelledArgs := testState.registry.MustGet("updateOrderStatus").
		Args("cancelled", id2).
		Build()

	testState.db.Exec(updateCompletedSQL, updateCompletedArgs...)
	testState.db.Exec(updateCancelledSQL, updateCancelledArgs...)

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Where(gl.In("status", "completed", "cancelled")).
		Build()

	n := countRows(t, listSQL, listArgs...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestOrder_ExcludeStatus(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	id1 := insertOrder(t, userID, productID, 1, 999.99)
	insertOrder(t, userID, productID, 2, 1999.98)
	insertOrder(t, userID, productID, 3, 2999.97)

	updateSQL, updateArgs := testState.registry.MustGet("updateOrderStatus").Args("cancelled", id1).Build()
	testState.db.Exec(updateSQL, updateArgs...)

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Exclude(gl.Eq("status", "cancelled")).
		Build()

	n := countRows(t, listSQL, listArgs...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestOrder_CompoundFilter(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	insertOrder(t, userID, productID, 1, 999.99)
	insertOrder(t, userID, productID, 2, 50.00)
	id3 := insertOrder(t, userID, productID, 3, 1500.00)

	updateSQL, updateArgs := testState.registry.MustGet("updateOrderStatus").Args("cancelled", id3).Build()
	testState.db.Exec(updateSQL, updateArgs...)

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Where(gl.And(
			gl.Eq("status", "pending"),
			gl.Gte("total", 100.00),
			gl.Null("deleted_at"),
		)).
		Build()

	n := countRows(t, listSQL, listArgs...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestOrder_ChainedWhere(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	insertOrder(t, userID, productID, 1, 999.99)
	insertOrder(t, userID, productID, 2, 50.00)
	id3 := insertOrder(t, userID, productID, 1, 999.99)

	softSQL, softArgs := testState.registry.MustGet("softDeleteOrder").Args(id3).Build()
	testState.db.Exec(softSQL, softArgs...)

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Where(gl.Eq("status", "pending")).
		Where(gl.Gte("total", 100.00)).
		Where(gl.Null("deleted_at")).
		Build()

	n := countRows(t, listSQL, listArgs...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestOrder_Pagination(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 30)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	insertOrder(t, userID, productID, 1, 100.00)
	insertOrder(t, userID, productID, 2, 200.00)
	insertOrder(t, userID, productID, 3, 300.00)
	insertOrder(t, userID, productID, 4, 400.00)
	insertOrder(t, userID, productID, 5, 500.00)

	sql, args := testState.registry.MustGet("listOrders").
		OrderBy("total ASC").
		Limit(2).
		Offset(2).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestOrder_NotFilter(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	userID := insertUser(t, "Alice", "alice@example.com", "active", 35)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	insertOrder(t, userID, productID, 1, 999.99)
	id2 := insertOrder(t, userID, productID, 2, 1999.98)

	updateSQL, updateArgs := testState.registry.MustGet("updateOrderStatus").Args("cancelled", id2).Build()
	testState.db.Exec(updateSQL, updateArgs...)

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Where(gl.Not(gl.Eq("status", "cancelled"))).
		Build()

	n := countRows(t, listSQL, listArgs...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestOrder_Subqueries(t *testing.T) {
	t.Cleanup(func() { cleanAll(t) })

	alice := insertUser(t, "Alice", "alice@example.com", "active", 30)
	bob := insertUser(t, "Bob", "bob@example.com", "active", 25)
	insertUser(t, "Charlie", "charlie@example.com", "active", 40)
	productID := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	completed := insertOrder(t, alice, productID, 1, 999.99)
	insertOrder(t, bob, productID, 1, 999.99)

	sql, args := testState.registry.MustGet("updateOrderStatus").Args("completed", completed).Build()
	if _, err := testState.db.Exec(sql, args...); err != nil {
		t.Fatalf("updateOrderStatus: %v", err)
	}

	for _, tc := range []struct {
		name string
		pred gl.Predicate
		want int
	}{
		{"InQuery", gl.InQuery("id", testState.registry.MustGet("orderUserIDs").Where(gl.Eq("status", "completed"))), 1},
		{"Exists", gl.Exists(testState.registry.MustGet("userOrders")), 2},
		{"Exists with filter", gl.Exists(testState.registry.MustGet("userOrders").Where(gl.Eq("o.status", "pending"))), 1},
		{"Not Exists", gl.Not(gl.Exists(testState.registry.MustGet("userOrders"))), 1},
	} {
		sql, args := testState.registry.MustGet("listUsers").
			Where(gl.Eq("status", "active"), tc.pred).
			Build()

		if n := countRows(t, sql, args...); n != tc.want {
			t.Errorf("%s: got %d rows, want %d\n%s %v", tc.name, n, tc.want, sql, args)
		}
	}
}
