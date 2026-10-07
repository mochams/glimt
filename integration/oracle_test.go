package integration

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// shown is how many findings of each check a failing test prints.
const shown = 10

// runOracle checks every statement and fails with a sample of the findings
// for each check.
func runOracle(t *testing.T, statements []string) {
	t.Helper()

	byCheck := map[string][]finding{}
	for _, src := range statements {
		for _, f := range check(src) {
			byCheck[f.check] = append(byCheck[f.check], f)
		}
	}

	checks := make([]string, 0, len(byCheck))
	for c := range byCheck {
		checks = append(checks, c)
	}

	slices.Sort(checks)

	for _, c := range checks {
		found := byCheck[c]

		var b strings.Builder
		fmt.Fprintf(&b, "%s: %d of %d statements disagree with Postgres", c, len(found), len(statements))

		for _, f := range found[:min(len(found), shown)] {
			fmt.Fprintf(&b, "\n  %s\n    %q", f.msg, f.src)
		}

		t.Error(b.String())
	}
}

// corpusFile loads one corpus file, failing the test on error.
func corpusFile(t *testing.T, path string) []string {
	t.Helper()

	statements, err := loadCorpus(path)
	if err != nil {
		t.Fatal(err)
	}

	return statements
}

func TestOracleGlimt(t *testing.T) {
	runOracle(t, corpusFile(t, "testdata/glimt.jsonl"))
}

func TestOracleQueries(t *testing.T) {
	runOracle(t, corpusFile(t, "testdata/queries.sql"))
}

func TestOracleRegress(t *testing.T) {
	files := regressFiles()
	if len(files) == 0 {
		t.Skip("no regression files: run make oracle-corpus")
	}

	statements := make([]string, 0, 50000)
	for _, f := range files {
		statements = append(statements, corpusFile(t, f)...)
	}

	t.Logf("%d statements from %d files", len(statements), len(files))
	runOracle(t, statements)
}

func FuzzOracle(f *testing.F) {
	for _, path := range []string{"testdata/glimt.jsonl", "testdata/queries.sql"} {
		statements, err := loadCorpus(path)
		if err != nil {
			f.Fatal(err)
		}

		for _, s := range statements {
			f.Add(s)
		}
	}

	f.Fuzz(func(t *testing.T, src string) {
		for _, found := range check(src) {
			t.Errorf("%s: %s\n%q", found.check, found.msg, found.src)
		}
	})
}
