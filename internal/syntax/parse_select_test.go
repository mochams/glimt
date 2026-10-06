package syntax

import "testing"

func TestParseSelect(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "all clauses", sql: "SELECT a, b FROM t WHERE x = :x GROUP BY a HAVING count(*) > 1 " +
			"ORDER BY a DESC LIMIT :n OFFSET 5", want: `
query
  select
    columns: a, b
    from: t
    where: x = :x
    group by: a
    having: count(*) > 1
  order by: a DESC
  limit: :n
  offset: 5
params: x n
`},
		{name: "offset before limit", sql: "SELECT a FROM t OFFSET 10 LIMIT 5", want: `
query
  select
    columns: a
    from: t
  limit: 5
  offset: 10
`},
		{name: "distinct", sql: "SELECT DISTINCT a FROM t", want: `
query
  select distinct
    columns: a
    from: t
`},
		{name: "distinct on", sql: "SELECT DISTINCT ON (a, b) a, c FROM t ORDER BY a", want: `
query
  select distinct on
    on: a, b
    columns: a, c
    from: t
  order by: a
`},
		{name: "all", sql: "SELECT ALL a FROM t", want: `
query
  select all
    columns: a
    from: t
`},
		{name: "empty select list", sql: "SELECT FROM t", want: `
query
  select
    columns:
    from: t
`},
		{name: "no from", sql: "SELECT 1 WHERE true", want: `
query
  select
    columns: 1
    where: true
`},
		{name: "joins are opaque", sql: "SELECT * FROM a JOIN b ON a.id = b.id LEFT JOIN c USING (id) WHERE a.x = :x",
			want: `
query
  select
    columns: *
    from: a JOIN b ON a.id = b.id LEFT JOIN c USING (id)
    where: a.x = :x
params: x
`},
		{name: "is distinct from", sql: "SELECT a IS DISTINCT FROM b, c IS NOT DISTINCT FROM d FROM t", want: `
query
  select
    columns: a IS DISTINCT FROM b, c IS NOT DISTINCT FROM d
    from: t
`},
		{name: "keywords inside parens and case", sql: "SELECT row_number() OVER (PARTITION BY a ORDER BY b), " +
			"count(*) FILTER (WHERE c), EXTRACT(YEAR FROM d), " +
			"percentile_cont(0.5) WITHIN GROUP (ORDER BY e), " +
			"CASE WHEN f THEN 'x' ELSE CASE g WHEN 1 THEN 'y' END END FROM t", want: `
query
  select
    columns: row_number() OVER (PARTITION BY a ORDER BY b), count(*) FILTER (WHERE c), EXTRACT(YEAR FROM d), percentile_cont(0.5) WITHIN GROUP (ORDER BY e), CASE WHEN f THEN 'x' ELSE CASE g WHEN 1 THEN 'y' END END
    from: t
`},
		{name: "array brackets", sql: "SELECT a[1], ARRAY[b, c] FROM t WHERE d = ANY(:ds)", want: `
query
  select
    columns: a[1], ARRAY[b, c]
    from: t
    where: d = ANY(:ds)
params: ds
`},
		{name: "subqueries", sql: "SELECT a, (SELECT max(b) FROM u WHERE u.a = t.a) AS m FROM t " +
			"WHERE EXISTS (SELECT 1 FROM v WHERE v.id = t.id AND v.k = :k)", want: `
query
  select
    columns: a, (SELECT max(b) FROM u WHERE u.a = t.a) AS m
      subquery
        query
          select
            columns: max(b)
            from: u
            where: u.a = t.a
    from: t
    where: EXISTS (SELECT 1 FROM v WHERE v.id = t.id AND v.k = :k)
      subquery
        query
          select
            columns: 1
            from: v
            where: v.id = t.id AND v.k = :k
params: k
`},
		{name: "nested parens around subquery", sql: "SELECT ((SELECT 1))", want: `
query
  select
    columns: ((SELECT 1))
      subquery
        query
          select
            columns: 1
`},
		{name: "job queue lock", sql: "SELECT id FROM jobs WHERE state = :s ORDER BY id LIMIT 10 FOR UPDATE SKIP LOCKED", want: `
query
  select
    columns: id
    from: jobs
    where: state = :s
  order by: id
  limit: 10
  for: UPDATE SKIP LOCKED
params: s
`},
		{name: "locking clauses then limit", sql: "SELECT * FROM a, b FOR UPDATE OF a NOWAIT FOR SHARE OF b LIMIT 1", want: `
query
  select
    columns: *
    from: a, b
  limit: 1
  for: UPDATE OF a NOWAIT FOR SHARE OF b
`},
		{name: "offset and fetch", sql: "SELECT a FROM t ORDER BY a OFFSET 5 ROWS FETCH NEXT :n ROWS WITH TIES", want: `
query
  select
    columns: a
    from: t
  order by: a
  offset: 5 ROWS
  fetch: NEXT :n ROWS WITH TIES
params: n
`},
		{name: "window", sql: "SELECT sum(a) OVER w FROM t WINDOW w AS (PARTITION BY b ORDER BY c) ORDER BY 1", want: `
query
  select
    columns: sum(a) OVER w
    from: t
    window: w AS (PARTITION BY b ORDER BY c)
  order by: 1
`},
		{name: "table", sql: "TABLE app.orders ORDER BY id LIMIT :n", want: `
query
  table app.orders
  order by: id
  limit: :n
params: n
`},
		{name: "table only and star", sql: "TABLE ONLY a UNION TABLE b *", want: `
query
  union
    table only a
    table b
`},
		{name: "table in set op and subquery", sql: "TABLE a UNION ALL SELECT * FROM b WHERE id IN (TABLE c)", want: `
query
  union all
    table a
    select
      columns: *
      from: b
      where: id IN (TABLE c)
        subquery
          query
            table c
`},
		{name: "for inside an expression", sql: "SELECT collation for ('foo') FROM t FOR NO KEY UPDATE", want: `
query
  select
    columns: collation for ('foo')
    from: t
  for: NO KEY UPDATE
`},
		{name: "locking forms", sql: "SELECT a FROM t FOR KEY SHARE OF t FOR SHARE SKIP LOCKED", want: `
query
  select
    columns: a
    from: t
  for: KEY SHARE OF t FOR SHARE SKIP LOCKED
`},
		{name: "rows from", sql: "SELECT * FROM ROWS FROM (f(1), g(2)) WITH ORDINALITY AS z(a, b, n) WHERE a > :a", want: `
query
  select
    columns: *
    from: ROWS FROM (f(1), g(2)) WITH ORDINALITY AS z(a, b, n)
    where: a > :a
params: a
`},
		{name: "rows from after a comma, join and lateral",
			sql: "SELECT * FROM t, ROWS FROM (f(1)) a JOIN ROWS FROM (g(2)) b ON true, LATERAL ROWS FROM (h(t.x)) c WHERE p",
			want: `
query
  select
    columns: *
    from: t, ROWS FROM (f(1)) a JOIN ROWS FROM (g(2)) b ON true, LATERAL ROWS FROM (h(t.x)) c
    where: p
`},
		{name: "join on a column named conflict", sql: "SELECT a FROM t JOIN u ON conflict = u.id WHERE p", want: `
query
  select
    columns: a
    from: t JOIN u ON conflict = u.id
    where: p
`},
		{name: "bare label select before a from", sql: "SELECT 1 SELECT FROM t", want: `
query
  select
    columns: 1 SELECT
    from: t
`},
		{name: "bare label select before from", sql: "SELECT 0 select, 1 table FROM t", want: `
query
  select
    columns: 0 select, 1 table
    from: t
`},
		{name: "bare label select at the end", sql: "SELECT count(*) select;", want: `
query
  select
    columns: count(*) select
`},
		{name: "column labelled distinct", sql: "SELECT a distinct FROM t WHERE b IS NOT DISTINCT FROM c", want: `
query
  select
    columns: a distinct
    from: t
    where: b IS NOT DISTINCT FROM c
`},
		{name: "column labelled rows", sql: "SELECT count(*) rows FROM t WHERE a = :a", want: `
query
  select
    columns: count(*) rows
    from: t
    where: a = :a
params: a
`},
		{name: "column named rows before from", sql: "SELECT 0, rows FROM a, ROWS FROM (f(1)) r", want: `
query
  select
    columns: 0, rows
    from: a, ROWS FROM (f(1)) r
`},
		{name: "column named rows before a subquery", sql: "SELECT a, rows FROM (SELECT 1) s", want: `
query
  select
    columns: a, rows
    from: (SELECT 1) s
      subquery
        query
          select
            columns: 1
`},
		{name: "column labelled rows in a group", sql: "SELECT s, count(*) rows FROM t GROUP BY s", want: `
query
  select
    columns: s, count(*) rows
    from: t
    group by: s
`},
		{name: "keyword column labels", sql: "SELECT count(*) AS order, a AS case FROM t", want: `
query
  select
    columns: count(*) AS order, a AS case
    from: t
`},
		{name: "keywords as field names", sql: "SELECT t.case, t.end FROM t WHERE t.offset > 5 AND x.in (:ids)", want: `
query
  select
    columns: t.case, t.end
    from: t
    where: t.offset > 5 AND x.in (:ids)
params: ids
`},
		{name: "slice bounds", sql: "SELECT a[lo:hi], a[:lo:hi] FROM t", want: `
query
  select
    columns: a[lo:hi], a[:lo:hi]
    from: t
params: lo
`},
		{name: "subquery in from", sql: "SELECT * FROM (SELECT a FROM t LIMIT :n) s, LATERAL (VALUES (s.a)) v(x)",
			want: `
query
  select
    columns: *
    from: (SELECT a FROM t LIMIT :n) s, LATERAL (VALUES (s.a)) v(x)
      subquery
        query
          select
            columns: a
            from: t
          limit: :n
      subquery
        query
          values
            row: [s.a]
params: n
`},
	})
}

func TestParseSetOps(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "union", sql: "SELECT 1 UNION SELECT 2", want: `
query
  union
    select
      columns: 1
    select
      columns: 2
`},
		{name: "union distinct", sql: "SELECT 1 UNION DISTINCT SELECT 2", want: `
query
  union
    select
      columns: 1
    select
      columns: 2
`},
		{name: "intersect binds tighter on the right", sql: "SELECT 1 UNION ALL SELECT 2 INTERSECT SELECT 3", want: `
query
  union all
    select
      columns: 1
    intersect
      select
        columns: 2
      select
        columns: 3
`},
		{name: "intersect binds tighter on the left", sql: "SELECT 1 INTERSECT SELECT 2 UNION SELECT 3", want: `
query
  union
    intersect
      select
        columns: 1
      select
        columns: 2
    select
      columns: 3
`},
		{name: "except then union is left-associative", sql: "SELECT 1 EXCEPT SELECT 2 UNION SELECT 3", want: `
query
  union
    except
      select
        columns: 1
      select
        columns: 2
    select
      columns: 3
`},
		{name: "union then except is left-associative", sql: "SELECT 1 UNION SELECT 2 EXCEPT ALL SELECT 3", want: `
query
  except all
    union
      select
        columns: 1
      select
        columns: 2
    select
      columns: 3
`},
		{name: "parenthesized operands and tail", sql: "(SELECT a FROM t ORDER BY a LIMIT 1) UNION " +
			"(SELECT b FROM u) ORDER BY 1 LIMIT 10", want: `
query
  union
    paren
      query
        select
          columns: a
          from: t
        order by: a
        limit: 1
    paren
      query
        select
          columns: b
          from: u
  order by: 1
  limit: 10
`},
		{name: "parens override precedence", sql: "SELECT 1 INTERSECT (SELECT 2 UNION SELECT 3)", want: `
query
  intersect
    select
      columns: 1
    paren
      query
        union
          select
            columns: 2
          select
            columns: 3
`},
		{name: "values operand", sql: "VALUES (1, :a), (2, :b) UNION SELECT 3, 4 ORDER BY 1", want: `
query
  union
    values
      row: [1] [:a]
      row: [2] [:b]
    select
      columns: 3, 4
  order by: 1
params: a b
`},
		{name: "compound in subquery", sql: "SELECT * FROM t WHERE id IN (SELECT a FROM x UNION SELECT b FROM y)",
			want: `
query
  select
    columns: *
    from: t
    where: id IN (SELECT a FROM x UNION SELECT b FROM y)
      subquery
        query
          union
            select
              columns: a
              from: x
            select
              columns: b
              from: y
`},
		{name: "compound in cte", sql: "WITH c AS (SELECT 1 UNION ALL SELECT 2) SELECT * FROM c", want: `
query
  with
    cte c
      query
        union all
          select
            columns: 1
          select
            columns: 2
  select
    columns: *
    from: c
`},
	})
}

func TestParseSelectErrors(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "empty from", sql: "SELECT a FROM WHERE x", err: true,
			want: `expected expression after FROM, found "WHERE"`},
		{name: "group without by", sql: "SELECT a FROM t GROUP a", err: true,
			want: `expected BY, found "a"`},
		{name: "empty order by", sql: "SELECT a FROM t ORDER BY", err: true,
			want: "expected expression after ORDER BY, found end of input"},
		{name: "clause out of order", sql: "SELECT a FROM t ORDER BY a WHERE b", err: true,
			want: `expected end of statement, found "WHERE"`},
		{name: "order by inside operand", sql: "SELECT 1 ORDER BY 1 UNION SELECT 2", err: true,
			want: `expected end of statement, found "UNION"`},
		{name: "repeated limit", sql: "SELECT 1 LIMIT 1 LIMIT 2", err: true,
			want: `expected end of statement, found "LIMIT"`},
		{name: "union without operand", sql: "SELECT 1 UNION", err: true,
			want: "expected SELECT, VALUES, TABLE or a parenthesized query, found end of input"},
		{name: "unclosed subquery", sql: "SELECT * FROM t WHERE a IN (SELECT b FROM u", err: true,
			want: `expected ")" after subquery, found end of input`},
		{name: "unclosed paren query", sql: "(SELECT 1", err: true,
			want: `expected ")", found end of input`},
		{name: "empty values row", sql: "VALUES ()", err: true,
			want: `expected value, found ")"`},
		{name: "empty distinct on", sql: "SELECT DISTINCT ON () a FROM t", err: true,
			want: `expected expression after DISTINCT ON, found ")"`},
		{name: "select into", sql: "SELECT a, b INTO TEMP x FROM t", err: true,
			want: "SELECT INTO is not supported"},
		{name: "into after from", sql: "SELECT a FROM t INTO x", err: true,
			want: `expected end of statement, found "INTO"`},
		{name: "interleaved parens and case", sql: "SELECT (CASE WHEN x) FROM t END", err: true,
			want: `expected END, found ")"`},
		{name: "limit and fetch", sql: "SELECT a FROM t LIMIT 1 FETCH FIRST 2 ROWS ONLY", err: true,
			want: "LIMIT and FETCH can't be used together"},
		{name: "two locking spans", sql: "SELECT a FROM t FOR UPDATE LIMIT 1 FOR SHARE", err: true,
			want: `expected end of statement, found "FOR"`},
		{name: "query with returning in parentheses", sql: "SELECT JSON_ARRAY(SELECT i FROM t RETURNING jsonb)", err: true,
			want: "RETURNING after a query in parentheses, as in JSON_ARRAY(SELECT … RETURNING …), is not supported"},
		{name: "returning on select", sql: "SELECT a FROM t RETURNING a", err: true,
			want: `expected end of statement, found "RETURNING"`},
	})
}
