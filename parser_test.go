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
