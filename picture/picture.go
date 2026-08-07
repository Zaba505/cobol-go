// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package picture

import (
	"fmt"
	"strings"
)

// Category is the category of a data item, derived from the set of symbols its
// PICTURE character-string contains (root SPEC.md, Semantics: "PICTURE
// determines category"). It governs what may be moved into the item and how it
// prints.
type Category int

const (
	// CategoryUnknown is the zero value; no valid PICTURE derives it.
	CategoryUnknown Category = iota
	// CategoryNumeric is a picture of 9 with optional S, V, and P: a number.
	CategoryNumeric
	// CategoryAlphabetic is a picture of A alone: letters and spaces.
	CategoryAlphabetic
	// CategoryAlphanumeric is a picture containing X, or mixing A and 9.
	CategoryAlphanumeric
	// CategoryNumericEdited is a picture of digit positions plus editing
	// symbols (Z * , . + - CR DB $ B 0 /).
	CategoryNumericEdited
	// CategoryAlphanumericEdited is a picture of A or X plus the simple
	// insertion symbols B, 0, and /.
	CategoryAlphanumericEdited
)

// String implements the [fmt.Stringer] interface.
func (c Category) String() string {
	switch c {
	case CategoryNumeric:
		return "numeric"
	case CategoryAlphabetic:
		return "alphabetic"
	case CategoryAlphanumeric:
		return "alphanumeric"
	case CategoryNumericEdited:
		return "numeric-edited"
	case CategoryAlphanumericEdited:
		return "alphanumeric-edited"
	}
	return "unknown"
}

// Symbol is one PICTURE symbol. The values name the symbol's role rather than
// the character that spelled it, which is what lets DECIMAL-POINT IS COMMA swap
// the characters '.' and ',' without changing the meaning of the sequence.
type Symbol int

const (
	// SymbolUnknown is the zero value; the scanner never emits it.
	SymbolUnknown Symbol = iota
	// SymbolDigit is 9, a stored digit position.
	SymbolDigit
	// SymbolAlphabetic is A, a letter or space.
	SymbolAlphabetic
	// SymbolAlphanumeric is X, any character.
	SymbolAlphanumeric
	// SymbolZeroSuppress is Z, a digit position whose leading zeros print as
	// spaces.
	SymbolZeroSuppress
	// SymbolCheckProtect is *, a digit position whose leading zeros print as
	// asterisks.
	SymbolCheckProtect
	// SymbolImpliedDecimal is V, the assumed decimal point. It occupies no
	// storage.
	SymbolImpliedDecimal
	// SymbolSign is S, the operational sign. It occupies no storage unless
	// SIGN IS SEPARATE, which is a clause outside the PICTURE.
	SymbolSign
	// SymbolScaling is P, a scaling position: a digit position of the value
	// that occupies no storage.
	SymbolScaling
	// SymbolSpaceInsert is B, which inserts a space.
	SymbolSpaceInsert
	// SymbolZeroInsert is 0, which inserts a zero.
	SymbolZeroInsert
	// SymbolSlashInsert is /, which inserts a slash.
	SymbolSlashInsert
	// SymbolGroupingSeparator is the thousands separator: ',' normally, '.'
	// under DECIMAL-POINT IS COMMA.
	SymbolGroupingSeparator
	// SymbolDecimalPoint is the actual, printed decimal point: '.' normally,
	// ',' under DECIMAL-POINT IS COMMA.
	SymbolDecimalPoint
	// SymbolPlus is +, which prints the sign of the value as '+' or '-'.
	SymbolPlus
	// SymbolMinus is -, which prints the sign of the value as a space or '-'.
	SymbolMinus
	// SymbolCredit is CR, printed when the value is negative.
	SymbolCredit
	// SymbolDebit is DB, printed when the value is negative.
	SymbolDebit
	// SymbolCurrency is the currency symbol, '$' by default.
	SymbolCurrency
)

// String implements the [fmt.Stringer] interface, returning the symbol's
// conventional spelling. The decimal point and grouping separator are spelled
// as they are without DECIMAL-POINT IS COMMA, since the value names the role
// rather than the character.
func (s Symbol) String() string {
	switch s {
	case SymbolDigit:
		return "9"
	case SymbolAlphabetic:
		return "A"
	case SymbolAlphanumeric:
		return "X"
	case SymbolZeroSuppress:
		return "Z"
	case SymbolCheckProtect:
		return "*"
	case SymbolImpliedDecimal:
		return "V"
	case SymbolSign:
		return "S"
	case SymbolScaling:
		return "P"
	case SymbolSpaceInsert:
		return "B"
	case SymbolZeroInsert:
		return "0"
	case SymbolSlashInsert:
		return "/"
	case SymbolGroupingSeparator:
		return ","
	case SymbolDecimalPoint:
		return "."
	case SymbolPlus:
		return "+"
	case SymbolMinus:
		return "-"
	case SymbolCredit:
		return "CR"
	case SymbolDebit:
		return "DB"
	case SymbolCurrency:
		return "$"
	}
	return "unknown"
}

// Picture is a parsed PICTURE character-string.
//
// Source is the lexeme as given, with its case preserved. Symbols is that
// lexeme with every repeat count expanded, one element per symbol occurrence,
// which is what an edited item's printer needs; CR and DB are one element each.
//
// Digits, Scale, and Signed are the attributes codec/SPEC.md ("From PICTURE to
// Attributes") defines, and hold for numeric and numeric-edited items:
//
//	value = unscaled_integer × 10^(−Scale)
//
// For alphabetic and alphanumeric items they are zero.
type Picture struct {
	// Source is the raw PICTURE character-string.
	Source string
	// Category is the item's category.
	Category Category
	// Digits is the number of digit positions: the 9 symbols of a numeric
	// item, and additionally the Z, *, and floating +, -, and currency
	// positions of a numeric-edited one. P positions are digit positions of
	// the value but occupy no storage and are not counted here.
	Digits int
	// Scale is the number of digit positions to the right of the decimal
	// point; it is negative when P positions push the point to the right of
	// the stored digits. PIC 9(3)V99 has scale 2, PIC 9(3)PPP scale -3.
	Scale int
	// Signed reports the S symbol: the item carries an operational sign.
	// The printed sign of an edited item (+, -, CR, DB) is not this.
	Signed bool
	// Size is the number of character positions the item occupies under
	// USAGE DISPLAY. S, V, and P occupy none; CR and DB occupy two each.
	// Widths under COMP, COMP-3, and the rest are a function of Digits and
	// belong to the codec package, not here.
	Size int
	// Symbols is the expanded symbol sequence, in source order.
	Symbols []Symbol
}

// String implements the [fmt.Stringer] interface, returning the raw
// character-string the [Picture] was parsed from.
func (p *Picture) String() string { return p.Source }

// ParseOption configures [Parse].
type ParseOption func(*parseConfig)

// parseConfig holds the resolved [ParseOption] values; its zero value selects
// the defaults.
type parseConfig struct {
	// decimalPointIsComma swaps the roles of '.' and ',' within the PICTURE
	// character-string. False — '.' is the decimal point — is the default.
	decimalPointIsComma bool
}

// WithDecimalPointIsComma parses the PICTURE character-string with the roles of
// '.' and ',' swapped, as the SPECIAL-NAMES clause DECIMAL-POINT IS COMMA
// requires: ',' becomes the actual decimal point and '.' the grouping
// separator. It is a property of the whole source unit, so the caller that
// parsed SPECIAL-NAMES passes it down.
func WithDecimalPointIsComma() ParseOption {
	return func(c *parseConfig) { c.decimalPointIsComma = true }
}

// Parse the PICTURE character-string into a [Picture].
//
// PICTURE symbols are case-insensitive, so "s9(3)v99" and "S9(3)V99" parse
// alike; Source keeps the spelling it was given. Repeat counts are expanded, so
// "9(7)" is exactly "9999999" and "S9(3)V9(2)" exactly "S999V99".
//
// No dialect digit limit is enforced: the standard caps Digits at 18 while IBM
// Enterprise COBOL with ARITH(EXTEND) and GnuCOBOL both raise it to 31, and
// which of those applies is the caller's dialect question rather than this
// package's.
func Parse(s string, opts ...ParseOption) (*Picture, error) {
	cfg := parseConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	if strings.TrimSpace(s) == "" {
		return nil, EmptyPictureError{Source: s}
	}

	symbols, err := scan(s, cfg.decimalPointIsComma)
	if err != nil {
		return nil, err
	}
	if err := validate(s, symbols); err != nil {
		return nil, err
	}

	digitPos := digitPositions(symbols)
	category, err := categoryOf(s, symbols, digitPos)
	if err != nil {
		return nil, err
	}
	digits, scale := digitsAndScale(symbols, digitPos)
	if category != CategoryNumeric && category != CategoryNumericEdited {
		// Only the numeric categories have a numeric value, so only they have
		// digit positions and a scale to report. A picture mixing A and 9 is
		// alphanumeric: its 9 positions hold characters, not digits.
		digits, scale = 0, 0
	}

	return &Picture{
		Source:   s,
		Category: category,
		Digits:   digits,
		Scale:    scale,
		Signed:   count(symbols, SymbolSign) > 0,
		Size:     sizeOf(symbols),
		Symbols:  symbols,
	}, nil
}

// count reports how many times sym occurs in symbols.
func count(symbols []Symbol, sym Symbol) int {
	n := 0
	for _, s := range symbols {
		if s == sym {
			n++
		}
	}
	return n
}

// validate checks the position rules the root SPEC.md states for the symbols
// whose placement is constrained: S is leftmost and appears once, the decimal
// point appears once, the P positions form a single run at one end, and CR/DB
// are rightmost.
//
// It is deliberately not a full legality check for edited pictures: where a
// floating insertion run may sit relative to the other insertion symbols is
// left to the compiler, since nothing this package derives depends on it.
func validate(src string, symbols []Symbol) error {
	if n := count(symbols, SymbolSign); n > 1 || (n == 1 && symbols[0] != SymbolSign) {
		return SymbolPlacementError{
			Source: src,
			Symbol: SymbolSign,
			Reason: "must appear at most once, as the leftmost symbol",
		}
	}
	if count(symbols, SymbolImpliedDecimal) > 1 {
		return SymbolPlacementError{
			Source: src,
			Symbol: SymbolImpliedDecimal,
			Reason: "must appear at most once",
		}
	}
	if count(symbols, SymbolDecimalPoint) > 1 {
		return SymbolPlacementError{
			Source: src,
			Symbol: SymbolDecimalPoint,
			Reason: "must appear at most once",
		}
	}
	if count(symbols, SymbolImpliedDecimal) > 0 && count(symbols, SymbolDecimalPoint) > 0 {
		return SymbolPlacementError{
			Source: src,
			Symbol: SymbolDecimalPoint,
			Reason: "must not appear with V; a picture has at most one decimal point",
		}
	}
	if count(symbols, SymbolZeroSuppress) > 0 && count(symbols, SymbolCheckProtect) > 0 {
		return SymbolPlacementError{
			Source: src,
			Symbol: SymbolCheckProtect,
			Reason: "must not appear with Z; a picture has one zero-suppression symbol",
		}
	}
	if err := validateScaling(src, symbols); err != nil {
		return err
	}
	for _, sym := range []Symbol{SymbolCredit, SymbolDebit} {
		if n := count(symbols, sym); n > 1 || (n == 1 && symbols[len(symbols)-1] != sym) {
			return SymbolPlacementError{
				Source: src,
				Symbol: sym,
				Reason: "must appear at most once, as the rightmost symbol",
			}
		}
	}
	if count(symbols, SymbolCredit) > 0 && count(symbols, SymbolDebit) > 0 {
		return SymbolPlacementError{
			Source: src,
			Symbol: SymbolDebit,
			Reason: "must not appear with CR; a picture has one sign-control symbol",
		}
	}
	plus, minus := count(symbols, SymbolPlus), count(symbols, SymbolMinus)
	if plus > 0 && minus > 0 {
		return SymbolPlacementError{
			Source: src,
			Symbol: SymbolMinus,
			Reason: "must not appear with +; a picture has one sign-control symbol",
		}
	}
	if plus+minus > 0 && count(symbols, SymbolCredit)+count(symbols, SymbolDebit) > 0 {
		// Report the sign-control symbol the picture actually holds: only one
		// of the two can be present here, the pair having been rejected above.
		sign := SymbolPlus
		if minus > 0 {
			sign = SymbolMinus
		}
		return SymbolPlacementError{
			Source: src,
			Symbol: sign,
			Reason: "must not appear with CR or DB; a picture has one sign-control symbol",
		}
	}
	return nil
}

// validateScaling checks the P positions: they form one contiguous run, and
// that run sits at the left end (behind at most an S and a V) or at the right
// end (ahead of at most a V).
func validateScaling(src string, symbols []Symbol) error {
	first, last := -1, -1
	for i, s := range symbols {
		if s == SymbolScaling {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return nil
	}

	placement := SymbolPlacementError{
		Source: src,
		Symbol: SymbolScaling,
		Reason: "must form a single run at the left or right end of the picture",
	}
	for i := first; i <= last; i++ {
		if symbols[i] != SymbolScaling {
			return placement
		}
	}

	leading := true
	for i := 0; i < first; i++ {
		if symbols[i] != SymbolSign && symbols[i] != SymbolImpliedDecimal {
			leading = false
			break
		}
	}
	trailing := true
	for i := last + 1; i < len(symbols); i++ {
		if symbols[i] != SymbolImpliedDecimal {
			trailing = false
			break
		}
	}
	if !leading && !trailing {
		return placement
	}
	return nil
}

// digitPositions marks, for each element of symbols, whether it is a digit
// position of the item's value.
//
// 9, Z, and * always are. A floating insertion symbol (+, -, or the currency
// symbol) repeated two or more times contributes one digit position per
// occurrence past the first, since the leftmost of the run is the sign or
// currency character itself: "$$,$$9.99" has three $ digit positions.
// A lone +, -, or currency symbol is a fixed insertion and no digit position at
// all.
func digitPositions(symbols []Symbol) []bool {
	counts := make(map[Symbol]int, len(symbols))
	for _, s := range symbols {
		counts[s]++
	}

	seen := make(map[Symbol]bool, 3)
	out := make([]bool, len(symbols))
	for i, s := range symbols {
		switch s {
		case SymbolDigit, SymbolZeroSuppress, SymbolCheckProtect:
			out[i] = true
		case SymbolPlus, SymbolMinus, SymbolCurrency:
			out[i] = counts[s] > 1 && seen[s]
			seen[s] = true
		}
	}
	return out
}

// categoryOf derives the item's category from the set of symbols present, per
// the table in root SPEC.md's Semantics section and codec/SPEC.md's "Category".
//
// The one case neither table spells out is a mix of A and 9 without an X, which
// the standard makes alphanumeric — the same as any other combination of two of
// A, X, and 9 — so that is what this returns.
func categoryOf(src string, symbols []Symbol, digitPos []bool) (Category, error) {
	has := make(map[Symbol]bool, len(symbols))
	for _, s := range symbols {
		has[s] = true
	}

	numericEditing := has[SymbolZeroSuppress] || has[SymbolCheckProtect] ||
		has[SymbolGroupingSeparator] || has[SymbolDecimalPoint] ||
		has[SymbolPlus] || has[SymbolMinus] ||
		has[SymbolCredit] || has[SymbolDebit] || has[SymbolCurrency]
	simpleInsertion := has[SymbolSpaceInsert] || has[SymbolZeroInsert] || has[SymbolSlashInsert]
	numericOnly := has[SymbolSign] || has[SymbolImpliedDecimal] || has[SymbolScaling]

	if has[SymbolAlphanumeric] || has[SymbolAlphabetic] {
		if numericEditing {
			return CategoryUnknown, CategoryError{
				Source: src,
				Reason: "numeric editing symbols must not appear with A or X",
			}
		}
		if numericOnly {
			return CategoryUnknown, CategoryError{
				Source: src,
				Reason: "S, V, and P must not appear with A or X",
			}
		}
		if simpleInsertion {
			return CategoryAlphanumericEdited, nil
		}
		if has[SymbolAlphanumeric] || has[SymbolDigit] {
			return CategoryAlphanumeric, nil
		}
		return CategoryAlphabetic, nil
	}

	digits := false
	for _, d := range digitPos {
		if d {
			digits = true
			break
		}
	}
	if !digits {
		return CategoryUnknown, CategoryError{
			Source: src,
			Reason: "no digit or character positions",
		}
	}
	if numericEditing || simpleInsertion {
		return CategoryNumericEdited, nil
	}
	return CategoryNumeric, nil
}

// digitsAndScale counts the digit positions and derives the scale, per
// codec/SPEC.md's "Digits" and "Scale".
//
// Scale is found by assigning an exponent to every position that carries a
// digit of the value — the digit positions and the P positions alike. The
// position immediately left of the decimal point has exponent 0, the next left
// 1, the one immediately right -1, and Scale is the negated exponent of the
// rightmost digit position, so that value = unscaled_integer × 10^(−Scale).
//
// The decimal point is at V (or at the actual decimal point of an edited item);
// with neither, it is immediately left of a leading P run, and otherwise at the
// right end.
func digitsAndScale(symbols []Symbol, digitPos []bool) (digits, scale int) {
	// valueIsDigit holds one element per value position — digit positions and
	// P positions, in order — reporting whether it is a stored digit position.
	var valueIsDigit []bool
	point := -1
	leadingScaling := false

	for i, s := range symbols {
		switch {
		case s == SymbolImpliedDecimal || s == SymbolDecimalPoint:
			if point < 0 {
				point = len(valueIsDigit)
			}
		case s == SymbolScaling:
			if len(valueIsDigit) == 0 {
				leadingScaling = true
			}
			valueIsDigit = append(valueIsDigit, false)
		case digitPos[i]:
			valueIsDigit = append(valueIsDigit, true)
			digits++
		}
	}

	if point < 0 {
		if leadingScaling {
			point = 0
		} else {
			point = len(valueIsDigit)
		}
	}

	rightmost := -1
	for i, d := range valueIsDigit {
		if d {
			rightmost = i
		}
	}
	if rightmost < 0 {
		return digits, 0
	}
	return digits, -(point - 1 - rightmost)
}

// sizeOf counts the character positions the item occupies under USAGE DISPLAY.
// S, V, and P occupy none — S occupies one only under SIGN IS SEPARATE, a
// clause outside the PICTURE — and CR and DB occupy two each.
func sizeOf(symbols []Symbol) int {
	size := 0
	for _, s := range symbols {
		switch s {
		case SymbolSign, SymbolImpliedDecimal, SymbolScaling:
		case SymbolCredit, SymbolDebit:
			size += 2
		default:
			size++
		}
	}
	return size
}

// SymbolPlacementError is returned when a PICTURE symbol appears somewhere the
// position rules do not allow it, or more often than they allow.
type SymbolPlacementError struct {
	Source string
	Symbol Symbol
	Reason string
}

// Error implements the [error] interface.
func (e SymbolPlacementError) Error() string {
	return fmt.Sprintf("invalid PICTURE %q: symbol %s %s", e.Source, e.Symbol, e.Reason)
}

// CategoryError is returned when the set of symbols in a PICTURE
// character-string belongs to no category: a combination the categories do not
// admit, or a picture with no digit or character positions at all.
type CategoryError struct {
	Source string
	Reason string
}

// Error implements the [error] interface.
func (e CategoryError) Error() string {
	return fmt.Sprintf("invalid PICTURE %q: %s", e.Source, e.Reason)
}
