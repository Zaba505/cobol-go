// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"errors"
	"iter"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokenizer(t *testing.T) {
	t.Parallel()

	collect := func(seq iter.Seq2[Token, error]) ([]Token, error) {
		var tokens []Token
		for tok, err := range seq {
			if err != nil {
				return tokens, err
			}
			t.Log(tok)
			tokens = append(tokens, tok)
		}
		return tokens, nil
	}

	testCases := []struct {
		name     string
		src      string
		expected []Token
	}{
		{
			name:     "empty input yields no tokens",
			src:      "",
			expected: nil,
		},
		{
			name: "single word",
			src:  "DIVISION",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("DIVISION")},
			},
		},
		{
			name: "hyphenated word",
			src:  "PROGRAM-ID",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PROGRAM-ID")},
			},
		},
		{
			name: "period separator",
			src:  ".",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			name: "alphanumeric literal",
			src:  `"Hello, world!"`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`"Hello, world!"`)},
			},
		},
		{
			name: "minimal free-format program",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DISPLAY \"Hello, world!\".\n" +
				"    STOP RUN.\n",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("IDENTIFICATION")},
				{Pos: Pos{Line: 1, Column: 16}, Type: TokenIdentifier, Value: []byte("DIVISION")},
				{Pos: Pos{Line: 1, Column: 24}, Type: TokenSymbol, Value: []byte(".")},
				{Pos: Pos{Line: 2, Column: 1}, Type: TokenIdentifier, Value: []byte("PROGRAM-ID")},
				{Pos: Pos{Line: 2, Column: 11}, Type: TokenSymbol, Value: []byte(".")},
				{Pos: Pos{Line: 2, Column: 13}, Type: TokenIdentifier, Value: []byte("HELLO")},
				{Pos: Pos{Line: 2, Column: 18}, Type: TokenSymbol, Value: []byte(".")},
				{Pos: Pos{Line: 3, Column: 1}, Type: TokenIdentifier, Value: []byte("PROCEDURE")},
				{Pos: Pos{Line: 3, Column: 11}, Type: TokenIdentifier, Value: []byte("DIVISION")},
				{Pos: Pos{Line: 3, Column: 19}, Type: TokenSymbol, Value: []byte(".")},
				{Pos: Pos{Line: 4, Column: 5}, Type: TokenIdentifier, Value: []byte("DISPLAY")},
				{Pos: Pos{Line: 4, Column: 13}, Type: TokenString, Value: []byte(`"Hello, world!"`)},
				{Pos: Pos{Line: 4, Column: 28}, Type: TokenSymbol, Value: []byte(".")},
				{Pos: Pos{Line: 5, Column: 5}, Type: TokenIdentifier, Value: []byte("STOP")},
				{Pos: Pos{Line: 5, Column: 10}, Type: TokenIdentifier, Value: []byte("RUN")},
				{Pos: Pos{Line: 5, Column: 13}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tokens, err := collect(Tokenize(strings.NewReader(tc.src)))

			require.NoError(t, err)
			require.Equal(t, tc.expected, tokens)
		})
	}
}

func TestTokenizerErrors(t *testing.T) {
	t.Parallel()

	collect := func(seq iter.Seq2[Token, error]) error {
		for _, err := range seq {
			if err != nil {
				return err
			}
		}
		return nil
	}

	testCases := []struct {
		name   string
		src    string
		assert func(t *testing.T, err error)
	}{
		{
			name: "unterminated alphanumeric literal",
			src:  `"unterminated`,
			assert: func(t *testing.T, err error) {
				var target UnterminatedStringError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 1, Column: 1}, target.Pos)
			},
		},
		{
			name: "unexpected character",
			src:  "@",
			assert: func(t *testing.T, err error) {
				var target UnexpectedCharacterError
				require.ErrorAs(t, err, &target)
				require.Equal(t, '@', target.R)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := collect(Tokenize(strings.NewReader(tc.src)))

			require.Error(t, err)
			tc.assert(t, err)
		})
	}
}

// errReader yields data, then fails every subsequent read with err. It drives a
// non-EOF I/O failure partway through a literal.
type errReader struct {
	data []byte
	err  error
}

func (r *errReader) Read(p []byte) (int, error) {
	if len(r.data) > 0 {
		n := copy(p, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	return 0, r.err
}

// A genuine read error inside a literal must propagate unchanged, not be
// reclassified as an UnterminatedStringError (which is reserved for EOF).
func TestTokenizerPropagatesReadError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")

	var got error
	for _, err := range Tokenize(&errReader{data: []byte(`"ab`), err: wantErr}) {
		if err != nil {
			got = err
			break
		}
	}

	require.ErrorIs(t, got, wantErr)
	var unterminated UnterminatedStringError
	require.NotErrorAs(t, got, &unterminated)
}
