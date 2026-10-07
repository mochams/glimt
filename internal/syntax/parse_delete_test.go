package syntax

import "testing"

func TestParseDelete(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "where", sql: "DELETE FROM orders WHERE id = :id", want: `
delete
  table: orders
  where: id = :id
params: id
`},
		{name: "only", sql: "DELETE FROM ONLY t WHERE a", want: `
delete
  table: only t
  where: a
`},
		{name: "inheritance star", sql: "DELETE FROM a* x WHERE b", want: `
delete
  table: a alias: x
  where: b
`},
		{name: "using a join on a column named conflict", sql: "DELETE FROM t USING u JOIN v ON conflict = v.id WHERE p", want: `
delete
  table: t
  using: u JOIN v ON conflict = v.id
  where: p
`},
		{name: "no where", sql: "DELETE FROM orders", want: `
delete
  table: orders
`},
		{name: "alias using where returning", sql: "DELETE FROM orders o USING users u " +
			"WHERE u.id = o.user_id AND u.banned AND o.id NOT IN (:keep) RETURNING o.id", want: `
delete
  table: orders alias: o
  using: users u
  where: u.id = o.user_id AND u.banned AND o.id NOT IN (:keep)
  returning: o.id
params: keep*
`},
		{name: "with cte", sql: "WITH old AS (SELECT id FROM t WHERE created < :cutoff) " +
			"DELETE FROM t WHERE id IN (SELECT id FROM old)", want: `
delete
  with
    cte old
      query
        select
          columns: id
          from: t
          where: created < :cutoff
  table: t
  where: id IN (SELECT id FROM old)
    subquery
      query
        select
          columns: id
          from: old
params: cutoff
`},
	})
}

func TestParseDeleteErrors(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "missing from", sql: "DELETE orders WHERE a", err: true, want: `expected FROM, found "orders"`},
		{name: "empty using", sql: "DELETE FROM t USING WHERE a", err: true,
			want: `expected expression after USING, found "WHERE"`},
	})
}
