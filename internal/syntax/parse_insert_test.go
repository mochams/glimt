package syntax

import "testing"

func TestParseInsert(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "values", sql: "INSERT INTO orders (user_id, total) VALUES (:user, :total), (:user, 0) RETURNING id",
			want: `
insert
  table: orders
  columns: (user_id, total)
  query
    values
      row: [:user] [:total]
      row: [:user] [0]
  returning: id
params: user total user
`},
		{name: "qualified table and alias", sql: `INSERT INTO app."Orders" AS o VALUES (1)`, want: `
insert
  table: app."Orders" alias: o
  query
    values
      row: [1]
`},
		{name: "overriding", sql: "INSERT INTO t (id, a) OVERRIDING SYSTEM VALUE VALUES (:id, :a)", want: `
insert
  table: t
  columns: (id, a)
  overriding system value
  query
    values
      row: [:id] [:a]
params: id a
`},
		{name: "overriding user without columns", sql: "INSERT INTO t OVERRIDING USER VALUE SELECT * FROM s", want: `
insert
  table: t
  overriding user value
  query
    select
      columns: *
      from: s
`},
		{name: "column paths", sql: "INSERT INTO t (addr.city, tags[:i]) VALUES (:city, :tag)", want: `
insert
  table: t
  columns: (addr.city, tags[:i])
  query
    values
      row: [:city] [:tag]
params: i city tag
`},
		{name: "keyword label in returning", sql: "INSERT INTO t VALUES (1) RETURNING id AS limit", want: `
insert
  table: t
  query
    values
      row: [1]
  returning: id AS limit
`},
		{name: "default values", sql: "INSERT INTO t DEFAULT VALUES RETURNING *", want: `
insert
  table: t
  default values
  returning: *
`},
		{name: "select source", sql: "INSERT INTO t (a) SELECT a FROM s WHERE b IN (:bs)", want: `
insert
  table: t
  columns: (a)
  query
    select
      columns: a
      from: s
      where: b IN (:bs)
params: bs*
`},
		{name: "parenthesized source is not a column list", sql: "INSERT INTO t (SELECT a FROM s)", want: `
insert
  table: t
  query
    paren
      query
        select
          columns: a
          from: s
`},
		{name: "with source", sql: "INSERT INTO t WITH x AS (SELECT 1) SELECT * FROM x", want: `
insert
  table: t
  query
    with
      cte x
        query
          select
            columns: 1
    select
      columns: *
      from: x
`},
		{name: "select join where on conflict update", sql: "INSERT INTO users (id, name) " +
			"SELECT s.id, s.name FROM staging s JOIN batches b ON b.id = s.batch_id WHERE s.active = true " +
			"ON CONFLICT (id) DO UPDATE SET name = excluded.name RETURNING id", want: `
insert
  table: users
  columns: (id, name)
  query
    select
      columns: s.id, s.name
      from: staging s JOIN batches b ON b.id = s.batch_id
      where: s.active = true
  on conflict
    target: (id)
    do update
      set: name = excluded.name
  returning: id
`},
		{name: "union source then on conflict", sql: "INSERT INTO t SELECT 1 UNION SELECT 2 ON CONFLICT DO NOTHING",
			want: `
insert
  table: t
  query
    union
      select
        columns: 1
      select
        columns: 2
  on conflict
    do nothing
`},
		{name: "on constraint", sql: "INSERT INTO t VALUES (:a) ON CONFLICT ON CONSTRAINT t_pkey DO NOTHING", want: `
insert
  table: t
  query
    values
      row: [:a]
  on conflict
    target: ON CONSTRAINT t_pkey
    do nothing
params: a
`},
		{name: "conflict target where and update where", sql: "INSERT INTO t (a, b) VALUES (:a, :b) " +
			"ON CONFLICT (a) WHERE b IS NOT NULL DO UPDATE SET b = excluded.b, (c, d) = (SELECT 1, 2) " +
			"WHERE t.b < excluded.b RETURNING a, b", want: `
insert
  table: t
  columns: (a, b)
  query
    values
      row: [:a] [:b]
  on conflict
    target: (a) WHERE b IS NOT NULL
    do update
      set: b = excluded.b
      set: (c, d) = (SELECT 1, 2)
        subquery
          query
            select
              columns: 1, 2
      where: t.b < excluded.b
  returning: a, b
params: a b
`},
	})
}

func TestParseInsertErrors(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "missing into", sql: "INSERT t VALUES (1)", err: true, want: `expected INTO, found "t"`},
		{name: "missing source", sql: "INSERT INTO t (a)", err: true,
			want: "expected VALUES, a query or DEFAULT VALUES, found end of input"},
		{name: "alias needs as", sql: "INSERT INTO t x VALUES (1)", err: true,
			want: `expected VALUES, a query or DEFAULT VALUES, found "x"`},
		{name: "param as table", sql: "INSERT INTO :t VALUES (1)", err: true, want: `expected table name, found ":t"`},
		{name: "default without values", sql: "INSERT INTO t DEFAULT", err: true,
			want: "expected VALUES, found end of input"},
		{name: "on without conflict", sql: "INSERT INTO t VALUES (1) ON x", err: true,
			want: `expected CONFLICT, found "x"`},
		{name: "conflict without action", sql: "INSERT INTO t VALUES (1) ON CONFLICT (a)", err: true,
			want: "expected DO, found end of input"},
		{name: "do something else", sql: "INSERT INTO t VALUES (1) ON CONFLICT DO DELETE", err: true,
			want: `expected UPDATE, found "DELETE"`},
		{name: "empty returning", sql: "INSERT INTO t VALUES (1) RETURNING", err: true,
			want: "expected expression after RETURNING, found end of input"},
		{name: "overriding without kind", sql: "INSERT INTO t OVERRIDING VALUE VALUES (1)", err: true,
			want: `expected SYSTEM or USER, found "VALUE"`},
		{name: "overriding without value", sql: "INSERT INTO t OVERRIDING SYSTEM VALUES (1)", err: true,
			want: `expected VALUE, found "VALUES"`},
		{name: "column list with expression", sql: "INSERT INTO t (a + 1) VALUES (1)", err: true,
			want: `expected ")", found "+"`},
	})
}
