// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"iter"
	"unicode"
	"unicode/utf8"
)

// Pos represents the position of a token in the input.
type Pos struct {
	Line   int
	Column int
}

// Token represents a single lexical token in the COBOL source.
type Token struct {
	Pos   Pos
	Type  TokenType
	Value []byte
}

func (t Token) String() string {
	return fmt.Sprintf("%s(%s)", t.Type, t.Value)
}

// TokenType represents the type of a [Token].
type TokenType int

const (
	TokenComment    TokenType = iota // e.g. * in column 7, or *> inline comment
	TokenIdentifier                  // e.g. IDENTIFICATION, DIVISION, a data name
	TokenSymbol                      // e.g. ".", "(", ")"
	TokenString                      // e.g. "literal"
	TokenNumber                      // e.g. 123, 45.67
)

func (tt TokenType) String() string {
	switch tt {
	case TokenComment:
		return "Comment"
	case TokenIdentifier:
		return "Identifier"
	case TokenSymbol:
		return "Symbol"
	case TokenString:
		return "String"
	case TokenNumber:
		return "Number"
	default:
		panic(fmt.Sprintf("unknown token type: %d", tt))
	}
}

// TokenizeOption configures the tokenizer before it begins reading.
type TokenizeOption func(*tokenizer)

// WithDecimalComma selects DECIMAL-POINT IS COMMA mode: a comma is the decimal
// point inside numeric literals and a period is not. The clause that enables
// this mode lives in the ENVIRONMENT DIVISION; until the parser recognizes it
// and drives the tokenizer, callers that know the dialect can request the mode
// directly.
func WithDecimalComma() TokenizeOption {
	return func(t *tokenizer) { t.decimalPoint = ',' }
}

// Tokenize the COBOL source defined in the given reader.
//
// Tokens are produced lazily via [iter.Seq2] so the parser can consume one
// token at a time and so errors surface at the position where they occur.
func Tokenize(r io.Reader, opts ...TokenizeOption) iter.Seq2[Token, error] {
	return func(yield func(Token, error) bool) {
		t := &tokenizer{
			pos:          Pos{Line: 1, Column: 1},
			buf:          bufio.NewReader(r),
			decimalPoint: '.',
		}
		for _, opt := range opts {
			opt(t)
		}

		for action := tokenizeCOBOL; action != nil; {
			action = action(t, yield)
		}
	}
}

type tokenizer struct {
	// pos tracks the current position in the input for error reporting.
	pos Pos

	buf *bufio.Reader

	// decimalPoint is the rune that acts as the decimal point inside numeric
	// literals: '.' normally, or ',' under DECIMAL-POINT IS COMMA.
	decimalPoint rune
}

func (t *tokenizer) next() (rune, error) {
	r, _, err := t.buf.ReadRune()
	if err != nil {
		return 0, err
	}
	t.pos.Column++
	if r == '\n' {
		t.pos.Line++
		t.pos.Column = 1
	}
	return r, nil
}

func (t *tokenizer) backup(previousPos Pos) error {
	err := t.buf.UnreadRune()
	if err != nil {
		return err
	}
	t.pos = previousPos
	return nil
}

// peekByte returns the next unread byte without consuming it (so position is
// unchanged), reporting false at end of input or on any read error. Numeric
// punctuation and literal delimiters are all ASCII, so byte-level lookahead is
// enough to decide them. The number tokenizer peeks before consuming so it never
// has to put back a rune — [bufio.Reader.Peek] invalidates a later UnreadRune, so
// peeking and then [tokenizer.backup]-ing the same rune is not allowed.
func (t *tokenizer) peekByte() (byte, bool) {
	b, _ := t.buf.Peek(1)
	if len(b) == 0 {
		return 0, false
	}
	return b[0], true
}

// peekIsDigit reports whether the next unread byte is an ASCII digit.
func (t *tokenizer) peekIsDigit() bool {
	b, ok := t.peekByte()
	return ok && '0' <= b && b <= '9'
}

// peekDecimalPointDigit reports whether an unconsumed decimal point (the next
// byte) is followed by a digit — i.e. it is a decimal point and not a trailing
// separator period.
func (t *tokenizer) peekDecimalPointDigit() bool {
	b, _ := t.buf.Peek(2)
	return len(b) >= 2 && '0' <= b[1] && b[1] <= '9'
}

// peekExponentDigits reports whether an unconsumed exponent marker (the next
// byte, 'E'/'e') is followed by a well-formed exponent: one or more digits,
// optionally preceded by a single '+'/'-' sign.
func (t *tokenizer) peekExponentDigits() bool {
	b, _ := t.buf.Peek(3)
	if len(b) >= 2 && '0' <= b[1] && b[1] <= '9' {
		return true
	}
	if len(b) >= 3 && (b[1] == '+' || b[1] == '-') && '0' <= b[2] && b[2] <= '9' {
		return true
	}
	return false
}

// tokenizerAction is one step of the tokenizer state machine: it reads some
// runes, optionally calls yield to emit a [Token], and returns the next action
// to run. Returning nil ends iteration.
type tokenizerAction func(t *tokenizer, yield func(Token, error) bool) tokenizerAction

// yieldErrorOr handles error propagation in the tokenizer chain. A nil error
// continues with next; reaching the end of input ([io.EOF] or
// [io.ErrUnexpectedEOF]) terminates the stream cleanly; any other error is
// yielded before continuing (or stops if the consumer stops).
func yieldErrorOr(err error, next tokenizerAction) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		if err == nil {
			return next
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil
		}
		if !yield(Token{}, err) {
			return nil
		}
		return next
	}
}

// yieldTokenThen yields a token and continues with the next action.
func yieldTokenThen(tok Token, next tokenizerAction) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		if !yield(tok, nil) {
			return nil
		}
		return next
	}
}

// skipWhitespace consumes leading whitespace, then runs next.
func skipWhitespace(next tokenizerAction) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		for {
			pos := t.pos
			r, err := t.next()
			if err != nil {
				return yieldErrorOr(err, next)
			}
			if !unicode.IsSpace(r) {
				return yieldErrorOr(t.backup(pos), next)
			}
		}
	}
}

// tokenizeCOBOL is the entry-point action. It skips leading whitespace, then
// dispatches on the next rune to a specific sub-tokenizer. Empty (or
// whitespace-only) input produces no tokens; any rune that begins no known
// token class yields an [UnexpectedCharacterError].
//
// This free-format slice recognizes COBOL words, the separator period,
// alphanumeric literals, and numeric literals. Comments and the remaining
// symbols/operators are tokenized by later stories.
func tokenizeCOBOL(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
	return skipWhitespace(
		func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
			pos := t.pos
			r, err := t.next()
			if err != nil {
				return yieldErrorOr(err, nil)
			}

			switch {
			case r == '.':
				return yieldTokenThen(
					Token{Pos: pos, Type: TokenSymbol, Value: []byte{'.'}},
					tokenizeCOBOL,
				)
			case r == '"' || r == '\'':
				return tokenizeString(pos, r)
			case isDigit(r):
				return tokenizeNumber(pos, utf8.AppendRune(nil, r))
			case (r == '+' || r == '-') && t.peekIsDigit():
				// A sign begins a numeric literal only when contiguous with a
				// digit; otherwise it is an operator, tokenized by a later story.
				return tokenizeNumber(pos, utf8.AppendRune(nil, r))
			case isWordStart(r):
				return tokenizeWord(pos, r)
			default:
				yield(Token{}, UnexpectedCharacterError{Pos: pos, R: r})
				return nil
			}
		},
	)
}

// tokenizeWord accumulates a maximal run of COBOL word runes, beginning with the
// already-read first rune at start. A non-word rune is backed up so the next
// action re-reads it; end of input simply ends the word. All COBOL words emit as
// [TokenIdentifier] — the parser's keyword table distinguishes reserved words
// from user-defined names.
func tokenizeWord(start Pos, first rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		value := utf8.AppendRune(nil, first)
		for {
			pos := t.pos
			r, err := t.next()
			if err != nil {
				tok := Token{Pos: start, Type: TokenIdentifier, Value: value}
				return yieldTokenThen(tok, yieldErrorOr(err, nil))
			}
			if !isWordContinue(r) {
				tok := Token{Pos: start, Type: TokenIdentifier, Value: value}
				return yieldErrorOr(t.backup(pos), yieldTokenThen(tok, tokenizeCOBOL))
			}
			value = utf8.AppendRune(value, r)
		}
	}
}

// tokenizeNumber accumulates a numeric literal, emitting a single [TokenNumber]
// whose value is the raw lexeme. value already holds the consumed prefix — an
// optional sign and/or the first digit. The literal is integer digits, an
// optional decimal point (t.decimalPoint, honoring DECIMAL-POINT IS COMMA)
// followed by fractional digits, and an optional E/e exponent with an optional
// sign. Decoding the value is deferred to a later story.
//
// It peeks each candidate rune before consuming it, so a rune that does not
// belong to the literal is left in the buffer for the next dispatch rather than
// backed up: a decimal point not followed by a digit stays a separator period
// (SPEC: a trailing '.' ends a sentence), and a malformed exponent marker stays a
// COBOL word. (Peeking then backing up the same rune is impossible — see
// [tokenizer.peekByte].)
func tokenizeNumber(start Pos, value []byte) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		seenPoint := false
		seenExp := false
		emit := func(next tokenizerAction) tokenizerAction {
			return yieldTokenThen(Token{Pos: start, Type: TokenNumber, Value: value}, next)
		}
		for {
			b, ok := t.peekByte()
			if !ok {
				// End of input or a read error follows the number: emit it, then
				// let the next read surface the condition (a clean EOF stops the
				// stream, any other error propagates unchanged).
				return emit(func(t *tokenizer, _ func(Token, error) bool) tokenizerAction {
					_, err := t.next()
					return yieldErrorOr(err, nil)
				})
			}

			switch {
			case '0' <= b && b <= '9':
				r, _ := t.next()
				value = utf8.AppendRune(value, r)
			case rune(b) == t.decimalPoint && !seenPoint && !seenExp && t.peekDecimalPointDigit():
				seenPoint = true
				r, _ := t.next()
				value = utf8.AppendRune(value, r)
			case (b == 'E' || b == 'e') && !seenExp && t.peekExponentDigits():
				// The exponent admits no further decimal point.
				seenExp = true
				seenPoint = true
				r, _ := t.next()
				value = utf8.AppendRune(value, r)
				if sb, ok := t.peekByte(); ok && (sb == '+' || sb == '-') {
					sign, _ := t.next()
					value = utf8.AppendRune(value, sign)
				}
			default:
				return emit(tokenizeCOBOL)
			}
		}
	}
}

// tokenizeString accumulates an alphanumeric literal delimited by delim (a
// double or single quote), which has already been consumed at start. The raw
// lexeme — including both delimiters and any doubled-delimiter escapes — becomes
// the token value; a doubled delimiter is an embedded delimiter,
// not the close. End of input before the matching delimiter is an
// [UnterminatedStringError]; any other read error propagates unchanged. Decoding
// the escapes is deferred to a later story.
func tokenizeString(start Pos, delim rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		value := utf8.AppendRune(nil, delim)
		for {
			r, err := t.next()
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
					err = UnterminatedStringError{Pos: start}
				}
				yield(Token{}, err)
				return nil
			}
			value = utf8.AppendRune(value, r)
			if r == delim {
				if b, ok := t.peekByte(); ok && rune(b) == delim {
					// Doubled delimiter: an escaped delimiter, not the close.
					escaped, _ := t.next()
					value = utf8.AppendRune(value, escaped)
					continue
				}
				tok := Token{Pos: start, Type: TokenString, Value: value}
				return yieldTokenThen(tok, tokenizeCOBOL)
			}
		}
	}
}

// isWordStart reports whether r may begin a COBOL word. Word start is restricted
// to an ASCII letter in this slice so the dispatch stays unambiguous before
// numeric literals (which also begin with a digit) are tokenized.
func isWordStart(r rune) bool {
	return ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z')
}

// isDigit reports whether r is an ASCII decimal digit.
func isDigit(r rune) bool {
	return '0' <= r && r <= '9'
}

// isWordContinue reports whether r may appear after the first rune of a COBOL
// word: ASCII letters, digits, hyphen, and underscore (SPEC §"User-defined
// word"). The "must contain a letter" and "may not begin or end with a hyphen or
// underscore" rules are semantic concerns left to the parser.
func isWordContinue(r rune) bool {
	return isWordStart(r) || ('0' <= r && r <= '9') || r == '-' || r == '_'
}

// UnexpectedCharacterError is returned by the tokenizer when it encounters a
// character that no action expected.
type UnexpectedCharacterError struct {
	Pos Pos
	R   rune
}

// Error implements the [error] interface.
func (e UnexpectedCharacterError) Error() string {
	return fmt.Sprintf("unexpected character '%c' at line %d, column %d", e.R, e.Pos.Line, e.Pos.Column)
}

// UnterminatedStringError is returned by the tokenizer when an alphanumeric
// literal is not closed by its matching delimiter before end of input.
type UnterminatedStringError struct {
	Pos Pos // position of the opening delimiter
}

// Error implements the [error] interface.
func (e UnterminatedStringError) Error() string {
	return fmt.Sprintf("unterminated string literal starting at line %d, column %d", e.Pos.Line, e.Pos.Column)
}
