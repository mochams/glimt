package integration

import (
	"database/sql"
	"flag"
	"log"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	gl "github.com/mochams/glimt"
)

// testState provides test state that can be shared across test functions.
// It is initialized in TestMain.
var testState struct {
	registry *gl.Registry
	db       *sql.DB
	dsn      string
}

// TestMain is the entry point for testing. It initializes the test environment and runs the tests.
func TestMain(m *testing.M) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	flag.StringVar(&testState.dsn, "dsn", dsn, "integration database DSN")
	flag.Parse()

	if testState.dsn == "" {
		log.Fatal("dsn is required — set -dsn flag or TEST_DATABASE_URL env var")
	}

	testState.registry = gl.NewRegistry(gl.DialectPostgres)

	if err := testState.registry.LoadDir("../testdata/queries"); err != nil {
		log.Fatalf("failed to load queries: %v", err)
	}

	db, err := openDatabase(testState.dsn)
	if err != nil {
		log.Fatal(err.Error())
	}
	testState.db = db

	setup()
	code := m.Run()
	teardown()
	db.Close()

	os.Exit(code)
}

// Setup

func setup() {
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

func openDatabase(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
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
