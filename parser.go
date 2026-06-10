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
// position of the program's first division header keyword.
type Program struct {
	Pos       Pos
	Divisions []Division
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
// PROCEDURE keyword. Its body is either a sequence of paragraphs (Paragraphs,
// with Sections nil) or a sequence of sections (Sections, with Paragraphs nil) —
// the two forms are mutually exclusive. Statements that appear directly under the
// division header, before any paragraph-name, form an anonymous leading paragraph
// (a [Paragraph] with a nil Name).
type ProcedureDivision struct {
	Pos        Pos
	Paragraphs []*Paragraph
	Sections   []*Section
}

func (*ProcedureDivision) division() {}

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
// keyword; Operands are the values to display.
type DisplayStatement struct {
	Pos      Pos
	Operands []Type
}

func (*DisplayStatement) statement() {}

// StopStatement is a STOP statement. Pos is the position of the STOP keyword;
// Run reports the STOP RUN form.
type StopStatement struct {
	Pos Pos
	Run bool
}

func (*StopStatement) statement() {}

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

// NumericLiteral is a numeric literal used as a value, e.g. a VALUE or OCCURS
// operand. Pos is the position of the literal; Value is its raw lexeme (sign,
// digits, and decimal point preserved). Decoding the value is deferred to a
// later story, mirroring [StringLiteral].
type NumericLiteral struct {
	Pos   Pos
	Value string
}

func (*NumericLiteral) cobol() {}

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
	// peeked holds a token read by peek but not yet consumed, giving the parser
	// one-token lookahead (the go/parser current-token model this package
	// mirrors). advance drains it before pulling from next.
	peeked *peekedToken
}

// peekedToken is a single (Token, error, ok) triple buffered by peek.
type peekedToken struct {
	tok Token
	err error
	ok  bool
}

// advance returns the next (Token, error, ok) triple, draining the peek buffer
// first so a peeked token is consumed exactly once.
func (p *parser) advance() (Token, error, bool) {
	if p.peeked != nil {
		pk := p.peeked
		p.peeked = nil
		return pk.tok, pk.err, pk.ok
	}
	return p.next()
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

	prog := &Program{Pos: tok.Pos}
	for action := dispatchDivision(tok); action != nil && err == nil; {
		action, err = action(p, prog)
	}
	if err != nil {
		return nil, err
	}

	f.Programs = append(f.Programs, prog)
	return parseFile, nil
}

// dispatchDivision returns the division parser for the already-read division
// header keyword kw. IDENTIFICATION/ID, ENVIRONMENT, DATA, and PROCEDURE are
// dispatched to their parsers; any other keyword falls through to the default and
// is reported as an unexpected division header.
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
	default:
		return func(_ *parser, _ *Program) (parserAction[*Program], error) {
			return nil, unexpectedKeyword(kw, "IDENTIFICATION", "ID", "ENVIRONMENT", "DATA", "PROCEDURE")
		}
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

// parseProcedureDivision parses the PROCEDURE DIVISION whose header keyword kw
// has already been read. For this slice the procedure division is terminal: its
// body runs to end of input. The USING/RETURNING phrases, DECLARATIVES, and
// END PROGRAM are deferred to later stories.
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
		return nil, nil
	}
}

// parseProcedureHeader consumes "DIVISION" "." after the PROCEDURE keyword.
func parseProcedureHeader(p *parser, _ *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
	if _, err := p.expectKeyword("DIVISION"); err != nil {
		return nil, err
	}
	if _, err := p.expectPeriod(); err != nil {
		return nil, err
	}
	return parseProcedureBody, nil
}

// parseProcedureBody parses the procedure body. For this slice the body is a
// single anonymous paragraph of sentences (named paragraphs and sections are
// added by a later step); it runs to end of input.
func parseProcedureBody(p *parser, div *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	para := &Paragraph{Pos: tok.Pos}
	for action := parseSentenceOpt; action != nil && err == nil; {
		action, err = action(p, para)
	}
	if err != nil {
		return nil, err
	}

	div.Paragraphs = append(div.Paragraphs, para)
	return nil, nil
}

// parseSentenceOpt parses one sentence — a statement list terminated by a
// separator period — and appends it to para, then returns itself to parse the
// next sentence. It ends the paragraph at end of input.
func parseSentenceOpt(p *parser, para *Paragraph) (parserAction[*Paragraph], error) {
	tok, err, ok := p.peek()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	sent := &Sentence{Pos: tok.Pos}
	stmts, err := parseStatementList(p, stopAtSentenceEnd)
	if err != nil {
		return nil, err
	}
	if len(stmts) == 0 {
		// A separator period with no preceding statement is an empty sentence; a
		// statement verb was required where tok (the period) appears.
		return nil, UnexpectedTokenError{Expected: []TokenType{TokenIdentifier}, Actual: tok}
	}
	if _, err := p.expectPeriod(); err != nil {
		return nil, err
	}
	sent.Statements = stmts
	para.Sentences = append(para.Sentences, sent)
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
	case keywordIs(tok, "STOP"):
		return parseStopStatement(p, tok)
	default:
		return nil, unexpectedKeyword(tok, "DISPLAY", "STOP")
	}
}

// parseDisplayStatement parses a DISPLAY statement whose verb kw has already been
// read, collecting operands. Only literal operands are recognized in this slice;
// identifier operands, UPON, and NO ADVANCING are added by a later step.
func parseDisplayStatement(p *parser, kw Token) (Statement, error) {
	stmt := &DisplayStatement{Pos: kw.Pos}
	for {
		tok, err, ok := p.peek()
		if err != nil {
			return nil, err
		}
		if !ok || tok.Type != TokenString {
			break
		}
		p.consume()
		stmt.Operands = append(stmt.Operands, valueNode(tok))
	}
	return stmt, nil
}

// parseStopStatement parses a STOP RUN statement whose verb kw has already been
// read. The STOP <literal> form is added by a later step.
func parseStopStatement(p *parser, kw Token) (Statement, error) {
	if _, err := p.expectKeyword("RUN"); err != nil {
		return nil, err
	}
	return &StopStatement{Pos: kw.Pos, Run: true}, nil
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
