package glimt

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// mapFS builds a filesystem of .sql files from file names and contents.
func mapFS(files map[string]string) fstest.MapFS {
	fsys := fstest.MapFS{}
	for name, src := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(src)}
	}

	return fsys
}

func TestLoad(t *testing.T) {
	fsys := mapFS(map[string]string{
		"q/a.sql":        "-- name: one\nSELECT 1;\n-- name: two\nSELECT :x;",
		"q/nested/b.sql": "-- name: three\nUPDATE t SET a = :a WHERE id = :id",
		"q/readme.txt":   "-- name: ignored\nSELECT 1",
		"other/c.sql":    "-- name: outside\nSELECT 1",
	})

	reg, err := Load(fsys, "q")
	if err != nil {
		t.Fatal(err)
	}

	if got, want := reg.Names(), []string{"one", "three", "two"}; !slices.Equal(got, want) {
		t.Errorf("Names = %v, want %v", got, want)
	}

	if q := reg.Get("three"); q.Name() != "three" || !slices.Equal(q.params, []string{"a", "id"}) {
		t.Errorf("Get(three) = %q with params %v", q.Name(), q.params)
	}
}

func TestLoadErrors(t *testing.T) {
	fsys := mapFS(map[string]string{
		"a.sql": "-- name: dup\nSELECT 1\n-- name: bad-name\nSELECT 2\n-- name: empty\n-- nothing\n-- name: broken\nSELECT a FROM WHERE",
		"b.sql": "-- name: dup\nSELECT 3",
		"c.sql": "-- name: open\nSELECT 'x",
		"d.sql": "-- name:\nSELECT 1",
	})

	_, err := Load(fsys, ".")
	if err == nil {
		t.Fatal("Load succeeded, want errors")
	}

	want := []string{
		`a.sql:3:1: glimt: invalid query name "bad-name": use letters, digits and _`,
		`a.sql:5:1: glimt: query "empty" has no SQL`,
		`a.sql:8:15: glimt: expected expression after FROM, found "WHERE"`,
		`b.sql:1:1: glimt: duplicate query name "dup", first defined at a.sql:1`,
		`c.sql:2:8: glimt: unterminated string literal`,
		`d.sql:1:1: glimt: malformed annotation "-- name:": want "-- name: <name>"`,
	}

	if got := strings.Split(err.Error(), "\n"); !slices.Equal(got, want) {
		t.Errorf("Load errors:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	var lerr *LoadError
	if !errors.As(err, &lerr) || lerr.File != "a.sql" {
		t.Errorf("errors.As(*LoadError) = %v", lerr)
	}
}

func TestLoadMissingDir(t *testing.T) {
	if _, err := Load(mapFS(nil), "missing"); err == nil {
		t.Error("Load of a missing directory succeeded")
	}
}

func TestLoadAnnotationMistakes(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"-- Name: first\nSELECT 1;\n-- name: second\nSELECT 2;",
			`q.sql:1:1: glimt: malformed annotation "-- Name: first": want "-- name: <name>"`},
		{"-- name: a\nSELECT 1\n-- name : b\nSELECT 2",
			`q.sql:3:1: glimt: malformed annotation "-- name : b": want "-- name: <name>"`},
		{"-- name: a\nSELECT 1\n--NAME: b\nSELECT 2",
			`q.sql:3:1: glimt: malformed annotation "-- NAME: b": want "-- name: <name>"`},
		{"SELECT 1\n-- name: a\nSELECT 2", `q.sql:1:1: glimt: SQL before the first "-- name:" annotation`},
		{"SELECT 1", `q.sql:1:1: glimt: SQL without a "-- name:" annotation`},
		{"-- name: a\nSELECT 1\nSELECT 2",
			`q.sql:3:1: glimt: a second statement starts at "SELECT": is a "-- name:" annotation missing or misspelled?`},
		{"-- name: a\nUPDATE t SET a = 1;\nUPDATE t SET b = 2;",
			`q.sql:3:1: glimt: a second statement starts at "UPDATE": is a "-- name:" annotation missing or misspelled?`},
	}

	for _, tt := range tests {
		_, err := Load(mapFS(map[string]string{"q.sql": tt.src}), ".")
		if err == nil || err.Error() != tt.want {
			t.Errorf("Load(%q) error = %v\nwant %s", tt.src, err, tt.want)
		}

		var lerr *LoadError
		if !errors.As(err, &lerr) {
			t.Errorf("Load(%q) error is not a *LoadError", tt.src)
		}
	}
}

func TestLoadComments(t *testing.T) {
	reg := mustLoad(t, map[string]string{"q.sql": "-- leading comment\n" +
		"-- name of the customer is in c.name\n-- name: a\nSELECT c.name FROM c;"})

	if got := reg.Names(); !slices.Equal(got, []string{"a"}) {
		t.Errorf("Names() = %v, want [a]", got)
	}
}

func TestLoadNoQueries(t *testing.T) {
	for _, files := range []map[string]string{{}, {"a.txt": "-- name: a\nSELECT 1"}, {"a.sql": "-- nothing\n"}} {
		if _, err := Load(mapFS(files), "."); err == nil || err.Error() != `glimt: no queries in "."` {
			t.Errorf("Load(%v) error = %v", files, err)
		}
	}
}

func TestGetPanics(t *testing.T) {
	reg, err := Load(mapFS(map[string]string{"a.sql": "-- name: one\nSELECT 1"}), ".")
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if r := recover(); r != `glimt: no query named "two"` {
			t.Errorf("recover() = %v", r)
		}
	}()

	reg.Get("two")
}

func TestLookup(t *testing.T) {
	reg := mustLoad(t, map[string]string{"a.sql": "-- name: one\nSELECT 1"})

	if q, ok := reg.Lookup("one"); !ok || q.Name() != "one" {
		t.Errorf("Lookup(one) = %v, %v", q, ok)
	}

	if q, ok := reg.Lookup("two"); ok || q != nil {
		t.Errorf("Lookup(two) = %v, %v; want nil, false", q, ok)
	}
}

func TestValidName(t *testing.T) {
	for name, want := range map[string]bool{
		"listOrders": true, "_x": true, "a1_b": true, "": false, "1a": false, "a-b": false, "a.b": false, "héllo": false,
	} {
		if got := validName(name); got != want {
			t.Errorf("validName(%q) = %v, want %v", name, got, want)
		}
	}
}
