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
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "DATADEMO"},
								},
							},
							&DataDivision{
								Pos: Pos{Line: 3, Column: 1},
								File: &FileSection{
									Pos: Pos{Line: 4, Column: 1},
									Entries: []*FileDescriptionEntry{
										{
											Pos:  Pos{Line: 5, Column: 1},
											Kind: "FD",
											Name: &Word{Pos: Pos{Line: 5, Column: 4}, Value: "CUST-FILE"},
											Records: []*DataDescriptionEntry{
												{
													Pos:   Pos{Line: 6, Column: 1},
													Level: 1,
													Name:  &Word{Pos: Pos{Line: 6, Column: 4}, Value: "CUST-RECORD"},
												},
												{
													Pos:   Pos{Line: 7, Column: 5},
													Level: 5,
													Name:  &Word{Pos: Pos{Line: 7, Column: 8}, Value: "CUST-ID"},
													Clauses: []DataClause{
														&PictureClause{Pos: Pos{Line: 7, Column: 16}, Picture: "9(5)"},
													},
												},
												{
													Pos:    Pos{Line: 8, Column: 5},
													Level:  5,
													Filler: true,
													Clauses: []DataClause{
														&PictureClause{Pos: Pos{Line: 8, Column: 15}, Picture: "X(20)"},
													},
												},
											},
										},
									},
								},
								WorkingStorage: &DataSection{
									Pos: Pos{Line: 9, Column: 1},
									Entries: []*DataDescriptionEntry{
										{
											Pos:   Pos{Line: 10, Column: 1},
											Level: 1,
											Name:  &Word{Pos: Pos{Line: 10, Column: 4}, Value: "COUNTER"},
											Clauses: []DataClause{
												&PictureClause{Pos: Pos{Line: 10, Column: 12}, Picture: "9(2)"},
												&UsageClause{Pos: Pos{Line: 10, Column: 21}, Usage: "COMP-3"},
												&ValueClause{Pos: Pos{Line: 10, Column: 34}, Values: []ValueSpec{{From: &Word{Pos: Pos{Line: 10, Column: 40}, Value: "ZERO"}}}},
											},
										},
										{
											Pos:   Pos{Line: 11, Column: 1},
											Level: 1,
											Name:  &Word{Pos: Pos{Line: 11, Column: 4}, Value: "TOTAL"},
											Clauses: []DataClause{
												&PictureClause{Pos: Pos{Line: 11, Column: 10}, Picture: "S9(5)V99"},
												&ValueClause{Pos: Pos{Line: 11, Column: 23}, Values: []ValueSpec{{From: &NumericLiteral{Pos: Pos{Line: 11, Column: 29}, Value: "0"}}}},
											},
										},
										{
											Pos:   Pos{Line: 12, Column: 1},
											Level: 1,
											Name:  &Word{Pos: Pos{Line: 12, Column: 4}, Value: "STATUS-FLAG"},
											Clauses: []DataClause{
												&PictureClause{Pos: Pos{Line: 12, Column: 16}, Picture: "X"},
												&ValueClause{Pos: Pos{Line: 12, Column: 22}, Values: []ValueSpec{{From: &StringLiteral{Pos: Pos{Line: 12, Column: 28}, Value: `"N"`}}}},
											},
										},
										{
											Pos:   Pos{Line: 13, Column: 5},
											Level: 88,
											Name:  &Word{Pos: Pos{Line: 13, Column: 8}, Value: "DONE"},
											Clauses: []DataClause{
												&ValueClause{Pos: Pos{Line: 13, Column: 13}, Values: []ValueSpec{{From: &StringLiteral{Pos: Pos{Line: 13, Column: 19}, Value: `"Y"`}}}},
											},
										},
										{
											Pos:   Pos{Line: 14, Column: 5},
											Level: 88,
											Name:  &Word{Pos: Pos{Line: 14, Column: 8}, Value: "PENDING"},
											Clauses: []DataClause{
												&ValueClause{Pos: Pos{Line: 14, Column: 16}, Values: []ValueSpec{{From: &StringLiteral{Pos: Pos{Line: 14, Column: 22}, Value: `"A"`}, Through: &StringLiteral{Pos: Pos{Line: 14, Column: 34}, Value: `"M"`}}}},
											},
										},
										{
											Pos:   Pos{Line: 15, Column: 1},
											Level: 1,
											Name:  &Word{Pos: Pos{Line: 15, Column: 4}, Value: "TABLE-DATA"},
										},
										{
											Pos:   Pos{Line: 16, Column: 5},
											Level: 5,
											Name:  &Word{Pos: Pos{Line: 16, Column: 8}, Value: "ITEM"},
											Clauses: []DataClause{
												&OccursClause{Pos: Pos{Line: 16, Column: 13}, Min: &NumericLiteral{Pos: Pos{Line: 16, Column: 20}, Value: "10"}},
												&PictureClause{Pos: Pos{Line: 16, Column: 29}, Picture: "9(4)"},
											},
										},
										{
											Pos:   Pos{Line: 17, Column: 1},
											Level: 1,
											Name:  &Word{Pos: Pos{Line: 17, Column: 4}, Value: "ALT"},
											Clauses: []DataClause{
												&RedefinesClause{Pos: Pos{Line: 17, Column: 8}, Name: &Word{Pos: Pos{Line: 17, Column: 18}, Value: "TABLE-DATA"}},
												&PictureClause{Pos: Pos{Line: 17, Column: 29}, Picture: "X(40)"},
											},
										},
										{
											Pos:   Pos{Line: 18, Column: 1},
											Level: 1,
											Name:  &Word{Pos: Pos{Line: 18, Column: 4}, Value: "FLAGS"},
										},
										{
											Pos:   Pos{Line: 19, Column: 5},
											Level: 5,
											Name:  &Word{Pos: Pos{Line: 19, Column: 8}, Value: "F1"},
											Clauses: []DataClause{
												&PictureClause{Pos: Pos{Line: 19, Column: 11}, Picture: "X"},
												&JustifiedClause{Pos: Pos{Line: 19, Column: 17}},
											},
										},
										{
											Pos:   Pos{Line: 20, Column: 5},
											Level: 5,
											Name:  &Word{Pos: Pos{Line: 20, Column: 8}, Value: "F2"},
											Clauses: []DataClause{
												&PictureClause{Pos: Pos{Line: 20, Column: 11}, Picture: "9"},
												&BlankWhenZeroClause{Pos: Pos{Line: 20, Column: 17}},
											},
										},
										{
											Pos:   Pos{Line: 21, Column: 5},
											Level: 5,
											Name:  &Word{Pos: Pos{Line: 21, Column: 8}, Value: "F3"},
											Clauses: []DataClause{
												&PictureClause{Pos: Pos{Line: 21, Column: 11}, Picture: "S9"},
												&SignClause{Pos: Pos{Line: 21, Column: 18}, Position: "LEADING", Separate: true},
											},
										},
										{
											Pos:   Pos{Line: 22, Column: 5},
											Level: 5,
											Name:  &Word{Pos: Pos{Line: 22, Column: 8}, Value: "F4"},
											Clauses: []DataClause{
												&PictureClause{Pos: Pos{Line: 22, Column: 11}, Picture: "9"},
												&SynchronizedClause{Pos: Pos{Line: 22, Column: 17}, Direction: "LEFT"},
											},
										},
										{
											Pos:   Pos{Line: 23, Column: 5},
											Level: 5,
											Name:  &Word{Pos: Pos{Line: 23, Column: 8}, Value: "F5"},
											Clauses: []DataClause{
												&PictureClause{Pos: Pos{Line: 23, Column: 11}, Picture: "X"},
												&GlobalClause{Pos: Pos{Line: 23, Column: 17}},
											},
										},
										{
											Pos:   Pos{Line: 24, Column: 5},
											Level: 5,
											Name:  &Word{Pos: Pos{Line: 24, Column: 8}, Value: "F6"},
											Clauses: []DataClause{
												&PictureClause{Pos: Pos{Line: 24, Column: 11}, Picture: "X"},
												&ExternalClause{Pos: Pos{Line: 24, Column: 17}},
											},
										},
										{
											Pos:   Pos{Line: 25, Column: 1},
											Level: 66,
											Name:  &Word{Pos: Pos{Line: 25, Column: 4}, Value: "RENAME-FIELD"},
											Clauses: []DataClause{
												&RenamesClause{Pos: Pos{Line: 25, Column: 17}, From: &Word{Pos: Pos{Line: 25, Column: 25}, Value: "F1"}, Through: &Word{Pos: Pos{Line: 25, Column: 36}, Value: "F2"}},
											},
										},
									},
								},
								Linkage: &DataSection{
									Pos: Pos{Line: 26, Column: 1},
									Entries: []*DataDescriptionEntry{
										{
											Pos:   Pos{Line: 27, Column: 1},
											Level: 1,
											Name:  &Word{Pos: Pos{Line: 27, Column: 4}, Value: "LK-PARM"},
											Clauses: []DataClause{
												&PictureClause{Pos: Pos{Line: 27, Column: 12}, Picture: "X(10)"},
											},
										},
									},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 28, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 29, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 29, Column: 5},
												Statements: []Statement{
													&StopStatement{Pos: Pos{Line: 29, Column: 5}, Run: true},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
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
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&DisplayStatement{
														Pos: Pos{Line: 4, Column: 5},
														Operands: []Type{
															&StringLiteral{Pos: Pos{Line: 4, Column: 13}, Value: `"Hello, world!"`},
														},
													},
												},
											},
											{
												Pos: Pos{Line: 5, Column: 5},
												Statements: []Statement{
													&StopStatement{Pos: Pos{Line: 5, Column: 5}, Run: true},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "procedure division simple statements",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    MOVE A TO B C.\n" +
				"    DISPLAY \"x\" A.\n" +
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
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&MoveStatement{
														Pos:    Pos{Line: 4, Column: 5},
														Source: &Identifier{Pos: Pos{Line: 4, Column: 10}, Name: &Word{Pos: Pos{Line: 4, Column: 10}, Value: "A"}},
														Targets: []*Identifier{
															{Pos: Pos{Line: 4, Column: 15}, Name: &Word{Pos: Pos{Line: 4, Column: 15}, Value: "B"}},
															{Pos: Pos{Line: 4, Column: 17}, Name: &Word{Pos: Pos{Line: 4, Column: 17}, Value: "C"}},
														},
													},
												},
											},
											{
												Pos: Pos{Line: 5, Column: 5},
												Statements: []Statement{
													&DisplayStatement{
														Pos: Pos{Line: 5, Column: 5},
														Operands: []Type{
															&StringLiteral{Pos: Pos{Line: 5, Column: 13}, Value: `"x"`},
															&Identifier{Pos: Pos{Line: 5, Column: 17}, Name: &Word{Pos: Pos{Line: 5, Column: 17}, Value: "A"}},
														},
													},
												},
											},
											{
												Pos: Pos{Line: 6, Column: 5},
												Statements: []Statement{
													&StopStatement{Pos: Pos{Line: 6, Column: 5}, Run: true},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "compute with operator precedence",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    COMPUTE X = A + B * C.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&ComputeStatement{
														Pos: Pos{Line: 4, Column: 5},
														Targets: []ComputeTarget{
															{
																Pos:  Pos{Line: 4, Column: 13},
																Name: &Identifier{Pos: Pos{Line: 4, Column: 13}, Name: &Word{Pos: Pos{Line: 4, Column: 13}, Value: "X"}},
															},
														},
														// A + (B * C): "*" binds tighter than "+".
														Expr: &BinaryExpr{
															Pos:  Pos{Line: 4, Column: 17},
															Op:   "+",
															Left: &Identifier{Pos: Pos{Line: 4, Column: 17}, Name: &Word{Pos: Pos{Line: 4, Column: 17}, Value: "A"}},
															Right: &BinaryExpr{
																Pos:   Pos{Line: 4, Column: 21},
																Op:    "*",
																Left:  &Identifier{Pos: Pos{Line: 4, Column: 21}, Name: &Word{Pos: Pos{Line: 4, Column: 21}, Value: "B"}},
																Right: &Identifier{Pos: Pos{Line: 4, Column: 25}, Name: &Word{Pos: Pos{Line: 4, Column: 25}, Value: "C"}},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "function reference as operand and arithmetic primary",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    MOVE FUNCTION CURRENT-DATE TO WS-NOW.\n" +
				"    COMPUTE WS-X = FUNCTION NUMVAL-C(WS-A).\n" +
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
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&MoveStatement{
														Pos: Pos{Line: 4, Column: 5},
														// FUNCTION reference in operand (MOVE source) position.
														Source: &FunctionReference{
															Pos:  Pos{Line: 4, Column: 10},
															Name: &Word{Pos: Pos{Line: 4, Column: 19}, Value: "CURRENT-DATE"},
														},
														Targets: []*Identifier{
															{Pos: Pos{Line: 4, Column: 35}, Name: &Word{Pos: Pos{Line: 4, Column: 35}, Value: "WS-NOW"}},
														},
													},
												},
											},
											{
												Pos: Pos{Line: 5, Column: 5},
												Statements: []Statement{
													&ComputeStatement{
														Pos: Pos{Line: 5, Column: 5},
														Targets: []ComputeTarget{
															{
																Pos:  Pos{Line: 5, Column: 13},
																Name: &Identifier{Pos: Pos{Line: 5, Column: 13}, Name: &Word{Pos: Pos{Line: 5, Column: 13}, Value: "WS-X"}},
															},
														},
														// FUNCTION reference in arithmetic-primary position.
														Expr: &FunctionReference{
															Pos:  Pos{Line: 5, Column: 20},
															Name: &Word{Pos: Pos{Line: 5, Column: 29}, Value: "NUMVAL-C"},
															Arguments: []Expr{
																&Identifier{Pos: Pos{Line: 5, Column: 38}, Name: &Word{Pos: Pos{Line: 5, Column: 38}, Value: "WS-A"}},
															},
														},
													},
												},
											},
											{
												Pos: Pos{Line: 6, Column: 5},
												Statements: []Statement{
													&StopStatement{Pos: Pos{Line: 6, Column: 5}, Run: true},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "function reference with reference modification",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    MOVE FUNCTION UPPER-CASE(WS-NAME)(1:1) TO WS-INITIAL.\n" +
				"    DISPLAY FUNCTION CURRENT-DATE(1:8).\n" +
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
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&MoveStatement{
														Pos: Pos{Line: 4, Column: 5},
														// Reference modification applied after the argument list.
														Source: &FunctionReference{
															Pos:  Pos{Line: 4, Column: 10},
															Name: &Word{Pos: Pos{Line: 4, Column: 19}, Value: "UPPER-CASE"},
															Arguments: []Expr{
																&Identifier{Pos: Pos{Line: 4, Column: 30}, Name: &Word{Pos: Pos{Line: 4, Column: 30}, Value: "WS-NAME"}},
															},
															RefMod: &ReferenceModifier{
																Pos:    Pos{Line: 4, Column: 38},
																Start:  &NumericLiteral{Pos: Pos{Line: 4, Column: 39}, Value: "1"},
																Length: &NumericLiteral{Pos: Pos{Line: 4, Column: 41}, Value: "1"},
															},
														},
														Targets: []*Identifier{
															{Pos: Pos{Line: 4, Column: 47}, Name: &Word{Pos: Pos{Line: 4, Column: 47}, Value: "WS-INITIAL"}},
														},
													},
												},
											},
											{
												Pos: Pos{Line: 5, Column: 5},
												Statements: []Statement{
													&DisplayStatement{
														Pos: Pos{Line: 5, Column: 5},
														// No-argument function with a reference modifier applied directly;
														// the colon distinguishes it from an argument list.
														Operands: []Type{
															&FunctionReference{
																Pos:  Pos{Line: 5, Column: 13},
																Name: &Word{Pos: Pos{Line: 5, Column: 22}, Value: "CURRENT-DATE"},
																RefMod: &ReferenceModifier{
																	Pos:    Pos{Line: 5, Column: 34},
																	Start:  &NumericLiteral{Pos: Pos{Line: 5, Column: 35}, Value: "1"},
																	Length: &NumericLiteral{Pos: Pos{Line: 5, Column: 37}, Value: "8"},
																},
															},
														},
													},
												},
											},
											{
												Pos: Pos{Line: 6, Column: 5},
												Statements: []Statement{
													&StopStatement{Pos: Pos{Line: 6, Column: 5}, Run: true},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "nested function reference with multiple arguments",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    COMPUTE WS-Z = FUNCTION MEAN(FUNCTION MAX(WS-A WS-B) WS-C).\n" +
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
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&ComputeStatement{
														Pos: Pos{Line: 4, Column: 5},
														Targets: []ComputeTarget{
															{
																Pos:  Pos{Line: 4, Column: 13},
																Name: &Identifier{Pos: Pos{Line: 4, Column: 13}, Name: &Word{Pos: Pos{Line: 4, Column: 13}, Value: "WS-Z"}},
															},
														},
														// FUNCTION MEAN(FUNCTION MAX(WS-A WS-B) WS-C): a nested
														// function reference as the first of two arguments.
														Expr: &FunctionReference{
															Pos:  Pos{Line: 4, Column: 20},
															Name: &Word{Pos: Pos{Line: 4, Column: 29}, Value: "MEAN"},
															Arguments: []Expr{
																&FunctionReference{
																	Pos:  Pos{Line: 4, Column: 34},
																	Name: &Word{Pos: Pos{Line: 4, Column: 43}, Value: "MAX"},
																	Arguments: []Expr{
																		&Identifier{Pos: Pos{Line: 4, Column: 47}, Name: &Word{Pos: Pos{Line: 4, Column: 47}, Value: "WS-A"}},
																		&Identifier{Pos: Pos{Line: 4, Column: 52}, Name: &Word{Pos: Pos{Line: 4, Column: 52}, Value: "WS-B"}},
																	},
																},
																&Identifier{Pos: Pos{Line: 4, Column: 58}, Name: &Word{Pos: Pos{Line: 4, Column: 58}, Value: "WS-C"}},
															},
														},
													},
												},
											},
											{
												Pos: Pos{Line: 5, Column: 5},
												Statements: []Statement{
													&StopStatement{Pos: Pos{Line: 5, Column: 5}, Run: true},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "if statement with end-if",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    IF A > B MOVE 1 TO C END-IF.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&IfStatement{
														Pos: Pos{Line: 4, Column: 5},
														Cond: &RelationCondition{
															Pos:   Pos{Line: 4, Column: 8},
															Left:  &Identifier{Pos: Pos{Line: 4, Column: 8}, Name: &Word{Pos: Pos{Line: 4, Column: 8}, Value: "A"}},
															Op:    ">",
															Right: &Identifier{Pos: Pos{Line: 4, Column: 12}, Name: &Word{Pos: Pos{Line: 4, Column: 12}, Value: "B"}},
														},
														Then: []Statement{
															&MoveStatement{
																Pos:     Pos{Line: 4, Column: 14},
																Source:  &NumericLiteral{Pos: Pos{Line: 4, Column: 19}, Value: "1"},
																Targets: []*Identifier{{Pos: Pos{Line: 4, Column: 24}, Name: &Word{Pos: Pos{Line: 4, Column: 24}, Value: "C"}}},
															},
														},
														EndIf: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "evaluate statement with operand subject and when other",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    EVALUATE WS-X WHEN 1 DISPLAY \"a\" WHEN OTHER DISPLAY \"b\" END-EVALUATE.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&EvaluateStatement{
														Pos: Pos{Line: 4, Column: 5},
														Subjects: []*EvaluateSubject{
															{Pos: Pos{Line: 4, Column: 14}, Operand: &Identifier{Pos: Pos{Line: 4, Column: 14}, Name: &Word{Pos: Pos{Line: 4, Column: 14}, Value: "WS-X"}}},
														},
														Whens: []*EvaluateWhen{
															{
																Pos: Pos{Line: 4, Column: 19},
																Objects: []*EvaluateObject{
																	{Pos: Pos{Line: 4, Column: 24}, Operand: &NumericLiteral{Pos: Pos{Line: 4, Column: 24}, Value: "1"}},
																},
																Body: []Statement{
																	&DisplayStatement{Pos: Pos{Line: 4, Column: 26}, Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 34}, Value: "\"a\""}}},
																},
															},
														},
														Other: []Statement{
															&DisplayStatement{Pos: Pos{Line: 4, Column: 49}, Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 57}, Value: "\"b\""}}},
														},
														HasOther: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "evaluate true subject with relation-condition object",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    EVALUATE TRUE WHEN A > B CONTINUE END-EVALUATE.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&EvaluateStatement{
														Pos: Pos{Line: 4, Column: 5},
														Subjects: []*EvaluateSubject{
															{Pos: Pos{Line: 4, Column: 14}, Bool: "TRUE"},
														},
														Whens: []*EvaluateWhen{
															{
																Pos: Pos{Line: 4, Column: 19},
																Objects: []*EvaluateObject{
																	{
																		Pos: Pos{Line: 4, Column: 24},
																		Cond: &RelationCondition{
																			Pos:   Pos{Line: 4, Column: 24},
																			Left:  &Identifier{Pos: Pos{Line: 4, Column: 24}, Name: &Word{Pos: Pos{Line: 4, Column: 24}, Value: "A"}},
																			Op:    ">",
																			Right: &Identifier{Pos: Pos{Line: 4, Column: 28}, Name: &Word{Pos: Pos{Line: 4, Column: 28}, Value: "B"}},
																		},
																	},
																},
																Body: []Statement{
																	&ContinueStatement{Pos: Pos{Line: 4, Column: 30}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "evaluate with also subjects range and any objects",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    EVALUATE X ALSO Y WHEN 1 THRU 9 ALSO ANY CONTINUE WHEN OTHER CONTINUE END-EVALUATE.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&EvaluateStatement{
														Pos: Pos{Line: 4, Column: 5},
														Subjects: []*EvaluateSubject{
															{Pos: Pos{Line: 4, Column: 14}, Operand: &Identifier{Pos: Pos{Line: 4, Column: 14}, Name: &Word{Pos: Pos{Line: 4, Column: 14}, Value: "X"}}},
															{Pos: Pos{Line: 4, Column: 21}, Operand: &Identifier{Pos: Pos{Line: 4, Column: 21}, Name: &Word{Pos: Pos{Line: 4, Column: 21}, Value: "Y"}}},
														},
														Whens: []*EvaluateWhen{
															{
																Pos: Pos{Line: 4, Column: 23},
																Objects: []*EvaluateObject{
																	{
																		Pos:     Pos{Line: 4, Column: 28},
																		Operand: &NumericLiteral{Pos: Pos{Line: 4, Column: 28}, Value: "1"},
																		Through: &NumericLiteral{Pos: Pos{Line: 4, Column: 35}, Value: "9"},
																	},
																	{Pos: Pos{Line: 4, Column: 42}, Any: true},
																},
																Body: []Statement{
																	&ContinueStatement{Pos: Pos{Line: 4, Column: 46}},
																},
															},
														},
														Other: []Statement{
															&ContinueStatement{Pos: Pos{Line: 4, Column: 66}},
														},
														HasOther: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "evaluate with negated condition object",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    EVALUATE TRUE WHEN NOT A = B CONTINUE END-EVALUATE.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&EvaluateStatement{
														Pos: Pos{Line: 4, Column: 5},
														Subjects: []*EvaluateSubject{
															{Pos: Pos{Line: 4, Column: 14}, Bool: "TRUE"},
														},
														Whens: []*EvaluateWhen{
															{
																Pos: Pos{Line: 4, Column: 19},
																Objects: []*EvaluateObject{
																	{
																		Pos: Pos{Line: 4, Column: 24},
																		Not: true,
																		Cond: &RelationCondition{
																			Pos:   Pos{Line: 4, Column: 28},
																			Left:  &Identifier{Pos: Pos{Line: 4, Column: 28}, Name: &Word{Pos: Pos{Line: 4, Column: 28}, Value: "A"}},
																			Op:    "=",
																			Right: &Identifier{Pos: Pos{Line: 4, Column: 32}, Name: &Word{Pos: Pos{Line: 4, Column: 32}, Value: "B"}},
																		},
																	},
																},
																Body: []Statement{
																	&ContinueStatement{Pos: Pos{Line: 4, Column: 34}},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "exponentiation is left-associative",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    COMPUTE X = A ** B ** C.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos:       Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{Pos: Pos{Line: 2, Column: 1}, Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"}},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&ComputeStatement{
														Pos:     Pos{Line: 4, Column: 5},
														Targets: []ComputeTarget{{Pos: Pos{Line: 4, Column: 13}, Name: &Identifier{Pos: Pos{Line: 4, Column: 13}, Name: &Word{Pos: Pos{Line: 4, Column: 13}, Value: "X"}}}},
														// (A ** B) ** C — left-associative.
														Expr: &BinaryExpr{
															Pos: Pos{Line: 4, Column: 17},
															Op:  "**",
															Left: &BinaryExpr{
																Pos:   Pos{Line: 4, Column: 17},
																Op:    "**",
																Left:  &Identifier{Pos: Pos{Line: 4, Column: 17}, Name: &Word{Pos: Pos{Line: 4, Column: 17}, Value: "A"}},
																Right: &Identifier{Pos: Pos{Line: 4, Column: 22}, Name: &Word{Pos: Pos{Line: 4, Column: 22}, Value: "B"}},
															},
															Right: &Identifier{Pos: Pos{Line: 4, Column: 27}, Name: &Word{Pos: Pos{Line: 4, Column: 27}, Value: "C"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "unary sign binds to the first primary before exponentiation",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    COMPUTE X = -A ** B.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos:       Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{Pos: Pos{Line: 2, Column: 1}, Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"}},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&ComputeStatement{
														Pos:     Pos{Line: 4, Column: 5},
														Targets: []ComputeTarget{{Pos: Pos{Line: 4, Column: 13}, Name: &Identifier{Pos: Pos{Line: 4, Column: 13}, Name: &Word{Pos: Pos{Line: 4, Column: 13}, Value: "X"}}}},
														// (-A) ** B — the sign binds to A, then exponentiation.
														Expr: &BinaryExpr{
															Pos: Pos{Line: 4, Column: 17},
															Op:  "**",
															Left: &UnaryExpr{
																Pos:     Pos{Line: 4, Column: 17},
																Op:      "-",
																Operand: &Identifier{Pos: Pos{Line: 4, Column: 18}, Name: &Word{Pos: Pos{Line: 4, Column: 18}, Value: "A"}},
															},
															Right: &Identifier{Pos: Pos{Line: 4, Column: 23}, Name: &Word{Pos: Pos{Line: 4, Column: 23}, Value: "B"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "add with rounded target and on size error",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    ADD A B TO C ROUNDED ON SIZE ERROR CONTINUE END-ADD.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos:       Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{Pos: Pos{Line: 2, Column: 1}, Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"}},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&ArithmeticStatement{
														Pos:  Pos{Line: 4, Column: 5},
														Verb: "ADD",
														Operands: []Type{
															&Identifier{Pos: Pos{Line: 4, Column: 9}, Name: &Word{Pos: Pos{Line: 4, Column: 9}, Value: "A"}},
															&Identifier{Pos: Pos{Line: 4, Column: 11}, Name: &Word{Pos: Pos{Line: 4, Column: 11}, Value: "B"}},
														},
														Connector: "TO",
														Targets: []*ArithmeticTarget{
															{Pos: Pos{Line: 4, Column: 16}, Name: &Identifier{Pos: Pos{Line: 4, Column: 16}, Name: &Word{Pos: Pos{Line: 4, Column: 16}, Value: "C"}}, Rounded: true},
														},
														SizeError: SizeErrorPhrases{
															HasOnSizeError: true,
															OnSizeError:    []Statement{&ContinueStatement{Pos: Pos{Line: 4, Column: 40}}},
														},
														EndScope: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "divide giving multiple rounded receivers and remainder",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DIVIDE A INTO B GIVING C ROUNDED D REMAINDER E.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos:       Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{Pos: Pos{Line: 2, Column: 1}, Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"}},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&ArithmeticStatement{
														Pos:       Pos{Line: 4, Column: 5},
														Verb:      "DIVIDE",
														Operands:  []Type{&Identifier{Pos: Pos{Line: 4, Column: 12}, Name: &Word{Pos: Pos{Line: 4, Column: 12}, Value: "A"}}},
														Connector: "INTO",
														Targets: []*ArithmeticTarget{
															{Pos: Pos{Line: 4, Column: 19}, Name: &Identifier{Pos: Pos{Line: 4, Column: 19}, Name: &Word{Pos: Pos{Line: 4, Column: 19}, Value: "B"}}},
														},
														Giving: []*ArithmeticTarget{
															{Pos: Pos{Line: 4, Column: 28}, Name: &Identifier{Pos: Pos{Line: 4, Column: 28}, Name: &Word{Pos: Pos{Line: 4, Column: 28}, Value: "C"}}, Rounded: true},
															{Pos: Pos{Line: 4, Column: 38}, Name: &Identifier{Pos: Pos{Line: 4, Column: 38}, Name: &Word{Pos: Pos{Line: 4, Column: 38}, Value: "D"}}},
														},
														Remainder: &Identifier{Pos: Pos{Line: 4, Column: 50}, Name: &Word{Pos: Pos{Line: 4, Column: 50}, Value: "E"}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "compute with on and not on size error",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    COMPUTE X = A + B ON SIZE ERROR CONTINUE NOT ON SIZE ERROR CONTINUE END-COMPUTE.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos:       Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{Pos: Pos{Line: 2, Column: 1}, Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"}},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&ComputeStatement{
														Pos:     Pos{Line: 4, Column: 5},
														Targets: []ComputeTarget{{Pos: Pos{Line: 4, Column: 13}, Name: &Identifier{Pos: Pos{Line: 4, Column: 13}, Name: &Word{Pos: Pos{Line: 4, Column: 13}, Value: "X"}}}},
														Expr: &BinaryExpr{
															Pos:   Pos{Line: 4, Column: 17},
															Op:    "+",
															Left:  &Identifier{Pos: Pos{Line: 4, Column: 17}, Name: &Word{Pos: Pos{Line: 4, Column: 17}, Value: "A"}},
															Right: &Identifier{Pos: Pos{Line: 4, Column: 21}, Name: &Word{Pos: Pos{Line: 4, Column: 21}, Value: "B"}},
														},
														SizeError: SizeErrorPhrases{
															HasOnSizeError:    true,
															OnSizeError:       []Statement{&ContinueStatement{Pos: Pos{Line: 4, Column: 37}}},
															HasNotOnSizeError: true,
															NotOnSizeError:    []Statement{&ContinueStatement{Pos: Pos{Line: 4, Column: 64}}},
														},
														EndScope: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "AND binds tighter than OR",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    IF A OR B AND C CONTINUE END-IF.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos:       Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{Pos: Pos{Line: 2, Column: 1}, Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"}},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&IfStatement{
														Pos: Pos{Line: 4, Column: 5},
														// A OR (B AND C) — AND binds tighter than OR.
														Cond: &LogicalCondition{
															Pos:  Pos{Line: 4, Column: 8},
															Op:   "OR",
															Left: &ConditionNameCondition{Pos: Pos{Line: 4, Column: 8}, Name: &Identifier{Pos: Pos{Line: 4, Column: 8}, Name: &Word{Pos: Pos{Line: 4, Column: 8}, Value: "A"}}},
															Right: &LogicalCondition{
																Pos:   Pos{Line: 4, Column: 13},
																Op:    "AND",
																Left:  &ConditionNameCondition{Pos: Pos{Line: 4, Column: 13}, Name: &Identifier{Pos: Pos{Line: 4, Column: 13}, Name: &Word{Pos: Pos{Line: 4, Column: 13}, Value: "B"}}},
																Right: &ConditionNameCondition{Pos: Pos{Line: 4, Column: 19}, Name: &Identifier{Pos: Pos{Line: 4, Column: 19}, Name: &Word{Pos: Pos{Line: 4, Column: 19}, Value: "C"}}},
															},
														},
														Then:  []Statement{&ContinueStatement{Pos: Pos{Line: 4, Column: 21}}},
														EndIf: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "negated relation records the NOT position",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    IF A NOT = B CONTINUE END-IF.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&IfStatement{
														Pos: Pos{Line: 4, Column: 5},
														// NotCondition.Pos is the NOT keyword (4,10), not the operand.
														Cond: &NotCondition{
															Pos: Pos{Line: 4, Column: 10},
															Cond: &RelationCondition{
																Pos:   Pos{Line: 4, Column: 8},
																Left:  &Identifier{Pos: Pos{Line: 4, Column: 8}, Name: &Word{Pos: Pos{Line: 4, Column: 8}, Value: "A"}},
																Op:    "=",
																Right: &Identifier{Pos: Pos{Line: 4, Column: 16}, Name: &Word{Pos: Pos{Line: 4, Column: 16}, Value: "B"}},
															},
														},
														Then:  []Statement{&ContinueStatement{Pos: Pos{Line: 4, Column: 18}}},
														EndIf: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "goback statement",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    GOBACK.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos:       Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{Pos: Pos{Line: 2, Column: 1}, Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"}},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos:        Pos{Line: 4, Column: 5},
												Statements: []Statement{&GobackStatement{Pos: Pos{Line: 4, Column: 5}}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "exit statement in every form",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    EXIT.\n" +
				"    EXIT PROGRAM.\n" +
				"    EXIT PARAGRAPH.\n" +
				"    EXIT SECTION.\n" +
				"    EXIT PERFORM.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos:       Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{Pos: Pos{Line: 2, Column: 1}, Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"}},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{Pos: Pos{Line: 4, Column: 5}, Statements: []Statement{&ExitStatement{Pos: Pos{Line: 4, Column: 5}}}},
											{Pos: Pos{Line: 5, Column: 5}, Statements: []Statement{&ExitStatement{Pos: Pos{Line: 5, Column: 5}, Option: "PROGRAM"}}},
											{Pos: Pos{Line: 6, Column: 5}, Statements: []Statement{&ExitStatement{Pos: Pos{Line: 6, Column: 5}, Option: "PARAGRAPH"}}},
											{Pos: Pos{Line: 7, Column: 5}, Statements: []Statement{&ExitStatement{Pos: Pos{Line: 7, Column: 5}, Option: "SECTION"}}},
											{Pos: Pos{Line: 8, Column: 5}, Statements: []Statement{&ExitStatement{Pos: Pos{Line: 8, Column: 5}, Option: "PERFORM"}}},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "perform varying with after phrases and with test composition",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    PERFORM VARYING I FROM 1 BY 1 UNTIL I > 10 AFTER J FROM 1 BY 1 UNTIL J > 5 AFTER K FROM 1 BY 1 UNTIL K > 3\n" +
				"        DISPLAY I\n" +
				"    END-PERFORM.\n" +
				"    PERFORM WITH TEST AFTER VARYING I FROM 1 BY 1 UNTIL I > 10 AFTER J FROM 1 BY 1 UNTIL J > 5\n" +
				"        DISPLAY I\n" +
				"    END-PERFORM.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&PerformStatement{
														Pos:    Pos{Line: 4, Column: 5},
														Inline: true,
														Varying: &PerformVarying{
															VaryingClause: VaryingClause{
																Pos:   Pos{Line: 4, Column: 13},
																Name:  &Identifier{Pos: Pos{Line: 4, Column: 21}, Name: &Word{Pos: Pos{Line: 4, Column: 21}, Value: "I"}},
																From:  &NumericLiteral{Pos: Pos{Line: 4, Column: 28}, Value: "1"},
																By:    &NumericLiteral{Pos: Pos{Line: 4, Column: 33}, Value: "1"},
																Until: &RelationCondition{Pos: Pos{Line: 4, Column: 41}, Left: &Identifier{Pos: Pos{Line: 4, Column: 41}, Name: &Word{Pos: Pos{Line: 4, Column: 41}, Value: "I"}}, Op: ">", Right: &NumericLiteral{Pos: Pos{Line: 4, Column: 45}, Value: "10"}},
															},
															After: []VaryingClause{{
																Pos:   Pos{Line: 4, Column: 48},
																Name:  &Identifier{Pos: Pos{Line: 4, Column: 54}, Name: &Word{Pos: Pos{Line: 4, Column: 54}, Value: "J"}},
																From:  &NumericLiteral{Pos: Pos{Line: 4, Column: 61}, Value: "1"},
																By:    &NumericLiteral{Pos: Pos{Line: 4, Column: 66}, Value: "1"},
																Until: &RelationCondition{Pos: Pos{Line: 4, Column: 74}, Left: &Identifier{Pos: Pos{Line: 4, Column: 74}, Name: &Word{Pos: Pos{Line: 4, Column: 74}, Value: "J"}}, Op: ">", Right: &NumericLiteral{Pos: Pos{Line: 4, Column: 78}, Value: "5"}},
															}, {
																Pos:   Pos{Line: 4, Column: 80},
																Name:  &Identifier{Pos: Pos{Line: 4, Column: 86}, Name: &Word{Pos: Pos{Line: 4, Column: 86}, Value: "K"}},
																From:  &NumericLiteral{Pos: Pos{Line: 4, Column: 93}, Value: "1"},
																By:    &NumericLiteral{Pos: Pos{Line: 4, Column: 98}, Value: "1"},
																Until: &RelationCondition{Pos: Pos{Line: 4, Column: 106}, Left: &Identifier{Pos: Pos{Line: 4, Column: 106}, Name: &Word{Pos: Pos{Line: 4, Column: 106}, Value: "K"}}, Op: ">", Right: &NumericLiteral{Pos: Pos{Line: 4, Column: 110}, Value: "3"}},
															}},
														},
														Body: []Statement{
															&DisplayStatement{
																Pos:      Pos{Line: 5, Column: 9},
																Operands: []Type{&Identifier{Pos: Pos{Line: 5, Column: 17}, Name: &Word{Pos: Pos{Line: 5, Column: 17}, Value: "I"}}},
															},
														},
														EndPerform: true,
													},
												},
											},
											{
												Pos: Pos{Line: 7, Column: 5},
												Statements: []Statement{
													&PerformStatement{
														Pos:       Pos{Line: 7, Column: 5},
														Inline:    true,
														TestAfter: true,
														Varying: &PerformVarying{
															VaryingClause: VaryingClause{
																Pos:   Pos{Line: 7, Column: 29},
																Name:  &Identifier{Pos: Pos{Line: 7, Column: 37}, Name: &Word{Pos: Pos{Line: 7, Column: 37}, Value: "I"}},
																From:  &NumericLiteral{Pos: Pos{Line: 7, Column: 44}, Value: "1"},
																By:    &NumericLiteral{Pos: Pos{Line: 7, Column: 49}, Value: "1"},
																Until: &RelationCondition{Pos: Pos{Line: 7, Column: 57}, Left: &Identifier{Pos: Pos{Line: 7, Column: 57}, Name: &Word{Pos: Pos{Line: 7, Column: 57}, Value: "I"}}, Op: ">", Right: &NumericLiteral{Pos: Pos{Line: 7, Column: 61}, Value: "10"}},
															},
															After: []VaryingClause{{
																Pos:   Pos{Line: 7, Column: 64},
																Name:  &Identifier{Pos: Pos{Line: 7, Column: 70}, Name: &Word{Pos: Pos{Line: 7, Column: 70}, Value: "J"}},
																From:  &NumericLiteral{Pos: Pos{Line: 7, Column: 77}, Value: "1"},
																By:    &NumericLiteral{Pos: Pos{Line: 7, Column: 82}, Value: "1"},
																Until: &RelationCondition{Pos: Pos{Line: 7, Column: 90}, Left: &Identifier{Pos: Pos{Line: 7, Column: 90}, Name: &Word{Pos: Pos{Line: 7, Column: 90}, Value: "J"}}, Op: ">", Right: &NumericLiteral{Pos: Pos{Line: 7, Column: 94}, Value: "5"}},
															}},
														},
														Body: []Statement{
															&DisplayStatement{
																Pos:      Pos{Line: 8, Column: 9},
																Operands: []Type{&Identifier{Pos: Pos{Line: 8, Column: 17}, Name: &Word{Pos: Pos{Line: 8, Column: 17}, Value: "I"}}},
															},
														},
														EndPerform: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "next sentence in both if branches",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    IF A NEXT SENTENCE ELSE NEXT SENTENCE END-IF.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos:       Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{Pos: Pos{Line: 2, Column: 1}, Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"}},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&IfStatement{
														Pos:     Pos{Line: 4, Column: 5},
														Cond:    &ConditionNameCondition{Pos: Pos{Line: 4, Column: 8}, Name: &Identifier{Pos: Pos{Line: 4, Column: 8}, Name: &Word{Pos: Pos{Line: 4, Column: 8}, Value: "A"}}},
														Then:    []Statement{&NextSentenceStatement{Pos: Pos{Line: 4, Column: 10}}},
														HasElse: true,
														Else:    []Statement{&NextSentenceStatement{Pos: Pos{Line: 4, Column: 29}}},
														EndIf:   true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "anonymous and named paragraphs",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DISPLAY \"a\".\n" +
				"MAIN.\n" +
				"    DISPLAY \"b\".\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&DisplayStatement{
														Pos:      Pos{Line: 4, Column: 5},
														Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 13}, Value: `"a"`}},
													},
												},
											},
										},
									},
									{
										Pos:  Pos{Line: 5, Column: 1},
										Name: &Word{Pos: Pos{Line: 5, Column: 1}, Value: "MAIN"},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 6, Column: 5},
												Statements: []Statement{
													&DisplayStatement{
														Pos:      Pos{Line: 6, Column: 5},
														Operands: []Type{&StringLiteral{Pos: Pos{Line: 6, Column: 13}, Value: `"b"`}},
													},
												},
											},
										},
									},
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
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 18, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 18, Column: 5},
												Statements: []Statement{
													&StopStatement{Pos: Pos{Line: 18, Column: 5}, Run: true},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "call statement with literal target",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    CALL \"PROG\".\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&CallStatement{
														Pos:    Pos{Line: 4, Column: 5},
														Target: &StringLiteral{Pos: Pos{Line: 4, Column: 10}, Value: "\"PROG\""},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "call statement with using returning and end-call",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    CALL WS-PROG USING WS-A WS-B RETURNING WS-RC END-CALL.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&CallStatement{
														Pos:    Pos{Line: 4, Column: 5},
														Target: &Identifier{Pos: Pos{Line: 4, Column: 10}, Name: &Word{Pos: Pos{Line: 4, Column: 10}, Value: "WS-PROG"}},
														Using: []*CallArgument{
															{Pos: Pos{Line: 4, Column: 24}, Operand: &Identifier{Pos: Pos{Line: 4, Column: 24}, Name: &Word{Pos: Pos{Line: 4, Column: 24}, Value: "WS-A"}}},
															{Pos: Pos{Line: 4, Column: 29}, Operand: &Identifier{Pos: Pos{Line: 4, Column: 29}, Name: &Word{Pos: Pos{Line: 4, Column: 29}, Value: "WS-B"}}},
														},
														Returning: &Identifier{Pos: Pos{Line: 4, Column: 44}, Name: &Word{Pos: Pos{Line: 4, Column: 44}, Value: "WS-RC"}},
														EndCall:   true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "call statement with by reference content and value modes",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    CALL \"P\" USING BY REFERENCE WS-A BY CONTENT WS-B BY VALUE WS-C.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&CallStatement{
														Pos:    Pos{Line: 4, Column: 5},
														Target: &StringLiteral{Pos: Pos{Line: 4, Column: 10}, Value: "\"P\""},
														Using: []*CallArgument{
															{Pos: Pos{Line: 4, Column: 20}, Mode: "REFERENCE", Operand: &Identifier{Pos: Pos{Line: 4, Column: 33}, Name: &Word{Pos: Pos{Line: 4, Column: 33}, Value: "WS-A"}}},
															{Pos: Pos{Line: 4, Column: 38}, Mode: "CONTENT", Operand: &Identifier{Pos: Pos{Line: 4, Column: 49}, Name: &Word{Pos: Pos{Line: 4, Column: 49}, Value: "WS-B"}}},
															{Pos: Pos{Line: 4, Column: 54}, Mode: "VALUE", Operand: &Identifier{Pos: Pos{Line: 4, Column: 63}, Name: &Word{Pos: Pos{Line: 4, Column: 63}, Value: "WS-C"}}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "open statement with modes and options",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    OPEN INPUT F-A REVERSED OUTPUT F-B I-O F-C EXTEND F-D WITH NO REWIND.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&OpenStatement{
														Pos: Pos{Line: 4, Column: 5},
														Groups: []*OpenGroup{
															{Pos: Pos{Line: 4, Column: 10}, Mode: "INPUT", Files: []*OpenFile{
																{Pos: Pos{Line: 4, Column: 16}, Name: &Word{Pos: Pos{Line: 4, Column: 16}, Value: "F-A"}, Option: "REVERSED"},
															}},
															{Pos: Pos{Line: 4, Column: 29}, Mode: "OUTPUT", Files: []*OpenFile{
																{Pos: Pos{Line: 4, Column: 36}, Name: &Word{Pos: Pos{Line: 4, Column: 36}, Value: "F-B"}},
															}},
															{Pos: Pos{Line: 4, Column: 40}, Mode: "I-O", Files: []*OpenFile{
																{Pos: Pos{Line: 4, Column: 44}, Name: &Word{Pos: Pos{Line: 4, Column: 44}, Value: "F-C"}},
															}},
															{Pos: Pos{Line: 4, Column: 48}, Mode: "EXTEND", Files: []*OpenFile{
																{Pos: Pos{Line: 4, Column: 55}, Name: &Word{Pos: Pos{Line: 4, Column: 55}, Value: "F-D"}, Option: "NO REWIND"},
															}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "close statement with options",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    CLOSE F-A WITH LOCK F-B F-C FOR REMOVAL.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&CloseStatement{
														Pos: Pos{Line: 4, Column: 5},
														Files: []*CloseFile{
															{Pos: Pos{Line: 4, Column: 11}, Name: &Word{Pos: Pos{Line: 4, Column: 11}, Value: "F-A"}, Option: "LOCK"},
															{Pos: Pos{Line: 4, Column: 25}, Name: &Word{Pos: Pos{Line: 4, Column: 25}, Value: "F-B"}},
															{Pos: Pos{Line: 4, Column: 29}, Name: &Word{Pos: Pos{Line: 4, Column: 29}, Value: "F-C"}, Option: "REMOVAL"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "read statement with at end and not at end",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    READ F-A NEXT RECORD INTO WS-X AT END DISPLAY \"e\" NOT AT END DISPLAY \"n\" END-READ.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&ReadStatement{
														Pos:       Pos{Line: 4, Column: 5},
														File:      &Word{Pos: Pos{Line: 4, Column: 10}, Value: "F-A"},
														Direction: "NEXT",
														Record:    true,
														Into:      &Identifier{Pos: Pos{Line: 4, Column: 31}, Name: &Word{Pos: Pos{Line: 4, Column: 31}, Value: "WS-X"}},
														Handler: ExceptionPhrases{
															Kind:  "AT END",
															HasOn: true,
															On: []Statement{
																&DisplayStatement{Pos: Pos{Line: 4, Column: 43}, Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 51}, Value: "\"e\""}}},
															},
															HasNotOn: true,
															NotOn: []Statement{
																&DisplayStatement{Pos: Pos{Line: 4, Column: 66}, Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 74}, Value: "\"n\""}}},
															},
														},
														EndRead: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "write statement with advancing and invalid key",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    WRITE REC FROM WS-X AFTER ADVANCING 2 LINES INVALID KEY DISPLAY \"d\" END-WRITE.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&WriteStatement{
														Pos:       Pos{Line: 4, Column: 5},
														Record:    &Word{Pos: Pos{Line: 4, Column: 11}, Value: "REC"},
														From:      &Identifier{Pos: Pos{Line: 4, Column: 20}, Name: &Word{Pos: Pos{Line: 4, Column: 20}, Value: "WS-X"}},
														Advancing: &AdvancingPhrase{Pos: Pos{Line: 4, Column: 25}, When: "AFTER", Amount: &NumericLiteral{Pos: Pos{Line: 4, Column: 41}, Value: "2"}},
														Handler: ExceptionPhrases{
															Kind:  "INVALID KEY",
															HasOn: true,
															On: []Statement{
																&DisplayStatement{Pos: Pos{Line: 4, Column: 61}, Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 69}, Value: "\"d\""}}},
															},
														},
														EndWrite: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "start statement with key relational operator",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    START F-A KEY >= CUST-ID INVALID KEY DISPLAY \"x\" END-START.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&StartStatement{
														Pos:  Pos{Line: 4, Column: 5},
														File: &Word{Pos: Pos{Line: 4, Column: 11}, Value: "F-A"},
														Key:  &StartKey{Pos: Pos{Line: 4, Column: 15}, Op: ">=", Name: &Identifier{Pos: Pos{Line: 4, Column: 22}, Name: &Word{Pos: Pos{Line: 4, Column: 22}, Value: "CUST-ID"}}},
														Handler: ExceptionPhrases{
															Kind:  "INVALID KEY",
															HasOn: true,
															On: []Statement{
																&DisplayStatement{Pos: Pos{Line: 4, Column: 42}, Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 50}, Value: "\"x\""}}},
															},
														},
														EndStart: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "sort statement with keys duplicates collating using giving",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SORT WORK-FILE ON ASCENDING KEY K1 K2 WITH DUPLICATES IN ORDER COLLATING SEQUENCE NS USING IN-FILE GIVING OUT-FILE.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&SortStatement{
														Pos:  Pos{Line: 4, Column: 5},
														File: &Word{Pos: Pos{Line: 4, Column: 10}, Value: "WORK-FILE"},
														Keys: []SortKey{
															{
																Pos:       Pos{Line: 4, Column: 20},
																Ascending: true,
																Names: []*Word{
																	{Pos: Pos{Line: 4, Column: 37}, Value: "K1"},
																	{Pos: Pos{Line: 4, Column: 40}, Value: "K2"},
																},
															},
														},
														Duplicates: true,
														InOrder:    true,
														Collating:  &Word{Pos: Pos{Line: 4, Column: 87}, Value: "NS"},
														Using:      []*Word{{Pos: Pos{Line: 4, Column: 96}, Value: "IN-FILE"}},
														Giving:     []*Word{{Pos: Pos{Line: 4, Column: 111}, Value: "OUT-FILE"}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "sort statement with input and output procedures",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SORT WORK-FILE DESCENDING KEY K1 INPUT PROCEDURE A THRU B OUTPUT PROCEDURE C.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&SortStatement{
														Pos:  Pos{Line: 4, Column: 5},
														File: &Word{Pos: Pos{Line: 4, Column: 10}, Value: "WORK-FILE"},
														Keys: []SortKey{
															{
																Pos:       Pos{Line: 4, Column: 20},
																Ascending: false,
																Names:     []*Word{{Pos: Pos{Line: 4, Column: 35}, Value: "K1"}},
															},
														},
														Input: &SortProcedure{
															Pos:     Pos{Line: 4, Column: 38},
															Target:  &Word{Pos: Pos{Line: 4, Column: 54}, Value: "A"},
															Through: &Word{Pos: Pos{Line: 4, Column: 61}, Value: "B"},
														},
														Output: &SortProcedure{
															Pos:    Pos{Line: 4, Column: 63},
															Target: &Word{Pos: Pos{Line: 4, Column: 80}, Value: "C"},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "merge statement with multi-file using and giving",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    MERGE M-F ON ASCENDING KEY MK USING M-A M-B GIVING OUT-FILE.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&MergeStatement{
														Pos:  Pos{Line: 4, Column: 5},
														File: &Word{Pos: Pos{Line: 4, Column: 11}, Value: "M-F"},
														Keys: []SortKey{
															{
																Pos:       Pos{Line: 4, Column: 15},
																Ascending: true,
																Names:     []*Word{{Pos: Pos{Line: 4, Column: 32}, Value: "MK"}},
															},
														},
														Using: []*Word{
															{Pos: Pos{Line: 4, Column: 41}, Value: "M-A"},
															{Pos: Pos{Line: 4, Column: 45}, Value: "M-B"},
														},
														Giving: []*Word{{Pos: Pos{Line: 4, Column: 56}, Value: "OUT-FILE"}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "release statement with from",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    RELEASE WORK-REC FROM WS-REC.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&ReleaseStatement{
														Pos:    Pos{Line: 4, Column: 5},
														Record: &Word{Pos: Pos{Line: 4, Column: 13}, Value: "WORK-REC"},
														From:   &Identifier{Pos: Pos{Line: 4, Column: 27}, Name: &Word{Pos: Pos{Line: 4, Column: 27}, Value: "WS-REC"}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "return statement with record into and at end handlers",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    RETURN W-F RECORD INTO WS-REC AT END DISPLAY \"e\" NOT AT END DISPLAY \"n\" END-RETURN.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&ReturnStatement{
														Pos:    Pos{Line: 4, Column: 5},
														File:   &Word{Pos: Pos{Line: 4, Column: 12}, Value: "W-F"},
														Record: true,
														Into:   &Identifier{Pos: Pos{Line: 4, Column: 28}, Name: &Word{Pos: Pos{Line: 4, Column: 28}, Value: "WS-REC"}},
														Handler: ExceptionPhrases{
															Kind:  "AT END",
															HasOn: true,
															On: []Statement{
																&DisplayStatement{Pos: Pos{Line: 4, Column: 42}, Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 50}, Value: "\"e\""}}},
															},
															HasNotOn: true,
															NotOn: []Statement{
																&DisplayStatement{Pos: Pos{Line: 4, Column: 65}, Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 73}, Value: "\"n\""}}},
															},
														},
														EndReturn: true,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "procedure division using and returning phrases",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. LINK.\n" +
				"PROCEDURE DIVISION USING BY REFERENCE WS-A BY VALUE WS-B RETURNING WS-RC.\n" +
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
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "LINK"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Using: []*Parameter{
									{Pos: Pos{Line: 3, Column: 26}, Mode: "REFERENCE", Name: &Word{Pos: Pos{Line: 3, Column: 39}, Value: "WS-A"}},
									{Pos: Pos{Line: 3, Column: 44}, Mode: "VALUE", Name: &Word{Pos: Pos{Line: 3, Column: 53}, Value: "WS-B"}},
								},
								Returning: &Word{Pos: Pos{Line: 3, Column: 68}, Value: "WS-RC"},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos:        Pos{Line: 4, Column: 5},
												Statements: []Statement{&StopStatement{Pos: Pos{Line: 4, Column: 5}, Run: true}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "declaratives with every use form and end program",
			src: "ID DIVISION.\n" +
				"PROGRAM-ID. D.\n" +
				"PROCEDURE DIVISION.\n" +
				"DECLARATIVES.\n" +
				"S1 SECTION.\n" +
				"    USE GLOBAL AFTER STANDARD ERROR PROCEDURE ON F1 F2.\n" +
				"P1.\n" +
				"    CONTINUE.\n" +
				"S2 SECTION.\n" +
				"    USE DEBUGGING ON P-A P-B.\n" +
				"S3 SECTION.\n" +
				"    USE GLOBAL BEFORE REPORTING RG.\n" +
				"END DECLARATIVES.\n" +
				"MAIN SECTION.\n" +
				"    STOP RUN.\n" +
				"END PROGRAM D.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "D"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Declaratives: []*DeclarativeSection{
									{
										Pos:  Pos{Line: 5, Column: 1},
										Name: &Word{Pos: Pos{Line: 5, Column: 1}, Value: "S1"},
										Use: &UseStatement{
											Pos: Pos{Line: 6, Column: 5},
											Spec: &ExceptionUse{
												Pos:    Pos{Line: 6, Column: 9},
												Global: true,
												Error:  true,
												Files: []*Word{
													{Pos: Pos{Line: 6, Column: 50}, Value: "F1"},
													{Pos: Pos{Line: 6, Column: 53}, Value: "F2"},
												},
											},
										},
										Paragraphs: []*Paragraph{
											{
												Pos:  Pos{Line: 7, Column: 1},
												Name: &Word{Pos: Pos{Line: 7, Column: 1}, Value: "P1"},
												Sentences: []*Sentence{
													{
														Pos:        Pos{Line: 8, Column: 5},
														Statements: []Statement{&ContinueStatement{Pos: Pos{Line: 8, Column: 5}}},
													},
												},
											},
										},
									},
									{
										Pos:  Pos{Line: 9, Column: 1},
										Name: &Word{Pos: Pos{Line: 9, Column: 1}, Value: "S2"},
										Use: &UseStatement{
											Pos: Pos{Line: 10, Column: 5},
											Spec: &DebuggingUse{
												Pos: Pos{Line: 10, Column: 9},
												Targets: []*Word{
													{Pos: Pos{Line: 10, Column: 22}, Value: "P-A"},
													{Pos: Pos{Line: 10, Column: 26}, Value: "P-B"},
												},
											},
										},
									},
									{
										Pos:  Pos{Line: 11, Column: 1},
										Name: &Word{Pos: Pos{Line: 11, Column: 1}, Value: "S3"},
										Use: &UseStatement{
											Pos: Pos{Line: 12, Column: 5},
											Spec: &ReportingUse{
												Pos:    Pos{Line: 12, Column: 9},
												Global: true,
												Report: &Word{Pos: Pos{Line: 12, Column: 33}, Value: "RG"},
											},
										},
									},
								},
								Sections: []*Section{
									{
										Pos:  Pos{Line: 14, Column: 1},
										Name: &Word{Pos: Pos{Line: 14, Column: 1}, Value: "MAIN"},
										Paragraphs: []*Paragraph{
											{
												Pos: Pos{Line: 15, Column: 5},
												Sentences: []*Sentence{
													{
														Pos:        Pos{Line: 15, Column: 5},
														Statements: []Statement{&StopStatement{Pos: Pos{Line: 15, Column: 5}, Run: true}},
													},
												},
											},
										},
									},
								},
							},
						},
						End: &EndProgram{Pos: Pos{Line: 16, Column: 1}, Name: &Word{Pos: Pos{Line: 16, Column: 13}, Value: "D"}},
					},
				},
			},
		},
		{
			name: "nested program",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. OUTER.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DISPLAY \"x\".\n" +
				"    IDENTIFICATION DIVISION.\n" +
				"    PROGRAM-ID. INNER.\n" +
				"    PROCEDURE DIVISION.\n" +
				"        STOP RUN.\n" +
				"    END PROGRAM INNER.\n" +
				"END PROGRAM OUTER.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "OUTER"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 5},
												Statements: []Statement{
													&DisplayStatement{
														Pos:      Pos{Line: 4, Column: 5},
														Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 13}, Value: "\"x\""}},
													},
												},
											},
										},
									},
								},
							},
						},
						Nested: []*Program{
							{
								Pos: Pos{Line: 5, Column: 5},
								Divisions: []Division{
									&IdentificationDivision{
										Pos: Pos{Line: 5, Column: 5},
										ProgramID: &ProgramID{
											Pos:  Pos{Line: 6, Column: 5},
											Name: &Word{Pos: Pos{Line: 6, Column: 17}, Value: "INNER"},
										},
									},
									&ProcedureDivision{
										Pos: Pos{Line: 7, Column: 5},
										Paragraphs: []*Paragraph{
											{
												Pos: Pos{Line: 8, Column: 9},
												Sentences: []*Sentence{
													{
														Pos:        Pos{Line: 8, Column: 9},
														Statements: []Statement{&StopStatement{Pos: Pos{Line: 8, Column: 9}, Run: true}},
													},
												},
											},
										},
									},
								},
								End: &EndProgram{Pos: Pos{Line: 9, Column: 5}, Name: &Word{Pos: Pos{Line: 9, Column: 17}, Value: "INNER"}},
							},
						},
						End: &EndProgram{Pos: Pos{Line: 10, Column: 1}, Name: &Word{Pos: Pos{Line: 10, Column: 13}, Value: "OUTER"}},
					},
				},
			},
		},
		{
			name: "concatenated sibling programs delimited by end program",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. A.\n" +
				"PROCEDURE DIVISION.\n" +
				"    STOP RUN.\n" +
				"END PROGRAM A.\n" +
				"IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. B.\n" +
				"PROCEDURE DIVISION.\n" +
				"    STOP RUN.\n" +
				"END PROGRAM B.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 1},
									Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "A"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 5},
										Sentences: []*Sentence{
											{
												Pos:        Pos{Line: 4, Column: 5},
												Statements: []Statement{&StopStatement{Pos: Pos{Line: 4, Column: 5}, Run: true}},
											},
										},
									},
								},
							},
						},
						End: &EndProgram{Pos: Pos{Line: 5, Column: 1}, Name: &Word{Pos: Pos{Line: 5, Column: 13}, Value: "A"}},
					},
					{
						Pos: Pos{Line: 6, Column: 1},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 6, Column: 1},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 7, Column: 1},
									Name: &Word{Pos: Pos{Line: 7, Column: 13}, Value: "B"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 8, Column: 1},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 9, Column: 5},
										Sentences: []*Sentence{
											{
												Pos:        Pos{Line: 9, Column: 5},
												Statements: []Statement{&StopStatement{Pos: Pos{Line: 9, Column: 5}, Run: true}},
											},
										},
									},
								},
							},
						},
						End: &EndProgram{Pos: Pos{Line: 10, Column: 1}, Name: &Word{Pos: Pos{Line: 10, Column: 13}, Value: "B"}},
					},
				},
			},
		},
		{
			name: "initialize statement",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    INITIALIZE A B.\n",
			expected: dataManipFile([]*Sentence{
				{
					Pos: Pos{Line: 4, Column: 5},
					Statements: []Statement{
						&InitializeStatement{
							Pos: Pos{Line: 4, Column: 5},
							Targets: []*Identifier{
								{Pos: Pos{Line: 4, Column: 16}, Name: &Word{Pos: Pos{Line: 4, Column: 16}, Value: "A"}},
								{Pos: Pos{Line: 4, Column: 18}, Name: &Word{Pos: Pos{Line: 4, Column: 18}, Value: "B"}},
							},
						},
					},
				},
			}),
		},
		{
			name: "set statement forms",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SET I TO 1.\n" +
				"    SET I UP BY 2.\n" +
				"    SET D TO TRUE.\n",
			expected: dataManipFile([]*Sentence{
				{
					Pos: Pos{Line: 4, Column: 5},
					Statements: []Statement{
						&SetStatement{
							Pos:     Pos{Line: 4, Column: 5},
							Targets: []*Identifier{{Pos: Pos{Line: 4, Column: 9}, Name: &Word{Pos: Pos{Line: 4, Column: 9}, Value: "I"}}},
							Mode:    "TO",
							Value:   &NumericLiteral{Pos: Pos{Line: 4, Column: 14}, Value: "1"},
						},
					},
				},
				{
					Pos: Pos{Line: 5, Column: 5},
					Statements: []Statement{
						&SetStatement{
							Pos:     Pos{Line: 5, Column: 5},
							Targets: []*Identifier{{Pos: Pos{Line: 5, Column: 9}, Name: &Word{Pos: Pos{Line: 5, Column: 9}, Value: "I"}}},
							Mode:    "UP BY",
							Value:   &NumericLiteral{Pos: Pos{Line: 5, Column: 17}, Value: "2"},
						},
					},
				},
				{
					Pos: Pos{Line: 6, Column: 5},
					Statements: []Statement{
						&SetStatement{
							Pos:     Pos{Line: 6, Column: 5},
							Targets: []*Identifier{{Pos: Pos{Line: 6, Column: 9}, Name: &Word{Pos: Pos{Line: 6, Column: 9}, Value: "D"}}},
							Mode:    "TO",
							Value:   &Word{Pos: Pos{Line: 6, Column: 14}, Value: "TRUE"},
						},
					},
				},
			}),
		},
		{
			name: "string statement",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    STRING WS-A WS-B DELIMITED BY SIZE INTO WS-R.\n",
			expected: dataManipFile([]*Sentence{
				{
					Pos: Pos{Line: 4, Column: 5},
					Statements: []Statement{
						&StringStatement{
							Pos: Pos{Line: 4, Column: 5},
							Sources: []*StringSource{
								{
									Pos: Pos{Line: 4, Column: 12},
									Operands: []Type{
										&Identifier{Pos: Pos{Line: 4, Column: 12}, Name: &Word{Pos: Pos{Line: 4, Column: 12}, Value: "WS-A"}},
										&Identifier{Pos: Pos{Line: 4, Column: 17}, Name: &Word{Pos: Pos{Line: 4, Column: 17}, Value: "WS-B"}},
									},
									Delimiter: &Word{Pos: Pos{Line: 4, Column: 35}, Value: "SIZE"},
								},
							},
							Into: &Identifier{Pos: Pos{Line: 4, Column: 45}, Name: &Word{Pos: Pos{Line: 4, Column: 45}, Value: "WS-R"}},
						},
					},
				},
			}),
		},
		{
			name: "unstring statement",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    UNSTRING WS-S DELIMITED BY \",\" INTO WS-A WS-B.\n",
			expected: dataManipFile([]*Sentence{
				{
					Pos: Pos{Line: 4, Column: 5},
					Statements: []Statement{
						&UnstringStatement{
							Pos:    Pos{Line: 4, Column: 5},
							Source: &Identifier{Pos: Pos{Line: 4, Column: 14}, Name: &Word{Pos: Pos{Line: 4, Column: 14}, Value: "WS-S"}},
							Delimiters: []*UnstringDelimiter{
								{Pos: Pos{Line: 4, Column: 32}, Value: &StringLiteral{Pos: Pos{Line: 4, Column: 32}, Value: "\",\""}},
							},
							Into: []*UnstringTarget{
								{Pos: Pos{Line: 4, Column: 41}, Into: &Identifier{Pos: Pos{Line: 4, Column: 41}, Name: &Word{Pos: Pos{Line: 4, Column: 41}, Value: "WS-A"}}},
								{Pos: Pos{Line: 4, Column: 46}, Into: &Identifier{Pos: Pos{Line: 4, Column: 46}, Name: &Word{Pos: Pos{Line: 4, Column: 46}, Value: "WS-B"}}},
							},
						},
					},
				},
			}),
		},
		{
			name: "inspect tallying and replacing statements",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    INSPECT WS-T TALLYING WS-C FOR ALL \"A\".\n" +
				"    INSPECT WS-T REPLACING CHARACTERS BY \" \".\n",
			expected: dataManipFile([]*Sentence{
				{
					Pos: Pos{Line: 4, Column: 5},
					Statements: []Statement{
						&InspectStatement{
							Pos:    Pos{Line: 4, Column: 5},
							Target: &Identifier{Pos: Pos{Line: 4, Column: 13}, Name: &Word{Pos: Pos{Line: 4, Column: 13}, Value: "WS-T"}},
							Tallying: []*InspectTally{
								{
									Pos:   Pos{Line: 4, Column: 27},
									Count: &Identifier{Pos: Pos{Line: 4, Column: 27}, Name: &Word{Pos: Pos{Line: 4, Column: 27}, Value: "WS-C"}},
									Specs: []*InspectMatch{
										{Pos: Pos{Line: 4, Column: 36}, Kind: "ALL", Item: &StringLiteral{Pos: Pos{Line: 4, Column: 40}, Value: "\"A\""}},
									},
								},
							},
						},
					},
				},
				{
					Pos: Pos{Line: 5, Column: 5},
					Statements: []Statement{
						&InspectStatement{
							Pos:    Pos{Line: 5, Column: 5},
							Target: &Identifier{Pos: Pos{Line: 5, Column: 13}, Name: &Word{Pos: Pos{Line: 5, Column: 13}, Value: "WS-T"}},
							Replacing: []*InspectReplace{
								{Pos: Pos{Line: 5, Column: 28}, Kind: "CHARACTERS", By: &StringLiteral{Pos: Pos{Line: 5, Column: 42}, Value: "\" \""}},
							},
						},
					},
				},
			}),
		},
		{
			name: "search statement",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SEARCH WS-T WHEN WS-X = 1 DISPLAY \"f\" END-SEARCH.\n",
			expected: dataManipFile([]*Sentence{
				{
					Pos: Pos{Line: 4, Column: 5},
					Statements: []Statement{
						&SearchStatement{
							Pos:    Pos{Line: 4, Column: 5},
							Target: &Identifier{Pos: Pos{Line: 4, Column: 12}, Name: &Word{Pos: Pos{Line: 4, Column: 12}, Value: "WS-T"}},
							Whens: []*SearchWhen{
								{
									Pos: Pos{Line: 4, Column: 17},
									Cond: &RelationCondition{
										Pos:   Pos{Line: 4, Column: 22},
										Left:  &Identifier{Pos: Pos{Line: 4, Column: 22}, Name: &Word{Pos: Pos{Line: 4, Column: 22}, Value: "WS-X"}},
										Op:    "=",
										Right: &NumericLiteral{Pos: Pos{Line: 4, Column: 29}, Value: "1"},
									},
									Body: []Statement{
										&DisplayStatement{
											Pos:      Pos{Line: 4, Column: 31},
											Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 39}, Value: "\"f\""}},
										},
									},
								},
							},
							EndSearch: true,
						},
					},
				},
			}),
		},
		{
			name: "inspect converting statement",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    INSPECT WS-T CONVERTING \"ab\" TO \"AB\" AFTER INITIAL \" \".\n",
			expected: dataManipFile([]*Sentence{
				{
					Pos: Pos{Line: 4, Column: 5},
					Statements: []Statement{
						&InspectStatement{
							Pos:    Pos{Line: 4, Column: 5},
							Target: &Identifier{Pos: Pos{Line: 4, Column: 13}, Name: &Word{Pos: Pos{Line: 4, Column: 13}, Value: "WS-T"}},
							Converting: &InspectConvert{
								Pos:  Pos{Line: 4, Column: 18},
								From: &StringLiteral{Pos: Pos{Line: 4, Column: 29}, Value: "\"ab\""},
								To:   &StringLiteral{Pos: Pos{Line: 4, Column: 37}, Value: "\"AB\""},
								Region: &InspectRegion{
									Pos:     Pos{Line: 4, Column: 42},
									Kind:    "AFTER",
									Initial: true,
									Operand: &StringLiteral{Pos: Pos{Line: 4, Column: 56}, Value: "\" \""},
								},
							},
						},
					},
				},
			}),
		},
		{
			name: "set pointer and switch forms",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SET P TO ADDRESS OF R.\n" +
				"    SET ADDRESS OF R TO P.\n" +
				"    SET S TO ON.\n" +
				"    SET S TO OFF.\n",
			expected: dataManipFile([]*Sentence{
				{
					Pos: Pos{Line: 4, Column: 5},
					Statements: []Statement{
						&SetStatement{
							Pos:     Pos{Line: 4, Column: 5},
							Targets: []*Identifier{{Pos: Pos{Line: 4, Column: 9}, Name: &Word{Pos: Pos{Line: 4, Column: 9}, Value: "P"}}},
							Mode:    "TO",
							Value: &AddressOf{
								Pos: Pos{Line: 4, Column: 14},
								Of:  &Identifier{Pos: Pos{Line: 4, Column: 25}, Name: &Word{Pos: Pos{Line: 4, Column: 25}, Value: "R"}},
							},
						},
					},
				},
				{
					Pos: Pos{Line: 5, Column: 5},
					Statements: []Statement{
						&SetStatement{
							Pos:          Pos{Line: 5, Column: 5},
							Targets:      []*Identifier{{Pos: Pos{Line: 5, Column: 20}, Name: &Word{Pos: Pos{Line: 5, Column: 20}, Value: "R"}}},
							TargetIsAddr: true,
							Mode:         "TO",
							Value:        &Identifier{Pos: Pos{Line: 5, Column: 25}, Name: &Word{Pos: Pos{Line: 5, Column: 25}, Value: "P"}},
						},
					},
				},
				{
					Pos: Pos{Line: 6, Column: 5},
					Statements: []Statement{
						&SetStatement{
							Pos:     Pos{Line: 6, Column: 5},
							Targets: []*Identifier{{Pos: Pos{Line: 6, Column: 9}, Name: &Word{Pos: Pos{Line: 6, Column: 9}, Value: "S"}}},
							Mode:    "TO",
							Value:   &Word{Pos: Pos{Line: 6, Column: 14}, Value: "ON"},
						},
					},
				},
				{
					Pos: Pos{Line: 7, Column: 5},
					Statements: []Statement{
						&SetStatement{
							Pos:     Pos{Line: 7, Column: 5},
							Targets: []*Identifier{{Pos: Pos{Line: 7, Column: 9}, Name: &Word{Pos: Pos{Line: 7, Column: 9}, Value: "S"}}},
							Mode:    "TO",
							Value:   &Word{Pos: Pos{Line: 7, Column: 14}, Value: "OFF"},
						},
					},
				},
			}),
		},
		{
			name: "initialize replacing filler value and default",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    INITIALIZE A REPLACING NUMERIC BY 0.\n" +
				"    INITIALIZE B WITH FILLER ALL TO VALUE.\n" +
				"    INITIALIZE C REPLACING ALPHANUMERIC DATA BY SPACE DEFAULT.\n",
			expected: dataManipFile([]*Sentence{
				{
					Pos: Pos{Line: 4, Column: 5},
					Statements: []Statement{
						&InitializeStatement{
							Pos:     Pos{Line: 4, Column: 5},
							Targets: []*Identifier{{Pos: Pos{Line: 4, Column: 16}, Name: &Word{Pos: Pos{Line: 4, Column: 16}, Value: "A"}}},
							Replacing: []*InitializeReplace{
								{
									Pos:      Pos{Line: 4, Column: 28},
									Category: &Word{Pos: Pos{Line: 4, Column: 28}, Value: "NUMERIC"},
									By:       &NumericLiteral{Pos: Pos{Line: 4, Column: 39}, Value: "0"},
								},
							},
						},
					},
				},
				{
					Pos: Pos{Line: 5, Column: 5},
					Statements: []Statement{
						&InitializeStatement{
							Pos:     Pos{Line: 5, Column: 5},
							Targets: []*Identifier{{Pos: Pos{Line: 5, Column: 16}, Name: &Word{Pos: Pos{Line: 5, Column: 16}, Value: "B"}}},
							Filler:  true,
							ToValue: []*Word{{Pos: Pos{Line: 5, Column: 30}, Value: "ALL"}},
						},
					},
				},
				{
					Pos: Pos{Line: 6, Column: 5},
					Statements: []Statement{
						&InitializeStatement{
							Pos:     Pos{Line: 6, Column: 5},
							Targets: []*Identifier{{Pos: Pos{Line: 6, Column: 16}, Name: &Word{Pos: Pos{Line: 6, Column: 16}, Value: "C"}}},
							Replacing: []*InitializeReplace{
								{
									Pos:      Pos{Line: 6, Column: 28},
									Category: &Word{Pos: Pos{Line: 6, Column: 28}, Value: "ALPHANUMERIC"},
									Data:     true,
									By:       &Identifier{Pos: Pos{Line: 6, Column: 49}, Name: &Word{Pos: Pos{Line: 6, Column: 49}, Value: "SPACE"}},
								},
							},
							Default: true,
						},
					},
				},
			}),
		},
		{
			name: "search all with conjunction",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SEARCH ALL T WHEN K = 1 AND V DISPLAY \"f\" END-SEARCH.\n",
			expected: dataManipFile([]*Sentence{
				{
					Pos: Pos{Line: 4, Column: 5},
					Statements: []Statement{
						&SearchStatement{
							Pos:    Pos{Line: 4, Column: 5},
							All:    true,
							Target: &Identifier{Pos: Pos{Line: 4, Column: 16}, Name: &Word{Pos: Pos{Line: 4, Column: 16}, Value: "T"}},
							Whens: []*SearchWhen{
								{
									Pos: Pos{Line: 4, Column: 18},
									Cond: &LogicalCondition{
										Pos: Pos{Line: 4, Column: 23},
										Op:  "AND",
										Left: &RelationCondition{
											Pos:   Pos{Line: 4, Column: 23},
											Left:  &Identifier{Pos: Pos{Line: 4, Column: 23}, Name: &Word{Pos: Pos{Line: 4, Column: 23}, Value: "K"}},
											Op:    "=",
											Right: &NumericLiteral{Pos: Pos{Line: 4, Column: 27}, Value: "1"},
										},
										Right: &ConditionNameCondition{Pos: Pos{Line: 4, Column: 33}, Name: &Identifier{Pos: Pos{Line: 4, Column: 33}, Name: &Word{Pos: Pos{Line: 4, Column: 33}, Value: "V"}}},
									},
									Body: []Statement{
										&DisplayStatement{
											Pos:      Pos{Line: 4, Column: 35},
											Operands: []Type{&StringLiteral{Pos: Pos{Line: 4, Column: 43}, Value: "\"f\""}},
										},
									},
								},
							},
							EndSearch: true,
						},
					},
				},
			}),
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

// TestParserFixedFormat drives Parse with WithSourceFormat(FixedFormat) to prove
// the source-format option is wired through to the tokenizer. The expected AST
// mirrors the free-format minimal program but with fixed-format column positions
// (copied from the "minimal program with Area A and Area B" tokenizer test).
func TestParserFixedFormat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      string
		expected *File
	}{
		{
			// Sequence numbers fill columns 1–6 (ignored only in fixed format),
			// headers/names begin in Area A (column 8), statements in Area B
			// (column 12 onward).
			name: "minimal program with sequence area, Area A and Area B",
			src: "000100 IDENTIFICATION DIVISION.\n" +
				"000200 PROGRAM-ID. HELLO.\n" +
				"000300 PROCEDURE DIVISION.\n" +
				"000400     DISPLAY \"Hello, world!\".\n" +
				"000500     STOP RUN.\n",
			expected: &File{
				Programs: []*Program{
					{
						Pos: Pos{Line: 1, Column: 8},
						Divisions: []Division{
							&IdentificationDivision{
								Pos: Pos{Line: 1, Column: 8},
								ProgramID: &ProgramID{
									Pos:  Pos{Line: 2, Column: 8},
									Name: &Word{Pos: Pos{Line: 2, Column: 20}, Value: "HELLO"},
								},
							},
							&ProcedureDivision{
								Pos: Pos{Line: 3, Column: 8},
								Paragraphs: []*Paragraph{
									{
										Pos: Pos{Line: 4, Column: 12},
										Sentences: []*Sentence{
											{
												Pos: Pos{Line: 4, Column: 12},
												Statements: []Statement{
													&DisplayStatement{
														Pos: Pos{Line: 4, Column: 12},
														Operands: []Type{
															&StringLiteral{Pos: Pos{Line: 4, Column: 20}, Value: `"Hello, world!"`},
														},
													},
												},
											},
											{
												Pos: Pos{Line: 5, Column: 12},
												Statements: []Statement{
													&StopStatement{Pos: Pos{Line: 5, Column: 12}, Run: true},
												},
											},
										},
									},
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

			f, err := Parse(strings.NewReader(tc.src), WithSourceFormat(FixedFormat))

			require.NoError(t, err)
			require.Equal(t, tc.expected, f)

			// The same source must NOT parse as free format: the columns 1–6
			// sequence numbers are only ignored in fixed format, so a free-format
			// parse sees them as stray tokens and errors. This proves the option
			// is actually wired through to the tokenizer rather than a no-op.
			_, freeErr := Parse(strings.NewReader(tc.src))
			require.Error(t, freeErr)
		})
	}
}

// TestParserComments pins how comments are attached to the AST: a comment is the
// leading comment of the structural node it precedes. The fixed-format column-7
// "*" lines here attach to the program (the banner), the PROCEDURE DIVISION, and
// the first sentence respectively, and their Text is normalized (the "*"
// introducer and one following space removed) so it matches a free-format "*>"
// re-emission on round-trip.
func TestParserComments(t *testing.T) {
	t.Parallel()

	src := "000100* program banner\n" +
		"000200 IDENTIFICATION DIVISION.\n" +
		"000300 PROGRAM-ID. C.\n" +
		"000400* before procedure\n" +
		"000500 PROCEDURE DIVISION.\n" +
		"000600 P.\n" +
		"000700* before sentence\n" +
		"000800     DISPLAY \"x\".\n" +
		"000900     STOP RUN.\n"

	expected := &File{
		Programs: []*Program{
			{
				Pos:      Pos{Line: 2, Column: 8},
				Comments: []*Comment{{Pos: Pos{Line: 1, Column: 7}, Text: "program banner"}},
				Divisions: []Division{
					&IdentificationDivision{
						Pos: Pos{Line: 2, Column: 8},
						ProgramID: &ProgramID{
							Pos:  Pos{Line: 3, Column: 8},
							Name: &Word{Pos: Pos{Line: 3, Column: 20}, Value: "C"},
						},
					},
					&ProcedureDivision{
						Pos:      Pos{Line: 5, Column: 8},
						Comments: []*Comment{{Pos: Pos{Line: 4, Column: 7}, Text: "before procedure"}},
						Paragraphs: []*Paragraph{
							{
								Pos:  Pos{Line: 6, Column: 8},
								Name: &Word{Pos: Pos{Line: 6, Column: 8}, Value: "P"},
								Sentences: []*Sentence{
									{
										Pos:      Pos{Line: 8, Column: 12},
										Comments: []*Comment{{Pos: Pos{Line: 7, Column: 7}, Text: "before sentence"}},
										Statements: []Statement{
											&DisplayStatement{
												Pos: Pos{Line: 8, Column: 12},
												Operands: []Type{
													&StringLiteral{Pos: Pos{Line: 8, Column: 20}, Value: `"x"`},
												},
											},
										},
									},
									{
										Pos: Pos{Line: 9, Column: 12},
										Statements: []Statement{
											&StopStatement{Pos: Pos{Line: 9, Column: 12}, Run: true},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	f, err := Parse(strings.NewReader(src), WithSourceFormat(FixedFormat))
	require.NoError(t, err)
	require.Equal(t, expected, f)
}

// TestParserDefaultSourceFormat pins the documented default: parsing with no
// options is free format, and WithSourceFormat(FreeFormat) is equivalent.
func TestParserDefaultSourceFormat(t *testing.T) {
	t.Parallel()

	src := "IDENTIFICATION DIVISION.\n" +
		"PROGRAM-ID. HELLO.\n" +
		"PROCEDURE DIVISION.\n" +
		"    DISPLAY \"Hello, world!\".\n" +
		"    STOP RUN.\n"

	defaultAST, err := Parse(strings.NewReader(src))
	require.NoError(t, err)

	explicitAST, err := Parse(strings.NewReader(src), WithSourceFormat(FreeFormat))
	require.NoError(t, err)

	require.Equal(t, defaultAST, explicitAST)
}

// TestParserFragment pins WithFragment: a source consisting solely of data
// description entries — a standalone copybook — parses without an IDENTIFICATION
// DIVISION, and its entries land on File.Fragment with File.Programs empty. The
// level rules are the ordinary ones (01/77 at the top, 02–49 subordinate, 88
// condition-names, 66 RENAMES), because the fragment reuses the very same entry
// loop the DATA DIVISION sections drive.
func TestParserFragment(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      string
		expected *File
	}{
		{
			// The issue's motivating example: the copybook that Parse used to
			// reject outright with "unexpected token Number(01)".
			name: "record with subordinate elementary item",
			src: "01 CUSTOMER-RECORD.\n" +
				"   05 CUST-ID PIC 9(6) COMP-3.\n",
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
								&UsageClause{Pos: Pos{Line: 2, Column: 24}, Usage: "COMP-3"},
							},
						},
					},
				},
			},
		},
		{
			// A level-77 independent item at the top level, with the level-88
			// condition-names it qualifies subordinate to it.
			name: "level 77 independent item with condition names",
			src: "77 CUST-STATUS PIC X.\n" +
				"   88 CUST-ACTIVE VALUE \"A\".\n" +
				"   88 CUST-CLOSED VALUE \"C\" THROUGH \"D\".\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 77,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUST-STATUS"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 1, Column: 16}, Picture: "X"},
							},
						},
						{
							Pos:   Pos{Line: 2, Column: 4},
							Level: 88,
							Name:  &Word{Pos: Pos{Line: 2, Column: 7}, Value: "CUST-ACTIVE"},
							Clauses: []DataClause{
								&ValueClause{
									Pos:    Pos{Line: 2, Column: 19},
									Values: []ValueSpec{{From: &StringLiteral{Pos: Pos{Line: 2, Column: 25}, Value: `"A"`}}},
								},
							},
						},
						{
							Pos:   Pos{Line: 3, Column: 4},
							Level: 88,
							Name:  &Word{Pos: Pos{Line: 3, Column: 7}, Value: "CUST-CLOSED"},
							Clauses: []DataClause{
								&ValueClause{
									Pos: Pos{Line: 3, Column: 19},
									Values: []ValueSpec{{
										From:    &StringLiteral{Pos: Pos{Line: 3, Column: 25}, Value: `"C"`},
										Through: &StringLiteral{Pos: Pos{Line: 3, Column: 37}, Value: `"D"`},
									}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "level 66 renames",
			src: "01 CUST-RECORD.\n" +
				"   05 F1 PIC X.\n" +
				"   05 F2 PIC X.\n" +
				"66 CUST-KEY RENAMES F1 THROUGH F2.\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 1,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUST-RECORD"},
						},
						{
							Pos:   Pos{Line: 2, Column: 4},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 2, Column: 7}, Value: "F1"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 2, Column: 10}, Picture: "X"},
							},
						},
						{
							Pos:   Pos{Line: 3, Column: 4},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 3, Column: 7}, Value: "F2"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 3, Column: 10}, Picture: "X"},
							},
						},
						{
							Pos:   Pos{Line: 4, Column: 1},
							Level: 66,
							Name:  &Word{Pos: Pos{Line: 4, Column: 4}, Value: "CUST-KEY"},
							Clauses: []DataClause{
								&RenamesClause{
									Pos:     Pos{Line: 4, Column: 13},
									From:    &Word{Pos: Pos{Line: 4, Column: 21}, Value: "F1"},
									Through: &Word{Pos: Pos{Line: 4, Column: 32}, Value: "F2"},
								},
							},
						},
					},
				},
			},
		},
		{
			// A comment leads the entry it precedes, exactly as in a program; the
			// one past the last entry has no entry to lead and becomes the
			// fragment's Trailing comments, so a copybook ending in a banner still
			// round-trips.
			name: "leading and trailing comments",
			src: "*> customer copybook\n" +
				"01 CUSTOMER-RECORD.\n" +
				"*> the identifier\n" +
				"   05 CUST-ID PIC 9(6).\n" +
				"*> end of copybook\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:      Pos{Line: 2, Column: 1},
							Comments: []*Comment{{Pos: Pos{Line: 1, Column: 1}, Text: "customer copybook"}},
							Level:    1,
							Name:     &Word{Pos: Pos{Line: 2, Column: 4}, Value: "CUSTOMER-RECORD"},
						},
						{
							Pos:      Pos{Line: 4, Column: 4},
							Comments: []*Comment{{Pos: Pos{Line: 3, Column: 1}, Text: "the identifier"}},
							Level:    5,
							Name:     &Word{Pos: Pos{Line: 4, Column: 7}, Value: "CUST-ID"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 4, Column: 15}, Picture: "9(6)"},
							},
						},
					},
					Trailing: []*Comment{{Pos: Pos{Line: 5, Column: 1}, Text: "end of copybook"}},
				},
			},
		},
		{
			// The hierarchy the level numbers imply is recorded, not enforced —
			// exactly as in a DATA DIVISION section. A copybook is routinely a
			// record's middle, COPYed in under a group the file itself never
			// names, so a fragment opening below level 01 parses rather than
			// being rejected on a hierarchy the source cannot show.
			name: "entries below the record level",
			src: "05 CUST-NAME.\n" +
				"   10 CUST-FIRST PIC X(20).\n",
			expected: &File{
				Fragment: &Fragment{
					Entries: []*DataDescriptionEntry{
						{
							Pos:   Pos{Line: 1, Column: 1},
							Level: 5,
							Name:  &Word{Pos: Pos{Line: 1, Column: 4}, Value: "CUST-NAME"},
						},
						{
							Pos:   Pos{Line: 2, Column: 4},
							Level: 10,
							Name:  &Word{Pos: Pos{Line: 2, Column: 7}, Value: "CUST-FIRST"},
							Clauses: []DataClause{
								&PictureClause{Pos: Pos{Line: 2, Column: 18}, Picture: "X(20)"},
							},
						},
					},
				},
			},
		},
		{
			// An empty copybook is a fragment with no entries — not a nil Fragment,
			// so a consumer can still tell it apart from a whole source file.
			name:     "empty source",
			src:      "",
			expected: &File{Fragment: &Fragment{}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, err := Parse(strings.NewReader(tc.src), WithFragment())

			require.NoError(t, err)
			require.Equal(t, tc.expected, f)
			require.Empty(t, f.Programs)
		})
	}
}

// TestParserFragmentFixedFormat pins fragments in fixed format: copybooks are
// commonly held in column-oriented libraries, so WithFragment has to compose with
// WithSourceFormat(FixedFormat) — the sequence area ignored, a column-7 "*"
// comment attached to the entry it precedes.
func TestParserFragmentFixedFormat(t *testing.T) {
	t.Parallel()

	src := "000100* customer copybook\n" +
		"000200 01  CUSTOMER-RECORD.\n" +
		"000300     05  CUST-ID     PIC 9(6) COMP-3.\n" +
		"000400     05  CUST-NAME   PIC X(20).\n"

	expected := &File{
		Fragment: &Fragment{
			Entries: []*DataDescriptionEntry{
				{
					Pos:      Pos{Line: 2, Column: 8},
					Comments: []*Comment{{Pos: Pos{Line: 1, Column: 7}, Text: "customer copybook"}},
					Level:    1,
					Name:     &Word{Pos: Pos{Line: 2, Column: 12}, Value: "CUSTOMER-RECORD"},
				},
				{
					Pos:   Pos{Line: 3, Column: 12},
					Level: 5,
					Name:  &Word{Pos: Pos{Line: 3, Column: 16}, Value: "CUST-ID"},
					Clauses: []DataClause{
						&PictureClause{Pos: Pos{Line: 3, Column: 28}, Picture: "9(6)"},
						&UsageClause{Pos: Pos{Line: 3, Column: 37}, Usage: "COMP-3"},
					},
				},
				{
					Pos:   Pos{Line: 4, Column: 12},
					Level: 5,
					Name:  &Word{Pos: Pos{Line: 4, Column: 16}, Value: "CUST-NAME"},
					Clauses: []DataClause{
						&PictureClause{Pos: Pos{Line: 4, Column: 28}, Picture: "X(20)"},
					},
				},
			},
		},
	}

	f, err := Parse(strings.NewReader(src), WithFragment(), WithSourceFormat(FixedFormat))
	require.NoError(t, err)
	require.Equal(t, expected, f)

	// The same source must NOT parse as a free-format fragment: the columns 1–6
	// sequence numbers are only ignored in fixed format, so the free-format parse
	// sees them as stray tokens. This proves the two options compose rather than
	// one overriding the other.
	_, freeErr := Parse(strings.NewReader(src), WithFragment())
	require.Error(t, freeErr)
}

// TestParserUsageComp6 pins COMP-6, the GnuCOBOL/Micro Focus packed-decimal
// usage-type with no sign nibble. It is grammar only: the spelling is admitted
// wherever any other usage-type is — bare and after USAGE [IS], in free format
// and in fixed — and lands on UsageClause.Usage canonicalized to upper case.
// Nothing here maps it to a width; that is a consumer's question.
func TestParserUsageComp6(t *testing.T) {
	t.Parallel()

	// Every case is the same one-entry copybook, so the expected AST differs
	// only in where the clauses sit.
	entry := func(entryPos, namePos, picPos, usagePos Pos) *File {
		return &File{
			Fragment: &Fragment{
				Entries: []*DataDescriptionEntry{
					{
						Pos:   entryPos,
						Level: 77,
						Name:  &Word{Pos: namePos, Value: "ODD"},
						Clauses: []DataClause{
							&PictureClause{Pos: picPos, Picture: "9(4)"},
							&UsageClause{Pos: usagePos, Usage: "COMP-6"},
						},
					},
				},
			},
		}
	}

	testCases := []struct {
		name     string
		src      string
		format   SourceFormat
		expected *File
	}{
		{
			// The issue's motivating spelling: the bare usage-type, no USAGE
			// keyword, which is how copybooks in the wild write it.
			name:     "bare usage type in free format",
			src:      "77 ODD PIC 9(4) COMP-6.\n",
			format:   FreeFormat,
			expected: entry(Pos{Line: 1, Column: 1}, Pos{Line: 1, Column: 4}, Pos{Line: 1, Column: 8}, Pos{Line: 1, Column: 17}),
		},
		{
			// The explicit form. Pos is the USAGE keyword, matching every other
			// usage-type: the clause starts where the clause starts.
			name:     "explicit USAGE IS in free format",
			src:      "77 ODD PIC 9(4) USAGE IS COMP-6.\n",
			format:   FreeFormat,
			expected: entry(Pos{Line: 1, Column: 1}, Pos{Line: 1, Column: 4}, Pos{Line: 1, Column: 8}, Pos{Line: 1, Column: 17}),
		},
		{
			// COBOL reserved words are case-insensitive and Usage is canonical
			// upper case, so a lower-case copybook yields the same AST.
			name:     "lower case spelling is canonicalized",
			src:      "77 ODD PIC 9(4) usage is comp-6.\n",
			format:   FreeFormat,
			expected: entry(Pos{Line: 1, Column: 1}, Pos{Line: 1, Column: 4}, Pos{Line: 1, Column: 8}, Pos{Line: 1, Column: 17}),
		},
		{
			// Copybooks are commonly held in column-oriented libraries, so the
			// fixed reference format has to admit the spelling too.
			name:     "bare usage type in fixed format",
			src:      "000100 77  ODD  PIC 9(4) COMP-6.\n",
			format:   FixedFormat,
			expected: entry(Pos{Line: 1, Column: 8}, Pos{Line: 1, Column: 12}, Pos{Line: 1, Column: 17}, Pos{Line: 1, Column: 26}),
		},
		{
			name:     "explicit USAGE IS in fixed format",
			src:      "000100 77  ODD  PIC 9(4) USAGE IS COMP-6.\n",
			format:   FixedFormat,
			expected: entry(Pos{Line: 1, Column: 8}, Pos{Line: 1, Column: 12}, Pos{Line: 1, Column: 17}, Pos{Line: 1, Column: 26}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f, err := Parse(strings.NewReader(tc.src), WithFragment(), WithSourceFormat(tc.format))

			require.NoError(t, err)
			require.Equal(t, tc.expected, f)
		})
	}
}

// TestParserFragmentErrors pins the two ends of the mode: a copybook is rejected
// without WithFragment (the behaviour the issue reported), and a whole source file
// is rejected with it, rather than the parser silently dropping everything past the
// entries it understood.
func TestParserFragmentErrors(t *testing.T) {
	t.Parallel()

	const copybook = "01 CUSTOMER-RECORD.\n" +
		"   05 CUST-ID PIC 9(6) COMP-3.\n"

	t.Run("copybook without WithFragment", func(t *testing.T) {
		t.Parallel()

		_, err := Parse(strings.NewReader(copybook))

		var unexpected UnexpectedTokenError
		require.ErrorAs(t, err, &unexpected)
		require.Equal(t, Pos{Line: 1, Column: 1}, unexpected.Actual.Pos)
	})

	t.Run("division header with WithFragment", func(t *testing.T) {
		t.Parallel()

		src := "01 CUSTOMER-RECORD.\n" +
			"IDENTIFICATION DIVISION.\n"

		_, err := Parse(strings.NewReader(src), WithFragment())

		// The fragment was already complete, so nothing would have been accepted
		// in the token's place: the expectation reported is end of input, not
		// another level-number.
		var trailing TrailingTokenError
		require.ErrorAs(t, err, &trailing)
		require.Equal(t, Pos{Line: 2, Column: 1}, trailing.Actual.Pos)
		require.Contains(t, trailing.Error(), "expected end of input")
	})

	t.Run("invalid level number", func(t *testing.T) {
		t.Parallel()

		_, err := Parse(strings.NewReader("50 CUST-ID PIC X.\n"), WithFragment())

		var invalid InvalidLevelNumberError
		require.ErrorAs(t, err, &invalid)
		require.Equal(t, "50", invalid.Value)
	})

	// A genuinely unknown usage-type is still rejected, and the alternatives it
	// lists are the ones the grammar admits — so a copybook author who mistyped
	// COMP-6 is shown that it exists rather than being told it does not.
	t.Run("unknown usage type lists the alternatives", func(t *testing.T) {
		t.Parallel()

		_, err := Parse(strings.NewReader("77 ODD PIC 9(4) USAGE IS COMP-7.\n"), WithFragment())

		var target UnexpectedKeywordError
		require.ErrorAs(t, err, &target)
		require.Equal(t, Pos{Line: 1, Column: 26}, target.Actual.Pos)
		require.Contains(t, target.Expected, "COMP-6")
		require.Contains(t, err.Error(), `"COMP-7"`)
	})
}

// TestSourceFormatString pins the String() rendering of each SourceFormat.
func TestSourceFormatString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		format   SourceFormat
		expected string
	}{
		{name: "free", format: FreeFormat, expected: "Free"},
		{name: "fixed", format: FixedFormat, expected: "Fixed"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.expected, tc.format.String())
		})
	}
}

// dataManipFile wraps procedure-division sentences in the standard single-paragraph
// program scaffold (program-name P) shared by the data-manipulation parser cases.
func dataManipFile(sentences []*Sentence) *File {
	return &File{
		Programs: []*Program{
			{
				Pos: Pos{Line: 1, Column: 1},
				Divisions: []Division{
					&IdentificationDivision{
						Pos: Pos{Line: 1, Column: 1},
						ProgramID: &ProgramID{
							Pos:  Pos{Line: 2, Column: 1},
							Name: &Word{Pos: Pos{Line: 2, Column: 13}, Value: "P"},
						},
					},
					&ProcedureDivision{
						Pos: Pos{Line: 3, Column: 1},
						Paragraphs: []*Paragraph{
							{
								Pos:       Pos{Line: 4, Column: 5},
								Sentences: sentences,
							},
						},
					},
				},
			},
		},
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
			name: "function reference missing function-name",
			// After the FUNCTION keyword a function-name (identifier) is required; the
			// separator period is not one.
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DISPLAY FUNCTION.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 21}, target.Actual.Pos)
			},
		},
		{
			name: "misspelled verb in statement position",
			// A bare "FOO." is a (valid, empty) paragraph named FOO; an unknown verb
			// only errors where a statement is required, e.g. inside an IF branch.
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    IF X = 0 FOO.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 14}, target.Actual.Pos)
			},
		},
		{
			name: "NEXT without SENTENCE in if branch",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    IF A NEXT FOO END-IF.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 15}, target.Actual.Pos)
			},
		},
		{
			name: "NOT not followed by SIZE ERROR after on size error",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    ADD A TO B ON SIZE ERROR CONTINUE NOT CONTINUE.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 43}, target.Actual.Pos)
			},
		},
		{
			name: "remainder rejected on non-divide verb",
			// REMAINDER is DIVIDE-only; on ADD the keyword is left for the sentence,
			// which has no statement verb to dispatch and so rejects it.
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    ADD A TO B GIVING C REMAINDER D.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 25}, target.Actual.Pos)
			},
		},
		{
			name: "reserved word rejected where a receiver is required",
			// "ON" cannot stand in for the receiver after the TO connector.
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    ADD A TO ON SIZE ERROR CONTINUE END-ADD.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 14}, target.Actual.Pos)
			},
		},
		{
			name: "reserved word rejected where a remainder target is required",
			// "ON" cannot stand in for the REMAINDER data-name.
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DIVIDE A INTO B GIVING C REMAINDER ON SIZE ERROR CONTINUE END-DIVIDE.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 40}, target.Actual.Pos)
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
			name: "MOVE missing TO",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    MOVE A B.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 12}, target.Actual.Pos)
			},
		},
		{
			name: "EVALUATE without END-EVALUATE",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    EVALUATE X WHEN 1 CONTINUE.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 31}, target.Actual.Pos)
			},
		},
		{
			name: "EVALUATE subject is an arithmetic expression ended by EOF",
			// "A + B" is not a valid bare subject operand; running out of tokens
			// must surface UnexpectedEndOfTokensError, not a zero-position token.
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    EVALUATE A + B\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedEndOfTokensError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name: "GO TO without a procedure-name",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    GO TO.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 10}, target.Actual.Pos)
			},
		},
		{
			name: "unterminated subscript",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    MOVE A(1.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 13}, target.Actual.Pos)
			},
		},
		{
			name: "two subscript groups (second must be a reference-modifier)",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    MOVE A(I)(J) TO B.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				// The second group lacks a ":", so the reference-modifier colon is required.
				require.Equal(t, Pos{Line: 4, Column: 16}, target.Actual.Pos)
			},
		},
		{
			name: "IF without a condition",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    IF.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 7}, target.Actual.Pos)
			},
		},
		{
			name: "PERFORM with a non-procedure-name operand",
			// A string literal is neither a count nor a procedure-name; the error
			// reports the real token (a String), not a synthesized identifier.
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    PERFORM \"X\".\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 13}, target.Actual.Pos)
				require.Equal(t, TokenString, target.Actual.Type)
			},
		},
		{
			name: "stray token inside a section body",
			// A token that is neither a paragraph header nor a verb must error rather
			// than loop forever (parseSectionParagraphOpt pre-validation).
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"MY-SEC SECTION.\n" +
				"    +.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 5, Column: 5}, target.Actual.Pos)
			},
		},
		{
			name: "section after loose paragraphs",
			// Once the body is paragraph-form, a SECTION cannot follow.
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. HELLO.\n" +
				"PROCEDURE DIVISION.\n" +
				"    DISPLAY \"a\".\n" +
				"MY-SEC SECTION.\n" +
				"    STOP RUN.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 5, Column: 8}, target.Actual.Pos)
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
			name: "unrecognized SPECIAL-NAMES clause",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. ENV.\n" +
				"ENVIRONMENT DIVISION.\n" +
				"CONFIGURATION SECTION.\n" +
				"SPECIAL-NAMES.\n" +
				"    ALPHABET FOO.\n",
			assert: func(t *testing.T, err error) {
				// An unimplemented/misspelled clause is reported at the clause
				// position, not silently truncated into a misleading later error.
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 6, Column: 5}, target.Actual.Pos)
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
			name: "deferred I-O-CONTROL paragraph after file-control",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. ENV.\n" +
				"ENVIRONMENT DIVISION.\n" +
				"INPUT-OUTPUT SECTION.\n" +
				"FILE-CONTROL.\n" +
				"    SELECT F ASSIGN TO \"f.dat\".\n" +
				"I-O-CONTROL.\n",
			assert: func(t *testing.T, err error) {
				// Reported at the section level, not as a division-dispatch error.
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 7, Column: 1}, target.Actual.Pos)
			},
		},
		{
			name: "SELECT entry outside FILE-CONTROL paragraph",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. ENV.\n" +
				"ENVIRONMENT DIVISION.\n" +
				"INPUT-OUTPUT SECTION.\n" +
				"    SELECT F ASSIGN TO \"f.dat\".\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 5, Column: 5}, target.Actual.Pos)
			},
		},
		{
			name: "invalid data-description level-number",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. X.\n" +
				"DATA DIVISION.\n" +
				"WORKING-STORAGE SECTION.\n" +
				"50 BADLEVEL PIC 9.\n",
			assert: func(t *testing.T, err error) {
				var target InvalidLevelNumberError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 5, Column: 1}, target.Pos)
				require.Equal(t, "50", target.Value)
			},
		},
		{
			name: "deferred file-clause in FD entry",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. X.\n" +
				"DATA DIVISION.\n" +
				"FILE SECTION.\n" +
				"FD CUST-FILE BLOCK CONTAINS 10 RECORDS.\n",
			assert: func(t *testing.T, err error) {
				// File-clauses are deferred (SPEC "« file-clause »"); a non-period
				// token after the file-name is reported rather than consumed.
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 5, Column: 14}, target.Actual.Pos)
			},
		},
		{
			name: "unrecognized data clause keyword",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. X.\n" +
				"DATA DIVISION.\n" +
				"WORKING-STORAGE SECTION.\n" +
				"01 FOO BOGUS.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 5, Column: 8}, target.Actual.Pos)
			},
		},
		{
			name: "level-88 condition-name without VALUE",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. X.\n" +
				"DATA DIVISION.\n" +
				"WORKING-STORAGE SECTION.\n" +
				"01 FLAG PIC X.\n" +
				"88 DONE PIC X.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 6, Column: 9}, target.Actual.Pos)
			},
		},
		{
			name: "call with numeric literal target",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    CALL 5.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 10}, target.Actual.Pos)
			},
		},
		{
			name: "call using with invalid by mode",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    CALL \"P\" USING BY WRONG WS-A.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 23}, target.Actual.Pos)
			},
		},
		{
			name: "call using with no operand before returning",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    CALL \"P\" USING RETURNING WS-RC.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 20}, target.Actual.Pos)
			},
		},
		{
			name: "procedure using with no data-name",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION USING.\n" +
				"    STOP RUN.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 3, Column: 25}, target.Actual.Pos)
			},
		},
		{
			name: "procedure using rejects returning as a data-name",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION USING RETURNING WS-RC.\n" +
				"    STOP RUN.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 3, Column: 26}, target.Actual.Pos)
			},
		},
		{
			name: "end program without program keyword",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    STOP RUN.\n" +
				"END P.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 5, Column: 5}, target.Actual.Pos)
			},
		},
		{
			name: "unknown use specification form",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"DECLARATIVES.\n" +
				"S SECTION.\n" +
				"    USE WHENEVER.\n" +
				"END DECLARATIVES.\n" +
				"MAIN SECTION.\n" +
				"    STOP RUN.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 6, Column: 9}, target.Actual.Pos)
			},
		},
		{
			name: "OPEN without an open-mode group",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    OPEN F-A.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 10}, target.Actual.Pos)
			},
		},
		{
			name: "OPEN mode group with no file-name",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    OPEN INPUT OUTPUT F-B.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 16}, target.Actual.Pos)
			},
		},
		{
			name: "READ AT not followed by END",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    READ F-A AT FOO.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 17}, target.Actual.Pos)
			},
		},
		{
			name: "READ NOT phrase condition does not match the handler",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    READ F-A AT END CONTINUE NOT INVALID KEY CONTINUE.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 34}, target.Actual.Pos)
			},
		},
		{
			name: "START KEY without a relational operator",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    START F-A KEY CUST-ID.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 19}, target.Actual.Pos)
			},
		},
		{
			name: "SORT without a USING or INPUT source",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SORT W-F ON ASCENDING KEY K.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 32}, target.Actual.Pos)
			},
		},
		{
			name: "SORT INPUT not followed by PROCEDURE",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SORT W-F ON ASCENDING KEY K INPUT FOO.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 39}, target.Actual.Pos)
			},
		},
		{
			name: "MERGE without the required USING",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    MERGE M-F ON ASCENDING KEY K GIVING OUT.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 34}, target.Actual.Pos)
			},
		},
		{
			name: "RETURN file-name position holds a reserved verb",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    RETURN DISPLAY \"x\".\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 12}, target.Actual.Pos)
			},
		},
		{
			name: "READ file-name position holds a reserved keyword",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    READ INTO WS-X.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 10}, target.Actual.Pos)
			},
		},
		{
			name: "OPEN file-name position holds an option keyword",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    OPEN INPUT REVERSED CUST-FILE.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 16}, target.Actual.Pos)
			},
		},
		{
			name: "CLOSE WITH not followed by LOCK or NO REWIND",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    CLOSE F-A WITH.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 19}, target.Actual.Pos)
			},
		},
		{
			name: "global is not valid for the debugging use form",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"DECLARATIVES.\n" +
				"S SECTION.\n" +
				"    USE GLOBAL DEBUGGING ON ALL PROCEDURES.\n" +
				"END DECLARATIVES.\n" +
				"MAIN SECTION.\n" +
				"    STOP RUN.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 6, Column: 16}, target.Actual.Pos)
			},
		},
		{
			name: "set without a connector",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SET I BY 1.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 11}, target.Actual.Pos)
			},
		},
		{
			name: "string without into",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    STRING WS-A DELIMITED BY SIZE.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 34}, target.Actual.Pos)
			},
		},
		{
			name: "string with dangling with at end of input",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    STRING WS-A DELIMITED BY SIZE INTO WS-R WITH\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedEndOfTokensError
				require.ErrorAs(t, err, &target)
			},
		},
		{
			name: "inspect without tallying or replacing",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    INSPECT WS-T.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 17}, target.Actual.Pos)
			},
		},
		{
			name: "search without when",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SEARCH WS-T END-SEARCH.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 17}, target.Actual.Pos)
			},
		},
		{
			name: "search all with multiple whens",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SEARCH ALL WS-T WHEN A = 1 WHEN B = 2 END-SEARCH.\n",
			assert: func(t *testing.T, err error) {
				var target SearchAllConstraintError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 32}, target.Pos)
			},
		},
		{
			name: "search all with or condition",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SEARCH ALL WS-T WHEN A = 1 OR B = 2 END-SEARCH.\n",
			assert: func(t *testing.T, err error) {
				var target SearchAllConstraintError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 21}, target.Pos)
			},
		},
		{
			name: "search all with non-equality condition",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    SEARCH ALL WS-T WHEN A > 1 END-SEARCH.\n",
			assert: func(t *testing.T, err error) {
				var target SearchAllConstraintError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 21}, target.Pos)
			},
		},
		{
			name: "initialize with clause keyword as target",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    INITIALIZE ALL TO VALUE.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedTokenError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 16}, target.Actual.Pos)
			},
		},
		{
			name: "initialize all mixed with category to value",
			src: "IDENTIFICATION DIVISION.\n" +
				"PROGRAM-ID. P.\n" +
				"PROCEDURE DIVISION.\n" +
				"    INITIALIZE X ALL NUMERIC TO VALUE.\n",
			assert: func(t *testing.T, err error) {
				var target UnexpectedKeywordError
				require.ErrorAs(t, err, &target)
				require.Equal(t, Pos{Line: 4, Column: 22}, target.Actual.Pos)
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
