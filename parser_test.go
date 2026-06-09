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
			name: "unknown division keyword",
			src:  "FOO DIVISION.",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
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
