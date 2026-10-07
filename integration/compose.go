package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	pg "github.com/pganalyze/pg_query_go/v6"

	"github.com/mochams/glimt/internal/render"
	"github.com/mochams/glimt/internal/syntax"
)

// The composition oracle's checks.
const (
	checkComposeWhere = "compose-where" // an added WHERE condition lands where Postgres's WHERE is
	checkComposeTail  = "compose-tail"  // a replaced ORDER BY and added LIMIT and OFFSET do too
	checkComposeCount = "compose-count" // the count query counts the query without its tail
)

// probeSQL is the fixed SQL the oracle's compositions write at each hole,
// so the expected trees are easy to build.
var probeSQL = map[render.Hole]string{render.HoleWhere: "glimt_c IS NULL", render.HoleOrder: "glimt_o"}

// renderProbe renders tmpl as p says, filling each hole with probeSQL.
func renderProbe(tmpl *render.Template, values []any, p render.Plan) (string, error) {
	var r render.Run
	if err := tmpl.Begin(&r, values, p); err != nil {
		return "", err
	}

	for h := r.Next(); h != render.Done; h = r.Next() {
		r.Writer().SQL(probeSQL[h])
	}

	sql, _, err := r.Finish()

	return sql, err
}

// refusals are the errors a composition can rightly get from a statement
// that doesn't allow it.
var refusals = []string{"can only be added", "can't be added", "can't replace", "only a query can be counted"}

// checkComposed composes p's statement three ways and checks each against the
// tree Postgres builds for the source, edited as the composition should
// change it. source is the statement with params as placeholders.
func checkComposed(p *syntax.Parsed, source string) []finding {
	tmpl, err := render.Compile(p)
	if err != nil {
		return nil // reported by the render check
	}

	values := make([]any, len(tmpl.Names()))
	for i := range values {
		values[i] = i
	}

	var out []finding

	for _, v := range []struct {
		check string
		plan  render.Plan
	}{
		{checkComposeWhere, render.Plan{Where: true}},
		{checkComposeTail, render.Plan{Order: render.OrderReplace, Limit: 1, Offset: 2}},
		{checkComposeCount, render.Plan{Count: true}},
	} {
		sql, err := renderProbe(tmpl, values, v.plan)
		if f := compareComposed(v.check, p.Src, source, sql, err); f != nil {
			out = append(out, *f)
		}
	}

	return out
}

// compareComposed compares the tree of the composed sql with the tree the
// composition should give, built from the source's tree.
func compareComposed(check, src, source, sql string, err error) *finding {
	if err != nil {
		if refused(err) {
			return nil
		}

		return &finding{check, src, err.Error()}
	}

	want, err := expectedTree(check, source)
	if err != nil {
		return &finding{check, src, "building the expected tree: " + err.Error()}
	}

	got, err := normalizedTree(sql)

	switch {
	case err != nil:
		return &finding{check, src, fmt.Sprintf("Postgres rejects the composed %q: %v", sql, err)}
	case got != want:
		return &finding{check, src, fmt.Sprintf("the composed %q parses differently", sql)}
	}

	return nil
}

// refused reports whether err is a composition the statement rightly refuses.
func refused(err error) bool {
	for _, r := range refusals {
		if strings.Contains(err.Error(), r) {
			return true
		}
	}

	return false
}

// expectedTree returns Postgres's tree for source, edited as check's
// composition changes it, normalized.
func expectedTree(check, source string) (string, error) {
	tree, err := decodedTree(source)
	if err != nil {
		return "", err
	}

	node, err := statementNode(tree)
	if err != nil {
		return "", err
	}

	switch check {
	case checkComposeWhere:
		err = addWhere(node)
	case checkComposeTail:
		err = replaceTail(node)
	case checkComposeCount:
		tree, err = countOf(node)
	}

	if err != nil {
		return "", err
	}

	return encodeTree(tree)
}

// addWhere ANDs the probe's condition with node's WHERE clause, flattening
// an AND as Postgres's parser does.
func addWhere(node map[string]any) error {
	pred, err := clauseOf("SELECT WHERE glimt_c IS NULL", "whereClause")
	if err != nil {
		return err
	}

	where, ok := node["whereClause"]
	if !ok {
		node["whereClause"] = pred

		return nil
	}

	if and, ok := andArgs(where); ok {
		and["args"] = append(and["args"].([]any), pred)

		return nil
	}

	node["whereClause"] = map[string]any{"BoolExpr": map[string]any{"boolop": "AND_EXPR", "args": []any{where, pred}}}

	return nil
}

// andArgs returns the body of an AND expression node.
func andArgs(n any) (map[string]any, bool) {
	m, ok := n.(map[string]any)
	if !ok {
		return nil, false
	}

	b, ok := m["BoolExpr"].(map[string]any)

	return b, ok && b["boolop"] == "AND_EXPR"
}

// replaceTail sets node's ORDER BY to the probe's and its LIMIT and OFFSET to params.
func replaceTail(node map[string]any) error {
	sort, err := clauseOf("SELECT 1 ORDER BY glimt_o", "sortClause")
	if err != nil {
		return err
	}

	node["sortClause"] = sort
	node["limitCount"] = map[string]any{"ParamRef": map[string]any{}}
	node["limitOffset"] = map[string]any{"ParamRef": map[string]any{}}
	node["limitOption"] = "LIMIT_OPTION_COUNT"

	return nil
}

// countOf returns the tree of SELECT count(*) over node without its tail.
func countOf(node map[string]any) (any, error) {
	for _, k := range []string{"sortClause", "limitCount", "limitOffset", "lockingClause"} {
		delete(node, k)
	}

	node["limitOption"] = "LIMIT_OPTION_DEFAULT"

	tree, err := decodedTree("SELECT count(*) FROM (SELECT 1) AS t")
	if err != nil {
		return nil, err
	}

	outer, err := statementNode(tree)
	if err != nil {
		return nil, err
	}

	sub, ok := dig(outer, "fromClause", 0, "RangeSubselect").(map[string]any)
	if !ok {
		return nil, errors.New("no subquery in the count template")
	}

	sub["subquery"] = map[string]any{"SelectStmt": node}

	return tree, nil
}

// clauseOf returns one clause of the statement Postgres parses from sql.
func clauseOf(sql, key string) (any, error) {
	tree, err := decodedTree(sql)
	if err != nil {
		return nil, err
	}

	node, err := statementNode(tree)
	if err != nil {
		return nil, err
	}

	return node[key], nil
}

// statementNode returns the body of the single statement in a decoded tree.
func statementNode(tree any) (map[string]any, error) {
	stmt, ok := dig(tree, "stmts", 0, "stmt").(map[string]any)
	if !ok || len(stmt) != 1 {
		return nil, errors.New("not a single statement")
	}

	for _, body := range stmt {
		if node, ok := body.(map[string]any); ok {
			return node, nil
		}
	}

	return nil, errors.New("no statement body")
}

// dig follows map keys and slice indexes into a decoded JSON value.
func dig(v any, path ...any) any {
	for _, step := range path {
		switch s := step.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}

			v = m[s]
		case int:
			a, ok := v.([]any)
			if !ok || s >= len(a) {
				return nil
			}

			v = a[s]
		}
	}

	return v
}

// decodedTree returns Postgres's parse tree for sql as decoded JSON.
func decodedTree(sql string) (any, error) {
	raw, err := pg.ParseToJSON(sql)
	if err != nil {
		return nil, err
	}

	var tree any

	return tree, json.Unmarshal([]byte(raw), &tree)
}

// normalizedTree returns Postgres's tree for sql, normalized.
func normalizedTree(sql string) (string, error) {
	tree, err := decodedTree(sql)
	if err != nil {
		return "", err
	}

	return encodeTree(tree)
}

// encodeTree encodes a decoded tree without positions or param numbers.
// Param numbers are left out because composed values are numbered after the
// query's own params, which the render check already verifies.
func encodeTree(tree any) (string, error) {
	out, err := json.Marshal(stripParamNumbers(stripPositions(tree)))

	return string(out), err
}

// stripParamNumbers removes the number of every ParamRef node.
func stripParamNumbers(v any) any {
	switch v := v.(type) {
	case map[string]any:
		if ref, ok := v["ParamRef"].(map[string]any); ok {
			delete(ref, "number")
		}

		for _, child := range v {
			stripParamNumbers(child)
		}
	case []any:
		for _, child := range v {
			stripParamNumbers(child)
		}
	}

	return v
}
