package render

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mochams/glimt/internal/syntax"
)

// fake is a composition for tests: a plan, and the SQL it fills holes with.
// Its where and order fragments are SQL with "?" for each value, taken in
// order from vals; "??" binds a list. A nil fake composes nothing.
type fake struct {
	where   string
	order   string
	mode    OrderMode
	vals    []any
	limit   any
	offset  any
	failure error // returned when filling a hole, when set
}

// plan returns the fake's plan.
func (f *fake) plan(count bool) Plan {
	if f == nil {
		return Plan{Count: count}
	}

	return Plan{Where: f.where != "", Order: f.mode, Limit: f.limit, Offset: f.offset, Count: count}
}

// render renders tmpl composed as the fake says, or counted.
func (f *fake) render(tmpl *Template, values []any, count bool) (string, []any, error) {
	var r Run
	if err := tmpl.Begin(&r, values, f.plan(count)); err != nil {
		return "", nil, err
	}

	for h := r.Next(); h != Done; h = r.Next() {
		frag := f.where
		if h == HoleOrder {
			frag = f.order
		}

		if err := f.write(r.Writer(), frag); err != nil {
			return "", nil, err
		}
	}

	return r.Finish()
}

// write writes a fragment, binding a value at each "?" and a list at each "??".
func (f *fake) write(w *Writer, frag string) error {
	if f.failure != nil {
		return f.failure
	}

	for {
		before, after, found := strings.Cut(frag, "?")
		w.SQL(before)

		if !found {
			return nil
		}

		v := f.vals[0]
		f.vals = f.vals[1:]

		if rest, list := strings.CutPrefix(after, "?"); list {
			if err := w.List(v); err != nil {
				return err
			}

			after = rest
		} else {
			w.Value(v)
		}

		frag = after
	}
}

func TestRunComposed(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		values   []any
		c        *fake
		wantSQL  string
		wantArgs []any
	}{
		{"and with the where clause", "SELECT a FROM t WHERE b = :b", []any{"x"}, &fake{where: "c = ?", vals: []any{5}},
			"SELECT a FROM t WHERE (b = $1) AND (c = $2)", []any{"x", 5}},
		{"add a where clause", "SELECT a FROM t GROUP BY a", nil, &fake{where: "c = ? OR d IN (??)", vals: []any{5, []int{1, 2}}},
			"SELECT a FROM t WHERE c = $1 OR d IN ($2, $3) GROUP BY a", []any{5, 1, 2}},
		{"replace order by", "SELECT a FROM t ORDER BY a", nil, &fake{order: "b DESC", mode: OrderReplace},
			"SELECT a FROM t ORDER BY b DESC", nil},
		{"append to order by", "SELECT a FROM t ORDER BY f(:x)", []any{1}, &fake{order: "b", mode: OrderAppend},
			"SELECT a FROM t ORDER BY f($1), b", []any{1}},
		{"add order by", "SELECT a FROM t", nil, &fake{order: "b NULLS LAST", mode: OrderAppend},
			"SELECT a FROM t ORDER BY b NULLS LAST", nil},
		{"add limit and offset", "SELECT a FROM t ORDER BY a", nil, &fake{limit: 10, offset: 20},
			"SELECT a FROM t ORDER BY a LIMIT $1 OFFSET $2", []any{10, 20}},
		{"replace limit, keep locking", "SELECT a FROM t LIMIT 5 FOR UPDATE", nil, &fake{limit: 10},
			"SELECT a FROM t LIMIT $1 FOR UPDATE", []any{10}},
		{"add limit next to an offset", "SELECT a FROM t OFFSET 5", nil, &fake{limit: 10},
			"SELECT a FROM t OFFSET 5 LIMIT $1", []any{10}},
		{"add offset next to a limit after locking", "SELECT a FROM t FOR UPDATE LIMIT 1", nil, &fake{offset: 2},
			"SELECT a FROM t FOR UPDATE LIMIT 1 OFFSET $1", []any{2}},
		{"order and limit on a union", "SELECT 1 UNION SELECT 2", nil, &fake{order: "1", mode: OrderReplace, limit: 3},
			"SELECT 1 UNION SELECT 2 ORDER BY 1 LIMIT $1", []any{3}},
		{"numbering follows the query's params", "SELECT a FROM t WHERE id IN (:ids) LIMIT :n", []any{[]int{7, 8}, 9},
			&fake{where: "b = ?", vals: []any{"y"}, offset: 3},
			"SELECT a FROM t WHERE (id IN ($1, $2)) AND (b = $3) LIMIT $4 OFFSET $5", []any{7, 8, "y", 9, 3}},
		{"tenant filter on update", "UPDATE t SET a = :a WHERE id = :id RETURNING id", []any{1, 2},
			&fake{where: "org = ?", vals: []any{3}},
			"UPDATE t SET a = $1 WHERE (id = $2) AND (org = $3) RETURNING id", []any{1, 2, 3}},
		{"where on a delete without one", "DELETE FROM t RETURNING id", nil, &fake{where: "org = ?", vals: []any{3}},
			"DELETE FROM t WHERE org = $1 RETURNING id", []any{3}},
		{"only the top level", "WITH c AS (SELECT 1 WHERE p) SELECT * FROM c", nil, &fake{where: "q"},
			"WITH c AS (SELECT 1 WHERE p) SELECT * FROM c WHERE q", nil},
		{"nothing composed", "SELECT a FROM t WHERE b = :b ORDER BY a", []any{1}, &fake{},
			"SELECT a FROM t WHERE b = $1 ORDER BY a", []any{1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := tt.c.render(compileSQL(t, tt.src), tt.values, false)
			if err != nil {
				t.Fatal(err)
			}

			if sql != tt.wantSQL || !equalArgs(args, tt.wantArgs) {
				t.Errorf("composed =\n%s %v\nwant\n%s %v", sql, args, tt.wantSQL, tt.wantArgs)
			}

			checkNumbering(t, sql, args)
		})
	}
}

func TestRunComposedErrors(t *testing.T) {
	boom := errors.New("boom")

	tests := []struct {
		src  string
		vals []any
		c    *fake
		want string
	}{
		{"SELECT 1 UNION SELECT 2", nil, &fake{where: "x"},
			"a WHERE condition can only be added to a SELECT, UPDATE or DELETE"},
		{"INSERT INTO t VALUES (1)", nil, &fake{where: "x"},
			"a WHERE condition can only be added to a SELECT, UPDATE or DELETE"},
		{"UPDATE t SET a = 1", nil, &fake{limit: 1}, "ORDER BY, LIMIT and OFFSET can only be added to a query"},
		{"SELECT a FROM t FETCH FIRST 1 ROWS ONLY", nil, &fake{limit: 1},
			"LIMIT can't be added to a query that uses FETCH"},
		{"SELECT a FROM t ORDER BY f(:x)", []any{1}, &fake{order: "b", mode: OrderReplace},
			"can't replace an ORDER BY that holds a param"},
		{"SELECT a FROM t LIMIT :n", []any{1}, &fake{limit: 5}, "can't replace a LIMIT that holds a param"},
		{"SELECT a FROM t OFFSET :n", []any{1}, &fake{offset: 5}, "can't replace an OFFSET that holds a param"},
		{"SELECT a FROM t", nil, &fake{where: "x = ?", failure: boom}, "boom"},
		{"SELECT a FROM t", nil, &fake{order: "x", mode: OrderReplace, failure: boom}, "boom"},
		{"SELECT a FROM t", nil, &fake{where: "x IN (??)", vals: []any{[]int{}}}, "is an empty list"},
		{"SELECT a FROM t", []any{1}, &fake{}, "got 1 values for 0 params"},
		{"UPDATE t SET a = 1 WHERE CURRENT OF c", nil, &fake{where: "x"},
			"a WHERE condition can't be added to WHERE CURRENT OF"},
	}

	for _, tt := range tests {
		if _, _, err := tt.c.render(compileSQL(t, tt.src), tt.vals, false); err == nil || err.Error() != tt.want {
			t.Errorf("composing %q: error = %v, want %q", tt.src, err, tt.want)
		}
	}
}

func TestRunCount(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		values   []any
		c        *fake
		wantSQL  string
		wantArgs []any
	}{
		{"drops the tail", "SELECT a FROM t WHERE b = :b ORDER BY a LIMIT :n OFFSET 5 FOR UPDATE", []any{1, 10},
			&fake{where: "c = ?", vals: []any{2}, order: "z", mode: OrderReplace, limit: 3},
			"SELECT count(*) FROM (SELECT a FROM t WHERE (b = $1) AND (c = $2)) AS t", []any{1, 2}},
		{"union", "SELECT a FROM x UNION SELECT a FROM y ORDER BY 1 FETCH FIRST 5 ROWS ONLY", nil, nil,
			"SELECT count(*) FROM (SELECT a FROM x UNION SELECT a FROM y) AS t", nil},
		{"group by counts groups", "SELECT a, count(*) FROM t GROUP BY a", nil, nil,
			"SELECT count(*) FROM (SELECT a, count(*) FROM t GROUP BY a) AS t", nil},
		{"a name in where and the tail", "SELECT a FROM t WHERE b = :n ORDER BY f(:x) LIMIT :n OFFSET :o",
			[]any{1, 2, 3}, &fake{where: "c IN (??)", vals: []any{[]int{4, 5}}},
			"SELECT count(*) FROM (SELECT a FROM t WHERE (b = $1) AND (c IN ($2, $3))) AS t", []any{1, 4, 5}},
		{"params only in the tail", "SELECT a FROM t ORDER BY f(:x) LIMIT :n FOR UPDATE", []any{1, 2}, nil,
			"SELECT count(*) FROM (SELECT a FROM t) AS t", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, args, err := tt.c.render(compileSQL(t, tt.src), tt.values, true)
			if err != nil {
				t.Fatal(err)
			}

			if sql != tt.wantSQL || !equalArgs(args, tt.wantArgs) {
				t.Errorf("counted =\n%s %v\nwant\n%s %v", sql, args, tt.wantSQL, tt.wantArgs)
			}

			checkNumbering(t, sql, args)
		})
	}

	var none *fake

	if _, _, err := none.render(compileSQL(t, "UPDATE t SET a = 1"), nil, true); err == nil {
		t.Error("counting an UPDATE succeeded")
	}

	if _, _, err := none.render(compileSQL(t, "SELECT 1"), []any{1}, true); err == nil {
		t.Error("counting with too many values succeeded")
	}
}

// fakeFor returns a composition chosen by the bits of ops.
func fakeFor(ops byte) *fake {
	c := &fake{mode: OrderMode(ops % 3)}
	if ops&4 != 0 {
		c.where, c.vals = "x = ? OR y IN (??)", []any{1, []int{2, 3}}
	}

	if ops&8 != 0 {
		c.order = "z"
	}

	if ops&16 != 0 {
		c.limit = 5
	}

	if ops&32 != 0 {
		c.offset = 6
	}

	return c
}

// FuzzCompose composes statements built from the fuzz vocabulary. Any
// composition must render without panicking, refuse only with the known
// errors, and number its placeholders exactly.
func FuzzCompose(f *testing.F) {
	f.Add([]byte{0}, byte(0xff))
	f.Add([]byte{0, 47, 0, 1, 2}, byte(0x3f))

	f.Fuzz(func(t *testing.T, data []byte, ops byte) {
		p, err := syntax.Parse(fuzzSource(data))
		if err != nil {
			return
		}

		tmpl, err := Compile(p)
		if err != nil {
			t.Fatal(err)
		}

		values := make([]any, len(tmpl.names))
		for i := range values {
			values[i] = []int{i, i + 1}
		}

		sql, args, err := fakeFor(ops).render(tmpl, values, ops&64 != 0)
		if err != nil {
			if !strings.Contains(err.Error(), "can") && !strings.Contains(err.Error(), "only a query") {
				t.Fatalf("unexpected error %v", err)
			}

			return
		}

		checkNumbering(t, sql, args)
	})
}

func TestRunReplacedClauseLeavesNoGap(t *testing.T) {
	sql, args, err := (&fake{limit: 9}).render(compileSQL(t, "SELECT a FROM t WHERE b = :b LIMIT 5 OFFSET :o"),
		[]any{1, 2}, false)
	if want := "SELECT a FROM t WHERE b = $1 LIMIT $2 OFFSET $3"; sql != want || !equalArgs(args, []any{1, 9, 2}) || err != nil {
		t.Errorf("replacing LIMIT = %q %v %v, want %q [1 9 2]", sql, args, err, want)
	}
}

func TestRunManyNames(t *testing.T) {
	sql, args, err := (&fake{where: "z = ?", vals: []any{6}}).render(
		compileSQL(t, "SELECT * FROM t WHERE a = :a AND b = :b AND c = :c AND d = :d AND e IN (:e)"),
		[]any{1, 2, 3, 4, []int{5}}, false)

	want := "SELECT * FROM t WHERE (a = $1 AND b = $2 AND c = $3 AND d = $4 AND e IN ($5)) AND (z = $6)"
	if sql != want || !equalArgs(args, []any{1, 2, 3, 4, 5, 6}) || err != nil {
		t.Errorf("composed = %q %v %v, want %q", sql, args, err, want)
	}
}

func TestRunDone(t *testing.T) {
	var r Run
	if err := compileSQL(t, "SELECT 1").Begin(&r, nil, Plan{}); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if r.Next() != Done {
			t.Fatal("Next does not stay Done")
		}
	}

	if sql, _, _ := r.Finish(); sql != "SELECT 1" {
		t.Errorf("Finish = %q, want the SQL written once", sql)
	}
}

func TestPlanComposes(t *testing.T) {
	for _, p := range []Plan{{Where: true}, {Order: OrderAppend}, {Limit: 1}, {Offset: 0}, {Count: true}} {
		if !p.Composes() {
			t.Errorf("%+v.Composes() = false", p)
		}
	}

	if p := (Plan{}); p.Composes() {
		t.Error("the zero Plan composes")
	}
}

func TestRunCountChecksOnlyKeptParams(t *testing.T) {
	var none *fake

	tail := compileSQL(t, "SELECT id FROM users ORDER BY id IN (:ids) LIMIT :n")
	for _, ids := range []any{[]int{}, nil, make([]int, maxArgs+1)} {
		sql, args, err := none.render(tail, []any{ids, 5}, true)
		if want := "SELECT count(*) FROM (SELECT id FROM users) AS t"; sql != want || len(args) != 0 || err != nil {
			t.Errorf("counting with :ids = %T of %d: %q %v %v, want %q", ids, listLenOr(ids), sql, args, err, want)
		}
	}

	if _, _, err := none.render(tail, []any{[]int{}, 5}, false); err == nil || err.Error() != ":ids is an empty list" {
		t.Errorf("rendering a kept empty list: error = %v", err)
	}
}

func TestRunCountChecksNamesItKeeps(t *testing.T) {
	var none *fake

	both := compileSQL(t, "SELECT id FROM users WHERE id IN (:ids) ORDER BY id IN (:ids)")
	if _, _, err := none.render(both, []any{[]int{}}, true); err == nil || err.Error() != ":ids is an empty list" {
		t.Errorf("counting with an empty list the WHERE clause keeps: error = %v", err)
	}

	sql, args, err := none.render(both, []any{[]int{1, 2}}, true)
	if want := "SELECT count(*) FROM (SELECT id FROM users WHERE id IN ($1, $2)) AS t"; sql != want ||
		!equalArgs(args, []any{1, 2}) || err != nil {
		t.Errorf("counting = %q %v %v, want %q", sql, args, err, want)
	}
}

// listLenOr returns the length of a []int, or 0 for anything else.
func listLenOr(v any) int {
	l, _ := v.([]int)

	return len(l)
}

func TestKeptUses(t *testing.T) {
	tmpl := compileSQL(t, "SELECT a FROM t WHERE b IN (:x) AND c = :y ORDER BY f(:x, :z) LIMIT :n FOR UPDATE")
	want := []use{useExpand, useScalar, 0, 0} // x, y, z, n

	if !slices.Equal(tmpl.kept, want) {
		t.Errorf("kept = %v, want %v", tmpl.kept, want)
	}

	if tmpl := compileSQL(t, "UPDATE t SET a = :a"); tmpl.kept != nil {
		t.Errorf("an UPDATE, which can't be counted, has kept uses %v", tmpl.kept)
	}
}
