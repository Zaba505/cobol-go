// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"fmt"
	"io"
	"strings"
)

// indentUnit is one level of free-format indentation. In free format columns are
// insignificant (SPEC.md "Reference format independence"), so the printer chooses a
// canonical layout: each nesting level — a statement under a paragraph, an inline
// IF/PERFORM body under its header, a subordinate data item under its group — is
// offset by one indentUnit from its parent.
const indentUnit = "    "

// indent returns the leading whitespace for the given nesting depth.
func indent(depth int) string {
	return strings.Repeat(indentUnit, depth)
}

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
		return printDataEntryAt(entry.Records, dataEntryDepths(entry.Records), 0, next)
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
		return printDataEntryAt(sec.Entries, dataEntryDepths(sec.Entries), 0, next)
	}
}

// dataEntryDepths maps each entry in a record to its nesting depth from the level
// numbers, which carry the subordination an entry list flattens: a group item's
// subordinates have higher level numbers than it. Levels 01–49 nest via a stack of
// open group levels; an 88 condition-name sits one level under the entry it
// qualifies; 66 (RENAMES) and 77 (independent item) sit at the record level. The
// printer uses these depths to indent each entry under its parent (free-format
// columns are insignificant — SPEC.md "Reference format independence").
func dataEntryDepths(entries []*DataDescriptionEntry) []int {
	depths := make([]int, len(entries))
	var stack []int  // levels of the currently-open group items (01–49)
	parentDepth := 0 // depth of the most recent non-88 entry — the parent of any 88
	for i, e := range entries {
		if e == nil {
			// A nil entry has no level to subordinate; printDataEntry rejects it.
			continue
		}
		switch e.Level {
		case 88:
			depths[i] = parentDepth + 1
		case 66, 77:
			// RENAMES (66) and independent items (77) sit at the record level
			// (depth 0). They don't disturb the 01–49 nesting stack — the
			// hierarchy is defined by 01–49 level numbers alone, and a later 01
			// pops the stack on its own — but they do become the parent of any
			// subordinate 88 (a 77 may carry condition-names).
			depths[i] = 0
			parentDepth = 0
		default: // 01–49
			for len(stack) > 0 && stack[len(stack)-1] >= e.Level {
				stack = stack[:len(stack)-1]
			}
			depths[i] = len(stack)
			stack = append(stack, e.Level)
			parentDepth = depths[i]
		}
	}
	return depths
}

// printDataEntryAt prints the data-description entry at index i of entries (at the
// matching depth in depths), then continues with the entry after it; once i is past
// the last entry it continues with next.
func printDataEntryAt(entries []*DataDescriptionEntry, depths []int, i int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(entries) {
			return next
		}
		return printDataEntry(entries[i], depths[i], printDataEntryAt(entries, depths, i+1, next))
	}
}

// printDataEntry prints one data-description entry: the level-number (two
// digits), the data-name or FILLER, then each clause, terminated with a separator
// period. The entry is indented by depth — its subordination level within the
// record (a canonical layout; free-format round-trips ignore positions — SPEC
// "Reference format independence"). A nil entry or an unsupported clause type stops
// the loop with an [UnsupportedNodeError].
func printDataEntry(entry *DataDescriptionEntry, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if entry == nil {
			return failPrint(UnsupportedNodeError{Node: entry})
		}
		pr.writef("%s%02d", indent(depth), entry.Level)
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
		return printStatement(sent.Statements[j], 1, writeThen(sep, printSentenceStatementAt(sent, j+1, next)))
	}
}

// printStatement dispatches to the printer for the concrete statement type,
// which continues with next when done. An unknown statement type (or a nil
// Statement) stops the loop with an [UnsupportedNodeError] rather than silently
// dropping the statement from the output.
func printStatement(stmt Statement, depth int, next printerAction) printerAction {
	switch s := stmt.(type) {
	case *DisplayStatement:
		return printDisplayStatement(s, depth, next)
	case *MoveStatement:
		return printMoveStatement(s, depth, next)
	case *AcceptStatement:
		return printAcceptStatement(s, depth, next)
	case *ArithmeticStatement:
		return printArithmeticStatement(s, depth, next)
	case *ComputeStatement:
		return printComputeStatement(s, depth, next)
	case *IfStatement:
		return printIfStatement(s, depth, next)
	case *PerformStatement:
		return printPerformStatement(s, depth, next)
	case *EvaluateStatement:
		return printEvaluateStatement(s, depth, next)
	case *GoToStatement:
		return printGoToStatement(s, depth, next)
	case *ContinueStatement:
		return printContinueStatement(s, depth, next)
	case *StopStatement:
		return printStopStatement(s, depth, next)
	default:
		return failPrint(UnsupportedNodeError{Node: stmt})
	}
}

// printDisplayStatement prints a DISPLAY statement: the indented verb, its
// space-separated operands, an optional UPON mnemonic, and an optional WITH NO
// ADVANCING phrase. The sentence-terminating period is emitted by the enclosing
// sentence, not here. A typed-nil statement is rejected with an
// [UnsupportedNodeError].
func printDisplayStatement(stmt *DisplayStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write(indent(depth) + "DISPLAY")
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
// sending operand, "TO", and the receiving identifiers. A typed-nil statement, one
// with no receiving identifier (MOVE requires at least one), or an unprintable
// operand is rejected with an [UnsupportedNodeError].
func printMoveStatement(stmt *MoveStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil || len(stmt.Targets) == 0 {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write(indent(depth) + "MOVE ")
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
func printAcceptStatement(stmt *AcceptStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		target, ok := identifierText(stmt.Target)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: stmt.Target})
		}
		pr.write(indent(depth) + "ACCEPT " + target)
		if stmt.From != nil {
			pr.write(" FROM " + stmt.From.Value)
		}
		return next
	}
}

// printArithmeticStatement prints an ADD/SUBTRACT/MULTIPLY/DIVIDE statement: the
// verb, source operands, optional connector and in-place targets, optional GIVING
// result, optional ROUNDED, and optional END-<verb> terminator. A statement with
// no verb, no operands, or no receiving field is rejected with an
// [UnsupportedNodeError]. The connector and in-place targets are paired: a
// connector without targets, or targets without a connector (which would silently
// drop the targets), is also rejected.
func printArithmeticStatement(stmt *ArithmeticStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil || stmt.Verb == "" || len(stmt.Operands) == 0 ||
			(len(stmt.Targets) == 0 && stmt.Giving == nil) ||
			(stmt.Connector == "") != (len(stmt.Targets) == 0) {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write(indent(depth) + stmt.Verb)
		for _, op := range stmt.Operands {
			text, ok := valueText(op)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: op})
			}
			pr.write(" " + text)
		}
		if stmt.Connector != "" {
			pr.write(" " + stmt.Connector)
			for _, t := range stmt.Targets {
				text, ok := identifierText(t)
				if !ok {
					return failPrint(UnsupportedNodeError{Node: t})
				}
				pr.write(" " + text)
			}
		}
		if stmt.Giving != nil {
			text, ok := identifierText(stmt.Giving)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: stmt.Giving})
			}
			pr.write(" GIVING " + text)
		}
		if stmt.Rounded {
			pr.write(" ROUNDED")
		}
		if stmt.EndScope {
			pr.write(" END-" + stmt.Verb)
		}
		return next
	}
}

// printComputeStatement prints a COMPUTE statement: the receiving fields (each
// optionally ROUNDED), "=", the arithmetic expression, and an optional
// END-COMPUTE. A statement with no targets or an unprintable expression is
// rejected with an [UnsupportedNodeError].
func printComputeStatement(stmt *ComputeStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil || len(stmt.Targets) == 0 {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write(indent(depth) + "COMPUTE")
		for _, tgt := range stmt.Targets {
			text, ok := identifierText(tgt.Name)
			if !ok {
				return failPrint(UnsupportedNodeError{Node: tgt.Name})
			}
			pr.write(" " + text)
			if tgt.Rounded {
				pr.write(" ROUNDED")
			}
		}
		e, ok := exprText(stmt.Expr)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: stmt.Expr})
		}
		pr.write(" = " + e)
		if stmt.EndScope {
			pr.write(" END-COMPUTE")
		}
		return next
	}
}

// printIfStatement prints an IF statement: the condition, the then-branch
// statements (each on its own line), an optional ELSE branch, and an optional
// END-IF. The sentence-terminating period is emitted by the enclosing sentence, so
// the final line carries no period here. A typed-nil statement or a missing
// condition is rejected with an [UnsupportedNodeError].
func printIfStatement(stmt *IfStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil || stmt.Cond == nil {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		cond, ok := conditionText(stmt.Cond)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: stmt.Cond})
		}
		pr.write(indent(depth) + "IF " + cond)

		end := next
		if stmt.EndIf {
			end = writeThen("\n"+indent(depth)+"END-IF", end)
		}
		tail := end
		if stmt.HasElse {
			tail = writeThen("\n"+indent(depth)+"ELSE", printBranchStatementAt(stmt.Else, 0, depth+1, end))
		}
		return printBranchStatementAt(stmt.Then, 0, depth+1, tail)
	}
}

// printPerformStatement prints a PERFORM statement. The out-of-line form prints the
// procedure-name(s) and loop on one line; the inline form prints the loop, the body
// statements (each on its own line), and END-PERFORM. The sentence-terminating
// period is emitted by the enclosing sentence. A typed-nil statement, an
// out-of-line PERFORM with no target, an inline PERFORM without END-PERFORM (which
// would merge with following statements on re-parse), or an unprintable loop is
// rejected with an [UnsupportedNodeError].
func printPerformStatement(stmt *PerformStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		loop, ok := performLoopText(stmt)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}

		if !stmt.Inline {
			if stmt.Target == nil {
				return failPrint(UnsupportedNodeError{Node: stmt})
			}
			pr.write(indent(depth) + "PERFORM " + stmt.Target.Value)
			if stmt.Through != nil {
				pr.write(" THROUGH " + stmt.Through.Value)
			}
			pr.write(loop)
			return next
		}

		// An inline PERFORM is delimited by END-PERFORM; without it the body would
		// merge with the following statements on re-parse, so reject it.
		if !stmt.EndPerform {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write(indent(depth) + "PERFORM" + loop)
		return printBranchStatementAt(stmt.Body, 0, depth+1, writeThen("\n"+indent(depth)+"END-PERFORM", next))
	}
}

// printEvaluateStatement prints an EVALUATE statement: the ALSO-joined subjects on
// the header line, each WHEN (and the optional WHEN OTHER) clause with its branch
// statements on their own lines, and the closing END-EVALUATE. Following the IF
// printer, the WHEN clauses sit at the statement's own depth and their bodies one
// level deeper; the sentence-terminating period is emitted by the enclosing
// sentence. A typed-nil statement, one with no subjects, or an unprintable
// subject/object is rejected with an [UnsupportedNodeError].
func printEvaluateStatement(stmt *EvaluateStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil || len(stmt.Subjects) == 0 {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		subjects, ok := evaluateSubjectsText(stmt.Subjects)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write(indent(depth) + "EVALUATE " + subjects)
		end := writeThen("\n"+indent(depth)+"END-EVALUATE", next)
		return printEvaluateWhenAt(stmt, 0, depth, end)
	}
}

// printEvaluateWhenAt prints the WHEN branch at index i and chains to the next, then
// to the WHEN OTHER branch once the indexed branches are exhausted.
func printEvaluateWhenAt(stmt *EvaluateStatement, i int, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(stmt.Whens) {
			return printEvaluateOther(stmt, depth, next)
		}
		when := stmt.Whens[i]
		if when == nil || len(when.Objects) == 0 {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		objects, ok := evaluateObjectsText(when.Objects)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write("\n" + indent(depth) + "WHEN " + objects)
		return printBranchStatementAt(when.Body, 0, depth+1, printEvaluateWhenAt(stmt, i+1, depth, next))
	}
}

// printEvaluateOther prints the WHEN OTHER branch when present, then continues.
func printEvaluateOther(stmt *EvaluateStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if !stmt.HasOther {
			return next
		}
		pr.write("\n" + indent(depth) + "WHEN OTHER")
		return printBranchStatementAt(stmt.Other, 0, depth+1, next)
	}
}

// evaluateSubjectsText renders the ALSO-joined subjects of an EVALUATE header.
func evaluateSubjectsText(subjects []*EvaluateSubject) (string, bool) {
	parts := make([]string, len(subjects))
	for i, s := range subjects {
		t, ok := evaluateSubjectText(s)
		if !ok {
			return "", false
		}
		parts[i] = t
	}
	return strings.Join(parts, " ALSO "), true
}

// evaluateSubjectText renders one EVALUATE subject: a boolean keyword, a condition,
// or an operand. Exactly one of Bool/Cond/Operand must be set; an inconsistent node
// is rejected (false) rather than silently dropping a field, so the print stays
// round-trippable.
func evaluateSubjectText(s *EvaluateSubject) (string, bool) {
	if s == nil {
		return "", false
	}
	set := 0
	if s.Bool != "" {
		set++
	}
	if s.Cond != nil {
		set++
	}
	if s.Operand != nil {
		set++
	}
	if set != 1 {
		return "", false
	}
	switch {
	case s.Bool != "":
		return s.Bool, true
	case s.Cond != nil:
		return conditionText(s.Cond)
	default:
		return valueText(s.Operand)
	}
}

// evaluateObjectsText renders the ALSO-joined objects of a WHEN branch.
func evaluateObjectsText(objects []*EvaluateObject) (string, bool) {
	parts := make([]string, len(objects))
	for i, o := range objects {
		t, ok := evaluateObjectText(o)
		if !ok {
			return "", false
		}
		parts[i] = t
	}
	return strings.Join(parts, " ALSO "), true
}

// evaluateObjectText renders one WHEN object: ANY, a boolean keyword, or an optional
// leading NOT applied to an operand (with an optional THROUGH range) or a condition.
// The union invariants are validated up front and an inconsistent node is rejected
// (false) so a malformed AST cannot print a lossy, non-round-trippable object:
// exactly one of Any/Bool/Cond/Operand must be set, a THROUGH range requires the
// operand form, and a leading NOT cannot combine with ANY or a boolean.
func evaluateObjectText(o *EvaluateObject) (string, bool) {
	if o == nil {
		return "", false
	}
	set := 0
	if o.Any {
		set++
	}
	if o.Bool != "" {
		set++
	}
	if o.Cond != nil {
		set++
	}
	if o.Operand != nil {
		set++
	}
	if set != 1 {
		return "", false
	}
	if o.Through != nil && o.Operand == nil {
		return "", false
	}
	if o.Not && (o.Any || o.Bool != "") {
		return "", false
	}

	switch {
	case o.Any:
		return "ANY", true
	case o.Bool != "":
		return o.Bool, true
	}
	var inner string
	switch {
	case o.Cond != nil:
		c, ok := conditionText(o.Cond)
		if !ok {
			return "", false
		}
		inner = c
	default:
		v, ok := valueText(o.Operand)
		if !ok {
			return "", false
		}
		inner = v
		if o.Through != nil {
			t, ok := valueText(o.Through)
			if !ok {
				return "", false
			}
			inner += " THROUGH " + t
		}
	}
	return negate(o.Not) + inner, true
}

// performLoopText returns the canonical text of a PERFORM loop specification, with
// a leading space when non-empty, and whether it could be rendered. WITH TEST
// BEFORE/AFTER only qualifies an UNTIL loop, so TestAfter without an Until is an
// inconsistent state that would silently drop the phrase and is rejected.
func performLoopText(stmt *PerformStatement) (string, bool) {
	if stmt.TestAfter && stmt.Until == nil {
		return "", false
	}
	switch {
	case stmt.Times != nil:
		t, ok := valueText(stmt.Times)
		if !ok {
			return "", false
		}
		return " " + t + " TIMES", true
	case stmt.Varying != nil:
		v := stmt.Varying
		name, ok := identifierText(v.Name)
		if !ok {
			return "", false
		}
		from, ok := valueText(v.From)
		if !ok {
			return "", false
		}
		by, ok := valueText(v.By)
		if !ok {
			return "", false
		}
		cond, ok := conditionText(v.Until)
		if !ok {
			return "", false
		}
		return " VARYING " + name + " FROM " + from + " BY " + by + " UNTIL " + cond, true
	case stmt.Until != nil:
		cond, ok := conditionText(stmt.Until)
		if !ok {
			return "", false
		}
		s := " "
		if stmt.TestAfter {
			s += "WITH TEST AFTER "
		}
		return s + "UNTIL " + cond, true
	default:
		return "", true
	}
}

// printBranchStatementAt prints the branch statement at index i on its own line
// (preceded by a newline so it sits under the IF/PERFORM header), then continues
// with the next; once i is past the last statement it continues with next. The
// statements are indented at depth — one level deeper than their enclosing header.
func printBranchStatementAt(stmts []Statement, i int, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if i >= len(stmts) {
			return next
		}
		pr.write("\n")
		return printStatement(stmts[i], depth, printBranchStatementAt(stmts, i+1, depth, next))
	}
}

// conditionText returns the canonical source text of a condition and whether it
// could be rendered: relational operators in symbol form, class/sign conditions as
// "operand IS [NOT] keyword", AND/OR/NOT combinators, and preserved parentheses.
func conditionText(c Condition) (string, bool) {
	switch n := c.(type) {
	case *RelationCondition:
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
	case *ClassCondition:
		if n == nil {
			return "", false
		}
		o, ok := exprText(n.Operand)
		if !ok {
			return "", false
		}
		return o + " IS " + negate(n.Not) + n.Class, true
	case *SignCondition:
		if n == nil {
			return "", false
		}
		o, ok := exprText(n.Operand)
		if !ok {
			return "", false
		}
		return o + " IS " + negate(n.Not) + n.Sign, true
	case *ConditionNameCondition:
		if n == nil {
			return "", false
		}
		return identifierText(n.Name)
	case *LogicalCondition:
		if n == nil {
			return "", false
		}
		l, ok := conditionText(n.Left)
		if !ok {
			return "", false
		}
		r, ok := conditionText(n.Right)
		if !ok {
			return "", false
		}
		return l + " " + n.Op + " " + r, true
	case *NotCondition:
		if n == nil {
			return "", false
		}
		inner, ok := conditionText(n.Cond)
		if !ok {
			return "", false
		}
		return "NOT " + inner, true
	case *ParenCondition:
		if n == nil {
			return "", false
		}
		inner, ok := conditionText(n.Cond)
		if !ok {
			return "", false
		}
		return "(" + inner + ")", true
	default:
		return "", false
	}
}

// negate returns the "NOT " prefix when not is set, for class/sign conditions.
func negate(not bool) string {
	if not {
		return "NOT "
	}
	return ""
}

// printGoToStatement prints a GO TO statement: the procedure-names and an optional
// DEPENDING ON selector. A typed-nil statement, one with no targets, or an
// unprintable selector is rejected with an [UnsupportedNodeError].
func printGoToStatement(stmt *GoToStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil || len(stmt.Targets) == 0 {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write(indent(depth) + "GO TO")
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
func printContinueStatement(stmt *ContinueStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		pr.write(indent(depth) + "CONTINUE")
		return next
	}
}

// printStopStatement prints a STOP statement (the indented verb; the sentence
// emits the terminating period): STOP RUN or STOP <literal>. Exactly one of Run or
// Literal must be present; a typed-nil statement, neither set, or both set
// (RUN would silently drop the literal) is rejected with an [UnsupportedNodeError].
func printStopStatement(stmt *StopStatement, depth int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if stmt == nil || stmt.Run == (stmt.Literal != nil) {
			return failPrint(UnsupportedNodeError{Node: stmt})
		}
		if stmt.Run {
			pr.write(indent(depth) + "STOP RUN")
			return next
		}
		text, ok := valueText(stmt.Literal)
		if !ok {
			return failPrint(UnsupportedNodeError{Node: stmt.Literal})
		}
		pr.write(indent(depth) + "STOP " + text)
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
