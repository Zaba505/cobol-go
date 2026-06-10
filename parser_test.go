// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Parser tests drive the public Parse with real source strings and assert the
// resulting AST, positions included, against a hand-built expected *File (the
// avro-go/idl parser-test style this package is modeled on).
func TestParser(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      string
		expected *File
	}{
		{
			name:     "empty input parses to empty file",
			src:      "",
			expected: &File{},
		},
		{
			name: "minimal free-format program",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DISPLAY \"Hello, world!\".\n" +
				"    STOP RUN.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "HELLO"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Statements: []Statement{
									&DisplayStatement{
										Pos: Pos{Line: 4, Column: 5},
										Operands: []Type{
											&StringLiteral{Pos: Pos{Line: 4, Column: 13}, Value: `"Hello, world!"`},
										},
									},
									&StopStatement{Pos: Pos{Line: 5, Column: 5}, Run: true},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "environment division with both sections",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. ENV.\n" +
				"ENVIRONMENT DIVISION.\n" +
				"CONFIGURATION SECTION.\n" +
				"SOURCE-COMPUTER. GNU.\n" +
				"OBJECT-COMPUTER. GNU.\n" +
				"SPECIAL-NAMES.\n" +
				"    DECIMAL-POINT IS COMMA\n" +
				"    CURRENCY SIGN IS \"$\".\n" +
				"INPUT-OUTPUT SECTION.\n" +
				"FILE-CONTROL.\n" +
				"    SELECT OPTIONAL F ASSIGN TO \"f.dat\"\n" +
				"        ORGANIZATION IS LINE SEQUENTIAL\n" +
				"        ACCESS MODE IS DYNAMIC\n" +
				"        RECORD KEY IS F-KEY\n" +
				"        FILE STATUS IS F-STAT.\n" +
				"PROCEDURE DIVISION.\n" +
				"    STOP RUN.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "ENV"},
								},
							},
							&EnvironmentDivision{
								Pos: Pos{Line: 3, Column: 1},
								Configuration: &ConfigurationSection{
									Pos: Pos{Line: 4, Column: 1},
									SourceComputer: &SourceComputerParagraph{
										Pos:          Pos{Line: 5, Column: 1},
										ComputerName: &Word{Pos: Pos{Line: 5, Column: 18}, Value: "GNU"},
									},
									ObjectComputer: &ObjectComputerParagraph{
										Pos:          Pos{Line: 6, Column: 1},
										ComputerName: &Word{Pos: Pos{Line: 6, Column: 18}, Value: "GNU"},
									},
									SpecialNames: &SpecialNamesParagraph{
										Pos: Pos{Line: 7, Column: 1},
										Clauses: []SpecialNamesClause{
											&DecimalPointClause{Pos: Pos{Line: 8, Column: 5}},
											&CurrencySignClause{
												Pos:  Pos{Line: 9, Column: 5},
												Sign: &StringLiteral{Pos: Pos{Line: 9, Column: 22}, Value: `"$"`},
											},
										},
									},
								},
								InputOutput: &InputOutputSection{
									Pos: Pos{Line: 10, Column: 1},
									FileControl: &FileControlParagraph{
										Pos: Pos{Line: 11, Column: 1},
										Entries: []*FileControlEntry{
											{
												Pos:      Pos{Line: 12, Column: 5},
												Optional: true,
												Name:     &Word{Pos: Pos{Line: 12, Column: 21}, Value: "F"},
												Assign:   &StringLiteral{Pos: Pos{Line: 12, Column: 33}, Value: `"f.dat"`},
												Clauses: []SelectClause{
													&OrganizationClause{Pos: Pos{Line: 13, Column: 9}, Organization: "LINE SEQUENTIAL"},
													&AccessClause{Pos: Pos{Line: 14, Column: 9}, Mode: "DYNAMIC"},
													&RecordKeyClause{
														Pos:  Pos{Line: 15, Column: 9},
														Name: &Word{Pos: Pos{Line: 15, Column: 23}, Value: "F-KEY"},
													},
													&FileStatusClause{
														Pos:  Pos{Line: 16, Column: 9},
														Name: &Word{Pos: Pos{Line: 16, Column: 24}, Value: "F-STAT"},
													},
												},
											},
										},
									},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 17, Column: 1},
								Statements: []Statement{
									&StopStatement{Pos: Pos{Line: 18, Column: 5}, Run: true},
								},
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

			f, err := Parse(strings.NewReader(tc.src))

			require.NoError(t, err)
			require.Equal(t, tc.expected, f)
		})
	}
}

// TestParserErrors pins the position-aware typed errors the parser reports.
func TestParserErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		src    string
		assert func(t *testing.T, err error)
	}{
		{
			name: "misspelled division keyword",
			src:  "FOO DIVISION.",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 1, Column: 1}, target.Actual.Pos)
				// The message names the keyword and surfaces its spelling.
				require.Contains(t, err.Error(), "unexpected keyword")
				require.Contains(t, err.Error(), `"FOO"`)
			},
		},
		{
			name: "non-identifier where a division is expected",
			src:  ".",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 1, Column: 1}, target.Actual.Pos)
			},
		},
		{
			name: "missing DIVISION after IDENTIFICATION",
			src:  "IDENTIFICATION.\nPROGRAM-ID. HELLO.",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 1, Column: 15}, target.Actual.Pos)
			},
		},
		{
			name: "misspelled verb in procedure body",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    FOO.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 5}, target.Actual.Pos)
			},
		},
		{
			name: "non-identifier where a statement is expected",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    .\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 5}, target.Actual.Pos)
			},
		},
		{
			name: "missing SECTION after CONFIGURATION",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. ENV.\n" +
				"ENVIRONMENT DIVISION.\n" +
				"CONFIGURATION.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 14}, target.Actual.Pos)
			},
		},
		{
			name: "invalid ORGANIZATION value",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. ENV.\n" +
				"ENVIRONMENT DIVISION.\n" +
				"INPUT-OUTPUT SECTION.\n" +
				"FILE-CONTROL.\n" +
				"    SELECT F ASSIGN TO \"f.dat\"\n" +
				"        ORGANIZATION IS BOGUS.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 7, Column: 25}, target.Actual.Pos)
			},
		},
		{
			name: "unimplemented DATA division after environment",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. ENV.\n" +
				"ENVIRONMENT DIVISION.\n" +
				"DATA DIVISION.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 1}, target.Actual.Pos)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse(strings.NewReader(tc.src))

			require.Error(t, err)
			tc.assert(t, err)
		})
	}
}
