package syntax

import "testing"

func TestParseMerge(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "upsert", sql: "MERGE INTO stock s USING deliveries d ON s.item = d.item " +
			"WHEN MATCHED THEN UPDATE SET qty = s.qty + d.qty " +
			"WHEN NOT MATCHED THEN INSERT (item, qty) VALUES (d.item, d.qty)", want: `
merge
  table: stock alias: s
  using: deliveries d
  on: s.item = d.item
  when matched
    update
      set: qty = s.qty + d.qty
  when not matched
    insert
      columns: (item, qty)
      row: [d.item] [d.qty]
`},
		{name: "every form", sql: "WITH src AS (SELECT * FROM staging WHERE batch = :batch) " +
			"MERGE INTO app.accounts AS a USING (SELECT * FROM src WHERE ok) AS s ON a.id = s.id " +
			"WHEN MATCHED AND s.deleted THEN DELETE " +
			"WHEN MATCHED AND CASE WHEN s.v > a.v THEN true ELSE false END THEN UPDATE SET v = s.v, tags[1] = :tag " +
			"WHEN MATCHED THEN DO NOTHING " +
			"WHEN NOT MATCHED BY SOURCE AND a.active THEN UPDATE SET active = false " +
			"WHEN NOT MATCHED BY TARGET AND s.id IN (:ids) THEN INSERT (id, v) OVERRIDING SYSTEM VALUE VALUES (s.id, s.v) " +
			"WHEN NOT MATCHED THEN INSERT DEFAULT VALUES " +
			"RETURNING merge_action(), a.id", want: `
merge
  with
    cte src
      query
        select
          columns: *
          from: staging
          where: batch = :batch
  table: app.accounts alias: a
  using: (SELECT * FROM src WHERE ok) AS s
    subquery
      query
        select
          columns: *
          from: src
          where: ok
  on: a.id = s.id
  when matched
    and: s.deleted
    delete
  when matched
    and: CASE WHEN s.v > a.v THEN true ELSE false END
    update
      set: v = s.v
      set: tags[1] = :tag
  when matched
    do nothing
  when not matched by source
    and: a.active
    update
      set: active = false
  when not matched
    and: s.id IN (:ids)
    insert
      columns: (id, v)
      overriding system value
      row: [s.id] [s.v]
  when not matched
    insert
      default values
  returning: merge_action(), a.id
params: batch tag ids*
`},
		{name: "only", sql: "MERGE INTO ONLY t USING s ON a WHEN MATCHED THEN DELETE", want: `
merge
  table: only t
  using: s
  on: a
  when matched
    delete
`},
		{name: "source is a join", sql: "MERGE INTO t USING s1 JOIN s2 ON s1.id = s2.id LEFT JOIN s3 USING (k) " +
			"NATURAL JOIN s4 CROSS JOIN s5 ON t.id = s1.id WHEN MATCHED THEN DELETE", want: `
merge
  table: t
  using: s1 JOIN s2 ON s1.id = s2.id LEFT JOIN s3 USING (k) NATURAL JOIN s4 CROSS JOIN s5
  on: t.id = s1.id
  when matched
    delete
`},
		{name: "insert without columns", sql: "MERGE INTO t USING s ON t.id = s.id " +
			"WHEN NOT MATCHED THEN INSERT VALUES (s.id, :now)", want: `
merge
  table: t
  using: s
  on: t.id = s.id
  when not matched
    insert
      row: [s.id] [:now]
params: now
`},
	})
}

func TestParseMergeErrors(t *testing.T) {
	runParseCases(t, []parseCase{
		{name: "missing into", sql: "MERGE t USING s ON a WHEN MATCHED THEN DELETE", err: true,
			want: `expected INTO, found "t"`},
		{name: "missing using", sql: "MERGE INTO t ON a WHEN MATCHED THEN DELETE", err: true,
			want: `expected USING, found "ON"`},
		{name: "empty source", sql: "MERGE INTO t USING ON a WHEN MATCHED THEN DELETE", err: true,
			want: `expected expression after USING, found "ON"`},
		{name: "missing on", sql: "MERGE INTO t USING s WHEN MATCHED THEN DELETE", err: true,
			want: `expected ON, found "WHEN"`},
		{name: "no when", sql: "MERGE INTO t USING s ON a", err: true,
			want: "expected WHEN, found end of input"},
		{name: "missing then", sql: "MERGE INTO t USING s ON a WHEN MATCHED DELETE", err: true,
			want: `expected THEN, found "DELETE"`},
		{name: "bad match", sql: "MERGE INTO t USING s ON a WHEN FOUND THEN DELETE", err: true,
			want: `expected NOT, found "FOUND"`},
		{name: "bad by", sql: "MERGE INTO t USING s ON a WHEN NOT MATCHED BY x THEN DELETE", err: true,
			want: `expected TARGET, found "x"`},
		{name: "insert when matched", sql: "MERGE INTO t USING s ON a WHEN MATCHED THEN INSERT VALUES (1)", err: true,
			want: `expected UPDATE, DELETE or DO NOTHING, found "INSERT"`},
		{name: "delete when not matched", sql: "MERGE INTO t USING s ON a WHEN NOT MATCHED THEN DELETE", err: true,
			want: `expected INSERT or DO NOTHING, found "DELETE"`},
		{name: "insert without values", sql: "MERGE INTO t USING s ON a WHEN NOT MATCHED THEN INSERT (a)", err: true,
			want: "expected VALUES, found end of input"},
		{name: "empty and", sql: "MERGE INTO t USING s ON a WHEN MATCHED AND THEN DELETE", err: true,
			want: `expected condition after AND, found "THEN"`},
		{name: "merge in cte", sql: "WITH m AS (MERGE INTO t USING s ON a WHEN MATCHED THEN DELETE) SELECT 1", err: true,
			want: "a data-modifying statement in WITH is not supported"},
	})
}
