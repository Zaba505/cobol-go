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
	"strings"
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
	TokenComment    TokenType = iota // e.g. *> inline or full-line comment
	TokenIdentifier                  // e.g. IDENTIFICATION, DIVISION, a data name
	TokenSymbol                      // e.g. ".", "(", ")"
	TokenString                      // e.g. "literal"
	TokenNumber                      // e.g. 123, 45.67
	TokenPicture                     // e.g. S9(4)V99, X(10) (the SPEC's PictureString)
	TokenDebug                       // a fixed-format column-7 'D'/'d' debugging line
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
	case TokenPicture:
		return "Picture"
	case TokenDebug:
		return "Debug"
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

// WithFixedFormat selects fixed-format ("reference format") source: each physical
// line is divided into column areas (sequence 1–6, indicator 7, Area A 8–11,
// Area B 12–72, identification 73–80) and the tokenizer becomes column-aware
// (SPEC §"Whitespace and Delimiters" → Fixed format). The default is free format,
// which has no column significance.
//
// The clause/directive that selects the source format (the >>SOURCE FORMAT
// directive, a command-line option, or a dialect default) is recognized by a
// later story; until then callers that know the dialect can request fixed format
// directly, the same way [WithDecimalComma] exposes DECIMAL-POINT IS COMMA.
func WithFixedFormat() TokenizeOption {
	return func(t *tokenizer) { t.fixed = true }
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

	// fixed selects fixed-format ("reference format") tokenization: column areas
	// carry meaning (see [WithFixedFormat]). When false the tokenizer is
	// free-format and ignores column positions beyond tracking them for errors.
	fixed bool
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

// fixedColumnInArea reports whether a lookahead byte at the given 0-based offset
// from the next unread position still falls within Area B (column ≤ 72) in fixed
// format. The peek helpers consult it so no lookahead considers a character in
// the ignored identification area (columns 73+) — e.g. a token at column 72 must
// not let "*>" / "**" / "<=" / a sign-digit / a decimal point / an exponent /
// a PICTURE decimal point be recognized using a character past column 72. In free
// format it is always true.
func (t *tokenizer) fixedColumnInArea(offset int) bool {
	return !t.fixed || t.pos.Column+offset <= fixedAreaBEndColumn
}

// peekByte returns the next unread byte without consuming it (so position is
// unchanged), reporting false at end of input or on any read error, and (in fixed
// format) when the next byte lies past column 72. Numeric punctuation and literal
// delimiters are all ASCII, so byte-level lookahead is enough to decide them. The
// number tokenizer peeks before consuming so it never has to put back a rune —
// [bufio.Reader.Peek] invalidates a later UnreadRune, so peeking and then
// [tokenizer.backup]-ing the same rune is not allowed.
func (t *tokenizer) peekByte() (byte, bool) {
	if !t.fixedColumnInArea(0) {
		return 0, false
	}
	b, _ := t.buf.Peek(1)
	if len(b) == 0 {
		return 0, false
	}
	return b[0], true
}

// peekRune returns the next unread rune without consuming it (so position is
// unchanged), reporting false at end of input and (in fixed format) when the next
// rune lies past column 72. Unlike [tokenizer.peekByte] it decodes a full UTF-8
// rune, so multi-byte Unicode whitespace is recognized the same way
// [skipWhitespace] recognizes it. As with peekByte, peeking and then
// [tokenizer.backup]-ing the same rune is not allowed.
func (t *tokenizer) peekRune() (rune, bool) {
	if !t.fixedColumnInArea(0) {
		return 0, false
	}
	b, _ := t.buf.Peek(utf8.UTFMax)
	if len(b) == 0 {
		return 0, false
	}
	r, _ := utf8.DecodeRune(b)
	return r, true
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
	return len(b) >= 2 && '0' <= b[1] && b[1] <= '9' && t.fixedColumnInArea(1)
}

// peekExponentDigits reports whether an unconsumed exponent marker (the next
// byte, 'E'/'e') is followed by a well-formed exponent: one or more digits,
// optionally preceded by a single '+'/'-' sign.
func (t *tokenizer) peekExponentDigits() bool {
	b, _ := t.buf.Peek(3)
	if len(b) >= 2 && '0' <= b[1] && b[1] <= '9' && t.fixedColumnInArea(1) {
		return true
	}
	if len(b) >= 3 && (b[1] == '+' || b[1] == '-') && '0' <= b[2] && b[2] <= '9' && t.fixedColumnInArea(2) {
		return true
	}
	return false
}

// peekIS reports whether the next unread bytes spell the reserved word IS
// (case-insensitive) followed by a word boundary. It is used after PIC/PICTURE
// to consume the optional IS before the PICTURE string. The lookahead is
// unambiguous: a PICTURE string never begins with I (not a PICTURE symbol), so
// IS followed by a boundary is always the keyword and never the start of the
// string.
func (t *tokenizer) peekIS() bool {
	b, _ := t.buf.Peek(3)
	if len(b) < 2 || (b[0] != 'I' && b[0] != 'i') || (b[1] != 'S' && b[1] != 's') {
		return false
	}
	if !t.fixedColumnInArea(1) {
		return false // the 'S' would fall in the ignored identification area
	}
	if !t.fixedColumnInArea(2) {
		return true // the character after "IS" is in the ignored area: a word boundary
	}
	return len(b) < 3 || !isWordContinue(rune(b[2]))
}

// peekPictureSeparatorPeriod reports whether an unconsumed '.' (the next byte)
// is a separator period — followed by whitespace or end of input — rather than
// the actual decimal point inside a PICTURE string (a '.' followed by another
// PICTURE character, e.g. the '.' in ZZ9.99). It decodes the rune after the '.'
// so multi-byte Unicode whitespace is recognized the same way [skipWhitespace]
// recognizes it. In fixed format a '.' at column 72 is always a separator period,
// since the character after it (column 73+) is ignored.
func (t *tokenizer) peekPictureSeparatorPeriod() bool {
	if !t.fixedColumnInArea(1) {
		return true // '.' at column 72: the rune after it is in the ignored area
	}
	b, _ := t.buf.Peek(1 + utf8.UTFMax)
	if len(b) < 2 {
		return true // '.' at end of input
	}
	r, _ := utf8.DecodeRune(b[1:])
	return unicode.IsSpace(r)
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
// This free-format slice recognizes COBOL words, alphanumeric literals, numeric
// literals, the separator period, the readability separators comma and
// semicolon, the parenthesis and colon separators, the special-character
// operators (+ - * / ** = < > <= >= <>), the concatenation operator & (the
// free-format literal-continuation mechanism), and the inline/full-line comment
// introducer *>. The compiler-directive introducer >> is tokenized by a later
// story; until then >> lexes as two > operators.
func tokenizeCOBOL(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
	if t.fixed {
		return tokenizeFixed
	}
	return skipWhitespace(
		func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
			pos := t.pos
			r, err := t.next()
			if err != nil {
				return yieldErrorOr(err, nil)
			}
			return dispatchRune(t, yield, pos, r)
		},
	)
}

// dispatchRune selects the sub-tokenizer for a content rune r already consumed at
// pos. It is the shared core of both reference formats' dispatch: the free-format
// entry point ([tokenizeCOBOL]) reaches it after skipping whitespace, and the
// fixed-format entry point ([tokenizeFixed]) reaches it after skipping the
// column areas. A rune that begins no known token class yields an
// [UnexpectedCharacterError].
//
// This recognizes COBOL words, alphanumeric literals, numeric literals, the
// separator period, the readability separators comma and semicolon, the
// parenthesis and colon separators, the special-character operators
// (+ - * / ** = < > <= >= <>), the concatenation operator & (the free-format
// literal-continuation mechanism), and the inline/full-line comment introducer
// *>. The compiler-directive introducer >> is tokenized by a later story; until
// then >> lexes as two > operators.
func dispatchRune(t *tokenizer, yield func(Token, error) bool, pos Pos, r rune) tokenizerAction {
	switch {
	case r == '.':
		return tokenizeSeparatorPeriod(pos)
	case r == '(' || r == ')' || r == ':' || r == '=' || r == '/' || r == '&':
		return yieldSymbol(pos, utf8.AppendRune(nil, r))
	case r == ',' || r == ';':
		return tokenizeSeparatorPunct(pos, r)
	case r == '"' || r == '\'':
		return tokenizeString(pos, r)
	case isDigit(r):
		return tokenizeNumber(pos, utf8.AppendRune(nil, r))
	case (r == '+' || r == '-') && t.peekIsDigit():
		// A sign begins a numeric literal only when contiguous with a
		// digit; otherwise it is an arithmetic operator (handled below).
		return tokenizeNumber(pos, utf8.AppendRune(nil, r))
	case r == '+' || r == '-':
		return yieldSymbol(pos, utf8.AppendRune(nil, r))
	case r == '*':
		// *> introduces a comment; a bare * is multiply and ** is
		// exponentiation, both handled by tokenizeOperator.
		if b, ok := t.peekByte(); ok && b == '>' {
			gt, _ := t.next()
			return tokenizeComment(pos, []byte{byte(r), byte(gt)})
		}
		return tokenizeOperator(pos, r)
	case r == '<' || r == '>':
		return tokenizeOperator(pos, r)
	case isWordStart(r):
		return tokenizeWord(pos, r)
	default:
		yield(Token{}, UnexpectedCharacterError{Pos: pos, R: r})
		return nil
	}
}

// Fixed-format column boundaries (1-based), per SPEC §"Whitespace and
// Delimiters" → Fixed format.
const (
	fixedIndicatorColumn  = 7  // the indicator area
	fixedAreaBStartColumn = 12 // first column of Area B (Area A is 8–11)
	fixedAreaBEndColumn   = 72 // last column scanned; columns 73+ are ignored
)

// tokenizeFixed is the fixed-format entry-point action. It advances past the
// non-content column areas — the sequence area (columns 1–6), the identification
// area (columns 73+), and ordinary whitespace within Area A/B — acting on the
// column-7 indicator of each physical line, then dispatches the next content rune
// through [dispatchRune] exactly as free format does.
//
// The indicator area (column 7) selects the line kind: '*' or '/' makes the whole
// line (columns 8–72) a [TokenComment]; 'D'/'d' makes it a [TokenDebug]; '-' is a
// continuation line whose join semantics live in the recognizers (they call
// [tokenizer.fixedAdvanceToContinuation] when a token reaches the column-72
// boundary), so at a token boundary it scans like a normal line; space (or any
// other character) is a normal line. A line shorter than 7 columns has no
// indicator and is likewise normal.
func tokenizeFixed(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
	for {
		pos := t.pos
		r, err := t.next()
		if err != nil {
			return yieldErrorOr(err, nil)
		}
		switch {
		case pos.Column < fixedIndicatorColumn:
			// Sequence area (columns 1–6): ignored; any character (including a
			// terminator ending a blank or short line) carries no meaning.
			continue
		case pos.Column == fixedIndicatorColumn:
			switch r {
			case '*', '/':
				return tokenizeFixedLineRest(pos, r, TokenComment)
			case 'D', 'd':
				return tokenizeFixedLineRest(pos, r, TokenDebug)
			default:
				// space, '-' (continuation at a token boundary — nothing to
				// join), or anything else: a normal line. Scan Area A/B content.
				continue
			}
		case pos.Column > fixedAreaBEndColumn:
			// Identification area (columns 73+): ignored.
			continue
		default:
			// Area A / Area B (columns 8–72).
			if unicode.IsSpace(r) {
				continue
			}
			return dispatchRune(t, yield, pos, r)
		}
	}
}

// tokenizeFixedLineRest captures the rest of a fixed-format indicator line as a
// single token of typ — a full-line comment ('*'/'/') or a debugging line
// ('D'/'d'). The indicator was already consumed at start (column 7); the token's
// value is that indicator followed by the line's columns 8–72, and its position
// is the indicator's (column 7), mirroring how free format keeps the *> marker in
// a comment's value so a fixed-format printer can later reconstruct it. Reading
// stops at the line terminator or column 73, whichever comes first (columns 73+
// are ignored). Fixed dispatch resumes on the next line.
func tokenizeFixedLineRest(start Pos, indicator rune, typ TokenType) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		value := utf8.AppendRune(nil, indicator)
		emit := func(next tokenizerAction) tokenizerAction {
			return yieldTokenThen(Token{Pos: start, Type: typ, Value: value}, next)
		}
		for {
			// Check the column-72 boundary before peeking: peekRune is
			// column-bounded (ok=false past column 72), so any bytes in the
			// ignored identification area (columns 73+) must resume tokenizeFixed
			// — which skips them and the terminator — rather than fall into the
			// genuine-EOF branch below and terminate the stream early.
			if t.pos.Column > fixedAreaBEndColumn {
				return emit(tokenizeFixed)
			}
			r, ok := t.peekRune()
			if !ok {
				return emit(func(t *tokenizer, _ func(Token, error) bool) tokenizerAction {
					_, err := t.next()
					return yieldErrorOr(err, nil)
				})
			}
			if r == '\n' || r == '\r' {
				return emit(tokenizeFixed)
			}
			r, _ = t.next()
			value = utf8.AppendRune(value, r)
		}
	}
}

// peekIsLineEnd reports whether the next unread rune is a physical line
// terminator ('\n' or '\r'). The fixed-format recognizers use it to detect a
// content line that ends short of column 72 with a token still open.
func (t *tokenizer) peekIsLineEnd() bool {
	r, ok := t.peekRune()
	return ok && (r == '\n' || r == '\r')
}

// fixedAdvanceToContinuation handles a fixed-format token still open at the end of
// a content line. It consumes the rest of the current physical line (through its
// terminator), then searches subsequent lines for the continuation line, skipping
// any intervening blank lines and full-line comment lines ('*'/'/' in column 7),
// as SPEC §"Line Continuation" requires. A '-' in column 7 marks the continuation
// line, which it consumes — leaving the reader at column 8 — and reports true. Any
// other indicator (a normal line, or a 'D'/'d' debugging line) is not a
// continuation: it reports false, leaving the reader at column 7 so [tokenizeFixed]
// handles that line normally. End of input also reports false.
//
// A skipped intervening comment line is consumed here, not emitted as a
// [TokenComment]; a debugging line breaks continuation rather than being skipped,
// because whether it is source depends on WITH DEBUGGING MODE, which the tokenizer
// cannot see. Both are vanishingly rare between continuation lines.
func (t *tokenizer) fixedAdvanceToContinuation() bool {
	if !t.fixedConsumeToLineEnd() {
		return false
	}
	for {
		// Skip the sequence area (columns 1–6). A terminator here marks a blank
		// or short line, which is skipped when finding the continuation line.
		blank := false
		for t.pos.Column < fixedIndicatorColumn {
			r, err := t.next()
			if err != nil {
				return false
			}
			if r == '\n' {
				blank = true
				break
			}
		}
		if blank {
			continue
		}
		// At the indicator column (7).
		r, ok := t.peekRune()
		if !ok {
			return false
		}
		switch r {
		case '\n', '\r':
			_, _ = t.next() // blank line through the indicator column
		case '-':
			_, _ = t.next() // consume the hyphen; the reader is now at column 8.
			return true
		case '*', '/':
			// Intervening full-line comment: skipped when finding the
			// continuation line (consumed, not emitted as a token).
			if !t.fixedConsumeToLineEnd() {
				return false
			}
		default:
			// A space indicator with no non-blank Area A/B content is an
			// all-blank record (the common 80-column form of a blank line):
			// skip it like any other blank line. A normal content line, a
			// 'D'/'d' debugging line, or any other indicator breaks
			// continuation — report false, leaving the reader at column 7.
			if unicode.IsSpace(r) && t.fixedRestOfLineBlank() {
				if !t.fixedConsumeToLineEnd() {
					return false
				}
				continue
			}
			return false
		}
	}
}

// fixedRestOfLineBlank reports whether the remainder of the current physical
// line — from the next unread rune through its terminator, considering only the
// Area A/B columns (≤ 72) — holds no non-whitespace character. It only peeks, so
// the reader stays put for a caller that must otherwise fall back to normal
// tokenization. Bytes in the ignored identification area (columns 73+) never
// count as content, so a record blank through column 72 is blank regardless of
// what follows. Used to tell an all-blank record (a skippable blank line) from a
// real content line when searching for a continuation line.
func (t *tokenizer) fixedRestOfLineBlank() bool {
	// From column 7 (the indicator) at most columns 7–72 matter: 66 columns,
	// each one ASCII byte in the common case. Peek generously so a multi-byte
	// rune near the boundary is still seen whole; the column guard below stops
	// the scan once it passes column 72.
	b, _ := t.buf.Peek((fixedAreaBEndColumn - fixedIndicatorColumn + 2) * utf8.UTFMax)
	col := t.pos.Column
	for i := 0; i < len(b); {
		if col > fixedAreaBEndColumn {
			return true // only the ignored identification area remains
		}
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			break // an incomplete rune at the end of the peek window
		}
		if r == '\n' || r == '\r' {
			return true // terminator reached with no content
		}
		if !unicode.IsSpace(r) {
			return false
		}
		i += size
		col++
	}
	return true // end of input (or peek window) reached with no content
}

// fixedConsumeToLineEnd consumes runes through the next line terminator ('\n'),
// reporting false at end of input before one is found.
func (t *tokenizer) fixedConsumeToLineEnd() bool {
	for {
		r, err := t.next()
		if err != nil {
			return false
		}
		if r == '\n' {
			return true
		}
	}
}

// fixedSkipToReopenDelim positions a continued alphanumeric literal at the
// character immediately after the matching re-opening delimiter on a continuation
// line. The reader is at column 8; Area A (columns 8–11) of a continuation line
// must be blank, so it is skipped, and the matching delimiter is sought in Area B
// (columns 12–72) — per SPEC §"Line Continuation". It consumes up to and including
// the first delim rune in Area B and reports true; it reports false — the literal
// is unterminated — if the line ends (terminator or column 73) before one appears.
func (t *tokenizer) fixedSkipToReopenDelim(delim rune) bool {
	for {
		if t.pos.Column > fixedAreaBEndColumn {
			return false
		}
		r, ok := t.peekRune()
		if !ok {
			return false
		}
		if r == '\n' || r == '\r' {
			return false
		}
		_, _ = t.next()
		// Area A is skipped; only an Area B delimiter re-opens the literal.
		if r == delim && t.pos.Column > fixedAreaBStartColumn {
			return true
		}
	}
}

// fixedSkipToAreaBContent positions a continued word or numeric literal at the
// character at which it resumes. The reader is at column 8; Area A (columns 8–11)
// of a continuation line must be blank, so it is skipped, and resumption is at the
// first non-blank character of Area B (columns 12–72) — per SPEC §"Line
// Continuation". It stops at that character, at the line terminator, or at
// column 73.
func (t *tokenizer) fixedSkipToAreaBContent() {
	for {
		if t.pos.Column > fixedAreaBEndColumn {
			return
		}
		r, ok := t.peekRune()
		if !ok {
			return
		}
		if r == '\n' || r == '\r' {
			return
		}
		if t.pos.Column >= fixedAreaBStartColumn && !unicode.IsSpace(r) {
			return
		}
		_, _ = t.next()
	}
}

// fixedContinueLiteral handles a fixed-format alphanumeric literal that reached
// the end of a content line without its closing delimiter. Per SPEC every column
// through 72 belongs to the literal, so it first pads value with spaces for any
// columns missing on a short line; then it advances to the continuation line
// (column 7 == '-') and skips to just after the matching re-opening delimiter in
// its Area B, leaving the reader positioned to resume the literal. It reports
// false when there is no valid continuation (the literal is unterminated).
func (t *tokenizer) fixedContinueLiteral(value *[]byte, delim rune) bool {
	for c := t.pos.Column; c <= fixedAreaBEndColumn; c++ {
		*value = append(*value, ' ')
	}
	if !t.fixedAdvanceToContinuation() {
		return false
	}
	return t.fixedSkipToReopenDelim(delim)
}

// yieldSymbol emits a [TokenSymbol] carrying value at pos, then continues at the
// dispatch entry point.
func yieldSymbol(pos Pos, value []byte) tokenizerAction {
	return yieldTokenThen(Token{Pos: pos, Type: TokenSymbol, Value: value}, tokenizeCOBOL)
}

// tokenizeOperator emits a special-character operator, greedily matching the
// two-character forms (** <= >= <>) before their single-character prefixes
// (SPEC §"Symbols and Operators"). first has already been consumed at start.
// The compiler-directive introducer >> is deferred to a later story, so a bare
// > followed by > emits two separate > operators.
func tokenizeOperator(start Pos, first rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		value := utf8.AppendRune(nil, first)
		switch first {
		case '*': // ** exponentiation
			if b, ok := t.peekByte(); ok && b == '*' {
				r, _ := t.next()
				value = utf8.AppendRune(value, r)
			}
		case '<': // <= or <>
			if b, ok := t.peekByte(); ok && (b == '=' || b == '>') {
				r, _ := t.next()
				value = utf8.AppendRune(value, r)
			}
		case '>': // >=
			if b, ok := t.peekByte(); ok && b == '=' {
				r, _ := t.next()
				value = utf8.AppendRune(value, r)
			}
		}
		return yieldSymbol(start, value)
	}
}

// tokenizeSeparatorPunct handles the readability separators ',' and ';', already
// consumed at start. Per SPEC they are separators only when immediately followed
// by whitespace or end of input, and are then consumed like whitespace (no token
// emitted) — so "(2, 3)" and "(2 3)" tokenize identically. A ',' or ';' followed
// by anything else is not a separator and yields an [UnexpectedCharacterError].
//
// Under DECIMAL-POINT IS COMMA the separator comma is unavailable — a ',' is a
// decimal point there (a ',' between digits is consumed by [tokenizeNumber]), so
// a ',' reaching here is never a separator and is rejected; list items must be
// separated by a semicolon or space. The semicolon stays a separator in either
// mode. (SPEC §"Whitespace and Delimiters".)
func tokenizeSeparatorPunct(start Pos, r rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		if r == ',' && t.decimalPoint == ',' {
			yield(Token{}, UnexpectedCharacterError{Pos: start, R: r})
			return nil
		}
		// A full rune is decoded so multi-byte Unicode whitespace counts as a
		// boundary, consistent with skipWhitespace.
		if next, ok := t.peekRune(); ok && !unicode.IsSpace(next) {
			yield(Token{}, UnexpectedCharacterError{Pos: start, R: r})
			return nil
		}
		// A valid separator carries no meaning beyond word separation: emit
		// nothing and let the next dispatch skip the following whitespace.
		return tokenizeCOBOL
	}
}

// tokenizeSeparatorPeriod handles a '.' already consumed at start. Per SPEC
// (§"Whitespace and Delimiters") a period is a separator period — the sentence
// and entry terminator, emitted as a [TokenSymbol] — only when immediately
// followed by whitespace or end of input. A '.' inside a numeric literal (3.14)
// or a PICTURE string (ZZ9.99) is consumed by [tokenizeNumber] /
// [tokenizePictureString] before it can reach here; a '.' followed by anything
// else (A.B, 5.X) is malformed and yields an [UnexpectedCharacterError]. Under
// DECIMAL-POINT IS COMMA a '.' is never a decimal point, so 3.14 reaches here as
// a stray period after the number 3 and is likewise rejected.
func tokenizeSeparatorPeriod(start Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		// A full rune is decoded so multi-byte Unicode whitespace counts as a
		// boundary, consistent with skipWhitespace.
		if next, ok := t.peekRune(); ok && !unicode.IsSpace(next) {
			yield(Token{}, UnexpectedCharacterError{Pos: start, R: '.'})
			return nil
		}
		return yieldSymbol(start, []byte("."))
	}
}

// tokenizeWord accumulates a maximal run of COBOL word runes, beginning with the
// already-read first rune at start. A non-word rune is backed up so the next
// action re-reads it; end of input simply ends the word. All COBOL words emit as
// [TokenIdentifier] — the parser's keyword table distinguishes reserved words
// from user-defined names.
//
// In fixed format a word that runs to the column-72 boundary either ends there or,
// when the next line is a continuation line (column 7 == '-'), resumes at the
// first non-blank character of that line's Area B with no intervening space (SPEC
// §"Line Continuation" → Words and numeric literals).
func tokenizeWord(start Pos, first rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		value := utf8.AppendRune(nil, first)
		for {
			if t.fixed && t.pos.Column > fixedAreaBEndColumn {
				if t.fixedAdvanceToContinuation() {
					t.fixedSkipToAreaBContent()
					continue
				}
				tok := Token{Pos: start, Type: TokenIdentifier, Value: value}
				return yieldTokenThen(tok, nextAfterWord(value))
			}
			pos := t.pos
			r, err := t.next()
			if err != nil {
				tok := Token{Pos: start, Type: TokenIdentifier, Value: value}
				return yieldTokenThen(tok, yieldErrorOr(err, nil))
			}
			if !isWordContinue(r) {
				tok := Token{Pos: start, Type: TokenIdentifier, Value: value}
				return yieldErrorOr(t.backup(pos), yieldTokenThen(tok, nextAfterWord(value)))
			}
			value = utf8.AppendRune(value, r)
		}
	}
}

// nextAfterWord picks the action to run after a COBOL word is emitted. The word
// PIC/PICTURE (case-insensitive) switches the tokenizer into PICTURE-scanning
// mode (SPEC §"PICTURE Character-Strings"); every other word resumes the normal
// dispatch.
func nextAfterWord(value []byte) tokenizerAction {
	if s := string(value); strings.EqualFold(s, "PIC") || strings.EqualFold(s, "PICTURE") {
		return tokenizePictureClause
	}
	return tokenizeCOBOL
}

// tokenizePictureClause runs after a PIC/PICTURE word. It skips whitespace,
// consumes an optional IS reserved word — emitted as its own [TokenIdentifier],
// source case preserved, since the grammar is ( "PICTURE" | "PIC" ) [ "IS" ]
// PictureString and the parser expects IS as a separate token — then scans the
// PICTURE character-string as one token starting at the current position.
func tokenizePictureClause(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
	return skipWhitespace(func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		pos := t.pos
		if t.peekIS() {
			r1, _ := t.next()
			r2, _ := t.next()
			isTok := Token{Pos: pos, Type: TokenIdentifier, Value: utf8.AppendRune(utf8.AppendRune(nil, r1), r2)}
			return yieldTokenThen(isTok, skipWhitespace(func(t *tokenizer, _ func(Token, error) bool) tokenizerAction {
				return tokenizePictureString(t.pos)
			}))
		}
		return tokenizePictureString(pos)
	})
}

// tokenizePictureString accumulates a PICTURE character-string into a single
// [TokenPicture] whose value is the raw lexeme (case preserved; symbol and
// category validation are left to the parser — SPEC §Semantics). The run begins
// at start and stops at a separator: whitespace, or a separator period (a '.'
// followed by whitespace or end of input). A '.' followed by another PICTURE
// character is the actual decimal point and stays in the string; ',', '(', ')',
// digits, '$', and the sign/insertion symbols are likewise consumed, so the
// repeat count (n) is part of the token.
//
// Like [tokenizeNumber] it peeks each candidate rune before consuming it, so a
// terminating rune is left in the buffer for the next dispatch rather than
// backed up (peeking then backing up the same rune is impossible — see
// [tokenizer.peekByte]). An empty run emits no token, so a malformed PIC. yields
// PIC then the separator period rather than an empty PICTURE token.
func tokenizePictureString(start Pos) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		var value []byte
		emit := func(next tokenizerAction) tokenizerAction {
			if len(value) == 0 {
				return next
			}
			return yieldTokenThen(Token{Pos: start, Type: TokenPicture, Value: value}, next)
		}
		for {
			if t.fixed && t.pos.Column > fixedAreaBEndColumn {
				// Fixed format: the PICTURE string ends at the column-72 boundary
				// (columns 73+ are ignored). PICTURE strings are short and fit on
				// one line, so column-7 continuation of a PICTURE string is not
				// supported.
				return emit(tokenizeCOBOL)
			}
			r, ok := t.peekRune()
			if !ok {
				// End of input or a read error follows the string: emit it, then
				// let the next read surface the condition (a clean EOF stops the
				// stream, any other error propagates unchanged).
				return emit(func(t *tokenizer, _ func(Token, error) bool) tokenizerAction {
					_, err := t.next()
					return yieldErrorOr(err, nil)
				})
			}
			if unicode.IsSpace(r) {
				return emit(tokenizeCOBOL)
			}
			if r == '.' && t.peekPictureSeparatorPeriod() {
				return emit(tokenizeCOBOL)
			}
			r, _ = t.next()
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
			if t.fixed && t.pos.Column > fixedAreaBEndColumn {
				// Fixed format: a numeric literal that runs to the column-72
				// boundary either ends there or resumes on a continuation line,
				// like a word (SPEC §"Line Continuation").
				if t.fixedAdvanceToContinuation() {
					t.fixedSkipToAreaBContent()
					continue
				}
				return emit(tokenizeCOBOL)
			}
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
//
// In fixed format a literal that reaches the column-72 boundary (or the end of a
// short content line) without its closing delimiter is continued on the next line
// (column 7 == '-'): every column through 72 — including trailing spaces —
// belongs to the literal, and it resumes at the character after the matching
// re-opening delimiter in the continuation line's Area B (SPEC §"Line
// Continuation" → Alphanumeric literals). The emitted value is the joined lexeme:
// the opening delimiter, all content, and the closing delimiter, as if the
// literal were written on one line. Missing the continuation is an
// [UnterminatedStringError].
func tokenizeString(start Pos, delim rune) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		value := utf8.AppendRune(nil, delim)
		for {
			if t.fixed && (t.pos.Column > fixedAreaBEndColumn || t.peekIsLineEnd()) {
				if !t.fixedContinueLiteral(&value, delim) {
					yield(Token{}, UnterminatedStringError{Pos: start})
					return nil
				}
				continue
			}
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
				// A doubled delimiter is an escaped delimiter, not the close. In
				// fixed format the second delimiter must still be within Area B; a
				// delimiter spilling into column 73+ is ignored, so the literal
				// closes here.
				if b, ok := t.peekByte(); ok && rune(b) == delim &&
					(!t.fixed || t.pos.Column <= fixedAreaBEndColumn) {
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

// tokenizeComment accumulates a free-format inline or full-line comment into a
// single [TokenComment]. The comment is introduced by *> (SPEC §Comments) and
// runs to the end of the physical line; value already holds the *> introducer,
// consumed at start. The raw lexeme — the *> marker and the comment text, case
// and spacing preserved — becomes the token value, so the printer can reproduce
// it verbatim. This mirrors go/ast's Comment.Text (markers kept, terminator
// excluded) and is consistent with how [tokenizeString] keeps its delimiters and
// [tokenizePictureString] keeps its raw lexeme.
//
// Like [tokenizePictureString] it peeks each candidate rune before consuming it,
// stopping at the line terminator ('\n' or '\r') without consuming it, so the
// next dispatch skips it as whitespace. value always holds at least *>, so the
// token is never empty. At end of input the comment is emitted, then the next
// read surfaces the condition (a clean EOF stops the stream).
func tokenizeComment(start Pos, value []byte) tokenizerAction {
	return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
		emit := func(next tokenizerAction) tokenizerAction {
			return yieldTokenThen(Token{Pos: start, Type: TokenComment, Value: value}, next)
		}
		for {
			if t.fixed && t.pos.Column > fixedAreaBEndColumn {
				// Fixed format: an inline *> comment ends at the column-72
				// boundary; columns 73+ are the ignored identification area.
				return emit(tokenizeCOBOL)
			}
			r, ok := t.peekRune()
			if !ok {
				return emit(func(t *tokenizer, _ func(Token, error) bool) tokenizerAction {
					_, err := t.next()
					return yieldErrorOr(err, nil)
				})
			}
			if r == '\n' || r == '\r' {
				return emit(tokenizeCOBOL)
			}
			r, _ = t.next()
			value = utf8.AppendRune(value, r)
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
