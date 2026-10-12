package glimt

import (
	"fmt"
	"slices"
)

// Args holds the values of a query's :name params, keyed by name without
// the colon:
//
//	glimt.Args{"org": 7, "ids": []int{4, 8}}
//
// Values go to the driver as they are, so anything the driver can bind
// works, including a nil pointer for NULL. A slice given for a param
// written as IN (:name) expands into one placeholder per element.
type Args map[string]any

// values returns the values of args in the order of names. A name without a
// value, and a value without a name, are errors.
func (a Args) values(names []string) ([]any, error) {
	if len(names) == 0 && len(a) == 0 {
		return nil, nil
	}

	out := make([]any, len(names))
	for i, name := range names {
		v, ok := a[name]
		if !ok {
			return nil, fmt.Errorf("%w for :%s", ErrMissingArg, name)
		}

		out[i] = v
	}

	if len(a) > len(names) {
		return nil, a.unknown(names)
	}

	return out, nil
}

// unknown reports the first name in args, in sorted order, that the query doesn't have.
func (a Args) unknown(names []string) error {
	keys := make([]string, 0, len(a))
	for k := range a {
		if !slices.Contains(names, k) {
			keys = append(keys, k)
		}
	}

	slices.Sort(keys)

	return fmt.Errorf("%w :%s", ErrUnknownArg, keys[0])
}
