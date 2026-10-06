package glimt

import (
	"slices"
	"testing"
)

// composed builds the query "SELECT * FROM t" with preds as its conditions.
func composed(t *testing.T, preds ...Pred) (string, []any, error) {
	t.Helper()

	reg := mustLoad(t, map[string]string{"q.sql": "-- name: q\nSELECT * FROM t"})

	return reg.Get("q").Bind(nil).Where(preds...).Build()
}

func TestPreds(t *testing.T) {
	tests := []struct {
		name  string
		preds []Pred
		where string
		args  []any
	}{
		{"comparisons", []Pred{Eq("a", 1), Ne("b", 2), Lt("c", 3), Le("d", 4), Gt("e", 5), Ge("f", 6)},
			"a = $1 AND b <> $2 AND c < $3 AND d <= $4 AND e > $5 AND f >= $6", []any{1, 2, 3, 4, 5, 6}},
		{"lists", []Pred{In("a", []int{1, 2}), NotIn("b", []string{"x"})},
			"a IN ($1, $2) AND b NOT IN ($3)", []any{1, 2, "x"}},
		{"nulls", []Pred{IsNull("a"), IsNotNull("t.b")}, "a IS NULL AND t.b IS NOT NULL", nil},
		{"patterns", []Pred{Like("a", "x%"), NotLike("b", "y"), ILike("c", "z"), NotILike("d", "w")},
			"a LIKE $1 AND b NOT LIKE $2 AND c ILIKE $3 AND d NOT ILIKE $4", []any{"x%", "y", "z", "w"}},
		{"escaped patterns", []Pred{Contains("a", "50%_off"), StartsWith("b", `c:\`), EndsWith("c", "z"),
			IContains("d", "x"), IStartsWith("e", "y"), IEndsWith("f", "z")},
			"a LIKE $1 AND b LIKE $2 AND c LIKE $3 AND d ILIKE $4 AND e ILIKE $5 AND f ILIKE $6",
			[]any{`%50\%\_off%`, `c:\\%`, "%z", "%x%", "y%", "%z"}},
		{"groups", []Pred{Or(Eq("a", 1), And(Eq("b", 2), Eq("c", 3))), Not(Or(IsNull("d"), Eq("e", 4)))},
			"(a = $1 OR (b = $2 AND c = $3)) AND NOT ((d IS NULL OR e = $4))", []any{1, 2, 3, 4}},
		{"single-operand groups need no parentheses", []Pred{Or(Eq("a", 1)), And(If(false, Eq("x", 0)), Eq("b", 2))},
			"a = $1 AND b = $2", []any{1, 2}},
		{"if", []Pred{If(true, Eq("a", 1)), If(false, Eq("b", 2))}, "a = $1", []any{1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := composed(t, tt.preds...)
			if err != nil {
				t.Fatal(err)
			}

			if want := "SELECT * FROM t WHERE " + tt.where; sql != want || !slices.Equal(args, tt.args) {
				t.Errorf("Build =\n%s %v\nwant\n%s %v", sql, args, want, tt.args)
			}
		})
	}
}

func TestEmptyPreds(t *testing.T) {
	sql, args, err := composed(t, If(false, Eq("a", 1)), Pred{}, And(), Or(Pred{}), Not(Pred{}))
	if err != nil || sql != "SELECT * FROM t" || len(args) != 0 {
		t.Errorf("Build = %q, %v, %v; want the query unchanged", sql, args, err)
	}
}

func TestPredErrors(t *testing.T) {
	tests := []struct {
		pred Pred
		want string
	}{
		{Eq("a b", 1), `glimt: query "q": "a b" is not a column reference`},
		{In("a", []int{}), `glimt: query "q": the value of In("a") is an empty list`},
		{NotIn("a", nil), `glimt: query "q": the value of NotIn("a") is a nil value`},
		{Or(Eq("ok", 1), IsNull("order")), `glimt: query "q": "order" is not a column reference: order is a reserved word, so quote it or qualify it`},
		{Eq("status", nil), `glimt: query "q": Eq("status") has a nil value: use IsNull`},
		{Ne("status", nil), `glimt: query "q": Ne("status") has a nil value: use IsNotNull`},
		{Not(Ge("n", nil)), `glimt: query "q": Ge("n") has a nil value`},
	}

	for _, tt := range tests {
		if _, _, err := composed(t, tt.pred); errText(err) != tt.want {
			t.Errorf("Build error = %q, want %q", errText(err), tt.want)
		}
	}
}

func TestPredNilPointer(t *testing.T) {
	var since *int

	sql, args, err := composed(t, Ge("n", since))
	if err != nil || sql != "SELECT * FROM t WHERE n >= $1" || len(args) != 1 || args[0] != since {
		t.Errorf("Build = %q, %v, %v; want a nil pointer bound as NULL", sql, args, err)
	}
}
