package glimt

import (
	"fmt"
	"io/fs"
	"os"
	"path"
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
// It panics if the query is not found.
func (r *Registry) MustGet(name string) *Query {
	q, err := r.Get(name)
	if err != nil {
		panic(err)
	}

	return q
}

// Query creates a new Query from the given SQL string.
// This method can be used to create ad-hoc queries that are not stored in the registry.
func (r *Registry) Query(sql string) *Query {
	return NewQuery(sql, r.dialect)
}

// Load reads SQL queries from a file at the given path and adds them to the registry.
// It returns an error if the file cannot be read or if there are duplicate query names.
func (r *Registry) Load(path string) error {
	// #nosec G304 -- path is provided by the developer at startup, not from user input
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("glimt: open %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	queries, err := parse(f)
	if err != nil {
		return fmt.Errorf("glimt: parse %s: %w", path, err)
	}

	return r.merge(path, queries)
}

// LoadFS reads SQL queries from a file in the given fs.FS at the specified path and adds them to the registry.
// It returns an error if the file cannot be read or if there are duplicate query names.
func (r *Registry) LoadFS(fsys fs.FS, path string) error {
	f, err := fsys.Open(path)
	if err != nil {
		return fmt.Errorf("glimt: open %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	queries, err := parse(f)
	if err != nil {
		return fmt.Errorf("glimt: parse %s: %w", path, err)
	}

	return r.merge(path, queries)
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

// LoadDir reads all .sql files in the specified directory and adds their queries to the registry.
// It returns an error if the directory cannot be read or if there are duplicate query names.
func (r *Registry) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("glimt: read dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			continue
		}

		err := r.Load(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
	}

	return nil
}

// WalkFS reads all .sql files in the specified directory of the given fs.FS and adds their queries to the registry.
// It returns an error if the directory cannot be read or if there are duplicate query names.
func (r *Registry) WalkFS(fsys fs.FS, dir string) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("glimt: read dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sql") {
			continue
		}

		err := r.LoadFS(fsys, path.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
	}

	return nil
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
