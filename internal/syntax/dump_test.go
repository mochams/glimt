package syntax

import "testing"

func TestDump(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "keeps source spacing", sql: "SELECT a.b,c ,  d FROM t", want: `
query
  select
    columns: a.b,c , d
    from: t
`},
		{name: "marks expanded params", sql: "SELECT :a FROM t WHERE b IN (:bs) AND c = :a", want: `
query
  select
    columns: :a
    from: t
    where: b IN (:bs) AND c = :a
params: a bs* a
`},
		{name: "values subqueries follow their row", sql: "VALUES ((SELECT 1), 2), (3, 4)", want: `
query
  values
    row: [(SELECT 1)] [2]
      subquery
        query
          select
            columns: 1
    row: [3] [4]
`},
		{name: "raw", sql: "TRUNCATE orders", want: `
raw: TRUNCATE orders
`},
	})
}
