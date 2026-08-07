// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package picture

import (
	"fmt"
	"strconv"
	"unicode/utf8"
)

// maxRepeatCount bounds a parenthesized repeat count. Nothing in COBOL needs a
// larger one — the standard caps an elementary item well below it — and the
// bound is what stops a malformed lexeme such as "X(4000000000)" from asking
// for an unbounded allocation.
const maxRepeatCount = 65535

// scanAction is one step of the PICTURE symbol scan: it consumes some of the
// source, optionally appends to the scanner's symbol sequence, and returns the
// next action to run. Returning (nil, nil) ends the scan successfully; every
// error path returns nil for the next action so the driver loop stays monotone.
//
// This mirrors the action-loop shape the root package's tokenizer and parser
// use; the repeat count is the state that earns it, since "(5)" is only
// meaningful in terms of the symbol that preceded it.
type scanAction func(s *scanner) (scanAction, error)

// scanner holds the scan state: the source, the byte offset of the next rune,
// whether DECIMAL-POINT IS COMMA is in effect, and the symbols accumulated so
// far.
type scanner struct {
	src     string
	offset  int
	comma   bool
	symbols []Symbol
}

// scan expands src into its sequence of PICTURE symbols, one element per symbol
// occurrence: a repeat count is expanded ("9(3)" yields three [SymbolDigit]),
// and the two-character symbols CR and DB yield one element each.
//
// It validates only what lexing can see — that every character is a PICTURE
// symbol and that every repeat count is well formed. Where the symbols may
// appear relative to one another is [validate]'s job.
func scan(src string, comma bool) ([]Symbol, error) {
	s := &scanner{src: src, comma: comma}

	var err error
	for action := scanSymbol; action != nil && err == nil; {
		action, err = action(s)
	}
	if err != nil {
		return nil, err
	}
	return s.symbols, nil
}

// next consumes and returns the next rune along with the byte offset it started
// at. ok is false at end of input.
func (s *scanner) next() (r rune, at int, ok bool) {
	if s.offset >= len(s.src) {
		return 0, s.offset, false
	}
	r, w := utf8.DecodeRuneInString(s.src[s.offset:])
	at = s.offset
	s.offset += w
	return r, at, true
}

// peek reports the next rune without consuming it.
func (s *scanner) peek() (rune, bool) {
	if s.offset >= len(s.src) {
		return 0, false
	}
	r, _ := utf8.DecodeRuneInString(s.src[s.offset:])
	return r, true
}

// scanSymbol reads one PICTURE symbol and continues with [scanRepeat] so that a
// parenthesized repeat count following it is applied to that symbol.
func scanSymbol(s *scanner) (scanAction, error) {
	r, at, ok := s.next()
	if !ok {
		return nil, nil
	}

	sym, err := s.symbolFor(r, at)
	if err != nil {
		return nil, err
	}
	s.symbols = append(s.symbols, sym)
	return scanRepeat(sym, at), nil
}

// symbolFor maps one source rune to its [Symbol]. PICTURE symbols are
// case-insensitive, so the letter symbols are matched in either case. CR and DB
// are two characters and consume their second one here.
//
// The roles of '.' and ',' are swapped under DECIMAL-POINT IS COMMA: the symbol
// values name the role (decimal point, grouping separator), not the character
// that spelled it.
func (s *scanner) symbolFor(r rune, at int) (Symbol, error) {
	switch r {
	case '9':
		return SymbolDigit, nil
	case 'A', 'a':
		return SymbolAlphabetic, nil
	case 'X', 'x':
		return SymbolAlphanumeric, nil
	case 'Z', 'z':
		return SymbolZeroSuppress, nil
	case '*':
		return SymbolCheckProtect, nil
	case 'V', 'v':
		return SymbolImpliedDecimal, nil
	case 'S', 's':
		return SymbolSign, nil
	case 'P', 'p':
		return SymbolScaling, nil
	case 'B', 'b':
		return SymbolSpaceInsert, nil
	case '0':
		return SymbolZeroInsert, nil
	case '/':
		return SymbolSlashInsert, nil
	case '+':
		return SymbolPlus, nil
	case '-':
		return SymbolMinus, nil
	case '$':
		return SymbolCurrency, nil
	case '.':
		if s.comma {
			return SymbolGroupingSeparator, nil
		}
		return SymbolDecimalPoint, nil
	case ',':
		if s.comma {
			return SymbolDecimalPoint, nil
		}
		return SymbolGroupingSeparator, nil
	case 'C', 'c':
		return s.twoRuneSymbol(at, 'R', SymbolCredit)
	case 'D', 'd':
		return s.twoRuneSymbol(at, 'B', SymbolDebit)
	case '(':
		return SymbolUnknown, RepeatCountError{
			Source: s.src,
			Offset: at,
			Reason: "repeat count must follow a PICTURE symbol",
		}
	}
	return SymbolUnknown, UnexpectedSymbolError{Source: s.src, Offset: at, R: r}
}

// twoRuneSymbol completes a two-character symbol (CR, DB) whose first rune has
// already been consumed. want is the expected second rune, matched
// case-insensitively; the first rune alone is not a PICTURE symbol, so a
// mismatch is reported against the character that started it.
func (s *scanner) twoRuneSymbol(at int, want rune, sym Symbol) (Symbol, error) {
	r, ok := s.peek()
	if !ok || (r != want && r != want+('a'-'A')) {
		first, _ := utf8.DecodeRuneInString(s.src[at:])
		return SymbolUnknown, UnexpectedSymbolError{Source: s.src, Offset: at, R: first}
	}
	s.next()
	return sym, nil
}

// scanRepeat applies an optional parenthesized repeat count to sym, which has
// already been appended once: "9(3)" appends two further [SymbolDigit]. at is
// the offset of the symbol itself, so an unterminated count is reported against
// the symbol it was meant to repeat.
func scanRepeat(sym Symbol, at int) scanAction {
	return func(s *scanner) (scanAction, error) {
		r, ok := s.peek()
		if !ok || r != '(' {
			return scanSymbol, nil
		}
		s.next() // the '('

		start := s.offset
		for {
			r, rat, ok := s.next()
			if !ok {
				return nil, RepeatCountError{
					Source: s.src,
					Offset: at,
					Reason: fmt.Sprintf("unterminated repeat count after %s", sym),
				}
			}
			if r == ')' {
				break
			}
			if r < '0' || r > '9' {
				return nil, RepeatCountError{
					Source: s.src,
					Offset: rat,
					Reason: fmt.Sprintf("repeat count contains %q, expected digits", r),
				}
			}
		}

		digits := s.src[start : s.offset-1] // the ')' is one byte
		if digits == "" {
			return nil, RepeatCountError{Source: s.src, Offset: at, Reason: "empty repeat count"}
		}
		n, err := strconv.Atoi(digits)
		if err != nil || n < 1 || n > maxRepeatCount {
			return nil, RepeatCountError{
				Source: s.src,
				Offset: at,
				Reason: fmt.Sprintf("repeat count %s out of range, expected 1-%d", digits, maxRepeatCount),
			}
		}

		for i := 1; i < n; i++ {
			s.symbols = append(s.symbols, sym)
		}
		return scanSymbol, nil
	}
}

// EmptyPictureError is returned when the PICTURE character-string is empty or
// holds nothing but whitespace.
type EmptyPictureError struct {
	Source string
}

// Error implements the [error] interface.
func (e EmptyPictureError) Error() string {
	return fmt.Sprintf("invalid PICTURE %q: empty character-string", e.Source)
}

// UnexpectedSymbolError is returned when the PICTURE character-string holds a
// character that is not a PICTURE symbol. Offset is the byte offset of that
// character within Source.
type UnexpectedSymbolError struct {
	Source string
	Offset int
	R      rune
}

// Error implements the [error] interface.
func (e UnexpectedSymbolError) Error() string {
	return fmt.Sprintf("invalid PICTURE %q: unexpected character %q at offset %d", e.Source, e.R, e.Offset)
}

// RepeatCountError is returned when a parenthesized repeat count is malformed —
// unterminated, empty, non-numeric, out of range, or not preceded by a symbol.
// Offset is the byte offset within Source of the symbol or character the
// problem is reported against.
type RepeatCountError struct {
	Source string
	Offset int
	Reason string
}

// Error implements the [error] interface.
func (e RepeatCountError) Error() string {
	return fmt.Sprintf("invalid PICTURE %q: %s at offset %d", e.Source, e.Reason, e.Offset)
}
