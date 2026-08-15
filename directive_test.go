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

// TestSkipListingDirectives is the direct test of the pass: a token stream in,
// the stream the parser sees out. The Parse-level tests show that copybooks and
// programs carrying these statements read; this one shows exactly which tokens
// the pass claims and — the part that matters — which it leaves alone.
func TestSkipListingDirectives(t *testing.T) {
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
		opts     []TokenizeOption
		expected []Token
	}{
		{
			name:     "every bare form",
			src:      "EJECT\nSKIP1\nSKIP2\nSKIP3\n",
			expected: nil,
		},
		{
			name:     "bare forms with separator periods",
			src:      "EJECT.\nSKIP1.\nSKIP2.\nSKIP3.\n",
			expected: nil,
		},
		{
			name:     "title with its literal operand and period",
			src:      "TITLE 'CUSTOMER RECORD'.\n",
			expected: nil,
		},
		{
			name:     "lowercase spellings",
			src:      "eject\nskip1\ntitle 'x'\n",
			expected: nil,
		},
		{
			// A directive word second on its line is an ordinary word: the "only
			// statement on the line" rule is what makes the recognition safe.
			name: "directive spelling later on the line",
			src:  "05 TITLE PIC X(2).\n",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("05")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenIdentifier, Value: []byte("TITLE")},
				{Pos: Pos{Line: 1, Column: 10}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 14}, Type: TokenPicture, Value: []byte("X(2)")},
				{Pos: Pos{Line: 1, Column: 18}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			// A TITLE statement always carries a literal, so a bare TITLE is a
			// word — here a paragraph name — and both it and its period survive.
			name: "bare title keeps its word and period",
			src:  "TITLE.\n",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("TITLE")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			// The operand has to be on the directive's own line too; a literal on
			// the next line belongs to whatever follows.
			name: "title whose literal is on the next line",
			src:  "TITLE\n'X'\n",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("TITLE")},
				{Pos: Pos{Line: 2, Column: 1}, Type: TokenString, Value: []byte("'X'")},
			},
		},
		{
			// The separator period is claimed only from the directive's own line,
			// so this one — terminating the entry above — survives.
			name: "period on a later line",
			src:  "05 B PIC X(2)\nSKIP1\n.\n",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("05")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenIdentifier, Value: []byte("B")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 10}, Type: TokenPicture, Value: []byte("X(2)")},
				{Pos: Pos{Line: 3, Column: 1}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			// Consecutive directives: dropping one must not make the next look
			// like it shares a line with an emitted token.
			name: "consecutive directives before an entry",
			src:  "SKIP1\nSKIP2\n01 A.\n",
			expected: []Token{
				{Pos: Pos{Line: 3, Column: 1}, Type: TokenNumber, Value: []byte("01")},
				{Pos: Pos{Line: 3, Column: 4}, Type: TokenIdentifier, Value: []byte("A")},
				{Pos: Pos{Line: 3, Column: 5}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			// Comments pass through untouched — they are the parser's to divert,
			// not this pass's — and a comment line does not stop the directive on
			// the line after it from opening its own line.
			name: "comment between directives",
			src:  "SKIP1\n*> note\nEJECT\n01 A.\n",
			expected: []Token{
				{Pos: Pos{Line: 2, Column: 1}, Type: TokenComment, Value: []byte("*> note")},
				{Pos: Pos{Line: 4, Column: 1}, Type: TokenNumber, Value: []byte("01")},
				{Pos: Pos{Line: 4, Column: 4}, Type: TokenIdentifier, Value: []byte("A")},
				{Pos: Pos{Line: 4, Column: 5}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			name: "fixed format Area A and Area B",
			src: "       EJECT\n" +
				"       01  A.\n" +
				"           SKIP1\n" +
				"           05  B PIC X(2).\n",
			opts: []TokenizeOption{WithFixedFormat()},
			expected: []Token{
				{Pos: Pos{Line: 2, Column: 8}, Type: TokenNumber, Value: []byte("01")},
				{Pos: Pos{Line: 2, Column: 12}, Type: TokenIdentifier, Value: []byte("A")},
				{Pos: Pos{Line: 2, Column: 13}, Type: TokenSymbol, Value: []byte(".")},
				{Pos: Pos{Line: 4, Column: 12}, Type: TokenNumber, Value: []byte("05")},
				{Pos: Pos{Line: 4, Column: 16}, Type: TokenIdentifier, Value: []byte("B")},
				{Pos: Pos{Line: 4, Column: 18}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 4, Column: 22}, Type: TokenPicture, Value: []byte("X(2)")},
				{Pos: Pos{Line: 4, Column: 26}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tokens, err := collect(skipListingDirectives(Tokenize(strings.NewReader(tc.src), tc.opts...)))
			require.NoError(t, err)
			require.Equal(t, tc.expected, tokens)
		})
	}
}

// TestSkipListingDirectivesPropagatesError pins the pass's error behavior: an
// error from the stream it wraps ends the pass, whether it arrives in the
// ordinary flow or in the middle of the lookahead a directive needs.
func TestSkipListingDirectivesPropagatesError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")

	seq := func(tokens ...Token) iter.Seq2[Token, error] {
		return func(yield func(Token, error) bool) {
			for _, tok := range tokens {
				if !yield(tok, nil) {
					return
				}
			}
			yield(Token{}, boom)
		}
	}

	testCases := []struct {
		name   string
		tokens []Token
	}{
		{
			name:   "error after an ordinary token",
			tokens: []Token{{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("01")}},
		},
		{
			// The lookahead for the optional separator period runs straight into
			// the error, which the pushback hands back to the ordinary path.
			name:   "error while scanning past a directive",
			tokens: []Token{{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("EJECT")}},
		},
		{
			// The lookahead for TITLE's literal operand meets it instead.
			name:   "error while scanning a title operand",
			tokens: []Token{{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("TITLE")}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got error
			for _, err := range skipListingDirectives(seq(tc.tokens...)) {
				if err != nil {
					got = err
				}
			}
			require.ErrorIs(t, got, boom)
		})
	}
}
