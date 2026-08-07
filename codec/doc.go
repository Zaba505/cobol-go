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
// packed decimal (COMP-3), binary (COMP, COMP-4, BINARY, COMP-5) and floating
// point (COMP-1, COMP-2). Zoned decimal accessors, and the charset axis beyond
// [ASCII] and [CP037], arrive in later stories; the [Encoding] axes they need
// are already declared here so that no field of it ever has to acquire a
// default retroactively.
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
// # Floating point: the sharpest fork in the package
//
// COMP-1 is four bytes and COMP-2 eight, and neither takes a PICTURE — the
// usage alone fixes the format — so [Reader.ReadFloat32] and
// [Reader.ReadFloat64] take no digit count and their writers take no
// [Signedness] either.
//
// What those bytes mean comes entirely from [Encoding.Float]: IEEE 754 on the
// distributed platforms, IBM hexadecimal floating point on z/OS. Neither format
// can detect the other, because every bit pattern is valid in both. IEEE 1.0 is
// 3F 80 00 00, which reads as 0.03125 under HFP; HFP 1.0 is 41 10 00 00, which
// reads as 9.0 under IEEE. Not an error, not a NaN, not out of range — a
// plausible number that passes every sanity check downstream, and the cleanest
// illustration in this package of why the axes are caller-declared.
//
// HFP is big-endian and [Encoding.ByteOrder] does not apply to it: the format
// predates any little-endian IBM platform and has no little-endian form. Under
// IEEE the axis applies as it does to a binary item. The two axes correlate in
// practice and are declared separately because Enterprise COBOL 6's
// FLOAT(NATIVE) writes IEEE into an otherwise thoroughly EBCDIC file.
//
// HFP has no NaN, no infinity and no negative zero, and its magnitude range,
// 16^-65 to 16^63, is neither a subset nor a superset of a float64's. Writing
// something it cannot represent, or reading a COMP-1 field whose value no
// float32 expresses, is a [FloatRangeError] rather than an infinity or a zero
// that would read back as an ordinary number.
//
// # Numeric items carry digits, not scale
//
// Every fixed-point numeric accessor takes the item's digit count and returns
// the unscaled integer. A PICTURE's V and P positions occupy no storage and are
// not recoverable from the bytes, so scale is not a decoding input: PIC
// S9(3)V99 COMP-3 holding -123.45 reads as -12345, and a generator emits the
// scale beside the field as a constant.
//
// The writers additionally take a [Signedness], because whether an item's
// PICTURE carries S selects the sign value stored with it and cannot be
// recovered from the value being written.
//
// Floating point is the exception to both, and to the width model above: a
// COMP-1 or COMP-2 item has no PICTURE to carry digits, a scale, or an S.
package codec
