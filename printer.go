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

// printFile is the entry action: it prints each program in source order. The
// empty (zero-value) *File prints nothing, since printProgramAt(0) ends
// immediately when there are no programs.
func printFile(pr *printer, f *File) printerAction {
	return printProgramAt(0)
}

// printProgramAt prints the program at index i, then continues with the program
// after it. It returns nil once i is past the last program, ending the loop.
func printProgramAt(i int) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(f.Programs) {
			return nil
		}
		return printDivisionAt(f.Programs[i], 0, printProgramAt(i+1))
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
// continues with next when done. ENVIRONMENT and DATA divisions are deferred to
// a later story, so the type set is closed today and the default is unreachable.
func printDivision(div Division, next printerAction) printerAction {
	switch d := div.(type) {
	case *IdentificationDivision:
		return printIdentificationDivision(d, next)
	case *ProcedureDivision:
		return printProcedureDivision(d, next)
	default:
		return next
	}
}

// printIdentificationDivision prints the IDENTIFICATION DIVISION header followed
// by its PROGRAM-ID paragraph. The keyword spelling is canonicalized to the long
// form (the AST does not record whether the source used ID or IDENTIFICATION).
func printIdentificationDivision(div *IdentificationDivision, next printerAction) printerAction {
	return writeThen("IDENTIFICATION DIVISION.\n", printProgramID(div.ProgramID, next))
}

// printProgramID prints the PROGRAM-ID paragraph naming the program.
func printProgramID(id *ProgramID, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		pr.writef("PROGRAM-ID. %s.\n", valueText(id.Name))
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
// which continues with next when done.
func printStatement(stmt Statement, next printerAction) printerAction {
	switch s := stmt.(type) {
	case *DisplayStatement:
		return printDisplayStatement(s, next)
	case *StopStatement:
		return printStopStatement(s, next)
	default:
		return next
	}
}

// printDisplayStatement prints a DISPLAY statement: the verb followed by its
// space-separated operands, indented and terminated with a separator period. The
// operand slice is a leaf walked with a local loop, not the action machinery.
func printDisplayStatement(stmt *DisplayStatement, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		pr.write("    DISPLAY")
		for _, op := range stmt.Operands {
			pr.write(" ")
			pr.write(valueText(op))
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

// valueText returns the source text of a value node: a [Word]'s spelling or a
// [StringLiteral]'s raw lexeme (including its delimiters).
func valueText(v Type) string {
	switch n := v.(type) {
	case *Word:
		return n.Value
	case *StringLiteral:
		return n.Value
	default:
		return ""
	}
}
