package syntax

import "testing"

func TestLookupKeyword(t *testing.T) {
	tests := []struct {
		text string
		want Keyword
	}{
		{"select", SELECT},
		{"SELECT", SELECT},
		{"SeLeCt", SELECT},
		{"materialized", MATERIALIZED},
		{"on", ON},
		{"users", 0},
		{"selects", 0},
		{"selec", 0},
		{"s", 0},
		{"", 0},
		{"select1", 0},
		{"materializedx", 0},
		{"sélect", 0},
	}

	for _, tt := range tests {
		if got := LookupKeyword(tt.text); got != tt.want {
			t.Errorf("LookupKeyword(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestLookupKeywordCoversEveryKeyword(t *testing.T) {
	for k := Keyword(1); k < numKeywords; k++ {
		name := k.String()
		if got := LookupKeyword(name); got != k {
			t.Errorf("LookupKeyword(%q) = %v, want %v", name, got, k)
		}

		if len(name) > maxKeywordLen {
			t.Errorf("%s is longer than maxKeywordLen", name)
		}
	}
}

func TestLookupKeywordDoesNotAllocate(t *testing.T) {
	allocs := testing.AllocsPerRun(100, func() {
		LookupKeyword("Returning")
		LookupKeyword("orders")
	})

	if allocs != 0 {
		t.Errorf("LookupKeyword allocates %v times per call", allocs)
	}
}

func TestKeywordCategories(t *testing.T) {
	for k := Keyword(1); k < numKeywords; k++ {
		if keywordCategories[k] == 0 {
			t.Errorf("%v has no category", k)
		}
	}

	tests := []struct {
		kw              Keyword
		reserved, colID bool
	}{
		{SELECT, true, false},
		{IS, false, false}, // a type or function name in Postgres, not reserved
		{VALUES, false, true},
		{SET, false, true},
		{SOURCE, false, true},
		{USER, true, false},
		{0, false, false},
		{numKeywords, false, false},
	}

	for _, tt := range tests {
		if tt.kw.reserved() != tt.reserved || tt.kw.colID() != tt.colID {
			t.Errorf("%v: reserved %v, colID %v; want %v, %v", tt.kw, tt.kw.reserved(), tt.kw.colID(), tt.reserved, tt.colID)
		}
	}
}

func TestKwSet(t *testing.T) {
	s := kws(ALL, WITHIN).with(CALL)
	for k := Keyword(0); k <= numKeywords; k++ {
		if want := k == ALL || k == WITHIN || k == CALL; s.has(k) != want {
			t.Errorf("has(%v) = %v, want %v", k, s.has(k), want)
		}
	}
}

func TestReservedKeywords(t *testing.T) {
	tests := []struct {
		kw   Keyword
		want bool
	}{
		{SELECT, true},
		{FROM, true},
		{RETURNING, true},
		{ONLY, true},
		{SET, false},
		{CONFLICT, false},
		{VALUES, false},
		{COMMENT, false},
		{IS, false},
		{0, false},
	}

	for _, tt := range tests {
		if got := tt.kw.reserved(); got != tt.want {
			t.Errorf("%v.reserved() = %v, want %v", tt.kw, got, tt.want)
		}
	}
}

// TestTerminatorsAreReserved guards the rule that lets column names never end
// a clause: every keyword in a stop set is reserved.
func TestTerminatorsAreReserved(t *testing.T) {
	sets := []stopSet{noStops, listStops, clauseStops, clauseItemStops, targetStops}

	for _, s := range sets {
		for k := Keyword(1); k < numKeywords; k++ {
			if s.kws.has(k) && !k.reserved() {
				t.Errorf("terminator %v is not reserved", k)
			}
		}
	}
}

func TestKindAndKeywordStrings(t *testing.T) {
	tests := []struct {
		got, want string
	}{
		{EOF.String(), "EOF"},
		{LPAREN.String(), "("},
		{Kind(200).String(), "Kind(?)"},
		{SELECT.String(), "SELECT"},
		{Keyword(0).String(), "Keyword(?)"},
		{numKeywords.String(), "Keyword(?)"},
	}

	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}
