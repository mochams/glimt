package integration

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mochams/glimt/internal/syntax"
)

// kwlistEntry matches one entry of Postgres's kwlist.h:
// PG_KEYWORD("order", ORDER, RESERVED_KEYWORD, AS_LABEL).
var kwlistEntry = regexp.MustCompile(`^PG_KEYWORD\("([a-z_]+)", \w+, (\w+)_KEYWORD, \w+\)`)

// categories maps kwlist.h's keyword categories to glimt's.
var categories = map[string]syntax.KeywordCategory{
	"UNRESERVED":     syntax.UnreservedKeyword,
	"COL_NAME":       syntax.ColNameKeyword,
	"TYPE_FUNC_NAME": syntax.TypeFuncNameKeyword,
	"RESERVED":       syntax.ReservedKeyword,
}

// readKwlist returns the category of every keyword in kwlist.h, fetched
// with the regression suite by make oracle-corpus.
func readKwlist(t *testing.T) map[string]syntax.KeywordCategory {
	t.Helper()

	f, err := os.Open(filepath.Join("testdata", "regress", "kwlist.h"))
	if os.IsNotExist(err) {
		t.Skip("no kwlist.h: run make oracle-corpus")
	}

	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	words := map[string]syntax.KeywordCategory{}

	for s := bufio.NewScanner(f); s.Scan(); {
		if m := kwlistEntry.FindStringSubmatch(s.Text()); m != nil {
			words[m[1]] = categories[m[2]]
		}
	}

	if len(words) < 400 {
		t.Fatalf("read %d keywords from kwlist.h, want about 490", len(words))
	}

	return words
}

// TestReservedWords checks syntax.ReservedWord against Postgres: it must
// hold for exactly the reserved and type/function-name keywords.
func TestReservedWords(t *testing.T) {
	for word, c := range readKwlist(t) {
		want := c == syntax.ReservedKeyword || c == syntax.TypeFuncNameKeyword
		if got := syntax.ReservedWord(word); got != want {
			t.Errorf("ReservedWord(%q) = %v; Postgres's category is %v", word, got, c)
		}
	}

	for _, word := range []string{"status", "created_at", "orders"} {
		if syntax.ReservedWord(word) {
			t.Errorf("ReservedWord(%q) = true for a word Postgres doesn't reserve", word)
		}
	}
}

// TestKeywordCategories checks that every keyword in the parser's table has
// the category Postgres gives it.
func TestKeywordCategories(t *testing.T) {
	words := readKwlist(t)

	for k := syntax.Keyword(1); k.Category() != 0; k++ {
		word := strings.ToLower(k.String())
		if c, ok := words[word]; !ok || c != k.Category() {
			t.Errorf("keyword %v has category %v; Postgres's is %v (listed: %v)", k, k.Category(), c, ok)
		}
	}
}
