package glimt

import (
	"testing"
)

// Helpers

func assertArgs(t *testing.T, gotArgs, wantArgs []any) {
	t.Helper()

	if len(gotArgs) != len(wantArgs) {
		t.Errorf("args length: got %d, want %d\ngot  %v\nwant %v",
			len(gotArgs), len(wantArgs), gotArgs, wantArgs)

		return
	}

	for i := range gotArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Errorf("arg[%d]: got %v, want %v", i, gotArgs[i], wantArgs[i])
		}
	}
}

func assertSQL(t *testing.T, got, want string) {
	t.Helper()

	if got != want {
		t.Errorf("SQL: got %q, want %q", got, want)
	}
}

// Tests

func TestBuilder_write(t *testing.T) {
	b := &sqlBuilder{}

	str := "SELECT * FROM users"

	b.write(str)

	assertSQL(t, b.string(), str)
}

func TestBuilder_writeByte(t *testing.T) {
	b := &sqlBuilder{}

	str := "SELECT * FROM users"

	for i := range len(str) {
		b.writeByte(str[i])
	}

	assertSQL(t, b.string(), str)
}

func TestBuilder_grow(t *testing.T) {
	b := &sqlBuilder{}

	b.write("a")
	b.grow(1024)
	b.write("b")

	assertSQL(t, b.string(), "ab")
}

func TestBuilder_arg(t *testing.T) {
	b := &sqlBuilder{}

	args := []any{1, "active", true}

	for _, a := range args {
		b.arg(a)
	}

	assertArgs(t, b.args, args)
}

func TestBuilder_string(t *testing.T) {
	b := &sqlBuilder{}

	str := "SELECT * FROM users"

	b.write(str)

	assertSQL(t, b.string(), str)
}

func TestBuilder_reset(t *testing.T) {
	b := &sqlBuilder{}

	b.write("SELECT * FROM users")
	b.arg(1)
	b.arg("active")

	b.reset()

	assertSQL(t, b.string(), "")
	assertArgs(t, b.args, nil)
}
