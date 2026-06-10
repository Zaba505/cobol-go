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
// continues with next when done. An unknown type (and a nil Division) stops the
// loop with an [UnsupportedNodeError] rather than silently dropping the division
// and emitting invalid source.
func printDivision(div Division, next printerAction) printerAction {
	switch d := div.(type) {
	case *IdentificationDivision:
		return printIdentificationDivision(d, next)
	case *EnvironmentDivision:
		return printEnvironmentDivision(d, next)
	case *DataDivision:
		return printDataDivision(d, next)
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
		// WITH DEBUGGING MODE qualifies a computer-name (grammar:
		// computer-name [ WITH DEBUGGING MODE ]); the flag set without a name is
		// an inconsistent AST that would not round-trip, so reject it rather than
		// silently dropping the flag.
		if para.ComputerName == nil && para.DebuggingMode {
			return failPrint(UnsupportedNodeError{Node: para})
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
		if c == nil {
			return "", false
		}
		return "DECIMAL-POINT IS COMMA", true
	case *CurrencySignClause:
		if c == nil || c.Sign == nil {
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
		if c == nil {
			return "", false
		}
		return "ORGANIZATION IS " + c.Organization, true
	case *AccessClause:
		if c == nil {
			return "", false
		}
		return "ACCESS MODE IS " + c.Mode, true
	case *RecordKeyClause:
		if c == nil || c.Name == nil {
			return "", false
		}
		return "RECORD KEY IS " + c.Name.Value, true
	case *FileStatusClause:
		if c == nil || c.Name == nil {
			return "", false
		}
		return "FILE STATUS IS " + c.Name.Value, true
	default:
		return "", false
	}
}

// printIdentificationDivision prints the IDENTIFICATION DIVISION header followed
// by its PROGRAM-ID paragraph. The keyword spelling is canonicalized to the long
// form (the AST does not record whether the source used ID or IDENTIFICATION). A
// typed-nil division is rejected with an [UnsupportedNodeError] rather than
// panicking.
func printIdentificationDivision(div *IdentificationDivision, next printerAction) printerAction {
	if div == nil {
		return failPrint(UnsupportedNodeError{Node: div})
	}
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

// printDataDivision prints the DATA DIVISION header followed by its optional
// FILE, WORKING-STORAGE, LOCAL-STORAGE, and LINKAGE sections in fixed order. A
// typed-nil division is rejected with an [UnsupportedNodeError] rather than
// panicking, matching the other printer entry points.
func printDataDivision(div *DataDivision, next printerAction) printerAction {
	if div == nil {
		return failPrint(UnsupportedNodeError{Node: div})
	}
	return writeThen("DATA DIVISION.\n",
		printFileSection(div.File,
			printDataSection("WORKING-STORAGE", div.WorkingStorage,
				printDataSection("LOCAL-STORAGE", div.LocalStorage,
					printDataSection("LINKAGE", div.Linkage, next)))))
}

// printFileSection prints the FILE SECTION header followed by its FD/SD entries;
// a nil section is skipped.
func printFileSection(sec *FileSection, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if sec == nil {
			return next
		}
		pr.write("FILE SECTION.\n")
		return printFileDescriptionEntryAt(sec, 0, next)
	}
}

// printFileDescriptionEntryAt prints sec's FD/SD entry at index i, then continues
// with the entry after it; once i is past the last entry it continues with next.
func printFileDescriptionEntryAt(sec *FileSection, i int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(sec.Entries) {
			return next
		}
		return printFileDescriptionEntry(sec.Entries[i], printFileDescriptionEntryAt(sec, i+1, next))
	}
}

// printFileDescriptionEntry prints one FD/SD file-description entry: the "FD
// file-name." (or SD) header line, then its subordinate record entries. A nil
// entry, a nil file-name, or an unrecognized Kind stops the loop with an
// [UnsupportedNodeError].
func printFileDescriptionEntry(entry *FileDescriptionEntry, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if entry == nil || entry.Name == nil || (entry.Kind != "FD" && entry.Kind != "SD") {
			return failPrint(UnsupportedNodeError{Node: entry})
		}
		pr.writef("%s %s.\n", entry.Kind, entry.Name.Value)
		return printDataEntryAt(entry.Records, 0, next)
	}
}

// printDataSection prints a WORKING-STORAGE/LOCAL-STORAGE/LINKAGE section: the
// "<header> SECTION." line followed by its data-description entries; a nil
// section is skipped.
func printDataSection(header string, sec *DataSection, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if sec == nil {
			return next
		}
		pr.writef("%s SECTION.\n", header)
		return printDataEntryAt(sec.Entries, 0, next)
	}
}

// printDataEntryAt prints the data-description entry at index i of entries, then
// continues with the entry after it; once i is past the last entry it continues
// with next.
func printDataEntryAt(entries []*DataDescriptionEntry, i int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(entries) {
			return next
		}
		return printDataEntry(entries[i], printDataEntryAt(entries, i+1, next))
	}
}

// printDataEntry prints one data-description entry: the level-number (two
// digits), the data-name or FILLER, then each clause, terminated with a separator
// period. The level is indented four spaces (a canonical layout; free-format
// round-trips ignore positions — SPEC "Reference format independence"). A nil
// entry or an unsupported clause type stops the loop with an [UnsupportedNodeError].
func printDataEntry(entry *DataDescriptionEntry, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if entry == nil {
			return failPrint(UnsupportedNodeError{Node: entry})
		}
		pr.writef("    %02d", entry.Level)
		if entry.Filler {
			pr.write(" FILLER")
		} else if entry.Name != nil {
			pr.writef(" %s", entry.Name.Value)
		}
		for _, clause := range entry.Clauses {
			text, ok := dataClauseText(clause)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: clause})
			}
			pr.write(" ")
			pr.write(text)
		}
		pr.write(".\n")
		return next
	}
}

// dataClauseText returns the source text of a data-description clause and whether
// the node was a printable clause type. It canonicalizes optional noise words
// (IS, KEY, ON, BY, CHARACTER) and the THROUGH/THRU and JUSTIFIED spellings.
func dataClauseText(clause DataClause) (string, bool) {
	switch c := clause.(type) {
	case *RedefinesClause:
		if c == nil || c.Name == nil {
			return "", false
		}
		return "REDEFINES " + c.Name.Value, true
	case *PictureClause:
		if c == nil {
			return "", false
		}
		return "PIC " + c.Picture, true
	case *UsageClause:
		if c == nil {
			return "", false
		}
		return "USAGE " + c.Usage, true
	case *ValueClause:
		return valueClauseText(c)
	case *OccursClause:
		return occursClauseText(c)
	case *SignClause:
		if c == nil || (c.Position != "LEADING" && c.Position != "TRAILING") {
			return "", false
		}
		s := "SIGN IS " + c.Position
		if c.Separate {
			s += " SEPARATE CHARACTER"
		}
		return s, true
	case *JustifiedClause:
		if c == nil {
			return "", false
		}
		return "JUSTIFIED RIGHT", true
	case *SynchronizedClause:
		if c == nil {
			return "", false
		}
		if c.Direction != "" {
			return "SYNCHRONIZED " + c.Direction, true
		}
		return "SYNCHRONIZED", true
	case *BlankWhenZeroClause:
		if c == nil {
			return "", false
		}
		return "BLANK WHEN ZERO", true
	case *GlobalClause:
		if c == nil {
			return "", false
		}
		return "GLOBAL", true
	case *ExternalClause:
		if c == nil {
			return "", false
		}
		return "EXTERNAL", true
	case *RenamesClause:
		if c == nil || c.From == nil {
			return "", false
		}
		s := "RENAMES " + c.From.Value
		if c.Through != nil {
			s += " THROUGH " + c.Through.Value
		}
		return s, true
	default:
		return "", false
	}
}

// valueClauseText returns the text of a VALUE clause: each value-spec, joined by
// spaces, a spec being "literal" or "literal THROUGH literal".
func valueClauseText(c *ValueClause) (string, bool) {
	if c == nil || len(c.Values) == 0 {
		return "", false
	}
	s := "VALUE"
	for _, spec := range c.Values {
		from, ok := valueText(spec.From)
		if !ok {
			return "", false
		}
		s += " " + from
		if spec.Through != nil {
			through, ok := valueText(spec.Through)
			if !ok {
				return "", false
			}
			s += " THROUGH " + through
		}
	}
	return s, true
}

// occursClauseText returns the text of an OCCURS clause:
// "OCCURS n [TO m] TIMES [DEPENDING ON d] {ASCENDING|DESCENDING KEY IS k}
// [INDEXED BY i]".
func occursClauseText(c *OccursClause) (string, bool) {
	if c == nil || c.Min == nil {
		return "", false
	}
	s := "OCCURS " + c.Min.Value
	if c.Max != nil {
		s += " TO " + c.Max.Value
	}
	s += " TIMES"
	if c.DependingOn != nil {
		s += " DEPENDING ON " + c.DependingOn.Value
	}
	for _, key := range c.Keys {
		if key.Name == nil {
			return "", false
		}
		if key.Ascending {
			s += " ASCENDING KEY IS " + key.Name.Value
		} else {
			s += " DESCENDING KEY IS " + key.Name.Value
		}
	}
	if c.IndexedBy != nil {
		s += " INDEXED BY " + c.IndexedBy.Value
	}
	return s, true
}

// printProcedureDivision prints the PROCEDURE DIVISION header followed by its
// body — either its paragraphs or its sections. A typed-nil division, or one with
// both Paragraphs and Sections set (the two body forms are mutually exclusive), is
// rejected with an [UnsupportedNodeError] rather than panicking.
func printProcedureDivision(div *ProcedureDivision, next printerAction) printerAction {
	if div == nil || (len(div.Paragraphs) > 0 && len(div.Sections) > 0) {
		return failPrint(UnsupportedNodeError{Node: div})
	}
	return writeThen("PROCEDURE DIVISION.\n",
		printParagraphAt(div.Paragraphs, 0,
			printSectionAt(div.Sections, 0, next)))
}

// printSectionAt prints the section at index i, then continues with the section
// after it; once i is past the last section it continues with next.
func printSectionAt(secs []*Section, i int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(secs) {
			return next
		}
		return printSection(secs[i], printSectionAt(secs, i+1, next))
	}
}

// printSection prints a section-name header (with its optional segment number)
// followed by the section's paragraphs. A typed-nil section or one missing its
// name is rejected with an [UnsupportedNodeError].
func printSection(sec *Section, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if sec == nil || sec.Name == nil {
			return failPrint(UnsupportedNodeError{Node: sec})
		}
		header := sec.Name.Value + " SECTION"
		if sec.Segment != nil {
			header += " " + sec.Segment.Value
		}
		return writeThen(header+".\n", printParagraphAt(sec.Paragraphs, 0, next))
	}
}

// printParagraphAt prints the paragraph at index i, then continues with the
// paragraph after it; once i is past the last paragraph it continues with next.
func printParagraphAt(paras []*Paragraph, i int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(paras) {
			return next
		}
		return printParagraph(paras[i], printParagraphAt(paras, i+1, next))
	}
}

// printParagraph prints an optional paragraph-name header followed by the
// paragraph's sentences. The anonymous leading paragraph (nil Name) prints no
// header. A typed-nil paragraph is rejected with an [UnsupportedNodeError].
func printParagraph(para *Paragraph, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if para == nil {
			return failPrint(UnsupportedNodeError{Node: para})
		}
		body := printSentenceAt(para.Sentences, 0, next)
		if para.Name == nil {
			return body
		}
		return writeThen(para.Name.Value+".\n", body)
	}
}

// printSentenceAt prints the sentence at index i, then continues with the
// sentence after it; once i is past the last sentence it continues with next.
func printSentenceAt(sents []*Sentence, i int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(sents) {
			return next
		}
		return printSentence(sents[i], printSentenceAt(sents, i+1, next))
	}
}

// printSentence prints a sentence's statements, the last terminated by the
// separator period that ends the sentence. A typed-nil or empty sentence is
// rejected rather than emitting a bare period.
func printSentence(sent *Sentence, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if sent == nil || len(sent.Statements) == 0 {
			return failPrint(UnsupportedNodeError{Node: sent})
		}
		return printSentenceStatementAt(sent, 0, next)
	}
}

// printSentenceStatementAt prints sent's statement at index j on its own line.
// The final statement is followed by the sentence-terminating period; all others
// by a newline. The period belongs to the sentence, not the statement, so a
// statement printer never emits it (it may emit its own scope terminator, e.g.
// END-IF, which is separate from the sentence period).
func printSentenceStatementAt(sent *Sentence, j int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if j >= len(sent.Statements) {
			return next
		}
		sep := "\n"
		if j == len(sent.Statements)-1 {
			sep = ".\n"
		}
		return printStatement(sent.Statements[j], writeThen(sep, printSentenceStatementAt(sent, j+1, next)))
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
	case *MoveStatement:
		return printMoveStatement(s, next)
	case *AcceptStatement:
		return printAcceptStatement(s, next)
	case *GoToStatement:
		return printGoToStatement(s, next)
	case *ContinueStatement:
		return printContinueStatement(s, next)
	case *StopStatement:
		return printStopStatement(s, next)
	default:
		return failPrint(UnsupportedNodeError{Node: stmt})
	}
}

// printDisplayStatement prints a DISPLAY statement: the indented verb, its
// space-separated operands, an optional UPON mnemonic, and an optional WITH NO
// ADVANCING phrase. The sentence-terminating period is emitted by the enclosing
// sentence, not here. A typed-nil statement is rejected with an
// [UnsupportedNodeError].
func printDisplayStatement(stmt *DisplayStatement, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write("    DISPLAY")
		for _, op := range stmt.Operands {
			text, ok := valueText(op)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: op})
			}
			pr.write(" ")
			pr.write(text)
		}
		if stmt.Upon != nil {
			pr.write(" UPON " + stmt.Upon.Value)
		}
		if stmt.NoAdvancing {
			pr.write(" WITH NO ADVANCING")
		}
		return next
	}
}

// printMoveStatement prints a MOVE statement: the optional CORRESPONDING, the
// sending operand, "TO", and the receiving identifiers. A typed-nil statement or
// an unprintable operand is rejected with an [UnsupportedNodeError].
func printMoveStatement(stmt *MoveStatement, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write("    MOVE ")
		if stmt.Corresponding {
			pr.write("CORRESPONDING ")
		}
		src, ok := valueText(stmt.Source)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: stmt.Source})
		}
		pr.write(src)
		pr.write(" TO")
		for _, t := range stmt.Targets {
			text, ok := identifierText(t)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: t})
			}
			pr.write(" " + text)
		}
		return next
	}
}

// printAcceptStatement prints an ACCEPT statement: the receiving identifier and an
// optional FROM source. A typed-nil statement or an unprintable target is rejected
// with an [UnsupportedNodeError].
func printAcceptStatement(stmt *AcceptStatement, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		target, ok := identifierText(stmt.Target)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: stmt.Target})
		}
		pr.write("    ACCEPT " + target)
		if stmt.From != nil {
			pr.write(" FROM " + stmt.From.Value)
		}
		return next
	}
}

// printGoToStatement prints a GO TO statement: the procedure-names and an optional
// DEPENDING ON selector. A typed-nil statement, one with no targets, or an
// unprintable selector is rejected with an [UnsupportedNodeError].
func printGoToStatement(stmt *GoToStatement, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil || len(stmt.Targets) == 0 {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write("    GO TO")
		for _, t := range stmt.Targets {
			if t == nil {
				return failPrint(UnsupportedNodeError{Node: stmt})
			}
			pr.write(" " + t.Value)
		}
		if stmt.DependingOn != nil {
			dep, ok := identifierText(stmt.DependingOn)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: stmt.DependingOn})
			}
			pr.write(" DEPENDING ON " + dep)
		}
		return next
	}
}

// printContinueStatement prints a CONTINUE statement. A typed-nil statement is
// rejected with an [UnsupportedNodeError].
func printContinueStatement(stmt *ContinueStatement, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write("    CONTINUE")
		return next
	}
}

// printStopStatement prints a STOP statement (the indented verb; the sentence
// emits the terminating period): STOP RUN or STOP <literal>. A typed-nil statement
// or a STOP form with neither RUN nor a literal is rejected with an
// [UnsupportedNodeError].
func printStopStatement(stmt *StopStatement, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		switch {
		case stmt.Run:
			pr.write("    STOP RUN")
		case stmt.Literal != nil:
			text, ok := valueText(stmt.Literal)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: stmt.Literal})
			}
			pr.write("    STOP " + text)
		default:
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		return next
	}
}

// valueText returns the source text of a value node — a [Word]'s spelling, a
// literal's raw lexeme (including its delimiters), or an [Identifier]'s rendered
// reference — and reports whether the node was a printable value type. The ok flag
// lets callers surface an explicit [UnsupportedNodeError] instead of emitting a
// blank operand or name.
func valueText(v Type) (string, bool) {
	switch n := v.(type) {
	case *Word:
		if n == nil {
			return "", false
		}
		return n.Value, true
	case *StringLiteral:
		if n == nil {
			return "", false
		}
		return n.Value, true
	case *NumericLiteral:
		if n == nil {
			return "", false
		}
		return n.Value, true
	case *Identifier:
		return identifierText(n)
	default:
		return "", false
	}
}

// exprText returns the canonical source text of an arithmetic expression and
// whether it could be rendered. Binary operators are surrounded by single spaces,
// a unary sign is attached to its operand, and parenthesized groups are preserved
// so the printed expression re-parses to the same tree.
func exprText(e Expr) (string, bool) {
	switch n := e.(type) {
	case *Identifier:
		return identifierText(n)
	case *NumericLiteral:
		if n == nil {
			return "", false
		}
		return n.Value, true
	case *StringLiteral:
		if n == nil {
			return "", false
		}
		return n.Value, true
	case *BinaryExpr:
		if n == nil {
			return "", false
		}
		l, ok := exprText(n.Left)
		if !ok {
			return "", false
		}
		r, ok := exprText(n.Right)
		if !ok {
			return "", false
		}
		return l + " " + n.Op + " " + r, true
	case *UnaryExpr:
		if n == nil {
			return "", false
		}
		o, ok := exprText(n.Operand)
		if !ok {
			return "", false
		}
		return n.Op + o, true
	case *ParenExpr:
		if n == nil {
			return "", false
		}
		inner, ok := exprText(n.Expr)
		if !ok {
			return "", false
		}
		return "(" + inner + ")", true
	default:
		return "", false
	}
}

// identifierText returns the canonical source text of a data reference: its name,
// any IN/OF qualifiers, an optional subscript list, and an optional
// reference-modifier. Subscripts are space-separated; the reference-modifier uses
// the "(start:length)" form with the length omitted when nil.
func identifierText(id *Identifier) (string, bool) {
	if id == nil || id.Name == nil {
		return "", false
	}
	s := id.Name.Value
	for _, q := range id.Qualifiers {
		if q == nil {
			return "", false
		}
		s += " OF " + q.Value
	}
	if len(id.Subscripts) > 0 {
		s += "("
		for i, sub := range id.Subscripts {
			t, ok := exprText(sub)
			if !ok {
				return "", false
			}
			if i > 0 {
				s += " "
			}
			s += t
		}
		s += ")"
	}
	if id.RefMod != nil {
		start, ok := exprText(id.RefMod.Start)
		if !ok {
			return "", false
		}
		s += "(" + start + ":"
		if id.RefMod.Length != nil {
			l, ok := exprText(id.RefMod.Length)
			if !ok {
				return "", false
			}
			s += l
		}
		s += ")"
	}
	return s, true
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
