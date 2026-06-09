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
)

// File is the root of the COBOL abstract syntax tree.
type File struct {
	// Divisions holds the top-level divisions of a COBOL program in source
	// order. It is the placeholder field for the scaffold; the empty input
	// parses to a zero-value *File (nil Divisions).
	Divisions []Division
}

// Division is implemented by the concrete COBOL division AST nodes
// (IDENTIFICATION, ENVIRONMENT, DATA, PROCEDURE).
type Division interface {
	division()
}

// Type is implemented by the concrete COBOL type/value AST nodes.
type Type interface {
	cobol()
}

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

// parserAction is one step of the parser state machine, generic over the AST
// node T currently being built. Returning (nil, nil) completes successfully;
// returning (nil, err) terminates with an error.
type parserAction[T any] func(p *parser, t T) (parserAction[T], error)

// parseFile is the top-level entry action. It is a stub that completes
// immediately, so empty input parses to a zero-value *File. The implementer
// dispatches into the division parsers here, each via its own inner action
// loop (see CLAUDE.md).
func parseFile(p *parser, f *File) (parserAction[*File], error) {
	return nil, nil
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
