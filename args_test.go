package glimt

import (
	"errors"
	"slices"
	"testing"
)

func TestArgsValues(t *testing.T) {
	got, err := Args{"b": 2, "a": 1}.values([]string{"a", "b"})
	if err != nil || !slices.Equal(got, []any{1, 2}) {
		t.Errorf("values = %v, %v; want [1 2]", got, err)
	}

	if got, err := Args(nil).values(nil); got != nil || err != nil {
		t.Errorf("values with no params = %v, %v", got, err)
	}
}

func TestArgsValuesErrors(t *testing.T) {
	tests := []struct {
		args  Args
		names []string
		want  string
		is    error
	}{
		{Args{"a": 1}, nil, "no param :a", ErrUnknownArg},
		{Args{"a": 1, "z": 2, "c": 3}, []string{"a"}, "no param :c", ErrUnknownArg},
		{Args{"a": 1}, []string{"a", "b"}, "no value for :b", ErrMissingArg},
		{nil, []string{"a"}, "no value for :a", ErrMissingArg},
	}

	for _, tt := range tests {
		_, err := tt.args.values(tt.names)
		if err == nil || err.Error() != tt.want || !errors.Is(err, tt.is) {
			t.Errorf("values(%v, %v) error = %v, want %q matching %v", tt.args, tt.names, err, tt.want, tt.is)
		}
	}
}
