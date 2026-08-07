// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

import (
	"errors"
	"fmt"
	"strconv"

	cobol "github.com/Zaba505/cobol-go"
	"github.com/Zaba505/cobol-go/picture"
)

// BinarySize is the width staircase a compiler applies to USAGE BINARY items —
// BINARY, COMP, COMPUTATIONAL, COMP-4 and COMP-5.
//
// A binary item's width is a staircase in its digit count and never the digit
// count itself: PIC 9(5) COMP is four bytes, not five (codec/SPEC.md, "Binary
// widths by digit count"). Which staircase is a property of the compiler that
// produced the file and is baked into the record layout, so it is declared here
// rather than inferred.
//
// The zero value [BinarySizeUnset] is invalid, because the 1–2 digit row is a
// real fork between compilers: PIC S9(2) COMP is two bytes under IBM and one
// under GnuCOBOL's default. Guessing it shifts every following field.
type BinarySize int

const (
	// BinarySizeUnset is the invalid zero value.
	BinarySizeUnset BinarySize = iota
	// BinarySize248 is 2/4/8/16 bytes by digit count: IBM Enterprise COBOL,
	// Micro Focus, and GnuCOBOL's binary-size: 2-4-8.
	BinarySize248
	// BinarySize1248 is 1/2/4/8/16 bytes by digit count: GnuCOBOL's default
	// binary-size: 1-2-4-8, which gives a 1–2 digit item one byte.
	BinarySize1248
	// BinarySizeSmallest is GnuCOBOL's binary-size: 1--8, the smallest byte
	// count from 1 to 8 whose signed range holds the digits.
	BinarySizeSmallest
	// BinarySizeFull is GnuCOBOL's binary-size: full, always eight bytes
	// (sixteen beyond eighteen digits).
	BinarySizeFull
)

// binarySizeNames maps each [BinarySize] to the spelling GnuCOBOL's binary-size
// runtime option uses for it, which is also what [BinarySize.String] returns.
var binarySizeNames = map[BinarySize]string{
	BinarySize248:      "2-4-8",
	BinarySize1248:     "1-2-4-8",
	BinarySizeSmallest: "1--8",
	BinarySizeFull:     "full",
}

// String implements the [fmt.Stringer] interface.
func (b BinarySize) String() string {
	if name, ok := binarySizeNames[b]; ok {
		return name
	}
	return "unset"
}

// valid reports whether the value names a member.
func (b BinarySize) valid() bool {
	_, ok := binarySizeNames[b]
	return ok
}

// smallestBinaryDigits[i] is the largest digit count a signed integer of i+1
// bytes holds: 2^(8n-1)-1 written as a number of decimal digits. It is the whole
// of [BinarySizeSmallest].
var smallestBinaryDigits = [8]int{2, 4, 6, 9, 11, 14, 16, 18}

// width reports the byte width of a binary item of the given digit count.
func (b BinarySize) width(digits int) int {
	switch b {
	case BinarySize1248:
		switch {
		case digits <= 2:
			return 1
		case digits <= 4:
			return 2
		case digits <= 9:
			return 4
		case digits <= 18:
			return 8
		}
	case BinarySizeSmallest:
		for i, max := range smallestBinaryDigits {
			if digits <= max {
				return i + 1
			}
		}
	case BinarySizeFull:
		if digits <= 18 {
			return 8
		}
	default:
		switch {
		case digits <= 4:
			return 2
		case digits <= 9:
			return 4
		case digits <= 18:
			return 8
		}
	}
	// Nineteen digits and beyond is a sixteen-byte item under every
	// staircase; IBM reaches it only under ARITH(EXTEND).
	return 16
}

// SyncMode says whether the SYNCHRONIZED clause inserts slack bytes.
//
// SYNCHRONIZED asks that an item start on its natural boundary, and the slack
// bytes that buys are part of the record: they change every following offset.
// Whether a compiler honours the clause or merely syntax-checks it is a property
// of the compiler, so it is declared rather than assumed — a reader MUST NOT
// assume a record is the simple sum of its fields' widths when SYNCHRONIZED is
// present anywhere (codec/SPEC.md, "Group items, OCCURS, and SYNCHRONIZED").
//
// The zero value [SyncUnset] is invalid.
type SyncMode int

const (
	// SyncUnset is the invalid zero value.
	SyncUnset SyncMode = iota
	// SyncIgnored accepts the clause and lays the record out as though it
	// were absent, which is what GnuCOBOL does.
	SyncIgnored
	// SyncAligned inserts slack bytes before a SYNCHRONIZED item to align it
	// to its natural boundary, and trailing slack in a group under OCCURS so
	// that every occurrence is laid out identically.
	SyncAligned
)

// String implements the [fmt.Stringer] interface.
func (s SyncMode) String() string {
	switch s {
	case SyncIgnored:
		return "ignored"
	case SyncAligned:
		return "aligned"
	}
	return "unset"
}

// valid reports whether the value names a member.
func (s SyncMode) valid() bool { return s == SyncIgnored || s == SyncAligned }

// RedefinesRule says what a compiler does with a subordinate REDEFINES item
// larger than the item it redefines.
//
// The zero value [RedefinesUnset] is invalid.
type RedefinesRule int

const (
	// RedefinesUnset is the invalid zero value.
	RedefinesUnset RedefinesRule = iota
	// RedefinesStrict rejects a subordinate redefining item larger than the
	// item it redefines, which is the standard's rule and IBM Enterprise
	// COBOL's. It is a rule about levels 02–49 only; a level-01 record may
	// redefine a shorter one, and a record's own REDEFINES clause is not a
	// constraint on its layout.
	RedefinesStrict
	// RedefinesLenient accepts a larger redefining item and lets it extend
	// the group that holds it, which is what a compiler configured to
	// downgrade the diagnostic to a warning does.
	RedefinesLenient
)

// String implements the [fmt.Stringer] interface.
func (r RedefinesRule) String() string {
	switch r {
	case RedefinesStrict:
		return "strict"
	case RedefinesLenient:
		return "lenient"
	}
	return "unset"
}

// valid reports whether the value names a member.
func (r RedefinesRule) valid() bool { return r == RedefinesStrict || r == RedefinesLenient }

// Dialect is the compiler-side half of a record's layout: the settings that fix
// where each field starts and how many bytes it occupies.
//
// codec/SPEC.md, "By compiler vs by file", splits the settings a copybook reader
// needs in two. Charset, zoned sign convention, binary byte order and float
// format are properties of the *file in hand* and live in
// [github.com/Zaba505/cobol-go/codec.Encoding]. The settings here are the other
// half: properties of the *compiler*, fixed at compile time and baked into the
// record layout. The two are declared apart because they are answered by
// different facts and a caller routinely knows one without the other.
//
// Every field has an invalid zero value and [Dialect.Validate] reports the one
// that was left out, for the same reason
// [github.com/Zaba505/cobol-go/codec.Encoding] does: a wrong layout
// setting is not visible in the result. It shifts every following field, which
// surfaces — if it surfaces at all — as a length mismatch or a validation
// failure several fields later, never as "the dialect was wrong".
//
// [IBMEnterprise], [GnuCOBOL] and [MicroFocus] are complete bundles a caller may
// pass; none of them is a fallback this package applies on its own.
type Dialect struct {
	// Binary is the width staircase for BINARY, COMP, COMP-4 and COMP-5
	// items.
	Binary BinarySize
	// Sync says whether SYNCHRONIZED inserts slack bytes.
	Sync SyncMode
	// Redefines says what a subordinate REDEFINES item larger than what it
	// redefines is.
	Redefines RedefinesRule
	// IndexWidth is the byte width of a USAGE INDEX item. It is four on IBM
	// and platform-dependent elsewhere (codec/SPEC.md, "Storage Widths").
	IndexWidth int
	// PointerWidth is the byte width of a USAGE POINTER item: four under
	// AMODE 31 and eight under AMODE 64, and the platform pointer width
	// elsewhere.
	PointerWidth int
}

// maxItemWidth bounds [Dialect.IndexWidth] and [Dialect.PointerWidth]. Nothing
// COBOL calls an index or a pointer is wider than sixteen bytes, and the bound
// is what turns a transposed field into an error rather than into a record
// length nobody questions.
const maxItemWidth = 16

// Validate reports the first field of the dialect that was left undeclared or
// set to a value naming no member, as a [DialectError].
func (d Dialect) Validate() error {
	switch {
	case !d.Binary.valid():
		return DialectError{Field: "Binary", Reason: "is not a declared BinarySize"}
	case !d.Sync.valid():
		return DialectError{Field: "Sync", Reason: "is not a declared SyncMode"}
	case !d.Redefines.valid():
		return DialectError{Field: "Redefines", Reason: "is not a declared RedefinesRule"}
	case d.IndexWidth < 1 || d.IndexWidth > maxItemWidth:
		return DialectError{
			Field:  "IndexWidth",
			Reason: fmt.Sprintf("is %d: must be between 1 and %d bytes", d.IndexWidth, maxItemWidth),
		}
	case d.PointerWidth < 1 || d.PointerWidth > maxItemWidth:
		return DialectError{
			Field:  "PointerWidth",
			Reason: fmt.Sprintf("is %d: must be between 1 and %d bytes", d.PointerWidth, maxItemWidth),
		}
	}
	return nil
}

// IBMEnterprise is the layout IBM Enterprise COBOL produces: binary widths of
// 2/4/8, SYNCHRONIZED honoured, a subordinate REDEFINES item held to the size of
// what it redefines, and four-byte indexes and pointers.
//
// The pointer width is AMODE 31's. Under AMODE 64 it is eight; set
// [Dialect.PointerWidth] to say so.
func IBMEnterprise() Dialect {
	return Dialect{
		Binary:       BinarySize248,
		Sync:         SyncAligned,
		Redefines:    RedefinesStrict,
		IndexWidth:   4,
		PointerWidth: 4,
	}
}

// GnuCOBOL is the layout GnuCOBOL produces with its defaults: binary-size
// 1-2-4-8, which gives a 1–2 digit binary item one byte, and SYNCHRONIZED
// accepted but ignored.
//
// The pointer width is a 64-bit build's. Both it and [Dialect.IndexWidth] are
// platform-dependent; set them where the target is not a 64-bit build with
// four-byte indexes.
func GnuCOBOL() Dialect {
	return Dialect{
		Binary:       BinarySize1248,
		Sync:         SyncIgnored,
		Redefines:    RedefinesLenient,
		IndexWidth:   4,
		PointerWidth: 8,
	}
}

// MicroFocus is the layout Micro Focus COBOL produces under its IBM-compatible
// directives: binary widths of 2/4/8 and SYNCHRONIZED honoured.
//
// Both of those are directive-dependent under Micro Focus rather than fixed
// (codec/SPEC.md, "Dialect Matrix"), so this bundle is the mainframe-compatible
// reading and not the only one; override the field a directive changes.
func MicroFocus() Dialect {
	return Dialect{
		Binary:       BinarySize248,
		Sync:         SyncAligned,
		Redefines:    RedefinesStrict,
		IndexWidth:   4,
		PointerWidth: 8,
	}
}

// Item is one field's place in a record's storage: where it starts, how many
// bytes one occurrence of it takes, and how far apart its occurrences are.
//
// Offset is measured from the first byte of the record the [Layout] was computed
// for, and is the offset of the field's *first* occurrence. Length is the bytes
// one occurrence occupies; for a group it is the extent of its subordinate
// items, including any slack between them. Stride is the distance from one
// occurrence to the next — Length plus the trailing slack that brings the next
// occurrence back onto its boundary — so the whole field occupies
// Stride × Occurs bytes, which is what [Item.Total] returns. A field that occurs
// once has no next occurrence and so no trailing slack: its Stride is its
// Length, whatever it is aligned to.
//
// Slack is the number of bytes skipped immediately before Offset to bring the
// item onto its boundary. It is part of the record and belongs to no field, so
// it is reported here rather than being folded into the preceding item's Length.
//
// Redefines is the item this one redefines, non-nil only for a field carrying a
// REDEFINES clause resolved against a preceding sibling. A redefining item
// shares its target's Offset and adds nothing to the group's extent unless it is
// longer and the dialect allows that.
type Item struct {
	// Field is the field this item lays out.
	Field *Field
	// Offset is the byte offset of the item's first occurrence, from the
	// start of the record.
	Offset int
	// Length is the bytes one occurrence of the item occupies.
	Length int
	// Stride is the bytes from the start of one occurrence to the start of
	// the next; it equals Length when no trailing slack is inserted.
	Stride int
	// Occurs is the occurrence count: 1 for a field with no OCCURS clause.
	Occurs int
	// Slack is the number of slack bytes inserted immediately before Offset.
	Slack int
	// Redefines is the item this one redefines, nil when it redefines
	// nothing.
	Redefines *Item
	// Parent is the item of the group this field is subordinate to, nil for
	// the record itself.
	Parent *Item
	// Children are the items of the subordinate fields, in source order.
	Children []*Item
}

// Total reports the bytes the item occupies in the record, every occurrence and
// its trailing slack included.
func (i *Item) Total() int { return i.Stride * i.Occurs }

// End reports the offset one byte past the item's storage: Offset plus
// [Item.Total].
func (i *Item) End() int { return i.Offset + i.Total() }

// OccurrenceOffset reports the byte offset of occurrence n, counted from zero.
// It returns an [OccurrenceError] for an n outside the item's occurrence count,
// so that a subscript one past the end is an error rather than an offset inside
// whatever follows the table.
func (i *Item) OccurrenceOffset(n int) (int, error) {
	if n < 0 || n >= i.Occurs {
		return 0, OccurrenceError{
			Pos:    i.Field.Pos,
			Name:   i.Field.Name,
			Occurs: i.Occurs,
			Index:  n,
		}
	}
	return i.Offset + n*i.Stride, nil
}

// Layout is the storage layout of one record: every field's byte offset and byte
// width, and the record's total length.
//
// It is a static layout, computed from the copybook alone. A record holding an
// OCCURS ... DEPENDING ON table has no static layout — its length is a function
// of its own data — and [NewLayout] reports one as an [OccursError] rather than
// advertising a fixed length that is right only at the maximum occurrence count.
type Layout struct {
	// Record is the item for the record itself, root of the item tree.
	Record *Item
	// Length is the total record length in bytes: Record.Total().
	Length int
	// Dialect is the dialect the layout was computed under.
	Dialect Dialect
}

// ErrNilRecord is returned by [NewLayout] when it is handed no record.
var ErrNilRecord = errors.New("nil record")

// NewLayout computes the storage layout of record under d.
//
// record is a field [Build] returned: a level-01 record or a level-77 standalone
// item. Offsets are measured from its first byte, so handing it a field from
// inside a record yields that subtree's layout with its own field at offset
// zero.
//
// Byte widths come from each elementary item's PICTURE, USAGE and the dialect,
// per codec/SPEC.md, "Storage Widths". A group's width is the extent of its
// subordinate items; OCCURS multiplies it; REDEFINES overlays the redefined item
// at the same offset; SYNCHRONIZED inserts slack where the dialect honours it.
// Level-88 condition-names and level-66 RENAMES entries occupy no storage of
// their own and are not items.
//
// It returns a [DialectError] for an incomplete dialect, a [LayoutError] for an
// elementary item whose width cannot be determined, an [OccursError] for an
// OCCURS clause that is not a fixed count, and a [RedefinesError] for a REDEFINES
// clause that cannot be resolved or that the dialect rejects.
func NewLayout(record *Field, d Dialect) (*Layout, error) {
	if record == nil {
		return nil, ErrNilRecord
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}

	l := &layouter{dialect: d}
	// A record's own REDEFINES clause names another record rather than a
	// field of this one, so it is not a constraint on this layout: the
	// record starts at offset zero either way.
	root, err := l.place(record, nil, 0, false)
	if err != nil {
		return nil, err
	}
	return &Layout{Record: root, Length: root.Total(), Dialect: d}, nil
}

// Items returns every item of the layout in source order, outermost first: the
// pre-order walk of the tree, the record included.
func (l *Layout) Items() []*Item {
	var items []*Item
	var walk func(*Item)
	walk = func(item *Item) {
		items = append(items, item)
		for _, child := range item.Children {
			walk(child)
		}
	}
	walk(l.Record)
	return items
}

// Find returns the item of the first field named name, in the [Layout.Items]
// order, or nil when the layout holds no such field. FILLER items have no name
// and are never returned.
func (l *Layout) Find(name string) *Item {
	for _, item := range l.Items() {
		if !item.Field.Filler && item.Field.Name == name {
			return item
		}
	}
	return nil
}

// layouter accumulates the item tree as the record is walked.
type layouter struct {
	dialect Dialect
}

// place lays out f and its subordinate items at or after byte offset at,
// returning the item. redefining says f redefines a preceding sibling, in which
// case at is that sibling's offset and no slack may be inserted before it: the
// two items are required to start at the same byte.
func (l *layouter) place(f *Field, parent *Item, at int, redefining bool) (*Item, error) {
	occurs, err := l.occurrences(f)
	if err != nil {
		return nil, err
	}

	align := 1
	if !redefining {
		align, err = l.alignOf(f)
		if err != nil {
			return nil, err
		}
	}

	offset := roundUp(at, align)
	item := &Item{
		Field:  f,
		Offset: offset,
		Occurs: occurs,
		Slack:  offset - at,
		Parent: parent,
	}

	if f.Kind == KindGroup {
		if err := l.placeChildren(f, item); err != nil {
			return nil, err
		}
	} else {
		width, err := l.width(f)
		if err != nil {
			return nil, err
		}
		item.Length = width
	}

	// Trailing slack exists only to bring the *next* occurrence back onto the
	// boundary, so a field that occurs once never has any: alignment is
	// bytes skipped before an item, and the item's own width is what it is.
	// Rounding a single occurrence up would pad it invisibly — the following
	// field would report Slack 0 while starting later than its own alignment
	// asked for — which matters wherever a width is not already a multiple of
	// its boundary, as BinarySizeSmallest's 3, 5, 6 and 7-byte items are not.
	item.Stride = item.Length
	if occurs > 1 {
		item.Stride = roundUp(item.Length, align)
	}
	return item, nil
}

// placeChildren lays out the subordinate items of a group, setting the group's
// Length to the extent they cover.
//
// cursor is the far end of everything placed so far, and so where the next item
// that redefines nothing goes. A redefining item starts back at its target's
// offset and normally ends within it, leaving cursor alone; one that is longer
// pushes cursor out, which is what makes the group grow to hold it under a
// lenient dialect rather than overlapping whatever follows.
func (l *layouter) placeChildren(f *Field, item *Item) error {
	cursor := item.Offset
	for _, child := range f.Children {
		target, err := l.redefinedBy(child, item)
		if err != nil {
			return err
		}

		at := cursor
		if target != nil {
			at = target.Offset
		}
		sub, err := l.place(child, item, at, target != nil)
		if err != nil {
			return err
		}
		sub.Redefines = target

		if target != nil && l.dialect.Redefines == RedefinesStrict && sub.Total() > target.Total() {
			return RedefinesError{
				Pos:    child.Pos,
				Name:   child.Name,
				Target: target.Field.Name,
				Reason: fmt.Sprintf("occupies %d bytes, more than the %d bytes of %s it redefines",
					sub.Total(), target.Total(), describe(target.Field)),
			}
		}
		if end := sub.End(); end > cursor {
			cursor = end
		}

		item.Children = append(item.Children, sub)
	}
	item.Length = cursor - item.Offset
	return nil
}

// redefinedBy resolves a child's REDEFINES clause against the items of its group
// already placed, returning nil when it carries no such clause.
func (l *layouter) redefinedBy(child *Field, group *Item) (*Item, error) {
	clause := redefinesOf(child.Entry)
	if clause == nil || clause.Name == nil {
		return nil, nil
	}

	for _, placed := range group.Children {
		if !placed.Field.Filler && placed.Field.Name == clause.Name.Value {
			return placed, nil
		}
	}
	return nil, RedefinesError{
		Pos:    child.Pos,
		Name:   child.Name,
		Target: clause.Name.Value,
		Reason: fmt.Sprintf("no preceding item named %q in %s", clause.Name.Value, describe(group.Field)),
	}
}

// occurrences reports the occurrence count of a field: 1 when it carries no
// OCCURS clause, and the clause's count when it carries a fixed one.
//
// An OCCURS ... DEPENDING ON table has no static occurrence count, so it is
// reported rather than laid out at either bound.
func (l *layouter) occurrences(f *Field) (int, error) {
	clause := occursOf(f.Entry)
	if clause == nil {
		return 1, nil
	}
	switch {
	case clause.DependingOn != nil:
		return 0, OccursError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: "OCCURS DEPENDING ON makes the record variable-length; a static layout cannot place the fields after it",
		}
	case clause.Max != nil:
		return 0, OccursError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: "OCCURS with a range of occurrence counts and no DEPENDING ON has no fixed length",
		}
	case clause.Min == nil:
		return 0, OccursError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: "OCCURS clause states no occurrence count",
		}
	}

	count, err := strconv.Atoi(clause.Min.Value)
	if err != nil || count < 1 {
		return 0, OccursError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: fmt.Sprintf("occurrence count %q is not a positive whole number", clause.Min.Value),
		}
	}
	return count, nil
}

// width reports the byte width of one occurrence of an elementary item, as
// codec/SPEC.md, "Storage Widths", defines it.
func (l *layouter) width(f *Field) (int, error) {
	switch f.Usage {
	case UsageDisplay:
		return l.displayWidth(f)
	case UsagePackedDecimal, UsageComp3:
		digits, err := digitsOf(f)
		if err != nil {
			return 0, err
		}
		// One nibble per digit plus the sign nibble, rounded up to a byte.
		return (digits + 2) / 2, nil
	case UsageBinary, UsageComp, UsageComp4, UsageComp5:
		digits, err := digitsOf(f)
		if err != nil {
			return 0, err
		}
		return l.dialect.Binary.width(digits), nil
	case UsageComp1:
		return 4, nil
	case UsageComp2:
		return 8, nil
	case UsageIndex:
		return l.dialect.IndexWidth, nil
	case UsagePointer:
		return l.dialect.PointerWidth, nil
	}
	return 0, LayoutError{
		Pos:    f.Pos,
		Name:   f.Name,
		Reason: fmt.Sprintf("has no storage width under USAGE %s", f.Usage),
	}
}

// displayWidth reports the width of a USAGE DISPLAY item: one character position
// per PICTURE character position, plus a byte for a separate sign.
func (l *layouter) displayWidth(f *Field) (int, error) {
	if f.Picture == nil {
		return 0, LayoutError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: "is an elementary USAGE DISPLAY item with no PICTURE clause",
		}
	}
	width := f.Picture.Size
	if f.Picture.Signed && separateSign(f) {
		width++
	}
	return width, nil
}

// digitsOf reports the digit count of an item whose width is a function of it:
// packed decimal and binary. Those usages take a numeric PICTURE and nothing
// else, so anything else is reported rather than measured as though it were the
// character count.
func digitsOf(f *Field) (int, error) {
	switch {
	case f.Picture == nil:
		return 0, LayoutError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: fmt.Sprintf("is a USAGE %s item with no PICTURE clause", f.Usage),
		}
	case f.Picture.Category != picture.CategoryNumeric:
		return 0, LayoutError{
			Pos:  f.Pos,
			Name: f.Name,
			Reason: fmt.Sprintf("is a USAGE %s item whose PICTURE %s is %s rather than numeric",
				f.Usage, f.Picture.Source, f.Picture.Category),
		}
	case f.Picture.Digits < 1:
		return 0, LayoutError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: fmt.Sprintf("is a USAGE %s item whose PICTURE %s has no digit positions", f.Usage, f.Picture.Source),
		}
	}
	return f.Picture.Digits, nil
}

// separateSign reports whether a signed USAGE DISPLAY item spends a byte of its
// own on its sign, which is what SIGN IS ... SEPARATE CHARACTER asks for.
//
// The clause may be written on the item or on any group above it, applying to
// every signed numeric DISPLAY item subordinate to it, so the nearest one wins —
// the same inheritance USAGE follows. Its absence is TRAILING, non-separate,
// which overpunches the sign into a digit byte and costs nothing.
func separateSign(f *Field) bool {
	for item := f; item != nil; item = item.Parent {
		if clause := signOf(item.Entry); clause != nil {
			return clause.Separate
		}
	}
	return false
}

// alignOf reports the boundary the field's storage must start on.
//
// A SYNCHRONIZED elementary item aligns to its natural boundary. A group aligns
// only where it has an OCCURS clause, and then to the widest boundary anything
// inside it needs: that is what keeps every occurrence laid out identically,
// which aligning the items individually would not.
func (l *layouter) alignOf(f *Field) (int, error) {
	if l.dialect.Sync != SyncAligned {
		return 1, nil
	}
	if f.Kind == KindGroup {
		if occursOf(f.Entry) == nil {
			return 1, nil
		}
		return l.subtreeAlign(f)
	}
	if syncOf(f.Entry) == nil {
		return 1, nil
	}
	return l.naturalAlign(f)
}

// subtreeAlign reports the widest boundary any SYNCHRONIZED item in the field's
// subtree needs, 1 when it holds none.
func (l *layouter) subtreeAlign(f *Field) (int, error) {
	if f.Kind != KindGroup {
		if syncOf(f.Entry) == nil {
			return 1, nil
		}
		return l.naturalAlign(f)
	}

	align := 1
	for _, child := range f.Children {
		a, err := l.subtreeAlign(child)
		if err != nil {
			return 0, err
		}
		if a > align {
			align = a
		}
	}
	return align, nil
}

// naturalAlign reports the boundary a SYNCHRONIZED item of this usage sits on.
//
// SYNCHRONIZED is syntax-checked and has no effect on USAGE DISPLAY and packed
// decimal items: their bytes are characters and nibbles, with no machine
// boundary to sit on. It is the binary, floating-point, index and pointer items
// that align, and each to the width of the machine datum it is held in.
func (l *layouter) naturalAlign(f *Field) (int, error) {
	switch f.Usage {
	case UsageBinary, UsageComp, UsageComp4, UsageComp5:
		width, err := l.width(f)
		if err != nil {
			return 0, err
		}
		return boundary(width), nil
	case UsageComp1:
		return 4, nil
	case UsageComp2:
		return 8, nil
	case UsageIndex:
		return boundary(l.dialect.IndexWidth), nil
	case UsagePointer:
		return boundary(l.dialect.PointerWidth), nil
	}
	return 1, nil
}

// boundary reports the machine boundary a datum of the given width sits on: the
// next power of two at or above it, capped at a doubleword. Nothing aligns past
// eight bytes, which is what the sixteen-byte ARITH(EXTEND) items rely on.
func boundary(width int) int {
	switch {
	case width <= 1:
		return 1
	case width <= 2:
		return 2
	case width <= 4:
		return 4
	}
	return 8
}

// roundUp rounds n up to the next multiple of align.
func roundUp(n, align int) int {
	if align <= 1 {
		return n
	}
	if rem := n % align; rem != 0 {
		return n + align - rem
	}
	return n
}

// occursOf returns the entry's OCCURS clause, or nil.
func occursOf(entry *cobol.DataDescriptionEntry) *cobol.OccursClause {
	for _, clause := range entry.Clauses {
		if occurs, ok := clause.(*cobol.OccursClause); ok {
			return occurs
		}
	}
	return nil
}

// redefinesOf returns the entry's REDEFINES clause, or nil.
func redefinesOf(entry *cobol.DataDescriptionEntry) *cobol.RedefinesClause {
	for _, clause := range entry.Clauses {
		if redefines, ok := clause.(*cobol.RedefinesClause); ok {
			return redefines
		}
	}
	return nil
}

// syncOf returns the entry's SYNCHRONIZED clause, or nil.
func syncOf(entry *cobol.DataDescriptionEntry) *cobol.SynchronizedClause {
	for _, clause := range entry.Clauses {
		if sync, ok := clause.(*cobol.SynchronizedClause); ok {
			return sync
		}
	}
	return nil
}

// signOf returns the entry's SIGN clause, or nil.
func signOf(entry *cobol.DataDescriptionEntry) *cobol.SignClause {
	for _, clause := range entry.Clauses {
		if sign, ok := clause.(*cobol.SignClause); ok {
			return sign
		}
	}
	return nil
}

// DialectError is returned when a [Dialect] field was left undeclared or set to
// a value naming no member. Field names the offending field.
type DialectError struct {
	// Field is the name of the offending [Dialect] field.
	Field string
	// Reason says what is wrong with it.
	Reason string
}

// Error implements the [error] interface.
func (e DialectError) Error() string {
	return fmt.Sprintf("invalid dialect: field %s %s", e.Field, e.Reason)
}

// LayoutError is returned when an elementary item's storage width cannot be
// determined: an item with no PICTURE where its usage requires one, or one whose
// PICTURE describes something that usage cannot hold.
type LayoutError struct {
	Pos    cobol.Pos
	Name   string
	Reason string
}

// Error implements the [error] interface.
func (e LayoutError) Error() string {
	return fmt.Sprintf("item %s at line %d, column %d %s",
		quoteOrItem(e.Name), e.Pos.Line, e.Pos.Column, e.Reason)
}

// OccursError is returned for an OCCURS clause a static layout cannot place: an
// OCCURS ... DEPENDING ON table, whose length is a function of the record's own
// data, or an occurrence count that is not a positive whole number.
type OccursError struct {
	Pos    cobol.Pos
	Name   string
	Reason string
}

// Error implements the [error] interface.
func (e OccursError) Error() string {
	return fmt.Sprintf("item %s at line %d, column %d: %s",
		quoteOrItem(e.Name), e.Pos.Line, e.Pos.Column, e.Reason)
}

// RedefinesError is returned for a REDEFINES clause that cannot be resolved
// against a preceding item of the same group, or for a redefining item larger
// than what it redefines under a [RedefinesStrict] dialect. Target is the
// data-name the clause names.
type RedefinesError struct {
	Pos    cobol.Pos
	Name   string
	Target string
	Reason string
}

// Error implements the [error] interface.
func (e RedefinesError) Error() string {
	return fmt.Sprintf("REDEFINES on item %s at line %d, column %d: %s",
		quoteOrItem(e.Name), e.Pos.Line, e.Pos.Column, e.Reason)
}

// OccurrenceError is returned by [Item.OccurrenceOffset] for an occurrence index
// outside the item's occurrence count.
type OccurrenceError struct {
	Pos    cobol.Pos
	Name   string
	Occurs int
	Index  int
}

// Error implements the [error] interface.
func (e OccurrenceError) Error() string {
	return fmt.Sprintf("item %s at line %d, column %d: occurrence %d is out of range: the item occurs %d times",
		quoteOrItem(e.Name), e.Pos.Line, e.Pos.Column, e.Index, e.Occurs)
}
