package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":8080", "HTTP server address")
	dsn := flag.String("dsn", "glimt.db", "SQLite database file")
	queries := flag.String("queries", "queries.sql", "SQL queries file or directory")
	flag.Parse()

	app, err := NewApplication(*dsn, *queries)
	if err != nil {
		log.Fatalf("failed to start application: %v", err)
	}
	defer app.Close()

	log.Printf("starting server on %s", *addr)

	if err := http.ListenAndServe(*addr, app); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
