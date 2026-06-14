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
		opts     []TokenizeOption
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
			name: "separator period followed by a space",
			src:  ". ",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			name: "separator period followed by a newline",
			src:  ".\n",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			name: "separator period followed by CRLF",
			src:  ".\r\n",
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
			name: "single-quoted literal",
			src:  `'single'`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`'single'`)},
			},
		},
		{
			name: "empty double-quoted literal",
			src:  `""`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`""`)},
			},
		},
		{
			name: "empty single-quoted literal",
			src:  `''`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`''`)},
			},
		},
		{
			name: "doubled double-quote escape",
			src:  `"He said ""hi"""`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`"He said ""hi"""`)},
			},
		},
		{
			name: "doubled single-quote escape",
			src:  `'it''s'`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`'it''s'`)},
			},
		},
		{
			name: "integer literal",
			src:  "42",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("42")},
			},
		},
		{
			name: "zero literal",
			src:  "0",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("0")},
			},
		},
		{
			name: "negative integer literal",
			src:  "-7",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("-7")},
			},
		},
		{
			name: "positive integer literal",
			src:  "+5",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("+5")},
			},
		},
		{
			name: "fixed-point literal",
			src:  "3.14",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("3.14")},
			},
		},
		{
			name: "negative fixed-point literal",
			src:  "-2.95",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("-2.95")},
			},
		},
		{
			name: "floating-point literal",
			src:  "9.92E25",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("9.92E25")},
			},
		},
		{
			name: "floating-point literal with signed exponent",
			src:  "5.7E-14",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("5.7E-14")},
			},
		},
		{
			name: "trailing period is a separator, not a decimal point",
			src:  "5.",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("5")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			name: "decimal comma mode honors comma as decimal point",
			src:  "3,14",
			opts: []TokenizeOption{WithDecimalComma()},
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("3,14")},
			},
		},
		{
			name: "ALL figurative constant precedes a literal",
			src:  `ALL "X"`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("ALL")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenString, Value: []byte(`"X"`)},
			},
		},
		{
			name: "left parenthesis separator",
			src:  "(",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("(")},
			},
		},
		{
			name: "right parenthesis separator",
			src:  ")",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte(")")},
			},
		},
		{
			name: "colon separator",
			src:  ":",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte(":")},
			},
		},
		{
			name: "parenthesized word",
			src:  "(A)",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenIdentifier, Value: []byte("A")},
				{Pos: Pos{Line: 1, Column: 3}, Type: TokenSymbol, Value: []byte(")")},
			},
		},
		{
			name: "plus operator",
			src:  "+",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("+")},
			},
		},
		{
			name: "minus operator",
			src:  "-",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("-")},
			},
		},
		{
			name: "multiply operator",
			src:  "*",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("*")},
			},
		},
		{
			name: "divide operator",
			src:  "/",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("/")},
			},
		},
		{
			name: "exponentiation operator",
			src:  "**",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("**")},
			},
		},
		{
			name: "equality operator",
			src:  "=",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("=")},
			},
		},
		{
			name: "less-than operator",
			src:  "<",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("<")},
			},
		},
		{
			name: "greater-than operator",
			src:  ">",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte(">")},
			},
		},
		{
			name: "less-than-or-equal operator",
			src:  "<=",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("<=")},
			},
		},
		{
			name: "greater-than-or-equal operator",
			src:  ">=",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte(">=")},
			},
		},
		{
			name: "not-equal operator",
			src:  "<>",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("<>")},
			},
		},
		{
			name: "spaced less-than and equals are two operators",
			src:  "< =",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("<")},
				{Pos: Pos{Line: 1, Column: 3}, Type: TokenSymbol, Value: []byte("=")},
			},
		},
		{
			name: "spaced asterisks are two operators",
			src:  "* *",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("*")},
				{Pos: Pos{Line: 1, Column: 3}, Type: TokenSymbol, Value: []byte("*")},
			},
		},
		{
			name: "minus between operands is an operator",
			src:  "A - 1",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("A")},
				{Pos: Pos{Line: 1, Column: 3}, Type: TokenSymbol, Value: []byte("-")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenNumber, Value: []byte("1")},
			},
		},
		{
			name: "plus between operands is an operator",
			src:  "A + B",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("A")},
				{Pos: Pos{Line: 1, Column: 3}, Type: TokenSymbol, Value: []byte("+")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenIdentifier, Value: []byte("B")},
			},
		},
		{
			name: "separator comma is consumed like whitespace",
			src:  "1, 2",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("1")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenNumber, Value: []byte("2")},
			},
		},
		{
			name: "separator semicolon is consumed like whitespace",
			src:  "A; B",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("A")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenIdentifier, Value: []byte("B")},
			},
		},
		{
			// A multi-byte Unicode space (U+00A0 NBSP) is a boundary, the same
			// as skipWhitespace treats it, so the comma is a valid separator.
			name: "separator comma followed by unicode whitespace",
			src:  "A,\u00A0B",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("A")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenIdentifier, Value: []byte("B")},
			},
		},
		{
			name: "subscripts with a separator comma",
			src:  "(2, 3)",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenNumber, Value: []byte("2")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenNumber, Value: []byte("3")},
				{Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte(")")},
			},
		},
		{
			name: "subscripts separated by a space tokenize the same",
			src:  "(2 3)",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("(")},
				{Pos: Pos{Line: 1, Column: 2}, Type: TokenNumber, Value: []byte("2")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenNumber, Value: []byte("3")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenSymbol, Value: []byte(")")},
			},
		},
		{
			name: "data description entry with level number and picture digits",
			src:  "01 WS-COUNT PIC 9(3).",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte("01")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenIdentifier, Value: []byte("WS-COUNT")},
				{Pos: Pos{Line: 1, Column: 13}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 17}, Type: TokenPicture, Value: []byte("9(3)")},
				{Pos: Pos{Line: 1, Column: 21}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			name: "signed numeric picture with repeat and assumed decimal",
			src:  "PIC S9(4)V99",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenPicture, Value: []byte("S9(4)V99")},
			},
		},
		{
			name: "alphanumeric picture with repeat",
			src:  "PIC X(10)",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenPicture, Value: []byte("X(10)")},
			},
		},
		{
			name: "single alphabetic picture",
			src:  "PIC A",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenPicture, Value: []byte("A")},
			},
		},
		{
			name: "numeric-edited picture with float sign, insertion comma, and actual decimal",
			src:  "PIC -ZZ,ZZ9.99",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenPicture, Value: []byte("-ZZ,ZZ9.99")},
			},
		},
		{
			name: "floating currency picture with trailing CR",
			src:  "PIC $$,$$9.99CR",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenPicture, Value: []byte("$$,$$9.99CR")},
			},
		},
		{
			name: "numeric picture with trailing DB",
			src:  "PIC 9(5)DB",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenPicture, Value: []byte("9(5)DB")},
			},
		},
		{
			name: "actual decimal point inside a picture is kept",
			src:  "PIC ZZ9.99",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenPicture, Value: []byte("ZZ9.99")},
			},
		},
		{
			name: "insertion comma inside a picture is not a separator",
			src:  "PIC ZZ,ZZ9",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenPicture, Value: []byte("ZZ,ZZ9")},
			},
		},
		{
			name: "PICTURE IS picture string",
			src:  "PICTURE IS X(5)",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PICTURE")},
				{Pos: Pos{Line: 1, Column: 9}, Type: TokenIdentifier, Value: []byte("IS")},
				{Pos: Pos{Line: 1, Column: 12}, Type: TokenPicture, Value: []byte("X(5)")},
			},
		},
		{
			name: "PIC IS picture string",
			src:  "PIC IS 9(5)",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenIdentifier, Value: []byte("IS")},
				{Pos: Pos{Line: 1, Column: 8}, Type: TokenPicture, Value: []byte("9(5)")},
			},
		},
		{
			name: "separator period ends a picture",
			src:  "PIC X(10).",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenPicture, Value: []byte("X(10)")},
				{Pos: Pos{Line: 1, Column: 10}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			name: "whitespace ends a picture and the next clause tokenizes",
			src:  "PIC X(10) USAGE DISPLAY",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 5}, Type: TokenPicture, Value: []byte("X(10)")},
				{Pos: Pos{Line: 1, Column: 11}, Type: TokenIdentifier, Value: []byte("USAGE")},
				{Pos: Pos{Line: 1, Column: 17}, Type: TokenIdentifier, Value: []byte("DISPLAY")},
			},
		},
		{
			// An empty picture run emits no TokenPicture: the separator period is
			// left for the next dispatch rather than swallowed or turned into an
			// empty-valued picture token.
			name: "empty picture run emits no picture token",
			src:  "PIC.",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("PIC")},
				{Pos: Pos{Line: 1, Column: 4}, Type: TokenSymbol, Value: []byte(".")},
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
		{
			name: "full-line comment",
			src:  "*> a full-line comment",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("*> a full-line comment")},
			},
		},
		{
			name: "inline comment after code",
			src:  `DISPLAY "hi".  *> say hi`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte("DISPLAY")},
				{Pos: Pos{Line: 1, Column: 9}, Type: TokenString, Value: []byte(`"hi"`)},
				{Pos: Pos{Line: 1, Column: 13}, Type: TokenSymbol, Value: []byte(".")},
				{Pos: Pos{Line: 1, Column: 16}, Type: TokenComment, Value: []byte("*> say hi")},
			},
		},
		{
			// The line terminator is not part of the comment; it is consumed as
			// whitespace, and the next physical line tokenizes normally.
			name: "comment ends at the newline and the next line tokenizes",
			src:  "*> note\nSTOP RUN.\n",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("*> note")},
				{Pos: Pos{Line: 2, Column: 1}, Type: TokenIdentifier, Value: []byte("STOP")},
				{Pos: Pos{Line: 2, Column: 6}, Type: TokenIdentifier, Value: []byte("RUN")},
				{Pos: Pos{Line: 2, Column: 9}, Type: TokenSymbol, Value: []byte(".")},
			},
		},
		{
			name: "bare comment introducer",
			src:  "*>",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenComment, Value: []byte("*>")},
			},
		},
		{
			// *> has no meaning inside an alphanumeric literal (SPEC §Comments):
			// "a*>b" is one literal whose body is a*>b, not a comment.
			name: "comment introducer inside a literal is not a comment",
			src:  `"a*>b"`,
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`"a*>b"`)},
			},
		},
		{
			name: "concatenation operator",
			src:  "&",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenSymbol, Value: []byte("&")},
			},
		},
		{
			// Free-format continuation: a long literal is split across physical
			// lines and joined with &. Tokens may span lines; the & carries the
			// continuation. Joining the fragments is left to the parser.
			name: "alphanumeric literal continued with the concatenation operator",
			src:  "\"This is a long literal that \"\n    & \"spans two source lines.\"",
			expected: []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenString, Value: []byte(`"This is a long literal that "`)},
				{Pos: Pos{Line: 2, Column: 5}, Type: TokenSymbol, Value: []byte("&")},
				{Pos: Pos{Line: 2, Column: 7}, Type: TokenString, Value: []byte(`"spans two source lines."`)},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tokens, err := collect(Tokenize(strings.NewReader(tc.src), tc.opts...))

			require.NoError(t, err)
			require.Equal(t, tc.expected, tokens)
		})
	}
}

// Figurative constants are reserved words, not a distinct lexical class: the
// tokenizer emits them as ordinary COBOL words (TokenIdentifier) and the parser's
// keyword table recognizes them. This pins that contract for every spelling.
func TestTokenizerFigurativeConstants(t *testing.T) {
	t.Parallel()

	words := []string{
		"ZERO", "ZEROS", "ZEROES",
		"SPACE", "SPACES",
		"HIGH-VALUE", "HIGH-VALUES",
		"LOW-VALUE", "LOW-VALUES",
		"QUOTE", "QUOTES",
		"NULL", "NULLS",
		"ALL",
	}

	for _, word := range words {
		t.Run(word, func(t *testing.T) {
			t.Parallel()

			var tokens []Token
			for tok, err := range Tokenize(strings.NewReader(word)) {
				require.NoError(t, err)
				tokens = append(tokens, tok)
			}

			require.Equal(t, []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenIdentifier, Value: []byte(word)},
			}, tokens)
		})
	}
}

// Level-numbers are not a distinct lexical class: a level-number is lexically a
// NumericLiteral (1–2 digits) and the parser recognizes it by position (the
// start of a data description entry). This pins that every level-number spelling
// — 01–49 for the record hierarchy, plus 66 (RENAMES), 77 (independent), and 88
// (condition-name) — tokenizes as a single TokenNumber.
func TestTokenizerLevelNumbers(t *testing.T) {
	t.Parallel()

	levels := []string{"01", "02", "09", "10", "49", "66", "77", "88"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			t.Parallel()

			var tokens []Token
			for tok, err := range Tokenize(strings.NewReader(level)) {
				require.NoError(t, err)
				tokens = append(tokens, tok)
			}

			require.Equal(t, []Token{
				{Pos: Pos{Line: 1, Column: 1}, Type: TokenNumber, Value: []byte(level)},
			}, tokens)
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
		opts   []TokenizeOption
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
			name: "literal closed only by an escaped delimiter",
			src:  `"abc""`,
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
		{
			name: "separator comma not followed by a space",
			src:  "1,2",
			assert: func(t *testing.T, err error) {
				var target UnexpectedCharacterError
				require.ErrorAs(t, err, &target)
				require.Equal(t, ',', target.R)
				require.Equal(t, Pos{Line: 1, Column: 2}, target.Pos)
			},
		},
		{
			name: "separator semicolon not followed by a space",
			src:  "1;2",
			assert: func(t *testing.T, err error) {
				var target UnexpectedCharacterError
				require.ErrorAs(t, err, &target)
				require.Equal(t, ';', target.R)
				require.Equal(t, Pos{Line: 1, Column: 2}, target.Pos)
			},
		},
		{
			name: "separator comma is unavailable under DECIMAL-POINT IS COMMA",
			src:  "1, 2",
			opts: []TokenizeOption{WithDecimalComma()},
			assert: func(t *testing.T, err error) {
				var target UnexpectedCharacterError
				require.ErrorAs(t, err, &target)
				require.Equal(t, ',', target.R)
				require.Equal(t, Pos{Line: 1, Column: 2}, target.Pos)
			},
		},
		{
			name: "period not followed by a delimiter",
			src:  "A.B",
			assert: func(t *testing.T, err error) {
				var target UnexpectedCharacterError
				require.ErrorAs(t, err, &target)
				require.Equal(t, '.', target.R)
				require.Equal(t, Pos{Line: 1, Column: 2}, target.Pos)
			},
		},
		{
			name: "period after a number not followed by a delimiter",
			src:  "5.X",
			assert: func(t *testing.T, err error) {
				var target UnexpectedCharacterError
				require.ErrorAs(t, err, &target)
				require.Equal(t, '.', target.R)
				require.Equal(t, Pos{Line: 1, Column: 2}, target.Pos)
			},
		},
		{
			name: "period in decimal-comma mode is rejected",
			src:  "3.14",
			opts: []TokenizeOption{WithDecimalComma()},
			assert: func(t *testing.T, err error) {
				var target UnexpectedCharacterError
				require.ErrorAs(t, err, &target)
				require.Equal(t, '.', target.R)
				require.Equal(t, Pos{Line: 1, Column: 2}, target.Pos)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := collect(Tokenize(strings.NewReader(tc.src), tc.opts...))

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
