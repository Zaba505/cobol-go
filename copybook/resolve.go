// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

import (
	"fmt"
	"math"
	"strconv"

	cobol "github.com/Zaba505/cobol-go"
	"github.com/Zaba505/cobol-go/picture"
)

// counter supplies the occurrence count of an OCCURS ... DEPENDING ON table.
//
// It is called once the table's controlling item has been placed, so control
// carries a concrete offset and length whatever the record before it holds: the
// controlling field is required to be defined before the table, and the walk
// resolves counts in that same order. low and high are the bounds the OCCURS
// clause declares; a counter is not required to honour them, because the layouter
// checks the count it returns against them either way.
type counter func(table *Field, control *Item, low, high int) (int, error)

// maxCount lays a table out at the largest occurrence count it declares, which is
// the storage a compiler reserves for it and so the honest static reading.
func maxCount(_ *Field, _ *Item, _, high int) (int, error) { return high, nil }

// minCount lays a table out at the smallest occurrence count it declares.
func minCount(_ *Field, _ *Item, low, _ int) (int, error) { return low, nil }

// Resolve lays the record out at the occurrence counts data carries, returning a
// layout whose offsets and length are concrete.
//
// data is one record's bytes, starting at the record's first byte. It may be
// longer than the record — a fixed-length block holding a shorter record is
// ordinary — but every controlling field must lie within it.
//
// A layout that is not [Layout.Variable] is returned unchanged and data is not
// read at all, so a caller holding a record of unknown shape may resolve it
// unconditionally.
//
// Resolve reads a controlling field whose USAGE is DISPLAY or PACKED-DECIMAL /
// COMP-3. Those two are the only representations whose digits are the same bytes
// whatever charset and byte order the file was written with (codec/SPEC.md,
// "Charset as a First-Class Axis"): a zoned digit's value is its low nibble in
// both ASCII and EBCDIC, and packed decimal is nibbles in neither. A binary
// controlling field is not read here, because its value depends on the byte order
// of the file in hand — a property of the file that this package does not carry
// and will not guess. Read it with
// [github.com/Zaba505/cobol-go/codec.Reader] and call [Layout.ResolveCounts]
// instead.
//
// It returns a [DependingError] for a count that cannot be read, that falls
// outside the bounds the OCCURS clause declares, or whose controlling field lies
// past the end of data.
func (l *Layout) Resolve(data []byte) (*Layout, error) {
	return l.resolve(func(f *Field, control *Item, _, _ int) (int, error) {
		return readCount(f, control, data)
	})
}

// ResolveCounts lays the record out at occurrence counts the caller has already
// read, returning a layout whose offsets and length are concrete.
//
// counts is keyed by the *controlling* field — the one an OCCURS ... DEPENDING ON
// phrase names, reachable as [Item.DependingOn] — and not by the table it
// controls, so two tables depending on the same field take one entry:
//
//	l, err := copybook.NewLayout(rec, copybook.IBMEnterprise())
//	n := l.Find("ITEM-COUNT")
//	fixed, err := l.ResolveCounts(map[*copybook.Field]int{n.Field: 3})
//
// It is the way to resolve a record whose controlling field is binary, whose
// bytes [Layout.Resolve] will not guess the order of; it also reads no bytes at
// all, so a caller who knows the count from elsewhere need not have the record in
// hand.
//
// A layout that is not [Layout.Variable] is returned unchanged. It returns a
// [DependingError] for a controlling field counts does not mention, and for a
// count outside the bounds the OCCURS clause declares.
func (l *Layout) ResolveCounts(counts map[*Field]int) (*Layout, error) {
	return l.resolve(func(f *Field, control *Item, _, _ int) (int, error) {
		n, ok := counts[control.Field]
		if !ok {
			return 0, DependingError{
				Pos:       f.Pos,
				Name:      f.Name,
				DependsOn: control.Field.Name,
				Reason: fmt.Sprintf("no occurrence count was supplied for controlling item %s",
					describe(control.Field)),
			}
		}
		return n, nil
	})
}

// resolve re-walks the record with count in place of the static maximum. The
// result is a layout of the same shape with concrete offsets, so everything
// [Layout] offers — [Layout.Items], [Layout.Find], [Item.OccurrenceOffset] —
// works on it unchanged.
func (l *Layout) resolve(count counter) (*Layout, error) {
	if !l.Variable {
		return l, nil
	}

	root, _, err := layoutRecord(l.Record.Field, l.Dialect, count)
	if err != nil {
		return nil, err
	}
	total := root.Total()
	return &Layout{
		Record:    root,
		Length:    total,
		MinLength: total,
		MaxLength: total,
		Dialect:   l.Dialect,
	}, nil
}

// dependingTable resolves an OCCURS ... DEPENDING ON clause: its bounds, the item
// it takes its count from, and the count itself.
func (l *layouter) dependingTable(f *Field, clause *cobol.OccursClause) (table, error) {
	low, high, err := occursBounds(f, clause)
	if err != nil {
		return table{}, err
	}

	control, err := l.control(f, clause.DependingOn)
	if err != nil {
		return table{}, err
	}

	// The record is variable-length because the copybook says so, whatever
	// count this particular walk lays the table out at.
	l.variable = true

	n, err := l.count(f, control, low, high)
	if err != nil {
		return table{}, err
	}
	if n < low || n > high {
		return table{}, DependingError{
			Pos:       f.Pos,
			Name:      f.Name,
			DependsOn: control.Field.Name,
			Reason: fmt.Sprintf("occurrence count %d is outside the %d to %d the OCCURS clause allows",
				n, low, high),
		}
	}
	return table{occurs: n, min: low, max: high, control: control}, nil
}

// occursBounds reports the smallest and largest occurrence counts a DEPENDING ON
// clause allows.
//
// OCCURS min TO max DEPENDING ON states both. OCCURS n DEPENDING ON states only
// n, which is the *maximum*: the standard's format is the TO form, and the
// compilers that accept the short one read the single count as the upper bound
// with a lower bound of one.
func occursBounds(f *Field, clause *cobol.OccursClause) (int, int, error) {
	if clause.Min == nil {
		return 0, 0, OccursError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: "OCCURS clause states no occurrence count",
		}
	}

	first, err := strconv.Atoi(clause.Min.Value)
	if err != nil || first < 0 {
		return 0, 0, OccursError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: fmt.Sprintf("occurrence count %q is not a whole number", clause.Min.Value),
		}
	}
	if clause.Max == nil {
		if first < 1 {
			return 0, 0, OccursError{
				Pos:    f.Pos,
				Name:   f.Name,
				Reason: "OCCURS 0 DEPENDING ON admits no occurrences at all",
			}
		}
		return 1, first, nil
	}

	last, err := strconv.Atoi(clause.Max.Value)
	if err != nil || last < 1 {
		return 0, 0, OccursError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: fmt.Sprintf("occurrence count %q is not a positive whole number", clause.Max.Value),
		}
	}
	if last < first {
		return 0, 0, OccursError{
			Pos:    f.Pos,
			Name:   f.Name,
			Reason: fmt.Sprintf("occurrence count range %d TO %d ends below where it starts", first, last),
		}
	}
	return first, last, nil
}

// control resolves the data-name of a DEPENDING ON phrase to the item holding the
// occurrence count.
//
// It is resolved against the items already placed, so a controlling field written
// after the table it controls is an error rather than a forward reference: the
// count has to be readable before the table's extent is known, and a field whose
// own offset depends on that extent never is.
func (l *layouter) control(f *Field, name *cobol.Word) (*Item, error) {
	for _, placed := range l.placed {
		if placed.Field.Filler || placed.Field.Name != name.Value {
			continue
		}
		if reason := unusableAsCount(placed.Field); reason != "" {
			return nil, DependingError{Pos: f.Pos, Name: f.Name, DependsOn: name.Value, Reason: reason}
		}
		return placed, nil
	}

	reason := fmt.Sprintf("no item named %q is defined before the table it controls", name.Value)
	if indexOf(flatten(recordOf(f)), name.Value) >= 0 {
		reason = fmt.Sprintf("item %q is defined after the table it controls", name.Value)
	}
	return nil, DependingError{Pos: f.Pos, Name: f.Name, DependsOn: name.Value, Reason: reason}
}

// unusableAsCount reports why a field cannot hold an occurrence count, and the
// empty string when it can. An occurrence count is an integer, so the field has
// to be an elementary item with an integer numeric PICTURE.
func unusableAsCount(f *Field) string {
	switch {
	case f.Kind == KindGroup:
		return fmt.Sprintf("controlling item %s is a group item rather than an integer", describe(f))
	case f.Picture == nil:
		return fmt.Sprintf("controlling item %s has no PICTURE clause and so no integer value", describe(f))
	case f.Picture.Category != picture.CategoryNumeric:
		return fmt.Sprintf("controlling item %s has PICTURE %s, which is %s rather than numeric",
			describe(f), f.Picture.Source, f.Picture.Category)
	case f.Picture.Scale != 0:
		return fmt.Sprintf("controlling item %s has PICTURE %s, which is not an integer",
			describe(f), f.Picture.Source)
	}
	return ""
}

// recordOf returns the top-level record a field belongs to.
func recordOf(f *Field) *Field {
	for f.Parent != nil {
		f = f.Parent
	}
	return f
}

// readCount reads the occurrence count of a table out of its controlling item's
// bytes.
func readCount(f *Field, control *Item, data []byte) (int, error) {
	fail := func(reason string, args ...any) (int, error) {
		return 0, DependingError{
			Pos:       f.Pos,
			Name:      f.Name,
			DependsOn: control.Field.Name,
			Reason:    fmt.Sprintf(reason, args...),
		}
	}

	end := control.Offset + control.Length
	if len(data) < end {
		return fail("controlling item %s ends at byte %d of a record only %d bytes long",
			describe(control.Field), end, len(data))
	}

	var (
		value int64
		err   error
	)
	switch control.Field.Usage {
	case UsageDisplay:
		value, err = zonedValue(control.Field, data[control.Offset:end])
	case UsagePackedDecimal, UsageComp3:
		value, err = packedValue(data[control.Offset:end])
	default:
		return fail("controlling item %s is a USAGE %s item, whose value depends on the byte order of the file it came from: read it with the codec package and call ResolveCounts",
			describe(control.Field), control.Field.Usage)
	}
	if err != nil {
		return fail("controlling item %s at byte %d: %s", describe(control.Field), control.Offset, err)
	}
	if value > math.MaxInt32 {
		return fail("controlling item %s holds %d, which is no occurrence count", describe(control.Field), value)
	}
	return int(value), nil
}

// zoned digit and sign zones, the high nibble of a USAGE DISPLAY digit byte.
//
// A digit byte is F0–F9 in EBCDIC and 30–39 in ASCII, so a zone of F or 3 is a
// plain digit under either charset. The last byte of a signed non-separate item
// carries the sign in that nibble instead, and the positive zones are C in
// EBCDIC, 4 in a translated-EBCDIC file, and 3 under the ASCII conventions —
// which is the plain digit zone (codec/SPEC.md, "Zoned Sign Conventions"). The
// negative zones are deliberately absent: an occurrence count is never negative,
// so a byte carrying one is an error and not a value to take the absolute value
// of.
const (
	zoneEBCDIC          = 0xF
	zoneASCII           = 0x3
	zonePositiveEBCDIC  = 0xC
	zonePositiveTranslt = 0x4
)

// zonedValue reads the unsigned integer value of a USAGE DISPLAY item.
//
// The digit is the low nibble of the byte under every charset and every sign
// convention this module admits, which is exactly why a zoned controlling field
// can be read without knowing which of them produced the record. The zone nibble
// is checked all the same, so that a blank field or a negative one is an error
// rather than a plausible count.
func zonedValue(f *Field, b []byte) (int64, error) {
	signed := f.Picture.Signed
	if signed && separateSign(f) {
		if clause := inheritedSign(f); clause != nil && clause.Position == "LEADING" {
			b = b[1:]
		} else {
			b = b[:len(b)-1]
		}
	}
	if len(b) == 0 {
		return 0, fmt.Errorf("holds no digit positions")
	}

	var value int64
	for i, c := range b {
		zone, digit := c>>4, int64(c&0x0f)
		ok := zone == zoneEBCDIC || zone == zoneASCII
		if !ok && signed && i == len(b)-1 {
			ok = zone == zonePositiveEBCDIC || zone == zonePositiveTranslt
		}
		if !ok || digit > 9 {
			return 0, fmt.Errorf("byte %d is %#02x, which is no unsigned digit", i, c)
		}
		value = value*10 + digit
	}
	return value, nil
}

// packedValue reads the unsigned integer value of a PACKED-DECIMAL / COMP-3 item:
// two digits per byte, most significant first, with the sign in the low nibble of
// the last byte.
//
// Packed decimal is nibbles rather than characters, so it reads the same whatever
// charset the file uses, and it has no byte order to get wrong.
func packedValue(b []byte) (int64, error) {
	if len(b) == 0 {
		return 0, fmt.Errorf("holds no digit positions")
	}

	var value int64
	for i, c := range b {
		hi, lo := int64(c>>4), int64(c&0x0f)
		if hi > 9 {
			return 0, fmt.Errorf("byte %d is %#02x, whose high nibble is no digit", i, c)
		}
		value = value*10 + hi

		if i == len(b)-1 {
			// D and B are the negative sign nibbles; C, F, A and E are
			// the positive and unsigned ones.
			if lo == 0xD || lo == 0xB {
				return 0, fmt.Errorf("byte %d is %#02x, whose sign nibble is negative", i, c)
			}
			if lo < 0xA {
				return 0, fmt.Errorf("byte %d is %#02x, whose low nibble is no sign", i, c)
			}
			break
		}
		if lo > 9 {
			return 0, fmt.Errorf("byte %d is %#02x, whose low nibble is no digit", i, c)
		}
		value = value*10 + lo
	}
	return value, nil
}

// DependingError is returned for an OCCURS ... DEPENDING ON phrase that cannot be
// resolved: a controlling data-name that names no item defined before the table,
// one that cannot hold an integer, a count that cannot be read from a record's
// bytes, and a count outside the bounds the OCCURS clause declares.
//
// Name is the table the clause is written on and DependsOn the data-name the
// phrase names.
type DependingError struct {
	Pos       cobol.Pos
	Name      string
	DependsOn string
	Reason    string
}

// Error implements the [error] interface.
func (e DependingError) Error() string {
	return fmt.Sprintf("OCCURS DEPENDING ON %s on item %s at line %d, column %d: %s",
		quoteOrItem(e.DependsOn), quoteOrItem(e.Name), e.Pos.Line, e.Pos.Column, e.Reason)
}
