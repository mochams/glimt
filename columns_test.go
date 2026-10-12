package glimt

import (
	"errors"
	"testing"
)

var testColumns = Columns{"created": "o.created_at", "total": `o."Total"`}

func TestColumnsCol(t *testing.T) {
	if col, err := testColumns.Col("created"); col != "o.created_at" || err != nil {
		t.Errorf("Col(created) = %q, %v", col, err)
	}

	col, err := testColumns.Col("password_hash")
	if col != "" || errText(err) != `unknown field "password_hash"` || !errors.Is(err, ErrUnknownField) {
		t.Errorf("Col(password_hash) = %q, %v", col, err)
	}
}

func TestColumnsOrder(t *testing.T) {
	reg := mustLoad(t, map[string]string{"q.sql": "-- name: q\nSELECT * FROM orders o"})

	sql, _, err := reg.Get("q").Bind(nil).
		OrderBy(testColumns.Order("total", true).NullsLast(), testColumns.Order("created", false)).Build()
	if want := `SELECT * FROM orders o ORDER BY o."Total" DESC NULLS LAST, o.created_at`; sql != want || err != nil {
		t.Errorf("Build = %q, %v; want %q", sql, err, want)
	}

	_, _, err = reg.Get("q").Bind(nil).OrderBy(testColumns.Order("bogus", true)).Build()
	if errText(err) != `glimt: query "q": unknown field "bogus"` || !errors.Is(err, ErrUnknownField) {
		t.Errorf("unknown field error = %v", err)
	}
}

func TestColumnsCheck(t *testing.T) {
	if err := testColumns.Check(); err != nil {
		t.Errorf("Check() = %v", err)
	}

	err := Columns{"z": "a b", "ok": "a", "a": "order"}.Check()
	want := `glimt: field "a": "order" is not a column reference: order is a reserved word, so quote it or qualify it` +
		"\n" + `glimt: field "z": "a b" is not a column reference`

	if errText(err) != want || !errors.Is(err, ErrBadColumn) {
		t.Errorf("Check() = %v\nwant %s", err, want)
	}
}
