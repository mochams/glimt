package integration

import (
	"cmp"
	"database/sql"
	"flag"
	"log"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	gl "github.com/mochams/glimt"
	_ "modernc.org/sqlite"
)

// backends maps TEST_DIALECT values to a glimt dialect and a database/sql driver.
// Each backend's schema and inserts live in queries/<name>; queries/common.sql is shared.
var backends = map[string]struct {
	dialect gl.Dialect
	driver  string
}{
	"postgres": {gl.DialectPostgres, "pgx"},
	"mysql":    {gl.DialectMySQL, "mysql"},
	"sqlite":   {gl.DialectSQLite, "sqlite"},
}

// testState provides test state that can be shared across test functions.
// It is initialized in TestMain.
var testState struct {
	registry *gl.Registry
	db       *sql.DB
	dialect  gl.Dialect
}

// TestMain is the entry point for testing. It initializes the test environment and runs the tests.
//
// TEST_DIALECT selects the database: postgres (default), mysql or sqlite.
// TEST_DATABASE_URL is its DSN; sqlite uses a temporary file when it is unset.
// MySQL DSNs need parseTime=true.
func TestMain(m *testing.M) {
	name := cmp.Or(os.Getenv("TEST_DIALECT"), "postgres")
	dsn := os.Getenv("TEST_DATABASE_URL")
	flag.StringVar(&name, "dialect", name, "integration database: postgres, mysql or sqlite")
	flag.StringVar(&dsn, "dsn", dsn, "integration database DSN")
	flag.Parse()

	backend, ok := backends[name]
	if !ok {
		log.Fatalf("unknown dialect %q: use postgres, mysql or sqlite", name)
	}

	cleanupDSN := func() {}

	if dsn == "" {
		if name != "sqlite" {
			log.Fatal("dsn is required — set -dsn flag or TEST_DATABASE_URL env var")
		}

		dir, err := os.MkdirTemp("", "glimt-integration")
		if err != nil {
			log.Fatal(err)
		}

		dsn = filepath.Join(dir, "test.db")
		cleanupDSN = func() { os.RemoveAll(dir) }
	}

	testState.dialect = backend.dialect
	testState.registry = gl.NewRegistry(backend.dialect)

	if err := testState.registry.LoadFile("queries/common.sql"); err != nil {
		log.Fatalf("failed to load queries: %v", err)
	}

	if err := testState.registry.Load(filepath.Join("queries", name)); err != nil {
		log.Fatalf("failed to load queries: %v", err)
	}

	db, err := openDatabase(backend.driver, dsn)
	if err != nil {
		log.Fatal(err.Error())
	}
	testState.db = db

	setup()
	code := m.Run()
	teardown()
	db.Close()
	cleanupDSN()

	os.Exit(code)
}

// Setup

func setup() {
	// Drop first so a previous interrupted run does not leave stale tables.
	teardown()

	for _, name := range []string{
		"createUsersTable",
		"createProductsTable",
		"createOrdersTable",
	} {
		sql, _ := testState.registry.MustGet(name).Build()
		if _, err := testState.db.Exec(sql); err != nil {
			log.Fatalf("setup %q failed: %v", name, err)
		}
	}
}

// Teardown

func teardown() {
	for _, name := range []string{
		"dropOrdersTable",
		"dropProductsTable",
		"dropUsersTable",
	} {
		sql, _ := testState.registry.MustGet(name).Build()
		if _, err := testState.db.Exec(sql); err != nil {
			log.Fatalf("teardown %q failed: %v", name, err)
		}
	}
}

// Helper

func openDatabase(driver, dsn string) (*sql.DB, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// insertID runs the named insert with args and returns the new row's id.
// Postgres and SQLite inserts use RETURNING id; MySQL reports it through LastInsertId.
func insertID(t *testing.T, name string, args ...any) int {
	t.Helper()

	sql, args := testState.registry.MustGet(name).Args(args...).Build()

	if testState.dialect == gl.DialectMySQL {
		res, err := testState.db.Exec(sql, args...)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		id, err := res.LastInsertId()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		return int(id)
	}

	var id int
	if err := testState.db.QueryRow(sql, args...).Scan(&id); err != nil {
		t.Fatalf("%s: %v", name, err)
	}

	return id
}
