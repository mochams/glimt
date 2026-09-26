package glimt

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrNotFound is returned by Get when no query has the requested name.
var ErrNotFound = errors.New("glimt: query not found")

// Registry is an in-memory registry of named SQL queries.
// Queries are sanitized once, when they are loaded or added: comments are
// stripped and whitespace is normalized, so retrieving a query does no string work.
//
// Load or add queries at startup. After that, a Registry is safe for concurrent Get calls.
type Registry struct {
	queries map[string]*template
	dialect Dialect
}

// NewRegistry creates a new Registry with an initialized queries map.
func NewRegistry(dialect Dialect) *Registry {
	return &Registry{
		queries: make(map[string]*template),
		dialect: dialect,
	}
}

// Has checks if a query with the given name exists in the registry.
func (r *Registry) Has(name string) bool {
	_, ok := r.queries[name]

	return ok
}

// Queries returns a sorted list of all query names in the registry.
func (r *Registry) Queries() []string {
	queries := make([]string, 0, len(r.queries))
	for name := range r.queries {
		queries = append(queries, name)
	}

	sort.Strings(queries)

	return queries
}

// Get retrieves a Query by name from the registry.
// It returns an error wrapping ErrNotFound if the query is not found.
func (r *Registry) Get(name string) (*Query, error) {
	tpl, ok := r.queries[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
	}

	return &Query{tpl: tpl, dialect: r.dialect}, nil
}

// MustGet retrieves a Query by name from the registry.
// Panics if the query is not found.
func (r *Registry) MustGet(name string) *Query {
	q, err := r.Get(name)
	if err != nil {
		panic(err)
	}

	return q
}

// Query creates a new Query from the given SQL string.
// Used to create ad-hoc queries that are not stored in the registry.
// The SQL is sanitized on every call; for SQL used on every request,
// register it once at startup with Add.
func (r *Registry) Query(sql string) *Query {
	return NewQuery(sql, r.dialect)
}

// Add registers a named query defined in Go code.
// The SQL is sanitized once, exactly like queries loaded from files.
func (r *Registry) Add(name, sql string) error {
	if !validName(name) {
		return fmt.Errorf("glimt: add: invalid query name %q", name)
	}

	tpl, err := parseSQL(sql, r.dialect, true)
	if err != nil {
		return fmt.Errorf("glimt: add %s: %w", name, err)
	}

	if tpl.empty() {
		return fmt.Errorf("glimt: add %s: empty query body", name)
	}

	return r.merge("Add", map[string]*template{name: tpl})
}

// LoadFile reads SQL queries from a single file at the given path.
func (r *Registry) LoadFile(path string) error {
	return r.loadFile(os.DirFS(filepath.Dir(path)), filepath.Base(path), path)
}

// LoadFileFS reads SQL queries from a single file in the given fs.FS.
func (r *Registry) LoadFileFS(fsys fs.FS, path string) error {
	return r.loadFile(fsys, path, path)
}

// Load reads all .sql files in the given directory recursively.
func (r *Registry) Load(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("glimt: read dir %s: %w", dir, err)
	}

	return r.loadDir(os.DirFS(dir), ".", dir)
}

// LoadFS reads all .sql files recursively from the given fs.FS starting at dir.
func (r *Registry) LoadFS(fsys fs.FS, dir string) error {
	return r.loadDir(fsys, dir, "")
}

// loadDir walks dir in fsys and loads every .sql file.
// base is prepended to paths in error messages.
func (r *Registry) loadDir(fsys fs.FS, dir, base string) error {
	return fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("glimt: read dir %s: %w", displayPath(base, path), err)
		}

		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".sql") {
			return nil
		}

		return r.loadFile(fsys, path, displayPath(base, path))
	})
}

// loadFile parses one file and merges its queries into the registry.
// display is the path used in error messages.
func (r *Registry) loadFile(fsys fs.FS, path, display string) error {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		return fmt.Errorf("glimt: open %s: %w", display, err)
	}

	queries, err := parseFile(string(data), r.dialect)
	if err != nil {
		return fmt.Errorf("glimt: parse %s: %w", display, err)
	}

	return r.merge(display, queries)
}

// merge adds queries to the registry. If any name already exists, nothing is added.
func (r *Registry) merge(source string, queries map[string]*template) error {
	for name := range queries {
		if _, exists := r.queries[name]; exists {
			return fmt.Errorf("glimt: duplicate query name %q in %s", name, source)
		}
	}

	maps.Copy(r.queries, queries)

	return nil
}

// displayPath joins base and an fs.FS path for error messages.
func displayPath(base, path string) string {
	if base == "" {
		return path
	}

	return filepath.Join(base, filepath.FromSlash(path))
}
