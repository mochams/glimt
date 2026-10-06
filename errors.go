package glimt

import (
	"errors"
	"fmt"

	"github.com/mochams/glimt/internal/render"
)

// Errors wrapped by the errors of Build, BuildCount and [Columns.Col], for
// use with [errors.Is].
var (
	// ErrMissingArg means a :name param has no value in [Args].
	ErrMissingArg = errors.New("no value")
	// ErrUnknownArg means [Args] holds a value the query has no :name param
	// for.
	ErrUnknownArg = errors.New("no param")
	// ErrEmptyList means an empty list was given to [In], [NotIn] or an
	// IN (:name) param.
	ErrEmptyList = render.ErrEmptyList
	// ErrNilValue means an untyped nil was given to a comparison such as
	// [Eq], or as a list. Use [IsNull] or [IsNotNull] to test for NULL.
	ErrNilValue = render.ErrNilValue
	// ErrBadColumn means a column is not a plain column reference, or starts
	// with an unquoted word Postgres reserves.
	ErrBadColumn = errors.New("not a column reference")
	// ErrUnknownField means a field is not in a [Columns].
	ErrUnknownField = errors.New("unknown field")
	// ErrNotComposable means the statement can't take a change, such as
	// Where on a UNION or Limit on an UPDATE.
	ErrNotComposable = render.ErrNotComposable
)

// LoadError is a problem [Load] found in a .sql file, with its location.
type LoadError struct {
	File string // the file's path in the loaded filesystem
	Line int    // 1-based
	Col  int    // 1-based byte column
	Err  error
}

// Error returns the message as file:line:col: glimt: err.
func (e *LoadError) Error() string {
	return fmt.Sprintf("%s:%d:%d: glimt: %v", e.File, e.Line, e.Col, e.Err)
}

// Unwrap returns the underlying error.
func (e *LoadError) Unwrap() error {
	return e.Err
}

// buildError adds the glimt prefix and the query's name to an error from
// Build or BuildCount.
func buildError(query string, err error) error {
	return fmt.Errorf("glimt: query %q: %w", query, err)
}
