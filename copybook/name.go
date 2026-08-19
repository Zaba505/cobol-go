// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

import "strings"

// sameName reports whether two data-names name the same item.
//
// COBOL words are case-insensitive, data-names included: `Header`, `HEADER` and
// `header` are one name, and every compiler resolves a `REDEFINES HEADER` to an
// item declared as `Header` (root SPEC.md, Names and Terminals). [Field.Name]
// holds the data-name with its source case preserved, so the fold belongs in the
// comparison rather than in what is stored — the printer round-trips the
// spelling the source used, and folding at storage time would lose it.
//
// Every data-name resolution in this package goes through this one function:
// the target of a REDEFINES clause, whether resolved by [Build] against the open
// chain or by [NewLayout] against a group's placed items; the endpoints of a
// level-66 RENAMES range; the data-name of an OCCURS DEPENDING ON phrase; and
// the public [Layout.Find]. That is deliberate and is the whole of the change:
// folding case at one site and not another would let [Build] and [NewLayout]
// mean different things by "the same name", so a copybook could be accepted by
// the tree builder on a match the layouter cannot reproduce.
func sameName(a, b string) bool {
	return strings.EqualFold(a, b)
}
