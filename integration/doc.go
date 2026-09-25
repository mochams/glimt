// Package integration runs glimt against real databases.
//
// TEST_DIALECT selects the database (postgres, mysql or sqlite) and
// TEST_DATABASE_URL its DSN. SQLite runs in-process and needs no DSN:
//
//	TEST_DIALECT=sqlite make integration
//	TEST_DATABASE_URL=postgres://... make integration
//	TEST_DIALECT=mysql TEST_DATABASE_URL='user:pass@tcp(127.0.0.1:3306)/glimt_test?parseTime=true' make integration
//
// queries/common.sql is shared by every dialect; each dialect directory holds
// its own schema and inserts.
package integration
