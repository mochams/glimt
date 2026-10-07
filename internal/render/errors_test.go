package render

import (
	"errors"
	"testing"
)

func TestComposeError(t *testing.T) {
	err := error(composeError("LIMIT can't be added here"))

	if got := err.Error(); got != "LIMIT can't be added here" {
		t.Errorf("Error() = %q", got)
	}

	if !errors.Is(err, ErrNotComposable) {
		t.Error("errors.Is(err, ErrNotComposable) = false")
	}

	if errors.Is(err, ErrEmptyList) {
		t.Error("errors.Is(err, ErrEmptyList) = true")
	}
}

func TestListErrors(t *testing.T) {
	tests := []struct {
		v    any
		want error
	}{
		{nil, ErrNilValue},
		{[]int{}, ErrEmptyList},
		{[]any{}, ErrEmptyList},
	}

	for _, tt := range tests {
		if _, err := listLen(tt.v); !errors.Is(err, tt.want) {
			t.Errorf("listLen(%#v) = %v, want %v", tt.v, err, tt.want)
		}
	}
}
