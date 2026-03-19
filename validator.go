package glimt

import (
	"strings"
	"unicode"
)

// notBlank returns false if a value is an empty string.
func notBlank(value string) bool {
	return strings.TrimSpace(value) != ""
}

// validName checks if a query name is valid (consists of letters, digits,
// and underscores, and does not start with a digit).
func validName(name string) bool {
	if !notBlank(name) {
		return false
	}

	for i, c := range name {
		if !unicode.IsLetter(c) && c != '_' && (i == 0 || !unicode.IsDigit(c)) {
			return false
		}
	}
	return true
}
