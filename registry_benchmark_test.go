package glimt

import (
	"testing"
)

// Helper

func getRegistry(b *testing.B) *Registry {
	b.Helper()

	registry := NewRegistry(DialectPostgres)

	err := registry.LoadDir("testdata/queries")
	if err != nil {
		b.Fatalf("failed to load queries: %v", err)
	}

	return registry
}

// Benchmarks

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
