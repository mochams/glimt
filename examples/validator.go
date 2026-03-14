package main

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// FieldError represents a validation error on a specific field.
type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string {
	return e.Field + ": " + e.Message
}

// Validator is a struct which contains a slice of FieldErrors.
// We can embed this struct in our serializers to provide validation functionality.
type Validator struct {
	Errors []FieldError
}

// Valid() returns true if the FieldErrors slice doesn't contain any entries.
func (v *Validator) Valid() bool {
	return len(v.Errors) == 0
}

// AddError() adds an error to the Errors slice.
func (v *Validator) AddError(err FieldError) {
	// Note: We need to initialize the slice first, if it isn't already
	// initialized.
	if v.Errors == nil {
		v.Errors = make([]FieldError, 0)
	}

	v.Errors = append(v.Errors, err)
}

// CleanField() is a helper method which executes a validation check and adds an error
func (v *Validator) CleanField(ok bool, key, message string) {
	if !ok {
		v.AddError(FieldError{Field: key, Message: message})
	}
}

// NotBlank() returns true if a value is not an empty string.
func NotBlank(value string) bool {
	return strings.TrimSpace(value) != ""
}

// MaxChars() returns true if a value contains no more than n characters.
func MaxChars(value string, n int) bool {
	return utf8.RuneCountInString(value) <= n
}

// PermittedValue() returns true if a value is in a list of specific permitted values.
func PermittedValue[T comparable](value T, permittedValues ...T) bool {
	return slices.Contains(permittedValues, value)
}
