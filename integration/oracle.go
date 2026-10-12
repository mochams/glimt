package integration

import (
	"fmt"
	"strings"

	pg "github.com/pganalyze/pg_query_go/v6"

	"github.com/mochams/glimt/internal/render"
	"github.com/mochams/glimt/internal/syntax"
)

// The oracle's checks.
const (
	checkTokens  = "tokens"  // glimt's tokens match Postgres's scanner
	checkRender  = "render"  // rendered SQL parses to the same tree as the source
	checkClauses = "clauses" // clause bodies end where Postgres's do
	checkAccept  = "accept"  // glimt accepts the statements Postgres accepts
	checkParams  = "params"  // glimt finds no params in SQL Postgres accepts
	// checkPresence, in presence.go: each clause is in glimt's AST exactly when it is in Postgres's tree
)

// finding is one way glimt disagreed with Postgres about a statement.
type finding struct {
	check string
	src   string
	msg   string
}

// statementKinds are the Postgres statement kinds glimt models.
var statementKinds = map[string]bool{
	"SelectStmt": true, "InsertStmt": true, "UpdateStmt": true, "DeleteStmt": true, "MergeStmt": true,
}

// knownRejections are glimt errors for SQL Postgres accepts that glimt
// rejects on purpose, each with its reason.
var knownRejections = map[string]string{
	"not supported":        "glimt reports what it doesn't support",
	"positional parameter": "glimt params are :name, never $n",
	"names may only hold":  "glimt param names are ASCII; after \"[\", :x is a param (the documented a[:hi])",
}

// check runs every oracle check on one statement.
func check(src string) []finding {
	if strings.ContainsRune(src, 0) {
		return nil // pg_query reads a C string and stops at the NUL, so it sees less than glimt
	}

	p, err := syntax.Parse(src)
	if err != nil {
		return checkRejection(src, err)
	}

	edits := paramEdits(p)

	want, err := pgTree(applyEdits(src, edits))
	if err != nil {
		return nil // invalid SQL glimt accepts: glimt is not a validator
	}

	out := make([]finding, 0, 4)
	out = append(out, checkFoundParams(p)...)
	out = append(out, checkTokenStream(p)...)
	out = append(out, checkRendered(p, want)...)
	out = append(out, checkClauseSpans(p, edits, want)...)
	out = append(out, checkClausePresence(p, want)...)
	out = append(out, checkComposed(p, applyEdits(src, edits))...)

	return out
}

// checkRejection reports a glimt error on a statement Postgres accepts,
// unless glimt rejects it on purpose.
func checkRejection(src string, err error) []finding {
	for known := range knownRejections {
		if strings.Contains(err.Error(), known) {
			return nil
		}
	}

	if kind, pgErr := statementKind(src); pgErr != nil || !statementKinds[kind] {
		return nil
	}

	return []finding{{checkAccept, src, err.Error()}}
}

// checkFoundParams reports params glimt found in SQL Postgres accepts as it
// is, where the ":" must be something else, except the documented a[:hi].
func checkFoundParams(p *syntax.Parsed) []finding {
	if len(p.Params) == 0 {
		return nil
	}

	if _, err := pg.Parse(p.Src); err != nil {
		return nil
	}

	for _, prm := range p.Params {
		if prm.Tok == 0 || p.Tokens[prm.Tok-1].Kind != syntax.LBRACK {
			return []finding{{checkParams, p.Src, fmt.Sprintf("found param :%s", prm.Name)}}
		}
	}

	return nil
}

// keywordKinds maps glimt's keyword categories to Postgres's.
var keywordKinds = map[syntax.KeywordCategory]pg.KeywordKind{
	syntax.UnreservedKeyword:   pg.KeywordKind_UNRESERVED_KEYWORD,
	syntax.ColNameKeyword:      pg.KeywordKind_COL_NAME_KEYWORD,
	syntax.TypeFuncNameKeyword: pg.KeywordKind_TYPE_FUNC_NAME_KEYWORD,
	syntax.ReservedKeyword:     pg.KeywordKind_RESERVED_KEYWORD,
}

// checkTokenStream compares glimt's tokens with Postgres's scanner: the same
// boundaries, and the same category for each keyword glimt knows. Statements
// with params are skipped, since Postgres reads :name differently.
func checkTokenStream(p *syntax.Parsed) []finding {
	if len(p.Params) > 0 {
		return nil
	}

	res, err := pg.Scan(p.Src)
	if err != nil {
		return []finding{{checkTokens, p.Src, "Postgres can't scan it: " + err.Error()}}
	}

	theirs := make([]*pg.ScanToken, 0, len(res.Tokens))
	for _, t := range res.Tokens {
		if t.Token != pg.Token_SQL_COMMENT && t.Token != pg.Token_C_COMMENT {
			theirs = append(theirs, t)
		}
	}

	ours := p.Tokens[:len(p.Tokens)-1] // without EOF
	if len(ours) != len(theirs) {
		return []finding{{checkTokens, p.Src, fmt.Sprintf("%d tokens, Postgres has %d", len(ours), len(theirs))}}
	}

	for i, t := range ours {
		if msg := compareToken(p.Src, t, theirs[i]); msg != "" {
			return []finding{{checkTokens, p.Src, msg}}
		}
	}

	return nil
}

// compareToken returns how glimt's token differs from Postgres's, or "".
func compareToken(src string, ours syntax.Token, theirs *pg.ScanToken) string {
	if ours.Pos != theirs.Start || ours.End != theirs.End {
		return fmt.Sprintf("token %q, Postgres has %q", ours.Text(src), src[theirs.Start:theirs.End])
	}

	if ours.Kw != 0 && keywordKinds[ours.Kw.Category()] != theirs.KeywordKind {
		return fmt.Sprintf("keyword %v is %v, Postgres says %v", ours.Kw, ours.Kw.Category(), theirs.KeywordKind)
	}

	return ""
}

// checkRendered renders p with a single value for each param and checks
// that Postgres parses the result to want, its tree for the source.
func checkRendered(p *syntax.Parsed, want string) []finding {
	tmpl, err := render.Compile(p)
	if err != nil {
		return []finding{{checkRender, p.Src, "compile: " + err.Error()}}
	}

	values := make([]any, len(tmpl.Names()))
	for i := range values {
		values[i] = i
	}

	sql, _, err := tmpl.Render(values)
	if err != nil {
		return []finding{{checkRender, p.Src, "render: " + err.Error()}}
	}

	got, err := pgTree(sql)

	switch {
	case err != nil:
		return []finding{{checkRender, p.Src, fmt.Sprintf("Postgres rejects the rendered %q: %v", sql, err)}}
	case got != want:
		return []finding{{checkRender, p.Src, fmt.Sprintf("the rendered %q parses differently", sql)}}
	}

	return nil
}

// checkClauseSpans wraps every clause body glimt found in parentheses and
// checks that Postgres's tree doesn't change. Postgres drops redundant
// parentheses, so a change means glimt cut a clause in the wrong place.
func checkClauseSpans(p *syntax.Parsed, params []edit, want string) []finding {
	spans := clauseSpans(p)
	if len(spans) == 0 {
		return nil
	}

	edits := append(parenEdits(p, spans), params...)
	sql := applyEdits(p.Src, edits)

	got, err := pgTree(sql)

	switch {
	case err != nil:
		return []finding{{checkClauses, p.Src, fmt.Sprintf("Postgres rejects %q: %v", sql, err)}}
	case got != want:
		return []finding{{checkClauses, p.Src, fmt.Sprintf("%q parses differently", sql)}}
	}

	return nil
}
