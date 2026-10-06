package render

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"slices"
)

// valuerType is the driver.Valuer interface type.
var valuerType = reflect.TypeFor[driver.Valuer]()

// isListType reports whether values of type t expand into their elements: t
// is a slice, but not a byte slice such as []byte or json.RawMessage, and
// not a driver.Valuer, which binds as one value.
func isListType(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && t.Elem().Kind() != reflect.Uint8 && !t.Implements(valuerType)
}

// isList reports whether v expands into its elements.
func isList(v any) bool {
	return v != nil && isListType(reflect.TypeOf(v))
}

// Errors for a value that IN (…) can't take. They have no subject: the
// caller names the param or column.
var (
	errNilList    = fmt.Errorf("is a %w", ErrNilValue)
	errEmptyList  = fmt.Errorf("is an %w", ErrEmptyList)
	errNestedList = errors.New("has elements that are lists, which IN (…) can't take")
)

// listLen returns how many args v binds when written as IN (…): the number
// of its elements when v is a list, or 1 when v is a single value. A nil
// value, an empty list and a list of lists are errors.
func listLen(v any) (int, error) {
	var n int

	switch v := v.(type) {
	case nil:
		return 0, errNilList
	case []int:
		n = len(v)
	case []int64:
		n = len(v)
	case []int32:
		n = len(v)
	case []string:
		n = len(v)
	case []any:
		if slices.ContainsFunc(v, isList) {
			return 0, errNestedList
		}

		n = len(v)
	default:
		return reflectListLen(v)
	}

	return n, checkEmpty(n)
}

// reflectListLen is listLen for the types without a fast path.
func reflectListLen(v any) (int, error) {
	if !isList(v) {
		return 1, nil
	}

	rv := reflect.ValueOf(v)
	if elemsAreLists(rv) {
		return 0, errNestedList
	}

	return rv.Len(), checkEmpty(rv.Len())
}

// elemsAreLists reports whether any element of the list rv is itself a list.
// The element type decides, unless it is an interface.
func elemsAreLists(rv reflect.Value) bool {
	if et := rv.Type().Elem(); et.Kind() != reflect.Interface {
		return isListType(et)
	}

	for i := range rv.Len() {
		if isList(rv.Index(i).Interface()) {
			return true
		}
	}

	return false
}

// checkEmpty reports an empty list, which IN can't take.
func checkEmpty(n int) error {
	if n == 0 {
		return errEmptyList
	}

	return nil
}

// appendList appends to args what v binds for a param written as IN (:name):
// each element when v is a list, or v itself. listLen must have accepted v.
func appendList(args []any, v any) []any {
	switch v := v.(type) {
	case []int:
		return appendAll(args, v)
	case []int64:
		return appendAll(args, v)
	case []int32:
		return appendAll(args, v)
	case []string:
		return appendAll(args, v)
	case []any:
		return append(args, v...)
	}

	if !isList(v) {
		return append(args, v)
	}

	rv := reflect.ValueOf(v)
	for i := range rv.Len() {
		args = append(args, rv.Index(i).Interface())
	}

	return args
}

// appendAll appends each element of list to args.
func appendAll[T any](args []any, list []T) []any {
	for _, e := range list {
		args = append(args, e)
	}

	return args
}
