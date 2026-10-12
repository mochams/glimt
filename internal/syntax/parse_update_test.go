package syntax

import "testing"

func TestParseUpdate(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "where in", sql: "UPDATE orders SET status = :status WHERE id IN (:ids)", want: `
update
  table: orders
  set: status = :status
  where: id IN (:ids)
params: status ids*
`},
		{name: "returning with a bare label", sql: "UPDATE t SET a = 1 RETURNING a select, b table", want: `
update
  table: t
  set: a = 1
  returning: a select, b table
`},
		{name: "from a join on a column named conflict", sql: "UPDATE t SET a = 1 FROM u JOIN v ON conflict = v.id WHERE p", want: `
update
  table: t
  set: a = 1
  from: u JOIN v ON conflict = v.id
  where: p
`},
		{name: "alias without as", sql: "UPDATE orders o SET total = o.total * 2", want: `
update
  table: orders alias: o
  set: total = o.total * 2
`},
		{name: "alias with as", sql: `UPDATE public.orders AS "O" SET a = 1`, want: `
update
  table: public.orders alias: "O"
  set: a = 1
`},
		{name: "several assignments and functions", sql: "UPDATE t SET a = coalesce(:a, a), b = DEFAULT, " +
			"c = CASE WHEN d THEN 1 ELSE 2 END WHERE e", want: `
update
  table: t
  set: a = coalesce(:a, a)
  set: b = DEFAULT
  set: c = CASE WHEN d THEN 1 ELSE 2 END
  where: e
params: a
`},
		{name: "tuple assignment from subquery", sql: "UPDATE t SET (a, b) = (SELECT x, y FROM s WHERE s.id = t.id)",
			want: `
update
  table: t
  set: (a, b) = (SELECT x, y FROM s WHERE s.id = t.id)
    subquery
      query
        select
          columns: x, y
          from: s
          where: s.id = t.id
`},
		{name: "target paths", sql: "UPDATE t SET addr.city = :city, tags[1] = 'a', data['k'][:i] = :v, " +
			"grid[1:2] = :g, t.x = 0", want: `
update
  table: t
  set: addr.city = :city
  set: tags[1] = 'a'
  set: data['k'][:i] = :v
  set: grid[1:2] = :g
  set: t.x = 0
params: city i v g
`},
		{name: "tuple of paths", sql: "UPDATE t SET (a.b, c[(SELECT 1)]) = (:x, :y)", want: `
update
  table: t
  set: (a.b, c[(SELECT 1)]) = (:x, :y)
params: x y
`},
		{name: "keyword in a qualified table", sql: "UPDATE public.order SET a = 1", want: `
update
  table: public.order
  set: a = 1
`},
		{name: "only", sql: "UPDATE ONLY t x SET a = 1", want: `
update
  table: only t alias: x
  set: a = 1
`},
		{name: "from where returning", sql: "UPDATE t SET a = s.a FROM s JOIN u ON u.id = s.uid " +
			"WHERE s.id = t.id AND u.org = :org RETURNING t.id, t.a", want: `
update
  table: t
  set: a = s.a
  from: s JOIN u ON u.id = s.uid
  where: s.id = t.id AND u.org = :org
  returning: t.id, t.a
params: org
`},
		{name: "keyword-like column names", sql: "UPDATE t SET comment = :c, values = 1 WHERE conflict IS NOT DISTINCT FROM :x",
			want: `
update
  table: t
  set: comment = :c
  set: values = 1
  where: conflict IS NOT DISTINCT FROM :x
params: c x
`},
		{name: "subquery in where", sql: "UPDATE t SET a = 1 WHERE id = ANY (SELECT id FROM u WHERE b = :b)", want: `
update
  table: t
  set: a = 1
  where: id = ANY (SELECT id FROM u WHERE b = :b)
    subquery
      query
        select
          columns: id
          from: u
          where: b = :b
params: b
`},
	})
}

func TestParseUpdateErrors(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "missing set", sql: "UPDATE t WHERE a = 1", err: true, want: `expected SET, found "WHERE"`},
		{name: "missing equals", sql: "UPDATE t SET a 1", err: true, want: `expected "=", found "1"`},
		{name: "empty value", sql: "UPDATE t SET a = WHERE b", err: true,
			want: `expected value after =, found "WHERE"`},
		{name: "trailing comma", sql: "UPDATE t SET a = 1, WHERE b", err: true,
			want: `expected column name, found "WHERE"`},
		{name: "empty where", sql: "UPDATE t SET a = 1 WHERE", err: true,
			want: "expected expression after WHERE, found end of input"},
		{name: "where then from", sql: "UPDATE t SET a = 1 WHERE b FROM s", err: true,
			want: `expected end of statement, found "FROM"`},
		{name: "unclosed subscript", sql: "UPDATE t SET a[1 = 2", err: true,
			want: `expected "]", found end of input`},
		{name: "empty subscript", sql: "UPDATE t SET a[] = 2", err: true,
			want: `expected subscript, found "]"`},
		{name: "path without field", sql: "UPDATE t SET a. = 2", err: true,
			want: `expected field name, found "="`},
	})
}
