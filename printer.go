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
// continues with next when done. ENVIRONMENT and DATA divisions are deferred to
// a later story; until their printers exist (and for a nil Division), an unknown
// type stops the loop with an [UnsupportedNodeError] rather than silently
// dropping the division and emitting invalid source.
func printDivision(div Division, next printerAction) printerAction {
	switch d := div.(type) {
	case *IdentificationDivision:
		return printIdentificationDivision(d, next)
	case *ProcedureDivision:
		return printProcedureDivision(d, next)
	default:
		return failPrint(UnsupportedNodeError{Node: div})
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
