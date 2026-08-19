// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

// sameName reports whether two data-names name the same item.
//
// COBOL words are case-insensitive, data-names included: `Header`, `HEADER` and
// `header` are one name, and every compiler resolves a `REDEFINES HEADER` to an
// item declared as `Header` (root SPEC.md, Names and Terminals). [Field.Name]
// holds the data-name with its source case preserved, so the fold belongs in the
// comparison rather than in what is stored — the printer round-trips the
// spelling the source used, and folding at storage time would lose it.
//
// The fold is ASCII and not [strings.EqualFold]'s Unicode simple folding, which
// is wider than COBOL's rule: simple folding makes U+017F (ſ) equal to "s" and
// U+212A (K) equal to "k". The root tokenizer admits only ASCII letters, digits,
// hyphen and underscore into a word, so nothing wider can reach here today —
// which is exactly why the rule is written out rather than inherited. A
// comparison that states its own alphabet cannot be broadened by a change to
// some other file.
//
// The empty string is not a name and matches nothing, itself included. A FILLER
// item carries one, so rejecting it here means a call site that forgets its
// [Field.Filler] guard still cannot match an unnamed item — the guard is a
// second line of defence rather than the only one.
//
// Every data-name *resolution* in this package goes through this one function:
// the target of a REDEFINES clause, whether resolved by [Build] against the open
// chain or by [NewLayout] against a group's placed items; the endpoints of a
// level-66 RENAMES range; the data-name of an OCCURS ... DEPENDING ON phrase;
// and the public [Layout.Find]. That is deliberate: folding case at one site and
// not another would let [Build] and [NewLayout] mean different things by "the
// same name", so a copybook could be accepted by the tree builder on a match the
// layouter cannot reproduce.
//
// Three constructs that name a data-name are absent from that list because this
// package does not resolve them at all, and each becomes a call site here on the
// day it does: an OCCURS clause's ASCENDING/DESCENDING KEY IS phrase, which is
// carried on the entry and not interpreted; a level-88 condition-name, which is
// attached to the item it follows rather than looked up; and duplicate-name
// detection, which this package does not perform. The last is the one to be
// careful with — folding changes what counts as a duplicate, since Header and
// HEADER in one group were two names before this function existed and are one
// now — so a check added for it must be written against this function rather
// than against its own comparison.
func sameName(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	// No ASCII fold changes a name's width, so unequal lengths settle it.
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'a' <= ca && ca <= 'z' {
			ca -= 'a' - 'A'
		}
		if 'a' <= cb && cb <= 'z' {
			cb -= 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
