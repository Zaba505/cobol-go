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
}

// expect pulls the next token and requires its type to be one of types,
// returning [UnexpectedEndOfTokensError] if the stream is exhausted or
// [UnexpectedTokenError] if the type does not match.
func (p *parser) expect(types ...TokenType) (Token, error) {
	tok, err, ok := p.next()
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
	tok, err, ok := p.next()
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
// header keyword kw. It is the division dispatch point; ENVIRONMENT and DATA are
// recognized only as future work and are not yet implemented.
func dispatchDivision(kw Token) parserAction[*Program] {
	switch {
	case keywordIs(kw, "IDENTIFICATION", "ID"):
		return parseIdentificationDivision(kw)
	case keywordIs(kw, "PROCEDURE"):
		return parseProcedureDivision(kw)
	default:
		return func(_ *parser, _ *Program) (parserAction[*Program], error) {
			return nil, UnexpectedKeywordError{
				Expected: []string{"IDENTIFICATION", "ID", "PROCEDURE"},
				Actual:   kw,
			}
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

		tok, err, ok := p.next()
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
	tok, err, ok := p.next()
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
		return nil, UnexpectedKeywordError{Expected: []string{"DISPLAY", "STOP"}, Actual: tok}
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
	return fmt.Sprintf("unexpected token %s at line %d, column %d, expected one of %v", e.Actual, e.Actual.Pos.Line, e.Actual.Pos.Column, e.Expected)
}
