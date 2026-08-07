// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"bytes"
	"embed"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

//go:embed testdata
var testdataFS embed.FS

// A COPY statement is replaced by the library text it names before the parser
// runs, so the AST holds the copied entries and nothing of the statement. Each
// case asserts the whole resulting fragment, positions included: a copied entry's
// position is its position within the copybook, which is the one thing about
// this pass a caller can observe and be surprised by.
func TestParserCopy(t *testing.T) {
	t.Parallel()

	books := map[string]string{
		"CUSTREC": "01 CUSTOMER-RECORD.\n" +
			"   05 CUST-ID PIC 9(6).\n",
		"TAGGED": "05 :TAG:-ID   PIC 9(6).\n" +
			"05 :TAG:-NAME PIC X(20).\n",
		"PAYLIB.CUSTREC": "01 PAY-RECORD.\n",
		"lower.cpy":      "01 LOWER-RECORD.\n",
	}

	testCases := []struct {
		name     string
		src      string
		expected *File
	}{
		{
			name: "copy a whole record",
			src:  "COPY CUSTREC.\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 1,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUSTOMER-RECORD"},
						},
						{
							Pos:   Pos{Line: 2, Column: 4},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 2, Column: 7}, Value: "CUST-ID"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 2, Column: 15}, Picture: "9(6)"},
							},
						},
					},
				},
			},
		},
		{
			// COBOL words are case-insensitive, so the text-name is too; the
			// resolvers this package ships match without regard to case.
			name: "text-name matches without regard to case",
			src:  "copy custrec.\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 1,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUSTOMER-RECORD"},
						},
						{
							Pos:   Pos{Line: 2, Column: 4},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 2, Column: 7}, Value: "CUST-ID"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 2, Column: 15}, Picture: "9(6)"},
							},
						},
					},
				},
			},
		},
		{
			// OF and IN are interchangeable, and the library qualifies the
			// lookup: PAYLIB's CUSTREC is not the unqualified one.
			name: "library-qualified with OF",
			src:  "COPY CUSTREC OF PAYLIB.\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 1,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "PAY-RECORD"},
						},
					},
				},
			},
		},
		{
			name: "library-qualified with IN",
			src:  "COPY CUSTREC IN PAYLIB.\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 1,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "PAY-RECORD"},
						},
					},
				},
			},
		},
		{
			// A literal text-name names a file a COBOL word cannot spell.
			name: "literal text-name",
			src:  "COPY \"lower.cpy\".\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 1,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "LOWER-RECORD"},
						},
					},
				},
			},
		},
		{
			// The pervasive idiom the story exists for: :TAG: is three text
			// words, the replacement covers exactly their span, and the
			// -ID that follows is left welded to what went in.
			name: "replacing a pseudo-text tag",
			src: "01 CUSTOMER-RECORD.\n" +
				"COPY TAGGED REPLACING ==:TAG:== BY ==CUST==.\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 1,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUSTOMER-RECORD"},
						},
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUST-ID"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 1, Column: 14}, Picture: "9(6)"},
							},
						},
						{
							Pos:   Pos{Line: 2, Column: 1},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 2, Column: 4}, Value: "CUST-NAME"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 2, Column: 14}, Picture: "X(20)"},
							},
						},
					},
				},
			},
		},
		{
			// A REPLACING phrase is a list with no separator; it ends at the
			// first token that cannot begin an operand.
			name: "two replacements in one phrase",
			src:  "COPY TAGGED REPLACING ==:TAG:== BY ==CUST== ==PIC 9(6)== BY ==PIC 9(9)==.\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUST-ID"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 1, Column: 14}, Picture: "9(9)"},
							},
						},
						{
							Pos:   Pos{Line: 2, Column: 1},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 2, Column: 4}, Value: "CUST-NAME"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 2, Column: 14}, Picture: "X(20)"},
							},
						},
					},
				},
			},
		},
		{
			// The operands need not be pseudo-text: a bare word replaces a
			// single text word.
			name: "word operands",
			src:  "COPY CUSTREC REPLACING CUST-ID BY CUST-KEY.\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 1,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUSTOMER-RECORD"},
						},
						{
							Pos:   Pos{Line: 2, Column: 4},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 2, Column: 7}, Value: "CUST-KEY"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 2, Column: 16}, Picture: "9(6)"},
							},
						},
					},
				},
			},
		},
		{
			// An empty replacement deletes the matched text words.
			name: "empty replacement deletes",
			src:  "COPY CUSTREC REPLACING ==PIC 9(6)== BY == ==.\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 1,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUSTOMER-RECORD"},
						},
						{
							Pos:   Pos{Line: 2, Column: 4},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 2, Column: 7}, Value: "CUST-ID"},
						},
					},
				},
			},
		},
		{
			// A comment written inside the COPY statement has no copied text of
			// its own to lead, so it leads the copybook's first entry.
			name: "comment inside the statement leads the copied text",
			src:  "COPY CUSTREC *> from the shared library\n.\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:      Pos{Line: 1, Column: 1},
							Comments: []*Comment{{Pos: Pos{Line: 1, Column: 14}, Text: "from the shared library"}},
							Level:    1,
							Name:     &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUSTOMER-RECORD"},
						},
						{
							Pos:   Pos{Line: 2, Column: 4},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 2, Column: 7}, Value: "CUST-ID"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 2, Column: 15}, Picture: "9(6)"},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, err := Parse(
				strings.NewReader(tc.src),
				WithFragment(),
				WithCopyBooks(MapCopyBooks(books)),
			)

			require.NoError(t, err)
			require.Equal(t, tc.expected, f)
		})
	}
}

// A copybook may itself COPY, to any depth; a copybook that copies itself,
// directly or around a longer loop, is reported rather than expanded forever.
func TestParserCopyNested(t *testing.T) {
	t.Parallel()

	books := MapCopyBooks(map[string]string{
		"OUTER":  "01 CUSTOMER-RECORD.\nCOPY MIDDLE.\n",
		"MIDDLE": "   05 CUST-ID PIC 9(6).\nCOPY INNER.\n",
		"INNER":  "   05 CUST-NAME PIC X(20).\n",
		"SELF":   "COPY SELF.\n",
		"LOOP-A": "COPY LOOP-B.\n",
		"LOOP-B": "COPY LOOP-A.\n",
	})

	t.Run("three levels deep", func(t *testing.T) {
		t.Parallel()

		f, err := Parse(strings.NewReader("COPY OUTER.\n"), WithFragment(), WithCopyBooks(books))
		require.NoError(t, err)

		names := make([]string, 0, len(f.Fragment.Entries))
		for _, e := range f.Fragment.Entries {
			names = append(names, e.Name.Value)
		}
		require.Equal(t, []string{"CUSTOMER-RECORD", "CUST-ID", "CUST-NAME"}, names)
	})

	t.Run("a copybook that copies itself", func(t *testing.T) {
		t.Parallel()

		_, err := Parse(strings.NewReader("COPY SELF.\n"), WithFragment(), WithCopyBooks(books))

		var target CopyBookCycleError
		require.ErrorAs(t, err, &target)
		require.Equal(t, "SELF", target.Name)
		require.Equal(t, []string{"SELF", "SELF"}, target.Stack)
	})

	t.Run("a cycle through a second copybook", func(t *testing.T) {
		t.Parallel()

		_, err := Parse(strings.NewReader("COPY LOOP-A.\n"), WithFragment(), WithCopyBooks(books))

		var target CopyBookCycleError
		require.ErrorAs(t, err, &target)
		require.Equal(t, []string{"LOOP-A", "LOOP-B", "LOOP-A"}, target.Stack)
	})

	t.Run("the same copybook twice in sequence is not a cycle", func(t *testing.T) {
		t.Parallel()

		f, err := Parse(
			strings.NewReader("COPY INNER.\nCOPY INNER.\n"),
			WithFragment(),
			WithCopyBooks(books),
		)
		require.NoError(t, err)
		require.Len(t, f.Fragment.Entries, 2)
	})
}

// COPY is not confined to the DATA DIVISION: it is a text-manipulation statement
// and may stand anywhere library text can, the PROCEDURE DIVISION included.
func TestParserCopyWholeProgram(t *testing.T) {
	t.Parallel()

	books := MapCopyBooks(map[string]string{
		"CUSTREC": "05 :TAG:-ID   PIC 9(6).\n" +
			"05 :TAG:-NAME PIC X(20).\n",
		"GREET": "DISPLAY \"Hello\".\n",
	})

	src := "IDENTIFICATION DIVISION.\n" +
		"PROGRAM-ID. DEMO.\n" +
		"DATA DIVISION.\n" +
		"WORKING-STORAGE SECTION.\n" +
		"01 CUSTOMER.\n" +
		"COPY CUSTREC REPLACING ==:TAG:== BY ==CUST==.\n" +
		"PROCEDURE DIVISION.\n" +
		"MAIN-PARA.\n" +
		"COPY GREET.\n" +
		"STOP RUN.\n"

	f, err := Parse(strings.NewReader(src), WithCopyBooks(books))
	require.NoError(t, err)

	var printed bytes.Buffer
	require.NoError(t, Print(&printed, f))

	require.Contains(t, printed.String(), "05 CUST-ID PIC 9(6).")
	require.Contains(t, printed.String(), "05 CUST-NAME PIC X(20).")
	require.Contains(t, printed.String(), `DISPLAY "Hello".`)

	// A program assembled from copybooks round-trips like any other: the
	// printed source no longer copies anything, so it re-parses with no
	// resolver at all.
	second, err := Parse(&printed)
	require.NoError(t, err)
	require.Equal(t, withoutPos(f), withoutPos(second))
}

// A fixed-format program's copybooks are fixed format too: the copied text is
// tokenized in the same reference format as the source that copied it, so
// nothing beyond the resolver has to be configured.
func TestParserCopyFixedFormat(t *testing.T) {
	t.Parallel()

	books := MapCopyBooks(map[string]string{
		//        ----+----1----+----2----+----3----+----4
		"CUSTREC": "000100     05  :TAG:-ID   PIC 9(6).\n" +
			"000200     05  :TAG:-NAME PIC X(20).\n",
	})

	src := "000100* customer record\n" +
		"000200 01  CUSTOMER-RECORD.\n" +
		"000300     COPY CUSTREC REPLACING ==:TAG:== BY\n" +
		"000400     ==CUST==.\n"

	f, err := Parse(
		strings.NewReader(src),
		WithFragment(),
		WithSourceFormat(FixedFormat),
		WithCopyBooks(books),
	)
	require.NoError(t, err)

	require.Equal(t, &File{
		Fragment: &Fragment{
			Entries: []*DataDescriptionEntry{
				{
					Pos:      Pos{Line: 2, Column: 8},
					Comments: []*Comment{{Pos: Pos{Line: 1, Column: 7}, Text: "customer record"}},
					Level:    1,
					Name:     &Word{Pos: Pos{Line: 2, Column: 12}, Value: "CUSTOMER-RECORD"},
				},
				{
					Pos:   Pos{Line: 1, Column: 12},
					Level: 5,
					Name:  &Word{Pos: Pos{Line: 1, Column: 16}, Value: "CUST-ID"},
					Clauses: []DataClause{
						&PictureClause{Pos: Pos{Line: 1, Column: 26}, Picture: "9(6)"},
					},
				},
				{
					Pos:   Pos{Line: 2, Column: 12},
					Level: 5,
					Name:  &Word{Pos: Pos{Line: 2, Column: 16}, Value: "CUST-NAME"},
					Clauses: []DataClause{
						&PictureClause{Pos: Pos{Line: 2, Column: 26}, Picture: "X(20)"},
					},
				},
			},
		},
	}, f)
}

func TestParserCopyErrors(t *testing.T) {
	t.Parallel()

	books := MapCopyBooks(map[string]string{"CUSTREC": "01 CUSTOMER-RECORD.\n"})

	testCases := []struct {
		name     string
		src      string
		resolver CopyBookResolver
		assert   func(t *testing.T, err error)
	}{
		{
			name:     "no resolver configured",
			src:      "COPY CUSTREC.\n",
			resolver: nil,
			assert: func(t *testing.T, err error) {
				var target MissingCopyBookResolverError
				require.ErrorAs(t, err, &target)
				require.Equal(t, "CUSTREC", target.Name)
				require.Equal(t, Pos{Line: 1, Column: 1}, target.Pos)
			},
		},
		{
			name:     "copybook the resolver does not have",
			src:      "COPY MISSING.\n",
			resolver: books,
			assert: func(t *testing.T, err error) {
				var target CopyBookNotFoundError
				require.ErrorAs(t, err, &target)
				require.Equal(t, "MISSING", target.Name)
				require.Equal(t, Pos{Line: 1, Column: 1}, target.Pos)
				require.ErrorIs(t, err, fs.ErrNotExist)
			},
		},
		{
			name:     "a resolver failure other than a missing copybook",
			src:      "COPY CUSTREC.\n",
			resolver: CopyBookFunc(func(string, string) ([]byte, error) { return nil, errBrokenLibrary }),
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, errBrokenLibrary)
				require.NotErrorIs(t, err, fs.ErrNotExist)
			},
		},
		{
			name:     "no text-name",
			src:      "COPY.\n",
			resolver: books,
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, []TokenType{TokenIdentifier, TokenString}, target.Expected)
			},
		},
		{
			name:     "no terminating period",
			src:      "COPY CUSTREC\n",
			resolver: books,
			assert: func(t *testing.T, err error) {
				var target UnexpectedEndOfTokensError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			// Nothing of the right *type* stands where BY should, so the
			// mismatch is reported as a token error, exactly as the AST
			// parser's expectKeyword does.
			name:     "a replacement with pseudo-text where BY should be",
			src:      "COPY CUSTREC REPLACING ==A== ==B==.\n",
			resolver: books,
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, TokenPseudoText, target.Actual.Type)
				require.Equal(t, []TokenType{TokenIdentifier}, target.Expected)
			},
		},
		{
			// A word of the right type but the wrong spelling is the
			// keyword error.
			name:     "a replacement with the wrong word where BY should be",
			src:      "COPY CUSTREC REPLACING ==A== WITH ==B==.\n",
			resolver: books,
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, []string{"BY"}, target.Expected)
			},
		},
		{
			// The stream ending mid-statement reports what that state was
			// waiting for, rather than one fixed guess for every state.
			name:     "the stream ends where an operand should be",
			src:      "COPY CUSTREC REPLACING ==A== BY\n",
			resolver: books,
			assert: func(t *testing.T, err error) {
				var target UnexpectedEndOfTokensError
				require.ErrorAs(t, err, &target)
				require.Equal(t, copyOperandTokens, target.Expected)
			},
		},
		{
			name:     "a replacement with no second operand",
			src:      "COPY CUSTREC REPLACING ==A== BY.\n",
			resolver: books,
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, TokenSymbol, target.Actual.Type)
			},
		},
		{
			name:     "a library-name that is not a name",
			src:      "COPY CUSTREC OF 5.\n",
			resolver: books,
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, TokenNumber, target.Actual.Type)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := []ParseOption{WithFragment()}
			if tc.resolver != nil {
				opts = append(opts, WithCopyBooks(tc.resolver))
			}

			_, err := Parse(strings.NewReader(tc.src), opts...)

			require.Error(t, err)
			tc.assert(t, err)
		})
	}
}

// errBrokenLibrary stands for a resolver failure that is not a missing copybook —
// an unreadable file, a network library that is down — which must reach the
// caller unchanged rather than being reported as "not found".
var errBrokenLibrary = errors.New("library unavailable")

// applyReplacing works over text words rather than raw bytes, which is the whole
// reason the ==:TAG:== idiom composes: a pattern can never match part of a word,
// and the span it does match is replaced in place so its neighbours keep touching
// it.
func TestApplyReplacing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		src          string
		replacements []copyReplacement
		expected     string
	}{
		{
			name:         "no replacements returns the text unchanged",
			src:          "05 CUST-ID PIC X(8).",
			replacements: nil,
			expected:     "05 CUST-ID PIC X(8).",
		},
		{
			name:         "a tag welded to the word that follows it",
			src:          "05 :TAG:-CUSTOMER-ID PIC X(8).",
			replacements: []copyReplacement{{From: ":TAG:", To: "CUST"}},
			expected:     "05 CUST-CUSTOMER-ID PIC X(8).",
		},
		{
			name:         "every occurrence is replaced",
			src:          "05 :TAG:-ID PIC X(8).\n05 :TAG:-NAME PIC X(20).",
			replacements: []copyReplacement{{From: ":TAG:", To: "CUST"}},
			expected:     "05 CUST-ID PIC X(8).\n05 CUST-NAME PIC X(20).",
		},
		{
			name:         "a pattern never matches part of a word",
			src:          "05 CUSTOMER-ID PIC X(8).",
			replacements: []copyReplacement{{From: "CUST", To: "XX"}},
			expected:     "05 CUSTOMER-ID PIC X(8).",
		},
		{
			name:         "matching ignores case as COBOL words do",
			src:          "05 cust-id PIC X(8).",
			replacements: []copyReplacement{{From: "CUST-ID", To: "CUSTOMER-KEY"}},
			expected:     "05 CUSTOMER-KEY PIC X(8).",
		},
		{
			name:         "a multi-word pattern keeps the spacing around it",
			src:          "05 CUST-ID    PIC X(8).",
			replacements: []copyReplacement{{From: "PIC X(8)", To: "PIC X(16)"}},
			expected:     "05 CUST-ID    PIC X(16).",
		},
		{
			name:         "an empty replacement deletes the match",
			src:          "05 CUST-ID PIC X(8) VALUE SPACES.",
			replacements: []copyReplacement{{From: "VALUE SPACES", To: ""}},
			expected:     "05 CUST-ID PIC X(8) .",
		},
		{
			// The period ending an entry is a text word of its own; the one
			// inside a numeric literal is not, so 1.50 stays one word.
			name:         "a separator period is a text word but a decimal point is not",
			src:          "05 AMT PIC 9(3)V99 VALUE 1.50.",
			replacements: []copyReplacement{{From: "1.50", To: "2.75"}},
			expected:     "05 AMT PIC 9(3)V99 VALUE 2.75.",
		},
		{
			name:         "an alphanumeric literal is one text word and compares exactly",
			src:          `88 ACTIVE VALUE "A".`,
			replacements: []copyReplacement{{From: `"A"`, To: `"Y"`}},
			expected:     `88 ACTIVE VALUE "Y".`,
		},
		{
			name:         "a literal's case is data, not a spelling",
			src:          `88 ACTIVE VALUE "a".`,
			replacements: []copyReplacement{{From: `"A"`, To: `"Y"`}},
			expected:     `88 ACTIVE VALUE "a".`,
		},
		{
			// Scanning resumes after what was substituted, so a replacement
			// producing its own pattern substitutes once rather than forever.
			name:         "the substituted text is not rescanned",
			src:          "05 A PIC X.",
			replacements: []copyReplacement{{From: "A", To: "A A"}},
			expected:     "05 A A PIC X.",
		},
		{
			// Where two replacements could match at one position the first one
			// written wins, matching the order the REPLACING phrase gives them.
			name: "the first matching replacement wins",
			src:  "05 CUST-ID PIC X(8).",
			replacements: []copyReplacement{
				{From: "CUST-ID", To: "FIRST"},
				{From: "CUST-ID", To: "SECOND"},
			},
			expected: "05 FIRST PIC X(8).",
		},
		{
			// A pattern that spells nothing could only ever match nothing;
			// skipping it is what stops == == BY ==X== from inserting X between
			// every pair of text words.
			name:         "an empty pattern is skipped",
			src:          "05 CUST-ID PIC X(8).",
			replacements: []copyReplacement{{From: "", To: "XX"}},
			expected:     "05 CUST-ID PIC X(8).",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.expected, applyReplacing(tc.src, tc.replacements))
		})
	}
}

func TestTextWords(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      string
		expected []string
	}{
		{
			name:     "empty text has no words",
			src:      "",
			expected: nil,
		},
		{
			name:     "a data description entry",
			src:      "05 CUST-ID PIC X(8).",
			expected: []string{"05", "CUST-ID", "PIC", "X", "(", "8", ")", "."},
		},
		{
			name:     "a colon-delimited tag",
			src:      ":TAG:-CUST-ID",
			expected: []string{":", "TAG", ":", "-CUST-ID"},
		},
		{
			name:     "an alphanumeric literal is one word, spacing included",
			src:      `VALUE "A  B" .`,
			expected: []string{"VALUE", `"A  B"`, "."},
		},
		{
			name:     "a doubled delimiter does not close a literal",
			src:      `VALUE "it""s"`,
			expected: []string{"VALUE", `"it""s"`},
		},
		{
			name:     "a decimal point is part of its numeric literal",
			src:      "VALUE 1.50 ZERO",
			expected: []string{"VALUE", "1.50", "ZERO"},
		},
		{
			name:     "a comma separator stands alone but a decimal comma does not",
			src:      "A, 1,50",
			expected: []string{"A", ",", "1,50"},
		},
		{
			name:     "line breaks separate words like spaces",
			src:      "05 CUST-ID\n   PIC X(8)",
			expected: []string{"05", "CUST-ID", "PIC", "X", "(", "8", ")"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			words := textWords(tc.src)

			got := make([]string, 0, len(words))
			for _, w := range words {
				got = append(got, w.text)
				// Every word's span must address the text it reports, since
				// applyReplacing substitutes over the span, not the text.
				require.Equal(t, w.text, tc.src[w.start:w.end])
			}
			if tc.expected == nil {
				require.Empty(t, got)
				return
			}
			require.Equal(t, tc.expected, got)
		})
	}
}

// FSCopyBooks serves a copybook library out of anything that is an fs.FS — a
// directory through os.DirFS, an embed.FS, or the in-memory fstest.MapFS used
// here — and reconciles COBOL's case-insensitive text-names with a
// case-sensitive filesystem.
func TestFSCopyBooks(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"CUSTREC.cpy":        {Data: []byte("01 CUSTOMER-RECORD.\n")},
		"PAYLIB/CUSTREC.cpy": {Data: []byte("01 PAY-RECORD.\n")},
		"verbatim":           {Data: []byte("01 VERBATIM-RECORD.\n")},
		"other.cbl":          {Data: []byte("01 OTHER-RECORD.\n")},
	}

	testCases := []struct {
		name     string
		src      string
		suffixes []string
		expected string
	}{
		{
			name:     "the conventional .cpy extension is supplied",
			src:      "COPY CUSTREC.\n",
			expected: "CUSTOMER-RECORD",
		},
		{
			name:     "a lower-case text-name finds the upper-case file",
			src:      "COPY custrec.\n",
			expected: "CUSTOMER-RECORD",
		},
		{
			name:     "a library-name is a directory",
			src:      "COPY CUSTREC OF PAYLIB.\n",
			expected: "PAY-RECORD",
		},
		{
			name:     "a file with no extension is found as written",
			src:      "COPY \"verbatim\".\n",
			expected: "VERBATIM-RECORD",
		},
		{
			name:     "caller-supplied extensions replace the defaults",
			src:      "COPY other.\n",
			suffixes: []string{".cbl"},
			expected: "OTHER-RECORD",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, err := Parse(
				strings.NewReader(tc.src),
				WithFragment(),
				WithCopyBooks(FSCopyBooks(fsys, tc.suffixes...)),
			)

			require.NoError(t, err)
			require.Len(t, f.Fragment.Entries, 1)
			require.Equal(t, tc.expected, f.Fragment.Entries[0].Name.Value)
		})
	}

	t.Run("a copybook the library does not hold", func(t *testing.T) {
		t.Parallel()

		_, err := Parse(
			strings.NewReader("COPY MISSING.\n"),
			WithFragment(),
			WithCopyBooks(FSCopyBooks(fsys)),
		)

		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("a text-name that is not a valid path", func(t *testing.T) {
		t.Parallel()

		_, err := Parse(
			strings.NewReader("COPY \"../escape\".\n"),
			WithFragment(),
			WithCopyBooks(FSCopyBooks(fsys)),
		)

		require.ErrorIs(t, err, fs.ErrNotExist)
	})
}

// MapCopyBooks is the in-memory library: keys are text-names matched without
// regard to case, and a library-qualified copybook is keyed "LIBRARY.TEXT-NAME".
func TestMapCopyBooks(t *testing.T) {
	t.Parallel()

	books := map[string]string{
		"custrec":         "01 CUSTOMER-RECORD.\n",
		"PAYLIB.PAYREC":   "01 PAY-RECORD.\n",
		"UNQUALIFIED-ONE": "01 SHARED-RECORD.\n",
	}
	resolver := MapCopyBooks(books)

	t.Run("keys match without regard to case", func(t *testing.T) {
		t.Parallel()

		text, err := resolver.ResolveCopyBook("CUSTREC", "")
		require.NoError(t, err)
		require.Equal(t, "01 CUSTOMER-RECORD.\n", string(text))
	})

	t.Run("a library-qualified key", func(t *testing.T) {
		t.Parallel()

		text, err := resolver.ResolveCopyBook("PAYREC", "paylib")
		require.NoError(t, err)
		require.Equal(t, "01 PAY-RECORD.\n", string(text))
	})

	t.Run("a qualified lookup falls back to the bare text-name", func(t *testing.T) {
		t.Parallel()

		text, err := resolver.ResolveCopyBook("UNQUALIFIED-ONE", "ANYLIB")
		require.NoError(t, err)
		require.Equal(t, "01 SHARED-RECORD.\n", string(text))
	})

	t.Run("a missing copybook", func(t *testing.T) {
		t.Parallel()

		_, err := resolver.ResolveCopyBook("NOPE", "")
		require.ErrorIs(t, err, fs.ErrNotExist)
	})

	t.Run("the map is copied", func(t *testing.T) {
		t.Parallel()

		mutable := map[string]string{"ONE": "01 A.\n"}
		r := MapCopyBooks(mutable)
		delete(mutable, "ONE")

		text, err := r.ResolveCopyBook("ONE", "")
		require.NoError(t, err)
		require.Equal(t, "01 A.\n", string(text))
	})
}

// The real fixed-format copybook this package keeps as a fixture, pulled in by
// COPY out of an embed.FS — the shape a Go program shipping its own copybook
// library actually takes. Copying it must produce exactly what parsing it
// directly produces, which is the strongest statement of what COPY means: the
// library text replaces the statement, and nothing else changes.
func TestParserCopyFromEmbedFS(t *testing.T) {
	t.Parallel()

	library, err := fs.Sub(testdataFS, "testdata")
	require.NoError(t, err)

	// The text-name has an underscore, which no COBOL word may hold, so it has
	// to be written as a literal.
	copied, err := Parse(
		strings.NewReader("000100     COPY \"customer_copybook.cpy\".\n"),
		WithFragment(),
		WithSourceFormat(FixedFormat),
		WithCopyBooks(FSCopyBooks(library)),
	)
	require.NoError(t, err)

	direct, err := Parse(
		bytes.NewReader(mustReadFile(t, "testdata/customer_copybook.cpy")),
		WithFragment(),
		WithSourceFormat(FixedFormat),
	)
	require.NoError(t, err)

	require.Equal(t, direct, copied)
	require.NotEmpty(t, copied.Fragment.Entries)
}

// os.DirFS reaches the same library through the filesystem rather than an
// embedded copy, which is the other half of what one fs.FS-shaped resolver buys.
func TestParserCopyFromDirFS(t *testing.T) {
	t.Parallel()

	f, err := Parse(
		strings.NewReader("000100     COPY \"customer_copybook.cpy\".\n"),
		WithFragment(),
		WithSourceFormat(FixedFormat),
		WithCopyBooks(FSCopyBooks(os.DirFS("testdata"))),
	)

	require.NoError(t, err)
	require.Equal(t, "CUSTOMER-RECORD", f.Fragment.Entries[0].Name.Value)
}

func mustReadFile(t *testing.T, name string) []byte {
	t.Helper()

	b, err := os.ReadFile(name)
	require.NoError(t, err)
	return b
}

// A source with no COPY statement in it must reach the parser byte for byte the
// same whether or not a resolver was configured: the expansion pass sits in the
// pipeline unconditionally, so it has to be transparent.
func TestParserCopyPassThrough(t *testing.T) {
	t.Parallel()

	src := "01 CUSTOMER-RECORD.\n" +
		"*> the identifier\n" +
		"   05 CUST-ID PIC 9(6).\n"

	without, err := Parse(strings.NewReader(src), WithFragment())
	require.NoError(t, err)

	with, err := Parse(
		strings.NewReader(src),
		WithFragment(),
		WithCopyBooks(MapCopyBooks(map[string]string{"UNUSED": "01 X.\n"})),
	)
	require.NoError(t, err)

	require.Equal(t, without, with)
}
