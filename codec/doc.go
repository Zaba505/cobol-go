// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package codec reads and writes the bytes of a COBOL data file.
//
// It follows the binary file library layout — types.go, decoder.go,
// encoder.go — and is the runtime half of this module: a generated file
// library links this package and nothing else, never the COBOL source parser
// in the root package. To keep that true, codec imports only the standard
// library, and it will not grow a dependency on the parser, the picture
// package, or anything outside std.
//
// # No default encoding
//
// Four settings must be known before a single byte of a data file can be
// interpreted: the character set, the zoned-decimal sign convention, the byte
// order of binary items, and the floating-point format. None of them is
// recoverable from the file with certainty, and every one of them fails
// silently when wrong — the wrong value yields a plausible but incorrect
// number rather than an error.
//
// So this package has no default for any of them and no usable zero-value
// [Reader]. [Encoding] carries all four, every field of it has an invalid zero
// value, and [NewReader] and [NewWriter] fail with an [EncodingError] naming
// the field that was left out. See codec/SPEC.md, "The Four Axes of an
// Encoding", which states that as a normative requirement on this package.
//
// The named bundles keep that ergonomic without making it implicit.
// [IBMEnterprise], [MicroFocusASCII], [GnuCOBOLASCII] and [ConvertedFromEBCDIC]
// each expand to a complete [Encoding], so a caller states an assumption in one
// call rather than inheriting one:
//
//	r, err := codec.NewReader(f, codec.MicroFocusASCII())
//
// A bundle is a value the caller passes, never a fallback the package applies.
// [ConvertedFromEBCDIC] exists because it is the combination real files hit and
// no compiler produces: a mainframe-written file converted to ASCII has ASCII
// characters but translated-EBCDIC signs.
//
// # Scope of this package as it stands
//
// [Reader] and [Writer] currently cover alphanumeric fields, raw byte fields,
// packed decimal (COMP-3) and binary (COMP, COMP-4, BINARY, COMP-5). Zoned
// decimal and floating-point accessors, and the charset axis beyond [ASCII] and
// [CP037], arrive in later stories; the [Encoding] axes they need are already
// declared here so that no field of it ever has to acquire a default
// retroactively.
//
// # Binary items: width, byte order and range
//
// A binary item's width is a staircase in its digit count and not the digit
// count itself: 2 bytes through 4 digits, 4 through 9, 8 through 18 and 16
// beyond. PIC 9(5) COMP is four bytes, not five, and a wrong step shifts every
// later field in the record.
//
// Byte order comes from [Encoding.ByteOrder] and is never inferred, because a
// swapped reading is a plausible number rather than an error. What the digit
// count *means* is a second fork, and it is a property of the compiler rather
// than of the file, so it is selected by which accessor is called rather than
// by an [Encoding] axis: [Reader.ReadBinaryInt16] and its family are TRUNC(STD),
// where a PIC S9(4) COMP item holds -9999 to 9999, and [Reader.ReadComp5Int16]
// and its family are TRUNC(BIN), where the same two bytes hold -32768 to 32767.
// See [Truncation].
//
// The TRUNC(STD) family validates what it reads, which is the one detector this
// package has for a misdeclared byte order: a byte-swapped small number usually
// overflows the PICTURE's decimal range and is reported as a
// [BinaryRangeError].
//
// # Numeric items carry digits, not scale
//
// Every numeric accessor takes the item's digit count and returns the unscaled
// integer. A PICTURE's V and P positions occupy no storage and are not
// recoverable from the bytes, so scale is not a decoding input: PIC S9(3)V99
// COMP-3 holding -123.45 reads as -12345, and a generator emits the scale
// beside the field as a constant.
//
// The writers additionally take a [Signedness], because whether an item's
// PICTURE carries S selects the sign value stored with it and cannot be
// recovered from the value being written.
package codec
