// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"bytes"
	"os"
	"path/filepath"
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
			name: "data division",
			input: &File{
				Programs: []*Program{
					{
						Divisions: []Division{
							&IdentificationDivision{
								ProgramID: &ProgramID{Name: &Word{Value: "DATADEMO"}},
							},
							&DataDivision{
								File: &FileSection{
									Entries: []*FileDescriptionEntry{
										{
											Kind: "FD",
											Name: &Word{Value: "CUST-FILE"},
											Records: []*DataDescriptionEntry{
												{Level: 1, Name: &Word{Value: "CUST-REC"}},
												{Level: 5, Name: &Word{Value: "CUST-ID"}, Clauses: []DataClause{&PictureClause{Picture: "9(5)"}}},
												{Level: 5, Filler: true, Clauses: []DataClause{&PictureClause{Picture: "X(20)"}}},
											},
										},
									},
								},
								WorkingStorage: &DataSection{
									Entries: []*DataDescriptionEntry{
										{Level: 1, Name: &Word{Value: "COUNTER"}, Clauses: []DataClause{
											&PictureClause{Picture: "9(2)"},
											&UsageClause{Usage: "COMP-3"},
											&ValueClause{Values: []ValueSpec{{From: &Word{Value: "ZERO"}}}},
										}},
										{Level: 1, Name: &Word{Value: "STATUS-FLAG"}, Clauses: []DataClause{
											&PictureClause{Picture: "X"},
											&ValueClause{Values: []ValueSpec{{From: &StringLiteral{Value: `"N"`}}}},
										}},
										{Level: 88, Name: &Word{Value: "PENDING"}, Clauses: []DataClause{
											&ValueClause{Values: []ValueSpec{{From: &StringLiteral{Value: `"A"`}, Through: &StringLiteral{Value: `"M"`}}}},
										}},
										{Level: 5, Name: &Word{Value: "ITEM"}, Clauses: []DataClause{
											&OccursClause{
												Min:         &NumericLiteral{Value: "1"},
												Max:         &NumericLiteral{Value: "10"},
												DependingOn: &Word{Value: "N"},
												Keys:        []OccursKey{{Ascending: true, Name: &Word{Value: "K"}}},
												IndexedBy:   &Word{Value: "IDX"},
											},
											&PictureClause{Picture: "9(4)"},
										}},
										{Level: 5, Name: &Word{Value: "F3"}, Clauses: []DataClause{
											&PictureClause{Picture: "S9"},
											&SignClause{Position: "LEADING", Separate: true},
											&JustifiedClause{},
											&SynchronizedClause{Direction: "LEFT"},
											&BlankWhenZeroClause{},
											&GlobalClause{},
											&ExternalClause{},
										}},
										{Level: 66, Name: &Word{Value: "RN"}, Clauses: []DataClause{
											&RenamesClause{From: &Word{Value: "A"}, Through: &Word{Value: "B"}},
										}},
									},
								},
								Linkage: &DataSection{
									Entries: []*DataDescriptionEntry{
										{Level: 1, Name: &Word{Value: "LK"}, Clauses: []DataClause{&PictureClause{Picture: "X(10)"}}},
									},
								},
							},
							&ProcedureDivision{
								Paragraphs: []*Paragraph{
									{Sentences: []*Sentence{{Statements: []Statement{&StopStatement{Run: true}}}}},
								},
							},
						},
					},
				},
			},
			expected: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. DATADEMO.\n" +
				"DATA DIVISION.\n" +
				"FILE SECTION.\n" +
				"FD CUST-FILE.\n" +
				"    01 CUST-REC.\n" +
				"    05 CUST-ID PIC 9(5).\n" +
				"    05 FILLER PIC X(20).\n" +
				"WORKING-STORAGE SECTION.\n" +
				"    01 COUNTER PIC 9(2) USAGE COMP-3 VALUE ZERO.\n" +
				"    01 STATUS-FLAG PIC X VALUE \"N\".\n" +
				"    88 PENDING VALUE \"A\" THROUGH \"M\".\n" +
				"    05 ITEM OCCURS 1 TO 10 TIMES DEPENDING ON N ASCENDING KEY IS K INDEXED BY IDX PIC 9(4).\n" +
				"    05 F3 PIC S9 SIGN IS LEADING SEPARATE CHARACTER JUSTIFIED RIGHT SYNCHRONIZED LEFT BLANK WHEN ZERO GLOBAL EXTERNAL.\n" +
				"    66 RN RENAMES A THROUGH B.\n" +
				"LINKAGE SECTION.\n" +
				"    01 LK PIC X(10).\n" +
				"PROCEDURE DIVISION.\n" +
				"    STOP RUN.\n",
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
								Paragraphs: []*Paragraph{
									{
										Sentences: []*Sentence{
											{Statements: []Statement{
												&DisplayStatement{
													Operands: []Type{
														&StringLiteral{Value: `"Hello, world!"`},
													},
												},
											}},
											{Statements: []Statement{&StopStatement{Run: true}}},
										},
									},
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
		{
			name: "procedure division statements",
			input: &File{
				Programs: []*Program{
					{
						Divisions: []Division{
							&IdentificationDivision{
								ProgramID: &ProgramID{Name: &Word{Value: "P"}},
							},
							&ProcedureDivision{
								Paragraphs: []*Paragraph{
									{Sentences: []*Sentence{
										{Statements: []Statement{&MoveStatement{
											Corresponding: true,
											Source:        &Identifier{Name: &Word{Value: "G"}},
											Targets:       []*Identifier{{Name: &Word{Value: "X"}}},
										}}},
										{Statements: []Statement{&MoveStatement{
											Source: &Identifier{
												Name:       &Word{Value: "A"},
												Subscripts: []Expr{&Identifier{Name: &Word{Value: "I"}}},
											},
											Targets: []*Identifier{{Name: &Word{Value: "B"}}},
										}}},
										{Statements: []Statement{&DisplayStatement{
											Operands: []Type{
												&StringLiteral{Value: `"x"`},
												&Identifier{Name: &Word{Value: "Y"}},
											},
											Upon: &Word{Value: "CONSOLE"},
										}}},
										{Statements: []Statement{&DisplayStatement{
											Operands:    []Type{&Identifier{Name: &Word{Value: "Z"}}},
											NoAdvancing: true,
										}}},
										{Statements: []Statement{&AcceptStatement{
											Target: &Identifier{Name: &Word{Value: "W"}},
											From:   &Word{Value: "DATE"},
										}}},
										{Statements: []Statement{&GoToStatement{
											Targets:     []*Word{{Value: "P1"}, {Value: "P2"}},
											DependingOn: &Identifier{Name: &Word{Value: "IDX"}},
										}}},
										{Statements: []Statement{&ContinueStatement{}}},
										{Statements: []Statement{&StopStatement{Literal: &StringLiteral{Value: `"DONE"`}}}},
									}},
								},
							},
						},
					},
				},
			},
			expected: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    MOVE CORRESPONDING G TO X.\n" +
				"    MOVE A(I) TO B.\n" +
				"    DISPLAY \"x\" Y UPON CONSOLE.\n" +
				"    DISPLAY Z WITH NO ADVANCING.\n" +
				"    ACCEPT W FROM DATE.\n" +
				"    GO TO P1 P2 DEPENDING ON IDX.\n" +
				"    CONTINUE.\n" +
				"    STOP \"DONE\".\n",
		},
		{
			name: "procedure division arithmetic",
			input: &File{
				Programs: []*Program{
					{
						Divisions: []Division{
							&IdentificationDivision{
								ProgramID: &ProgramID{Name: &Word{Value: "A"}},
							},
							&ProcedureDivision{
								Paragraphs: []*Paragraph{
									{Sentences: []*Sentence{
										{Statements: []Statement{&ArithmeticStatement{
											Verb:      "ADD",
											Operands:  []Type{&Identifier{Name: &Word{Value: "A"}}, &Identifier{Name: &Word{Value: "B"}}},
											Connector: "TO",
											Targets:   []*Identifier{{Name: &Word{Value: "C"}}},
										}}},
										{Statements: []Statement{&ArithmeticStatement{
											Verb:      "SUBTRACT",
											Operands:  []Type{&Identifier{Name: &Word{Value: "A"}}},
											Connector: "FROM",
											Targets:   []*Identifier{{Name: &Word{Value: "B"}}},
											Giving:    &Identifier{Name: &Word{Value: "C"}},
											Rounded:   true,
										}}},
										{Statements: []Statement{&ComputeStatement{
											Targets: []ComputeTarget{{Name: &Identifier{Name: &Word{Value: "Z"}}, Rounded: true}},
											Expr: &BinaryExpr{
												Op: "*",
												Left: &ParenExpr{Expr: &BinaryExpr{
													Op:    "+",
													Left:  &Identifier{Name: &Word{Value: "A"}},
													Right: &Identifier{Name: &Word{Value: "B"}},
												}},
												Right: &Identifier{Name: &Word{Value: "C"}},
											},
											EndScope: true,
										}}},
									}},
								},
							},
						},
					},
				},
			},
			expected: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. A.\n" +
				"PROCEDURE DIVISION.\n" +
				"    ADD A B TO C.\n" +
				"    SUBTRACT A FROM B GIVING C ROUNDED.\n" +
				"    COMPUTE Z ROUNDED = (A + B) * C END-COMPUTE.\n",
		},
		{
			name: "procedure division if",
			input: &File{
				Programs: []*Program{
					{
						Divisions: []Division{
							&IdentificationDivision{
								ProgramID: &ProgramID{Name: &Word{Value: "P"}},
							},
							&ProcedureDivision{
								Paragraphs: []*Paragraph{
									{Sentences: []*Sentence{
										{Statements: []Statement{&IfStatement{
											Cond: &RelationCondition{
												Left:  &Identifier{Name: &Word{Value: "A"}},
												Op:    ">",
												Right: &Identifier{Name: &Word{Value: "B"}},
											},
											Then:    []Statement{&MoveStatement{Source: &NumericLiteral{Value: "1"}, Targets: []*Identifier{{Name: &Word{Value: "C"}}}}},
											Else:    []Statement{&MoveStatement{Source: &NumericLiteral{Value: "2"}, Targets: []*Identifier{{Name: &Word{Value: "C"}}}}},
											HasElse: true,
											EndIf:   true,
										}}},
										{Statements: []Statement{&IfStatement{
											Cond: &ConditionNameCondition{Name: &Identifier{Name: &Word{Value: "FLAG"}}},
											Then: []Statement{&DisplayStatement{Operands: []Type{&StringLiteral{Value: `"x"`}}}},
										}}},
									}},
								},
							},
						},
					},
				},
			},
			expected: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    IF A > B\n" +
				"    MOVE 1 TO C\n" +
				"    ELSE\n" +
				"    MOVE 2 TO C\n" +
				"    END-IF.\n" +
				"    IF FLAG\n" +
				"    DISPLAY \"x\".\n",
		},
		{
			name: "environment division",
			input: &File{
				Programs: []*Program{
					{
						Divisions: []Division{
							&IdentificationDivision{
								ProgramID: &ProgramID{Name: &Word{Value: "ENV"}},
							},
							&EnvironmentDivision{
								Configuration: &ConfigurationSection{
									SourceComputer: &SourceComputerParagraph{
										ComputerName:  &Word{Value: "GNU"},
										DebuggingMode: true,
									},
									ObjectComputer: &ObjectComputerParagraph{
										ComputerName: &Word{Value: "GNU"},
									},
									SpecialNames: &SpecialNamesParagraph{
										Clauses: []SpecialNamesClause{
											&DecimalPointClause{},
											&CurrencySignClause{Sign: &StringLiteral{Value: `"$"`}},
										},
									},
								},
								InputOutput: &InputOutputSection{
									FileControl: &FileControlParagraph{
										Entries: []*FileControlEntry{
											{
												Optional: true,
												Name:     &Word{Value: "LOG-FILE"},
												Assign:   &StringLiteral{Value: `"log.txt"`},
												Clauses: []SelectClause{
													&OrganizationClause{Organization: "LINE SEQUENTIAL"},
													&AccessClause{Mode: "SEQUENTIAL"},
												},
											},
											{
												Name:   &Word{Value: "CUST-FILE"},
												Assign: &StringLiteral{Value: `"customers.dat"`},
												Clauses: []SelectClause{
													&OrganizationClause{Organization: "INDEXED"},
													&AccessClause{Mode: "DYNAMIC"},
													&RecordKeyClause{Name: &Word{Value: "CUST-ID"}},
													&FileStatusClause{Name: &Word{Value: "WS-FILE-STATUS"}},
												},
											},
										},
									},
								},
							},
							&ProcedureDivision{
								Paragraphs: []*Paragraph{
									{Sentences: []*Sentence{{Statements: []Statement{&StopStatement{Run: true}}}}},
								},
							},
						},
					},
				},
			},
			expected: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. ENV.\n" +
				"ENVIRONMENT DIVISION.\n" +
				"CONFIGURATION SECTION.\n" +
				"SOURCE-COMPUTER. GNU WITH DEBUGGING MODE.\n" +
				"OBJECT-COMPUTER. GNU.\n" +
				"SPECIAL-NAMES.\n" +
				"    DECIMAL-POINT IS COMMA\n" +
				"    CURRENCY SIGN IS \"$\".\n" +
				"INPUT-OUTPUT SECTION.\n" +
				"FILE-CONTROL.\n" +
				"    SELECT OPTIONAL LOG-FILE ASSIGN TO \"log.txt\"\n" +
				"        ORGANIZATION IS LINE SEQUENTIAL\n" +
				"        ACCESS MODE IS SEQUENTIAL.\n" +
				"    SELECT CUST-FILE ASSIGN TO \"customers.dat\"\n" +
				"        ORGANIZATION IS INDEXED\n" +
				"        ACCESS MODE IS DYNAMIC\n" +
				"        RECORD KEY IS CUST-ID\n" +
				"        FILE STATUS IS WS-FILE-STATUS.\n" +
				"PROCEDURE DIVISION.\n" +
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
			name: "data division entries",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. DATADEMO.\n" +
				"DATA DIVISION.\n" +
				"FILE SECTION.\n" +
				"FD CUST-FILE.\n" +
				"01 CUST-RECORD.\n" +
				"    05 CUST-ID PIC 9(5).\n" +
				"    05 FILLER PIC X(20).\n" +
				"WORKING-STORAGE SECTION.\n" +
				"01 COUNTER PIC 9(2) USAGE COMP-3 VALUE ZERO.\n" +
				"01 TOTAL PIC S9(5)V99 VALUE 0.\n" +
				"01 STATUS-FLAG PIC X VALUE \"N\".\n" +
				"    88 DONE VALUE \"Y\".\n" +
				"    88 PENDING VALUE \"A\" THROUGH \"M\".\n" +
				"01 TABLE-DATA.\n" +
				"    05 ITEM OCCURS 10 TIMES PIC 9(4).\n" +
				"01 ALT REDEFINES TABLE-DATA PIC X(40).\n" +
				"01 FLAGS.\n" +
				"    05 F1 PIC X JUSTIFIED RIGHT.\n" +
				"    05 F2 PIC 9 BLANK WHEN ZERO.\n" +
				"    05 F3 PIC S9 SIGN IS LEADING SEPARATE.\n" +
				"    05 F4 PIC 9 SYNCHRONIZED LEFT.\n" +
				"    05 F5 PIC X GLOBAL.\n" +
				"    05 F6 PIC X EXTERNAL.\n" +
				"66 RENAME-FIELD RENAMES F1 THROUGH F2.\n" +
				"LINKAGE SECTION.\n" +
				"01 LK-PARM PIC X(10).\n" +
				"PROCEDURE DIVISION.\n" +
				"    STOP RUN.\n",
		},
		{
			name: "hello world program",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. hello.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DISPLAY \"Hello, world!\".\n" +
				"    STOP RUN.\n",
		},
		{
			name: "environment division",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. ENV.\n" +
				"ENVIRONMENT DIVISION.\n" +
				"CONFIGURATION SECTION.\n" +
				"SOURCE-COMPUTER. GNU WITH DEBUGGING MODE.\n" +
				"OBJECT-COMPUTER. GNU.\n" +
				"SPECIAL-NAMES.\n" +
				"    DECIMAL-POINT IS COMMA\n" +
				"    CURRENCY SIGN IS \"$\".\n" +
				"INPUT-OUTPUT SECTION.\n" +
				"FILE-CONTROL.\n" +
				"    SELECT OPTIONAL LOG-FILE ASSIGN TO \"log.txt\"\n" +
				"        ORGANIZATION IS LINE SEQUENTIAL\n" +
				"        ACCESS MODE IS SEQUENTIAL.\n" +
				"PROCEDURE DIVISION.\n" +
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

// TestRoundTripFromTestdata is the reusable, fixture-driven round-trip harness:
// it reads a golden COBOL program from testdata/, runs Parse -> Print -> Parse,
// and asserts the two ASTs are equal ignoring positions. Later fixture-based
// stories extend it by dropping a file in testdata/ and adding one table row.
func TestRoundTripFromTestdata(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		fixture string
	}{
		{name: "hello_cob", fixture: "hello.cob"},
		{name: "environment_cob", fixture: "environment.cob"},
		{name: "data_cob", fixture: "data.cob"},
		{name: "procedure_paragraphs_cob", fixture: "procedure_paragraphs.cob"},
		{name: "procedure_arithmetic_cob", fixture: "procedure_arithmetic.cob"},
		{name: "procedure_conditional_cob", fixture: "procedure_conditional.cob"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(filepath.Join("testdata", tc.fixture))
			require.NoError(t, err)

			first, err := Parse(bytes.NewReader(data))
			require.NoError(t, err)

			var buf bytes.Buffer
			require.NoError(t, Print(&buf, first))

			second, err := Parse(&buf)
			require.NoError(t, err)

			// The printer reformats canonically, so positions shift between the
			// fixture and the printed output; compare structure only (SPEC.md
			// "Reference format independence").
			require.Equal(t, withoutPos(first), withoutPos(second))
		})
	}
}

// withoutPos zeroes every Pos in f and returns it, so round-trip comparisons
// assert AST structure while ignoring the line/column positions the printer is
// free to choose (SPEC.md "Reference format independence").
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
			case *EnvironmentDivision:
				clearEnvironmentPos(d)
			case *DataDivision:
				clearDataDivisionPos(d)
			case *ProcedureDivision:
				clearProcedurePos(d)
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
	case *NumericLiteral:
		n.Pos = Pos{}
	case *Identifier:
		clearIdentifierPos(n)
	}
}

// clearWordPos zeroes the Pos of an optional *Word child (a no-op when nil).
func clearWordPos(w *Word) {
	if w != nil {
		w.Pos = Pos{}
	}
}

// clearNumericPos zeroes the Pos of an optional *NumericLiteral child (a no-op
// when nil).
func clearNumericPos(n *NumericLiteral) {
	if n != nil {
		n.Pos = Pos{}
	}
}

// clearProcedurePos zeroes every Pos beneath a PROCEDURE DIVISION so round-trip
// comparisons ignore the positions the printer is free to choose.
func clearProcedurePos(div *ProcedureDivision) {
	div.Pos = Pos{}
	for _, para := range div.Paragraphs {
		clearParagraphPos(para)
	}
	for _, sec := range div.Sections {
		sec.Pos = Pos{}
		clearWordPos(sec.Name)
		clearNumericPos(sec.Segment)
		for _, para := range sec.Paragraphs {
			clearParagraphPos(para)
		}
	}
}

// clearParagraphPos zeroes the Pos of a paragraph, its optional name, and every
// statement in its sentences.
func clearParagraphPos(para *Paragraph) {
	para.Pos = Pos{}
	clearWordPos(para.Name)
	for _, sent := range para.Sentences {
		sent.Pos = Pos{}
		for _, stmt := range sent.Statements {
			clearStatementPos(stmt)
		}
	}
}

// clearStatementPos zeroes the Pos of a statement and all of its nested operands,
// identifiers, and expressions.
func clearStatementPos(stmt Statement) {
	switch s := stmt.(type) {
	case *DisplayStatement:
		s.Pos = Pos{}
		for _, op := range s.Operands {
			clearTypePos(op)
		}
		clearWordPos(s.Upon)
	case *MoveStatement:
		s.Pos = Pos{}
		clearTypePos(s.Source)
		for _, t := range s.Targets {
			clearIdentifierPos(t)
		}
	case *AcceptStatement:
		s.Pos = Pos{}
		clearIdentifierPos(s.Target)
		clearWordPos(s.From)
	case *ArithmeticStatement:
		s.Pos = Pos{}
		for _, op := range s.Operands {
			clearTypePos(op)
		}
		for _, t := range s.Targets {
			clearIdentifierPos(t)
		}
		clearIdentifierPos(s.Giving)
	case *ComputeStatement:
		s.Pos = Pos{}
		for i := range s.Targets {
			s.Targets[i].Pos = Pos{}
			clearIdentifierPos(s.Targets[i].Name)
		}
		clearExprPos(s.Expr)
	case *IfStatement:
		s.Pos = Pos{}
		clearConditionPos(s.Cond)
		for _, st := range s.Then {
			clearStatementPos(st)
		}
		for _, st := range s.Else {
			clearStatementPos(st)
		}
	case *GoToStatement:
		s.Pos = Pos{}
		for _, t := range s.Targets {
			clearWordPos(t)
		}
		clearIdentifierPos(s.DependingOn)
	case *ContinueStatement:
		s.Pos = Pos{}
	case *StopStatement:
		s.Pos = Pos{}
		clearTypePos(s.Literal)
	}
}

// clearIdentifierPos zeroes the Pos of an identifier, its name, qualifiers,
// subscripts, and reference-modifier (a no-op when nil).
func clearIdentifierPos(id *Identifier) {
	if id == nil {
		return
	}
	id.Pos = Pos{}
	clearWordPos(id.Name)
	for _, q := range id.Qualifiers {
		clearWordPos(q)
	}
	for _, sub := range id.Subscripts {
		clearExprPos(sub)
	}
	if id.RefMod != nil {
		id.RefMod.Pos = Pos{}
		clearExprPos(id.RefMod.Start)
		clearExprPos(id.RefMod.Length)
	}
}

// clearConditionPos zeroes the Pos of a condition node and its nested operands and
// sub-conditions.
func clearConditionPos(c Condition) {
	switch n := c.(type) {
	case *RelationCondition:
		n.Pos = Pos{}
		clearExprPos(n.Left)
		clearExprPos(n.Right)
	case *ClassCondition:
		n.Pos = Pos{}
		clearExprPos(n.Operand)
	case *SignCondition:
		n.Pos = Pos{}
		clearExprPos(n.Operand)
	case *ConditionNameCondition:
		n.Pos = Pos{}
		clearIdentifierPos(n.Name)
	case *LogicalCondition:
		n.Pos = Pos{}
		clearConditionPos(n.Left)
		clearConditionPos(n.Right)
	case *NotCondition:
		n.Pos = Pos{}
		clearConditionPos(n.Cond)
	case *ParenCondition:
		n.Pos = Pos{}
		clearConditionPos(n.Cond)
	}
}

// clearExprPos zeroes the Pos of an arithmetic-expression node and its operands.
func clearExprPos(e Expr) {
	switch n := e.(type) {
	case *Identifier:
		clearIdentifierPos(n)
	case *NumericLiteral:
		if n != nil {
			n.Pos = Pos{}
		}
	case *StringLiteral:
		if n != nil {
			n.Pos = Pos{}
		}
	case *BinaryExpr:
		if n != nil {
			n.Pos = Pos{}
			clearExprPos(n.Left)
			clearExprPos(n.Right)
		}
	case *UnaryExpr:
		if n != nil {
			n.Pos = Pos{}
			clearExprPos(n.Operand)
		}
	case *ParenExpr:
		if n != nil {
			n.Pos = Pos{}
			clearExprPos(n.Expr)
		}
	}
}

// clearEnvironmentPos zeroes every Pos beneath an ENVIRONMENT DIVISION so
// round-trip comparisons ignore the positions the printer is free to choose.
func clearEnvironmentPos(div *EnvironmentDivision) {
	div.Pos = Pos{}
	if sec := div.Configuration; sec != nil {
		sec.Pos = Pos{}
		if p := sec.SourceComputer; p != nil {
			p.Pos = Pos{}
			clearWordPos(p.ComputerName)
		}
		if p := sec.ObjectComputer; p != nil {
			p.Pos = Pos{}
			clearWordPos(p.ComputerName)
		}
		if p := sec.SpecialNames; p != nil {
			p.Pos = Pos{}
			for _, clause := range p.Clauses {
				switch c := clause.(type) {
				case *DecimalPointClause:
					c.Pos = Pos{}
				case *CurrencySignClause:
					c.Pos = Pos{}
					clearTypePos(c.Sign)
				}
			}
		}
	}
	if sec := div.InputOutput; sec != nil {
		sec.Pos = Pos{}
		if para := sec.FileControl; para != nil {
			para.Pos = Pos{}
			for _, entry := range para.Entries {
				entry.Pos = Pos{}
				clearWordPos(entry.Name)
				clearTypePos(entry.Assign)
				for _, clause := range entry.Clauses {
					switch c := clause.(type) {
					case *OrganizationClause:
						c.Pos = Pos{}
					case *AccessClause:
						c.Pos = Pos{}
					case *RecordKeyClause:
						c.Pos = Pos{}
						clearWordPos(c.Name)
					case *FileStatusClause:
						c.Pos = Pos{}
						clearWordPos(c.Name)
					}
				}
			}
		}
	}
}

// clearDataDivisionPos zeroes every Pos beneath a DATA DIVISION so round-trip
// comparisons ignore the positions the printer is free to choose.
func clearDataDivisionPos(div *DataDivision) {
	div.Pos = Pos{}
	if sec := div.File; sec != nil {
		sec.Pos = Pos{}
		for _, entry := range sec.Entries {
			entry.Pos = Pos{}
			clearWordPos(entry.Name)
			for _, rec := range entry.Records {
				clearDataEntryPos(rec)
			}
		}
	}
	for _, sec := range []*DataSection{div.WorkingStorage, div.LocalStorage, div.Linkage} {
		if sec == nil {
			continue
		}
		sec.Pos = Pos{}
		for _, entry := range sec.Entries {
			clearDataEntryPos(entry)
		}
	}
}

// clearDataEntryPos zeroes the Pos of a data-description entry and every node
// beneath it (name and clauses).
func clearDataEntryPos(entry *DataDescriptionEntry) {
	entry.Pos = Pos{}
	clearWordPos(entry.Name)
	for _, clause := range entry.Clauses {
		switch c := clause.(type) {
		case *RedefinesClause:
			c.Pos = Pos{}
			clearWordPos(c.Name)
		case *PictureClause:
			c.Pos = Pos{}
		case *UsageClause:
			c.Pos = Pos{}
		case *ValueClause:
			c.Pos = Pos{}
			for _, spec := range c.Values {
				clearTypePos(spec.From)
				clearTypePos(spec.Through)
			}
		case *OccursClause:
			c.Pos = Pos{}
			clearNumericPos(c.Min)
			clearNumericPos(c.Max)
			clearWordPos(c.DependingOn)
			for i := range c.Keys {
				c.Keys[i].Pos = Pos{}
				clearWordPos(c.Keys[i].Name)
			}
			clearWordPos(c.IndexedBy)
		case *SignClause:
			c.Pos = Pos{}
		case *JustifiedClause:
			c.Pos = Pos{}
		case *SynchronizedClause:
			c.Pos = Pos{}
		case *BlankWhenZeroClause:
			c.Pos = Pos{}
		case *GlobalClause:
			c.Pos = Pos{}
		case *ExternalClause:
			c.Pos = Pos{}
		case *RenamesClause:
			c.Pos = Pos{}
			clearWordPos(c.From)
			clearWordPos(c.Through)
		}
	}
}

// fakeDivision, fakeStatement, and fakeValue satisfy the sealed AST interfaces
// with concrete types the printer does not know, so the error-path test can drive
// every "unsupported node" branch without waiting for real future node types.
type fakeDivision struct{}

func (fakeDivision) division() {}

type fakeStatement struct{}

func (fakeStatement) statement() {}

type fakeValue struct{}

func (fakeValue) cobol() {}

type fakeExpr struct{}

func (fakeExpr) expr() {}

type fakeCondition struct{}

func (fakeCondition) condition() {}

type fakeSpecialNamesClause struct{}

func (fakeSpecialNamesClause) specialNamesClause() {}

type fakeSelectClause struct{}

func (fakeSelectClause) selectClause() {}

type fakeDataClause struct{}

func (fakeDataClause) dataClause() {}

// TestPrinterErrors pins the typed error the printer reports for nil and
// unknown-type AST nodes, so the public Print API fails cleanly instead of
// panicking or emitting invalid COBOL.
func TestPrinterErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input *File
	}{
		{
			name:  "nil file",
			input: nil,
		},
		{
			name:  "nil program element",
			input: &File{Programs: []*Program{nil}},
		},
		{
			name:  "unknown division type",
			input: &File{Programs: []*Program{{Divisions: []Division{fakeDivision{}}}}},
		},
		{
			name:  "missing program-id paragraph",
			input: &File{Programs: []*Program{{Divisions: []Division{&IdentificationDivision{}}}}},
		},
		{
			name: "unsupported program name type",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&IdentificationDivision{ProgramID: &ProgramID{Name: fakeValue{}}},
			}}}},
		},
		{
			name: "typed-nil program name",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&IdentificationDivision{ProgramID: &ProgramID{Name: (*Word)(nil)}},
			}}}},
		},
		{
			name: "typed-nil file-control assignment target",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&EnvironmentDivision{InputOutput: &InputOutputSection{
					FileControl: &FileControlParagraph{Entries: []*FileControlEntry{
						{Name: &Word{Value: "F"}, Assign: (*StringLiteral)(nil)},
					}},
				}},
			}}}},
		},
		{
			name: "unknown statement type",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{fakeStatement{}}}}},
				}},
			}}}},
		},
		{
			name: "unsupported display operand type",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{
						&DisplayStatement{Operands: []Type{fakeValue{}}},
					}}}},
				}},
			}}}},
		},
		{
			name: "typed-nil identification division",
			input: &File{Programs: []*Program{{Divisions: []Division{
				(*IdentificationDivision)(nil),
			}}}},
		},
		{
			name: "typed-nil procedure division",
			input: &File{Programs: []*Program{{Divisions: []Division{
				(*ProcedureDivision)(nil),
			}}}},
		},
		{
			name: "typed-nil display statement",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{(*DisplayStatement)(nil)}}}},
				}},
			}}}},
		},
		{
			name: "typed-nil stop statement",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{(*StopStatement)(nil)}}}},
				}},
			}}}},
		},
		{
			name: "typed-nil move statement",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{(*MoveStatement)(nil)}}}},
				}},
			}}}},
		},
		{
			name: "unsupported move source type",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{
						&MoveStatement{Source: fakeValue{}, Targets: []*Identifier{{Name: &Word{Value: "X"}}}},
					}}}},
				}},
			}}}},
		},
		{
			name: "typed-nil accept statement",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{(*AcceptStatement)(nil)}}}},
				}},
			}}}},
		},
		{
			name: "go to with no targets",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{&GoToStatement{}}}}},
				}},
			}}}},
		},
		{
			name: "typed-nil continue statement",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{(*ContinueStatement)(nil)}}}},
				}},
			}}}},
		},
		{
			name: "empty sentence",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{}}},
				}},
			}}}},
		},
		{
			name: "both paragraphs and sections set",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{
					Paragraphs: []*Paragraph{{Sentences: []*Sentence{{Statements: []Statement{&StopStatement{Run: true}}}}}},
					Sections:   []*Section{{Name: &Word{Value: "S"}}},
				},
			}}}},
		},
		{
			name: "typed-nil if statement",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{(*IfStatement)(nil)}}}},
				}},
			}}}},
		},
		{
			name: "unknown condition type",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{
						&IfStatement{Cond: fakeCondition{}, Then: []Statement{&ContinueStatement{}}},
					}}}},
				}},
			}}}},
		},
		{
			name: "unknown expression type in compute",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&ProcedureDivision{Paragraphs: []*Paragraph{
					{Sentences: []*Sentence{{Statements: []Statement{
						&ComputeStatement{Targets: []ComputeTarget{{Name: &Identifier{Name: &Word{Value: "X"}}}}, Expr: fakeExpr{}},
					}}}},
				}},
			}}}},
		},
		{
			name: "typed-nil environment division",
			input: &File{Programs: []*Program{{Divisions: []Division{
				(*EnvironmentDivision)(nil),
			}}}},
		},
		{
			name: "nil file-control entry element",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&EnvironmentDivision{InputOutput: &InputOutputSection{
					FileControl: &FileControlParagraph{Entries: []*FileControlEntry{nil}},
				}},
			}}}},
		},
		{
			name: "unknown special-names clause type",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&EnvironmentDivision{Configuration: &ConfigurationSection{
					SpecialNames: &SpecialNamesParagraph{
						Clauses: []SpecialNamesClause{fakeSpecialNamesClause{}},
					},
				}},
			}}}},
		},
		{
			name: "typed-nil special-names clause",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&EnvironmentDivision{Configuration: &ConfigurationSection{
					SpecialNames: &SpecialNamesParagraph{
						Clauses: []SpecialNamesClause{(*CurrencySignClause)(nil)},
					},
				}},
			}}}},
		},
		{
			name: "typed-nil select-clause",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&EnvironmentDivision{InputOutput: &InputOutputSection{
					FileControl: &FileControlParagraph{Entries: []*FileControlEntry{
						{
							Name:    &Word{Value: "F"},
							Assign:  &StringLiteral{Value: `"f.dat"`},
							Clauses: []SelectClause{(*OrganizationClause)(nil)},
						},
					}},
				}},
			}}}},
		},
		{
			name: "debugging mode without computer name",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&EnvironmentDivision{Configuration: &ConfigurationSection{
					SourceComputer: &SourceComputerParagraph{DebuggingMode: true},
				}},
			}}}},
		},
		{
			name: "unsupported file-control assignment target",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&EnvironmentDivision{InputOutput: &InputOutputSection{
					FileControl: &FileControlParagraph{Entries: []*FileControlEntry{
						{Name: &Word{Value: "F"}, Assign: fakeValue{}},
					}},
				}},
			}}}},
		},
		{
			name: "unknown select-clause type",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&EnvironmentDivision{InputOutput: &InputOutputSection{
					FileControl: &FileControlParagraph{Entries: []*FileControlEntry{
						{
							Name:    &Word{Value: "F"},
							Assign:  &StringLiteral{Value: `"f.dat"`},
							Clauses: []SelectClause{fakeSelectClause{}},
						},
					}},
				}},
			}}}},
		},
		{
			name: "typed-nil data division",
			input: &File{Programs: []*Program{{Divisions: []Division{
				(*DataDivision)(nil),
			}}}},
		},
		{
			name: "nil data-description entry element",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&DataDivision{WorkingStorage: &DataSection{Entries: []*DataDescriptionEntry{nil}}},
			}}}},
		},
		{
			name: "nil file-description entry element",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&DataDivision{File: &FileSection{Entries: []*FileDescriptionEntry{nil}}},
			}}}},
		},
		{
			name: "file-description entry with unknown kind",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&DataDivision{File: &FileSection{Entries: []*FileDescriptionEntry{
					{Kind: "XD", Name: &Word{Value: "F"}},
				}}},
			}}}},
		},
		{
			name: "unknown data clause type",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&DataDivision{WorkingStorage: &DataSection{Entries: []*DataDescriptionEntry{
					{Level: 1, Name: &Word{Value: "X"}, Clauses: []DataClause{fakeDataClause{}}},
				}}},
			}}}},
		},
		{
			name: "typed-nil data clause",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&DataDivision{WorkingStorage: &DataSection{Entries: []*DataDescriptionEntry{
					{Level: 1, Name: &Word{Value: "X"}, Clauses: []DataClause{(*PictureClause)(nil)}},
				}}},
			}}}},
		},
		{
			name: "unsupported value-clause literal type",
			input: &File{Programs: []*Program{{Divisions: []Division{
				&DataDivision{WorkingStorage: &DataSection{Entries: []*DataDescriptionEntry{
					{Level: 1, Name: &Word{Value: "X"}, Clauses: []DataClause{
						&ValueClause{Values: []ValueSpec{{From: fakeValue{}}}},
					}},
				}}},
			}}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := Print(&buf, tc.input)

			var target UnsupportedNodeError
			require.ErrorAs(t, err, &target)
		})
	}
}
