package glimt

import (
	"errors"
	"fmt"
	"slices"
)

// Columns maps the field names a request may use, such as the sort key in
// ?sort=created, to the columns they stand for. Use it whenever a request
// chooses a column: only the listed columns can be reached, and the
// request's text never becomes SQL. Define a Columns once and call
// [Columns.Check] at startup.
//
//	var sortable = glimt.Columns{"created": "o.created_at", "total": "o.total"}
type Columns map[string]string

// Col returns the column for field, for use in a condition such as
// Eq(col, v). An unknown field is an error that wraps [ErrUnknownField]
// and names the field.
func (c Columns) Col(field string) (string, error) {
	col, ok := c[field]
	if !ok {
		return "", unknownField(field)
	}

	return col, nil
}

// Order returns the order term for field, descending when desc is true. An
// unknown field is reported by Build as an error that wraps
// [ErrUnknownField], so the call can be chained like [Asc] and [Desc]:
//
//	b = b.OrderBy(sortable.Order(req.Sort, req.Desc))
func (c Columns) Order(field string, desc bool) Order {
	col, ok := c[field]
	if !ok {
		return Order{err: unknownField(field)}
	}

	return Order{col: col, desc: desc}
}

// Check reports every entry whose column is not a plain column reference,
// sorted by field. Call it once, at startup.
func (c Columns) Check() error {
	fields := make([]string, 0, len(c))
	for field := range c {
		fields = append(fields, field)
	}

	slices.Sort(fields)

	var errs []error

	for _, field := range fields {
		if err := checkColumn(c[field]); err != nil {
			errs = append(errs, fmt.Errorf("glimt: field %q: %w", field, err))
		}
	}

	return errors.Join(errs...)
}

// unknownField reports a field that is not in a Columns.
func unknownField(field string) error {
	return fmt.Errorf("%w %q", ErrUnknownField, field)
}
