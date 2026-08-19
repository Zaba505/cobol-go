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
// Five settings must be known before a single byte of a data file can be
// interpreted: the character set, the zoned-decimal sign convention, the byte
// order of binary items, the floating-point format, and the width staircase
// binary items were compiled under. None of them is recoverable from the file
// with certainty, and every one of them fails silently when wrong — the wrong
// value yields a plausible but incorrect number rather than an error, and the
// last of them yields a plausible but incorrect *record*, since it decides how
// many bytes a binary field occupies.
//
// So this package has no default for any of them and no usable zero-value
// [Reader]. [Encoding] carries all five, every field of it has an invalid zero
// value, and every constructor — [NewReader], [NewBytesReader], [NewWriter],
// [NewBytesWriter] — fails with an [EncodingError] naming the field that was
// left out. See codec/SPEC.md, "The Five Axes of an
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
// # A stream, or bytes the caller already holds
//
// A [Reader] reads either an [io.Reader] or a []byte, and a [Writer] writes
// either an [io.Writer] or a []byte it appends to. The byte-backed pair —
// [NewBytesReader] and [NewBytesWriter] — is what [Unmarshal] and [Marshal]
// are built on, since a caller stepping through a data file has each record's
// bytes in hand already and wrapping them in a reader is an allocation for
// nothing.
//
// [Reader.Reset] and [Writer.Reset] rewind one onto the next record. Everything
// the [Encoding] derived survives — the zoned decimal byte tables, the
// alphanumeric translation table, the scratch buffers every field is read into,
// and the writer's buffer at its capacity — so a codec kept for a file, or
// pooled across a fleet of them, pays for those once rather than once per
// record. The [Encoding] itself cannot change; a different one needs a
// different [Reader].
//
// Both hold the caller's slice rather than copying it, until the next Reset and
// no longer. Nothing a read returns views it — every accessor decodes through
// the [Reader]'s own scratch — so values from earlier records survive both the
// next Reset and a later write into the caller's buffer.
//
// # Scope of this package as it stands
//
// [Reader] and [Writer] currently cover alphanumeric fields, raw byte fields,
// zoned decimal (USAGE DISPLAY), packed decimal (COMP-3 and COMP-6), binary
// (COMP, COMP-4, BINARY, COMP-5) and floating point (COMP-1, COMP-2).
//
// USAGE is a property of each item and not a mode of the file, so those
// families coexist inside one record: a record holding a DISPLAY field, a
// COMP-3 field and a COMP field is ordinary, and each field's width comes from
// its own usage.
//
// What remains out of scope is numeric-edited de-editing — a PICTURE carrying
// an actual '.', a currency sign or insertion characters, whose bytes are a
// presentation of a number rather than the number. See codec/SPEC.md,
// "Numeric-edited de-editing".
//
// # Zoned decimal: two independent facts about the sign
//
// A USAGE DISPLAY item spends one character byte per digit, and its sign — if
// its PICTURE carries S — is normally *overpunched* into the zone nibble of a
// digit byte rather than given a byte of its own. Reading one takes two facts
// that come from different places and are named apart for that reason:
//
//   - [SignPosition] comes from the copybook: whether there is an S at all, and
//     what the SIGN clause says. It is passed to every zoned accessor, it is
//     what makes a SEPARATE field digits+1 bytes wide, and it takes the place
//     of the [Signedness] the other numeric writers require.
//   - [SignConvention] comes from the file, through [Encoding.Sign], and says
//     how the sign-carrying byte is spelled.
//
// Neither is recoverable from the bytes and neither implies the other, so
// neither has a default: [SignPositionUnset] and [SignUnset] are both invalid.
//
// # Character sets and sign conventions
//
// [Charset] is an interface rather than an enum, and [ASCII] and [CP037] are
// the two tables that ship. Neither is a default and neither is special: cp500,
// cp1047 and cp1140 are a caller's own implementation away — over
// golang.org/x/text/encoding/charmap, say — and this package depends on nothing
// outside the standard library to keep that a choice rather than an
// inheritance.
//
// Charset translation applies to alphanumeric fields and to nothing else.
// Numeric decoding compares raw byte values, because digit bytes (F0-F9 against
// 30-39) and the overpunched sign zones are byte-level facts: translating one
// would throw away the zone that carries the sign. The declared charset still
// says what those bytes are — whether a digit is F5 or 35, and whether a
// SIGN SEPARATE byte is 4E/60 or 2B/2D — and this package asks the [Charset]
// for them rather than switching on a known page.
//
// [Encoding.Sign] is the second, independent axis, and EBCDIC and ASCII are not
// symmetric on it: EBCDIC has one universal convention while ASCII has four
// incompatible ones in production use, which is why [SignConvention] is a
// selectable value with no default and no inference. Bytes invalid under the
// declared convention are rejected rather than coerced — a sign byte of 7B read
// under [SignASCIIZone37] is a [ZonedSignError], not a digit — and since the
// four conventions are mutually detectable at that byte, the rejection is what
// turns most wrong-convention mistakes into a loud failure at the first
// negative value rather than into silently flipped signs.
//
// # Packed decimal: COMP-3 and COMP-6 are not one family
//
// COMP-3 spends a nibble per digit plus a trailing sign nibble, so a field is
// ceil((digits+1)/2) bytes wide and its pad nibble appears when the digit count
// is even. COMP-6, the GnuCOBOL and Micro Focus extension, stores no sign at
// all: ceil(digits/2) bytes, every nibble a digit, and the pad nibble on the
// opposite parity. PIC 9(4) COMP-3 is three bytes and PIC 9(4) COMP-6 is two.
//
// The two widths coincide at every odd digit count and differ only at even
// ones, so a copybook that has the usage wrong shifts the record half the time
// and is otherwise caught only by the nibbles — which is what the pad check and
// the digit check are for.
//
// They are therefore separate accessors — [Reader.ReadComp6Int32] and its
// family beside [Reader.ReadPackedInt32] and its — and not one accessor with a
// flag. The COMP-6 writers take no [Signedness] because the encoding has
// nowhere to put one, and a negative value is a [PackedRangeError] rather than
// something stored as its magnitude. On the reading side no nibble of a COMP-6
// field may be A-F, which is what makes a COMP-3 field read at a COMP-6 offset
// fail loudly instead of yielding a plausible number.
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
// recovered from the value being written. The zoned accessors are the exception
// and are stricter rather than looser: their [SignPosition] already says
// whether there is an S, so it is required on the *reading* side too, where it
// also fixes the field's width.
//
// Floating point is the exception to both, and to the width model above: a
// COMP-1 or COMP-2 item has no PICTURE to carry digits, a scale, or an S.
package codec
