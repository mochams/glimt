package glimt

import (
	"fmt"
	"strings"

	"github.com/mochams/glimt/internal/render"
)

// Pred is a condition for [Builder.Where]. Make it with [Eq], [In] and the
// other functions below, and combine it with [And], [Or] and [Not].
//
// Its column is written into the SQL as given, so it must come from your
// code or from a [Columns], never from a request. Its value is always bound
// as an arg. The zero Pred is no condition, and is skipped.
type Pred struct {
	op    predOp
	col   string
	val   any
	preds []Pred // the operands of And, Or and Not
}

// predOp is the kind of a Pred.
type predOp uint8

// Pred kinds.
const (
	opNone predOp = iota
	opEq
	opNe
	opLt
	opLe
	opGt
	opGe
	opLike
	opNotLike
	opILike
	opNotILike
	opIn
	opNotIn
	opIsNull
	opIsNotNull
	opAnd
	opOr
	opNot
)

// opSQL is the SQL written after the column for each comparison.
var opSQL = [...]string{
	opEq: " = ", opNe: " <> ", opLt: " < ", opLe: " <= ", opGt: " > ", opGe: " >= ",
	opLike: " LIKE ", opNotLike: " NOT LIKE ", opILike: " ILIKE ", opNotILike: " NOT ILIKE ",
	opIn: " IN (", opNotIn: " NOT IN (", opIsNull: " IS NULL", opIsNotNull: " IS NOT NULL",
}

// opName names the operators that take a value, in errors.
var opName = [...]string{
	opEq: "Eq", opNe: "Ne", opLt: "Lt", opLe: "Le", opGt: "Gt", opGe: "Ge", opIn: "In", opNotIn: "NotIn",
}

// nullTest names the Pred to use in place of comparing with nil, for each
// operator that takes a single value.
var nullTest = [opIn]string{opEq: ": use IsNull", opNe: ": use IsNotNull"}

// Eq returns the condition col = v.
//
// An untyped nil v is an error at Build, since col = NULL is never true; use
// [IsNull]. A nil pointer binds NULL, as in database/sql, and so matches no
// rows.
func Eq(col string, v any) Pred { return Pred{op: opEq, col: col, val: v} }

// Ne returns the condition col <> v. As with [Eq], an untyped nil v is an
// error; use [IsNotNull].
func Ne(col string, v any) Pred { return Pred{op: opNe, col: col, val: v} }

// Lt returns the condition col < v.
func Lt(col string, v any) Pred { return Pred{op: opLt, col: col, val: v} }

// Le returns the condition col <= v.
func Le(col string, v any) Pred { return Pred{op: opLe, col: col, val: v} }

// Gt returns the condition col > v.
func Gt(col string, v any) Pred { return Pred{op: opGt, col: col, val: v} }

// Ge returns the condition col >= v.
func Ge(col string, v any) Pred { return Pred{op: opGe, col: col, val: v} }

// In returns the condition col IN (…), with one placeholder per element of
// the slice list. []byte, arrays and driver.Valuer types count as one
// value, not a list. An empty list is an error at Build, since IN () isn't
// valid SQL; use [If] to leave the condition out instead.
func In(col string, list any) Pred { return Pred{op: opIn, col: col, val: list} }

// NotIn returns the condition col NOT IN (…). See [In].
func NotIn(col string, list any) Pred { return Pred{op: opNotIn, col: col, val: list} }

// IsNull returns the condition col IS NULL.
func IsNull(col string) Pred { return Pred{op: opIsNull, col: col} }

// IsNotNull returns the condition col IS NOT NULL.
func IsNotNull(col string) Pred { return Pred{op: opIsNotNull, col: col} }

// Like returns the condition col LIKE pattern, where % and _ in pattern are
// wildcards. To match text as written, use [Contains], [StartsWith] or
// [EndsWith].
func Like(col, pattern string) Pred { return Pred{op: opLike, col: col, val: pattern} }

// NotLike returns the condition col NOT LIKE pattern.
func NotLike(col, pattern string) Pred { return Pred{op: opNotLike, col: col, val: pattern} }

// ILike returns the condition col ILIKE pattern: [Like], ignoring case.
func ILike(col, pattern string) Pred { return Pred{op: opILike, col: col, val: pattern} }

// NotILike returns the condition col NOT ILIKE pattern.
func NotILike(col, pattern string) Pred { return Pred{op: opNotILike, col: col, val: pattern} }

// Contains matches rows where col contains s. Unlike in [Like], % and _ in
// s match themselves, so s can be a user's search text.
func Contains(col, s string) Pred { return Like(col, "%"+escapeLike(s)+"%") }

// StartsWith matches rows where col starts with s. See [Contains].
func StartsWith(col, s string) Pred { return Like(col, escapeLike(s)+"%") }

// EndsWith matches rows where col ends with s. See [Contains].
func EndsWith(col, s string) Pred { return Like(col, "%"+escapeLike(s)) }

// IContains is [Contains], ignoring case.
func IContains(col, s string) Pred { return ILike(col, "%"+escapeLike(s)+"%") }

// IStartsWith is [StartsWith], ignoring case.
func IStartsWith(col, s string) Pred { return ILike(col, escapeLike(s)+"%") }

// IEndsWith is [EndsWith], ignoring case.
func IEndsWith(col, s string) Pred { return ILike(col, "%"+escapeLike(s)) }

// And returns a condition that holds when all of preds hold. Zero Preds are
// skipped, and an And with nothing left adds no condition.
func And(preds ...Pred) Pred { return Pred{op: opAnd, preds: preds} }

// Or returns a condition that holds when any of preds holds. Zero Preds are
// skipped, and an Or with nothing left adds no condition.
func Or(preds ...Pred) Pred { return Pred{op: opOr, preds: preds} }

// Not returns the negation of p. When p adds no condition, neither does Not.
func Not(p Pred) Pred { return Pred{op: opNot, preds: []Pred{p}} }

// If returns p when cond is true, and the zero Pred, which adds no
// condition, when it is false. It keeps optional filters to one line:
//
//	b = b.Where(glimt.If(req.Status != "", glimt.Eq("o.status", req.Status)))
//
// As with any Go call, p is evaluated even when cond is false, so
// If(f != nil, Eq("x", *f)) panics on a nil f. Pass the pointer itself:
// If(f != nil, Eq("x", f)) binds the value f points to, as database/sql
// does.
func If(cond bool, p Pred) Pred {
	if cond {
		return p
	}

	return Pred{}
}

// likeEscaper escapes LIKE wildcards with backslashes, Postgres's default
// LIKE escape character.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// escapeLike makes s match itself in a LIKE pattern.
func escapeLike(s string) string {
	return likeEscaper.Replace(s)
}

// empty reports whether p adds no condition: it is zero, or a group whose
// operands all add none.
func (p Pred) empty() bool {
	switch p.op {
	case opNone:
		return true
	case opAnd, opOr, opNot:
		for _, q := range p.preds {
			if !q.empty() {
				return false
			}
		}

		return true
	}

	return false
}

// write writes p's SQL and binds its values.
func (p Pred) write(w *render.Writer) error {
	switch p.op {
	case opAnd:
		return writeGroup(w, p.preds, " AND ")
	case opOr:
		return writeGroup(w, p.preds, " OR ")
	case opNot:
		w.SQL("NOT (")

		if err := writeJoined(w, p.preds, ""); err != nil {
			return err
		}

		w.SQL(")")

		return nil
	}

	if err := checkColumn(p.col); err != nil {
		return err
	}

	w.SQL(p.col)
	w.SQL(opSQL[p.op])

	return p.writeValue(w)
}

// writeValue binds the value of a comparison, if it has one.
func (p Pred) writeValue(w *render.Writer) error {
	switch p.op {
	case opIsNull, opIsNotNull:
		return nil
	case opIn, opNotIn:
		if err := w.List(p.val); err != nil {
			return fmt.Errorf("the value of %s(%q) %w", opName[p.op], p.col, err)
		}

		w.SQL(")")

		return nil
	}

	if p.val == nil {
		return fmt.Errorf("%s(%q) has a %w%s", opName[p.op], p.col, ErrNilValue, nullTest[p.op])
	}

	w.Value(p.val)

	return nil
}

// writeGroup writes the non-empty preds joined by sep, in parentheses when
// there is more than one, so the group binds as one condition.
func writeGroup(w *render.Writer, preds []Pred, sep string) error {
	n := 0
	for _, p := range preds {
		if !p.empty() {
			n++
		}
	}

	if n < 2 {
		return writeJoined(w, preds, sep)
	}

	w.SQL("(")

	if err := writeJoined(w, preds, sep); err != nil {
		return err
	}

	w.SQL(")")

	return nil
}

// writeJoined writes the non-empty preds joined by sep.
func writeJoined(w *render.Writer, preds []Pred, sep string) error {
	j := joiner{w: w, sep: sep}
	for _, p := range preds {
		if err := j.add(p); err != nil {
			return err
		}
	}

	return nil
}

// joiner writes conditions one at a time, joined by sep, skipping empty ones.
type joiner struct {
	w       *render.Writer
	sep     string
	written int
}

// add writes p after a separator, unless p is empty.
func (j *joiner) add(p Pred) error {
	if p.empty() {
		return nil
	}

	if j.written > 0 {
		j.w.SQL(j.sep)
	}

	j.written++

	return p.write(j.w)
}
