package glimt

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"

	"github.com/mochams/glimt/internal/render"
	"github.com/mochams/glimt/internal/syntax"
)

// Registry holds the queries loaded by [Load], by name. It never changes
// after Load, so it is safe to share between goroutines.
type Registry struct {
	queries map[string]*Query
}

// Load reads every .sql file under dir in fsys, recursively, and parses and
// compiles each query in it. fsys is usually an [embed.FS].
//
// A query starts at a line "-- name: <name>" and runs to the next one. End
// each query with ";": glimt rejects a second statement in one query, and
// the ";" is how it tells where the first one ends. Only comments may come
// before a file's first annotation, and a line that looks like an
// annotation but isn't one, such as "-- Name: x", is an error.
//
// Load doesn't stop at the first problem. It reports one error per broken
// query, each a [*LoadError] with its file, line and column, joined with
// [errors.Join]. Finding no queries at all is an error too.
func Load(fsys fs.FS, dir string) (*Registry, error) {
	l := &loader{queries: map[string]*Query{}, defined: map[string]string{}}

	err := fs.WalkDir(fsys, dir, func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(name) != ".sql" {
			return err
		}

		src, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}

		l.file(name, string(src))

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("glimt: %w", err)
	}

	if len(l.errs) > 0 {
		return nil, errors.Join(l.errs...)
	}

	if len(l.queries) == 0 {
		return nil, fmt.Errorf("glimt: no queries in %q", dir)
	}

	return &Registry{queries: l.queries}, nil
}

// Get returns the named query. It panics when there is none, since asking
// for a query that isn't in the .sql files is a bug in the code. Call Get
// for every query right after Load, so a missing one fails at startup
// rather than in a request:
//
//	type queries struct{ listOrders, cancelOrder *glimt.Query }
//
//	q := queries{listOrders: reg.Get("listOrders"), cancelOrder: reg.Get("cancelOrder")}
func (r *Registry) Get(name string) *Query {
	q, ok := r.queries[name]
	if !ok {
		panic(fmt.Sprintf("glimt: no query named %q", name))
	}

	return q
}

// Lookup returns the named query and whether it exists. Use it when the
// name is chosen at run time, and [Registry.Get] for names written in code.
func (r *Registry) Lookup(name string) (*Query, bool) {
	q, ok := r.queries[name]

	return q, ok
}

// Names returns the names of all queries, sorted.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.queries))
	for name := range r.queries {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// loader collects the queries and errors of a Load.
type loader struct {
	queries map[string]*Query
	defined map[string]string // where each name was defined: file:line
	errs    []error
}

// file loads the queries in one .sql file.
func (l *loader) file(name, src string) {
	chunks, err := syntax.SplitFile(src)
	if err != nil {
		l.fail(name, src, offsetOf(err), err)

		return
	}

	for _, c := range chunks {
		l.chunk(name, src, c)
	}
}

// chunk compiles one query and records it, or the problem with it.
func (l *loader) chunk(file, src string, c syntax.Chunk) {
	here := fmt.Sprintf("%s:%d", file, c.Line)

	switch first, dup := l.defined[c.Name]; {
	case !validName(c.Name):
		l.failAt(file, c.Line, fmt.Errorf("invalid query name %q: use letters, digits and _", c.Name))

		return
	case dup:
		l.failAt(file, c.Line, fmt.Errorf("duplicate query name %q, first defined at %s", c.Name, first))

		return
	case c.Empty:
		l.failAt(file, c.Line, fmt.Errorf("query %q has no SQL", c.Name))

		return
	}

	l.defined[c.Name] = here

	q, err := compile(c.Name, c.Body)
	if err != nil {
		l.fail(file, src, c.Offset+offsetOf(err), err)

		return
	}

	l.queries[c.Name] = q
}

// compile parses and compiles one query.
func compile(name, body string) (*Query, error) {
	p, err := syntax.Parse(body)
	if err != nil {
		return nil, err
	}

	tmpl, err := render.Compile(p)
	if err != nil {
		return nil, err
	}

	return &Query{name: name, tmpl: tmpl, params: tmpl.Names()}, nil
}

// fail records err at byte offset off of a file.
func (l *loader) fail(file, src string, off int, err error) {
	line, col := syntax.Position(src, off)
	l.errs = append(l.errs, &LoadError{File: file, Line: line, Col: col, Err: err})
}

// failAt records err at the start of a line of a file.
func (l *loader) failAt(file string, line int, err error) {
	l.errs = append(l.errs, &LoadError{File: file, Line: line, Col: 1, Err: err})
}

// offsetOf returns the source offset of a syntax error, or 0.
func offsetOf(err error) int {
	var serr *syntax.Error
	if errors.As(err, &serr) {
		return int(serr.Pos)
	}

	return 0
}

// validName reports whether name is a valid query name: ASCII letters,
// digits and _, not starting with a digit.
func validName(name string) bool {
	if name == "" || '0' <= name[0] && name[0] <= '9' {
		return false
	}

	for i := range len(name) {
		if !isNameByte(name[i]) {
			return false
		}
	}

	return true
}

// isNameByte reports whether c can be part of a query name.
func isNameByte(c byte) bool {
	return 'a' <= c|0x20 && c|0x20 <= 'z' || '0' <= c && c <= '9' || c == '_'
}
