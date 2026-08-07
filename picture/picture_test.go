// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package picture

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// rep returns n copies of the given symbols, the expanded form of a repeat
// count: rep(3, SymbolDigit) is what "9(3)" scans to.
func rep(n int, symbols ...Symbol) []Symbol {
	out := make([]Symbol, 0, n*len(symbols))
	for range n {
		out = append(out, symbols...)
	}
	return out
}

func TestParse(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
		opts []ParseOption
		want *Picture
	}{
		{
			name: "unsigned integer",
			src:  "9(5)",
			want: &Picture{
				Source:   "9(5)",
				Category: CategoryNumeric,
				Digits:   5,
				Size:     5,
				Symbols:  rep(5, SymbolDigit),
			},
		},
		{
			name: "single digit",
			src:  "9",
			want: &Picture{
				Source:   "9",
				Category: CategoryNumeric,
				Digits:   1,
				Size:     1,
				Symbols:  []Symbol{SymbolDigit},
			},
		},
		{
			name: "signed with implied decimal and repeat counts",
			src:  "S9(3)V9(2)",
			want: &Picture{
				Source:   "S9(3)V9(2)",
				Category: CategoryNumeric,
				Digits:   5,
				Scale:    2,
				Signed:   true,
				Size:     5,
				Symbols: slices.Concat(
					[]Symbol{SymbolSign},
					rep(3, SymbolDigit),
					[]Symbol{SymbolImpliedDecimal},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "lower case is the same picture",
			src:  "s9(3)v99",
			want: &Picture{
				Source:   "s9(3)v99",
				Category: CategoryNumeric,
				Digits:   5,
				Scale:    2,
				Signed:   true,
				Size:     5,
				Symbols: slices.Concat(
					[]Symbol{SymbolSign},
					rep(3, SymbolDigit),
					[]Symbol{SymbolImpliedDecimal},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "signed packed-decimal style picture",
			src:  "S9(7)V99",
			want: &Picture{
				Source:   "S9(7)V99",
				Category: CategoryNumeric,
				Digits:   9,
				Scale:    2,
				Signed:   true,
				Size:     9,
				Symbols: slices.Concat(
					[]Symbol{SymbolSign},
					rep(7, SymbolDigit),
					[]Symbol{SymbolImpliedDecimal},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "implied decimal at the left end",
			src:  "V9(5)",
			want: &Picture{
				Source:   "V9(5)",
				Category: CategoryNumeric,
				Digits:   5,
				Scale:    5,
				Size:     5,
				Symbols:  slices.Concat([]Symbol{SymbolImpliedDecimal}, rep(5, SymbolDigit)),
			},
		},
		{
			name: "implied decimal in the middle",
			src:  "9(3)V99",
			want: &Picture{
				Source:   "9(3)V99",
				Category: CategoryNumeric,
				Digits:   5,
				Scale:    2,
				Size:     5,
				Symbols: slices.Concat(
					rep(3, SymbolDigit),
					[]Symbol{SymbolImpliedDecimal},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "trailing scaling positions",
			src:  "9(5)PPP",
			want: &Picture{
				Source:   "9(5)PPP",
				Category: CategoryNumeric,
				Digits:   5,
				Scale:    -3,
				Size:     5,
				Symbols:  slices.Concat(rep(5, SymbolDigit), rep(3, SymbolScaling)),
			},
		},
		{
			name: "one trailing scaling position",
			src:  "9(5)P",
			want: &Picture{
				Source:   "9(5)P",
				Category: CategoryNumeric,
				Digits:   5,
				Scale:    -1,
				Size:     5,
				Symbols:  slices.Concat(rep(5, SymbolDigit), []Symbol{SymbolScaling}),
			},
		},
		{
			name: "leading scaling positions",
			src:  "PPP9(5)",
			want: &Picture{
				Source:   "PPP9(5)",
				Category: CategoryNumeric,
				Digits:   5,
				Scale:    8,
				Size:     5,
				Symbols:  slices.Concat(rep(3, SymbolScaling), rep(5, SymbolDigit)),
			},
		},
		{
			name: "signed leading scaling positions",
			src:  "SPPP99",
			want: &Picture{
				Source:   "SPPP99",
				Category: CategoryNumeric,
				Digits:   2,
				Scale:    5,
				Signed:   true,
				Size:     2,
				Symbols: slices.Concat(
					[]Symbol{SymbolSign},
					rep(3, SymbolScaling),
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "alphabetic",
			src:  "A(4)",
			want: &Picture{
				Source:   "A(4)",
				Category: CategoryAlphabetic,
				Size:     4,
				Symbols:  rep(4, SymbolAlphabetic),
			},
		},
		{
			name: "alphanumeric",
			src:  "X(10)",
			want: &Picture{
				Source:   "X(10)",
				Category: CategoryAlphanumeric,
				Size:     10,
				Symbols:  rep(10, SymbolAlphanumeric),
			},
		},
		{
			name: "alphanumeric mixing A and 9",
			src:  "A(3)9",
			want: &Picture{
				Source:   "A(3)9",
				Category: CategoryAlphanumeric,
				Size:     4,
				Symbols:  slices.Concat(rep(3, SymbolAlphabetic), []Symbol{SymbolDigit}),
			},
		},
		{
			name: "alphanumeric edited with space insertion",
			src:  "XXBXX",
			want: &Picture{
				Source:   "XXBXX",
				Category: CategoryAlphanumericEdited,
				Size:     5,
				Symbols: slices.Concat(
					rep(2, SymbolAlphanumeric),
					[]Symbol{SymbolSpaceInsert},
					rep(2, SymbolAlphanumeric),
				),
			},
		},
		{
			name: "alphanumeric edited with slash insertion",
			src:  "XX/XX/XX",
			want: &Picture{
				Source:   "XX/XX/XX",
				Category: CategoryAlphanumericEdited,
				Size:     8,
				Symbols: slices.Concat(
					rep(2, SymbolAlphanumeric),
					[]Symbol{SymbolSlashInsert},
					rep(2, SymbolAlphanumeric),
					[]Symbol{SymbolSlashInsert},
					rep(2, SymbolAlphanumeric),
				),
			},
		},
		{
			name: "alphabetic edited",
			src:  "AAB(2)AA",
			want: &Picture{
				Source:   "AAB(2)AA",
				Category: CategoryAlphanumericEdited,
				Size:     6,
				Symbols: slices.Concat(
					rep(2, SymbolAlphabetic),
					rep(2, SymbolSpaceInsert),
					rep(2, SymbolAlphabetic),
				),
			},
		},
		{
			name: "numeric edited with zero suppression",
			src:  "ZZ,ZZ9.99",
			want: &Picture{
				Source:   "ZZ,ZZ9.99",
				Category: CategoryNumericEdited,
				Digits:   7,
				Scale:    2,
				Size:     9,
				Symbols: slices.Concat(
					rep(2, SymbolZeroSuppress),
					[]Symbol{SymbolGroupingSeparator},
					rep(2, SymbolZeroSuppress),
					[]Symbol{SymbolDigit, SymbolDecimalPoint},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "numeric edited with check protection",
			src:  "**9.99",
			want: &Picture{
				Source:   "**9.99",
				Category: CategoryNumericEdited,
				Digits:   5,
				Scale:    2,
				Size:     6,
				Symbols: slices.Concat(
					rep(2, SymbolCheckProtect),
					[]Symbol{SymbolDigit, SymbolDecimalPoint},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "numeric edited with floating currency",
			src:  "$$,$$9.99",
			want: &Picture{
				Source:   "$$,$$9.99",
				Category: CategoryNumericEdited,
				Digits:   6,
				Scale:    2,
				Size:     9,
				Symbols: slices.Concat(
					rep(2, SymbolCurrency),
					[]Symbol{SymbolGroupingSeparator},
					rep(2, SymbolCurrency),
					[]Symbol{SymbolDigit, SymbolDecimalPoint},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "numeric edited with fixed currency",
			src:  "$9(4).99",
			want: &Picture{
				Source:   "$9(4).99",
				Category: CategoryNumericEdited,
				Digits:   6,
				Scale:    2,
				Size:     8,
				Symbols: slices.Concat(
					[]Symbol{SymbolCurrency},
					rep(4, SymbolDigit),
					[]Symbol{SymbolDecimalPoint},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "numeric edited with floating sign",
			src:  "++++.99",
			want: &Picture{
				Source:   "++++.99",
				Category: CategoryNumericEdited,
				Digits:   5,
				Scale:    2,
				Size:     7,
				Symbols: slices.Concat(
					rep(4, SymbolPlus),
					[]Symbol{SymbolDecimalPoint},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "numeric edited with trailing sign",
			src:  "ZZZ9-",
			want: &Picture{
				Source:   "ZZZ9-",
				Category: CategoryNumericEdited,
				Digits:   4,
				Size:     5,
				Symbols: slices.Concat(
					rep(3, SymbolZeroSuppress),
					[]Symbol{SymbolDigit, SymbolMinus},
				),
			},
		},
		{
			name: "numeric edited with trailing CR",
			src:  "9(4)CR",
			want: &Picture{
				Source:   "9(4)CR",
				Category: CategoryNumericEdited,
				Digits:   4,
				Size:     6,
				Symbols:  slices.Concat(rep(4, SymbolDigit), []Symbol{SymbolCredit}),
			},
		},
		{
			name: "numeric edited with trailing DB in lower case",
			src:  "9(4)db",
			want: &Picture{
				Source:   "9(4)db",
				Category: CategoryNumericEdited,
				Digits:   4,
				Size:     6,
				Symbols:  slices.Concat(rep(4, SymbolDigit), []Symbol{SymbolDebit}),
			},
		},
		{
			name: "numeric edited with simple insertion only",
			src:  "99B99",
			want: &Picture{
				Source:   "99B99",
				Category: CategoryNumericEdited,
				Digits:   4,
				Size:     5,
				Symbols: slices.Concat(
					rep(2, SymbolDigit),
					[]Symbol{SymbolSpaceInsert},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "numeric edited with zero insertion",
			src:  "9(3)000",
			want: &Picture{
				Source:   "9(3)000",
				Category: CategoryNumericEdited,
				Digits:   3,
				Size:     6,
				Symbols:  slices.Concat(rep(3, SymbolDigit), rep(3, SymbolZeroInsert)),
			},
		},
		{
			name: "numeric edited with scaling positions",
			src:  "ZZZPPP",
			want: &Picture{
				Source:   "ZZZPPP",
				Category: CategoryNumericEdited,
				Digits:   3,
				Scale:    -3,
				Size:     3,
				Symbols:  slices.Concat(rep(3, SymbolZeroSuppress), rep(3, SymbolScaling)),
			},
		},
		{
			name: "decimal point is comma swaps the roles of . and ,",
			src:  "ZZ.ZZ9,99",
			opts: []ParseOption{WithDecimalPointIsComma()},
			want: &Picture{
				Source:   "ZZ.ZZ9,99",
				Category: CategoryNumericEdited,
				Digits:   7,
				Scale:    2,
				Size:     9,
				Symbols: slices.Concat(
					rep(2, SymbolZeroSuppress),
					[]Symbol{SymbolGroupingSeparator},
					rep(2, SymbolZeroSuppress),
					[]Symbol{SymbolDigit, SymbolDecimalPoint},
					rep(2, SymbolDigit),
				),
			},
		},
		{
			name: "decimal point is comma leaves V alone",
			src:  "S9(3)V99",
			opts: []ParseOption{WithDecimalPointIsComma()},
			want: &Picture{
				Source:   "S9(3)V99",
				Category: CategoryNumeric,
				Digits:   5,
				Scale:    2,
				Signed:   true,
				Size:     5,
				Symbols: slices.Concat(
					[]Symbol{SymbolSign},
					rep(3, SymbolDigit),
					[]Symbol{SymbolImpliedDecimal},
					rep(2, SymbolDigit),
				),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(tc.src, tc.opts...)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseRepeatCountEquivalence(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		repeated string
		expanded string
	}{
		{name: "digits", repeated: "9(7)", expanded: "9999999"},
		{name: "mixed forms", repeated: "S9(3)V9(2)", expanded: "S999V99"},
		{name: "alphanumeric", repeated: "X(4)", expanded: "XXXX"},
		{name: "zero suppression", repeated: "Z(4)9", expanded: "ZZZZ9"},
		{name: "repeat count of one", repeated: "9(1)V9(1)", expanded: "9V9"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			repeated, err := Parse(tc.repeated)
			require.NoError(t, err)
			expanded, err := Parse(tc.expanded)
			require.NoError(t, err)

			// Source is the one field that is expected to differ.
			repeated.Source = expanded.Source
			require.Equal(t, expanded, repeated)
		})
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	t.Run("empty picture", func(t *testing.T) {
		t.Parallel()

		for _, src := range []string{"", "   "} {
			_, err := Parse(src)

			var target EmptyPictureError
			require.ErrorAs(t, err, &target)
			require.Equal(t, src, target.Source)
		}
	})

	t.Run("unexpected symbol", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name   string
			src    string
			r      rune
			offset int
		}{
			{name: "not a picture symbol", src: "9Q9", r: 'Q', offset: 1},
			{name: "C without R", src: "9C9", r: 'C', offset: 1},
			{name: "D without B", src: "9D", r: 'D', offset: 1},
			{name: "C at end of input", src: "9C", r: 'C', offset: 1},
			{name: "embedded space", src: "9 9", r: ' ', offset: 1},
			{name: "unmatched close paren", src: "9)", r: ')', offset: 1},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				_, err := Parse(tc.src)

				var target UnexpectedSymbolError
				require.ErrorAs(t, err, &target)
				require.Equal(t, tc.src, target.Source)
				require.Equal(t, tc.r, target.R)
				require.Equal(t, tc.offset, target.Offset)
			})
		}
	})

	t.Run("repeat count", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string
			src  string
		}{
			{name: "unterminated", src: "9(3"},
			{name: "empty", src: "9()"},
			{name: "not a number", src: "9(x)"},
			{name: "zero", src: "9(0)"},
			{name: "beyond the maximum", src: "9(65536)"},
			{name: "no preceding symbol", src: "(3)"},
			{name: "doubled", src: "9(3)(2)"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				_, err := Parse(tc.src)

				var target RepeatCountError
				require.ErrorAs(t, err, &target)
				require.Equal(t, tc.src, target.Source)
			})
		}
	})

	t.Run("symbol placement", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name   string
			src    string
			symbol Symbol
		}{
			{name: "S is not leftmost", src: "99S", symbol: SymbolSign},
			{name: "S appears twice", src: "S9S9", symbol: SymbolSign},
			{name: "V appears twice", src: "9V9V9", symbol: SymbolImpliedDecimal},
			{name: "decimal point appears twice", src: "9.9.9", symbol: SymbolDecimalPoint},
			{name: "decimal point with V", src: "9V9.9", symbol: SymbolDecimalPoint},
			{name: "Z with check protection", src: "Z*9", symbol: SymbolCheckProtect},
			{name: "P run is not contiguous", src: "9P9P", symbol: SymbolScaling},
			{name: "P run is in the middle", src: "99PP99", symbol: SymbolScaling},
			{name: "CR is not rightmost", src: "9CR9", symbol: SymbolCredit},
			{name: "CR appears twice", src: "9CRCR", symbol: SymbolCredit},
			{name: "CR with DB", src: "9CRDB", symbol: SymbolCredit},
			{name: "plus with minus", src: "+99-", symbol: SymbolMinus},
			{name: "plus with CR", src: "+99CR", symbol: SymbolPlus},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				_, err := Parse(tc.src)

				var target SymbolPlacementError
				require.ErrorAs(t, err, &target)
				require.Equal(t, tc.src, target.Source)
				require.Equal(t, tc.symbol, target.Symbol)
			})
		}
	})

	t.Run("category", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name string
			src  string
		}{
			{name: "X with an implied decimal", src: "X9V9"},
			{name: "X with a sign", src: "SX(3)"},
			{name: "X with zero suppression", src: "XZ9"},
			{name: "A with a decimal point", src: "AA.AA"},
			{name: "no digit or character positions", src: "V"},
			{name: "sign alone", src: "S"},
			{name: "currency alone", src: "$"},
			{name: "scaling positions alone", src: "PPP"},
			{name: "insertion symbols alone", src: ",,,"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				_, err := Parse(tc.src)

				var target CategoryError
				require.ErrorAs(t, err, &target)
				require.Equal(t, tc.src, target.Source)
			})
		}
	})
}

func TestPictureString(t *testing.T) {
	t.Parallel()

	p, err := Parse("S9(5)V99")
	require.NoError(t, err)
	require.Equal(t, "S9(5)V99", p.String())
}

func TestCategoryString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		category Category
		want     string
	}{
		{name: "numeric", category: CategoryNumeric, want: "numeric"},
		{name: "alphabetic", category: CategoryAlphabetic, want: "alphabetic"},
		{name: "alphanumeric", category: CategoryAlphanumeric, want: "alphanumeric"},
		{name: "numeric edited", category: CategoryNumericEdited, want: "numeric-edited"},
		{name: "alphanumeric edited", category: CategoryAlphanumericEdited, want: "alphanumeric-edited"},
		{name: "unknown", category: CategoryUnknown, want: "unknown"},
		{name: "out of range", category: Category(42), want: "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.category.String())
		})
	}
}

func TestSymbolString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		symbol Symbol
		want   string
	}{
		{name: "digit", symbol: SymbolDigit, want: "9"},
		{name: "alphabetic", symbol: SymbolAlphabetic, want: "A"},
		{name: "alphanumeric", symbol: SymbolAlphanumeric, want: "X"},
		{name: "zero suppress", symbol: SymbolZeroSuppress, want: "Z"},
		{name: "check protect", symbol: SymbolCheckProtect, want: "*"},
		{name: "implied decimal", symbol: SymbolImpliedDecimal, want: "V"},
		{name: "sign", symbol: SymbolSign, want: "S"},
		{name: "scaling", symbol: SymbolScaling, want: "P"},
		{name: "space insert", symbol: SymbolSpaceInsert, want: "B"},
		{name: "zero insert", symbol: SymbolZeroInsert, want: "0"},
		{name: "slash insert", symbol: SymbolSlashInsert, want: "/"},
		{name: "grouping separator", symbol: SymbolGroupingSeparator, want: ","},
		{name: "decimal point", symbol: SymbolDecimalPoint, want: "."},
		{name: "plus", symbol: SymbolPlus, want: "+"},
		{name: "minus", symbol: SymbolMinus, want: "-"},
		{name: "credit", symbol: SymbolCredit, want: "CR"},
		{name: "debit", symbol: SymbolDebit, want: "DB"},
		{name: "currency", symbol: SymbolCurrency, want: "$"},
		{name: "unknown", symbol: SymbolUnknown, want: "unknown"},
		{name: "out of range", symbol: Symbol(42), want: "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.symbol.String())
		})
	}
}
