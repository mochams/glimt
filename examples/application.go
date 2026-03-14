package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
	gl "github.com/mochams/glimt"
)

// Application holds the application dependencies.
type Application struct {
	db       *sql.DB
	registry *gl.Registry
	mux      *http.ServeMux
}

// NewApplication creates and wires up the application.
func NewApplication(dsn, queriesFile string) (*Application, error) {
	db, err := openDatabase(dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	registry := gl.NewRegistry(gl.DialectSQLite)
	if err := registry.Load(queriesFile); err != nil {
		db.Close()
		return nil, fmt.Errorf("load queries: %w", err)
	}

	app := &Application{
		db:       db,
		registry: registry,
		mux:      http.NewServeMux(),
	}

	app.setup()

	return app, nil
}

// setup initializes the database schema and registers routes.
func (a *Application) setup() {
	a.migrate()
	a.routes()
}

// migrate creates the database schema.
func (a *Application) migrate() {
	query, _ := a.registry.MustGet("createUsersTable").Build()
	if _, err := a.db.Exec(query); err != nil {
		log.Fatalf("migrate: create users table: %v", err)
	}
}

// routes registers all HTTP routes.
func (a *Application) routes() {
	repo := NewUserRepository(a.db, a.registry)
	handler := NewUserHandler(repo)

	a.mux.HandleFunc("GET /users", handler.List)
	a.mux.HandleFunc("GET /users/{id}", handler.Get)
	a.mux.HandleFunc("POST /users", handler.Create)
	a.mux.HandleFunc("DELETE /users/{id}", handler.Delete)
}

// ServeHTTP implements the http.Handler interface.
func (a *Application) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

// Close releases application resources.
func (a *Application) Close() error {
	return a.db.Close()
}

// Database

func openDatabase(dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return db, nil
}
