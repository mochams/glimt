package glimt

import (
	"errors"
	"testing"
)

func TestCheckColumn(t *testing.T) {
	tests := []struct {
		col  string
		want string // "" when valid
	}{
		{"status", ""},
		{"o.status", ""},
		{"app.orders.created_at", ""},
		{`"Order"`, ""},
		{`t."select"`, ""},
		{`"a""b"`, ""},
		{"t.order", ""},
		{"value", ""},
		{"héllo", ""},
		{"a$1", ""},
		{"o.order", ""},
		{`"null"`, ""},
		{"t.null.to", ""},
		{"order", `"order" is not a column reference: order is a reserved word, so quote it or qualify it`},
		{"user", `"user" is not a column reference: user is a reserved word, so quote it or qualify it`},
		{"null", `"null" is not a column reference: null is a reserved word, so quote it or qualify it`},
		{"TRUE", `"TRUE" is not a column reference: TRUE is a reserved word, so quote it or qualify it`},
		{"current_date", `"current_date" is not a column reference: current_date is a reserved word, so quote it or qualify it`},
		{"to", `"to" is not a column reference: to is a reserved word, so quote it or qualify it`},
		{"check", `"check" is not a column reference: check is a reserved word, so quote it or qualify it`},
		{"left", `"left" is not a column reference: left is a reserved word, so quote it or qualify it`},
		{"order.id", `"order.id" is not a column reference: order is a reserved word, so quote it`},
		{"user.id", `"user.id" is not a column reference: user is a reserved word, so quote it`},
		{"", `"" is not a column reference`},
		{"1a", `"1a" is not a column reference`},
		{"a.", `"a." is not a column reference`},
		{".a", `".a" is not a column reference`},
		{"a b", `"a b" is not a column reference`},
		{"a; DROP TABLE t", `"a; DROP TABLE t" is not a column reference`},
		{"lower(a)", `"lower(a)" is not a column reference`},
		{`""`, `"\"\"" is not a column reference`},
		{`"open`, `"\"open" is not a column reference`},
		{"\"a\x00\"", `"\"a\x00\"" is not a column reference`},
	}

	for _, tt := range tests {
		err := checkColumn(tt.col)

		if got := errText(err); got != tt.want {
			t.Errorf("checkColumn(%q) = %q, want %q", tt.col, got, tt.want)
		}

		if tt.want != "" && !errors.Is(err, ErrBadColumn) {
			t.Errorf("checkColumn(%q) error does not match ErrBadColumn", tt.col)
		}
	}
}

func TestCheckColumnDoesNotAllocate(t *testing.T) {
	if n := testing.AllocsPerRun(50, func() { _ = checkColumn(`app."Orders".created_at`) }); n != 0 {
		t.Errorf("checkColumn allocates %v times", n)
	}
}

// errText returns err's message, or "" for nil.
func errText(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
