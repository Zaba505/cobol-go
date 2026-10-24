// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package cobol provides tools for working with COBOL.
//
// # Error positions
//
// Every error this module returns that carries a source position — as a Pos
// field, or as the Pos of the [Token] it was reported against — has a Position
// method returning it, and the exported [Positioned] interface names that
// method. The interface is exported, rather than left for each caller to spell
// as an anonymous interface{ Position() Pos }, so that the set of errors with a
// position is a type of this package that its documentation and its tests can
// hold to: a test fails if an exported error type carrying a position lacks the
// method, so one added later cannot fall through a caller's errors.As.
//
//	var p cobol.Positioned
//	if errors.As(err, &p) {
//		fmt.Printf("%d:%d: %v\n", p.Position().Line, p.Position().Column, p)
//	}
//
// Positions on errors raised inside copied text are positions within the
// copybook, as they are on the nodes built from it. Errors with no source
// position — the codec package's, a PICTURE character-string's byte offsets in
// the picture package, [UnexpectedEndOfTokensError], [UnsupportedNodeError] —
// do not implement [Positioned].
package cobol
