// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package copybook assembles the flat data description entries a COBOL parse
// produces into the record tree their level numbers imply.
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
// What this package does not do is compute storage: no offsets, no byte widths,
// no OCCURS arithmetic. Those need the byte-level representation rules that live
// in codec, so they are layered on top of this tree rather than mixed into it.
// Every [Field] keeps a pointer to the [cobol.DataDescriptionEntry] it was built
// from, so the clauses this package does not interpret — OCCURS, REDEFINES,
// SYNCHRONIZED, JUSTIFIED, VALUE on an ordinary item — remain reachable without
// this package re-modelling the AST.
//
// This package imports the root cobol package (for the AST) and picture (for the
// parsed PICTURE character-string of each elementary item), and nothing else in
// this module.
package copybook
