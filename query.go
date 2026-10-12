package glimt

import (
	"github.com/mochams/glimt/internal/render"
)

// Query is a named query from a .sql file, parsed and compiled by [Load].
// It never changes after Load, so it is safe to share between goroutines.
type Query struct {
	name   string
	tmpl   *render.Template
	params []string // param names, in the order the template takes their values
}

// Name returns the query's name.
func (q *Query) Name() string {
	return q.name
}

// Build returns the query's SQL, with each :name param written as a $n
// placeholder, and the args to pass with it. args must hold a value for
// every param and nothing else: a missing value is an error that wraps
// [ErrMissingArg], and an extra one wraps [ErrUnknownArg].
//
// Build runs the query as written. To add conditions, ordering or paging,
// use [Query.Bind].
func (q *Query) Build(args Args) (string, []any, error) {
	values, err := args.values(q.params)
	if err != nil {
		return "", nil, buildError(q.name, err)
	}

	sql, out, err := q.tmpl.Render(values)
	if err != nil {
		return "", nil, buildError(q.name, err)
	}

	return sql, out, nil
}
