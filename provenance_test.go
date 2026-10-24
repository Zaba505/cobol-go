// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Every entry and clause position names the text it is in — the source given to
// Parse, or a library text by the name and library the resolver was asked for —
// and a copied one carries the chain of COPY statements that reached it, two
// levels deep here.
func TestParserCopyProvenance(t *testing.T) {
	t.Parallel()

	books := MapCopyBooks(map[string]string{
		"OUTER": "05 OUTER-ID PIC 9(4).\n" +
			"   COPY middle OF PayLib.\n",
		"PAYLIB.MIDDLE": "05 MIDDLE-NAME PIC X(10).\n",
	})
	src := "01 SOURCE-RECORD.\n" +
		"   COPY OUTER.\n"

	f, err := Parse(strings.NewReader(src), WithFragment(), WithCopyBooks(books))
	require.NoError(t, err)

	// The names are spelled as the COPY statement spelled them, which is what
	// the resolver was asked for; the library is the OF phrase's.
	outer := &LibraryText{Name: "OUTER", Copy: Pos{Line: 2, Column: 4}}
	middle := &LibraryText{Name: "middle", Library: "PayLib", Copy: Pos{Line: 2, Column: 4, Text: outer}}

	require.Equal(t, &File{
		Fragment: &Fragment{
			Entries: []*DataDescriptionEntry{
				{
					Pos:   Pos{Line: 1, Column: 1},
					Level: 1,
					Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "SOURCE-RECORD"},
				},
				{
					Pos:     Pos{Line: 1, Column: 1, Text: outer},
					Level:   5,
					Name:    &Word{Pos: Pos{Line: 1, Column: 4, Text: outer}, Value: "OUTER-ID"},
					Clauses: []DataClause{&PictureClause{Pos: Pos{Line: 1, Column: 13, Text: outer}, Picture: "9(4)"}},
				},
				{
					Pos:     Pos{Line: 1, Column: 1, Text: middle},
					Level:   5,
					Name:    &Word{Pos: Pos{Line: 1, Column: 4, Text: middle}, Value: "MIDDLE-NAME"},
					Clauses: []DataClause{&PictureClause{Pos: Pos{Line: 1, Column: 16, Text: middle}, Picture: "X(10)"}},
				},
			},
		},
	}, f)

	entries := f.Fragment.Entries
	// Every position copied from one expansion shares its LibraryText, so ==
	// compares them as positions in one text.
	require.Same(t, entries[2].Pos.Text, entries[2].Clauses[0].(*PictureClause).Pos.Text)
	// The chain runs innermost first: the COPY that brought MIDDLE in, written
	// in OUTER, then the COPY that brought OUTER in, written in the source.
	require.Equal(t, []Pos{
		{Line: 2, Column: 4, Text: outer},
		{Line: 2, Column: 4},
	}, entries[2].Pos.CopyChain())
	require.Equal(t, []Pos{{Line: 2, Column: 4}}, entries[1].Pos.CopyChain())
	require.Nil(t, entries[0].Pos.CopyChain())
}

// The same copybook copied twice is two expansions: each names its own COPY
// statement, so its entries are told apart by Text where Line and Column agree.
func TestParserCopyProvenanceTwoExpansions(t *testing.T) {
	t.Parallel()

	books := MapCopyBooks(map[string]string{"ITEM": "05 ITEM-ID PIC X.\n"})
	src := "01 A.\nCOPY ITEM.\n01 B.\nCOPY ITEM.\n"

	f, err := Parse(strings.NewReader(src), WithFragment(), WithCopyBooks(books))
	require.NoError(t, err)

	first, second := f.Fragment.Entries[1].Pos, f.Fragment.Entries[3].Pos
	require.Equal(t, Pos{Line: 1, Column: 1, Text: &LibraryText{Name: "ITEM", Copy: Pos{Line: 2, Column: 1}}}, first)
	require.Equal(t, Pos{Line: 1, Column: 1, Text: &LibraryText{Name: "ITEM", Copy: Pos{Line: 4, Column: 1}}}, second)
	require.NotEqual(t, first, second)
}

// A position in a library text copied with REPLACING is the position in the
// text as written, and a token the substitution produced is positioned at the
// first character of the text words it replaced.
func TestReplacingPositionsAsWritten(t *testing.T) {
	t.Parallel()

	type tokenAt struct {
		Value  string
		Line   int
		Column int
	}

	testCases := []struct {
		name         string
		written      string
		replacements []copyReplacement
		expected     []tokenAt
	}{
		{
			name:    "no replacing",
			written: "05 A PIC X.\n",
			expected: []tokenAt{
				{"05", 1, 1}, {"A", 1, 4}, {"PIC", 1, 6}, {"X", 1, 10}, {".", 1, 11},
			},
		},
		{
			// The word gets two shorter; the column of every token after it on
			// the line is still its column as written.
			name:         "a shorter word",
			written:      "05 :TAG:-ID PIC X.\n05 NEXT PIC X.\n",
			replacements: []copyReplacement{{From: ":TAG:", To: "CU"}},
			expected: []tokenAt{
				{"05", 1, 1}, {"CU-ID", 1, 4}, {"PIC", 1, 13}, {"X", 1, 17}, {".", 1, 18},
				{"05", 2, 1}, {"NEXT", 2, 4}, {"PIC", 2, 9}, {"X", 2, 13}, {".", 2, 14},
			},
		},
		{
			name:         "a longer word",
			written:      "05 A PIC X.\n",
			replacements: []copyReplacement{{From: "A", To: "MUCH-LONGER"}},
			expected: []tokenAt{
				{"05", 1, 1}, {"MUCH-LONGER", 1, 4}, {"PIC", 1, 6}, {"X", 1, 10}, {".", 1, 11},
			},
		},
		{
			// Two substitutions on one line each move the columns after them,
			// and the second is measured from the end of the first as written.
			name:         "two substitutions on one line",
			written:      "05 A PIC B.\n",
			replacements: []copyReplacement{{From: "A", To: "LONG-A"}, {From: "B", To: "X(20)"}},
			expected: []tokenAt{
				{"05", 1, 1}, {"LONG-A", 1, 4}, {"PIC", 1, 6}, {"X(20)", 1, 10}, {".", 1, 11},
			},
		},
		{
			name:         "a deletion",
			written:      "05 A VALUE 1 PIC X.\n",
			replacements: []copyReplacement{{From: "VALUE 1", To: ""}},
			expected: []tokenAt{
				{"05", 1, 1}, {"A", 1, 4}, {"PIC", 1, 14}, {"X", 1, 18}, {".", 1, 19},
			},
		},
		{
			// One substitution producing several tokens positions every one of
			// them at the start of the words it replaced.
			name:         "a substitution producing several tokens",
			written:      "05 A :CLAUSES:.\n",
			replacements: []copyReplacement{{From: ":CLAUSES:", To: "PIC X(4) COMP"}},
			expected: []tokenAt{
				{"05", 1, 1}, {"A", 1, 4}, {"PIC", 1, 6}, {"X(4)", 1, 6}, {"COMP", 1, 6}, {".", 1, 15},
			},
		},
		{
			// A replacement spanning lines moves the lines after it back to
			// where they are as written, and the rest of its last line too.
			name:         "a replacement spanning lines",
			written:      "05 A :P: B.\n05 C.\n",
			replacements: []copyReplacement{{From: ":P:", To: "PIC\n   X"}},
			expected: []tokenAt{
				{"05", 1, 1}, {"A", 1, 4}, {"PIC", 1, 6}, {"X", 1, 6}, {"B", 1, 10}, {".", 1, 11},
				{"05", 2, 1}, {"C", 2, 4}, {".", 2, 5},
			},
		},
		{
			// Matched words spanning lines as written: what follows them is
			// positioned on the line where they end.
			name:         "matched words spanning lines",
			written:      "05 A PIC\n   X(4) VALUE 1.\n05 B.\n",
			replacements: []copyReplacement{{From: "PIC X(4)", To: "PIC 9"}},
			expected: []tokenAt{
				{"05", 1, 1}, {"A", 1, 4}, {"PIC", 1, 6}, {"9", 1, 6}, {"VALUE", 2, 9}, {"1", 2, 15}, {".", 2, 16},
				{"05", 3, 1}, {"B", 3, 4}, {".", 3, 5},
			},
		},
		{
			// Columns count runes, as the tokenizer's do.
			name:         "multibyte text before a substitution",
			written:      "05 A VALUE 'é' PIC :T:.\n",
			replacements: []copyReplacement{{From: ":T:", To: "XX"}},
			expected: []tokenAt{
				{"05", 1, 1}, {"A", 1, 4}, {"VALUE", 1, 6}, {"'é'", 1, 12}, {"PIC", 1, 16}, {"XX", 1, 20}, {".", 1, 23},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			origin := &LibraryText{Name: "BOOK", Copy: Pos{Line: 9, Column: 9}}
			replaced, spans := applyReplacing(tc.written, tc.replacements)

			var got []tokenAt
			for tok, err := range relocateTokens(Tokenize(strings.NewReader(replaced)), asWritten(tc.written, replaced, spans, origin)) {
				require.NoError(t, err)
				require.Same(t, origin, tok.Pos.Text)
				got = append(got, tokenAt{string(tok.Value), tok.Pos.Line, tok.Pos.Column})
			}
			require.Equal(t, tc.expected, got)
		})
	}
}

// A substitution made in an outer copybook is not seen by a nested one: each
// expansion maps positions back through its own REPLACING phrase only.
func TestParserCopyReplacingNested(t *testing.T) {
	t.Parallel()

	books := MapCopyBooks(map[string]string{
		"OUTER": "05 :P:-ID PIC X.\nCOPY INNER REPLACING ==:Q:== BY ==LONGER-PREFIX==.\n",
		"INNER": "05 :Q:-NAME PIC X.\n",
	})
	f, err := Parse(
		strings.NewReader("COPY OUTER REPLACING ==:P:== BY ==P==.\n"),
		WithFragment(),
		WithCopyBooks(books),
	)
	require.NoError(t, err)

	outer := &LibraryText{Name: "OUTER", Copy: Pos{Line: 1, Column: 1}}
	inner := &LibraryText{Name: "INNER", Copy: Pos{Line: 2, Column: 1, Text: outer}}
	require.Equal(t, []*DataDescriptionEntry{
		{
			Pos:     Pos{Line: 1, Column: 1, Text: outer},
			Level:   5,
			Name:    &Word{Pos: Pos{Line: 1, Column: 4, Text: outer}, Value: "P-ID"},
			Clauses: []DataClause{&PictureClause{Pos: Pos{Line: 1, Column: 11, Text: outer}, Picture: "X"}},
		},
		{
			Pos:     Pos{Line: 1, Column: 1, Text: inner},
			Level:   5,
			Name:    &Word{Pos: Pos{Line: 1, Column: 4, Text: inner}, Value: "LONGER-PREFIX-NAME"},
			Clauses: []DataClause{&PictureClause{Pos: Pos{Line: 1, Column: 13, Text: inner}, Picture: "X"}},
		},
	}, f.Fragment.Entries)
}

// Every positioned error raised inside a nested member reports the member and
// the chain that reached it, through errors.As on Positioned as well as on its
// own type. The member sits two COPY statements deep, and the OUTER copy's
// REPLACING moves nothing in INNER, while INNER's own text is copied with a
// REPLACING that shortens a word ahead of the fault.
func TestParserCopyProvenanceErrors(t *testing.T) {
	t.Parallel()

	outer := &LibraryText{Name: "OUTER", Copy: Pos{Line: 1, Column: 1}}
	inner := &LibraryText{Name: "INNER", Copy: Pos{Line: 1, Column: 1, Text: outer}}

	testCases := []struct {
		name   string
		inner  string
		target func() any
		pos    Pos
	}{
		{
			name:   "copybook not found",
			inner:  "05 :T:-A PIC X.\n   COPY MISSING.\n",
			target: func() any { return &CopyBookNotFoundError{} },
			pos:    Pos{Line: 2, Column: 4, Text: inner},
		},
		{
			name:   "copybook cycle",
			inner:  "05 :T:-A PIC X. COPY OUTER.\n",
			target: func() any { return &CopyBookCycleError{} },
			pos:    Pos{Line: 1, Column: 17, Text: inner},
		},
		{
			name:   "prefixed literal naming a copybook",
			inner:  "05 :T:-A PIC X. COPY X'C1'.\n",
			target: func() any { return &InvalidLiteralError{} },
			pos:    Pos{Line: 1, Column: 22, Text: inner},
		},
		{
			name:   "unexpected character",
			inner:  "05 :T:-A PIC X. ~\n",
			target: func() any { return &UnexpectedCharacterError{} },
			pos:    Pos{Line: 1, Column: 17, Text: inner},
		},
		{
			name:   "unterminated string",
			inner:  "05 :T:-A PIC X VALUE 'OPEN.\n",
			target: func() any { return &UnterminatedStringError{} },
			pos:    Pos{Line: 1, Column: 22, Text: inner},
		},
		{
			name:   "unterminated pseudo-text",
			inner:  "05 :T:-A PIC X. COPY B REPLACING ==A\n",
			target: func() any { return &UnterminatedPseudoTextError{} },
			pos:    Pos{Line: 1, Column: 34, Text: inner},
		},
		{
			name:   "invalid literal",
			inner:  "05 :T:-A PIC X VALUE X'0'.\n",
			target: func() any { return &InvalidLiteralError{} },
			pos:    Pos{Line: 1, Column: 22, Text: inner},
		},
		{
			name:   "invalid level number",
			inner:  "05 :T:-A PIC X.\n   50 B PIC X.\n",
			target: func() any { return &InvalidLevelNumberError{} },
			pos:    Pos{Line: 2, Column: 4, Text: inner},
		},
		{
			name:   "unexpected token",
			inner:  "05 :T:-A PIC X VALUE.\n",
			target: func() any { return &UnexpectedTokenError{} },
			pos:    Pos{Line: 1, Column: 21, Text: inner},
		},
		{
			name:   "trailing token",
			inner:  "05 :T:-A PIC X ).\n",
			target: func() any { return &TrailingTokenError{} },
			pos:    Pos{Line: 1, Column: 17, Text: inner},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			books := MapCopyBooks(map[string]string{
				"OUTER": "COPY INNER REPLACING ==:T:== BY ==T==.\n",
				"INNER": tc.inner,
			})
			_, err := Parse(
				strings.NewReader("COPY OUTER.\n"),
				WithFragment(),
				WithCopyBooks(books),
			)
			wrapped := fmt.Errorf("reading copybook: %w", err)

			var p Positioned
			require.ErrorAs(t, wrapped, &p)
			require.Equal(t, tc.pos, p.Position())
			require.Equal(t, []Pos{{Line: 1, Column: 1, Text: outer}, {Line: 1, Column: 1}}, p.Position().CopyChain())

			target := tc.target()
			require.ErrorAs(t, wrapped, target)
			require.Equal(t, tc.pos, target.(Positioned).Position())
		})
	}
}

// A MissingCopyBookResolverError is raised by the first COPY statement Parse
// meets, before any library text has been read — with no resolver there is none
// to read — so it is always positioned in the source given to Parse.
func TestMissingCopyBookResolverIsInTheSource(t *testing.T) {
	t.Parallel()

	_, err := Parse(strings.NewReader("01 A.\n   COPY OUTER.\n"), WithFragment())

	var target MissingCopyBookResolverError
	require.True(t, errors.As(err, &target))
	require.Equal(t, Pos{Line: 2, Column: 4}, target.Pos)
	require.Nil(t, target.Pos.CopyChain())
}

// Each tokenizer error a copybook's text can raise is relocated with the tokens
// around it, so none is left reporting a position in the substituted text.
func TestTokenizerErrorsRelocate(t *testing.T) {
	t.Parallel()

	from := Pos{Line: 1, Column: 1}
	to := Pos{Line: 2, Column: 3, Text: &LibraryText{Name: "BOOK"}}

	testCases := []struct {
		name string
		err  relocatable
	}{
		{name: "unexpected character", err: UnexpectedCharacterError{Pos: from, R: '~'}},
		{name: "unterminated string", err: UnterminatedStringError{Pos: from}},
		{name: "unterminated pseudo-text", err: UnterminatedPseudoTextError{Pos: from}},
		{name: "invalid literal", err: InvalidLiteralError{Pos: from, Kind: LiteralHexadecimal, Literal: "X'0'", Reason: "odd"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			moved := tc.err.relocate(func(p Pos) Pos {
				require.Equal(t, from, p)
				return to
			})

			var p Positioned
			require.ErrorAs(t, moved, &p)
			require.Equal(t, to, p.Position())
			require.IsType(t, tc.err, moved)
		})
	}
}
