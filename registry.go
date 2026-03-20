package glimt

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Registry is a simple in-memory registry for storing named SQL queries.
// It provides methods to load queries from files and directories
// It provides as well as to retrieve queries by name.
type Registry struct {
	queries map[string]string
	dialect Dialect
}

// NewRegistry creates a new Registry with an initialized queries map.
func NewRegistry(dialect Dialect) *Registry {
	return &Registry{
		queries: make(map[string]string),
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
// It returns an error if the query is not found.
func (r *Registry) Get(name string) (*Query, error) {
	sql, ok := r.queries[name]
	if !ok {
		return nil, fmt.Errorf("query %q not found", name)
	}

	return NewQuery(sql, r.dialect), nil
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
func (r *Registry) Query(sql string) *Query {
	return NewQuery(sql, r.dialect)
}

// LoadFile reads SQL queries from a single file at the given path.
func (r *Registry) LoadFile(path string) error {
	return r.LoadFileFS(os.DirFS(filepath.Dir(path)), filepath.Base(path))
}

// LoadFileFS reads SQL queries from a single file in the given fs.FS.
func (r *Registry) LoadFileFS(fsys fs.FS, path string) error {
	f, err := fsys.Open(path)
	if err != nil {
		return fmt.Errorf("glimt: open %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	p := newParser()
	if err := p.parse(f); err != nil {
		return fmt.Errorf("glimt: parse %s: %w", path, err)
	}

	return r.merge(path, p.queries)
}

// Load reads all .sql files in the given directory recursively.
func (r *Registry) Load(dir string) error {
	return r.LoadFS(os.DirFS(dir), ".")
}

// LoadFS reads all .sql files recursively from the given fs.FS starting at dir.
func (r *Registry) LoadFS(fsys fs.FS, dir string) error {
	return fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("glimt: read dir %s: %w", path, err)
		}

		if d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".sql") {
			return nil
		}

		return r.LoadFileFS(fsys, path)
	})
}

// merge adds queries to the registry, checking for duplicates.
// It returns an error if a duplicate is found.
func (r *Registry) merge(path string, queries map[string]string) error {
	for name, sql := range queries {
		if _, exists := r.queries[name]; exists {
			return fmt.Errorf("glimt: duplicate query name %q in %s", name, path)
		}

		r.queries[name] = sql
	}

	return nil
}
