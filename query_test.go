package glimt

import (
	"slices"
	"testing"
)

// mustLoad loads queries from files, failing the test on error.
func mustLoad(t *testing.T, files map[string]string) *Registry {
	t.Helper()

	reg, err := Load(mapFS(files), ".")
	if err != nil {
		t.Fatal(err)
	}

	return reg
}

func TestQueryBuild(t *testing.T) {
	reg := mustLoad(t, map[string]string{"q.sql": `
-- name: byIDs
SELECT * FROM orders WHERE id IN (:ids) AND org = :org AND (:org > 0);
-- name: plain
SELECT 1`})

	sql, args, err := reg.Get("byIDs").Build(Args{"org": 7, "ids": []int{1, 2}})
	if err != nil {
		t.Fatal(err)
	}

	if want := "SELECT * FROM orders WHERE id IN ($1, $2) AND org = $3 AND ($3 > 0)"; sql != want {
		t.Errorf("sql = %q, want %q", sql, want)
	}

	if want := []any{1, 2, 7}; !slices.Equal(args, want) {
		t.Errorf("args = %v, want %v", args, want)
	}

	if sql, args, err := reg.Get("plain").Build(nil); err != nil || sql != "SELECT 1" || args != nil {
		t.Errorf("Build(nil) = %q, %v, %v", sql, args, err)
	}
}

func TestQueryBuildErrors(t *testing.T) {
	reg := mustLoad(t, map[string]string{"q.sql": "-- name: q\nSELECT * FROM t WHERE id IN (:ids) AND org = :org"})

	tests := []struct {
		args Args
		want string
	}{
		{Args{"ids": []int{1}}, `glimt: query "q": no value for :org`},
		{Args{"ids": []int{1}, "org": 1, "zz": 2, "extra": 3}, `glimt: query "q": no param :extra`},
		{Args{"ids": []int{}, "org": 1}, `glimt: query "q": :ids is an empty list`},
		{nil, `glimt: query "q": no value for :ids`},
	}

	for _, tt := range tests {
		if _, _, err := reg.Get("q").Build(tt.args); err == nil || err.Error() != tt.want {
			t.Errorf("Build(%v) error = %v, want %q", tt.args, err, tt.want)
		}
	}
}

func TestQueryBuildAllocations(t *testing.T) {
	reg := mustLoad(t, map[string]string{"q.sql": "-- name: q\nSELECT * FROM t WHERE a = :a AND b = :b"})
	q, args := reg.Get("q"), Args{"a": 1, "b": "x"}

	// Only the args slice handed to database/sql.
	if n := testing.AllocsPerRun(50, func() { _, _, _ = q.Build(args) }); n != 1 {
		t.Errorf("Build allocates %v times, want 1", n)
	}
}
