package render

import "errors"

// Errors a caller can match with errors.Is. The glimt package exports them.
var (
	// ErrEmptyList is a list param or IN condition given an empty list.
	ErrEmptyList = errors.New("empty list")
	// ErrNilValue is a nil value where a value or a list is needed.
	ErrNilValue = errors.New("nil value")
	// ErrNotComposable is a composition the query can't take.
	ErrNotComposable = errors.New("not composable")
)

// composeError is a composition the query can't take. Its message stands
// alone, and it matches ErrNotComposable.
type composeError string

// Error returns the message.
func (e composeError) Error() string {
	return string(e)
}

// Is reports whether target is ErrNotComposable.
func (composeError) Is(target error) bool {
	return target == ErrNotComposable
}
