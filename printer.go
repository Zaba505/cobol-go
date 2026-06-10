// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"fmt"
	"io"
)

// Print the given [File] to the given writer as COBOL source.
func Print(w io.Writer, f *File) error {
	pr := &printer{w: w}
	for action := printFile; action != nil && pr.err == nil; {
		action = action(pr, f)
	}
	return pr.err
}

type printer struct {
	w   io.Writer
	err error
}

func (pr *printer) write(s string) {
	if pr.err != nil {
		return
	}
	_, pr.err = io.WriteString(pr.w, s)
}

func (pr *printer) writef(format string, args ...any) {
	if pr.err != nil {
		return
	}
	_, pr.err = fmt.Fprintf(pr.w, format, args...)
}

// setErr records the first error encountered; later calls are no-ops, matching
// the short-circuit in write/writef so the first failure wins.
func (pr *printer) setErr(err error) {
	if pr.err == nil {
		pr.err = err
	}
}

// printerAction is one step of the printer state machine: it writes some
// output and returns the next action. Returning nil ends printing. Errors are
// accumulated in pr.err rather than returned, so the driver loop stops on the
// first write failure.
type printerAction func(pr *printer, f *File) printerAction

// writeThen writes a string and returns the next action — the printer
// equivalent of [yieldTokenThen].
func writeThen(s string, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		pr.write(s)
		return next
	}
}

// failPrint returns a terminal action that records err and stops the loop — the
// printer's only structural-error path (write failures already set pr.err).
func failPrint(err error) printerAction {
	return func(pr *printer, f *File) printerAction {
		pr.setErr(err)
		return nil
	}
}

// printFile is the entry action: it prints each program in source order. The
// empty (zero-value) *File prints nothing, since printProgramAt(0) ends
// immediately when there are no programs. A nil *File is rejected with an
// [UnsupportedNodeError] rather than panicking, since Print is a public API.
func printFile(pr *printer, f *File) printerAction {
	if f == nil {
		return failPrint(UnsupportedNodeError{Node: f})
	}
	return printProgramAt(0)
}

// printProgramAt prints the program at index i, then continues with the program
// after it. It returns nil once i is past the last program, ending the loop. A
// nil program element is rejected rather than panicking on prog.Divisions.
func printProgramAt(i int) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(f.Programs) {
			return nil
		}
		prog := f.Programs[i]
		if prog == nil {
			return failPrint(UnsupportedNodeError{Node: prog})
		}
		return printDivisionAt(prog, 0, printProgramAt(i+1))
	}
}

// printDivisionAt prints prog's division at index i, then continues with the
// division after it; once i is past the last division it continues with next
// (the action that prints the following program).
func printDivisionAt(prog *Program, i int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(prog.Divisions) {
			return next
		}
		return printDivision(prog.Divisions[i], printDivisionAt(prog, i+1, next))
	}
}

// printDivision dispatches to the printer for the concrete division type, which
// continues with next when done. The DATA division is deferred to a later story;
// until its printer exists (and for a nil Division), an unknown type stops the
// loop with an [UnsupportedNodeError] rather than silently dropping the division
// and emitting invalid source.
func printDivision(div Division, next printerAction) printerAction {
	switch d := div.(type) {
	case *IdentificationDivision:
		return printIdentificationDivision(d, next)
	case *EnvironmentDivision:
		return printEnvironmentDivision(d, next)
	case *ProcedureDivision:
		return printProcedureDivision(d, next)
	default:
		return failPrint(UnsupportedNodeError{Node: div})
	}
}

// printEnvironmentDivision prints the ENVIRONMENT DIVISION header followed by its
// optional CONFIGURATION and INPUT-OUTPUT sections. A typed-nil division (a
// Division interface holding a nil *EnvironmentDivision) is rejected with an
// [UnsupportedNodeError] rather than panicking, matching the other printer
// entry points.
func printEnvironmentDivision(div *EnvironmentDivision, next printerAction) printerAction {
	if div == nil {
		return failPrint(UnsupportedNodeError{Node: div})
	}
	return writeThen("ENVIRONMENT DIVISION.\n",
		printConfigurationSection(div.Configuration,
			printInputOutputSection(div.InputOutput, next)))
}

// printConfigurationSection prints the CONFIGURATION SECTION and its optional
// paragraphs; a nil section is skipped by continuing with next.
func printConfigurationSection(sec *ConfigurationSection, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if sec == nil {
			return next
		}
		pr.write("CONFIGURATION SECTION.\n")
		return printSourceComputer(sec.SourceComputer,
			printObjectComputer(sec.ObjectComputer,
				printSpecialNames(sec.SpecialNames, next)))
	}
}

// printSourceComputer prints the SOURCE-COMPUTER paragraph; a nil paragraph is
// skipped. An empty body prints just the header period.
func printSourceComputer(para *SourceComputerParagraph, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if para == nil {
			return next
		}
		pr.write("SOURCE-COMPUTER.")
		if para.ComputerName != nil {
			pr.writef(" %s", para.ComputerName.Value)
			if para.DebuggingMode {
				pr.write(" WITH DEBUGGING MODE")
			}
			pr.write(".")
		}
		pr.write("\n")
		return next
	}
}

// printObjectComputer prints the OBJECT-COMPUTER paragraph; a nil paragraph is
// skipped. An empty body prints just the header period.
func printObjectComputer(para *ObjectComputerParagraph, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if para == nil {
			return next
		}
		pr.write("OBJECT-COMPUTER.")
		if para.ComputerName != nil {
			pr.writef(" %s.", para.ComputerName.Value)
		}
		pr.write("\n")
		return next
	}
}

// printSpecialNames prints the SPECIAL-NAMES paragraph, one clause per indented
// line; the last clause carries the paragraph-terminating period. A nil
// paragraph is skipped. The clause slice is a leaf walked with a local loop, not
// the action machinery.
func printSpecialNames(para *SpecialNamesParagraph, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if para == nil {
			return next
		}
		pr.write("SPECIAL-NAMES.\n")
		for i, clause := range para.Clauses {
			text, ok := specialNamesClauseText(clause)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: clause})
			}
			pr.write("    ")
			pr.write(text)
			if i == len(para.Clauses)-1 {
				pr.write(".")
			}
			pr.write("\n")
		}
		return next
	}
}

// printInputOutputSection prints the INPUT-OUTPUT SECTION and its optional
// FILE-CONTROL paragraph; a nil section is skipped.
func printInputOutputSection(sec *InputOutputSection, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if sec == nil {
			return next
		}
		pr.write("INPUT-OUTPUT SECTION.\n")
		return printFileControl(sec.FileControl, next)
	}
}

// printFileControl prints the FILE-CONTROL paragraph header followed by its
// entries; a nil paragraph is skipped.
func printFileControl(para *FileControlParagraph, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if para == nil {
			return next
		}
		pr.write("FILE-CONTROL.\n")
		return printFileControlEntryAt(para, 0, next)
	}
}

// printFileControlEntryAt prints para's entry at index i, then continues with the
// entry after it; once i is past the last entry it continues with next.
func printFileControlEntryAt(para *FileControlParagraph, i int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(para.Entries) {
			return next
		}
		return printFileControlEntry(para.Entries[i], printFileControlEntryAt(para, i+1, next))
	}
}

// printFileControlEntry prints one SELECT ... ASSIGN entry: the SELECT clause on
// its own indented line, then each select-clause on a continued line, terminated
// with a separator period. A nil entry (a nil element in Entries), a nil
// file-name or assignment target, or an unsupported clause type, stops the loop
// with an [UnsupportedNodeError].
func printFileControlEntry(entry *FileControlEntry, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if entry == nil {
			return failPrint(UnsupportedNodeError{Node: entry})
		}
		if entry.Name == nil {
			return failPrint(UnsupportedNodeError{Node: entry.Name})
		}
		assign, ok := valueText(entry.Assign)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: entry.Assign})
		}
		pr.write("    SELECT ")
		if entry.Optional {
			pr.write("OPTIONAL ")
		}
		pr.writef("%s ASSIGN TO %s", entry.Name.Value, assign)
		for _, clause := range entry.Clauses {
			text, ok := selectClauseText(clause)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: clause})
			}
			pr.write("\n        ")
			pr.write(text)
		}
		pr.write(".\n")
		return next
	}
}

// specialNamesClauseText returns the source text of a SPECIAL-NAMES clause and
// whether the node was a printable clause type.
func specialNamesClauseText(clause SpecialNamesClause) (string, bool) {
	switch c := clause.(type) {
	case *DecimalPointClause:
		return "DECIMAL-POINT IS COMMA", true
	case *CurrencySignClause:
		if c.Sign == nil {
			return "", false
		}
		return "CURRENCY SIGN IS " + c.Sign.Value, true
	default:
		return "", false
	}
}

// selectClauseText returns the source text of a file-control select-clause and
// whether the node was a printable clause type.
func selectClauseText(clause SelectClause) (string, bool) {
	switch c := clause.(type) {
	case *OrganizationClause:
		return "ORGANIZATION IS " + c.Organization, true
	case *AccessClause:
		return "ACCESS MODE IS " + c.Mode, true
	case *RecordKeyClause:
		if c.Name == nil {
			return "", false
		}
		return "RECORD KEY IS " + c.Name.Value, true
	case *FileStatusClause:
		if c.Name == nil {
			return "", false
		}
		return "FILE STATUS IS " + c.Name.Value, true
	default:
		return "", false
	}
}

// printIdentificationDivision prints the IDENTIFICATION DIVISION header followed
// by its PROGRAM-ID paragraph. The keyword spelling is canonicalized to the long
// form (the AST does not record whether the source used ID or IDENTIFICATION).
func printIdentificationDivision(div *IdentificationDivision, next printerAction) printerAction {
	return writeThen("IDENTIFICATION DIVISION.\n", printProgramID(div.ProgramID, next))
}

// printProgramID prints the PROGRAM-ID paragraph naming the program. A missing
// paragraph (nil id) or a program-name of an unsupported value type stops the
// loop with an [UnsupportedNodeError] rather than panicking or emitting a blank
// name.
func printProgramID(id *ProgramID, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if id == nil {
			return failPrint(UnsupportedNodeError{Node: id})
		}
		name, ok := valueText(id.Name)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: id.Name})
		}
		pr.writef("PROGRAM-ID. %s.\n", name)
		return next
	}
}

// printProcedureDivision prints the PROCEDURE DIVISION header followed by its
// statements, one sentence per line.
func printProcedureDivision(div *ProcedureDivision, next printerAction) printerAction {
	return writeThen("PROCEDURE DIVISION.\n", printStatementAt(div, 0, next))
}

// printStatementAt prints div's statement at index i, then continues with the
// statement after it; once i is past the last statement it continues with next.
func printStatementAt(div *ProcedureDivision, i int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(div.Statements) {
			return next
		}
		return printStatement(div.Statements[i], printStatementAt(div, i+1, next))
	}
}

// printStatement dispatches to the printer for the concrete statement type,
// which continues with next when done. An unknown statement type (or a nil
// Statement) stops the loop with an [UnsupportedNodeError] rather than silently
// dropping the statement from the output.
func printStatement(stmt Statement, next printerAction) printerAction {
	switch s := stmt.(type) {
	case *DisplayStatement:
		return printDisplayStatement(s, next)
	case *StopStatement:
		return printStopStatement(s, next)
	default:
		return failPrint(UnsupportedNodeError{Node: stmt})
	}
}

// printDisplayStatement prints a DISPLAY statement: the verb followed by its
// space-separated operands, indented and terminated with a separator period. The
// operand slice is a leaf walked with a local loop, not the action machinery.
func printDisplayStatement(stmt *DisplayStatement, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		pr.write("    DISPLAY")
		for _, op := range stmt.Operands {
			text, ok := valueText(op)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: op})
			}
			pr.write(" ")
			pr.write(text)
		}
		pr.write(".\n")
		return next
	}
}

// printStopStatement prints a STOP statement. Only the STOP RUN form is produced
// by the parser today; the bare-STOP branch is forward-looking and harmless.
func printStopStatement(stmt *StopStatement, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt.Run {
			pr.write("    STOP RUN.\n")
		} else {
			pr.write("    STOP.\n")
		}
		return next
	}
}

// valueText returns the source text of a value node — a [Word]'s spelling or a
// [StringLiteral]'s raw lexeme (including its delimiters) — and reports whether
// the node was a printable value type. The ok flag lets callers surface an
// explicit [UnsupportedNodeError] instead of emitting a blank operand or name.
func valueText(v Type) (string, bool) {
	switch n := v.(type) {
	case *Word:
		return n.Value, true
	case *StringLiteral:
		return n.Value, true
	default:
		return "", false
	}
}

// UnsupportedNodeError is returned by [Print] when it encounters an AST node it
// cannot emit: a nil *File or required child node, or a division, statement, or
// value node of an unknown concrete type. It mirrors the parser's typed errors so
// callers can match it with errors.As.
type UnsupportedNodeError struct {
	// Node is the offending AST node (possibly a typed-nil pointer).
	Node any
}

// Error implements the [error] interface.
func (e UnsupportedNodeError) Error() string {
	return fmt.Sprintf("cannot print unsupported node %T", e.Node)
}
