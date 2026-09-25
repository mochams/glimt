package glimt

import (
	"strings"
	"testing"
)

// Helpers

func getRegistry(b *testing.B) *Registry {
	b.Helper()

	registry := NewRegistry(DialectPostgres)

	err := registry.Load("testdata/queries")
	if err != nil {
		b.Fatalf("failed to load queries: %v", err)
	}

	return registry
}

// Benchmarks

func BenchmarkBuilder_write(b *testing.B) {
	builder := &sqlBuilder{}

	b.ReportAllocs()

	for b.Loop() {
		builder.reset()

		builder.write("SELECT * FROM table WHERE col = ?")
		builder.arg(42)
	}
}

func BenchmarkBuilder_writeLarge(b *testing.B) {
	builder := &sqlBuilder{}
	sql := "SELECT * FROM table WHERE col1 = ? AND col2 = ? AND col3 = ? AND col4 = ? AND col5 = ?"
	str := strings.Repeat(sql, 100)

	b.ReportAllocs()

	for b.Loop() {
		builder.reset()

		builder.write(str)

		for j := range 100 {
			builder.arg(j)
		}
	}
}

func BenchmarkBuilder_Read(b *testing.B) {
	builder := &sqlBuilder{}
	sql := "SELECT * FROM table WHERE col1 = ? AND col2 = ? AND col3 = ? AND col4 = ? AND col5 = ?"
	str := strings.Repeat(sql, 100)

	builder.write(str)

	for j := range 100 {
		builder.arg(j)
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = builder.string()
		_ = builder.args
	}
}

func BenchmarkPredicate_Cond(b *testing.B) {
	builder := &sqlBuilder{}

	for b.Loop() {
		builder.reset()

		pred := Cond("name = ?", "doe")
		pred(builder)
	}
}

func BenchmarkPredicate_CondComplex(b *testing.B) {
	builder := &sqlBuilder{}

	for b.Loop() {
		builder.reset()

		pred := Cond("name in (?, ?, ?, ?)", "doe", "smith", "johnson", "williams")
		pred(builder)
	}
}

func BenchmarkPredicate_And(b *testing.B) {
	builder := &sqlBuilder{}

	for b.Loop() {
		builder.reset()

		pred := And(
			Cond("name = ?", "doe"),
			Cond("age > ?", 30),
			Cond("status = ?", "active"),
		)
		pred(builder)
	}
}

func BenchmarkPredicate_AndComplex(b *testing.B) {
	builder := &sqlBuilder{}

	for b.Loop() {
		builder.reset()

		pred := And(
			Cond("name = ?", "doe"),
			Cond("age > ?", 30),
			Cond("status = ?", "active"),
			In("name", "doe", "smith", "johnson"),
			Cond("role = ?", "admin"),
			Cond("email = ?", "active@example.com"),
		)
		pred(builder)
	}
}

func BenchmarkPredicate_Eq(b *testing.B) {
	builder := &sqlBuilder{}

	for b.Loop() {
		builder.reset()

		pred := Eq("age", 967)
		pred(builder)
	}
}

func BenchmarkPredicate_In(b *testing.B) {
	builder := &sqlBuilder{}

	for b.Loop() {
		builder.reset()

		pred := In(
			"name",
			"doe",
			"smith",
			"johnson",
			"williams",
			"brown",
			"jones",
			"garcia",
			"miller",
			"davis",
		)
		pred(builder)
	}
}

func BenchmarkRegistry_Get(b *testing.B) {
	registry := getRegistry(b)

	b.ReportAllocs()

	for b.Loop() {
		sq, err := registry.Get("listUsers")
		if err != nil {
			b.Fatalf("failed to get query: %v", err)
		}

		sq.Build()
	}
}

func BenchmarkRegistry_MustGet(b *testing.B) {
	registry := getRegistry(b)

	b.ReportAllocs()

	for b.Loop() {
		registry.MustGet("listUsers").Build()
	}
}

func BenchmarkRegistry_SimpleQuery(b *testing.B) {
	registry := getRegistry(b)

	b.ReportAllocs()

	for b.Loop() {
		registry.MustGet("listUsers").Where(
			Eq("id", 42),
		).Build()
	}
}

func BenchmarkRegistry_ComplexQuery(b *testing.B) {
	registry := getRegistry(b)

	b.ReportAllocs()

	for b.Loop() {
		registry.MustGet("listUsers").
			Where(
				And(
					Eq("status", "active"),
					In("role", "admin", "mod", "user"),
					RangeOpen("age", 18, 65),
					Or(
						Eq("region", "us"),
						Eq("region", "eu"),
					),
					NotNull("email_verified_at"),
					Not(
						Or(
							Eq("account_status", "suspended"),
							Eq("account_status", "banned"),
						),
					),
					Cond("created_at > ?", "2024-01-01"),
				),
			).
			Exclude(Null("deleted_at")).
			GroupBy("status", "role", "region").
			Having(Gt("COUNT(*)", 5)).
			OrderBy("created_at DESC", "name ASC").
			Limit(20).
			Offset(100).
			Build()
	}
}

func BenchmarkRegistry_AdHocQuery(b *testing.B) {
	registry := getRegistry(b)

	b.ReportAllocs()

	for b.Loop() {
		registry.Query("SELECT * FROM users -- ad-hoc").Where(
			Eq("id", 42),
		).Build()
	}
}
