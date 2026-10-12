package glimt

import (
	"errors"
	"fmt"
	"slices"

	"github.com/mochams/glimt/internal/render"
)

// Builder adds conditions, ordering and paging to a query for one request.
// Make one with [Query.Bind].
//
// A Builder is a value. Each method returns a new Builder and leaves the one
// it was called on unchanged, so a base Builder can be shared and extended
// per request. Mistakes are reported by [Builder.Build] and
// [Builder.BuildCount] rather than by the method that made them, so calls
// can be chained.
//
// Conditions are ANDed with the query's own WHERE clause and never ORed, so
// a condition written in the query, such as a tenant filter, always holds.
type Builder struct {
	q      *Query
	values []any // the query's param values, in the order of q.params; never changed
	where  short[Pred]
	order  short[Order]
	mode   render.OrderMode
	limit  any // nil when not set
	offset any // nil when not set
	err    error
}

// short is a list whose first few items live inline, so composing a few
// conditions or order terms doesn't allocate. Copying it is safe: an append
// past the inline items never writes to an array another copy shares.
type short[T any] struct {
	inline [4]T
	n      int
	more   []T
}

// with returns the list with items appended.
func (s short[T]) with(items []T) short[T] {
	for _, it := range items {
		if s.n < len(s.inline) {
			s.inline[s.n] = it
		} else {
			s.more = append(slices.Clip(s.more), it)
		}

		s.n++
	}

	return s
}

// at returns item i.
func (s *short[T]) at(i int) T {
	if i < len(s.inline) {
		return s.inline[i]
	}

	return s.more[i-len(s.inline)]
}

// Bind returns a [Builder] for q, with args as the values of its :name
// params. Bind copies the values out of args, so changing the map
// afterwards has no effect. A missing or extra value is reported by Build,
// as with [Query.Build].
func (q *Query) Bind(args Args) Builder {
	values, err := args.values(q.params)

	return Builder{q: q, values: values, err: err}
}

// Where ANDs preds with the query's WHERE clause, and adds a WHERE clause
// when the query has none. Preds passed together, or in several calls, are
// ANDed with each other. Zero Preds, such as those from a false [If], are
// skipped.
//
// SELECT, UPDATE and DELETE take conditions. On other statements, such as a
// UNION, Build returns an error that wraps [ErrNotComposable].
func (b Builder) Where(preds ...Pred) Builder {
	b.where = b.where.with(preds)

	return b
}

// OrderBy replaces the query's ORDER BY with terms, or adds an ORDER BY
// when the query has none. It also drops the terms of any earlier OrderBy
// or ThenBy. Replacing an ORDER BY that holds a :param is an error at
// Build.
func (b Builder) OrderBy(terms ...Order) Builder {
	b.order = short[Order]{}.with(terms)
	b.mode = render.OrderReplace

	return b
}

// ThenBy adds terms after the query's ORDER BY, or after the terms of an
// earlier OrderBy, and adds an ORDER BY when there is none. Use it for a
// unique tiebreaker, such as the primary key, so pages are stable.
func (b Builder) ThenBy(terms ...Order) Builder {
	b.order = b.order.with(terms)
	if b.mode == render.OrderKeep {
		b.mode = render.OrderAppend
	}

	return b
}

// Limit sets the query's LIMIT to n, bound as an arg, replacing any LIMIT
// the query has. A negative n, a LIMIT that holds a :param, and a query
// that uses FETCH are errors at Build.
func (b Builder) Limit(n int) Builder {
	if n < 0 {
		b.err = firstErr(b.err, fmt.Errorf("negative limit %d", n))
	}

	b.limit = n

	return b
}

// Offset sets the query's OFFSET to n, bound as an arg, replacing any
// OFFSET the query has. A negative n and an OFFSET that holds a :param are
// errors at Build.
func (b Builder) Offset(n int) Builder {
	if n < 0 {
		b.err = firstErr(b.err, fmt.Errorf("negative offset %d", n))
	}

	b.offset = n

	return b
}

// firstErr returns the first of two errors that is not nil.
func firstErr(first, second error) error {
	if first != nil {
		return first
	}

	return second
}

// Build returns the composed query's SQL and the args to pass with it. When
// nothing was added, it renders the query as written.
func (b Builder) Build() (string, []any, error) {
	if err := b.check(); err != nil {
		return "", nil, err
	}

	p := b.plan(false)
	if !p.Composes() {
		// Render returns the values as the args when no param expands, and
		// every Builder made from b shares them, so they are copied.
		sql, args, err := b.q.tmpl.Render(slices.Clone(b.values))

		return sql, args, b.wrap(err)
	}

	return b.build(&p)
}

// BuildCount returns SQL that counts the rows the composed query returns,
// as SELECT count(*) FROM (…) AS t, for the total in paging. The counted
// query leaves out ORDER BY, LIMIT, OFFSET, FETCH and locking clauses, and
// the args leave out the values only those clauses use.
func (b Builder) BuildCount() (string, []any, error) {
	if err := b.check(); err != nil {
		return "", nil, err
	}

	p := b.plan(true)

	return b.build(&p)
}

// check reports a Builder that can't be built: not made by Bind, or with an
// error from Bind, Limit or Offset.
func (b *Builder) check() error {
	if b.q == nil {
		return errors.New("glimt: Builder not made by Query.Bind")
	}

	if b.err != nil {
		return buildError(b.q.name, b.err)
	}

	return nil
}

// plan returns what b adds to its query, asking each question once.
func (b *Builder) plan(count bool) render.Plan {
	p := render.Plan{Limit: b.limit, Offset: b.offset, Count: count}

	for i := range b.where.n {
		if !b.where.at(i).empty() {
			p.Where = true

			break
		}
	}

	if b.order.n > 0 {
		p.Order = b.mode
	}

	return p
}

// build renders the query as p says, writing b's conditions and order terms
// at the holes the render stops at.
func (b *Builder) build(p *render.Plan) (string, []any, error) {
	var r render.Run
	if err := b.q.tmpl.Begin(&r, b.values, *p); err != nil {
		return "", nil, b.wrap(err)
	}

	for h := r.Next(); h != render.Done; h = r.Next() {
		var err error

		switch h {
		case render.HoleWhere:
			err = b.writeWhere(r.Writer())
		case render.HoleOrder:
			err = b.writeOrder(r.Writer())
		}

		if err != nil {
			return "", nil, b.wrap(err)
		}
	}

	sql, args, err := r.Finish()

	return sql, args, b.wrap(err)
}

// writeWhere writes the added conditions, ANDed together. Render puts them
// in parentheses of their own when it ANDs them with the query's WHERE.
func (b *Builder) writeWhere(w *render.Writer) error {
	j := joiner{w: w, sep: " AND "}
	for i := range b.where.n {
		if err := j.add(b.where.at(i)); err != nil {
			return err
		}
	}

	return nil
}

// writeOrder writes the order terms, separated by ", ".
func (b *Builder) writeOrder(w *render.Writer) error {
	for i := range b.order.n {
		if i > 0 {
			w.SQL(", ")
		}

		if err := b.order.at(i).write(w); err != nil {
			return err
		}
	}

	return nil
}

// wrap adds the query's name to a render error.
func (b *Builder) wrap(err error) error {
	if err == nil {
		return nil
	}

	return buildError(b.q.name, err)
}
