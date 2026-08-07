// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package picture parses COBOL PICTURE character-strings into a structured
// [Picture]: the item's category, its digit count, its scale, whether it
// carries an operational sign, and the expanded sequence of PICTURE symbols.
//
// The root cobol package's PictureClause carries the raw PICTURE lexeme with
// its source case preserved; deriving anything from it — how many digits it
// describes, where the assumed decimal point falls, which category it belongs
// to — is this package's job. Neither storage width nor digit count falls out
// of counting characters (PIC ZZ,ZZ9.99 occupies nine character positions but
// describes seven digit positions), so this is a parser rather than a regex.
//
// This package imports nothing else in this module, deliberately: the root
// package may come to import it for PICTURE category validation, so it sits
// below the root in the dependency graph.
//
// The rules implemented here are the ones stated in the root SPEC.md ("PICTURE
// Character-Strings" and the Semantics section) and in codec/SPEC.md ("From
// PICTURE to Attributes"), which is the normative contract for Category,
// Digits, Scale, and Signed.
package picture
