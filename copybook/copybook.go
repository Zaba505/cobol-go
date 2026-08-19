// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

import (
	"fmt"
	"slices"

	cobol "github.com/Zaba505/cobol-go"
	"github.com/Zaba505/cobol-go/picture"
)

// Level numbers COBOL gives a meaning of their own, as opposed to the 01–49
// levels that build the record hierarchy.
const (
	levelRecord  = 1  // the outermost level of a record
	levelMaxItem = 49 // the innermost level a record hierarchy may use
	levelRenames = 66 // a RENAMES entry: a name for a range of fields
	levelStand   = 77 // a standalone elementary item
	levelCond    = 88 // a condition-name
)

// Option configures [Build].
type Option func(*config)

// config holds the resolved [Option] values; its zero value selects the
// defaults.
type config struct {
	// pictureOpts are forwarded to [picture.Parse] for every PICTURE clause.
	pictureOpts []picture.ParseOption
}

// WithDecimalPointIsComma builds the tree as though the source unit declared
// DECIMAL-POINT IS COMMA in SPECIAL-NAMES: the roles of '.' and ',' are swapped
// inside the PICTURE character-strings, so PIC 9(3),99 is the one with two
// decimal places (root SPEC.md, Semantics).
//
// The clause is a property of the source unit rather than of any one entry, and
// a standalone copybook does not carry the ENVIRONMENT DIVISION that states it,
// so it is passed in here rather than read off the entries.
func WithDecimalPointIsComma() Option {
	return func(c *config) {
		c.pictureOpts = append(c.pictureOpts, picture.WithDecimalPointIsComma())
	}
}

// Build assembles a flat list of data description entries — [cobol.DataSection]'s
// Entries, [cobol.Fragment]'s Entries, or a [cobol.FileDescriptionEntry]'s
// Records — into the record tree their level numbers imply, returning the
// top-level items in source order.
//
// A top-level item is a level-01 record or a level-77 standalone elementary
// item. Everything else hangs off one of them: levels 02–49 as [Field.Children],
// level-88 condition-names as [Field.Conditions] on the item they qualify, and
// level-66 RENAMES entries as [Field.Aliases] on the record whose fields they
// regroup. Only fields occupy storage, which is why the other two are held apart
// from Children rather than mixed into it.
//
// A level between 02 and 49 is subordinate to the nearest preceding item with a
// lower level number, which is the rule COBOL compilers apply; sibling items are
// not required to share one level number. A REDEFINES clause does not move an
// entry: an entry the level numbers place *inside* the item it names is
// subordinate to that item and so can never be the sibling REDEFINES asks for,
// which Build reports rather than re-read as a sibling (root SPEC.md, Semantics:
// "A REDEFINES entry subordinate to its target"). An entry numbered above a
// target it is not inside — one naming a group that has already closed — is not
// this error and is left to [NewLayout], which reports that no item of that name
// precedes it.
//
// USAGE flows down from a group to the items subordinate to it unless one states
// its own (root SPEC.md, Semantics: "USAGE is inherited by subordinate items from
// a group unless overridden"), and an entry with no data-name is FILLER, exactly
// as one written FILLER is.
//
// It returns a [LevelError] for a level number COBOL does not admit, a
// [LevelSequenceError] for a level number in a position the sequence does not
// allow, a [RenamesError] for a level-66 entry whose range cannot be resolved,
// and a [PictureError] for a PICTURE character-string that does not parse.
func Build(entries []*cobol.DataDescriptionEntry, opts ...Option) ([]*Field, error) {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	b := &builder{cfg: cfg}
	for _, entry := range entries {
		if err := b.add(entry); err != nil {
			return nil, err
		}
	}
	return b.records, nil
}

// builder accumulates the record tree as entries arrive.
//
// open is the chain of group items the next 02–49 entry could be subordinate to,
// outermost first; it is emptied by every new top-level item. last is the most
// recent field, which is the item a level-88 condition-name qualifies. lastWas66
// records that the most recent entry was a RENAMES entry, so a condition-name
// following one is reported rather than silently attached to last.
type builder struct {
	cfg       config
	records   []*Field
	open      []*Field
	last      *Field
	lastWas66 bool
}

// add folds one data description entry into the tree.
func (b *builder) add(entry *cobol.DataDescriptionEntry) error {
	switch level := entry.Level; {
	case level == levelCond:
		return b.addCondition(entry)
	case level == levelRenames:
		return b.addAlias(entry)
	case level == levelStand:
		return b.addStandalone(entry)
	case level == levelRecord:
		return b.addRecord(entry)
	case level > levelRecord && level <= levelMaxItem:
		return b.addSubordinate(entry)
	default:
		return LevelError{Pos: entry.Pos, Level: level}
	}
}

// addRecord starts a new level-01 record. It ends every group left open by the
// previous record, since nothing subordinate can span two records.
func (b *builder) addRecord(entry *cobol.DataDescriptionEntry) error {
	field, err := b.newField(entry, nil)
	if err != nil {
		return err
	}
	b.records = append(b.records, field)
	b.open = []*Field{field}
	return nil
}

// addStandalone adds a level-77 standalone elementary item. It is a top-level
// item that takes no subordinates, so it closes every open group and opens none.
func (b *builder) addStandalone(entry *cobol.DataDescriptionEntry) error {
	field, err := b.newField(entry, nil)
	if err != nil {
		return err
	}
	b.records = append(b.records, field)
	b.open = nil
	return nil
}

// addSubordinate adds a level 02–49 item beneath the nearest preceding item with
// a lower level number.
//
// Level numbers are relative, so an entry numbered at or below the nearest open
// item closes items until it finds one numbered below it and becomes that one's
// child. That is what lets the siblings of a group carry unequal numbers — an
// IBM extension over the 85 Standard, and what a REDEFINES written with a level
// number differing from its target's relies on (root SPEC.md, Semantics:
// "Level numbers are relative").
//
// A REDEFINES clause does not change where the entry lands: the tree is built
// from level numbers alone, so an entry the numbers place inside the item it
// names is subordinate to that item rather than a redefinition of it, and is
// reported here rather than admitted (root SPEC.md, Semantics: "A REDEFINES
// entry subordinate to its target").
func (b *builder) addSubordinate(entry *cobol.DataDescriptionEntry) error {
	for len(b.open) > 0 && b.open[len(b.open)-1].Level >= entry.Level {
		b.open = b.open[:len(b.open)-1]
	}
	if len(b.open) == 0 {
		reason := "has no containing record"
		if b.last != nil && b.last.Level == levelStand {
			reason = "has no containing record; a level-77 item takes no subordinate items"
		}
		return LevelSequenceError{
			Pos:    entry.Pos,
			Level:  entry.Level,
			Name:   nameOf(entry),
			Reason: reason,
		}
	}

	parent := b.open[len(b.open)-1]
	if enclosing := b.redefinesEnclosing(entry, parent); enclosing != nil {
		return LevelSequenceError{
			Pos:   entry.Pos,
			Level: entry.Level,
			Name:  nameOf(entry),
			Reason: fmt.Sprintf("redefines %s, which its level number makes it subordinate to rather than a sibling of; "+
				"a REDEFINES entry must be an item of the same group as its target, so write it at its target's level number, %02d",
				describe(enclosing), enclosing.Level),
		}
	}
	if parent.Picture != nil {
		return LevelSequenceError{
			Pos:   entry.Pos,
			Level: entry.Level,
			Name:  nameOf(entry),
			Reason: fmt.Sprintf("is numbered above %s, which has a PICTURE and so is an elementary item and takes no subordinate items; "+
				"a level number greater than the nearest preceding item's makes the entry subordinate to that item",
				describe(parent)),
		}
	}

	field, err := b.newField(entry, parent)
	if err != nil {
		return err
	}
	parent.Kind = KindGroup
	parent.Children = append(parent.Children, field)
	b.open = append(b.open, field)
	return nil
}

// redefinesEnclosing reports the item an entry's REDEFINES clause names when
// that item is one the entry is being placed *inside* rather than beside,
// returning nil in every other case: no REDEFINES clause, a clause whose target
// is a preceding sibling, a clause naming something this record has not opened
// — a group already closed, or a name belonging to another record — and a clause
// carrying no data-name at all. The last of those the grammar does not admit, so
// it is left to [NewLayout] rather than reported here as though the level
// numbers were at fault.
//
// Both scans match the data-name case-insensitively; see [sameName].
//
// The sibling scan returns nil for a matching sibling whatever that sibling is,
// including one no REDEFINES may legally name. Deciding that is [NewLayout]'s,
// which has the sizes; all this function settles is whether the entry is beside
// its target or inside it.
//
// An entry may never redefine an item it is subordinate to: REDEFINES asks for
// an item of the same group as its target, and the open chain is exactly the
// items this entry is subordinate to. The chain is only consulted once no
// preceding sibling carries the name, because a data-name may repeat within a
// record — a record and a field of it may share one — and the sibling is what a
// correct copybook means (root SPEC.md, Semantics: "REDEFINES asks for the same
// level, not the same level number").
//
// This is the shape a production copybook reaches when it writes every record
// type of a file as a REDEFINES of a generic record and numbers one of them
// above its target. Reporting it here rather than leaving it to [NewLayout] is
// what lets the diagnostic name the level rule: by layout time the entry is a
// child of its own target, and all that is left to say is that the target is not
// among its own children.
func (b *builder) redefinesEnclosing(entry *cobol.DataDescriptionEntry, parent *Field) *Field {
	clause := redefinesOf(entry)
	if clause == nil || clause.Name == nil {
		return nil
	}
	name := clause.Name.Value

	for _, sibling := range parent.Children {
		if !sibling.Filler && sameName(sibling.Name, name) {
			return nil
		}
	}
	for i := len(b.open) - 1; i >= 0; i-- {
		if open := b.open[i]; !open.Filler && sameName(open.Name, name) {
			return open
		}
	}
	return nil
}

// addCondition attaches a level-88 condition-name to the item it qualifies: the
// most recent field, which is the item the entry follows in the source.
func (b *builder) addCondition(entry *cobol.DataDescriptionEntry) error {
	switch {
	case b.last == nil:
		return LevelSequenceError{
			Pos:    entry.Pos,
			Level:  entry.Level,
			Name:   nameOf(entry),
			Reason: "condition-name has no preceding item to qualify",
		}
	case b.lastWas66:
		return LevelSequenceError{
			Pos:    entry.Pos,
			Level:  entry.Level,
			Name:   nameOf(entry),
			Reason: "follows a level-66 RENAMES entry; a condition-name on an alias is not represented by this package",
		}
	case entry.Name == nil:
		return LevelSequenceError{
			Pos:    entry.Pos,
			Level:  entry.Level,
			Reason: "condition-name entry has no condition-name",
		}
	}

	cond := &Condition{
		Pos:    entry.Pos,
		Name:   entry.Name.Value,
		Values: valuesOf(entry),
		Parent: b.last,
		Entry:  entry,
	}
	b.last.Conditions = append(b.last.Conditions, cond)
	return nil
}

// addAlias resolves a level-66 RENAMES entry against the fields of the record it
// belongs to. A RENAMES entry follows the items it renames, so the record is
// complete by the time this runs.
func (b *builder) addAlias(entry *cobol.DataDescriptionEntry) error {
	if len(b.open) == 0 {
		return LevelSequenceError{
			Pos:    entry.Pos,
			Level:  entry.Level,
			Name:   nameOf(entry),
			Reason: "RENAMES entry has no containing record",
		}
	}
	if entry.Name == nil {
		return LevelSequenceError{
			Pos:    entry.Pos,
			Level:  entry.Level,
			Reason: "RENAMES entry has no data-name",
		}
	}

	record := b.open[0]
	clause := renamesOf(entry)
	if clause == nil {
		return RenamesError{
			Pos:    entry.Pos,
			Name:   entry.Name.Value,
			Reason: "entry has no RENAMES clause",
		}
	}

	fields := flatten(record)
	from := indexOf(fields, clause.From.Value)
	if from < 0 {
		return RenamesError{
			Pos:    entry.Pos,
			Name:   entry.Name.Value,
			Target: clause.From.Value,
			Reason: fmt.Sprintf("no field named %q in %s", clause.From.Value, describe(record)),
		}
	}

	through := from
	if clause.Through != nil {
		through = indexOf(fields, clause.Through.Value)
		switch {
		case through < 0:
			return RenamesError{
				Pos:    entry.Pos,
				Name:   entry.Name.Value,
				Target: clause.Through.Value,
				Reason: fmt.Sprintf("no field named %q in %s", clause.Through.Value, describe(record)),
			}
		case through <= from:
			return RenamesError{
				Pos:    entry.Pos,
				Name:   entry.Name.Value,
				Target: clause.Through.Value,
				Reason: fmt.Sprintf("%q does not follow %q", clause.Through.Value, clause.From.Value),
			}
		}
	}

	// A range ends at the end of the storage its last endpoint occupies, so a
	// group endpoint carries the items subordinate to it into the range. They
	// are the entries that follow it in this pre-order walk.
	end := through + descendants(fields[through])

	alias := &Alias{
		Pos:    entry.Pos,
		Name:   entry.Name.Value,
		From:   fields[from],
		Fields: slices.Clone(fields[from : end+1]),
		Record: record,
		Entry:  entry,
	}
	if clause.Through != nil {
		alias.Through = fields[through]
	}
	record.Aliases = append(record.Aliases, alias)
	b.lastWas66 = true
	return nil
}

// newField builds one storage-occupying field, resolving its PICTURE and the
// USAGE it inherits from parent. It records the field as the most recent one, so
// a level-88 entry following it knows what it qualifies.
func (b *builder) newField(entry *cobol.DataDescriptionEntry, parent *Field) (*Field, error) {
	field := &Field{
		Pos:    entry.Pos,
		Level:  entry.Level,
		Filler: entry.Filler || entry.Name == nil,
		Kind:   KindElementary,
		Parent: parent,
		Entry:  entry,
	}
	if entry.Name != nil {
		field.Name = entry.Name.Value
	}

	if clause := pictureOf(entry); clause != nil {
		pic, err := picture.Parse(clause.Picture, b.cfg.pictureOpts...)
		if err != nil {
			return nil, PictureError{
				Pos:    clause.Pos,
				Name:   field.Name,
				Source: clause.Picture,
				Err:    err,
			}
		}
		field.Picture = pic
	}

	// USAGE is the item's own when it states one, and the nearest ancestor's
	// otherwise; with no ancestor stating one it is DISPLAY, the zero value.
	if parent != nil {
		field.Usage = parent.Usage
	}
	if clause := usageOf(entry); clause != nil {
		usage, ok := usageFromString(clause.Usage)
		if !ok {
			return nil, UsageError{Pos: clause.Pos, Name: field.Name, Usage: clause.Usage}
		}
		field.Usage = usage
	}

	b.last = field
	b.lastWas66 = false
	return field, nil
}

// flatten returns the record's fields in source order, outermost first: the
// pre-order walk of its subordinate items, excluding the record itself. It is
// the order a RENAMES range is measured in.
func flatten(record *Field) []*Field {
	var fields []*Field
	var walk func(*Field)
	walk = func(f *Field) {
		for _, child := range f.Children {
			fields = append(fields, child)
			walk(child)
		}
	}
	walk(record)
	return fields
}

// descendants counts the fields subordinate to f, at any depth: the length of
// the run f's subtree occupies in the [flatten] order after f itself.
func descendants(f *Field) int {
	n := len(f.Children)
	for _, child := range f.Children {
		n += descendants(child)
	}
	return n
}

// indexOf reports the position of the first field named name, or -1. The match
// is case-insensitive; see [sameName]. A data-name may repeat within a record
// when every reference to it is qualified; an unqualified RENAMES of such a name
// is not valid COBOL, so taking the first match costs nothing a correct copybook
// relies on.
func indexOf(fields []*Field, name string) int {
	for i, f := range fields {
		if !f.Filler && sameName(f.Name, name) {
			return i
		}
	}
	return -1
}

// pictureOf returns the entry's PICTURE clause, or nil.
func pictureOf(entry *cobol.DataDescriptionEntry) *cobol.PictureClause {
	for _, clause := range entry.Clauses {
		if pic, ok := clause.(*cobol.PictureClause); ok {
			return pic
		}
	}
	return nil
}

// usageOf returns the entry's USAGE clause, or nil.
func usageOf(entry *cobol.DataDescriptionEntry) *cobol.UsageClause {
	for _, clause := range entry.Clauses {
		if usage, ok := clause.(*cobol.UsageClause); ok {
			return usage
		}
	}
	return nil
}

// renamesOf returns the entry's RENAMES clause, or nil.
func renamesOf(entry *cobol.DataDescriptionEntry) *cobol.RenamesClause {
	for _, clause := range entry.Clauses {
		if renames, ok := clause.(*cobol.RenamesClause); ok {
			return renames
		}
	}
	return nil
}

// valuesOf returns the values of the entry's VALUE clause, or nil.
func valuesOf(entry *cobol.DataDescriptionEntry) []cobol.ValueSpec {
	for _, clause := range entry.Clauses {
		if value, ok := clause.(*cobol.ValueClause); ok {
			return value.Values
		}
	}
	return nil
}

// nameOf returns the entry's data-name, empty for a FILLER or unnamed entry.
func nameOf(entry *cobol.DataDescriptionEntry) string {
	if entry.Name == nil {
		return ""
	}
	return entry.Name.Value
}

// describe names a field for an error message, falling back to its level and
// position for a FILLER item, which has no name to give.
func describe(f *Field) string {
	if f.Name != "" {
		return fmt.Sprintf("%q", f.Name)
	}
	return fmt.Sprintf("the level-%02d item at line %d, column %d", f.Level, f.Pos.Line, f.Pos.Column)
}

// LevelError is returned for a level number COBOL does not admit. The levels it
// does are 01–49 for a record hierarchy, 66 for a RENAMES entry, 77 for a
// standalone elementary item, and 88 for a condition-name.
type LevelError struct {
	Pos   cobol.Pos
	Level int
}

// Error implements the [error] interface.
func (e LevelError) Error() string {
	return fmt.Sprintf("invalid level number %d at line %d, column %d: valid levels are 01-49, 66, 77, and 88",
		e.Level, e.Pos.Line, e.Pos.Column)
}

// LevelSequenceError is returned for a level number COBOL admits appearing where
// the sequence of entries does not allow it: a subordinate item with no
// containing group, a condition-name with nothing to qualify, a RENAMES entry
// outside a record.
type LevelSequenceError struct {
	Pos    cobol.Pos
	Level  int
	Name   string
	Reason string
}

// Error implements the [error] interface.
func (e LevelSequenceError) Error() string {
	if e.Name != "" {
		return fmt.Sprintf("level-%02d item %q at line %d, column %d %s",
			e.Level, e.Name, e.Pos.Line, e.Pos.Column, e.Reason)
	}
	return fmt.Sprintf("level-%02d item at line %d, column %d %s",
		e.Level, e.Pos.Line, e.Pos.Column, e.Reason)
}

// RenamesError is returned for a level-66 entry whose range cannot be resolved:
// an endpoint naming no field of the record, or a range whose end does not
// follow its start. Name is the name the entry declares and Target the endpoint
// at fault, empty when the fault is the entry itself.
type RenamesError struct {
	Pos    cobol.Pos
	Name   string
	Target string
	Reason string
}

// Error implements the [error] interface.
func (e RenamesError) Error() string {
	return fmt.Sprintf("level-66 entry %q at line %d, column %d: %s",
		e.Name, e.Pos.Line, e.Pos.Column, e.Reason)
}

// PictureError is returned when an item's PICTURE character-string does not
// parse. Err is the error [picture.Parse] returned and is what Unwrap yields.
type PictureError struct {
	Pos    cobol.Pos
	Name   string
	Source string
	Err    error
}

// Error implements the [error] interface.
func (e PictureError) Error() string {
	return fmt.Sprintf("item %s at line %d, column %d: %s",
		quoteOrItem(e.Name), e.Pos.Line, e.Pos.Column, e.Err)
}

// Unwrap implements the convention [errors.Unwrap] follows, exposing the
// [picture.Parse] error underneath.
func (e PictureError) Unwrap() error { return e.Err }

// UsageError is returned for a USAGE clause naming a usage-type this package
// does not know. The parser's grammar admits no such clause, so it reports a
// usage-type added to the parser and not to this package.
type UsageError struct {
	Pos   cobol.Pos
	Name  string
	Usage string
}

// Error implements the [error] interface.
func (e UsageError) Error() string {
	return fmt.Sprintf("item %s at line %d, column %d: unknown USAGE %q",
		quoteOrItem(e.Name), e.Pos.Line, e.Pos.Column, e.Usage)
}

// quoteOrItem quotes a data-name, naming a FILLER item generically instead.
func quoteOrItem(name string) string {
	if name == "" {
		return "FILLER"
	}
	return fmt.Sprintf("%q", name)
}
