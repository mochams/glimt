package integration

import (
	"testing"

	gl "github.com/mochams/glimt"
)

// Models

type Product struct {
	ID        int
	Name      string
	Price     float64
	Stock     int
	Category  string
	Status    string
	CreatedAt string
	DeletedAt *string
}

// Helpers

func insertProduct(t *testing.T, name, category, status string, price float64, stock int) int {
	t.Helper()
	sql, _ := testState.registry.MustGet("insertProduct").Build()
	var id int
	if err := testState.db.QueryRow(sql, name, price, stock, category, status).Scan(&id); err != nil {
		t.Fatalf("insertProduct: %v", err)
	}
	return id
}

func scanProduct(t *testing.T, rows interface{ Scan(...any) error }) Product {
	t.Helper()
	var p Product
	if err := rows.Scan(&p.ID, &p.Name, &p.Price, &p.Stock, &p.Category, &p.Status, &p.CreatedAt, &p.DeletedAt); err != nil {
		t.Fatalf("scanProduct: %v", err)
	}
	return p
}

func cleanProducts(t *testing.T) {
	t.Helper()
	sql, _ := testState.registry.Query("DELETE FROM products").Build()
	if _, err := testState.db.Exec(sql); err != nil {
		t.Fatalf("cleanProducts: %v", err)
	}
}

// Tests

func TestProduct_Insert(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	id := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	if id == 0 {
		t.Error("expected non-zero id after insert")
	}
}

func TestProduct_GetByID(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	id := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	sql, args := testState.registry.MustGet("listProducts").Where(gl.Eq("id", id)).Build()
	row := testState.db.QueryRow(sql, args...)
	p := scanProduct(t, row)

	if p.ID != id {
		t.Errorf("ID: got %d, want %d", p.ID, id)
	}
	if p.Name != "Laptop" {
		t.Errorf("Name: got %q, want %q", p.Name, "Laptop")
	}
	if p.Price != 999.99 {
		t.Errorf("Price: got %f, want %f", p.Price, 999.99)
	}
	if p.Stock != 10 {
		t.Errorf("Stock: got %d, want %d", p.Stock, 10)
	}
}

func TestProduct_UpdateStock(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	id := insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	sql, args := testState.registry.MustGet("updateProductStock").Build()
	if _, err := testState.db.Exec(sql, append(args, 5, id)...); err != nil {
		t.Fatalf("updateProductStock: %v", err)
	}

	getSQL, getArgs := testState.registry.MustGet("listProducts").Where(gl.Eq("id", id)).Build()
	row := testState.db.QueryRow(getSQL, getArgs...)
	p := scanProduct(t, row)

	if p.Stock != 5 {
		t.Errorf("Stock: got %d, want %d", p.Stock, 5)
	}
}

func TestProduct_ListAll(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "active", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)

	sql, args := testState.registry.MustGet("listProducts").Build()
	n := countRows(t, sql, args...)

	if n != 3 {
		t.Errorf("count: got %d, want 3", n)
	}
}

func TestProduct_FilterByCategory(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "active", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.Eq("category", "electronics")).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestProduct_FilterByStatus(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "inactive", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.Eq("status", "active")).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestProduct_FilterByPriceRange(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "active", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)
	insertProduct(t, "Pen", "stationery", "active", 1.99, 100)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.Between("price", 100.00, 600.00)).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestProduct_FilterByPriceRangeExclusive(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "active", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)
	insertProduct(t, "Pen", "stationery", "active", 299.99, 100)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.RangeOpen("price", 299.99, 999.99)).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestProduct_FilterByStock(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 0)
	insertProduct(t, "Phone", "electronics", "active", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.Gt("stock", 0)).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestProduct_FilterByMultipleCategories(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "active", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)
	insertProduct(t, "Pen", "stationery", "active", 1.99, 100)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.In("category", "electronics", "furniture")).
		Build()

	n := countRows(t, sql, args...)
	if n != 3 {
		t.Errorf("count: got %d, want 3", n)
	}
}

func TestProduct_ExcludeCategory(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "active", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)

	sql, args := testState.registry.MustGet("listProducts").
		Exclude(gl.Eq("category", "electronics")).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestProduct_CompoundFilter(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "inactive", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)
	insertProduct(t, "Pen", "stationery", "active", 1.99, 0)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.And(
			gl.Eq("status", "active"),
			gl.Gt("stock", 0),
			gl.Null("deleted_at"),
		)).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestProduct_ChainedWhere(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "inactive", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 0)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.Eq("status", "active")).
		Where(gl.Gt("stock", 0)).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestProduct_Pagination(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "active", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)
	insertProduct(t, "Pen", "stationery", "active", 1.99, 100)
	insertProduct(t, "Chair", "furniture", "active", 199.99, 8)

	sql, args := testState.registry.MustGet("listProducts").
		OrderBy("price ASC").
		Limit(2).
		Offset(2).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}

func TestProduct_FilterEmptyResult(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.Eq("category", "nonexistent")).
		Build()

	n := countRows(t, sql, args...)
	if n != 0 {
		t.Errorf("count: got %d, want 0", n)
	}
}

func TestProduct_NotInFilter(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop", "electronics", "active", 999.99, 10)
	insertProduct(t, "Phone", "electronics", "active", 499.99, 20)
	insertProduct(t, "Desk", "furniture", "active", 299.99, 5)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.NotIn("category", "electronics")).
		Build()

	n := countRows(t, sql, args...)
	if n != 1 {
		t.Errorf("count: got %d, want 1", n)
	}
}

func TestProduct_LikeFilter(t *testing.T) {
	t.Cleanup(func() { cleanProducts(t) })

	insertProduct(t, "Laptop Pro", "electronics", "active", 999.99, 10)
	insertProduct(t, "Laptop Air", "electronics", "active", 799.99, 15)
	insertProduct(t, "Phone", "electronics", "active", 499.99, 20)

	sql, args := testState.registry.MustGet("listProducts").
		Where(gl.Like("name", "Laptop%")).
		Build()

	n := countRows(t, sql, args...)
	if n != 2 {
		t.Errorf("count: got %d, want 2", n)
	}
}
