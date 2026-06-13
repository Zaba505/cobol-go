// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"fmt"
	"io"
	"iter"
	"slices"
	"strconv"
	"strings"
)

// File is the root of the COBOL abstract syntax tree. A COBOL source file is a
// sequence of one or more programs (GnuCOBOL §2.1.17), so File is a thin
// container with no position of its own; each [Program] and the nodes below it
// carry a [Pos].
type File struct {
	// Programs holds the programs of a COBOL source file in source order.
	Programs []*Program
}

// Program is a single COBOL program: an ordered list of divisions. Pos is the
// position of the program's first division header keyword. Nested holds the
// contained (nested) programs declared at the end of this program's PROCEDURE
// DIVISION, in source order; it is nil when there are none. End is the END PROGRAM
// marker terminating the program, nil when the program is not explicitly ended (a
// lone program may omit it).
type Program struct {
	Pos       Pos
	Divisions []Division
	Nested    []*Program
	End       *EndProgram
}

// EndProgram is the END PROGRAM marker that terminates a program (SPEC.md
// "source-file": "END" "PROGRAM" program-name "."). Pos is the position of the END
// keyword; Name is the program-name (a [Word] or [StringLiteral]), which by rule
// matches the program's PROGRAM-ID.
type EndProgram struct {
	Pos  Pos
	Name Type
}

// Division is implemented by the concrete COBOL division AST nodes
// (IDENTIFICATION, ENVIRONMENT, DATA, PROCEDURE).
type Division interface {
	division()
}

// Type is implemented by the concrete COBOL type/value AST nodes — literals and
// user-defined names that appear as operands.
type Type interface {
	cobol()
}

// Statement is implemented by the concrete COBOL procedure statement AST nodes.
type Statement interface {
	statement()
}

// IdentificationDivision is the IDENTIFICATION (or ID) DIVISION. Pos is the
// position of the IDENTIFICATION/ID keyword.
type IdentificationDivision struct {
	Pos       Pos
	ProgramID *ProgramID
}

func (*IdentificationDivision) division() {}

// ProgramID is the PROGRAM-ID paragraph naming the program. Pos is the position
// of the PROGRAM-ID keyword; Name is the program-name (a [Word] or
// [StringLiteral]).
type ProgramID struct {
	Pos  Pos
	Name Type
}

// ProcedureDivision is the PROCEDURE DIVISION. Pos is the position of the
// PROCEDURE keyword. Using holds the USING phrase parameters (program linkage) in
// source order; Returning is the optional RETURNING data-name, nil when absent.
// Declaratives holds the DECLARATIVES sections, nil when there is no DECLARATIVES
// block. Its body is either a sequence of paragraphs (Paragraphs, with Sections
// nil) or a sequence of sections (Sections, with Paragraphs nil) — the two forms
// are mutually exclusive. Statements that appear directly under the division
// header, before any paragraph-name, form an anonymous leading paragraph (a
// [Paragraph] with a nil Name).
type ProcedureDivision struct {
	Pos          Pos
	Using        []*Parameter
	Returning    *Word
	Declaratives []*DeclarativeSection
	Paragraphs   []*Paragraph
	Sections     []*Section
}

func (*ProcedureDivision) division() {}

// Parameter is one data-name of a PROCEDURE DIVISION USING phrase (SPEC.md
// "using-phrase"). Pos is the position of the parameter (the BY keyword when
// present, otherwise the data-name). Mode is the passing mechanism written
// immediately before this data-name — "REFERENCE" or "VALUE" in canonical upper
// case — or "" when no BY phrase precedes it, in which case the parameter inherits
// the mechanism set by an earlier BY phrase (COBOL defaults to BY REFERENCE at the
// start of the phrase). The printer emits a BY phrase only where Mode is set, so
// the source grouping is preserved; consumers that need the effective mechanism of
// an unmarked parameter must carry the most recent non-empty Mode forward. Name is
// the parameter data-name.
type Parameter struct {
	Pos  Pos
	Mode string
	Name *Word
}

// DeclarativeSection is one section of the PROCEDURE DIVISION DECLARATIVES (SPEC.md
// "declaratives"). Pos is the position of the section-name; Name is the
// section-name; Use is the section's USE statement (its mandatory first sentence);
// Paragraphs are the section's paragraphs in source order.
type DeclarativeSection struct {
	Pos        Pos
	Name       *Word
	Use        *UseStatement
	Paragraphs []*Paragraph
}

// UseStatement is the USE statement that opens a DECLARATIVES section (SPEC.md
// "declaratives": "USE" «use-spec» "."). Pos is the position of the USE keyword;
// Spec is the form-specific specification.
type UseStatement struct {
	Pos  Pos
	Spec UseSpec
}

// UseSpec is implemented by the concrete USE specification forms: [ExceptionUse],
// [DebuggingUse], and [ReportingUse].
type UseSpec interface {
	useSpec()
}

// ExceptionUse is the I-O error-handling USE form: "USE [GLOBAL] AFTER STANDARD
// (EXCEPTION | ERROR) PROCEDURE ON (file-name… | INPUT | OUTPUT | I-O | EXTEND)".
// Global reports the GLOBAL phrase; Error reports the ERROR spelling (vs the
// EXCEPTION synonym). Mode is the open-mode target ("INPUT", "OUTPUT", "I-O", or
// "EXTEND") and is "" when Files names one or more file-names instead.
type ExceptionUse struct {
	Pos    Pos
	Global bool
	Error  bool
	Mode   string
	Files  []*Word
}

func (*ExceptionUse) useSpec() {}

// DebuggingUse is the debugging USE form: "USE [FOR] DEBUGGING ON (procedure-name…
// | ALL PROCEDURES)". AllProcs reports the ALL PROCEDURES target; otherwise Targets
// names the procedure-names being debugged, in source order.
type DebuggingUse struct {
	Pos      Pos
	AllProcs bool
	Targets  []*Word
}

func (*DebuggingUse) useSpec() {}

// ReportingUse is the report-writer USE form: "USE [GLOBAL] BEFORE REPORTING
// report-group". Global reports the GLOBAL phrase; Report is the report-group
// data-name.
type ReportingUse struct {
	Pos    Pos
	Global bool
	Report *Word
}

func (*ReportingUse) useSpec() {}

// Section is a named section in the PROCEDURE DIVISION body. Pos is the position
// of the section-name; Segment is the optional priority number; Paragraphs are
// the section's paragraphs in source order.
type Section struct {
	Pos        Pos
	Name       *Word
	Segment    *NumericLiteral
	Paragraphs []*Paragraph
}

// Paragraph is a paragraph in the PROCEDURE DIVISION body. Name is nil for the
// anonymous leading paragraph (statements that appear before any paragraph-name);
// otherwise Pos is the position of the paragraph-name. Sentences are the
// paragraph's sentences in source order.
type Paragraph struct {
	Pos       Pos
	Name      *Word
	Sentences []*Sentence
}

// Sentence is one or more statements terminated by a separator period. Pos is the
// position of its first statement.
type Sentence struct {
	Pos        Pos
	Statements []Statement
}

// DisplayStatement is a DISPLAY statement. Pos is the position of the DISPLAY
// keyword; Operands are the values to display. Upon is the optional UPON
// mnemonic-name; NoAdvancing reports the WITH NO ADVANCING phrase.
type DisplayStatement struct {
	Pos         Pos
	Operands    []Type
	Upon        *Word
	NoAdvancing bool
}

func (*DisplayStatement) statement() {}

// MoveStatement is a MOVE statement. Pos is the position of the MOVE keyword;
// Corresponding reports the CORRESPONDING/CORR form; Source is the sending operand
// (an identifier or literal); Targets are the one or more receiving identifiers.
type MoveStatement struct {
	Pos           Pos
	Corresponding bool
	Source        Type
	Targets       []*Identifier
}

func (*MoveStatement) statement() {}

// AcceptStatement is an ACCEPT statement. Pos is the position of the ACCEPT
// keyword; Target is the receiving identifier; From is the optional FROM source
// (a mnemonic-name or a device such as DATE/TIME), nil when absent.
type AcceptStatement struct {
	Pos    Pos
	Target *Identifier
	From   *Word
}

func (*AcceptStatement) statement() {}

// CallStatement is a CALL statement (SPEC.md "call-statement"): an interprogram
// call. Pos is the position of the CALL keyword; Target is the called program — an
// AlphanumericLiteral (*StringLiteral) or an identifier (*Identifier). Using holds
// the USING arguments in source order; Returning is the optional RETURNING
// identifier, nil when absent; EndCall reports an explicit END-CALL scope
// terminator.
type CallStatement struct {
	Pos       Pos
	Target    Type // *StringLiteral or *Identifier
	Using     []*CallArgument
	Returning *Identifier
	EndCall   bool
}

func (*CallStatement) statement() {}

// CallArgument is one operand of a CALL … USING phrase. Pos is the position of the
// argument (the BY keyword when present, otherwise the operand). Mode is the
// passing mechanism in canonical upper case — "REFERENCE", "CONTENT", or "VALUE" —
// or "" when no BY phrase preceded this operand. Operand is the passed value, an
// identifier or a literal.
type CallArgument struct {
	Pos     Pos
	Mode    string
	Operand Type
}

// GoToStatement is a GO TO statement. Pos is the position of the GO keyword;
// Targets are the procedure-names; DependingOn is the GO TO … DEPENDING ON
// selector identifier, nil for the unconditional form.
type GoToStatement struct {
	Pos         Pos
	Targets     []*Word
	DependingOn *Identifier
}

func (*GoToStatement) statement() {}

// ContinueStatement is a CONTINUE statement (a no-op). Pos is the position of the
// CONTINUE keyword.
type ContinueStatement struct {
	Pos Pos
}

func (*ContinueStatement) statement() {}

// GobackStatement is a GOBACK statement (returns control to the caller, or ends
// the run unit for a main program). Pos is the position of the GOBACK keyword.
type GobackStatement struct {
	Pos Pos
}

func (*GobackStatement) statement() {}

// IfStatement is an IF statement. Pos is the position of the IF keyword; Cond is
// the condition; Then and Else are the branch statement lists; HasElse reports an
// ELSE clause (distinguishing an empty ELSE from no ELSE); EndIf reports an
// explicit END-IF scope terminator (a bare IF without it runs to the sentence
// period).
type IfStatement struct {
	Pos     Pos
	Cond    Condition
	Then    []Statement
	Else    []Statement
	HasElse bool
	EndIf   bool
}

func (*IfStatement) statement() {}

// NextSentenceStatement is a NEXT SENTENCE branch alternative within an IF
// statement (SPEC.md "if-statement"): instead of a statement list, a branch may
// transfer control to the statement following the next period. Pos is the
// position of the NEXT keyword.
type NextSentenceStatement struct {
	Pos Pos
}

func (*NextSentenceStatement) statement() {}

// PerformStatement is a PERFORM statement. Pos is the position of the PERFORM
// keyword. Out-of-line form: Target (and optional Through) name the procedure(s)
// to run; Inline is false; Body is nil. Inline form: Inline is true, Body holds
// the inline statements, and EndPerform reports the END-PERFORM terminator. The
// loop is one of Times (n TIMES), Until (UNTIL, with TestAfter for WITH TEST
// AFTER), or Varying (VARYING …); all nil for a plain PERFORM.
type PerformStatement struct {
	Pos        Pos
	Target     *Word
	Through    *Word
	Times      Type
	TestAfter  bool
	Until      Condition
	Varying    *PerformVarying
	Body       []Statement
	Inline     bool
	EndPerform bool
}

func (*PerformStatement) statement() {}

// PerformVarying is the VARYING phrase of a PERFORM loop. Pos is the position of
// the loop variable; From and By are its initial value and increment; Until is the
// termination condition.
type PerformVarying struct {
	Pos   Pos
	Name  *Identifier
	From  Type
	By    Type
	Until Condition
}

// ArithmeticStatement is an ADD, SUBTRACT, MULTIPLY, or DIVIDE statement. Pos is
// the position of the verb; Verb is the canonical verb keyword. Operands are the
// source operands; Connector is the keyword joining them to the targets ("TO",
// "FROM", "BY", or "INTO", empty for the GIVING-only form); Targets are the
// in-place receiving fields (each optionally ROUNDED); Giving are the GIVING
// result fields (each optionally ROUNDED); Remainder is the DIVIDE … REMAINDER
// result (nil otherwise); SizeError carries the optional [NOT] ON SIZE ERROR
// phrases; EndScope reports an explicit END-<verb> terminator.
type ArithmeticStatement struct {
	Pos       Pos
	Verb      string
	Operands  []Type
	Connector string
	Targets   []*ArithmeticTarget
	Giving    []*ArithmeticTarget
	Remainder *Identifier
	SizeError SizeErrorPhrases
	EndScope  bool
}

func (*ArithmeticStatement) statement() {}

// ArithmeticTarget is one receiving field of an arithmetic statement: an in-place
// TO/FROM/BY/INTO target or a GIVING result. Pos is the position of the
// identifier; Rounded reports a trailing ROUNDED phrase. It mirrors ComputeTarget.
type ArithmeticTarget struct {
	Pos     Pos
	Name    *Identifier
	Rounded bool
}

// SizeErrorPhrases holds the optional [ON] SIZE ERROR and NOT [ON] SIZE ERROR
// imperative phrases shared by the arithmetic statements and COMPUTE. OnSizeError
// and NotOnSizeError are the phrase bodies (nested statement lists); the Has flags
// distinguish a present-but-empty phrase from an absent one (a nil body).
type SizeErrorPhrases struct {
	OnSizeError       []Statement
	HasOnSizeError    bool
	NotOnSizeError    []Statement
	HasNotOnSizeError bool
}

// ComputeStatement is a COMPUTE statement. Pos is the position of the COMPUTE
// keyword; Targets are the receiving fields (each optionally ROUNDED); Expr is the
// assigned arithmetic expression; SizeError carries the optional [NOT] ON SIZE
// ERROR phrases; EndScope reports an explicit END-COMPUTE.
type ComputeStatement struct {
	Pos       Pos
	Targets   []ComputeTarget
	Expr      Expr
	SizeError SizeErrorPhrases
	EndScope  bool
}

func (*ComputeStatement) statement() {}

// ComputeTarget is one receiving field of a COMPUTE statement. Pos is the position
// of the identifier; Rounded reports a trailing ROUNDED phrase.
type ComputeTarget struct {
	Pos     Pos
	Name    *Identifier
	Rounded bool
}

// StopStatement is a STOP statement. Pos is the position of the STOP keyword.
// Run reports the STOP RUN form; otherwise Literal is the STOP <literal> operand.
type StopStatement struct {
	Pos     Pos
	Run     bool
	Literal Type
}

func (*StopStatement) statement() {}

// ExitStatement is an EXIT statement (SPEC.md "exit-statement"). Pos is the
// position of the EXIT keyword; Option is the optional object keyword
// ("PROGRAM", "PARAGRAPH", "SECTION", or "PERFORM"), empty for a bare EXIT.
type ExitStatement struct {
	Pos    Pos
	Option string
}

func (*ExitStatement) statement() {}

// EvaluateStatement is an EVALUATE statement (SPEC.md "evaluate-statement"): a
// multi-branch case construct. Pos is the position of the EVALUATE keyword;
// Subjects are the selection subjects joined by ALSO; Whens are the WHEN branches
// in source order; Other is the WHEN OTHER branch body and HasOther reports its
// presence (distinguishing an empty WHEN OTHER from none). The construct is always
// closed by END-EVALUATE.
type EvaluateStatement struct {
	Pos      Pos
	Subjects []*EvaluateSubject
	Whens    []*EvaluateWhen
	Other    []Statement
	HasOther bool
}

func (*EvaluateStatement) statement() {}

// EvaluateSubject is one selection subject of an EVALUATE (SPEC.md "subject"):
// exactly one of a boolean (Bool is "TRUE"/"FALSE"), a Cond, or an Operand. Pos is
// the position of its first token.
type EvaluateSubject struct {
	Pos     Pos
	Bool    string    // "TRUE" or "FALSE"; "" when not a boolean subject
	Cond    Condition // condition subject; nil otherwise
	Operand Type      // operand subject; nil otherwise
}

// EvaluateWhen is one WHEN branch of an EVALUATE: one or more match objects joined
// by ALSO and the statement list run when they match. Pos is the position of the
// WHEN keyword.
type EvaluateWhen struct {
	Pos     Pos
	Objects []*EvaluateObject
	Body    []Statement
}

// EvaluateObject is one match object of a WHEN branch (SPEC.md "object"): ANY, a
// boolean (Bool is "TRUE"/"FALSE"), a possibly NOT-negated Operand with an optional
// THROUGH/THRU range (Through), or a possibly NOT-negated Cond. Pos is the position
// of its first token.
type EvaluateObject struct {
	Pos     Pos
	Any     bool      // ANY
	Bool    string    // "TRUE" or "FALSE"; "" otherwise
	Not     bool      // leading NOT on the operand-range or condition form
	Operand Type      // operand form; the lower bound when Through is set
	Through Type      // upper bound of a THROUGH/THRU range; nil otherwise
	Cond    Condition // condition form
}

// Word is a COBOL word used as a value, e.g. a user-defined name. Pos is the
// position of the word; Value is its text.
type Word struct {
	Pos   Pos
	Value string
}

func (*Word) cobol() {}

// StringLiteral is an alphanumeric literal. Pos is the position of the opening
// delimiter; Value is the raw lexeme including both delimiters. Decoding the
// content is deferred to a later story.
type StringLiteral struct {
	Pos   Pos
	Value string
}

func (*StringLiteral) cobol() {}
func (*StringLiteral) expr()  {}

// NumericLiteral is a numeric literal used as a value, e.g. a VALUE or OCCURS
// operand. Pos is the position of the literal; Value is its raw lexeme (sign,
// digits, and decimal point preserved). Decoding the value is deferred to a
// later story, mirroring [StringLiteral].
type NumericLiteral struct {
	Pos   Pos
	Value string
}

func (*NumericLiteral) cobol() {}
func (*NumericLiteral) expr()  {}

// Expr is implemented by the arithmetic-expression AST nodes (SPEC.md
// "arithmetic-expression"): the operator nodes [BinaryExpr], [UnaryExpr], and
// [ParenExpr], plus the value-leaf nodes [Identifier], [NumericLiteral], and
// [StringLiteral].
type Expr interface {
	expr()
}

// Identifier is a procedure-division data reference: a data-name with optional
// IN/OF qualifiers, subscripts, and a reference-modifier (SPEC.md "identifier").
// Pos is the position of the data-name. An identifier is both a value [Type] and
// an expression [Expr].
type Identifier struct {
	Pos        Pos
	Name       *Word
	Qualifiers []*Word            // IN/OF chain, each a more inclusive containing name
	Subscripts []Expr             // subscript operands; nil when unsubscripted
	RefMod     *ReferenceModifier // optional reference modifier
}

func (*Identifier) cobol() {}
func (*Identifier) expr()  {}

// ReferenceModifier is the "(start:length)" suffix selecting a substring of an
// identifier. Pos is the position of the opening parenthesis; Length is nil for
// the open-ended "(start:)" form.
type ReferenceModifier struct {
	Pos    Pos
	Start  Expr
	Length Expr
}

// BinaryExpr is a binary arithmetic expression. Pos is the position of its left
// operand; Op is one of "+", "-", "*", "/", "**". All binary operators are
// left-associative (COBOL evaluates equal-precedence operators left to right).
type BinaryExpr struct {
	Pos   Pos
	Op    string
	Left  Expr
	Right Expr
}

func (*BinaryExpr) expr() {}

// UnaryExpr is a sign-prefixed arithmetic expression. Pos is the position of the
// sign; Op is "+" or "-".
type UnaryExpr struct {
	Pos     Pos
	Op      string
	Operand Expr
}

func (*UnaryExpr) expr() {}

// ParenExpr is a parenthesized arithmetic expression, preserved so the printer can
// reproduce the grouping. Pos is the position of the opening parenthesis.
type ParenExpr struct {
	Pos  Pos
	Expr Expr
}

func (*ParenExpr) expr() {}

// Condition is implemented by the conditional-expression AST nodes (SPEC.md
// "condition"): the relation/class/sign/condition-name simple conditions and the
// AND/OR/NOT/parenthesized combinators.
type Condition interface {
	condition()
}

// RelationCondition compares two expressions. Pos is the position of the left
// operand; Op is the canonical relational operator (">", "<", "=", ">=", "<=",
// "<>"). A negated relation (e.g. "NOT =") is represented by wrapping this in a
// [NotCondition].
type RelationCondition struct {
	Pos   Pos
	Left  Expr
	Op    string
	Right Expr
}

func (*RelationCondition) condition() {}

// ClassCondition tests an operand's class. Pos is the position of the operand; Not
// reports the "IS NOT" form; Class is the canonical class keyword ("NUMERIC",
// "ALPHABETIC", "ALPHABETIC-LOWER", "ALPHABETIC-UPPER").
type ClassCondition struct {
	Pos     Pos
	Operand Expr
	Not     bool
	Class   string
}

func (*ClassCondition) condition() {}

// SignCondition tests an operand's sign. Pos is the position of the operand; Not
// reports the "IS NOT" form; Sign is the canonical sign keyword ("POSITIVE",
// "NEGATIVE", "ZERO").
type SignCondition struct {
	Pos     Pos
	Operand Expr
	Not     bool
	Sign    string
}

func (*SignCondition) condition() {}

// ConditionNameCondition references a level-88 condition-name used as a bare
// condition. Pos is the position of the condition-name.
type ConditionNameCondition struct {
	Pos  Pos
	Name *Identifier
}

func (*ConditionNameCondition) condition() {}

// LogicalCondition combines two conditions with AND or OR. Pos is the position of
// the left condition; Op is "AND" or "OR".
type LogicalCondition struct {
	Pos   Pos
	Op    string
	Left  Condition
	Right Condition
}

func (*LogicalCondition) condition() {}

// NotCondition negates a condition. Pos is the position of the NOT keyword.
type NotCondition struct {
	Pos  Pos
	Cond Condition
}

func (*NotCondition) condition() {}

// ParenCondition is a parenthesized condition, preserved so the printer can
// reproduce the grouping. Pos is the position of the opening parenthesis.
type ParenCondition struct {
	Pos  Pos
	Cond Condition
}

func (*ParenCondition) condition() {}

// EnvironmentDivision is the ENVIRONMENT DIVISION. Pos is the position of the
// ENVIRONMENT keyword. Both sections are optional; a nil field means the section
// was absent in the source.
type EnvironmentDivision struct {
	Pos           Pos
	Configuration *ConfigurationSection
	InputOutput   *InputOutputSection
}

func (*EnvironmentDivision) division() {}

// ConfigurationSection is the CONFIGURATION SECTION of the ENVIRONMENT DIVISION.
// Pos is the position of the CONFIGURATION keyword. Each paragraph is optional;
// a nil field means the paragraph was absent.
type ConfigurationSection struct {
	Pos            Pos
	SourceComputer *SourceComputerParagraph
	ObjectComputer *ObjectComputerParagraph
	SpecialNames   *SpecialNamesParagraph
}

// SourceComputerParagraph is the SOURCE-COMPUTER paragraph. Pos is the position
// of the SOURCE-COMPUTER keyword; ComputerName is nil when the paragraph body is
// empty (just "SOURCE-COMPUTER."); DebuggingMode reports WITH DEBUGGING MODE.
type SourceComputerParagraph struct {
	Pos           Pos
	ComputerName  *Word
	DebuggingMode bool
}

// ObjectComputerParagraph is the OBJECT-COMPUTER paragraph. Pos is the position
// of the OBJECT-COMPUTER keyword; ComputerName is nil when the body is empty.
// The object-computer-clauses (MEMORY SIZE, PROGRAM COLLATING SEQUENCE, …) are
// deferred to a later story (SPEC "« object-computer-clause »").
type ObjectComputerParagraph struct {
	Pos          Pos
	ComputerName *Word
}

// SpecialNamesParagraph is the SPECIAL-NAMES paragraph. Pos is the position of
// the SPECIAL-NAMES keyword; Clauses are its clauses in source order.
type SpecialNamesParagraph struct {
	Pos     Pos
	Clauses []SpecialNamesClause
}

// SpecialNamesClause is implemented by the concrete SPECIAL-NAMES clause AST
// nodes. The device-name/mnemonic-name and ALPHABET associations are deferred to
// a later story (SPEC "« other implementor associations »").
type SpecialNamesClause interface {
	specialNamesClause()
}

// DecimalPointClause is the DECIMAL-POINT IS COMMA clause. Pos is the position
// of the DECIMAL-POINT keyword. Honoring its lexing effect (swapping the roles
// of '.' and ',' in numeric literals and PICTURE strings) is a semantic concern
// deferred to a later story; here the clause is only recorded in the AST.
type DecimalPointClause struct {
	Pos Pos
}

func (*DecimalPointClause) specialNamesClause() {}

// CurrencySignClause is the CURRENCY SIGN IS literal clause. Pos is the position
// of the CURRENCY keyword; Sign is the alphanumeric literal naming the currency
// symbol.
type CurrencySignClause struct {
	Pos  Pos
	Sign *StringLiteral
}

func (*CurrencySignClause) specialNamesClause() {}

// InputOutputSection is the INPUT-OUTPUT SECTION of the ENVIRONMENT DIVISION.
// Pos is the position of the INPUT-OUTPUT keyword; FileControl is nil when the
// FILE-CONTROL paragraph is absent. The I-O-CONTROL paragraph is deferred to a
// later story (SPEC "« i-o-control-clause »").
type InputOutputSection struct {
	Pos         Pos
	FileControl *FileControlParagraph
}

// FileControlParagraph is the FILE-CONTROL paragraph. Pos is the position of the
// FILE-CONTROL keyword; Entries are its file-control entries in source order.
type FileControlParagraph struct {
	Pos     Pos
	Entries []*FileControlEntry
}

// FileControlEntry is a single SELECT ... ASSIGN file-control entry. Pos is the
// position of the SELECT keyword; Optional reports SELECT OPTIONAL; Name is the
// file-name; Assign is the assignment target (a user-defined [Word] or an
// alphanumeric [StringLiteral]); Clauses are its select-clauses in source order.
type FileControlEntry struct {
	Pos      Pos
	Optional bool
	Name     *Word
	Assign   Type
	Clauses  []SelectClause
}

// SelectClause is implemented by the concrete file-control select-clause AST
// nodes (ORGANIZATION, ACCESS, RECORD KEY, FILE STATUS).
type SelectClause interface {
	selectClause()
}

// OrganizationClause is the ORGANIZATION clause. Pos is the position of the
// ORGANIZATION keyword; Organization is the canonical upper-case organization
// name ("SEQUENTIAL", "LINE SEQUENTIAL", "RELATIVE", or "INDEXED").
type OrganizationClause struct {
	Pos          Pos
	Organization string
}

func (*OrganizationClause) selectClause() {}

// AccessClause is the ACCESS MODE clause. Pos is the position of the ACCESS
// keyword; Mode is the canonical upper-case access mode ("SEQUENTIAL", "RANDOM",
// or "DYNAMIC").
type AccessClause struct {
	Pos  Pos
	Mode string
}

func (*AccessClause) selectClause() {}

// RecordKeyClause is the RECORD KEY clause. Pos is the position of the RECORD
// keyword; Name is the data-name naming the record key.
type RecordKeyClause struct {
	Pos  Pos
	Name *Word
}

func (*RecordKeyClause) selectClause() {}

// FileStatusClause is the FILE STATUS clause. Pos is the position of the FILE
// keyword; Name is the data-name receiving the file status.
type FileStatusClause struct {
	Pos  Pos
	Name *Word
}

func (*FileStatusClause) selectClause() {}

// DataDivision is the DATA DIVISION. Pos is the position of the DATA keyword.
// Every section is optional; a nil field means the section was absent in the
// source. FILE differs structurally from the others (FD/SD entries), so it has
// its own type; WORKING-STORAGE, LOCAL-STORAGE, and LINKAGE share the identical
// [DataSection] body, the owning field identifying which section it is.
type DataDivision struct {
	Pos            Pos
	File           *FileSection
	WorkingStorage *DataSection
	LocalStorage   *DataSection
	Linkage        *DataSection
}

func (*DataDivision) division() {}

// DataSection is the body of a WORKING-STORAGE, LOCAL-STORAGE, or LINKAGE
// SECTION: a flat list of data description entries in source order. Pos is the
// position of the section header keyword. The record hierarchy is implied by the
// entries' level numbers (SPEC: entries "keyed by level number") rather than
// nested in the AST.
type DataSection struct {
	Pos     Pos
	Entries []*DataDescriptionEntry
}

// FileSection is the FILE SECTION of the DATA DIVISION. Pos is the position of
// the FILE keyword; Entries are its file-description entries in source order.
type FileSection struct {
	Pos     Pos
	Entries []*FileDescriptionEntry
}

// FileDescriptionEntry is a single FD/SD file-description entry. Pos is the
// position of the FD/SD keyword; Kind is "FD" or "SD"; Name is the file-name;
// Records are the subordinate record data-description entries. The file-clauses
// (BLOCK CONTAINS, RECORD CONTAINS, LABEL RECORDS, …) are deferred to a later
// story (SPEC "« file-clause »").
type FileDescriptionEntry struct {
	Pos     Pos
	Kind    string
	Name    *Word
	Records []*DataDescriptionEntry
}

// DataDescriptionEntry is one level-numbered data description entry. Pos is the
// position of the level-number; Level is the integer level (1–49, 66, 77, or
// 88); Name is the data-name or condition-name (nil when FILLER or omitted);
// Filler reports the FILLER keyword; Clauses are its clauses in source order. A
// group item has subordinate entries (higher level numbers following) and no
// PICTURE; an elementary item has a PICTURE — both share this one node type.
type DataDescriptionEntry struct {
	Pos     Pos
	Level   int
	Name    *Word
	Filler  bool
	Clauses []DataClause
}

// DataClause is implemented by the concrete data-description clause AST nodes.
type DataClause interface {
	dataClause()
}

// RedefinesClause is the REDEFINES clause. Pos is the position of the REDEFINES
// keyword; Name is the data-name being redefined.
type RedefinesClause struct {
	Pos  Pos
	Name *Word
}

func (*RedefinesClause) dataClause() {}

// PictureClause is the PICTURE/PIC clause. Pos is the position of the
// PICTURE/PIC keyword; Picture is the raw PICTURE character-string lexeme (case
// preserved; category validation is deferred — SPEC §Semantics).
type PictureClause struct {
	Pos     Pos
	Picture string
}

func (*PictureClause) dataClause() {}

// UsageClause is the USAGE clause (with or without the USAGE keyword). Pos is the
// position of the clause; Usage is the canonical upper-case usage-type
// ("DISPLAY", "BINARY", "PACKED-DECIMAL", "COMP", "COMP-1"…"COMP-5", "INDEX",
// "POINTER").
type UsageClause struct {
	Pos   Pos
	Usage string
}

func (*UsageClause) dataClause() {}

// ValueClause is the VALUE/VALUES clause. Pos is the position of the VALUE
// keyword; Values holds one [ValueSpec] for an ordinary item and one or more
// (possibly THROUGH ranges) for a level-88 condition-name.
type ValueClause struct {
	Pos    Pos
	Values []ValueSpec
}

func (*ValueClause) dataClause() {}

// ValueSpec is one value or value-range in a VALUE clause. From is the literal
// (or figurative constant); Through is the upper bound of a THROUGH/THRU range,
// nil for a single value.
type ValueSpec struct {
	From    Type
	Through Type
}

// OccursClause is the OCCURS clause. Pos is the position of the OCCURS keyword;
// Min is the occurrence count (the lower bound when Max is set); Max is the upper
// bound of a TO range, nil for a fixed count; DependingOn is the DEPENDING ON
// data-name, nil when absent; Keys are the ASCENDING/DESCENDING KEY phrases;
// IndexedBy is the INDEXED BY index-name, nil when absent.
type OccursClause struct {
	Pos         Pos
	Min         *NumericLiteral
	Max         *NumericLiteral
	DependingOn *Word
	Keys        []OccursKey
	IndexedBy   *Word
}

func (*OccursClause) dataClause() {}

// OccursKey is one ASCENDING/DESCENDING KEY phrase of an OCCURS clause.
// Ascending reports ASCENDING (vs DESCENDING); Name is the key data-name.
type OccursKey struct {
	Pos       Pos
	Ascending bool
	Name      *Word
}

// SignClause is the SIGN clause. Pos is the position of the SIGN keyword;
// Position is "LEADING" or "TRAILING"; Separate reports SEPARATE [CHARACTER].
type SignClause struct {
	Pos      Pos
	Position string
	Separate bool
}

func (*SignClause) dataClause() {}

// JustifiedClause is the JUSTIFIED/JUST [RIGHT] clause. Pos is the position of
// the JUSTIFIED keyword. RIGHT is the only justification COBOL allows, so the
// clause carries no further data.
type JustifiedClause struct {
	Pos Pos
}

func (*JustifiedClause) dataClause() {}

// SynchronizedClause is the SYNCHRONIZED/SYNC [LEFT|RIGHT] clause. Pos is the
// position of the SYNCHRONIZED keyword; Direction is "", "LEFT", or "RIGHT".
type SynchronizedClause struct {
	Pos       Pos
	Direction string
}

func (*SynchronizedClause) dataClause() {}

// BlankWhenZeroClause is the BLANK [WHEN] ZERO clause. Pos is the position of the
// BLANK keyword.
type BlankWhenZeroClause struct {
	Pos Pos
}

func (*BlankWhenZeroClause) dataClause() {}

// GlobalClause is the GLOBAL clause. Pos is the position of the GLOBAL keyword.
type GlobalClause struct {
	Pos Pos
}

func (*GlobalClause) dataClause() {}

// ExternalClause is the EXTERNAL clause. Pos is the position of the EXTERNAL
// keyword.
type ExternalClause struct {
	Pos Pos
}

func (*ExternalClause) dataClause() {}

// RenamesClause is the RENAMES clause of a level-66 entry. Pos is the position of
// the RENAMES keyword; From is the first renamed data-name; Through is the
// THROUGH/THRU upper bound, nil for a single name.
type RenamesClause struct {
	Pos     Pos
	From    *Word
	Through *Word
}

func (*RenamesClause) dataClause() {}

// Parse the COBOL source from the given reader into a [File].
//
// It pulls tokens from [Tokenize] with [iter.Pull2] and runs the top-level
// action loop against a *File.
func Parse(r io.Reader) (*File, error) {
	next, stop := iter.Pull2(Tokenize(r))
	defer stop()

	p := &parser{next: next}
	f := &File{}

	var err error
	for action := parseFile; action != nil && err == nil; {
		action, err = action(p, f)
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

type parser struct {
	// next pulls the next (Token, error) pair from the tokenizer; the final
	// bool reports whether a value was produced (false once exhausted).
	next func() (Token, error, bool)
	// peeked and peeked2 hold up to two tokens read by peek/peekSecond but not yet
	// consumed, giving the parser two-token lookahead. One token suffices almost
	// everywhere (the go/parser current-token model this package mirrors); the
	// second is needed only to distinguish a section header (name SECTION) from a
	// paragraph header (name .). advance drains them before pulling from next.
	peeked  *peekedToken
	peeked2 *peekedToken
}

// peekedToken is a single (Token, error, ok) triple buffered by peek.
type peekedToken struct {
	tok Token
	err error
	ok  bool
}

// advance returns the next (Token, error, ok) triple, draining the peek buffers
// first so a peeked token is consumed exactly once.
func (p *parser) advance() (Token, error, bool) {
	if p.peeked != nil {
		pk := p.peeked
		p.peeked = p.peeked2
		p.peeked2 = nil
		return pk.tok, pk.err, pk.ok
	}
	return p.next()
}

// peekSecond returns the token after the next without consuming either, buffering
// both. It is the parser's only two-token lookahead, used to tell a section header
// (name SECTION) from a paragraph header (name .).
func (p *parser) peekSecond() (Token, error, bool) {
	if p.peeked == nil {
		tok, err, ok := p.next()
		p.peeked = &peekedToken{tok: tok, err: err, ok: ok}
	}
	if p.peeked2 == nil {
		tok, err, ok := p.next()
		p.peeked2 = &peekedToken{tok: tok, err: err, ok: ok}
	}
	return p.peeked2.tok, p.peeked2.err, p.peeked2.ok
}

// atSectionHeader reports whether the parser is positioned at a section header: a
// non-verb name (an identifier or an all-digit word) followed by the SECTION
// keyword.
func (p *parser) atSectionHeader() (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok || !isProcedureName(tok) {
		return false, nil
	}
	second, err, ok2 := p.peekSecond()
	if err != nil {
		return false, err
	}
	return ok2 && keywordIs(second, "SECTION"), nil
}

// atProgramBoundary reports whether the parser is positioned at a token that ends
// the PROCEDURE DIVISION body and belongs to the enclosing program layer: the END
// of an END PROGRAM marker, or the IDENTIFICATION/ID header of a nested program.
// END is its own token, distinct from scope terminators such as END-IF.
func (p *parser) atProgramBoundary() (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return keywordIs(tok, "END", "IDENTIFICATION", "ID"), nil
}

// atEndDeclaratives reports whether the parser is positioned at the END
// DECLARATIVES marker: the END keyword followed by DECLARATIVES.
func (p *parser) atEndDeclaratives() (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok || !keywordIs(tok, "END") {
		return false, nil
	}
	second, err, ok2 := p.peekSecond()
	if err != nil {
		return false, err
	}
	return ok2 && keywordIs(second, "DECLARATIVES"), nil
}

// atParagraphHeader reports whether the parser is positioned at a paragraph header:
// a non-verb name followed by a separator period.
func (p *parser) atParagraphHeader() (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok || !isProcedureName(tok) {
		return false, nil
	}
	second, err, ok2 := p.peekSecond()
	if err != nil {
		return false, err
	}
	return ok2 && isPeriod(second), nil
}

// isProcedureName reports whether tok can name a paragraph or section: an
// identifier that is not a reserved verb, or an all-digit word (a numeric
// paragraph/section name).
func isProcedureName(tok Token) bool {
	switch tok.Type {
	case TokenNumber:
		return true
	case TokenIdentifier:
		return !isStatementVerb(tok)
	default:
		return false
	}
}

// consume discards the next token. It is used only after peek has already
// classified that token (so its error is known to be nil), keeping the
// dispatch-then-consume call sites free of a redundant error check.
func (p *parser) consume() {
	_, _, _ = p.advance()
}

// peek returns the next (Token, error, ok) triple without consuming it; repeated
// calls return the same token until advance consumes it.
func (p *parser) peek() (Token, error, bool) {
	if p.peeked == nil {
		tok, err, ok := p.next()
		p.peeked = &peekedToken{tok: tok, err: err, ok: ok}
	}
	return p.peeked.tok, p.peeked.err, p.peeked.ok
}

// peekKeyword reports whether the next (unconsumed) token is an identifier
// matching one of kw case-insensitively. End of input reports false; a tokenizer
// error is propagated so optional-construct detection fails cleanly.
func (p *parser) peekKeyword(kw ...string) (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return keywordIs(tok, kw...), nil
}

// skipOptionalKeyword consumes the next token iff it is an identifier matching
// one of kw, used for the optional noise words IS / TO / MODE / KEY.
func (p *parser) skipOptionalKeyword(kw ...string) error {
	is, err := p.peekKeyword(kw...)
	if err != nil {
		return err
	}
	if is {
		p.consume()
	}
	return nil
}

// acceptKeyword consumes and returns the next token iff it is an identifier
// matching one of kw, reporting whether it was consumed. It is the consuming
// counterpart of peekKeyword for optional keywords whose token (its position) is
// needed.
func (p *parser) acceptKeyword(kw ...string) (Token, bool, error) {
	is, err := p.peekKeyword(kw...)
	if err != nil {
		return Token{}, false, err
	}
	if !is {
		return Token{}, false, nil
	}
	tok, _, _ := p.advance()
	return tok, true, nil
}

// acceptKeywordValue is like acceptKeyword but returns the matched keyword in its
// canonical upper-case spelling (COBOL words are case-insensitive), for storing a
// normalized keyword in the AST.
func (p *parser) acceptKeywordValue(kw ...string) (string, bool, error) {
	tok, ok, err := p.acceptKeyword(kw...)
	if err != nil || !ok {
		return "", false, err
	}
	return strings.ToUpper(string(tok.Value)), true, nil
}

// expectFollow verifies that the next (unconsumed) token, if any, is an
// identifier matching one of follow, allowing end of input. It does not consume
// the token, so a valid follower remains for the caller (typically the division
// boundary). It lets a construct report an unimplemented or misplaced keyword at
// its own level rather than letting the token surface later as a misleading
// division-dispatch error.
func (p *parser) expectFollow(follow ...string) error {
	tok, err, ok := p.peek()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if keywordIs(tok, follow...) {
		return nil
	}
	return unexpectedKeyword(tok, follow...)
}

// expect pulls the next token and requires its type to be one of types,
// returning [UnexpectedEndOfTokensError] if the stream is exhausted or
// [UnexpectedTokenError] if the type does not match.
func (p *parser) expect(types ...TokenType) (Token, error) {
	tok, err, ok := p.advance()
	if err != nil {
		return Token{}, err
	}
	if !ok {
		return Token{}, UnexpectedEndOfTokensError{Expected: types}
	}
	if !slices.Contains(types, tok.Type) {
		return Token{}, UnexpectedTokenError{Expected: types, Actual: tok}
	}
	return tok, nil
}

// expectKeyword pulls the next token, requires it to be an identifier, and
// requires its value to match one of kw case-insensitively (COBOL reserved words
// are case-insensitive). It returns [UnexpectedKeywordError] when an identifier
// of the wrong spelling is read.
func (p *parser) expectKeyword(kw ...string) (Token, error) {
	tok, err := p.expect(TokenIdentifier)
	if err != nil {
		return Token{}, err
	}
	if !keywordIs(tok, kw...) {
		return Token{}, UnexpectedKeywordError{Expected: kw, Actual: tok}
	}
	return tok, nil
}

// expectPeriod consumes the separator period that ends a sentence or header,
// returning [UnexpectedTokenError] when the next token is not a period symbol.
func (p *parser) expectPeriod() (Token, error) {
	tok, err := p.expect(TokenSymbol)
	if err != nil {
		return Token{}, err
	}
	if !isPeriod(tok) {
		return Token{}, UnexpectedTokenError{Expected: []TokenType{TokenSymbol}, Actual: tok}
	}
	return tok, nil
}

// isPeriod reports whether tok is the separator-period symbol.
func isPeriod(tok Token) bool {
	return tok.Type == TokenSymbol && string(tok.Value) == "."
}

// peekSymbol reports whether the next (unconsumed) token is a symbol whose value
// matches one of vals (e.g. an operator or a parenthesis). End of input and a
// non-symbol token report false; a tokenizer error is propagated.
func (p *parser) peekSymbol(vals ...string) (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok || tok.Type != TokenSymbol {
		return false, nil
	}
	return slices.Contains(vals, string(tok.Value)), nil
}

// expectSymbol consumes the next token, requiring it to be a symbol whose value
// matches one of vals. Unlike [parser.expect], it checks the symbol's value, not
// just its token type.
func (p *parser) expectSymbol(vals ...string) (Token, error) {
	tok, err := p.expect(TokenSymbol)
	if err != nil {
		return Token{}, err
	}
	if !slices.Contains(vals, string(tok.Value)) {
		return Token{}, UnexpectedTokenError{Expected: []TokenType{TokenSymbol}, Actual: tok}
	}
	return tok, nil
}

// keywordIs reports whether tok is an identifier whose value matches one of kws
// case-insensitively.
func keywordIs(tok Token, kws ...string) bool {
	if tok.Type != TokenIdentifier {
		return false
	}
	for _, k := range kws {
		if strings.EqualFold(string(tok.Value), k) {
			return true
		}
	}
	return false
}

// unexpectedKeyword builds the error for tok read where one of the keywords in
// expected was required. An identifier of the wrong spelling yields
// [UnexpectedKeywordError]; any other token type is a structural mismatch (an
// identifier was expected) and yields [UnexpectedTokenError], so callers using
// errors.As can tell a misspelled keyword from a wrong token class.
func unexpectedKeyword(tok Token, expected ...string) error {
	if tok.Type != TokenIdentifier {
		return UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: tok}
	}
	return UnexpectedKeywordError{Expected: expected, Actual: tok}
}

// valueNode wraps an identifier, alphanumeric-literal, or numeric-literal token
// as the corresponding value AST node. An identifier becomes a [Word] (a
// user-defined name or a figurative constant such as ZERO/SPACES, whose identity
// is its spelling).
func valueNode(tok Token) Type {
	switch tok.Type {
	case TokenString:
		return &StringLiteral{Pos: tok.Pos, Value: string(tok.Value)}
	case TokenNumber:
		return &NumericLiteral{Pos: tok.Pos, Value: string(tok.Value)}
	default:
		return &Word{Pos: tok.Pos, Value: string(tok.Value)}
	}
}

// parserAction is one step of the parser state machine, generic over the AST
// node T currently being built. Returning (nil, nil) completes successfully;
// returning (nil, err) terminates with an error.
type parserAction[T any] func(p *parser, t T) (parserAction[T], error)

// parseFile is the top-level program loop. Each call reads the next program's
// first division header keyword, builds the [Program], appends it to f.Programs,
// and returns parseFile to parse the next program. An exhausted stream completes
// the loop, so empty input parses to a zero-value *File.
func parseFile(p *parser, f *File) (parserAction[*File], error) {
	tok, err, ok := p.advance()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	prog, err := parseProgram(p, tok)
	if err != nil {
		return nil, err
	}

	f.Programs = append(f.Programs, prog)
	return parseFile, nil
}

// parseProgram builds one program whose first division header keyword firstKw has
// already been read, by driving the division action chain. The program ends at its
// END PROGRAM marker (recorded on prog.End by parseEndProgram) or at end of input.
// It is shared by the top-level loop (parseFile) and nested-program parsing
// (parseNestedProgram), so nesting recurses through it to any depth.
func parseProgram(p *parser, firstKw Token) (*Program, error) {
	prog := &Program{Pos: firstKw.Pos}
	var err error
	for action := dispatchDivision(firstKw); action != nil && err == nil; {
		action, err = action(p, prog)
	}
	if err != nil {
		return nil, err
	}
	return prog, nil
}

// dispatchDivision returns the division parser for the already-read division
// header keyword kw. IDENTIFICATION/ID, ENVIRONMENT, DATA, and PROCEDURE are
// dispatched to their parsers; END dispatches the END PROGRAM marker that
// terminates the program; any other keyword falls through to the default and is
// reported as an unexpected division header.
func dispatchDivision(kw Token) parserAction[*Program] {
	switch {
	case keywordIs(kw, "IDENTIFICATION", "ID"):
		return parseIdentificationDivision(kw)
	case keywordIs(kw, "ENVIRONMENT"):
		return parseEnvironmentDivision(kw)
	case keywordIs(kw, "DATA"):
		return parseDataDivision(kw)
	case keywordIs(kw, "PROCEDURE"):
		return parseProcedureDivision(kw)
	case keywordIs(kw, "END"):
		return parseEndProgram(kw)
	default:
		return func(_ *parser, _ *Program) (parserAction[*Program], error) {
			return nil, unexpectedKeyword(kw, "IDENTIFICATION", "ID", "ENVIRONMENT", "DATA", "PROCEDURE")
		}
	}
}

// parseEndProgram parses the END PROGRAM marker whose END keyword kw has already
// been read: "PROGRAM" program-name ".". It records the marker on prog.End and
// returns (nil, nil) to complete the program.
func parseEndProgram(kw Token) parserAction[*Program] {
	return func(p *parser, prog *Program) (parserAction[*Program], error) {
		if _, err := p.expectKeyword("PROGRAM"); err != nil {
			return nil, err
		}
		name, err := p.expect(TokenIdentifier, TokenString)
		if err != nil {
			return nil, err
		}
		if _, err := p.expectPeriod(); err != nil {
			return nil, err
		}
		prog.End = &EndProgram{Pos: kw.Pos, Name: valueNode(name)}
		return nil, nil
	}
}

// parseNestedProgram parses one nested (contained) program whose first division
// header keyword firstKw has already been read, appends it to the parent's Nested,
// then continues with the next nested sibling (another IDENTIFICATION/ID header) or
// dispatches the parent's END PROGRAM marker. Each nested program is itself ended
// by its own END PROGRAM, so parseProgram consumes it before control returns here.
func parseNestedProgram(firstKw Token) parserAction[*Program] {
	return func(p *parser, parent *Program) (parserAction[*Program], error) {
		child, err := parseProgram(p, firstKw)
		if err != nil {
			return nil, err
		}
		parent.Nested = append(parent.Nested, child)

		tok, err, ok := p.advance()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		if keywordIs(tok, "IDENTIFICATION", "ID") {
			return parseNestedProgram(tok), nil
		}
		return dispatchDivision(tok), nil
	}
}

// parseIdentificationDivision parses the IDENTIFICATION DIVISION whose header
// keyword kw has already been read, then reads the boundary token to dispatch
// the next division or ends the program at end of input. Identification comment
// paragraphs (AUTHOR, …) are deferred to a later story.
func parseIdentificationDivision(kw Token) parserAction[*Program] {
	return func(p *parser, prog *Program) (parserAction[*Program], error) {
		div := &IdentificationDivision{Pos: kw.Pos}

		var err error
		for action := parseIdentificationHeader; action != nil && err == nil; {
			action, err = action(p, div)
		}
		if err != nil {
			return nil, err
		}
		prog.Divisions = append(prog.Divisions, div)

		tok, err, ok := p.advance()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return dispatchDivision(tok), nil
	}
}

// parseIdentificationHeader consumes "DIVISION" "." after the IDENTIFICATION/ID
// keyword.
func parseIdentificationHeader(p *parser, _ *IdentificationDivision) (parserAction[*IdentificationDivision], error) {
	if _, err := p.expectKeyword("DIVISION"); err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}
	return parseProgramIDParagraph, nil
}

// parseProgramIDParagraph consumes "PROGRAM-ID" "." program-name "." and records
// the program name. The optional INITIAL/RECURSIVE/COMMON clause is deferred to
// a later story.
func parseProgramIDParagraph(p *parser, div *IdentificationDivision) (parserAction[*IdentificationDivision], error) {
	kw, err := p.expectKeyword("PROGRAM-ID")
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}

	name, err := p.expect(TokenIdentifier, TokenString)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}

	div.ProgramID = &ProgramID{Pos: kw.Pos, Name: valueNode(name)}
	return nil, nil
}

// parseEnvironmentDivision parses the ENVIRONMENT DIVISION whose header keyword
// kw has already been read. Both the CONFIGURATION and INPUT-OUTPUT sections are
// optional; the inner action loop ends without consuming the token that begins
// the next division, which the post-loop advance then reads to dispatch (or ends
// the program at end of input).
func parseEnvironmentDivision(kw Token) parserAction[*Program] {
	return func(p *parser, prog *Program) (parserAction[*Program], error) {
		div := &EnvironmentDivision{Pos: kw.Pos}

		var err error
		for action := parseEnvironmentHeader; action != nil && err == nil; {
			action, err = action(p, div)
		}
		if err != nil {
			return nil, err
		}
		prog.Divisions = append(prog.Divisions, div)

		tok, err, ok := p.advance()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return dispatchDivision(tok), nil
	}
}

// parseEnvironmentHeader consumes "DIVISION" "." after the ENVIRONMENT keyword.
func parseEnvironmentHeader(p *parser, _ *EnvironmentDivision) (parserAction[*EnvironmentDivision], error) {
	if _, err := p.expectKeyword("DIVISION"); err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}
	return parseConfigurationSectionOpt, nil
}

// isEnvironmentHeaderKeyword reports whether tok is one of the section,
// paragraph, or division header keywords that can follow an optional paragraph
// body in or after the ENVIRONMENT DIVISION. A computer-name (a user-defined
// word) is never a reserved word, so this distinguishes "the optional body is
// present" from "the next header begins."
func isEnvironmentHeaderKeyword(tok Token) bool {
	return keywordIs(tok,
		"CONFIGURATION", "SOURCE-COMPUTER", "OBJECT-COMPUTER", "SPECIAL-NAMES",
		"INPUT-OUTPUT", "FILE-CONTROL", "I-O-CONTROL",
		"DATA", "PROCEDURE",
	)
}

// peekParagraphBody reports whether the next (unconsumed) token begins an
// optional paragraph body — an identifier that is not an environment header
// keyword (i.e. a computer-name) rather than the start of the next header.
func (p *parser) peekParagraphBody() (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok || tok.Type != TokenIdentifier {
		return false, nil
	}
	return !isEnvironmentHeaderKeyword(tok), nil
}

// parseConfigurationSectionOpt parses the optional CONFIGURATION SECTION. When
// the next token is not the CONFIGURATION keyword the section is absent and the
// token is left for parseInputOutputSectionOpt.
func parseConfigurationSectionOpt(p *parser, div *EnvironmentDivision) (parserAction[*EnvironmentDivision], error) {
	is, err := p.peekKeyword("CONFIGURATION")
	if err != nil {
		return nil, err
	}
	if !is {
		return parseInputOutputSectionOpt, nil
	}
	hdr, _, _ := p.advance() // CONFIGURATION
	if _, err := p.expectKeyword("SECTION"); err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}

	sec := &ConfigurationSection{Pos: hdr.Pos}
	for action := parseSourceComputerOpt; action != nil && err == nil; {
		action, err = action(p, sec)
	}
	if err != nil {
		return nil, err
	}
	div.Configuration = sec
	return parseInputOutputSectionOpt, nil
}

// parseSourceComputerOpt parses the optional SOURCE-COMPUTER paragraph:
// "SOURCE-COMPUTER" "." [ computer-name [ "WITH" "DEBUGGING" "MODE" ] "." ].
func parseSourceComputerOpt(p *parser, sec *ConfigurationSection) (parserAction[*ConfigurationSection], error) {
	is, err := p.peekKeyword("SOURCE-COMPUTER")
	if err != nil {
		return nil, err
	}
	if !is {
		return parseObjectComputerOpt, nil
	}
	hdr, _, _ := p.advance() // SOURCE-COMPUTER
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}
	para := &SourceComputerParagraph{Pos: hdr.Pos}

	hasBody, err := p.peekParagraphBody()
	if err != nil {
		return nil, err
	}
	if hasBody {
		name, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		para.ComputerName = &Word{Pos: name.Pos, Value: string(name.Value)}

		with, err := p.peekKeyword("WITH")
		if err != nil {
			return nil, err
		}
		if with {
			p.consume() // WITH
			if _, err := p.expectKeyword("DEBUGGING"); err != nil {
				return nil, err
			}
			if _, err := p.expectKeyword("MODE"); err != nil {
				return nil, err
			}
			para.DebuggingMode = true
		}
		if _, err := p.expect(TokenSymbol); err != nil {
			return nil, err
		}
	}

	sec.SourceComputer = para
	return parseObjectComputerOpt, nil
}

// parseObjectComputerOpt parses the optional OBJECT-COMPUTER paragraph:
// "OBJECT-COMPUTER" "." [ computer-name "." ]. The object-computer-clauses are
// deferred to a later story.
func parseObjectComputerOpt(p *parser, sec *ConfigurationSection) (parserAction[*ConfigurationSection], error) {
	is, err := p.peekKeyword("OBJECT-COMPUTER")
	if err != nil {
		return nil, err
	}
	if !is {
		return parseSpecialNamesOpt, nil
	}
	hdr, _, _ := p.advance() // OBJECT-COMPUTER
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}
	para := &ObjectComputerParagraph{Pos: hdr.Pos}

	hasBody, err := p.peekParagraphBody()
	if err != nil {
		return nil, err
	}
	if hasBody {
		name, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		para.ComputerName = &Word{Pos: name.Pos, Value: string(name.Value)}
		if _, err := p.expect(TokenSymbol); err != nil {
			return nil, err
		}
	}

	sec.ObjectComputer = para
	return parseSpecialNamesOpt, nil
}

// parseSpecialNamesOpt parses the optional SPECIAL-NAMES paragraph header and
// runs the clause loop over its body.
func parseSpecialNamesOpt(p *parser, sec *ConfigurationSection) (parserAction[*ConfigurationSection], error) {
	is, err := p.peekKeyword("SPECIAL-NAMES")
	if err != nil {
		return nil, err
	}
	if !is {
		return nil, nil
	}
	hdr, _, _ := p.advance() // SPECIAL-NAMES
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}

	para := &SpecialNamesParagraph{Pos: hdr.Pos}
	for action := parseSpecialNamesClause; action != nil && err == nil; {
		action, err = action(p, para)
	}
	if err != nil {
		return nil, err
	}
	sec.SpecialNames = para
	return nil, nil
}

// parseSpecialNamesClause parses one SPECIAL-NAMES clause and loops; the clause
// list ends at the optional paragraph-terminating period (consumed) or at the
// next header keyword (left for the caller). Only DECIMAL-POINT IS COMMA and
// CURRENCY SIGN IS literal are recognized; device-name and ALPHABET associations
// are deferred to a later story.
func parseSpecialNamesClause(p *parser, para *SpecialNamesParagraph) (parserAction[*SpecialNamesParagraph], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	switch {
	case keywordIs(tok, "DECIMAL-POINT"):
		p.consume()
		if err := p.skipOptionalKeyword("IS"); err != nil {
			return nil, err
		}
		if _, err := p.expectKeyword("COMMA"); err != nil {
			return nil, err
		}
		para.Clauses = append(para.Clauses, &DecimalPointClause{Pos: tok.Pos})
		return parseSpecialNamesClause, nil
	case keywordIs(tok, "CURRENCY"):
		p.consume()
		if _, err := p.expectKeyword("SIGN"); err != nil {
			return nil, err
		}
		if err := p.skipOptionalKeyword("IS"); err != nil {
			return nil, err
		}
		lit, err := p.expect(TokenString)
		if err != nil {
			return nil, err
		}
		para.Clauses = append(para.Clauses, &CurrencySignClause{
			Pos:  tok.Pos,
			Sign: &StringLiteral{Pos: lit.Pos, Value: string(lit.Value)},
		})
		return parseSpecialNamesClause, nil
	case tok.Type == TokenSymbol:
		p.consume() // the optional paragraph-terminating period
		return nil, nil
	case isEnvironmentHeaderKeyword(tok):
		// The next section/division header ends the clause list (the paragraph
		// period is optional); leave the token for the caller.
		return nil, nil
	default:
		// An identifier that is neither a recognized clause nor a header — an
		// unimplemented or misspelled clause (e.g. ALPHABET …). Report it at the
		// clause position rather than silently truncating the paragraph and
		// failing later with a misleading division-dispatch error.
		return nil, unexpectedKeyword(tok, "DECIMAL-POINT", "CURRENCY")
	}
}

// parseInputOutputSectionOpt parses the optional INPUT-OUTPUT SECTION and its
// optional FILE-CONTROL paragraph. The I-O-CONTROL paragraph is deferred.
func parseInputOutputSectionOpt(p *parser, div *EnvironmentDivision) (parserAction[*EnvironmentDivision], error) {
	is, err := p.peekKeyword("INPUT-OUTPUT")
	if err != nil {
		return nil, err
	}
	if !is {
		return nil, nil
	}
	hdr, _, _ := p.advance() // INPUT-OUTPUT
	if _, err := p.expectKeyword("SECTION"); err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}
	sec := &InputOutputSection{Pos: hdr.Pos}

	isFileControl, err := p.peekKeyword("FILE-CONTROL")
	if err != nil {
		return nil, err
	}
	if isFileControl {
		fcHdr, _, _ := p.advance() // FILE-CONTROL
		if _, err := p.expect(TokenSymbol); err != nil {
			return nil, err
		}
		para := &FileControlParagraph{Pos: fcHdr.Pos}
		for action := parseFileControlEntryOpt; action != nil && err == nil; {
			action, err = action(p, para)
		}
		if err != nil {
			return nil, err
		}
		sec.FileControl = para
	}

	div.InputOutput = sec

	// The INPUT-OUTPUT SECTION ends the ENVIRONMENT DIVISION's content, so what
	// follows must be the next division (DATA or PROCEDURE) or end of input —
	// plus FILE-CONTROL when it has not yet appeared. Validate it here so a
	// deferred I-O-CONTROL paragraph, or a SELECT entry outside a FILE-CONTROL
	// paragraph, is reported at this level instead of surfacing later as a
	// misleading division-dispatch error.
	follow := []string{"DATA", "PROCEDURE"}
	if sec.FileControl == nil {
		follow = append([]string{"FILE-CONTROL"}, follow...)
	}
	if err := p.expectFollow(follow...); err != nil {
		return nil, err
	}
	return nil, nil
}

// parseFileControlEntryOpt parses one file-control entry and loops; the entry
// list ends when the next token is not the SELECT keyword.
func parseFileControlEntryOpt(p *parser, para *FileControlParagraph) (parserAction[*FileControlParagraph], error) {
	is, err := p.peekKeyword("SELECT")
	if err != nil {
		return nil, err
	}
	if !is {
		return nil, nil
	}
	sel, _, _ := p.advance() // SELECT
	entry := &FileControlEntry{Pos: sel.Pos}

	optional, err := p.peekKeyword("OPTIONAL")
	if err != nil {
		return nil, err
	}
	if optional {
		p.consume()
		entry.Optional = true
	}

	name, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	entry.Name = &Word{Pos: name.Pos, Value: string(name.Value)}

	if _, err := p.expectKeyword("ASSIGN"); err != nil {
		return nil, err
	}
	if err := p.skipOptionalKeyword("TO"); err != nil {
		return nil, err
	}
	assign, err := p.expect(TokenIdentifier, TokenString)
	if err != nil {
		return nil, err
	}
	entry.Assign = valueNode(assign)

	for action := parseSelectClause; action != nil && err == nil; {
		action, err = action(p, entry)
	}
	if err != nil {
		return nil, err
	}

	para.Entries = append(para.Entries, entry)
	return parseFileControlEntryOpt, nil
}

// parseSelectClause dispatches one select-clause and loops; the clause list ends
// at the entry-terminating separator period (consumed here).
func parseSelectClause(p *parser, entry *FileControlEntry) (parserAction[*FileControlEntry], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, UnexpectedEndOfTokensError{Expected: []TokenType{TokenSymbol}}
	}
	switch {
	case keywordIs(tok, "ORGANIZATION"):
		return parseOrganizationClause, nil
	case keywordIs(tok, "ACCESS"):
		return parseAccessClause, nil
	case keywordIs(tok, "RECORD"):
		return parseRecordKeyClause, nil
	case keywordIs(tok, "FILE"):
		return parseFileStatusClause, nil
	case tok.Type == TokenSymbol:
		p.consume() // the entry-terminating "."
		return nil, nil
	default:
		return nil, unexpectedKeyword(tok, "ORGANIZATION", "ACCESS", "RECORD", "FILE")
	}
}

// parseOrganizationClause parses ORGANIZATION [IS] ( SEQUENTIAL | LINE SEQUENTIAL
// | RELATIVE | INDEXED ), normalizing the organization to a canonical upper-case
// name.
func parseOrganizationClause(p *parser, entry *FileControlEntry) (parserAction[*FileControlEntry], error) {
	hdr, _, _ := p.advance() // ORGANIZATION
	if err := p.skipOptionalKeyword("IS"); err != nil {
		return nil, err
	}
	val, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	org := strings.ToUpper(string(val.Value))
	switch org {
	case "SEQUENTIAL", "RELATIVE", "INDEXED":
	case "LINE":
		if _, err := p.expectKeyword("SEQUENTIAL"); err != nil {
			return nil, err
		}
		org = "LINE SEQUENTIAL"
	default:
		return nil, unexpectedKeyword(val, "SEQUENTIAL", "LINE", "RELATIVE", "INDEXED")
	}
	entry.Clauses = append(entry.Clauses, &OrganizationClause{Pos: hdr.Pos, Organization: org})
	return parseSelectClause, nil
}

// parseAccessClause parses ACCESS [MODE] [IS] ( SEQUENTIAL | RANDOM | DYNAMIC ),
// normalizing the mode to a canonical upper-case name.
func parseAccessClause(p *parser, entry *FileControlEntry) (parserAction[*FileControlEntry], error) {
	hdr, _, _ := p.advance() // ACCESS
	if err := p.skipOptionalKeyword("MODE"); err != nil {
		return nil, err
	}
	if err := p.skipOptionalKeyword("IS"); err != nil {
		return nil, err
	}
	val, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	mode := strings.ToUpper(string(val.Value))
	switch mode {
	case "SEQUENTIAL", "RANDOM", "DYNAMIC":
	default:
		return nil, unexpectedKeyword(val, "SEQUENTIAL", "RANDOM", "DYNAMIC")
	}
	entry.Clauses = append(entry.Clauses, &AccessClause{Pos: hdr.Pos, Mode: mode})
	return parseSelectClause, nil
}

// parseRecordKeyClause parses RECORD [KEY] [IS] data-name.
func parseRecordKeyClause(p *parser, entry *FileControlEntry) (parserAction[*FileControlEntry], error) {
	hdr, _, _ := p.advance() // RECORD
	if err := p.skipOptionalKeyword("KEY"); err != nil {
		return nil, err
	}
	if err := p.skipOptionalKeyword("IS"); err != nil {
		return nil, err
	}
	name, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	entry.Clauses = append(entry.Clauses, &RecordKeyClause{
		Pos:  hdr.Pos,
		Name: &Word{Pos: name.Pos, Value: string(name.Value)},
	})
	return parseSelectClause, nil
}

// parseFileStatusClause parses FILE STATUS [IS] data-name.
func parseFileStatusClause(p *parser, entry *FileControlEntry) (parserAction[*FileControlEntry], error) {
	hdr, _, _ := p.advance() // FILE
	if _, err := p.expectKeyword("STATUS"); err != nil {
		return nil, err
	}
	if err := p.skipOptionalKeyword("IS"); err != nil {
		return nil, err
	}
	name, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	entry.Clauses = append(entry.Clauses, &FileStatusClause{
		Pos:  hdr.Pos,
		Name: &Word{Pos: name.Pos, Value: string(name.Value)},
	})
	return parseSelectClause, nil
}

// parseDataDivision parses the DATA DIVISION whose header keyword kw has already
// been read. Every section is optional; the inner action loop ends without
// consuming the token that begins the next division, which the post-loop advance
// then reads to dispatch (or ends the program at end of input). It mirrors
// parseEnvironmentDivision.
func parseDataDivision(kw Token) parserAction[*Program] {
	return func(p *parser, prog *Program) (parserAction[*Program], error) {
		div := &DataDivision{Pos: kw.Pos}

		var err error
		for action := parseDataHeader; action != nil && err == nil; {
			action, err = action(p, div)
		}
		if err != nil {
			return nil, err
		}
		prog.Divisions = append(prog.Divisions, div)

		tok, err, ok := p.advance()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return dispatchDivision(tok), nil
	}
}

// parseDataHeader consumes "DIVISION" "." after the DATA keyword.
func parseDataHeader(p *parser, _ *DataDivision) (parserAction[*DataDivision], error) {
	if _, err := p.expectKeyword("DIVISION"); err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}
	return parseFileSectionOpt, nil
}

// parseFileSectionOpt parses the optional FILE SECTION. When the next token is
// not the FILE keyword the section is absent and the token is left for
// parseWorkingStorageOpt.
func parseFileSectionOpt(p *parser, div *DataDivision) (parserAction[*DataDivision], error) {
	is, err := p.peekKeyword("FILE")
	if err != nil {
		return nil, err
	}
	if !is {
		return parseWorkingStorageOpt, nil
	}
	hdr, _, _ := p.advance() // FILE
	if _, err := p.expectKeyword("SECTION"); err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}

	sec := &FileSection{Pos: hdr.Pos}
	for action := parseFileDescriptionEntryOpt; action != nil && err == nil; {
		action, err = action(p, sec)
	}
	if err != nil {
		return nil, err
	}
	div.File = sec
	return parseWorkingStorageOpt, nil
}

// parseFileDescriptionEntryOpt parses one FD/SD file-description entry and loops;
// the entry list ends when the next token is not the FD/SD keyword (a following
// section/division header or end of input). The file-clauses are deferred (SPEC
// "« file-clause »"): a non-period token after the file-name surfaces as an
// UnexpectedTokenError rather than being silently consumed.
func parseFileDescriptionEntryOpt(p *parser, sec *FileSection) (parserAction[*FileSection], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok || !keywordIs(tok, "FD", "SD") {
		return nil, nil
	}
	p.consume() // FD/SD
	entry := &FileDescriptionEntry{Pos: tok.Pos, Kind: strings.ToUpper(string(tok.Value))}

	name, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	entry.Name = &Word{Pos: name.Pos, Value: string(name.Value)}

	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}

	records, err := parseDataEntries(p)
	if err != nil {
		return nil, err
	}
	entry.Records = records

	sec.Entries = append(sec.Entries, entry)
	return parseFileDescriptionEntryOpt, nil
}

// parseWorkingStorageOpt parses the optional WORKING-STORAGE SECTION.
func parseWorkingStorageOpt(p *parser, div *DataDivision) (parserAction[*DataDivision], error) {
	return parseDataSectionOpt(p, "WORKING-STORAGE", func(sec *DataSection) { div.WorkingStorage = sec }, parseLocalStorageOpt)
}

// parseLocalStorageOpt parses the optional LOCAL-STORAGE SECTION.
func parseLocalStorageOpt(p *parser, div *DataDivision) (parserAction[*DataDivision], error) {
	return parseDataSectionOpt(p, "LOCAL-STORAGE", func(sec *DataSection) { div.LocalStorage = sec }, parseLinkageOpt)
}

// parseLinkageOpt parses the optional LINKAGE SECTION. It is the last section, so
// it ends the inner loop (returning nil); the post-loop advance in
// parseDataDivision then reads the next division header or ends the program.
func parseLinkageOpt(p *parser, div *DataDivision) (parserAction[*DataDivision], error) {
	return parseDataSectionOpt(p, "LINKAGE", func(sec *DataSection) { div.Linkage = sec }, nil)
}

// parseDataSectionOpt parses an optional WORKING-STORAGE/LOCAL-STORAGE/LINKAGE
// section: "<header>" "SECTION" "." { data-description-entry }. When the next
// token is not the header keyword the section is absent and the token is left for
// the next section. assign stores the parsed section on the division; next is the
// action for the following section (nil after LINKAGE).
func parseDataSectionOpt(p *parser, header string, assign func(*DataSection), next parserAction[*DataDivision]) (parserAction[*DataDivision], error) {
	is, err := p.peekKeyword(header)
	if err != nil {
		return nil, err
	}
	if !is {
		return next, nil
	}
	hdr, _, _ := p.advance() // header keyword
	if _, err := p.expectKeyword("SECTION"); err != nil {
		return nil, err
	}
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}

	entries, err := parseDataEntries(p)
	if err != nil {
		return nil, err
	}
	assign(&DataSection{Pos: hdr.Pos, Entries: entries})
	return next, nil
}

// parseDataEntries runs the data-description-entry loop, returning the entries in
// source order. It is shared by the three data sections and by FD record
// descriptions. The loop ends at the first token that does not begin an entry (a
// non-level-number: a following section/division header, or end of input), which
// is left unconsumed for the caller.
func parseDataEntries(p *parser) ([]*DataDescriptionEntry, error) {
	var entries []*DataDescriptionEntry
	var err error
	for action := parseDataEntryOpt; action != nil && err == nil; {
		action, err = action(p, &entries)
	}
	return entries, err
}

// parseDataEntryOpt parses one data-description entry and loops. An entry begins
// with a level-number ([TokenNumber]); any other token (or end of input) ends the
// list and is left for the caller. The level-number is validated to be 1–49, 66,
// 77, or 88.
func parseDataEntryOpt(p *parser, entries *[]*DataDescriptionEntry) (parserAction[*[]*DataDescriptionEntry], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok || tok.Type != TokenNumber {
		return nil, nil
	}
	p.consume() // level-number

	level, err := strconv.Atoi(string(tok.Value))
	if err != nil || !validLevel(level) {
		return nil, InvalidLevelNumberError{Pos: tok.Pos, Value: string(tok.Value)}
	}
	entry := &DataDescriptionEntry{Pos: tok.Pos, Level: level}

	var aerr error
	for action := parseEntryName; action != nil && aerr == nil; {
		action, aerr = action(p, entry)
	}
	if aerr != nil {
		return nil, aerr
	}

	*entries = append(*entries, entry)
	return parseDataEntryOpt, nil
}

// validLevel reports whether level is a valid data-description level-number:
// 01–49 (record hierarchy), 66 (RENAMES), 77 (standalone elementary), or 88
// (condition-name).
func validLevel(level int) bool {
	return (level >= 1 && level <= 49) || level == 66 || level == 77 || level == 88
}

// parseEntryName reads the entry's optional name (or FILLER) and dispatches to
// the clause parser for the entry's level. Level 88 (condition-name) and level 66
// (RENAMES) require a name and have a fixed clause; other levels take an optional
// name/FILLER followed by the general data-clause loop.
func parseEntryName(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	switch entry.Level {
	case 88:
		name, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		entry.Name = &Word{Pos: name.Pos, Value: string(name.Value)}
		return parseConditionValueClause, nil
	case 66:
		name, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		entry.Name = &Word{Pos: name.Pos, Value: string(name.Value)}
		return parseRenamesClause, nil
	default:
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, UnexpectedEndOfTokensError{Expected: []TokenType{TokenIdentifier, TokenSymbol}}
		}
		switch {
		case keywordIs(tok, "FILLER"):
			p.consume()
			entry.Filler = true
		case tok.Type == TokenIdentifier && !isDataClauseKeyword(tok):
			p.consume()
			entry.Name = &Word{Pos: tok.Pos, Value: string(tok.Value)}
		}
		return parseDataClause, nil
	}
}

// isDataClauseKeyword reports whether tok is a keyword that introduces a
// data-clause (or is a bare usage-type). It distinguishes an entry-name from the
// start of the clause list when the name is omitted.
func isDataClauseKeyword(tok Token) bool {
	return keywordIs(tok,
		"REDEFINES", "PICTURE", "PIC", "USAGE", "VALUE", "VALUES", "OCCURS",
		"SIGN", "JUSTIFIED", "JUST", "SYNCHRONIZED", "SYNC", "BLANK",
		"GLOBAL", "EXTERNAL", "RENAMES",
	) || isUsageType(tok)
}

// isUsageType reports whether tok is a bare usage-type keyword (a USAGE clause
// written without the USAGE keyword).
func isUsageType(tok Token) bool {
	return keywordIs(tok,
		"DISPLAY", "BINARY", "PACKED-DECIMAL",
		"COMP", "COMP-1", "COMP-2", "COMP-3", "COMP-4", "COMP-5",
		"INDEX", "POINTER",
	)
}

// parseDataClause dispatches one data-clause and loops; the clause list ends at
// the entry-terminating separator period (consumed here). It mirrors
// parseSelectClause.
func parseDataClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, UnexpectedEndOfTokensError{Expected: []TokenType{TokenSymbol}}
	}
	switch {
	case keywordIs(tok, "REDEFINES"):
		return parseRedefinesClause, nil
	case keywordIs(tok, "PICTURE", "PIC"):
		return parsePictureClause, nil
	case keywordIs(tok, "USAGE") || isUsageType(tok):
		return parseUsageClause, nil
	case keywordIs(tok, "VALUE", "VALUES"):
		return parseValueClause, nil
	case keywordIs(tok, "OCCURS"):
		return parseOccursClause, nil
	case keywordIs(tok, "SIGN"):
		return parseSignClause, nil
	case keywordIs(tok, "JUSTIFIED", "JUST"):
		return parseJustifiedClause, nil
	case keywordIs(tok, "SYNCHRONIZED", "SYNC"):
		return parseSynchronizedClause, nil
	case keywordIs(tok, "BLANK"):
		return parseBlankWhenZeroClause, nil
	case keywordIs(tok, "GLOBAL"):
		return parseGlobalClause, nil
	case keywordIs(tok, "EXTERNAL"):
		return parseExternalClause, nil
	case tok.Type == TokenSymbol:
		p.consume() // the entry-terminating "."
		return nil, nil
	default:
		return nil, unexpectedKeyword(tok,
			"REDEFINES", "PICTURE", "PIC", "USAGE", "VALUE", "OCCURS",
			"SIGN", "JUSTIFIED", "SYNCHRONIZED", "BLANK", "GLOBAL", "EXTERNAL",
		)
	}
}

// parseRedefinesClause parses REDEFINES data-name.
func parseRedefinesClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // REDEFINES
	name, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	entry.Clauses = append(entry.Clauses, &RedefinesClause{
		Pos:  hdr.Pos,
		Name: &Word{Pos: name.Pos, Value: string(name.Value)},
	})
	return parseDataClause, nil
}

// parsePictureClause parses ( "PICTURE" | "PIC" ) [ "IS" ] PictureString. The
// tokenizer emits the optional IS as its own identifier and the picture run as a
// single [TokenPicture] (SPEC §"PICTURE Character-Strings").
func parsePictureClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // PICTURE/PIC
	if err := p.skipOptionalKeyword("IS"); err != nil {
		return nil, err
	}
	pic, err := p.expect(TokenPicture)
	if err != nil {
		return nil, err
	}
	entry.Clauses = append(entry.Clauses, &PictureClause{Pos: hdr.Pos, Picture: string(pic.Value)})
	return parseDataClause, nil
}

// parseUsageClause parses [ "USAGE" [ "IS" ] ] usage-type, normalizing the
// usage-type to a canonical upper-case name.
func parseUsageClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // USAGE keyword or the bare usage-type
	pos := hdr.Pos
	usageTok := hdr
	if keywordIs(hdr, "USAGE") {
		if err := p.skipOptionalKeyword("IS"); err != nil {
			return nil, err
		}
		tok, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		usageTok = tok
	}
	if !isUsageType(usageTok) {
		return nil, unexpectedKeyword(usageTok,
			"DISPLAY", "BINARY", "PACKED-DECIMAL",
			"COMP", "COMP-1", "COMP-2", "COMP-3", "COMP-4", "COMP-5", "INDEX", "POINTER",
		)
	}
	entry.Clauses = append(entry.Clauses, &UsageClause{Pos: pos, Usage: strings.ToUpper(string(usageTok.Value))})
	return parseDataClause, nil
}

// parseValueClause parses ( "VALUE" | "VALUES" ) [ "IS" ] literal for an ordinary
// item: a single value with no range. Level-88 value lists are parsed by
// parseConditionValueClause.
func parseValueClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // VALUE/VALUES
	if err := p.skipOptionalKeyword("IS"); err != nil {
		return nil, err
	}
	lit, err := p.expect(TokenString, TokenNumber, TokenIdentifier)
	if err != nil {
		return nil, err
	}
	entry.Clauses = append(entry.Clauses, &ValueClause{Pos: hdr.Pos, Values: []ValueSpec{{From: valueNode(lit)}}})
	return parseDataClause, nil
}

// parseOccursClause parses
// "OCCURS" NumericLiteral [ "TO" NumericLiteral ] [ "TIMES" ]
// [ "DEPENDING" [ "ON" ] data-name ]
// { ( "ASCENDING" | "DESCENDING" ) [ "KEY" ] [ "IS" ] data-name }
// [ "INDEXED" [ "BY" ] index-name ].
func parseOccursClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // OCCURS
	clause := &OccursClause{Pos: hdr.Pos}

	minTok, err := p.expect(TokenNumber)
	if err != nil {
		return nil, err
	}
	clause.Min = &NumericLiteral{Pos: minTok.Pos, Value: string(minTok.Value)}

	isTo, err := p.peekKeyword("TO")
	if err != nil {
		return nil, err
	}
	if isTo {
		p.consume() // TO
		maxTok, err := p.expect(TokenNumber)
		if err != nil {
			return nil, err
		}
		clause.Max = &NumericLiteral{Pos: maxTok.Pos, Value: string(maxTok.Value)}
	}

	if err := p.skipOptionalKeyword("TIMES"); err != nil {
		return nil, err
	}

	isDepending, err := p.peekKeyword("DEPENDING")
	if err != nil {
		return nil, err
	}
	if isDepending {
		p.consume() // DEPENDING
		if err := p.skipOptionalKeyword("ON"); err != nil {
			return nil, err
		}
		name, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		clause.DependingOn = &Word{Pos: name.Pos, Value: string(name.Value)}
	}

	for {
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok || !keywordIs(tok, "ASCENDING", "DESCENDING") {
			break
		}
		p.consume() // ASCENDING/DESCENDING
		if err := p.skipOptionalKeyword("KEY"); err != nil {
			return nil, err
		}
		if err := p.skipOptionalKeyword("IS"); err != nil {
			return nil, err
		}
		name, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		clause.Keys = append(clause.Keys, OccursKey{
			Pos:       tok.Pos,
			Ascending: keywordIs(tok, "ASCENDING"),
			Name:      &Word{Pos: name.Pos, Value: string(name.Value)},
		})
	}

	isIndexed, err := p.peekKeyword("INDEXED")
	if err != nil {
		return nil, err
	}
	if isIndexed {
		p.consume() // INDEXED
		if err := p.skipOptionalKeyword("BY"); err != nil {
			return nil, err
		}
		name, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		clause.IndexedBy = &Word{Pos: name.Pos, Value: string(name.Value)}
	}

	entry.Clauses = append(entry.Clauses, clause)
	return parseDataClause, nil
}

// parseSignClause parses "SIGN" [ "IS" ] ( "LEADING" | "TRAILING" )
// [ "SEPARATE" [ "CHARACTER" ] ].
func parseSignClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // SIGN
	if err := p.skipOptionalKeyword("IS"); err != nil {
		return nil, err
	}
	pos, err := p.expectKeyword("LEADING", "TRAILING")
	if err != nil {
		return nil, err
	}
	clause := &SignClause{Pos: hdr.Pos, Position: strings.ToUpper(string(pos.Value))}

	isSeparate, err := p.peekKeyword("SEPARATE")
	if err != nil {
		return nil, err
	}
	if isSeparate {
		p.consume() // SEPARATE
		clause.Separate = true
		if err := p.skipOptionalKeyword("CHARACTER"); err != nil {
			return nil, err
		}
	}

	entry.Clauses = append(entry.Clauses, clause)
	return parseDataClause, nil
}

// parseJustifiedClause parses ( "JUSTIFIED" | "JUST" ) [ "RIGHT" ].
func parseJustifiedClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // JUSTIFIED/JUST
	if err := p.skipOptionalKeyword("RIGHT"); err != nil {
		return nil, err
	}
	entry.Clauses = append(entry.Clauses, &JustifiedClause{Pos: hdr.Pos})
	return parseDataClause, nil
}

// parseSynchronizedClause parses ( "SYNCHRONIZED" | "SYNC" ) [ "LEFT" | "RIGHT" ].
func parseSynchronizedClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // SYNCHRONIZED/SYNC
	clause := &SynchronizedClause{Pos: hdr.Pos}

	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if ok && keywordIs(tok, "LEFT", "RIGHT") {
		p.consume()
		clause.Direction = strings.ToUpper(string(tok.Value))
	}

	entry.Clauses = append(entry.Clauses, clause)
	return parseDataClause, nil
}

// parseBlankWhenZeroClause parses "BLANK" [ "WHEN" ] "ZERO".
func parseBlankWhenZeroClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // BLANK
	if err := p.skipOptionalKeyword("WHEN"); err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("ZERO"); err != nil {
		return nil, err
	}
	entry.Clauses = append(entry.Clauses, &BlankWhenZeroClause{Pos: hdr.Pos})
	return parseDataClause, nil
}

// parseGlobalClause parses "GLOBAL".
func parseGlobalClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // GLOBAL
	entry.Clauses = append(entry.Clauses, &GlobalClause{Pos: hdr.Pos})
	return parseDataClause, nil
}

// parseExternalClause parses "EXTERNAL".
func parseExternalClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, _, _ := p.advance() // EXTERNAL
	entry.Clauses = append(entry.Clauses, &ExternalClause{Pos: hdr.Pos})
	return parseDataClause, nil
}

// parseConditionValueClause parses the VALUE clause of a level-88 condition-name:
// ( "VALUE" | "VALUES" ) [ "IS" ] value-spec { value-spec } "." where
// value-spec = literal [ ( "THROUGH" | "THRU" ) literal ]. It is the only clause
// of a condition-name entry, so it consumes the entry-terminating period and ends
// the entry (returns nil).
func parseConditionValueClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, err := p.expectKeyword("VALUE", "VALUES")
	if err != nil {
		return nil, err
	}
	if err := p.skipOptionalKeyword("IS"); err != nil {
		return nil, err
	}

	clause := &ValueClause{Pos: hdr.Pos}
	for {
		lit, err := p.expect(TokenString, TokenNumber, TokenIdentifier)
		if err != nil {
			return nil, err
		}
		spec := ValueSpec{From: valueNode(lit)}

		isThrough, err := p.peekKeyword("THROUGH", "THRU")
		if err != nil {
			return nil, err
		}
		if isThrough {
			p.consume() // THROUGH/THRU
			hi, err := p.expect(TokenString, TokenNumber, TokenIdentifier)
			if err != nil {
				return nil, err
			}
			spec.Through = valueNode(hi)
		}
		clause.Values = append(clause.Values, spec)

		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, UnexpectedEndOfTokensError{Expected: []TokenType{TokenSymbol}}
		}
		if tok.Type == TokenSymbol {
			p.consume() // entry-terminating "."
			break
		}
	}

	entry.Clauses = append(entry.Clauses, clause)
	return nil, nil
}

// parseRenamesClause parses the RENAMES clause of a level-66 entry:
// "RENAMES" data-name [ ( "THROUGH" | "THRU" ) data-name ] "." It is the only
// clause of a RENAMES entry, so it consumes the entry-terminating period and ends
// the entry (returns nil).
func parseRenamesClause(p *parser, entry *DataDescriptionEntry) (parserAction[*DataDescriptionEntry], error) {
	hdr, err := p.expectKeyword("RENAMES")
	if err != nil {
		return nil, err
	}
	from, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	clause := &RenamesClause{Pos: hdr.Pos, From: &Word{Pos: from.Pos, Value: string(from.Value)}}

	isThrough, err := p.peekKeyword("THROUGH", "THRU")
	if err != nil {
		return nil, err
	}
	if isThrough {
		p.consume() // THROUGH/THRU
		to, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		clause.Through = &Word{Pos: to.Pos, Value: string(to.Value)}
	}

	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}

	entry.Clauses = append(entry.Clauses, clause)
	return nil, nil
}

// parseProcedureDivision parses the PROCEDURE DIVISION whose header keyword kw has
// already been read. After the body, the procedure division is no longer terminal:
// it reads the boundary token and either parses the program's nested programs (an
// IDENTIFICATION/ID header), dispatches the program's END PROGRAM marker (END), or
// ends at end of input — mirroring how the other divisions hand off to the next.
func parseProcedureDivision(kw Token) parserAction[*Program] {
	return func(p *parser, prog *Program) (parserAction[*Program], error) {
		div := &ProcedureDivision{Pos: kw.Pos}

		var err error
		for action := parseProcedureHeader; action != nil && err == nil; {
			action, err = action(p, div)
		}
		if err != nil {
			return nil, err
		}
		prog.Divisions = append(prog.Divisions, div)

		tok, err, ok := p.advance()
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		if keywordIs(tok, "IDENTIFICATION", "ID") {
			return parseNestedProgram(tok), nil
		}
		return dispatchDivision(tok), nil
	}
}

// parseProcedureHeader consumes "DIVISION" after the PROCEDURE keyword, then the
// optional USING and RETURNING phrases (SPEC.md "using-phrase", "returning-phrase")
// and the terminating period. It hands off to parseDeclarativesOpt so an optional
// DECLARATIVES block is parsed before the body.
func parseProcedureHeader(p *parser, div *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
	if _, err := p.expectKeyword("DIVISION"); err != nil {
		return nil, err
	}

	if using, err := p.peekKeyword("USING"); err != nil {
		return nil, err
	} else if using {
		p.consume()
		params, err := parseProcedureUsing(p)
		if err != nil {
			return nil, err
		}
		div.Using = params
	}

	if returning, err := p.peekKeyword("RETURNING"); err != nil {
		return nil, err
	} else if returning {
		p.consume()
		name, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		div.Returning = &Word{Pos: name.Pos, Value: string(name.Value)}
	}

	if _, err := p.expectPeriod(); err != nil {
		return nil, err
	}
	return parseDeclarativesOpt, nil
}

// parseProcedureUsing parses the data-names of a PROCEDURE DIVISION USING phrase:
// one or more parameters, each a data-name optionally preceded by a BY
// REFERENCE/VALUE mode. It continues while the next token is BY or a data-name and
// stops at the RETURNING keyword or the header period.
func parseProcedureUsing(p *parser) ([]*Parameter, error) {
	var params []*Parameter
	for {
		byTok, by, err := p.acceptKeyword("BY")
		if err != nil {
			return nil, err
		}
		var mode string
		if by {
			m, err := p.expectKeyword("REFERENCE", "VALUE")
			if err != nil {
				return nil, err
			}
			mode = strings.ToUpper(string(m.Value))
		}

		// A data-name is required here. RETURNING is a valid identifier token, so
		// reject it explicitly; otherwise it would be swallowed as a parameter name
		// and the RETURNING phrase silently dropped.
		if lead, err, ok := p.peek(); err != nil {
			return nil, err
		} else if ok && keywordIs(lead, "RETURNING") {
			return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: lead}
		}
		name, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		pos := name.Pos
		if by {
			pos = byTok.Pos
		}
		params = append(params, &Parameter{
			Pos:  pos,
			Mode: mode,
			Name: &Word{Pos: name.Pos, Value: string(name.Value)},
		})

		// Another parameter follows when the next token is a BY phrase or a
		// data-name; RETURNING and the header period end the phrase.
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		if keywordIs(tok, "BY") {
			continue
		}
		if tok.Type == TokenIdentifier && !keywordIs(tok, "RETURNING") {
			continue
		}
		break
	}
	return params, nil
}

// parseDeclarativesOpt parses the optional DECLARATIVES block (SPEC.md
// "declaratives") that may precede the procedure body. With no DECLARATIVES keyword
// it hands straight off to the body; otherwise it consumes "DECLARATIVES" "." and
// loops over the declarative sections.
func parseDeclarativesOpt(p *parser, div *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
	decl, err := p.peekKeyword("DECLARATIVES")
	if err != nil {
		return nil, err
	}
	if !decl {
		return parseProcedureBody, nil
	}
	p.consume()
	if _, err := p.expectPeriod(); err != nil {
		return nil, err
	}
	return parseDeclarativeSectionOpt, nil
}

// parseDeclarativeSectionOpt parses one declarative section and appends it, then
// returns itself; at the END DECLARATIVES marker it consumes "END" "DECLARATIVES"
// "." and hands off to the procedure body.
func parseDeclarativeSectionOpt(p *parser, div *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
	end, err := p.atEndDeclaratives()
	if err != nil {
		return nil, err
	}
	if end {
		p.consume() // END
		if _, err := p.expectKeyword("DECLARATIVES"); err != nil {
			return nil, err
		}
		if _, err := p.expectPeriod(); err != nil {
			return nil, err
		}
		return parseProcedureBody, nil
	}

	sec, err := parseDeclarativeSection(p)
	if err != nil {
		return nil, err
	}
	div.Declaratives = append(div.Declaratives, sec)
	return parseDeclarativeSectionOpt, nil
}

// parseDeclarativeSection parses one DECLARATIVES section: a section header
// (name SECTION .), its mandatory USE statement, and the paragraphs that run until
// the next declarative section header or END DECLARATIVES.
func parseDeclarativeSection(p *parser) (*DeclarativeSection, error) {
	name, err := p.expect(TokenIdentifier, TokenNumber)
	if err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("SECTION"); err != nil {
		return nil, err
	}
	if _, err := p.expectPeriod(); err != nil {
		return nil, err
	}
	sec := &DeclarativeSection{Pos: name.Pos, Name: &Word{Pos: name.Pos, Value: string(name.Value)}}

	use, err := parseUseStatement(p)
	if err != nil {
		return nil, err
	}
	sec.Use = use

	for action := parseDeclarativeParagraphOpt; action != nil && err == nil; {
		action, err = action(p, sec)
	}
	return sec, err
}

// parseDeclarativeParagraphOpt parses one paragraph of a declarative section and
// appends it, then returns itself; it ends the section at END DECLARATIVES, at the
// next declarative section header, or at end of input.
func parseDeclarativeParagraphOpt(p *parser, sec *DeclarativeSection) (parserAction[*DeclarativeSection], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	// END DECLARATIVES is checked before the section header: its END token would
	// otherwise fall through to the paragraph-start error below.
	if end, err := p.atEndDeclaratives(); err != nil {
		return nil, err
	} else if end {
		return nil, nil
	}
	if section, err := p.atSectionHeader(); err != nil {
		return nil, err
	} else if section {
		return nil, nil
	}

	header, err := p.atParagraphHeader()
	if err != nil {
		return nil, err
	}
	if !header && !isStatementVerb(tok) {
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: tok}
	}

	para, err := parseParagraph(p)
	if err != nil {
		return nil, err
	}
	sec.Paragraphs = append(sec.Paragraphs, para)
	return parseDeclarativeParagraphOpt, nil
}

// parseUseStatement parses a USE statement (SPEC.md "declaratives": "USE"
// «use-spec» "."): the USE keyword, a form-specific specification, and the
// terminating period.
func parseUseStatement(p *parser) (*UseStatement, error) {
	kw, err := p.expectKeyword("USE")
	if err != nil {
		return nil, err
	}
	spec, err := parseUseSpec(p)
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPeriod(); err != nil {
		return nil, err
	}
	return &UseStatement{Pos: kw.Pos, Spec: spec}, nil
}

// parseUseSpec parses the body of a USE statement, dispatching among its forms: the
// optional GLOBAL phrase precedes AFTER (exception/error procedure) and BEFORE
// (report-writer) forms; FOR/DEBUGGING begins the debugging form.
func parseUseSpec(p *parser) (UseSpec, error) {
	first, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, UnexpectedEndOfTokensError{Expected: []TokenType{TokenIdentifier}}
	}
	pos := first.Pos

	global := false
	if _, ok, err := p.acceptKeyword("GLOBAL"); err != nil {
		return nil, err
	} else if ok {
		global = true
	}

	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, UnexpectedEndOfTokensError{Expected: []TokenType{TokenIdentifier}}
	}
	switch {
	case keywordIs(tok, "AFTER"):
		return parseExceptionUse(p, pos, global)
	case keywordIs(tok, "BEFORE"):
		return parseReportingUse(p, pos, global)
	case !global && keywordIs(tok, "FOR", "DEBUGGING"):
		return parseDebuggingUse(p, pos)
	default:
		// GLOBAL is valid only for the AFTER (exception/error) and BEFORE
		// (reporting) forms, so once it is seen the debugging form is rejected.
		if global {
			return nil, unexpectedKeyword(tok, "AFTER", "BEFORE")
		}
		return nil, unexpectedKeyword(tok, "AFTER", "BEFORE", "FOR", "DEBUGGING")
	}
}

// parseExceptionUse parses the I-O error-handling USE form: "AFTER STANDARD
// (EXCEPTION | ERROR) PROCEDURE ON (file-name… | INPUT | OUTPUT | I-O | EXTEND)".
// The GLOBAL phrase, if any, has already been consumed into global.
func parseExceptionUse(p *parser, pos Pos, global bool) (UseSpec, error) {
	if _, err := p.expectKeyword("AFTER"); err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("STANDARD"); err != nil {
		return nil, err
	}
	kind, err := p.expectKeyword("EXCEPTION", "ERROR")
	if err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("PROCEDURE"); err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("ON"); err != nil {
		return nil, err
	}

	use := &ExceptionUse{Pos: pos, Global: global, Error: keywordIs(kind, "ERROR")}
	if mode, ok, err := p.acceptKeyword("INPUT", "OUTPUT", "I-O", "EXTEND"); err != nil {
		return nil, err
	} else if ok {
		use.Mode = strings.ToUpper(string(mode.Value))
		return use, nil
	}

	// Otherwise one or more file-names, running to the terminating period.
	for {
		file, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		use.Files = append(use.Files, &Word{Pos: file.Pos, Value: string(file.Value)})

		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok || tok.Type != TokenIdentifier {
			break
		}
	}
	return use, nil
}

// parseReportingUse parses the report-writer USE form: "BEFORE REPORTING
// report-group". The GLOBAL phrase, if any, has already been consumed into global.
func parseReportingUse(p *parser, pos Pos, global bool) (UseSpec, error) {
	if _, err := p.expectKeyword("BEFORE"); err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("REPORTING"); err != nil {
		return nil, err
	}
	name, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	return &ReportingUse{Pos: pos, Global: global, Report: &Word{Pos: name.Pos, Value: string(name.Value)}}, nil
}

// parseDebuggingUse parses the debugging USE form: "[FOR] DEBUGGING ON
// (procedure-name… | ALL PROCEDURES)".
func parseDebuggingUse(p *parser, pos Pos) (UseSpec, error) {
	if err := p.skipOptionalKeyword("FOR"); err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("DEBUGGING"); err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("ON"); err != nil {
		return nil, err
	}

	use := &DebuggingUse{Pos: pos}
	if _, ok, err := p.acceptKeyword("ALL"); err != nil {
		return nil, err
	} else if ok {
		if _, err := p.expectKeyword("PROCEDURES"); err != nil {
			return nil, err
		}
		use.AllProcs = true
		return use, nil
	}

	// Otherwise one or more procedure-names (which may be all digits), running to
	// the terminating period.
	for {
		name, err := p.expect(TokenIdentifier, TokenNumber)
		if err != nil {
			return nil, err
		}
		use.Targets = append(use.Targets, &Word{Pos: name.Pos, Value: string(name.Value)})

		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok || (tok.Type != TokenIdentifier && tok.Type != TokenNumber) {
			break
		}
	}
	return use, nil
}

// parseProcedureBody parses the procedure body. The body is either paragraph-form
// (loose paragraphs, possibly led by an anonymous one) or section-form (a sequence
// of sections); the first header decides which, and the two forms are mutually
// exclusive.
func parseProcedureBody(p *parser, div *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
	if _, err, ok := p.peek(); err != nil {
		return nil, err
	} else if !ok {
		return nil, nil
	}

	section, err := p.atSectionHeader()
	if err != nil {
		return nil, err
	}

	action := parseParagraphOpt
	if section {
		action = parseSectionOpt
	}
	for action != nil && err == nil {
		action, err = action(p, div)
	}
	return nil, err
}

// parseParagraphOpt parses one paragraph of the paragraph-form body and appends it,
// then returns itself; it ends the body at end of input. A SECTION header in this
// form (sections cannot follow loose paragraphs) is an error.
func parseParagraphOpt(p *parser, div *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	if boundary, err := p.atProgramBoundary(); err != nil {
		return nil, err
	} else if boundary {
		return nil, nil
	}

	section, err := p.atSectionHeader()
	if err != nil {
		return nil, err
	}
	if section {
		second, _, _ := p.peekSecond()
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenSymbol}, Actual: second}
	}

	header, err := p.atParagraphHeader()
	if err != nil {
		return nil, err
	}
	if !header && !isStatementVerb(tok) {
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: tok}
	}

	para, err := parseParagraph(p)
	if err != nil {
		return nil, err
	}
	div.Paragraphs = append(div.Paragraphs, para)
	return parseParagraphOpt, nil
}

// parseSectionOpt parses one section of the section-form body and appends it, then
// returns itself; it ends the body at end of input. A non-section construct at the
// top level of a section-form body is an error.
func parseSectionOpt(p *parser, div *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	if boundary, err := p.atProgramBoundary(); err != nil {
		return nil, err
	} else if boundary {
		return nil, nil
	}

	section, err := p.atSectionHeader()
	if err != nil {
		return nil, err
	}
	if !section {
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: tok}
	}

	sec, err := parseSection(p)
	if err != nil {
		return nil, err
	}
	div.Sections = append(div.Sections, sec)
	return parseSectionOpt, nil
}

// parseSection parses a section header (name SECTION [segment-number] .) and its
// paragraphs, which run until the next section header or end of input.
func parseSection(p *parser) (*Section, error) {
	name, err := p.expect(TokenIdentifier, TokenNumber)
	if err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("SECTION"); err != nil {
		return nil, err
	}
	sec := &Section{Pos: name.Pos, Name: &Word{Pos: name.Pos, Value: string(name.Value)}}

	if seg, terr, ok := p.peek(); terr != nil {
		return nil, terr
	} else if ok && seg.Type == TokenNumber {
		p.consume()
		sec.Segment = &NumericLiteral{Pos: seg.Pos, Value: string(seg.Value)}
	}

	if _, err := p.expectPeriod(); err != nil {
		return nil, err
	}

	for action := parseSectionParagraphOpt; action != nil && err == nil; {
		action, err = action(p, sec)
	}
	return sec, err
}

// parseSectionParagraphOpt parses one paragraph of a section and appends it, then
// returns itself; it ends the section at the next section header or end of input.
func parseSectionParagraphOpt(p *parser, sec *Section) (parserAction[*Section], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	if boundary, err := p.atProgramBoundary(); err != nil {
		return nil, err
	} else if boundary {
		return nil, nil
	}

	section, err := p.atSectionHeader()
	if err != nil {
		return nil, err
	}
	if section {
		return nil, nil
	}

	// Require a paragraph start (a name header or a verb); otherwise parseParagraph
	// would consume nothing and loop forever on a stray token (mirrors parseParagraphOpt).
	header, err := p.atParagraphHeader()
	if err != nil {
		return nil, err
	}
	if !header && !isStatementVerb(tok) {
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: tok}
	}

	para, err := parseParagraph(p)
	if err != nil {
		return nil, err
	}
	sec.Paragraphs = append(sec.Paragraphs, para)
	return parseSectionParagraphOpt, nil
}

// parseParagraph parses an optional paragraph-name header (name .) followed by the
// paragraph's sentences. With no header it is the anonymous paragraph (its
// statements begin immediately).
func parseParagraph(p *parser) (*Paragraph, error) {
	para := &Paragraph{}

	header, err := p.atParagraphHeader()
	if err != nil {
		return nil, err
	}
	if header {
		name, _, _ := p.advance()
		if _, err := p.expectPeriod(); err != nil {
			return nil, err
		}
		para.Pos = name.Pos
		para.Name = &Word{Pos: name.Pos, Value: string(name.Value)}
	} else {
		tok, _, _ := p.peek()
		para.Pos = tok.Pos
	}

	for action := parseSentenceOpt; action != nil && err == nil; {
		action, err = action(p, para)
	}
	return para, err
}

// parseSentenceOpt parses one sentence — a statement list terminated by a separator
// period — and appends it to para, then returns itself. The paragraph ends when the
// next token is not a statement verb (a paragraph/section header or end of input).
func parseSentenceOpt(p *parser, para *Paragraph) (parserAction[*Paragraph], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok || !isStatementVerb(tok) {
		return nil, nil
	}

	stmts, err := parseStatementList(p, stopAtSentenceEnd)
	if err != nil {
		return nil, err
	}
	if _, err := p.expectPeriod(); err != nil {
		return nil, err
	}
	para.Sentences = append(para.Sentences, &Sentence{Pos: tok.Pos, Statements: stmts})
	return parseSentenceOpt, nil
}

// stopFunc reports whether a statement list is complete before the next token.
// It peeks, never consuming, so the terminating token (period, scope terminator,
// next verb) remains for the caller.
type stopFunc func(p *parser) (bool, error)

// stopAtSentenceEnd stops a top-level statement list at the sentence-terminating
// separator period or end of input.
func stopAtSentenceEnd(p *parser) (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return isPeriod(tok), nil
}

// parseStatementList parses statements until stop reports the list is complete,
// returning them in source order. It drives an inner action loop so each
// statement kind stays its own small action, per the parser's state-machine
// style; nested statement lists (an IF branch, a PERFORM body) recurse through
// this same helper with a different stop.
func parseStatementList(p *parser, stop stopFunc) ([]Statement, error) {
	var stmts []Statement
	var err error
	for action := parseStatementOpt(stop); action != nil && err == nil; {
		action, err = action(p, &stmts)
	}
	return stmts, err
}

// parseStatementOpt parses one statement and appends it, then returns itself to
// parse the next; it ends the list when stop fires.
func parseStatementOpt(stop stopFunc) parserAction[*[]Statement] {
	return func(p *parser, stmts *[]Statement) (parserAction[*[]Statement], error) {
		done, err := stop(p)
		if err != nil {
			return nil, err
		}
		if done {
			return nil, nil
		}
		stmt, err := parseStatement(p)
		if err != nil {
			return nil, err
		}
		*stmts = append(*stmts, stmt)
		return parseStatementOpt(stop), nil
	}
}

// procedureVerbs are the statement-leading verb keywords the parser recognizes as
// the start of a statement. Any of them ends a preceding operand list (a COBOL
// reserved word cannot be a data-name, so an operand list cannot contain one).
var procedureVerbs = []string{
	"DISPLAY", "MOVE", "ACCEPT", "ADD", "SUBTRACT", "MULTIPLY", "DIVIDE",
	"COMPUTE", "IF", "PERFORM", "EVALUATE", "CALL", "GO", "CONTINUE", "STOP",
	"GOBACK", "EXIT",
}

// procedureScopeTerminators are the explicit scope terminators that close one
// statement without ending the sentence.
var procedureScopeTerminators = []string{
	"END-IF", "END-PERFORM", "END-COMPUTE", "END-ADD", "END-SUBTRACT",
	"END-MULTIPLY", "END-DIVIDE", "END-EVALUATE", "END-CALL",
}

// procedurePhraseKeywords are the clause/phrase keywords that introduce a part of
// a statement and so end a preceding operand list.
var procedurePhraseKeywords = []string{
	"TO", "FROM", "BY", "INTO", "GIVING", "REMAINDER", "UPON", "ROUNDED",
	"CORRESPONDING", "CORR", "THEN", "ELSE", "THROUGH", "THRU", "TIMES",
	"UNTIL", "VARYING", "WITH", "TEST", "BEFORE", "AFTER", "NO", "ADVANCING",
	"DEPENDING", "ON", "RUN", "WHEN", "ALSO", "ANY", "OTHER",
	"USING", "RETURNING", "NOT", "SIZE", "ERROR",
}

// isStatementVerb reports whether tok is a recognized statement-leading verb.
func isStatementVerb(tok Token) bool { return keywordIs(tok, procedureVerbs...) }

// isScopeTerminator reports whether tok is an explicit scope terminator (END-IF …).
func isScopeTerminator(tok Token) bool { return keywordIs(tok, procedureScopeTerminators...) }

// isPhraseKeyword reports whether tok is a statement clause/phrase keyword.
func isPhraseKeyword(tok Token) bool { return keywordIs(tok, procedurePhraseKeywords...) }

// isOperandStart reports whether tok can begin an operand in a statement's operand
// list. Literals always can; an identifier can unless it is a reserved verb, a
// scope terminator, or a phrase keyword — any of which ends the list.
func isOperandStart(tok Token) bool {
	switch tok.Type {
	case TokenString, TokenNumber:
		return true
	case TokenIdentifier:
		return !isStatementVerb(tok) && !isScopeTerminator(tok) && !isPhraseKeyword(tok)
	default:
		return false
	}
}

// parseStatement reads one statement's verb and dispatches to its parser. The
// statement parsers leave the sentence-terminating period for the enclosing
// sentence loop and the next verb for the enclosing statement list.
func parseStatement(p *parser) (Statement, error) {
	tok, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	switch {
	case keywordIs(tok, "DISPLAY"):
		return parseDisplayStatement(p, tok)
	case keywordIs(tok, "MOVE"):
		return parseMoveStatement(p, tok)
	case keywordIs(tok, "ACCEPT"):
		return parseAcceptStatement(p, tok)
	case keywordIs(tok, "ADD", "SUBTRACT", "MULTIPLY", "DIVIDE"):
		return parseArithmeticStatement(p, tok)
	case keywordIs(tok, "COMPUTE"):
		return parseComputeStatement(p, tok)
	case keywordIs(tok, "IF"):
		return parseIfStatement(p, tok)
	case keywordIs(tok, "PERFORM"):
		return parsePerformStatement(p, tok)
	case keywordIs(tok, "EVALUATE"):
		return parseEvaluateStatement(p, tok)
	case keywordIs(tok, "CALL"):
		return parseCallStatement(p, tok)
	case keywordIs(tok, "GO"):
		return parseGoToStatement(p, tok)
	case keywordIs(tok, "CONTINUE"):
		return &ContinueStatement{Pos: tok.Pos}, nil
	case keywordIs(tok, "STOP"):
		return parseStopStatement(p, tok)
	case keywordIs(tok, "GOBACK"):
		return &GobackStatement{Pos: tok.Pos}, nil
	case keywordIs(tok, "EXIT"):
		return parseExitStatement(p, tok)
	default:
		return nil, unexpectedKeyword(tok, "DISPLAY", "MOVE", "ACCEPT",
			"ADD", "SUBTRACT", "MULTIPLY", "DIVIDE", "COMPUTE", "IF", "PERFORM", "EVALUATE", "CALL", "GO", "CONTINUE", "STOP",
			"GOBACK", "EXIT")
	}
}

// stopAtNested stops a nested statement list (an IF branch or a PERFORM body) at a
// separator period, an ELSE, any explicit scope terminator (END-IF, END-PERFORM,
// …), or end of input — each of which is consumed by an enclosing construct, not
// the nested list.
func stopAtNested(p *parser) (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return isPeriod(tok) || keywordIs(tok, "ELSE") || isScopeTerminator(tok), nil
}

// stopAtSizeError stops a SIZE ERROR phrase body at a separator period, the NOT
// keyword introducing the NOT ON SIZE ERROR phrase, any explicit scope terminator
// (END-ADD, a nested END-IF …), or end of input — each consumed by an enclosing
// construct, not the phrase body.
func stopAtSizeError(p *parser) (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return isPeriod(tok) || keywordIs(tok, "NOT") || isScopeTerminator(tok), nil
}

// parseIfStatement parses an IF statement whose verb kw has already been read: a
// condition, an optional THEN, a then-branch, an optional ELSE branch, and an
// optional END-IF. Each branch is either a nested statement list or the NEXT
// SENTENCE alternative (see parseIfBranch).
func parseIfStatement(p *parser, kw Token) (Statement, error) {
	cond, err := parseCondition(p)
	if err != nil {
		return nil, err
	}
	stmt := &IfStatement{Pos: kw.Pos, Cond: cond}

	if err := p.skipOptionalKeyword("THEN"); err != nil {
		return nil, err
	}

	then, err := parseIfBranch(p)
	if err != nil {
		return nil, err
	}
	stmt.Then = then

	hasElse, err := p.peekKeyword("ELSE")
	if err != nil {
		return nil, err
	}
	if hasElse {
		p.consume()
		stmt.HasElse = true
		els, err := parseIfBranch(p)
		if err != nil {
			return nil, err
		}
		stmt.Else = els
	}

	endIf, err := p.peekKeyword("END-IF")
	if err != nil {
		return nil, err
	}
	if endIf {
		p.consume()
		stmt.EndIf = true
	}
	return stmt, nil
}

// parseIfBranch parses one branch of an IF statement: either the NEXT SENTENCE
// alternative — yielding a single [NextSentenceStatement] — or a nested statement
// list (stopping at the branch's enclosing delimiters via stopAtNested).
func parseIfBranch(p *parser) ([]Statement, error) {
	isNext, err := p.peekKeyword("NEXT")
	if err != nil {
		return nil, err
	}
	if isNext {
		next, _, _ := p.peek()
		p.consume() // NEXT
		if _, err := p.expectKeyword("SENTENCE"); err != nil {
			return nil, err
		}
		return []Statement{&NextSentenceStatement{Pos: next.Pos}}, nil
	}
	return parseStatementList(p, stopAtNested)
}

// stopAtEvaluateBranch stops a WHEN-branch statement list at the next WHEN, any
// explicit scope terminator (END-EVALUATE, a nested END-IF …), a separator period,
// or end of input — each consumed by an enclosing construct, not the branch list.
func stopAtEvaluateBranch(p *parser) (bool, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return isPeriod(tok) || keywordIs(tok, "WHEN") || isScopeTerminator(tok), nil
}

// parseEvaluateStatement parses an EVALUATE statement whose verb kw has already been
// read: one or more ALSO-joined subjects, a run of WHEN branches, an optional WHEN
// OTHER branch, and the required END-EVALUATE terminator.
func parseEvaluateStatement(p *parser, kw Token) (Statement, error) {
	stmt := &EvaluateStatement{Pos: kw.Pos}

	subj, err := parseEvaluateSubject(p)
	if err != nil {
		return nil, err
	}
	stmt.Subjects = append(stmt.Subjects, subj)
	for {
		_, also, err := p.acceptKeyword("ALSO")
		if err != nil {
			return nil, err
		}
		if !also {
			break
		}
		subj, err := parseEvaluateSubject(p)
		if err != nil {
			return nil, err
		}
		stmt.Subjects = append(stmt.Subjects, subj)
	}

	for {
		whenTok, ok, err := p.acceptKeyword("WHEN")
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		_, other, err := p.acceptKeyword("OTHER")
		if err != nil {
			return nil, err
		}
		if other {
			body, err := parseStatementList(p, stopAtEvaluateBranch)
			if err != nil {
				return nil, err
			}
			stmt.Other = body
			stmt.HasOther = true
			break // WHEN OTHER is the final branch; only END-EVALUATE may follow.
		}
		when, err := parseEvaluateWhen(p, whenTok)
		if err != nil {
			return nil, err
		}
		stmt.Whens = append(stmt.Whens, when)
	}

	if _, err := p.expectKeyword("END-EVALUATE"); err != nil {
		return nil, err
	}
	return stmt, nil
}

// parseEvaluateWhen parses one WHEN branch whose WHEN keyword whenTok has already
// been read: one or more ALSO-joined objects followed by the branch statement list.
func parseEvaluateWhen(p *parser, whenTok Token) (*EvaluateWhen, error) {
	when := &EvaluateWhen{Pos: whenTok.Pos}

	obj, err := parseEvaluateObject(p)
	if err != nil {
		return nil, err
	}
	when.Objects = append(when.Objects, obj)
	for {
		_, also, err := p.acceptKeyword("ALSO")
		if err != nil {
			return nil, err
		}
		if !also {
			break
		}
		obj, err := parseEvaluateObject(p)
		if err != nil {
			return nil, err
		}
		when.Objects = append(when.Objects, obj)
	}

	body, err := parseStatementList(p, stopAtEvaluateBranch)
	if err != nil {
		return nil, err
	}
	when.Body = body
	return when, nil
}

// parseEvaluateSubject parses one EVALUATE subject (SPEC.md "subject"): the boolean
// TRUE/FALSE, a condition, or an operand. An operand and a condition both begin with
// an operand, so the operand is parsed first and a trailing condition continuation
// (a relational operator, IS, a class keyword, …) decides between them.
func parseEvaluateSubject(p *parser) (*EvaluateSubject, error) {
	if tok, ok, err := p.acceptKeyword("TRUE", "FALSE"); err != nil {
		return nil, err
	} else if ok {
		return &EvaluateSubject{Pos: tok.Pos, Bool: strings.ToUpper(string(tok.Value))}, nil
	}

	// A leading "(" or NOT can only begin a condition.
	cond, ok, err := parseLeadingCondition(p)
	if err != nil {
		return nil, err
	}
	if ok {
		return &EvaluateSubject{Pos: conditionPos(cond), Cond: cond}, nil
	}

	left, err := parseExpr(p)
	if err != nil {
		return nil, err
	}
	cond, isCond, err := parseConditionFromExpr(p, left)
	if err != nil {
		return nil, err
	}
	if isCond {
		return &EvaluateSubject{Pos: exprPos(left), Cond: cond}, nil
	}
	op, err := operandFromExpr(p, left)
	if err != nil {
		return nil, err
	}
	return &EvaluateSubject{Pos: exprPos(left), Operand: op}, nil
}

// parseEvaluateObject parses one EVALUATE WHEN object (SPEC.md "object"): ANY, the
// boolean TRUE/FALSE, or an optional leading NOT applied to either an operand (with
// an optional THROUGH/THRU range) or a condition. As with subjects, the operand and
// condition forms both begin with an operand, disambiguated by what follows it.
func parseEvaluateObject(p *parser) (*EvaluateObject, error) {
	if tok, ok, err := p.acceptKeyword("ANY"); err != nil {
		return nil, err
	} else if ok {
		return &EvaluateObject{Pos: tok.Pos, Any: true}, nil
	}
	if tok, ok, err := p.acceptKeyword("TRUE", "FALSE"); err != nil {
		return nil, err
	} else if ok {
		return &EvaluateObject{Pos: tok.Pos, Bool: strings.ToUpper(string(tok.Value))}, nil
	}

	obj := &EvaluateObject{}
	if notTok, ok, err := p.acceptKeyword("NOT"); err != nil {
		return nil, err
	} else if ok {
		obj.Not = true
		obj.Pos = notTok.Pos
	}

	// A leading "(" can only begin a (parenthesized) condition.
	cond, ok, err := parseLeadingCondition(p)
	if err != nil {
		return nil, err
	}
	if ok {
		obj.Cond = cond
		if obj.Pos == (Pos{}) {
			obj.Pos = conditionPos(cond)
		}
		return obj, nil
	}

	left, err := parseExpr(p)
	if err != nil {
		return nil, err
	}
	if obj.Pos == (Pos{}) {
		obj.Pos = exprPos(left)
	}

	cond, isCond, err := parseConditionFromExpr(p, left)
	if err != nil {
		return nil, err
	}
	if isCond {
		obj.Cond = cond
		return obj, nil
	}

	op, err := operandFromExpr(p, left)
	if err != nil {
		return nil, err
	}
	obj.Operand = op

	if _, ok, err := p.acceptKeyword("THROUGH", "THRU"); err != nil {
		return nil, err
	} else if ok {
		upper, err := parseOperand(p)
		if err != nil {
			return nil, err
		}
		obj.Through = upper
	}
	return obj, nil
}

// operandFromExpr narrows a parsed expression to a bare operand. An EVALUATE operand
// is an identifier or literal (SPEC.md "operand"), both of which are also [Type]; an
// arithmetic expression is not a valid bare operand and is rejected.
func operandFromExpr(p *parser, e Expr) (Type, error) {
	if op, ok := e.(Type); ok {
		return op, nil
	}
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, UnexpectedEndOfTokensError{Expected: []TokenType{TokenIdentifier, TokenString, TokenNumber}}
	}
	return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier, TokenString, TokenNumber}, Actual: tok}
}

// parsePerformStatement parses a PERFORM statement whose verb kw has already been
// read. It distinguishes the out-of-line form (a procedure-name, optional THROUGH,
// and an optional loop) from the inline form (an optional loop, an inline body, and
// END-PERFORM) by what follows the verb.
func parsePerformStatement(p *parser, kw Token) (Statement, error) {
	stmt := &PerformStatement{Pos: kw.Pos}

	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, UnexpectedEndOfTokensError{Expected: []TokenType{TokenIdentifier}}
	}

	switch {
	case keywordIs(tok, "UNTIL", "VARYING", "WITH"):
		// Inline loop with no procedure-name.
		stmt.Inline = true
		if err := parsePerformLoopSpec(p, stmt); err != nil {
			return nil, err
		}
		return parsePerformInlineBody(p, stmt)
	case isStatementVerb(tok):
		// Inline body executed once.
		stmt.Inline = true
		return parsePerformInlineBody(p, stmt)
	case isOperandStart(tok):
		return parsePerformWithOperand(p, stmt)
	default:
		return nil, unexpectedKeyword(tok, "UNTIL", "VARYING", "WITH")
	}
}

// parsePerformWithOperand handles a PERFORM whose first token is an operand: either
// a count ("n TIMES …" inline) or a procedure-name (out-of-line).
func parsePerformWithOperand(p *parser, stmt *PerformStatement) (Statement, error) {
	lead, _, _ := p.peek()
	first, err := parseOperand(p)
	if err != nil {
		return nil, err
	}

	times, err := p.peekKeyword("TIMES")
	if err != nil {
		return nil, err
	}
	if times {
		p.consume()
		stmt.Times = first
		stmt.Inline = true
		return parsePerformInlineBody(p, stmt)
	}

	name, ok := procedureNameFromOperand(first)
	if !ok {
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier, TokenNumber}, Actual: lead}
	}
	stmt.Target = name

	through, err := p.peekKeyword("THROUGH", "THRU")
	if err != nil {
		return nil, err
	}
	if through {
		p.consume()
		// A procedure-name may be an identifier or an all-digit word.
		t, err := p.expect(TokenIdentifier, TokenNumber)
		if err != nil {
			return nil, err
		}
		stmt.Through = &Word{Pos: t.Pos, Value: string(t.Value)}
	}

	if err := parsePerformLoopSpec(p, stmt); err != nil {
		return nil, err
	}
	return stmt, nil
}

// parsePerformLoopSpec parses an optional PERFORM loop specification onto stmt:
// "n TIMES", "[WITH TEST BEFORE|AFTER] UNTIL condition", or "VARYING …".
func parsePerformLoopSpec(p *parser, stmt *PerformStatement) error {
	with, err := p.peekKeyword("WITH")
	if err != nil {
		return err
	}
	if with {
		p.consume()
		if _, err := p.expectKeyword("TEST"); err != nil {
			return err
		}
		ba, err := p.expectKeyword("BEFORE", "AFTER")
		if err != nil {
			return err
		}
		stmt.TestAfter = keywordIs(ba, "AFTER")
		if _, err := p.expectKeyword("UNTIL"); err != nil {
			return err
		}
		cond, err := parseCondition(p)
		if err != nil {
			return err
		}
		stmt.Until = cond
		return nil
	}

	until, err := p.peekKeyword("UNTIL")
	if err != nil {
		return err
	}
	if until {
		p.consume()
		cond, err := parseCondition(p)
		if err != nil {
			return err
		}
		stmt.Until = cond
		return nil
	}

	varying, err := p.peekKeyword("VARYING")
	if err != nil {
		return err
	}
	if varying {
		vtok, _, _ := p.advance()
		v, err := parsePerformVarying(p, vtok)
		if err != nil {
			return err
		}
		stmt.Varying = v
		return nil
	}

	// "n TIMES" loop (a count operand followed by TIMES).
	tok, terr, tokOK := p.peek()
	if terr != nil {
		return terr
	}
	if tokOK && isOperandStart(tok) {
		count, err := parseOperand(p)
		if err != nil {
			return err
		}
		if _, err := p.expectKeyword("TIMES"); err != nil {
			return err
		}
		stmt.Times = count
	}
	return nil
}

// parsePerformVarying parses the VARYING phrase after its keyword vtok: the loop
// variable, FROM initial value, BY increment, and UNTIL termination condition.
func parsePerformVarying(p *parser, vtok Token) (*PerformVarying, error) {
	name, err := parseIdentifierToken(p)
	if err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("FROM"); err != nil {
		return nil, err
	}
	from, err := parseOperand(p)
	if err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("BY"); err != nil {
		return nil, err
	}
	by, err := parseOperand(p)
	if err != nil {
		return nil, err
	}
	if _, err := p.expectKeyword("UNTIL"); err != nil {
		return nil, err
	}
	cond, err := parseCondition(p)
	if err != nil {
		return nil, err
	}
	return &PerformVarying{Pos: vtok.Pos, Name: name, From: from, By: by, Until: cond}, nil
}

// parsePerformInlineBody parses the inline body of a PERFORM and its required
// END-PERFORM terminator.
func parsePerformInlineBody(p *parser, stmt *PerformStatement) (Statement, error) {
	body, err := parseStatementList(p, stopAtNested)
	if err != nil {
		return nil, err
	}
	stmt.Body = body
	if _, err := p.expectKeyword("END-PERFORM"); err != nil {
		return nil, err
	}
	stmt.EndPerform = true
	return stmt, nil
}

// parseOperandList collects operands until the next token cannot begin one
// (a period, verb, scope terminator, or phrase keyword).
func parseOperandList(p *parser) ([]Type, error) {
	var ops []Type
	for {
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok || !isOperandStart(tok) {
			return ops, nil
		}
		op, err := parseOperand(p)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
}

// parseDisplayStatement parses a DISPLAY statement whose verb kw has already been
// read: its operands, an optional UPON mnemonic, and an optional [WITH] NO
// ADVANCING phrase.
func parseDisplayStatement(p *parser, kw Token) (Statement, error) {
	ops, err := parseOperandList(p)
	if err != nil {
		return nil, err
	}
	stmt := &DisplayStatement{Pos: kw.Pos, Operands: ops}

	upon, err := p.peekKeyword("UPON")
	if err != nil {
		return nil, err
	}
	if upon {
		p.consume()
		m, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		stmt.Upon = &Word{Pos: m.Pos, Value: string(m.Value)}
	}

	noAdv, err := parseNoAdvancing(p)
	if err != nil {
		return nil, err
	}
	stmt.NoAdvancing = noAdv
	return stmt, nil
}

// parseNoAdvancing consumes an optional "[WITH] NO ADVANCING" phrase and reports
// whether it was present. In a DISPLAY a leading WITH can only introduce NO
// ADVANCING, so it is safe to consume before requiring NO ADVANCING.
func parseNoAdvancing(p *parser) (bool, error) {
	withIs, err := p.peekKeyword("WITH")
	if err != nil {
		return false, err
	}
	if withIs {
		p.consume()
		if _, err := p.expectKeyword("NO"); err != nil {
			return false, err
		}
		if _, err := p.expectKeyword("ADVANCING"); err != nil {
			return false, err
		}
		return true, nil
	}

	noIs, err := p.peekKeyword("NO")
	if err != nil {
		return false, err
	}
	if noIs {
		p.consume()
		if _, err := p.expectKeyword("ADVANCING"); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// parseMoveStatement parses a MOVE statement whose verb kw has already been read:
// an optional CORRESPONDING/CORR, a sending operand, "TO", and one or more
// receiving identifiers.
func parseMoveStatement(p *parser, kw Token) (Statement, error) {
	stmt := &MoveStatement{Pos: kw.Pos}

	corr, err := p.peekKeyword("CORRESPONDING", "CORR")
	if err != nil {
		return nil, err
	}
	if corr {
		p.consume()
		stmt.Corresponding = true
	}

	src, err := parseOperand(p)
	if err != nil {
		return nil, err
	}
	stmt.Source = src

	if _, err := p.expectKeyword("TO"); err != nil {
		return nil, err
	}

	targets, err := parseIdentifierList(p)
	if err != nil {
		return nil, err
	}
	stmt.Targets = targets
	return stmt, nil
}

// parseIdentifierList parses one or more receiving identifiers, stopping at the
// first token that cannot begin an operand.
func parseIdentifierList(p *parser) ([]*Identifier, error) {
	first, err := parseIdentifierToken(p)
	if err != nil {
		return nil, err
	}
	ids := []*Identifier{first}
	for {
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok || !isOperandStart(tok) {
			return ids, nil
		}
		id, err := parseIdentifierToken(p)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
}

// parseAcceptStatement parses an ACCEPT statement whose verb kw has already been
// read: a receiving identifier and an optional FROM source.
func parseAcceptStatement(p *parser, kw Token) (Statement, error) {
	target, err := parseIdentifierToken(p)
	if err != nil {
		return nil, err
	}
	stmt := &AcceptStatement{Pos: kw.Pos, Target: target}

	from, err := p.peekKeyword("FROM")
	if err != nil {
		return nil, err
	}
	if from {
		p.consume()
		src, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		stmt.From = &Word{Pos: src.Pos, Value: string(src.Value)}
	}
	return stmt, nil
}

// parseGoToStatement parses a GO TO statement whose verb kw has already been read:
// an optional TO, one or more procedure-names, and an optional DEPENDING ON
// selector.
func parseGoToStatement(p *parser, kw Token) (Statement, error) {
	if err := p.skipOptionalKeyword("TO"); err != nil {
		return nil, err
	}
	stmt := &GoToStatement{Pos: kw.Pos}

	for {
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		// A procedure-name is an identifier or an all-digit word; stop at verbs,
		// scope terminators, and phrase keywords (e.g. DEPENDING).
		if !ok || (tok.Type != TokenIdentifier && tok.Type != TokenNumber) ||
			isStatementVerb(tok) || isScopeTerminator(tok) || isPhraseKeyword(tok) {
			break
		}
		p.consume()
		stmt.Targets = append(stmt.Targets, &Word{Pos: tok.Pos, Value: string(tok.Value)})
	}

	depending, err := p.peekKeyword("DEPENDING")
	if err != nil {
		return nil, err
	}
	if depending {
		p.consume()
		if err := p.skipOptionalKeyword("ON"); err != nil {
			return nil, err
		}
		id, err := parseIdentifierToken(p)
		if err != nil {
			return nil, err
		}
		stmt.DependingOn = id
	}

	if len(stmt.Targets) == 0 {
		tok, _, _ := p.peek()
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: tok}
	}
	return stmt, nil
}

// parseStopStatement parses a STOP statement whose verb kw has already been read:
// either STOP RUN or STOP <literal>.
func parseStopStatement(p *parser, kw Token) (Statement, error) {
	run, err := p.peekKeyword("RUN")
	if err != nil {
		return nil, err
	}
	if run {
		p.consume()
		return &StopStatement{Pos: kw.Pos, Run: true}, nil
	}

	lit, err := p.expect(TokenString, TokenNumber)
	if err != nil {
		return nil, err
	}
	return &StopStatement{Pos: kw.Pos, Literal: valueNode(lit)}, nil
}

// parseExitStatement parses an EXIT statement whose verb kw has already been read:
// a bare EXIT or EXIT followed by one of PROGRAM/PARAGRAPH/SECTION/PERFORM. The
// optional object keyword is matched explicitly, so a bare EXIT (followed by a
// period or the next statement's verb) leaves Option empty.
func parseExitStatement(p *parser, kw Token) (Statement, error) {
	hasOption, err := p.peekKeyword("PROGRAM", "PARAGRAPH", "SECTION", "PERFORM")
	if err != nil {
		return nil, err
	}
	stmt := &ExitStatement{Pos: kw.Pos}
	if hasOption {
		opt, _ := p.expectKeyword("PROGRAM", "PARAGRAPH", "SECTION", "PERFORM")
		stmt.Option = strings.ToUpper(string(opt.Value))
	}
	return stmt, nil
}

// parseArithmeticStatement parses an ADD/SUBTRACT/MULTIPLY/DIVIDE statement whose
// verb kw has already been read: source operands, an optional connector
// (TO/FROM/BY/INTO) with in-place receivers, an optional GIVING result list, an
// optional DIVIDE REMAINDER, optional [NOT] ON SIZE ERROR phrases, and an optional
// END-<verb> scope terminator. Each receiver carries its own optional ROUNDED.
func parseArithmeticStatement(p *parser, kw Token) (Statement, error) {
	verb := strings.ToUpper(string(kw.Value))
	stmt := &ArithmeticStatement{Pos: kw.Pos, Verb: verb}

	ops, err := parseOperandList(p)
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		tok, _, _ := p.peek()
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier, TokenNumber}, Actual: tok}
	}
	stmt.Operands = ops

	connector, err := p.peekKeyword("TO", "FROM", "BY", "INTO")
	if err != nil {
		return nil, err
	}
	if connector {
		conn, _ := p.expectKeyword("TO", "FROM", "BY", "INTO")
		stmt.Connector = strings.ToUpper(string(conn.Value))
		targets, err := parseArithmeticReceivers(p)
		if err != nil {
			return nil, err
		}
		stmt.Targets = targets
	}

	giving, err := p.peekKeyword("GIVING")
	if err != nil {
		return nil, err
	}
	if giving {
		p.consume()
		receivers, err := parseArithmeticReceivers(p)
		if err != nil {
			return nil, err
		}
		stmt.Giving = receivers

		// REMAINDER is a DIVIDE-only phrase; for the other verbs the keyword is
		// left for the enclosing sentence to reject.
		if verb == "DIVIDE" {
			remainder, err := p.peekKeyword("REMAINDER")
			if err != nil {
				return nil, err
			}
			if remainder {
				p.consume()
				id, err := parseReceiverIdentifier(p)
				if err != nil {
					return nil, err
				}
				stmt.Remainder = id
			}
		}
	}

	if len(stmt.Targets) == 0 && len(stmt.Giving) == 0 {
		tok, _, _ := p.peek()
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: tok}
	}

	sizeErr, err := parseSizeErrorPhrases(p)
	if err != nil {
		return nil, err
	}
	stmt.SizeError = sizeErr

	endScope, err := p.peekKeyword("END-" + verb)
	if err != nil {
		return nil, err
	}
	if endScope {
		p.consume()
		stmt.EndScope = true
	}
	return stmt, nil
}

// parseArithmeticReceivers parses one or more receiving fields, each an identifier
// with an optional trailing ROUNDED, as used after a TO/FROM/BY/INTO connector and
// after GIVING. It reads the first receiver, then continues while the next token
// can begin an operand (a further receiver).
func parseArithmeticReceivers(p *parser) ([]*ArithmeticTarget, error) {
	var targets []*ArithmeticTarget
	for {
		// A receiver data-name is required as the first element and continues the
		// list only while the next token can start an operand; reject a reserved
		// verb, scope terminator, or phrase keyword standing in for a data-name.
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok || !isOperandStart(tok) {
			if len(targets) == 0 {
				return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: tok}
			}
			return targets, nil
		}

		id, err := parseIdentifierToken(p)
		if err != nil {
			return nil, err
		}
		t := &ArithmeticTarget{Pos: id.Pos, Name: id}
		rounded, err := p.peekKeyword("ROUNDED")
		if err != nil {
			return nil, err
		}
		if rounded {
			p.consume()
			t.Rounded = true
		}
		targets = append(targets, t)
	}
}

// parseReceiverIdentifier parses a single data-name in a position that requires one
// (the DIVIDE REMAINDER target), rejecting a reserved verb, scope terminator, or
// phrase keyword standing in for the data-name before consuming it.
func parseReceiverIdentifier(p *parser) (*Identifier, error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok || !isOperandStart(tok) {
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: tok}
	}
	return parseIdentifierToken(p)
}

// parseSizeErrorPhrases parses the optional [ON] SIZE ERROR and NOT [ON] SIZE ERROR
// imperative phrases that may follow an arithmetic statement's receivers or a
// COMPUTE expression. Each phrase body is a nested statement list (stopAtSizeError).
// ON, SIZE, ERROR, and NOT are reserved words, so one-token lookahead disambiguates
// the phrase from a following statement or scope terminator.
func parseSizeErrorPhrases(p *parser) (SizeErrorPhrases, error) {
	var ph SizeErrorPhrases

	onSize, err := p.consumeSizeErrorLead()
	if err != nil {
		return ph, err
	}
	if onSize {
		ph.HasOnSizeError = true
		body, err := parseStatementList(p, stopAtSizeError)
		if err != nil {
			return ph, err
		}
		ph.OnSizeError = body
	}

	hasNot, err := p.peekKeyword("NOT")
	if err != nil {
		return ph, err
	}
	if hasNot {
		p.consume() // NOT
		if err := p.skipOptionalKeyword("ON"); err != nil {
			return ph, err
		}
		if _, err := p.expectKeyword("SIZE"); err != nil {
			return ph, err
		}
		if _, err := p.expectKeyword("ERROR"); err != nil {
			return ph, err
		}
		ph.HasNotOnSizeError = true
		body, err := parseStatementList(p, stopAtSizeError)
		if err != nil {
			return ph, err
		}
		ph.NotOnSizeError = body
	}

	return ph, nil
}

// consumeSizeErrorLead consumes an optional [ "ON" ] "SIZE" "ERROR" lead-in,
// reporting whether it was present. ON, SIZE, and ERROR are reserved words, so the
// next token unambiguously signals the phrase.
func (p *parser) consumeSizeErrorLead() (bool, error) {
	on, err := p.peekKeyword("ON")
	if err != nil {
		return false, err
	}
	size, err := p.peekKeyword("SIZE")
	if err != nil {
		return false, err
	}
	if !on && !size {
		return false, nil
	}
	if on {
		p.consume() // ON
	}
	if _, err := p.expectKeyword("SIZE"); err != nil {
		return false, err
	}
	if _, err := p.expectKeyword("ERROR"); err != nil {
		return false, err
	}
	return true, nil
}

// parseComputeStatement parses a COMPUTE statement whose verb kw has already been
// read: one or more receiving fields (each optionally ROUNDED), "=" (or EQUAL), an
// arithmetic expression, optional [NOT] ON SIZE ERROR phrases, and an optional
// END-COMPUTE.
func parseComputeStatement(p *parser, kw Token) (Statement, error) {
	stmt := &ComputeStatement{Pos: kw.Pos}

	for {
		id, err := parseIdentifierToken(p)
		if err != nil {
			return nil, err
		}
		target := ComputeTarget{Pos: id.Pos, Name: id}
		rounded, err := p.peekKeyword("ROUNDED")
		if err != nil {
			return nil, err
		}
		if rounded {
			p.consume()
			target.Rounded = true
		}
		stmt.Targets = append(stmt.Targets, target)

		isEq, err := p.peekSymbol("=")
		if err != nil {
			return nil, err
		}
		isEqual, err := p.peekKeyword("EQUAL")
		if err != nil {
			return nil, err
		}
		if isEq || isEqual {
			break
		}
	}

	if isEqual, err := p.peekKeyword("EQUAL"); err != nil {
		return nil, err
	} else if isEqual {
		p.consume()
	} else if _, err := p.expectSymbol("="); err != nil {
		return nil, err
	}

	expr, err := parseExpr(p)
	if err != nil {
		return nil, err
	}
	stmt.Expr = expr

	sizeErr, err := parseSizeErrorPhrases(p)
	if err != nil {
		return nil, err
	}
	stmt.SizeError = sizeErr

	endScope, err := p.peekKeyword("END-COMPUTE")
	if err != nil {
		return nil, err
	}
	if endScope {
		p.consume()
		stmt.EndScope = true
	}
	return stmt, nil
}

// parseCallStatement parses a CALL statement whose verb kw has already been read:
// the called program (an alphanumeric literal or an identifier), an optional USING
// phrase of operands each with an optional BY REFERENCE/CONTENT/VALUE mode, an
// optional RETURNING identifier, and an optional END-CALL scope terminator.
func parseCallStatement(p *parser, kw Token) (Statement, error) {
	stmt := &CallStatement{Pos: kw.Pos}

	// Target: an AlphanumericLiteral or an identifier (a numeric literal is not a
	// valid program-name).
	tok, err := p.expect(TokenIdentifier, TokenString)
	if err != nil {
		return nil, err
	}
	if tok.Type == TokenString {
		stmt.Target = &StringLiteral{Pos: tok.Pos, Value: string(tok.Value)}
	} else {
		id, err := parseIdentifier(p, tok)
		if err != nil {
			return nil, err
		}
		stmt.Target = id
	}

	using, err := p.peekKeyword("USING")
	if err != nil {
		return nil, err
	}
	if using {
		p.consume()
		args, err := parseCallUsing(p)
		if err != nil {
			return nil, err
		}
		stmt.Using = args
	}

	returning, err := p.peekKeyword("RETURNING")
	if err != nil {
		return nil, err
	}
	if returning {
		p.consume()
		id, err := parseIdentifierToken(p)
		if err != nil {
			return nil, err
		}
		stmt.Returning = id
	}

	endCall, err := p.peekKeyword("END-CALL")
	if err != nil {
		return nil, err
	}
	if endCall {
		p.consume()
		stmt.EndCall = true
	}
	return stmt, nil
}

// parseCallUsing parses the operands of a CALL … USING phrase: one or more
// arguments, each an operand optionally preceded by a BY REFERENCE/CONTENT/VALUE
// mode. It requires at least one argument and stops at the first token that can
// neither introduce a mode nor begin an operand.
func parseCallUsing(p *parser) ([]*CallArgument, error) {
	first, err := parseCallArgument(p)
	if err != nil {
		return nil, err
	}
	args := []*CallArgument{first}
	for {
		by, err := p.peekKeyword("BY")
		if err != nil {
			return nil, err
		}
		if !by {
			tok, err, ok := p.peek()
			if err != nil {
				return nil, err
			}
			if !ok || !isOperandStart(tok) {
				return args, nil
			}
		}
		arg, err := parseCallArgument(p)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
}

// parseCallArgument parses one CALL … USING argument: an optional BY
// REFERENCE/CONTENT/VALUE mode followed by an operand. The argument's position is
// the BY keyword when present, otherwise the operand.
func parseCallArgument(p *parser) (*CallArgument, error) {
	byTok, by, err := p.acceptKeyword("BY")
	if err != nil {
		return nil, err
	}
	var mode string
	if by {
		m, err := p.expectKeyword("REFERENCE", "CONTENT", "VALUE")
		if err != nil {
			return nil, err
		}
		mode = strings.ToUpper(string(m.Value))
	}

	// An operand is required here. parseOperand accepts any identifier token, so
	// reject a reserved keyword (e.g. RETURNING or a scope terminator) that would
	// otherwise be silently swallowed as a data-name, hiding the clause it begins.
	lead, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, UnexpectedEndOfTokensError{Expected: []TokenType{TokenIdentifier, TokenString, TokenNumber}}
	}
	if !isOperandStart(lead) {
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier, TokenString, TokenNumber}, Actual: lead}
	}

	// The operand begins at the next token; capture its position before parsing so
	// the argument points at its start when no BY phrase precedes it.
	op, err := parseOperand(p)
	if err != nil {
		return nil, err
	}
	pos := lead.Pos
	if by {
		pos = byTok.Pos
	}
	return &CallArgument{Pos: pos, Mode: mode, Operand: op}, nil
}

// The following identifier and arithmetic-expression sub-parsers are recursive
// descent, not part of the statement action machinery. The action-loop rule
// (CLAUDE.md) governs AST-accreting loops over divisions/sections/paragraphs/
// statements; an arithmetic expression or identifier reference is a leaf
// sub-grammar with its own natural precedence recursion, so plain recursive
// helpers returning (Expr, error) / (*Identifier, error) are idiomatic here.

// parseOperand parses an operand: an identifier or a literal (SPEC.md "operand").
// A figurative constant (ZERO, SPACES, …) tokenizes as an identifier and so parses
// as a single-name [Identifier]; its identity is its spelling.
func parseOperand(p *parser) (Type, error) {
	tok, err := p.expect(TokenIdentifier, TokenString, TokenNumber)
	if err != nil {
		return nil, err
	}
	switch tok.Type {
	case TokenString:
		return &StringLiteral{Pos: tok.Pos, Value: string(tok.Value)}, nil
	case TokenNumber:
		return &NumericLiteral{Pos: tok.Pos, Value: string(tok.Value)}, nil
	default:
		return parseIdentifier(p, tok)
	}
}

// parseIdentifierToken consumes one identifier token and parses the data
// reference it begins, including any IN/OF qualification, subscript, or
// reference-modifier.
func parseIdentifierToken(p *parser) (*Identifier, error) {
	tok, err := p.expect(TokenIdentifier)
	if err != nil {
		return nil, err
	}
	return parseIdentifier(p, tok)
}

// parseIdentifier parses a data reference whose data-name token name has already
// been read: optional IN/OF qualification followed by an optional parenthesized
// subscript or reference-modifier (SPEC.md "identifier").
func parseIdentifier(p *parser, name Token) (*Identifier, error) {
	id := &Identifier{Pos: name.Pos, Name: &Word{Pos: name.Pos, Value: string(name.Value)}}

	for {
		is, err := p.peekKeyword("IN", "OF")
		if err != nil {
			return nil, err
		}
		if !is {
			break
		}
		p.consume() // IN / OF
		q, err := p.expect(TokenIdentifier)
		if err != nil {
			return nil, err
		}
		id.Qualifiers = append(id.Qualifiers, &Word{Pos: q.Pos, Value: string(q.Value)})
	}

	// An identifier may carry a subscript and/or a reference-modifier, in that
	// order (SPEC.md: identifier = qualified-name [ subscript ] [ reference-modifier ]),
	// e.g. A(I)(1:3).
	open, err := p.peekSymbol("(")
	if err != nil {
		return nil, err
	}
	if open {
		isRefMod, err := parseSubscriptOrRefMod(p, id)
		if err != nil {
			return nil, err
		}
		// A reference-modifier may follow a subscript; a second parenthesized group
		// after a subscript must be the reference-modifier (it requires a colon).
		if !isRefMod {
			open2, err := p.peekSymbol("(")
			if err != nil {
				return nil, err
			}
			if open2 {
				if err := parseReferenceModifier(p, id); err != nil {
					return nil, err
				}
			}
		}
	}
	return id, nil
}

// parseSubscriptOrRefMod parses the first parenthesized suffix of an identifier. A
// top-level ":" marks a reference-modifier "(start:length)"; otherwise it is a
// subscript list "(sub {sub})". It reports whether the suffix was a
// reference-modifier. The opening parenthesis has not yet been consumed.
func parseSubscriptOrRefMod(p *parser, id *Identifier) (bool, error) {
	open, err := p.expectSymbol("(")
	if err != nil {
		return false, err
	}
	first, err := parseExpr(p)
	if err != nil {
		return false, err
	}

	isColon, err := p.peekSymbol(":")
	if err != nil {
		return false, err
	}
	if isColon {
		p.consume() // ":"
		rm, err := finishReferenceModifier(p, open, first)
		if err != nil {
			return false, err
		}
		id.RefMod = rm
		return true, nil
	}

	subs := []Expr{first}
	for {
		closing, err := p.peekSymbol(")")
		if err != nil {
			return false, err
		}
		if closing {
			break
		}
		sub, err := parseExpr(p)
		if err != nil {
			return false, err
		}
		subs = append(subs, sub)
	}
	if _, err := p.expectSymbol(")"); err != nil {
		return false, err
	}
	id.Subscripts = subs
	return false, nil
}

// parseReferenceModifier parses a "(start:length)" reference-modifier, requiring
// the colon. It is used for the optional reference-modifier that may follow a
// subscript; a parenthesized group there without a colon (a second subscript list)
// is invalid and reported via the missing-colon error.
func parseReferenceModifier(p *parser, id *Identifier) error {
	open, err := p.expectSymbol("(")
	if err != nil {
		return err
	}
	start, err := parseExpr(p)
	if err != nil {
		return err
	}
	if _, err := p.expectSymbol(":"); err != nil {
		return err
	}
	rm, err := finishReferenceModifier(p, open, start)
	if err != nil {
		return err
	}
	id.RefMod = rm
	return nil
}

// finishReferenceModifier parses the optional length and closing ")" of a
// reference-modifier whose opening "(" (open), start expression, and ":" have
// already been consumed.
func finishReferenceModifier(p *parser, open Token, start Expr) (*ReferenceModifier, error) {
	rm := &ReferenceModifier{Pos: open.Pos, Start: start}
	closing, err := p.peekSymbol(")")
	if err != nil {
		return nil, err
	}
	if !closing {
		length, err := parseExpr(p)
		if err != nil {
			return nil, err
		}
		rm.Length = length
	}
	if _, err := p.expectSymbol(")"); err != nil {
		return nil, err
	}
	return rm, nil
}

// parseExpr parses an arithmetic expression: terms joined by "+"/"-"
// (left-associative).
func parseExpr(p *parser) (Expr, error) {
	left, err := parseTerm(p)
	if err != nil {
		return nil, err
	}
	for {
		is, err := p.peekSymbol("+", "-")
		if err != nil {
			return nil, err
		}
		if !is {
			return left, nil
		}
		op, _ := p.expectSymbol("+", "-")
		right, err := parseTerm(p)
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Pos: exprPos(left), Op: string(op.Value), Left: left, Right: right}
	}
}

// parseTerm parses a term: factors joined by "*"/"/" (left-associative).
func parseTerm(p *parser) (Expr, error) {
	left, err := parseFactor(p)
	if err != nil {
		return nil, err
	}
	for {
		is, err := p.peekSymbol("*", "/")
		if err != nil {
			return nil, err
		}
		if !is {
			return left, nil
		}
		op, _ := p.expectSymbol("*", "/")
		right, err := parseFactor(p)
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Pos: exprPos(left), Op: string(op.Value), Left: left, Right: right}
	}
}

// parseFactor parses a factor: an optional leading sign applied to the first
// primary, followed by a left-associative chain of "**" exponentiations. Per
// SPEC.md (`factor = [sign] primary { "**" primary }`) the sign binds only to the
// first primary (so "-A ** B" is "(-A) ** B"), and exponentiation — equal-
// precedence, evaluated left to right in COBOL — still parses after a signed
// primary.
func parseFactor(p *parser) (Expr, error) {
	signed, err := p.peekSymbol("+", "-")
	if err != nil {
		return nil, err
	}
	var sign *Token
	if signed {
		op, _ := p.expectSymbol("+", "-")
		sign = &op
	}

	left, err := parsePrimary(p)
	if err != nil {
		return nil, err
	}
	if sign != nil {
		left = &UnaryExpr{Pos: sign.Pos, Op: string(sign.Value), Operand: left}
	}

	for {
		is, err := p.peekSymbol("**")
		if err != nil {
			return nil, err
		}
		if !is {
			return left, nil
		}
		op, _ := p.expectSymbol("**")
		right, err := parsePrimary(p)
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Pos: exprPos(left), Op: string(op.Value), Left: left, Right: right}
	}
}

// parsePrimary parses a primary: a parenthesized expression or an operand.
func parsePrimary(p *parser) (Expr, error) {
	open, err := p.peekSymbol("(")
	if err != nil {
		return nil, err
	}
	if open {
		lp, _ := p.expectSymbol("(")
		inner, err := parseExpr(p)
		if err != nil {
			return nil, err
		}
		if _, err := p.expectSymbol(")"); err != nil {
			return nil, err
		}
		return &ParenExpr{Pos: lp.Pos, Expr: inner}, nil
	}

	tok, err := p.expect(TokenIdentifier, TokenString, TokenNumber)
	if err != nil {
		return nil, err
	}
	switch tok.Type {
	case TokenString:
		return &StringLiteral{Pos: tok.Pos, Value: string(tok.Value)}, nil
	case TokenNumber:
		return &NumericLiteral{Pos: tok.Pos, Value: string(tok.Value)}, nil
	default:
		return parseIdentifier(p, tok)
	}
}

// procedureNameFromOperand extracts a procedure-name word from an operand, for the
// PERFORM target. A procedure-name is a bare data-name (an [Identifier] with no
// qualification, subscript, or reference-modifier) or an all-digit numeric literal;
// a string literal or a qualified/subscripted reference is not a valid
// procedure-name and reports false.
func procedureNameFromOperand(t Type) (*Word, bool) {
	switch v := t.(type) {
	case *Identifier:
		if v.Name != nil && len(v.Qualifiers) == 0 && len(v.Subscripts) == 0 && v.RefMod == nil {
			return v.Name, true
		}
	case *NumericLiteral:
		return &Word{Pos: v.Pos, Value: v.Value}, true
	}
	return nil, false
}

// exprPos returns the source position of an expression node.
func exprPos(e Expr) Pos {
	switch n := e.(type) {
	case *Identifier:
		return n.Pos
	case *NumericLiteral:
		return n.Pos
	case *StringLiteral:
		return n.Pos
	case *BinaryExpr:
		return n.Pos
	case *UnaryExpr:
		return n.Pos
	case *ParenExpr:
		return n.Pos
	default:
		return Pos{}
	}
}

// classKeywords are the class-condition keywords.
var classKeywords = []string{"NUMERIC", "ALPHABETIC", "ALPHABETIC-LOWER", "ALPHABETIC-UPPER"}

// signKeywords are the sign-condition keywords.
var signKeywords = []string{"POSITIVE", "NEGATIVE", "ZERO"}

// parseCondition parses a condition: combinable conditions joined by OR (the
// lowest-precedence combinator, so it is the outermost loop). It is a
// recursive-descent sub-parser, like the arithmetic-expression parsers.
func parseCondition(p *parser) (Condition, error) {
	left, err := parseAndCondition(p)
	if err != nil {
		return nil, err
	}
	for {
		is, err := p.peekKeyword("OR")
		if err != nil {
			return nil, err
		}
		if !is {
			return left, nil
		}
		p.consume()
		right, err := parseAndCondition(p)
		if err != nil {
			return nil, err
		}
		left = &LogicalCondition{Pos: conditionPos(left), Op: "OR", Left: left, Right: right}
	}
}

// parseAndCondition parses combinable conditions joined by AND.
func parseAndCondition(p *parser) (Condition, error) {
	left, err := parseCombinable(p)
	if err != nil {
		return nil, err
	}
	for {
		is, err := p.peekKeyword("AND")
		if err != nil {
			return nil, err
		}
		if !is {
			return left, nil
		}
		p.consume()
		right, err := parseCombinable(p)
		if err != nil {
			return nil, err
		}
		left = &LogicalCondition{Pos: conditionPos(left), Op: "AND", Left: left, Right: right}
	}
}

// parseCombinable parses a combinable condition: an optional leading NOT applied
// to either a parenthesized condition or a simple condition.
func parseCombinable(p *parser) (Condition, error) {
	notTok, hasNot, err := p.acceptKeyword("NOT")
	if err != nil {
		return nil, err
	}

	cond, err := parseParenOrSimpleCondition(p)
	if err != nil {
		return nil, err
	}
	if hasNot {
		return &NotCondition{Pos: notTok.Pos, Cond: cond}, nil
	}
	return cond, nil
}

// parseParenOrSimpleCondition parses a parenthesized condition or a simple one.
func parseParenOrSimpleCondition(p *parser) (Condition, error) {
	paren, err := p.peekSymbol("(")
	if err != nil {
		return nil, err
	}
	if paren {
		lp, _ := p.expectSymbol("(")
		inner, err := parseCondition(p)
		if err != nil {
			return nil, err
		}
		if _, err := p.expectSymbol(")"); err != nil {
			return nil, err
		}
		return &ParenCondition{Pos: lp.Pos, Cond: inner}, nil
	}
	return parseSimpleCondition(p)
}

// parseSimpleCondition parses a relation, class, sign, or condition-name
// condition. It reads the left operand expression, an optional "IS" and "NOT",
// then dispatches on what follows: a class/sign keyword, a relational operator, or
// nothing (a bare condition-name reference).
func parseSimpleCondition(p *parser) (Condition, error) {
	left, err := parseExpr(p)
	if err != nil {
		return nil, err
	}
	return parseSimpleConditionFrom(p, left)
}

// parseSimpleConditionFrom finishes a relation, class, sign, or condition-name
// condition whose left operand expression left has already been parsed. It is the
// shared tail of parseSimpleCondition and the EVALUATE subject/object parser, which
// must pre-parse the operand to disambiguate the operand-vs-condition alternative.
func parseSimpleConditionFrom(p *parser, left Expr) (Condition, error) {
	pos := exprPos(left)

	if err := p.skipOptionalKeyword("IS"); err != nil {
		return nil, err
	}
	notTok, hasNot, err := p.acceptKeyword("NOT")
	if err != nil {
		return nil, err
	}

	if class, ok, err := p.acceptKeywordValue(classKeywords...); err != nil {
		return nil, err
	} else if ok {
		return &ClassCondition{Pos: pos, Operand: left, Not: hasNot, Class: class}, nil
	}

	if sign, ok, err := p.acceptKeywordValue(signKeywords...); err != nil {
		return nil, err
	} else if ok {
		return &SignCondition{Pos: pos, Operand: left, Not: hasNot, Sign: sign}, nil
	}

	op, found, err := parseRelationalOperator(p)
	if err != nil {
		return nil, err
	}
	if found {
		right, err := parseExpr(p)
		if err != nil {
			return nil, err
		}
		rel := &RelationCondition{Pos: pos, Left: left, Op: op, Right: right}
		if hasNot {
			return &NotCondition{Pos: notTok.Pos, Cond: rel}, nil
		}
		return rel, nil
	}

	if hasNot {
		// "NOT" with no relation/class/sign following is malformed.
		tok, _, _ := p.peek()
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenSymbol, TokenIdentifier}, Actual: tok}
	}
	if id, ok := left.(*Identifier); ok {
		return &ConditionNameCondition{Pos: pos, Name: id}, nil
	}
	tok, _, _ := p.peek()
	return nil, UnexpectedTokenError{Expected: []TokenType{TokenSymbol}, Actual: tok}
}

// parseRelationalOperator parses a relational operator and returns its canonical
// symbol form. Symbol operators (= < > <= >= <>) and the word forms GREATER
// [THAN], LESS [THAN], and EQUAL [TO] are recognized.
func parseRelationalOperator(p *parser) (string, bool, error) {
	if sym, err := p.peekSymbol("=", "<", ">", "<=", ">=", "<>"); err != nil {
		return "", false, err
	} else if sym {
		tok, _ := p.expectSymbol("=", "<", ">", "<=", ">=", "<>")
		return string(tok.Value), true, nil
	}

	if _, ok, err := p.acceptKeyword("GREATER"); err != nil {
		return "", false, err
	} else if ok {
		if err := p.skipOptionalKeyword("THAN"); err != nil {
			return "", false, err
		}
		return ">", true, nil
	}
	if _, ok, err := p.acceptKeyword("LESS"); err != nil {
		return "", false, err
	} else if ok {
		if err := p.skipOptionalKeyword("THAN"); err != nil {
			return "", false, err
		}
		return "<", true, nil
	}
	if _, ok, err := p.acceptKeyword("EQUAL"); err != nil {
		return "", false, err
	} else if ok {
		if err := p.skipOptionalKeyword("TO"); err != nil {
			return "", false, err
		}
		return "=", true, nil
	}
	return "", false, nil
}

// conditionFollowKeywords are the keywords that, after an operand, continue it into
// a condition: the relation words, IS/NOT, and the class and sign keywords.
var conditionFollowKeywords = slices.Concat(
	[]string{"IS", "NOT", "GREATER", "LESS", "EQUAL"}, classKeywords, signKeywords)

// isEvaluateConditionFollow reports whether the next token continues a preceding
// operand into a condition (a relational symbol or a condition-follow keyword). It
// is the lookahead that disambiguates the operand-vs-condition alternative in an
// EVALUATE subject or object.
func isEvaluateConditionFollow(p *parser) (bool, error) {
	if sym, err := p.peekSymbol("=", "<", ">", "<=", ">=", "<>"); err != nil {
		return false, err
	} else if sym {
		return true, nil
	}
	return p.peekKeyword(conditionFollowKeywords...)
}

// parseLeadingCondition parses a condition that begins unambiguously with "(" or a
// NOT keyword — the two starts that cannot be a bare operand — for the EVALUATE
// subject/object parsers. It reports false, consuming nothing, when the next token
// is neither.
func parseLeadingCondition(p *parser) (Condition, bool, error) {
	open, err := p.peekSymbol("(")
	if err != nil {
		return nil, false, err
	}
	not, err := p.peekKeyword("NOT")
	if err != nil {
		return nil, false, err
	}
	if !open && !not {
		return nil, false, nil
	}
	cond, err := parseCondition(p)
	if err != nil {
		return nil, false, err
	}
	return cond, true, nil
}

// parseConditionFromExpr finishes a condition whose left operand expression left has
// already been parsed, when a condition continuation follows it. It reports whether
// a condition was found; on false the caller keeps left as a bare operand.
func parseConditionFromExpr(p *parser, left Expr) (Condition, bool, error) {
	follow, err := isEvaluateConditionFollow(p)
	if err != nil {
		return nil, false, err
	}
	if !follow {
		return nil, false, nil
	}
	simple, err := parseSimpleConditionFrom(p, left)
	if err != nil {
		return nil, false, err
	}
	cond, err := parseConditionTail(p, simple)
	if err != nil {
		return nil, false, err
	}
	return cond, true, nil
}

// parseConditionTail climbs the AND/OR combinators from a pre-parsed combinable
// condition left, mirroring parseAndCondition and parseCondition so a condition
// begun from an already-parsed operand keeps the same precedence (AND before OR).
func parseConditionTail(p *parser, left Condition) (Condition, error) {
	for {
		is, err := p.peekKeyword("AND")
		if err != nil {
			return nil, err
		}
		if !is {
			break
		}
		p.consume()
		right, err := parseCombinable(p)
		if err != nil {
			return nil, err
		}
		left = &LogicalCondition{Pos: conditionPos(left), Op: "AND", Left: left, Right: right}
	}
	for {
		is, err := p.peekKeyword("OR")
		if err != nil {
			return nil, err
		}
		if !is {
			break
		}
		p.consume()
		right, err := parseAndCondition(p)
		if err != nil {
			return nil, err
		}
		left = &LogicalCondition{Pos: conditionPos(left), Op: "OR", Left: left, Right: right}
	}
	return left, nil
}

// conditionPos returns the source position of a condition node.
func conditionPos(c Condition) Pos {
	switch n := c.(type) {
	case *RelationCondition:
		return n.Pos
	case *ClassCondition:
		return n.Pos
	case *SignCondition:
		return n.Pos
	case *ConditionNameCondition:
		return n.Pos
	case *LogicalCondition:
		return n.Pos
	case *NotCondition:
		return n.Pos
	case *ParenCondition:
		return n.Pos
	default:
		return Pos{}
	}
}

// UnexpectedEndOfTokensError is returned when the parser needs another token
// but the stream is exhausted.
type UnexpectedEndOfTokensError struct {
	Expected []TokenType
}

// Error implements the [error] interface.
func (e UnexpectedEndOfTokensError) Error() string {
	return fmt.Sprintf("unexpected end of tokens, expected one of %v", e.Expected)
}

// UnexpectedTokenError is returned when the parser reads a token whose type is
// not one it expected.
type UnexpectedTokenError struct {
	Expected []TokenType
	Actual   Token
}

// Error implements the [error] interface.
func (e UnexpectedTokenError) Error() string {
	return fmt.Sprintf("unexpected token %s at line %d, column %d, expected one of %v", e.Actual, e.Actual.Pos.Line, e.Actual.Pos.Column, e.Expected)
}

// UnexpectedKeywordError is returned when the parser reads an identifier whose
// spelling is not one of the keywords it expected.
type UnexpectedKeywordError struct {
	Expected []string
	Actual   Token
}

// Error implements the [error] interface.
func (e UnexpectedKeywordError) Error() string {
	return fmt.Sprintf("unexpected keyword %q at line %d, column %d, expected one of %v", string(e.Actual.Value), e.Actual.Pos.Line, e.Actual.Pos.Column, e.Expected)
}

// InvalidLevelNumberError is returned when a data-description entry's
// level-number is not an integer 01–49, 66, 77, or 88.
type InvalidLevelNumberError struct {
	Pos   Pos
	Value string
}

// Error implements the [error] interface.
func (e InvalidLevelNumberError) Error() string {
	return fmt.Sprintf("invalid level-number %q at line %d, column %d, expected 01-49, 66, 77, or 88", e.Value, e.Pos.Line, e.Pos.Column)
}
