package main

import (
	"net/http"
	"slices"
	"strings"

	"strconv"
)

// Params parses and validates any values i.e query parameters, form data, etc.
type Params struct {
	Values map[string][]string
	Validator
}

// ParseRequestQuery creates a new Values from the given HTTP request.
func ParseRequestQuery(r *http.Request) *Params {
	return &Params{
		Values: r.URL.Query(),
	}
}

// Get returns the first value for the given key or an empty string if absent.
func (p *Params) Get(key string) string {
	vs := p.Values[key]
	if len(vs) == 0 {
		return ""
	}
	return vs[0]

}

// Has returns true if the given key is present in the query parameters.
func (p *Params) Has(key string) bool {
	_, ok := p.Values[key]
	return ok
}

// addError records a field error — skips duplicates on the same key.
func (p *Params) addError(key, message string) {
	p.AddError(FieldError{Field: key, Message: message})
}

// -------------------
// String
// -------------------

// String returns the value for the given key or the default if absent.
func (p *Params) String(key, def string) string {
	v := p.Get(key)
	if v == "" {
		return def
	}
	return v
}

// RequiredString returns the value for the given key and records an error if absent.
func (p *Params) RequiredString(key string) string {
	v := p.Get(key)
	if v == "" {
		p.addError(key, "is required")
	}
	return v
}

// OneOfString returns the value for the given key if it's in the list of permitted values or the default if absent or invalid.
func (p *Params) OneOfString(key string, def string, permitted ...string) string {
	v := p.Get(key)
	if v == "" {
		return def
	}

	if !PermittedValue(v, permitted...) {
		p.addError(key, "must be one of: "+strings.Join(permitted, ", "))
	}

	return v
}

// -------------------
// Int
// -------------------

// Int returns the integer value for the given key or the default if absent or invalid.
func (p *Params) Int(key string, def int) int {
	v := p.Get(key)
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
func (p *Params) RequiredInt(key string) int {
	v := p.Get(key)
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
func (p *Params) Float64(key string, def float64) float64 {
	v := p.Get(key)
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
func (p *Params) Bool(key string, def bool) bool {
	v := p.Get(key)
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

// OneOf returns true if the value is in the list of permitted values.
func OneOf[T comparable](v T, allowed ...T) bool {
	if !slices.Contains(allowed, v) {
		return false
	}
	return true
}
