package glimt

import (
	"errors"
	"strings"
	"testing"
)

func TestLoadError(t *testing.T) {
	inner := errors.New("boom")
	err := &LoadError{File: "a.sql", Line: 3, Col: 7, Err: inner}

	if got := err.Error(); got != "a.sql:3:7: glimt: boom" {
		t.Errorf("Error() = %q", got)
	}

	if !errors.Is(err, inner) {
		t.Error("LoadError does not unwrap to its error")
	}
}

func TestBuildErrorSentinels(t *testing.T) {
	reg := mustLoad(t, map[string]string{"q.sql": `
-- name: q
SELECT * FROM t WHERE a = :a;
-- name: list
SELECT * FROM t WHERE id IN (:ids);
-- name: both
SELECT 1 UNION SELECT 2;`})

	q, list := reg.Get("q"), reg.Get("list")

	tests := []struct {
		name  string
		build func() (string, []any, error)
		want  error
	}{
		{"missing arg", func() (string, []any, error) { return q.Build(nil) }, ErrMissingArg},
		{"unknown arg", func() (string, []any, error) { return q.Build(Args{"a": 1, "b": 2}) }, ErrUnknownArg},
		{"bound missing arg", q.Bind(nil).Build, ErrMissingArg},
		{"empty IN param", func() (string, []any, error) { return list.Build(Args{"ids": []int{}}) }, ErrEmptyList},
		{"nil IN param", func() (string, []any, error) { return list.Build(Args{"ids": nil}) }, ErrNilValue},
		{"empty In", q.Bind(Args{"a": 1}).Where(In("b", []string{})).BuildCount, ErrEmptyList},
		{"nil Eq", q.Bind(Args{"a": 1}).Where(Eq("b", nil)).Build, ErrNilValue},
		{"bad column", q.Bind(Args{"a": 1}).Where(Eq("null", 1)).BuildCount, ErrBadColumn},
		{"unknown field", q.Bind(Args{"a": 1}).OrderBy(Columns{}.Order("x", false)).Build, ErrUnknownField},
		{"not composable", reg.Get("both").Bind(nil).Where(Eq("a", 1)).Build, ErrNotComposable},
		{"limit on a union", reg.Get("both").Bind(nil).Limit(1).Build, nil},
	}

	for _, tt := range tests {
		_, _, err := tt.build()

		switch {
		case tt.want == nil:
			if err != nil {
				t.Errorf("%s: %v", tt.name, err)
			}
		case !errors.Is(err, tt.want):
			t.Errorf("%s: error %v does not match %v", tt.name, err, tt.want)
		case strings.Count(err.Error(), "glimt:") != 1 || !strings.HasPrefix(err.Error(), "glimt: "):
			t.Errorf("%s: error %q does not start with exactly one glimt: prefix", tt.name, err)
		}
	}
}

func TestLoadErrorPrefix(t *testing.T) {
	_, err := Load(mapFS(map[string]string{"q.sql": "-- name: a\nSELECT FROM WHERE;\n-- name: a\nSELECT 'x"}), ".")
	for _, line := range strings.Split(errText(err), "\n") {
		if strings.Count(line, "glimt:") != 1 {
			t.Errorf("load error %q does not hold exactly one glimt: prefix", line)
		}
	}
}
