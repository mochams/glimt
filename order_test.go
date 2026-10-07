package glimt

import "testing"

func TestOrder(t *testing.T) {
	reg := mustLoad(t, map[string]string{"q.sql": "-- name: q\nSELECT * FROM t"})

	sql, _, err := reg.Get("q").Bind(nil).
		OrderBy(Desc("a").NullsLast(), Asc("t.b"), Asc(`"C"`).NullsFirst()).Build()
	if err != nil {
		t.Fatal(err)
	}

	if want := `SELECT * FROM t ORDER BY a DESC NULLS LAST, t.b, "C" NULLS FIRST`; sql != want {
		t.Errorf("sql = %q, want %q", sql, want)
	}

	if _, _, err := reg.Get("q").Bind(nil).OrderBy(Asc("a, b")).Build(); errText(err) !=
		`glimt: query "q": "a, b" is not a column reference` {
		t.Errorf("bad column error = %v", err)
	}
}
