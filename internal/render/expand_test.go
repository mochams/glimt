package render

import (
	"database/sql/driver"
	"encoding/json"
	"slices"
	"testing"
)

// valuer is a slice type that binds as one value.
type valuer []string

func (v valuer) Value() (driver.Value, error) { return "{" + v[0] + "}", nil }

// ids is a named slice type without a fast path.
type ids []uint

func TestListLen(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
	}{
		{"ints", []int{1, 2, 3}, 3},
		{"int64s", []int64{1}, 1},
		{"int32s", []int32{1, 2}, 2},
		{"strings", []string{"a", "b"}, 2},
		{"anys", []any{1, "a"}, 2},
		{"named slice", ids{4, 5}, 2},
		{"slice of byte slices", [][]byte{{1}, {2}}, 2},
		{"scalar", 7, 1},
		{"byte slice is one value", []byte("abc"), 1},
		{"raw json is one value", json.RawMessage(`[1,2]`), 1},
		{"array is one value", [16]byte{}, 1},
		{"valuer is one value", valuer{"x"}, 1},
		{"pointer is one value", &[]int{1, 2}, 1},
	}

	for _, tt := range tests {
		got, err := listLen(tt.value)
		if err != nil || got != tt.want {
			t.Errorf("%s: listLen = %d, %v, want %d", tt.name, got, err, tt.want)
		}
	}
}

func TestListLenErrors(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, "is a nil value"},
		{"empty ints", []int{}, "is an empty list"},
		{"empty named slice", ids{}, "is an empty list"},
		{"nil slice", []string(nil), "is an empty list"},
		{"nested", [][]int{{1}}, "has elements that are lists, which IN (…) can't take"},
		{"nested in anys", []any{1, []string{"a"}}, "has elements that are lists, which IN (…) can't take"},
		{"empty interface-typed slice", []interface{ String() string }{}, "is an empty list"},
	}

	for _, tt := range tests {
		if _, err := listLen(tt.value); err == nil || err.Error() != tt.want {
			t.Errorf("%s: listLen error = %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestAppendList(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  []any
	}{
		{"ints", []int{1, 2}, []any{1, 2}},
		{"int64s", []int64{3}, []any{int64(3)}},
		{"int32s", []int32{4}, []any{int32(4)}},
		{"strings", []string{"a"}, []any{"a"}},
		{"anys", []any{1, "b"}, []any{1, "b"}},
		{"named slice", ids{5, 6}, []any{uint(5), uint(6)}},
		{"scalar", "x", []any{"x"}},
		{"valuer", valuer{"x"}, []any{valuer{"x"}}},
	}

	for _, tt := range tests {
		got := appendList([]any{"before"}, tt.value)
		if want := append([]any{"before"}, tt.want...); !equalArgs(got, want) {
			t.Errorf("%s: appendList = %v, want %v", tt.name, got, want)
		}
	}
}

// equalArgs compares args, including uncomparable values such as slices, by their printed form.
func equalArgs(a, b []any) bool {
	return slices.EqualFunc(a, b, func(x, y any) bool { return sprint(x) == sprint(y) })
}
