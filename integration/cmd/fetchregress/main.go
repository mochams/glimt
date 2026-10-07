// Command fetchregress downloads the SQL files of Postgres's regression
// suite at one release into a directory, for the oracle corpus, along with
// the parser's keyword list, kwlist.h.
//
// Usage, from the integration directory:
//
//	go run ./cmd/fetchregress REL_17_7 testdata/regress
package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: fetchregress <tag> <dir>")
		os.Exit(2)
	}

	n, err := fetch(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("fetched %d files into %s\n", n, os.Args[2])
}

// fetch downloads Postgres at tag and writes its src/test/regress/sql files
// and src/include/parser/kwlist.h to dir.
func fetch(tag, dir string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	url := "https://codeload.github.com/postgres/postgres/tar.gz/refs/tags/" + tag

	// The host is fixed; only the tag comes from the command line.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // fixed host
	if err != nil {
		return 0, err
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // fixed host
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // the caller's directory
		return 0, err
	}

	return extract(resp.Body, dir)
}

// extract writes the regression SQL files and kwlist.h from a gzipped
// tarball to dir.
func extract(r io.Reader, dir string) (int, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return 0, err
	}

	tr := tar.NewReader(gz)
	n := 0

	for {
		h, err := tr.Next()
		if err == io.EOF {
			return n, nil
		}

		if err != nil {
			return n, err
		}

		if !wanted(h.Name) {
			continue
		}

		if err := writeFile(filepath.Join(dir, path.Base(h.Name)), tr); err != nil {
			return n, err
		}

		n++
	}
}

// wanted reports whether the file at name in the tarball is one to keep.
func wanted(name string) bool {
	return strings.Contains(name, "/src/test/regress/sql/") && path.Ext(name) == ".sql" ||
		strings.HasSuffix(name, "/src/include/parser/kwlist.h")
}

// writeFile copies r to a new file at name.
func writeFile(name string, r io.Reader) error {
	f, err := os.Create(name) //nolint:gosec // a path under the target directory
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, r); err != nil { //nolint:gosec // trusted archive
		_ = f.Close()

		return err
	}

	return f.Close()
}
