module github.com/mochams/glimt/integration

go 1.25.0

require (
	github.com/jackc/pgx/v5 v5.11.0
	github.com/mochams/glimt v0.0.0
	github.com/pganalyze/pg_query_go/v6 v6.2.5
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
)

replace github.com/mochams/glimt => ../
