package integration

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/mochams/glimt/internal/syntax"
)

// checkPresence is the oracle check that each clause is present in glimt's
// AST exactly when it is in Postgres's tree. The clause check can't see a
// clause folded into the one before it, as when a column labelled rows
// swallowed FROM; this check can.
const checkPresence = "presence"

// shape is which clauses one statement has, with the statements nested in
// it: CTE bodies, the two sides of a set operation, and an INSERT's source.
// Subqueries inside expressions are left out. Clauses are named by
// Postgres's field names, such as whereClause.
type shape struct {
	kind       string
	clauses    []string
	ctes       []*shape
	larg, rarg *shape
	source     *shape
}

// String writes the shape as kind{clauses}, followed by its nested
// statements, so two shapes compare as strings.
func (s *shape) String() string {
	if s == nil {
		return "-"
	}

	var b strings.Builder

	slices.Sort(s.clauses)
	fmt.Fprintf(&b, "%s{%s}", s.kind, strings.Join(s.clauses, ","))

	for _, c := range s.ctes {
		fmt.Fprintf(&b, " cte(%v)", c)
	}

	if s.larg != nil || s.rarg != nil {
		fmt.Fprintf(&b, " larg(%v) rarg(%v)", s.larg, s.rarg)
	}

	if s.source != nil {
		fmt.Fprintf(&b, " source(%v)", s.source)
	}

	return b.String()
}

// add records the clause named name when present holds.
func (s *shape) add(present bool, name string) {
	if present && !slices.Contains(s.clauses, name) {
		s.clauses = append(s.clauses, name)
	}
}

// checkClausePresence compares the shape of glimt's AST with the shape of
// Postgres's tree, the position-free JSON in want.
func checkClausePresence(p *syntax.Parsed, want string) []finding {
	ours := statementShape(p.Stmt)
	if ours == nil {
		return nil // a statement glimt passes through as written
	}

	var tree struct {
		Stmts []struct {
			Stmt map[string]any `json:"stmt"`
		} `json:"stmts"`
	}

	if err := json.Unmarshal([]byte(want), &tree); err != nil || len(tree.Stmts) != 1 {
		return []finding{{checkPresence, p.Src, fmt.Sprintf("decoding Postgres's tree: %v", err)}}
	}

	theirs := pgStatementShape(tree.Stmts[0].Stmt)
	if got, want := ours.String(), theirs.String(); got != want {
		return []finding{{checkPresence, p.Src, fmt.Sprintf("glimt has %s, Postgres has %s", got, want)}}
	}

	return nil
}

// statementShape returns the shape of a statement glimt models, or nil.
func statementShape(s syntax.Statement) *shape {
	switch s := s.(type) {
	case *syntax.Query:
		return queryShape(s)
	case *syntax.Insert:
		sh := &shape{kind: "insert", ctes: cteShapes(s.With)}
		sh.add(s.Returning != nil, "returningList")

		if s.Source != nil {
			sh.source = queryShape(s.Source)
		}

		return sh
	case *syntax.Update:
		sh := &shape{kind: "update", ctes: cteShapes(s.With)}
		sh.add(s.From != nil, "fromClause")
		sh.add(s.Where != nil, "whereClause")
		sh.add(s.Returning != nil, "returningList")

		return sh
	case *syntax.Delete:
		sh := &shape{kind: "delete", ctes: cteShapes(s.With)}
		sh.add(s.Using != nil, "usingClause")
		sh.add(s.Where != nil, "whereClause")
		sh.add(s.Returning != nil, "returningList")

		return sh
	case *syntax.Merge:
		return &shape{kind: "merge", ctes: cteShapes(s.With)}
	}

	return nil
}

// queryShape returns the shape of a query. Postgres has no node for
// parentheses: a parenthesized query's trailing clauses and WITH belong to
// the statement inside, so they go on that statement's shape.
func queryShape(q *syntax.Query) *shape {
	sh := setExprShape(q.Body)
	sh.ctes = append(sh.ctes, cteShapes(q.With)...)
	sh.add(q.OrderBy != nil, "sortClause")
	sh.add(q.Limit != nil || q.Fetch != nil, "limitCount")
	sh.add(q.Offset != nil, "limitOffset")
	sh.add(q.Locking != nil, "lockingClause")

	return sh
}

// setExprShape returns the shape of the body of a query.
func setExprShape(e syntax.SetExpr) *shape {
	sh := &shape{kind: "select"}

	switch e := e.(type) {
	case *syntax.Select:
		sh.add(e.From != nil, "fromClause")
		sh.add(e.Where != nil, "whereClause")
		sh.add(e.GroupBy != nil, "groupClause")
		sh.add(e.Having != nil, "havingClause")
		sh.add(e.Window != nil, "windowClause")
	case *syntax.TableQuery:
		sh.add(true, "fromClause") // TABLE t is SELECT * FROM t to Postgres
	case *syntax.SetOp:
		sh.larg, sh.rarg = setExprShape(e.Left), setExprShape(e.Right)
	case *syntax.ParenQuery:
		return queryShape(e.Query)
	}

	return sh
}

// cteShapes returns the shapes of a WITH clause's CTE bodies.
func cteShapes(w *syntax.With) []*shape {
	if w == nil {
		return nil
	}

	out := make([]*shape, len(w.CTEs))
	for i, c := range w.CTEs {
		out[i] = queryShape(c.Body)
	}

	return out
}

// pgClauses are the fields of each Postgres statement node the presence
// check compares.
var pgClauses = map[string]struct {
	kind   string
	fields []string
}{
	"SelectStmt": {"select", []string{
		"fromClause", "whereClause", "groupClause", "havingClause", "windowClause",
		"sortClause", "limitCount", "limitOffset", "lockingClause",
	}},
	"InsertStmt": {"insert", []string{"returningList"}},
	"UpdateStmt": {"update", []string{"fromClause", "whereClause", "returningList"}},
	"DeleteStmt": {"delete", []string{"usingClause", "whereClause", "returningList"}},
	"MergeStmt":  {"merge", nil},
}

// pgStatementShape returns the shape of a Postgres statement node, such as
// {"SelectStmt": {…}}, or nil for a kind glimt doesn't model.
func pgStatementShape(node map[string]any) *shape {
	for name, v := range node {
		if fields, ok := v.(map[string]any); ok {
			if _, known := pgClauses[name]; known {
				return pgShape(name, fields)
			}
		}
	}

	return nil
}

// pgShape returns the shape of a Postgres statement of the named kind.
func pgShape(name string, fields map[string]any) *shape {
	spec := pgClauses[name]
	sh := &shape{kind: spec.kind}

	for _, f := range spec.fields {
		_, present := fields[f]
		sh.add(present, f)
	}

	if w, ok := fields["withClause"].(map[string]any); ok {
		ctes, _ := w["ctes"].([]any)
		for _, c := range ctes {
			cte, _ := c.(map[string]any)["CommonTableExpr"].(map[string]any)
			body, _ := cte["ctequery"].(map[string]any)
			sh.ctes = append(sh.ctes, pgStatementShape(body))
		}
	}

	if larg, ok := fields["larg"].(map[string]any); ok {
		sh.larg = pgShape("SelectStmt", larg)
	}

	if rarg, ok := fields["rarg"].(map[string]any); ok {
		sh.rarg = pgShape("SelectStmt", rarg)
	}

	if src, ok := fields["selectStmt"].(map[string]any); ok {
		sh.source = pgStatementShape(src)
	}

	return sh
}
