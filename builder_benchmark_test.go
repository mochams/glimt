package glimt

import (
	"strings"
	"testing"
)

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
