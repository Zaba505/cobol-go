// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

import (
	cobol "github.com/Zaba505/cobol-go"
	"github.com/Zaba505/cobol-go/picture"
)

// Kind classifies a [Field] as a group item or an elementary item.
//
// The classification is structural, exactly as COBOL defines it: a group item is
// one that has subordinate items, and an elementary item is one that does not.
// A group item therefore never carries a PICTURE, and an elementary item usually
// does — usually rather than always, because USAGE INDEX, USAGE POINTER,
// COMP-1 and COMP-2 items are elementary and take no PICTURE clause at all.
type Kind int

const (
	// KindElementary is an item with no subordinate items: a field that holds
	// a value. It is the zero value because most fields are elementary.
	KindElementary Kind = iota
	// KindGroup is an item with subordinate items: a field made of other
	// fields, holding no value of its own.
	KindGroup
)

// String implements the [fmt.Stringer] interface.
func (k Kind) String() string {
	if k == KindGroup {
		return "group"
	}
	return "elementary"
}

// Usage is the USAGE of a data item: how its value is represented in storage.
//
// One value corresponds to each usage-type the grammar admits, spelling and all:
// COMP and BINARY stay distinct here even where a dialect makes them synonyms,
// because which representation a given spelling selects is a property of the
// dialect and belongs to the codec package rather than to the record tree.
//
// The zero value is [UsageDisplay], which is COBOL's default for an item with no
// USAGE clause anywhere above it.
type Usage int

const (
	// UsageDisplay is USAGE DISPLAY, the default: one character position per
	// digit or character.
	UsageDisplay Usage = iota
	// UsageBinary is USAGE BINARY.
	UsageBinary
	// UsagePackedDecimal is USAGE PACKED-DECIMAL.
	UsagePackedDecimal
	// UsageComp is USAGE COMP / COMPUTATIONAL.
	UsageComp
	// UsageComp1 is USAGE COMP-1: single-precision floating point.
	UsageComp1
	// UsageComp2 is USAGE COMP-2: double-precision floating point.
	UsageComp2
	// UsageComp3 is USAGE COMP-3.
	UsageComp3
	// UsageComp4 is USAGE COMP-4.
	UsageComp4
	// UsageComp5 is USAGE COMP-5.
	UsageComp5
	// UsageIndex is USAGE INDEX.
	UsageIndex
	// UsagePointer is USAGE POINTER.
	UsagePointer
	// UsageComp6 is USAGE COMP-6, a GnuCOBOL and Micro Focus extension:
	// packed decimal with no sign nibble at all, and so always unsigned.
	//
	// It is declared last rather than beside the other COMP-n members
	// because these constants are iota-based: appending leaves every
	// existing member's value alone, where inserting it after UsageComp5
	// would renumber UsageIndex and UsagePointer under anything that has
	// persisted the int.
	UsageComp6
)

// usageNames maps each [Usage] to the canonical COBOL spelling the parser
// produces for it, which is also what [Usage.String] returns.
var usageNames = map[Usage]string{
	UsageDisplay:       "DISPLAY",
	UsageBinary:        "BINARY",
	UsagePackedDecimal: "PACKED-DECIMAL",
	UsageComp:          "COMP",
	UsageComp1:         "COMP-1",
	UsageComp2:         "COMP-2",
	UsageComp3:         "COMP-3",
	UsageComp4:         "COMP-4",
	UsageComp5:         "COMP-5",
	UsageComp6:         "COMP-6",
	UsageIndex:         "INDEX",
	UsagePointer:       "POINTER",
}

// String implements the [fmt.Stringer] interface, returning the canonical COBOL
// spelling of the usage-type.
func (u Usage) String() string {
	if name, ok := usageNames[u]; ok {
		return name
	}
	return "DISPLAY"
}

// usageFromString maps the canonical usage-type of a [cobol.UsageClause] onto a
// [Usage]. It reports false for a usage-type this package does not know, which
// the parser's grammar does not admit.
func usageFromString(s string) (Usage, bool) {
	for usage, name := range usageNames {
		if name == s {
			return usage, true
		}
	}
	return UsageDisplay, false
}

// Field is one storage-occupying item of a record: a node in the tree the entry
// level numbers imply.
//
// Level is the source level number — 01–49 for an item in a record hierarchy, or
// 77 for a standalone elementary item. Level-88 condition-names and level-66
// RENAMES entries occupy no storage and are not fields; they hang off the field
// or the record they qualify, as [Field.Conditions] and [Field.Aliases].
//
// Level is the number the entry was *written* with and not a depth: COBOL reads
// level numbers relatively, so the children of one group may carry different
// numbers and two fields with the same number may sit at different depths. Read
// the hierarchy from [Field.Parent] and [Field.Children]; comparing Level to
// infer it is wrong on any copybook using the IBM extension (root SPEC.md,
// Semantics: "Level numbers are relative").
//
// Name is the data-name with its source case preserved, and is empty for a
// FILLER item — see [Field.Filler]. Names are matched case-insensitively, as
// COBOL matches them: the fold is in the comparison and never in what is stored,
// so a printer re-emits the spelling the source used, and a caller wanting an
// exact match compares this field itself. Picture is the parsed PICTURE
// character-string, nil for a group item and for the elementary items that take
// no PICTURE clause (USAGE INDEX, USAGE POINTER, COMP-1, COMP-2). Usage is the
// item's own USAGE clause where it has one and the USAGE inherited from its
// nearest ancestor otherwise.
//
// Entry is the [cobol.DataDescriptionEntry] the field was built from, so the
// clauses this package does not interpret — OCCURS, REDEFINES, SYNCHRONIZED,
// JUSTIFIED, BLANK WHEN ZERO, VALUE on an ordinary item — stay reachable.
type Field struct {
	// Pos is the position of the entry's level-number.
	Pos cobol.Pos
	// Level is the source level number: 01–49, or 77 for a standalone item.
	// Siblings may carry different numbers; it is not a depth.
	Level int
	// Name is the data-name, empty for a FILLER item.
	Name string
	// Filler reports an unnamed item: one written FILLER, and one whose
	// data-name was simply omitted, which COBOL treats the same way.
	Filler bool
	// Kind reports whether the field is a group or an elementary item.
	Kind Kind
	// Picture is the parsed PICTURE character-string, nil when the entry has
	// no PICTURE clause.
	Picture *picture.Picture
	// Usage is the item's USAGE, inherited from the nearest ancestor that
	// states one when the item does not state its own.
	Usage Usage
	// Parent is the group item this field is subordinate to, nil for a
	// top-level record and for a level-77 item.
	Parent *Field
	// Children are the subordinate fields in source order, empty for an
	// elementary item.
	Children []*Field
	// Conditions are the level-88 condition-names declared on this field, in
	// source order.
	Conditions []*Condition
	// Aliases are the level-66 RENAMES entries declared over this record's
	// fields, in source order. Only a top-level record carries them, since
	// RENAMES regroups items of the record it belongs to.
	Aliases []*Alias
	// Entry is the data description entry the field was built from.
	Entry *cobol.DataDescriptionEntry
}

// Condition is a level-88 condition-name: a named set of values of the field it
// is declared on.
//
// A condition-name occupies no storage — it is a test against its parent field's
// value, not a field — so it is held here rather than among [Field.Children].
// Values is the entry's VALUE list, one [cobol.ValueSpec] per value or
// THROUGH range, in source order.
type Condition struct {
	// Pos is the position of the entry's level-number.
	Pos cobol.Pos
	// Name is the condition-name.
	Name string
	// Values are the values and value ranges the condition tests for.
	Values []cobol.ValueSpec
	// Parent is the field the condition-name is declared on.
	Parent *Field
	// Entry is the data description entry the condition was built from.
	Entry *cobol.DataDescriptionEntry
}

// Alias is a level-66 RENAMES entry: a name for a range of a record's fields.
//
// An alias occupies no storage of its own — it is another way to address storage
// the fields it renames already occupy — so it is held on the record rather than
// among its children. From and Through are the resolved endpoints of the range,
// with Through nil when the entry renames a single item. Fields is the run the
// alias covers.
//
// COBOL admits a level-88 condition-name on a RENAMES entry. An alias is not a
// [Field] and carries no Conditions, so [Build] reports such an entry as a
// [LevelSequenceError] rather than attaching the condition-name to the field the
// alias happens to follow.
type Alias struct {
	// Pos is the position of the entry's level-number.
	Pos cobol.Pos
	// Name is the name the RENAMES entry declares.
	Name string
	// From is the first field of the renamed range.
	From *Field
	// Through is the last field of the renamed range, nil when the entry
	// renames a single item.
	Through *Field
	// Fields is the run of fields the alias covers: every field of the
	// record, in source order, from From through Through inclusive,
	// including the fields subordinate to any group in the run — a range
	// ends at the end of the storage its last endpoint occupies, so a group
	// endpoint brings its subordinate items in with it. It holds From and
	// its subordinate items when Through is nil.
	Fields []*Field
	// Record is the top-level record whose fields the alias regroups.
	Record *Field
	// Entry is the data description entry the alias was built from.
	Entry *cobol.DataDescriptionEntry
}
