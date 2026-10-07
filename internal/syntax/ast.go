package syntax

// Span is a half-open range [From, To) of token indexes.
type Span struct {
	From, To int32
}

// Bounds returns s. Every node embeds a Span, so every node gets this method
// and satisfies Node.
func (s Span) Bounds() Span {
	return s
}

// Empty reports whether s covers no tokens.
func (s Span) Empty() bool {
	return s.To <= s.From
}

// Contains reports whether inner lies within s.
func (s Span) Contains(inner Span) bool {
	return s.From <= inner.From && inner.To <= s.To
}

// Node is implemented by every AST node.
type Node interface {
	Bounds() Span
}

// Statement is a complete statement: *Query, *Insert, *Update, *Delete, *Merge or *Raw.
type Statement interface {
	Node
	stmt()
}

// SetExpr is the body of a query: *Select, *SetOp, *Values, *TableQuery or *ParenQuery.
type SetExpr interface {
	Node
	setExpr()
}

// Query is a SELECT, a VALUES list or a compound of them. OrderBy, Limit and
// Offset apply to the whole Body.
type Query struct {
	Span
	With    *With
	Body    SetExpr
	OrderBy *Expr
	Limit   *Expr
	Offset  *Expr
	Fetch   *Expr // FETCH { FIRST | NEXT } …, which replaces LIMIT
	Locking *Expr // FOR { UPDATE | SHARE | … } …, every locking clause in one span
}

// Select is a single SELECT without its trailing ORDER BY, LIMIT and OFFSET,
// which belong to the enclosing Query.
type Select struct {
	Span
	All        bool // SELECT ALL, the explicit default
	Distinct   bool
	DistinctOn *Expr // the list inside DISTINCT ON ( … )
	Columns    Expr  // the select list; may be empty, as Postgres allows
	From       *Expr // opaque: joins pass through untouched
	Where      *Expr
	GroupBy    *Expr
	Having     *Expr
	Window     *Expr // the window definitions after WINDOW
}

// SetOpKind is a set operation.
type SetOpKind uint8

// Set operations.
const (
	Union SetOpKind = iota + 1
	Intersect
	Except
)

// String returns the operation's keyword.
func (k SetOpKind) String() string {
	switch k {
	case Union:
		return "UNION"
	case Intersect:
		return "INTERSECT"
	case Except:
		return "EXCEPT"
	}

	return "SetOpKind(?)"
}

// SetOp combines two query bodies: Left UNION [ALL] Right, and so on.
type SetOp struct {
	Span
	Op    SetOpKind
	All   bool
	Left  SetExpr
	Right SetExpr
}

// Values is a VALUES list. Each row holds one Expr per column.
type Values struct {
	Span
	Rows [][]Expr
}

// TableQuery is TABLE name, shorthand for SELECT * FROM name.
type TableQuery struct {
	Span
	Only bool // TABLE ONLY: leave out tables that inherit from Name
	Name ObjectName
}

// ParenQuery is a parenthesized query used as a set operand: ( query ).
// Its Span includes the parentheses.
type ParenQuery struct {
	Span
	Query *Query
}

// With is a WITH clause.
type With struct {
	Span
	Recursive bool
	CTEs      []*CTE
}

// Materialization is a CTE's [NOT] MATERIALIZED option.
type Materialization uint8

// Materialization options.
const (
	MaterializeDefault Materialization = iota // no option given
	Materialized                              // AS MATERIALIZED
	NotMaterialized                           // AS NOT MATERIALIZED
)

// CTE is one common table expression: name [(cols)] AS [[NOT] MATERIALIZED]
// (body), with a recursive query's optional SEARCH and CYCLE options.
type CTE struct {
	Span
	Name         Ident
	Columns      []Ident
	Materialized Materialization
	Body         *Query
	Search       *Expr // after SEARCH: {BREADTH | DEPTH} FIRST BY cols SET col
	Cycle        *Expr // after CYCLE: cols SET col [TO v DEFAULT v] USING col
}

// Insert is an INSERT statement.
type Insert struct {
	Span
	With          *With
	Table         ObjectName
	Alias         *Ident
	Columns       []Target
	Overriding    Overriding
	Source        *Query // VALUES or a query; nil when DefaultValues is set
	DefaultValues bool
	OnConflict    *OnConflict
	Returning     *Expr
}

// Overriding is an INSERT's OVERRIDING { SYSTEM | USER } VALUE option.
type Overriding uint8

// Overriding options.
const (
	OverridingNone   Overriding = iota // no option given
	OverridingSystem                   // OVERRIDING SYSTEM VALUE
	OverridingUser                     // OVERRIDING USER VALUE
)

// OnConflict is an INSERT's ON CONFLICT clause.
type OnConflict struct {
	Span
	Target    *Expr        // "(cols) [WHERE …]" or "ON CONSTRAINT name"; nil when absent
	DoNothing bool         // DO NOTHING; otherwise DO UPDATE
	Set       []Assignment // DO UPDATE SET …
	Where     *Expr        // DO UPDATE … WHERE …
}

// Update is an UPDATE statement.
type Update struct {
	Span
	With      *With
	Only      bool // UPDATE ONLY: leave out tables that inherit from Table
	Table     ObjectName
	Alias     *Ident
	Set       []Assignment
	From      *Expr // opaque, like Select.From
	Where     *Expr
	Returning *Expr
}

// Delete is a DELETE statement.
type Delete struct {
	Span
	With      *With
	Only      bool // DELETE FROM ONLY
	Table     ObjectName
	Alias     *Ident
	Using     *Expr // opaque, like Select.From
	Where     *Expr
	Returning *Expr
}

// Assignment is one SET item: col = value, or (a, b) = value.
type Assignment struct {
	Span
	Targets []Target
	Tuple   bool // the targets were written in parentheses, even if only one
	Value   Expr
}

// Target is a column written to by SET or INSERT, optionally followed by a
// path of fields and subscripts: col, col.field, col[i] or data['key'].
type Target struct {
	Span
	Column Ident
	Path   *Expr // the .field and [subscript] chain after Column; nil when absent
}

// Merge is a MERGE statement.
type Merge struct {
	Span
	With      *With
	Only      bool // MERGE INTO ONLY
	Table     ObjectName
	Alias     *Ident
	Using     *Expr // the data source, alias included; opaque like Select.From
	On        *Expr // the join condition
	When      []*MergeWhen
	Returning *Expr
}

// MergeMatch is the condition kind of a MERGE WHEN clause.
type MergeMatch uint8

// MERGE WHEN conditions.
const (
	Matched            MergeMatch = iota + 1 // WHEN MATCHED
	NotMatched                               // WHEN NOT MATCHED [BY TARGET]
	NotMatchedBySource                       // WHEN NOT MATCHED BY SOURCE
)

// String returns the condition as written after WHEN.
func (m MergeMatch) String() string {
	switch m {
	case Matched:
		return "matched"
	case NotMatched:
		return "not matched"
	case NotMatchedBySource:
		return "not matched by source"
	}

	return "MergeMatch(?)"
}

// MergeAction is what a MERGE WHEN clause does.
type MergeAction uint8

// MERGE actions.
const (
	MergeDoNothing MergeAction = iota + 1 // DO NOTHING
	MergeUpdate                           // UPDATE SET …
	MergeDelete                           // DELETE
	MergeInsert                           // INSERT …
)

// String returns the action's keywords.
func (a MergeAction) String() string {
	switch a {
	case MergeDoNothing:
		return "do nothing"
	case MergeUpdate:
		return "update"
	case MergeDelete:
		return "delete"
	case MergeInsert:
		return "insert"
	}

	return "MergeAction(?)"
}

// MergeWhen is one WHEN clause of a MERGE.
type MergeWhen struct {
	Span
	Match         MergeMatch
	Cond          *Expr // the AND condition; nil when absent
	Action        MergeAction
	Set           []Assignment // MergeUpdate
	Columns       []Target     // MergeInsert
	Overriding    Overriding   // MergeInsert
	Values        []Expr       // MergeInsert: the VALUES row; nil with DefaultValues
	DefaultValues bool         // MergeInsert
}

// Raw is a statement glimt passes through without modelling it, such as DDL.
// Its params are still found.
type Raw struct {
	Span
	Body Expr
}

// Expr is an expression kept as a contiguous token span, with the positions of
// the params and subqueries inside it. A clause's Expr never includes the
// keyword that introduces it: for "WHERE active = true" it covers "active = true".
type Expr struct {
	Span
	Params     []*Param    // params in the span, outside any subquery, in order
	Subqueries []*Subquery // subqueries in the span, in order
}

// Subquery is a query in parentheses inside an expression. Its Span covers
// the parentheses; its Query's Span covers what is between them.
type Subquery struct {
	Span
	Query *Query
}

// ParamMode tells how a param is used.
type ParamMode uint8

// Param modes.
const (
	Scalar ParamMode = iota // a single value
	Expand                  // written exactly as IN (:name): expands to a list
)

// String returns the mode's name.
func (m ParamMode) String() string {
	if m == Expand {
		return "expand"
	}

	return "scalar"
}

// Param is one occurrence of a named parameter.
type Param struct {
	Name string // without the leading colon
	Tok  int32  // index of the PARAM token
	Mode ParamMode
}

// Ident is an identifier. Name is the text as written for a bare identifier,
// and the unquoted text for a quoted one.
type Ident struct {
	Name   string
	Quoted bool
	Tok    int32 // index of the identifier's token
}

// ObjectName is a possibly qualified name, such as schema.table.
type ObjectName struct {
	Span
	Parts []Ident
}

// Parsed is the result of Parse.
type Parsed struct {
	Src    string
	Tokens []Token
	Stmt   Statement
	Params []*Param // every param occurrence in source order, subqueries included
}

func (*Query) stmt()  {}
func (*Insert) stmt() {}
func (*Update) stmt() {}
func (*Delete) stmt() {}
func (*Merge) stmt()  {}
func (*Raw) stmt()    {}

func (*Select) setExpr()     {}
func (*SetOp) setExpr()      {}
func (*Values) setExpr()     {}
func (*TableQuery) setExpr() {}
func (*ParenQuery) setExpr() {}
