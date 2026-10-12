package glimt

import "github.com/mochams/glimt/internal/render"

// Order is one ORDER BY term. Make it with [Asc], [Desc] or
// [Columns.Order]. Its column is written into the SQL as given, so it must
// come from your code or from a [Columns], never from a request.
type Order struct {
	col   string
	desc  bool
	nulls nullsOrder
	err   error // from Columns.Order, reported at Build
}

// nullsOrder is where an order term puts NULLs.
type nullsOrder uint8

// NULLs orders: Postgres's default, or as NullsFirst or NullsLast set.
const (
	nullsDefault nullsOrder = iota
	nullsFirst
	nullsLast
)

// nullsSQL is the SQL written after an order term for each nullsOrder.
var nullsSQL = [...]string{nullsDefault: "", nullsFirst: " NULLS FIRST", nullsLast: " NULLS LAST"}

// Asc orders by col, smallest first. Postgres puts NULLs last.
func Asc(col string) Order { return Order{col: col} }

// Desc orders by col, largest first. Postgres puts NULLs first.
func Desc(col string) Order { return Order{col: col, desc: true} }

// NullsFirst puts NULLs before every other value.
func (o Order) NullsFirst() Order {
	o.nulls = nullsFirst

	return o
}

// NullsLast puts NULLs after every other value.
func (o Order) NullsLast() Order {
	o.nulls = nullsLast

	return o
}

// write writes the term.
func (o Order) write(w *render.Writer) error {
	if o.err != nil {
		return o.err
	}

	if err := checkColumn(o.col); err != nil {
		return err
	}

	w.SQL(o.col)

	if o.desc {
		w.SQL(" DESC")
	}

	w.SQL(nullsSQL[o.nulls])

	return nil
}
