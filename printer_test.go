// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrinter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    *File
		expected string
	}{
		{
			name:     "empty file prints nothing",
			input:    &File{},
			expected: "",
		},
		{
			name: "hello world program",
			// The printer never reads Pos, so the hand-built AST omits it.
			input: &File{
				Programs: []*Program{
					{
						Divisions: []Division{
							&IdentificationDivision{
								ProgramID: &ProgramID{Name: &Word{Value: "hello"}},
							},
							&ProcedureDivision{
								Statements: []Statement{
									&DisplayStatement{
										Operands: []Type{
											&StringLiteral{Value: `"Hello, world!"`},
										},
									},
									&StopStatement{Run: true},
								},
							},
						},
					},
				},
			},
			expected: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. hello.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DISPLAY \"Hello, world!\".\n" +
				"    STOP RUN.\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := Print(&buf, tc.input)

			require.NoError(t, err)
			require.Equal(t, tc.expected, buf.String())
		})
	}
}

// TestPrinterRoundTrip pins the Parse -> Print -> Parse -> Equal contract.
// Every printer method added later must have a round-trip case here.
func TestPrinterRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
	}{
		{
			name: "empty source",
			src:  "",
		},
		{
			name: "hello world program",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. hello.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DISPLAY \"Hello, world!\".\n" +
				"    STOP RUN.\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file1, err := Parse(strings.NewReader(tc.src))
			require.NoError(t, err)

			var buf bytes.Buffer
			err = Print(&buf, file1)
			require.NoError(t, err)

			file2, err := Parse(strings.NewReader(buf.String()))
			require.NoError(t, err)

			// The printer reformats canonically, so positions shift between the
			// original source and the printed output; compare structure only.
			require.Equal(t, withoutPos(file1), withoutPos(file2))
		})
	}
}

// withoutPos zeroes every Pos in f and returns it, so round-trip comparisons
// assert AST structure while ignoring the column information the printer is free
// to choose (SPEC.md "Reference format independence").
func withoutPos(f *File) *File {
	for _, prog := range f.Programs {
		prog.Pos = Pos{}
		for _, div := range prog.Divisions {
			switch d := div.(type) {
			case *IdentificationDivision:
				d.Pos = Pos{}
				if d.ProgramID != nil {
					d.ProgramID.Pos = Pos{}
					clearTypePos(d.ProgramID.Name)
				}
			case *ProcedureDivision:
				d.Pos = Pos{}
				for _, stmt := range d.Statements {
					switch s := stmt.(type) {
					case *DisplayStatement:
						s.Pos = Pos{}
						for _, op := range s.Operands {
							clearTypePos(op)
						}
					case *StopStatement:
						s.Pos = Pos{}
					}
				}
			}
		}
	}
	return f
}

// clearTypePos zeroes the Pos of a value node.
func clearTypePos(v Type) {
	switch n := v.(type) {
	case *Word:
		n.Pos = Pos{}
	case *StringLiteral:
		n.Pos = Pos{}
	}
}
