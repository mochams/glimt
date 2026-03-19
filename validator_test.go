package glimt

import "testing"

func TestValidator_validName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid name", "getUserByID", true},
		{"name with spaces", "get user by id", false},
		{"name with special chars", "get-user-by-id!", false},
		{"empty name", "", false},
		{"name with only spaces", "   ", false},
		{"name with underscores", "get_user_by_id", true},
		{"name starting with underscore", "_getUserByID", true},
		{"name with leading digit", "9getUserByID", false},
		{"name with trailing digit", "getUserByID9", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validName(tt.input); got != tt.want {
				t.Errorf("validName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
