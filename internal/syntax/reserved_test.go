package syntax

import "testing"

func TestReservedWord(t *testing.T) {
	for word, want := range map[string]bool{
		"order": true, "ORDER": true, "Order": true, "null": true, "true": true, "current_date": true,
		"CURRENT_TIMESTAMP": true, "to": true, "check": true, "left": true, "user": true, "ilike": true,
		"status": false, "name": false, "offsets": false, "current?date": false, "current_datE": true,
		"": false, "a": false, "current_timestamps": false, "values": false, "update": false, "id": false,
	} {
		if got := ReservedWord(word); got != want {
			t.Errorf("ReservedWord(%q) = %v, want %v", word, got, want)
		}
	}
}

func TestReservedWordsCoverKeywords(t *testing.T) {
	for k := Keyword(1); k < numKeywords; k++ {
		c := k.Category()
		want := c == ReservedKeyword || c == TypeFuncNameKeyword

		if got := ReservedWord(k.String()); got != want {
			t.Errorf("ReservedWord(%v) = %v, but it is a keyword of category %v", k, got, c)
		}
	}
}

func TestReservedWordsSorted(t *testing.T) {
	for i := 1; i < len(reservedWords); i++ {
		if reservedWords[i-1] >= reservedWords[i] {
			t.Errorf("reservedWords: %q before %q", reservedWords[i-1], reservedWords[i])
		}

		if len(reservedWords[i]) > maxReservedLen {
			t.Errorf("reservedWords: %q is longer than maxReservedLen", reservedWords[i])
		}
	}
}

func BenchmarkReservedWord(b *testing.B) {
	for b.Loop() {
		_ = ReservedWord("created_at")
	}
}
