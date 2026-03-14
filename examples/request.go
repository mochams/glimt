package main

import (
	"net/http"
	"net/url"
	"strconv"
)

// RequestParams parses and validates query parameters from an HTTP request.
type RequestParams struct {
	values url.Values
	errors []FieldError
}

// FromRequest creates a new RequestParams from the given HTTP request.
func FromRequest(r *http.Request) *RequestParams {
	return &RequestParams{
		values: r.URL.Query(),
		errors: []FieldError{},
	}
}

// Valid returns true if no errors have been recorded.
func (p *RequestParams) Valid() bool {
	return len(p.errors) == 0
}

// Errors returns the recorded field errors.
func (p *RequestParams) Errors() []FieldError {
	return p.errors
}

// addError records a field error — skips duplicates on the same key.
func (p *RequestParams) addError(key, message string) {
	for _, e := range p.errors {
		if e.Field == key {
			return
		}
	}
	p.errors = append(p.errors, FieldError{Field: key, Message: message})
}

// -------------------
// String
// -------------------

// String returns the value for the given key or the default if absent.
func (p *RequestParams) String(key, def string) string {
	v := p.values.Get(key)
	if v == "" {
		return def
	}
	return v
}

// RequiredString returns the value for the given key and records an error if absent.
func (p *RequestParams) RequiredString(key string) string {
	v := p.values.Get(key)
	if v == "" {
		p.addError(key, "is required")
	}
	return v
}

// -------------------
// Int
// -------------------

// Int returns the integer value for the given key or the default if absent or invalid.
func (p *RequestParams) Int(key string, def int) int {
	v := p.values.Get(key)
	if v == "" {
		return def
	}

	i, err := strconv.Atoi(v)
	if err != nil {
		p.addError(key, "must be an integer")
		return def
	}

	return i
}

// RequiredInt returns the integer value for the given key and records an error if absent or invalid.
func (p *RequestParams) RequiredInt(key string) int {
	v := p.values.Get(key)
	if v == "" {
		p.addError(key, "is required")
		return 0
	}

	i, err := strconv.Atoi(v)
	if err != nil {
		p.addError(key, "must be an integer")
		return 0
	}

	return i
}

// -------------------
// Float64
// -------------------

// Float64 returns the float value for the given key or the default if absent or invalid.
func (p *RequestParams) Float64(key string, def float64) float64 {
	v := p.values.Get(key)
	if v == "" {
		return def
	}

	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		p.addError(key, "must be a number")
		return def
	}

	return f
}

// -------------------
// Bool
// -------------------

// Bool returns the boolean value for the given key or the default if absent or invalid.
func (p *RequestParams) Bool(key string, def bool) bool {
	v := p.values.Get(key)
	if v == "" {
		return def
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		p.addError(key, "must be a boolean")
		return def
	}

	return b
}
