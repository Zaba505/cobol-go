// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package copybook assembles the flat data description entries a COBOL parse
// produces into the record tree their level numbers imply, and computes where
// each field of that tree sits in a record's bytes.
//
// The root cobol package deliberately keeps [cobol.DataSection.Entries] and
// [cobol.Fragment.Entries] flat: "the record hierarchy is implied by the
// entries' level numbers rather than nested in the AST". Every consumer of a
// copybook — a code generator, a record decoder, a schema exporter — has to
// rebuild that hierarchy before it can do anything useful, and rebuilding it
// means knowing that a level-88 occupies no storage, that a level-66 is a view
// over other fields rather than a field of its own, that an omitted data-name
// means FILLER, and that USAGE flows down from a group to the items subordinate
// to it. This package does that once.
//
// [Build] is the whole entry point: hand it a flat entry list and it returns the
// top-level records, each a [Field] with its subordinate fields nested beneath
// it.
//
//	f, err := cobol.Parse(src, cobol.WithFragment())
//	records, err := copybook.Build(f.Fragment.Entries)
//
// Every [Field] keeps a pointer to the [cobol.DataDescriptionEntry] it was built
// from, so the clauses [Build] itself does not interpret — JUSTIFIED, BLANK WHEN
// ZERO, VALUE on an ordinary item — remain reachable without this package
// re-modelling the AST.
//
// # Storage layout
//
// [NewLayout] is the second half: hand it a record and a [Dialect] and it
// returns the byte offset and byte width of every field, and the record's total
// length.
//
//	l, err := copybook.NewLayout(records[0], copybook.IBMEnterprise())
//	zip := l.Find("ADDR-ZIP")   // zip.Offset, zip.Length
//
// Widths come from each elementary item's PICTURE, USAGE and the dialect, per
// codec/SPEC.md, "Storage Widths": PIC 9(5) COMP is four bytes and not five, a
// COMP-3 item is one nibble per digit plus a sign nibble, and a group is as wide
// as its subordinate items. OCCURS multiplies a subtree and gives it a stride,
// REDEFINES overlays its target at the same offset, and SYNCHRONIZED inserts
// slack bytes where the dialect honours it.
//
// The layout is *static*. A record holding an OCCURS ... DEPENDING ON table has
// no static layout, and [NewLayout] reports one rather than advertising a fixed
// length that is right only at one occurrence count.
//
// A [Dialect] is required and every field of it has an invalid zero value, for
// the same reason [github.com/Zaba505/cobol-go/codec.Encoding] does: a wrong
// layout setting shifts every following field, and surfaces — if at all —
// several fields later as a length mismatch rather than as "the dialect was
// wrong". The two are separate types because they answer different questions: a
// [Dialect] is a property of the compiler that produced the record, a
// [github.com/Zaba505/cobol-go/codec.Encoding] a property of the file in hand
// (codec/SPEC.md, "By compiler vs by file").
//
// This package imports the root cobol package (for the AST) and picture (for the
// parsed PICTURE character-string of each elementary item), and nothing else in
// this module. In particular it does not import codec: nothing here reads or
// writes a byte, and the byte-level rules it shares with codec are stated once in
// codec/SPEC.md rather than in either package's code.
package copybook
