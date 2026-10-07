package integration

import (
	"encoding/json"
	"fmt"
	"strings"

	pg "github.com/pganalyze/pg_query_go/v6"
)

// isPositionKey reports whether a field of a Postgres parse tree records
// where something was in the source: location, arg_location and the like,
// and stmt_len. Two trees for the same statement differ only in these.
func isPositionKey(k string) bool {
	return k == "stmt_len" || strings.HasSuffix(k, "location")
}

// pgTree parses sql with Postgres's parser and returns its parse tree as
// JSON without positions, so trees for differently formatted SQL compare equal.
func pgTree(sql string) (string, error) {
	raw, err := pg.ParseToJSON(sql)
	if err != nil {
		return "", err
	}

	var tree any
	if err := json.Unmarshal([]byte(raw), &tree); err != nil {
		return "", fmt.Errorf("decoding the parse tree: %w", err)
	}

	out, err := json.Marshal(stripPositions(tree))
	if err != nil {
		return "", fmt.Errorf("encoding the parse tree: %w", err)
	}

	return string(out), nil
}

// stripPositions removes the position fields from a decoded JSON value.
func stripPositions(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for k, child := range v {
			if isPositionKey(k) {
				delete(v, k)

				continue
			}

			v[k] = stripPositions(child)
		}
	case []any:
		for i, child := range v {
			v[i] = stripPositions(child)
		}
	}

	return v
}

// statementKind returns the kind of the single statement in sql as Postgres
// parses it, such as "SelectStmt", or an error when Postgres rejects sql.
func statementKind(sql string) (string, error) {
	res, err := pg.Parse(sql)
	if err != nil {
		return "", err
	}

	if len(res.Stmts) != 1 {
		return "", fmt.Errorf("%d statements", len(res.Stmts))
	}

	switch res.Stmts[0].Stmt.Node.(type) {
	case *pg.Node_SelectStmt:
		return "SelectStmt", nil
	case *pg.Node_InsertStmt:
		return "InsertStmt", nil
	case *pg.Node_UpdateStmt:
		return "UpdateStmt", nil
	case *pg.Node_DeleteStmt:
		return "DeleteStmt", nil
	case *pg.Node_MergeStmt:
		return "MergeStmt", nil
	}

	return "other", nil
}
