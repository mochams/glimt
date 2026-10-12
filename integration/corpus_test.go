package integration

import (
	"slices"
	"testing"
)

func TestSplitSQL(t *testing.T) {
	got := splitSQL("SELECT 1; SELECT ';'; -- a comment\nSELECT $$a;b$$;\n\n")
	want := []string{"SELECT 1", "SELECT ';'", "-- a comment\nSELECT $$a;b$$"}

	if !slices.Equal(got, want) {
		t.Errorf("splitSQL = %q, want %q", got, want)
	}
}

func TestDecodeLines(t *testing.T) {
	got, err := decodeLines("\"SELECT 1\"\n\n\"SELECT 'a\\nb'\"\n")
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"SELECT 1", "SELECT 'a\nb'"}; !slices.Equal(got, want) {
		t.Errorf("decodeLines = %q, want %q", got, want)
	}

	if _, err := decodeLines("not json"); err == nil {
		t.Error("want an error for a line that is not a JSON string")
	}
}

func TestCorpusFiles(t *testing.T) {
	for _, path := range []string{"testdata/glimt.jsonl", "testdata/queries.sql"} {
		statements, err := loadCorpus(path)
		if err != nil || len(statements) == 0 {
			t.Errorf("loadCorpus(%q) = %d statements, %v", path, len(statements), err)
		}
	}
}
