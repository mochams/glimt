package render

import (
	"strings"
	"testing"
)

// clauseNames names each clause kind in marks.
var clauseNames = [numClauses]string{"where", "order", "limit", "offset", "fetch", "lock"}

// marked writes a template with each param as :name and each clause edge as
// a mark: {where.start}, {where.body} and {where.end} around a clause the
// query has, {where.insert} where it would go.
func marked(t *Template) string {
	var b strings.Builder
	for i, s := range t.slots {
		b.WriteString(t.parts[i])

		if s.isParam() {
			b.WriteString(":" + t.names[s.param])

			continue
		}

		b.WriteString("{" + clauseNames[s.clause] + "." + edgeName(&t.clauses[s.clause], i) + "}")
	}

	b.WriteString(t.parts[len(t.parts)-1])

	return b.String()
}

// edgeName names which edge of c slot i is.
func edgeName(c *clause, i int) string {
	switch {
	case c.absent:
		return "insert"
	case i == c.start:
		return "start"
	case i == c.body:
		return "body"
	case i == c.end:
		return "end"
	}

	return "?"
}

func TestClauses(t *testing.T) {
	tests := []struct {
		src  string
		want string
		kind statementKind
	}{
		{"SELECT a FROM t WHERE b = :b ORDER BY a LIMIT :n",
			"SELECT a FROM t{where.start} WHERE {where.body}b = :b{where.end}" +
				"{order.start} ORDER BY {order.body}a{order.end}" +
				"{limit.start} LIMIT {limit.body}:n{limit.end}{offset.insert}", selectStatement},
		{"SELECT a FROM t GROUP BY a",
			"SELECT a FROM t{where.insert} GROUP BY a{order.insert}{limit.insert}{offset.insert}", selectStatement},
		{"SELECT a FROM t OFFSET 5 FETCH FIRST 1 ROWS ONLY FOR UPDATE",
			"SELECT a FROM t{where.insert}{order.insert}{offset.start} OFFSET {offset.body}5{offset.end}" +
				"{fetch.start} FETCH {fetch.body}FIRST 1 ROWS ONLY{fetch.end}" +
				"{lock.start} FOR {lock.body}UPDATE{lock.end}", selectStatement},
		{"SELECT a FROM t FOR UPDATE LIMIT 1",
			"SELECT a FROM t{where.insert}{order.insert}{lock.start} FOR {lock.body}UPDATE{lock.end}" +
				"{limit.start} LIMIT {limit.body}1{limit.end}{offset.insert}", selectStatement},
		{"SELECT a FROM t OFFSET 5 LIMIT 1",
			"SELECT a FROM t{where.insert}{order.insert}{offset.start} OFFSET {offset.body}5{offset.end}" +
				"{limit.start} LIMIT {limit.body}1{limit.end}", selectStatement},
		{"SELECT 1 UNION SELECT 2 ORDER BY 1",
			"SELECT 1 UNION SELECT 2{order.start} ORDER BY {order.body}1{order.end}{limit.insert}{offset.insert}",
			queryStatement},
		{"WITH c AS (SELECT 1 FROM x WHERE y LIMIT 1) SELECT * FROM c WHERE z IN (SELECT 1 WHERE w)",
			"WITH c AS (SELECT 1 FROM x WHERE y LIMIT 1) SELECT * FROM c{where.start} WHERE {where.body}" +
				"z IN (SELECT 1 WHERE w){where.end}{order.insert}{limit.insert}{offset.insert}", selectStatement},
		{"UPDATE t SET a = 1 FROM s RETURNING a", "UPDATE t SET a = 1 FROM s{where.insert} RETURNING a", dmlStatement},
		{"DELETE FROM t USING s WHERE x", "DELETE FROM t USING s{where.start} WHERE {where.body}x{where.end}", dmlStatement},
		{"UPDATE t SET a = 1 WHERE CURRENT OF c", "UPDATE t SET a = 1 WHERE CURRENT OF c", dmlStatement},
		{"INSERT INTO t SELECT 1 WHERE x", "INSERT INTO t SELECT 1 WHERE x", fixedStatement},
		{"VALUES (1)", "VALUES (1){order.insert}{limit.insert}{offset.insert}", queryStatement},
		{"(SELECT 0 LIMIT 0)",
			"(SELECT 0{limit.start} LIMIT {limit.body}0{limit.end}{offset.insert}){order.insert}", queryStatement},
		{"((SELECT a FROM t ORDER BY a) OFFSET 2 FOR UPDATE)",
			"((SELECT a FROM t{order.start} ORDER BY {order.body}a{order.end}){offset.start} OFFSET {offset.body}2" +
				"{offset.end}{limit.insert}{lock.start} FOR {lock.body}UPDATE{lock.end})", queryStatement},
		{"(SELECT a FROM t LIMIT 1) ORDER BY a",
			"(SELECT a FROM t{limit.start} LIMIT {limit.body}1{limit.end}{offset.insert})" +
				"{order.start} ORDER BY {order.body}a{order.end}", queryStatement},
		{"(SELECT a FROM t ORDER BY a) LIMIT 1",
			"(SELECT a FROM t{order.start} ORDER BY {order.body}a{order.end})" +
				"{limit.start} LIMIT {limit.body}1{limit.end}{offset.insert}", queryStatement},
		{"(SELECT 1 LIMIT 1) UNION SELECT 2",
			"(SELECT 1 LIMIT 1) UNION SELECT 2{order.insert}{limit.insert}{offset.insert}", queryStatement},
	}

	for _, tt := range tests {
		tmpl := compileSQL(t, tt.src)
		if got := marked(tmpl); got != tt.want || tmpl.kind != tt.kind {
			t.Errorf("clauses of %q:\n%s (kind %d)\nwant\n%s (kind %d)", tt.src, got, tmpl.kind, tt.want, tt.kind)
		}
	}
}

func TestClausesRecordParams(t *testing.T) {
	tmpl := compileSQL(t, "SELECT a FROM t WHERE b = :b ORDER BY f(:x) LIMIT 10 OFFSET :o")

	want := [numClauses]bool{clauseWhere: true, clauseOrder: true, clauseOffset: true}
	for k, params := range want {
		if c := tmpl.clauses[k]; c.params != params {
			t.Errorf("%s.params = %v, want %v", clauseNames[k], c.params, params)
		}
	}

	if c := tmpl.clauses[clauseFetch]; c.start != -1 || c.has() {
		t.Errorf("a missing FETCH has cuts: %+v", c)
	}

	if c := compileSQL(t, "UPDATE t SET a = 1 WHERE CURRENT OF c").clauses[clauseWhere]; c.start != -1 {
		t.Errorf("WHERE CURRENT OF has cuts: %+v", c)
	}
}

func TestClauseIntro(t *testing.T) {
	for k, intro := range clauseIntro {
		if intro != " "+clauseKeyword[k]+" " {
			t.Errorf("clauseIntro[%s] = %q", clauseNames[k], intro)
		}
	}
}

func TestMissingPartner(t *testing.T) {
	tests := []struct {
		k             clauseKind
		limit, offset bool
		want          clauseKind
		ok            bool
	}{
		{clauseOffset, false, true, clauseLimit, true},
		{clauseOffset, true, true, 0, false},
		{clauseLimit, true, false, clauseOffset, true},
		{clauseFetch, true, false, clauseOffset, true},
		{clauseLimit, true, true, 0, false},
		{clauseLock, false, false, 0, false},
	}

	for _, tt := range tests {
		if got, ok := missingPartner(tt.k, tt.limit, tt.offset); got != tt.want || ok != tt.ok {
			t.Errorf("missingPartner(%s, %v, %v) = %v, %v", clauseNames[tt.k], tt.limit, tt.offset, got, ok)
		}
	}
}
