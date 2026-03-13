package glimt

import (
	"testing"
)

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
