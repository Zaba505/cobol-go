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
// PROCEDURE keyword; Statements are its sentences in source order.
type ProcedureDivision struct {
	Pos        Pos
	Statements []Statement
}

func (*ProcedureDivision) division() {}

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

// valueNode wraps an identifier or alphanumeric-literal token as the
// corresponding value AST node.
func valueNode(tok Token) Type {
	if tok.Type == TokenString {
		return &StringLiteral{Pos: tok.Pos, Value: string(tok.Value)}
	}
	return &Word{Pos: tok.Pos, Value: string(tok.Value)}
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
// header keyword kw. It is the division dispatch point; DATA is recognized only
// as future work and is not yet implemented.
func dispatchDivision(kw Token) parserAction[*Program] {
	switch {
	case keywordIs(kw, "IDENTIFICATION", "ID"):
		return parseIdentificationDivision(kw)
	case keywordIs(kw, "ENVIRONMENT"):
		return parseEnvironmentDivision(kw)
	case keywordIs(kw, "PROCEDURE"):
		return parseProcedureDivision(kw)
	default:
		return func(_ *parser, _ *Program) (parserAction[*Program], error) {
			return nil, unexpectedKeyword(kw, "IDENTIFICATION", "ID", "ENVIRONMENT", "PROCEDURE")
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
	if _, err := p.expect(TokenSymbol); err != nil {
		return nil, err
	}
	return parseProcedureBody, nil
}

// parseProcedureBody reads one verb and dispatches to its statement parser,
// looping until the token stream is exhausted.
func parseProcedureBody(p *parser, _ *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
	tok, err, ok := p.advance()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	switch {
	case keywordIs(tok, "DISPLAY"):
		return parseDisplayStatement(tok), nil
	case keywordIs(tok, "STOP"):
		return parseStopStatement(tok), nil
	default:
		return nil, unexpectedKeyword(tok, "DISPLAY", "STOP")
	}
}

// parseDisplayStatement parses a DISPLAY statement whose verb kw has already been
// read, collecting operands up to the separator period. Only literal operands
// are recognized in this slice; UPON and NO ADVANCING are deferred.
func parseDisplayStatement(kw Token) parserAction[*ProcedureDivision] {
	return func(p *parser, div *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
		stmt := &DisplayStatement{Pos: kw.Pos}
		for {
			tok, err := p.expect(TokenString, TokenSymbol)
			if err != nil {
				return nil, err
			}
			if tok.Type == TokenSymbol {
				break
			}
			stmt.Operands = append(stmt.Operands, valueNode(tok))
		}
		div.Statements = append(div.Statements, stmt)
		return parseProcedureBody, nil
	}
}

// parseStopStatement parses a STOP RUN statement whose verb kw has already been
// read. The STOP <literal> form is deferred to a later story.
func parseStopStatement(kw Token) parserAction[*ProcedureDivision] {
	return func(p *parser, div *ProcedureDivision) (parserAction[*ProcedureDivision], error) {
		if _, err := p.expectKeyword("RUN"); err != nil {
			return nil, err
		}
		if _, err := p.expect(TokenSymbol); err != nil {
			return nil, err
		}
		div.Statements = append(div.Statements, &StopStatement{Pos: kw.Pos, Run: true})
		return parseProcedureBody, nil
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
