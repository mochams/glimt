package integration

import (
	"testing"
	"time"

	glimt "github.com/mochams/glimt"
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
	sql, _ := testState.registry.MustGet("insertOrder").Build()
	var id int
	if err := testState.db.QueryRow(sql, userID, productID, quantity, total).Scan(&id); err != nil {
		t.Fatalf("insertOrder: %v", err)
	}
	return id
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

	sql, args := testState.registry.MustGet("listOrders").Where(glimt.Eq("id", id)).Build()
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

	sql, args := testState.registry.MustGet("listOrders").Where(glimt.Eq("user_id", user1)).Build()
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
	sql, args := testState.registry.MustGet("updateOrderStatus").Build()
	if _, err := testState.db.Exec(sql, append(args, "completed", id1)...); err != nil {
		t.Fatalf("updateOrderStatus: %v", err)
	}

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Where(glimt.Eq("status", "pending")).
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
		Where(glimt.Between("total", 100.00, 2000.00)).
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
		Where(glimt.RangeOpen("total", 100.00, 2000.00)).
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

	updateSQL, updateArgs := testState.registry.MustGet("updateOrderStatus").Build()
	testState.db.Exec(updateSQL, append(updateArgs, "completed", id1)...)
	testState.db.Exec(updateSQL, append(updateArgs, "cancelled", id2)...)

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Where(glimt.In("status", "completed", "cancelled")).
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

	updateSQL, updateArgs := testState.registry.MustGet("updateOrderStatus").Build()
	testState.db.Exec(updateSQL, append(updateArgs, "cancelled", id1)...)

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Exclude(glimt.Eq("status", "cancelled")).
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

	updateSQL, updateArgs := testState.registry.MustGet("updateOrderStatus").Build()
	testState.db.Exec(updateSQL, append(updateArgs, "cancelled", id3)...)

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Where(glimt.And(
			glimt.Eq("status", "pending"),
			glimt.Gte("total", 100.00),
			glimt.Null("deleted_at"),
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

	softSQL, softArgs := testState.registry.MustGet("softDeleteOrder").Build()
	testState.db.Exec(softSQL, append(softArgs, id3)...)

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Where(glimt.Eq("status", "pending")).
		Where(glimt.Gte("total", 100.00)).
		Where(glimt.Null("deleted_at")).
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

	updateSQL, updateArgs := testState.registry.MustGet("updateOrderStatus").Build()
	testState.db.Exec(updateSQL, append(updateArgs, "cancelled", id2)...)

	listSQL, listArgs := testState.registry.MustGet("listOrders").
		Where(glimt.Not(glimt.Eq("status", "cancelled"))).
		Build()

	n := countRows(t, listSQL, listArgs...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}
