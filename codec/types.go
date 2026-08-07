// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
)

// Charset is the character set of a data file: the mapping between the bytes
// of an alphanumeric field and the characters they spell.
//
// Charset *translation* is applied to alphanumeric and alphabetic fields and to
// nothing else. Numeric decoding operates on raw byte values, because digit
// bytes (F0-F9 in EBCDIC, 30-39 in ASCII) and overpunched sign zones are
// byte-level facts; routing them through a character translation would lose
// information and make the sign convention unrepresentable.
//
// Which charset is declared still matters to a numeric field, because it is
// what says whether that field's digits are spelled F0-F9 or 30-39 and whether
// a separate sign is 4E/60 or 2B/2D. Those byte values are compared, not
// translated — the distinction this paragraph and the one above it draw.
// See codec/SPEC.md, "Charset as a First-Class Axis".
//
// A Charset is an interface rather than an enum so that nil is detectable:
// [Encoding] must have an invalid zero value in every field. It is also the
// extension point for the code pages this package does not ship, since EBCDIC
// is not one table — cp037, cp500, cp1047 and cp1140 differ in where they put
// the bracket, currency and accent characters.
//
// ToUnicode must be total: every one of the 256 byte values must decode to
// some rune, because any byte may appear in a PIC X field and such fields are
// routinely used to carry binary payloads. FromUnicode may fail, since a
// caller can always ask to write a character the charset has no byte for.
type Charset interface {
	// Name reports the charset's name, as it would be written in
	// documentation: "ASCII", "cp037".
	Name() string

	// ToUnicode decodes one byte to the character it spells. It is total: it
	// never fails, and never reports a substitution.
	ToUnicode(b byte) rune

	// FromUnicode encodes one character as the byte that spells it,
	// reporting false when the charset has no such byte.
	FromUnicode(r rune) (byte, bool)

	// Space reports the byte that pads a short alphanumeric field: 0x20 in
	// ASCII, 0x40 in EBCDIC.
	Space() byte
}

// ASCII returns the identity character set: byte b decodes to the code point
// numbered b.
//
// Bytes 0x80-0xFF are not ASCII characters. They decode to the equally
// numbered code points rather than to a replacement character, because a
// translation table must be total (codec/SPEC.md, "Alphanumeric and Alphabetic
// Items") — a reader must not fail on an untranslatable byte in a PIC X field.
// The mapping is therefore bijective over all 256 bytes, so alphanumeric data
// round-trips through it unchanged. Use [Reader.ReadBytes] where the bytes are
// a binary payload and no character reading is wanted at all.
func ASCII() Charset { return asciiCharset{} }

type asciiCharset struct{}

func (asciiCharset) Name() string { return "ASCII" }

func (asciiCharset) ToUnicode(b byte) rune { return rune(b) }

func (asciiCharset) FromUnicode(r rune) (byte, bool) {
	if r < 0 || r > 0xFF {
		return 0, false
	}
	return byte(r), true
}

func (asciiCharset) Space() byte { return 0x20 }

// CP037 returns the EBCDIC code page 037 character set, the US/Canada page and
// the default of IBM Enterprise COBOL on z/OS.
//
// cp037 is bijective over all 256 bytes, so alphanumeric data round-trips
// through it unchanged. It is one EBCDIC page among several: cp500, cp1047 and
// cp1140 agree with it on the digits, the letters and the space 0x40 — which is
// why zoned decimal can be specified charset-generically — but differ on '[',
// ']', '!', '^', '~' and the currency symbol.
func CP037() Charset { return cp037Charset{} }

type cp037Charset struct{}

func (cp037Charset) Name() string { return "cp037" }

func (cp037Charset) ToUnicode(b byte) rune { return cp037ToUnicode[b] }

func (cp037Charset) FromUnicode(r rune) (byte, bool) {
	b, ok := cp037FromUnicode[r]
	return b, ok
}

func (cp037Charset) Space() byte { return 0x40 }

// cp037ToUnicode maps each of the 256 cp037 bytes to its code point. The C1
// control characters occupy the positions EBCDIC leaves to controls; the
// printable range is what the table is really for.
var cp037ToUnicode = [256]rune{
	0x00, 0x01, 0x02, 0x03, 0x9C, 0x09, 0x86, 0x7F, // 00-07
	0x97, 0x8D, 0x8E, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, // 08-0F
	0x10, 0x11, 0x12, 0x13, 0x9D, 0x85, 0x08, 0x87, // 10-17
	0x18, 0x19, 0x92, 0x8F, 0x1C, 0x1D, 0x1E, 0x1F, // 18-1F
	0x80, 0x81, 0x82, 0x83, 0x84, 0x0A, 0x17, 0x1B, // 20-27
	0x88, 0x89, 0x8A, 0x8B, 0x8C, 0x05, 0x06, 0x07, // 28-2F
	0x90, 0x91, 0x16, 0x93, 0x94, 0x95, 0x96, 0x04, // 30-37
	0x98, 0x99, 0x9A, 0x9B, 0x14, 0x15, 0x9E, 0x1A, // 38-3F
	' ', 0xA0, 0xE2, 0xE4, 0xE0, 0xE1, 0xE3, 0xE5, // 40-47
	0xE7, 0xF1, 0xA2, '.', '<', '(', '+', '|', // 48-4F
	'&', 0xE9, 0xEA, 0xEB, 0xE8, 0xED, 0xEE, 0xEF, // 50-57
	0xEC, 0xDF, '!', '$', '*', ')', ';', 0xAC, // 58-5F
	'-', '/', 0xC2, 0xC4, 0xC0, 0xC1, 0xC3, 0xC5, // 60-67
	0xC7, 0xD1, 0xA6, ',', '%', '_', '>', '?', // 68-6F
	0xF8, 0xC9, 0xCA, 0xCB, 0xC8, 0xCD, 0xCE, 0xCF, // 70-77
	0xCC, '`', ':', '#', '@', '\'', '=', '"', // 78-7F
	0xD8, 'a', 'b', 'c', 'd', 'e', 'f', 'g', // 80-87
	'h', 'i', 0xAB, 0xBB, 0xF0, 0xFD, 0xFE, 0xB1, // 88-8F
	0xB0, 'j', 'k', 'l', 'm', 'n', 'o', 'p', // 90-97
	'q', 'r', 0xAA, 0xBA, 0xE6, 0xB8, 0xC6, 0xA4, // 98-9F
	0xB5, '~', 's', 't', 'u', 'v', 'w', 'x', // A0-A7
	'y', 'z', 0xA1, 0xBF, 0xD0, 0xDD, 0xDE, 0xAE, // A8-AF
	'^', 0xA3, 0xA5, 0xB7, 0xA9, 0xA7, 0xB6, 0xBC, // B0-B7
	0xBD, 0xBE, '[', ']', 0xAF, 0xA8, 0xB4, 0xD7, // B8-BF
	'{', 'A', 'B', 'C', 'D', 'E', 'F', 'G', // C0-C7
	'H', 'I', 0xAD, 0xF4, 0xF6, 0xF2, 0xF3, 0xF5, // C8-CF
	'}', 'J', 'K', 'L', 'M', 'N', 'O', 'P', // D0-D7
	'Q', 'R', 0xB9, 0xFB, 0xFC, 0xF9, 0xFA, 0xFF, // D8-DF
	'\\', 0xF7, 'S', 'T', 'U', 'V', 'W', 'X', // E0-E7
	'Y', 'Z', 0xB2, 0xD4, 0xD6, 0xD2, 0xD3, 0xD5, // E8-EF
	'0', '1', '2', '3', '4', '5', '6', '7', // F0-F7
	'8', '9', 0xB3, 0xDB, 0xDC, 0xD9, 0xDA, 0x9F, // F8-FF
}

// cp037FromUnicode is the inverse of cp037ToUnicode. The table is bijective, so
// the inverse is total over its 256 code points and loses nothing.
var cp037FromUnicode = func() map[rune]byte {
	m := make(map[rune]byte, len(cp037ToUnicode))
	for b, r := range cp037ToUnicode {
		m[r] = byte(b)
	}
	return m
}()

// SignConvention is how an overpunched sign is spelled in the sign-carrying
// byte of a zoned decimal field.
//
// EBCDIC has one convention. ASCII has at least four in production use and they
// are mutually incompatible, so this cannot be modelled as a charset flag.
// Choosing wrong yields silently wrong signs — negative values read as positive
// — with no parse error, which is why it is a required [Encoding] field with an
// invalid zero value. See codec/SPEC.md, "Zoned Sign Conventions".
//
// The conventions are declared here, in the story that fixes the axes; the
// decoding they govern arrives with the zoned decimal accessors.
type SignConvention int

const (
	// SignUnset is the zero value. It is not a convention, and construction
	// fails on it.
	SignUnset SignConvention = iota
	// SignEBCDIC is the one EBCDIC convention: zone C positive, zone D
	// negative, zone F unsigned. Negative digits appear as '}' and 'J'-'R'.
	SignEBCDIC
	// SignASCIIZone37 is the Micro Focus and GnuCOBOL ASCII convention: zone
	// 3 positive, zone 7 negative. Negative digits appear as 'p'-'y'.
	SignASCIIZone37
	// SignTranslatedEBCDIC is what an EBCDIC file converted to ASCII carries:
	// the EBCDIC overpunch bytes put through a character translation, so
	// positive is 0x7B and 0x41-0x49 and negative is 0x7D and 0x4A-0x52. No
	// compiler produces it; a conversion does.
	SignTranslatedEBCDIC
	// SignRealia is the CA-Realia ASCII convention: zone 3 positive, zone 2
	// negative. Negative digits appear as a space and '!'-')'.
	SignRealia
)

// String implements the [fmt.Stringer] interface.
func (s SignConvention) String() string {
	switch s {
	case SignEBCDIC:
		return "ebcdic"
	case SignASCIIZone37:
		return "ascii-zone-3-7"
	case SignTranslatedEBCDIC:
		return "translated-ebcdic"
	case SignRealia:
		return "realia"
	case SignUnset:
		return "unset"
	}
	return "SignConvention(" + strconv.Itoa(int(s)) + ")"
}

// valid reports whether s names a convention rather than the zero value or an
// out-of-range one.
func (s SignConvention) valid() bool {
	return s >= SignEBCDIC && s <= SignRealia
}

// FloatFormat is the representation of COMP-1 and COMP-2 items.
//
// The two formats are incompatible and neither is self-describing: an IBM
// hexadecimal float holding 1.0 reads as 9.0 under IEEE with no signal at all,
// so this is a required [Encoding] field with an invalid zero value. See
// codec/SPEC.md, "Floating Point".
type FloatFormat int

const (
	// FloatUnset is the zero value. It is not a format, and construction
	// fails on it.
	FloatUnset FloatFormat = iota
	// FloatIEEE is IEEE 754 binary32 and binary64, what GnuCOBOL and Micro
	// Focus write and what IBM writes under FLOAT(NATIVE).
	FloatIEEE
	// FloatHFP is IBM hexadecimal floating point, what IBM Enterprise COBOL
	// writes by default.
	FloatHFP
)

// String implements the [fmt.Stringer] interface.
func (f FloatFormat) String() string {
	switch f {
	case FloatIEEE:
		return "ieee-754"
	case FloatHFP:
		return "ibm-hfp"
	case FloatUnset:
		return "unset"
	}
	return "FloatFormat(" + strconv.Itoa(int(f)) + ")"
}

// valid reports whether f names a format rather than the zero value or an
// out-of-range one.
func (f FloatFormat) valid() bool {
	return f >= FloatIEEE && f <= FloatHFP
}

// Encoding is the complete byte-level interpretation of a data file: the four
// independent axes that must be known before a single byte can be read.
//
// Every field is required and every field's zero value is invalid, so an
// Encoding that was never filled in cannot be mistaken for a working one.
// [Encoding.Validate] reports the first field that is missing.
//
// These are independent axes and not one dialect flag. The combination real
// files hit most often and that no compiler produces is a mainframe-written
// file converted to ASCII — ASCII characters, translated-EBCDIC signs,
// big-endian binary — which a boolean "is it EBCDIC" cannot express. The named
// bundles ([IBMEnterprise] and friends) are constructors that fill in all four,
// never defaults the package applies on its own.
type Encoding struct {
	// Charset is the character set of the file. Its translation table is
	// applied to alphanumeric fields; for zoned decimal it fixes which byte
	// values the digit zone and a separate sign take, which are compared
	// rather than translated. See [Charset]. Required; nil is invalid.
	Charset Charset
	// Sign governs how an overpunched zoned decimal sign is spelled.
	// Required; [SignUnset] is invalid.
	Sign SignConvention
	// ByteOrder governs COMP, COMP-4 and COMP-5 binary integers. Required;
	// nil is invalid. Use [binary.BigEndian], [binary.LittleEndian] or
	// [binary.NativeEndian].
	ByteOrder binary.ByteOrder
	// Float governs COMP-1 and COMP-2. Required; [FloatUnset] is invalid.
	Float FloatFormat
}

// Validate reports whether every axis has been declared, returning an
// [EncodingError] naming the first field that has not. [NewReader] and
// [NewWriter] call it, so a caller rarely needs to.
func (e Encoding) Validate() error {
	if e.Charset == nil {
		return EncodingError{Field: "Charset", Reason: "is required and has no default"}
	}
	if e.Sign == SignUnset {
		return EncodingError{Field: "Sign", Reason: "is required and has no default"}
	}
	if !e.Sign.valid() {
		return EncodingError{Field: "Sign", Reason: "has unknown value " + strconv.Itoa(int(e.Sign))}
	}
	if e.ByteOrder == nil {
		return EncodingError{Field: "ByteOrder", Reason: "is required and has no default"}
	}
	if e.Float == FloatUnset {
		return EncodingError{Field: "Float", Reason: "is required and has no default"}
	}
	if !e.Float.valid() {
		return EncodingError{Field: "Float", Reason: "has unknown value " + strconv.Itoa(int(e.Float))}
	}
	return nil
}

// IBMEnterprise returns the encoding of a file written by IBM Enterprise COBOL
// on z/OS with its defaults: cp037 EBCDIC characters, EBCDIC overpunched signs,
// big-endian binary, and IBM hexadecimal floating point.
//
// A site compiling under FLOAT(NATIVE) writes IEEE floats instead; set
// [Encoding.Float] to [FloatIEEE] on the returned value.
func IBMEnterprise() Encoding {
	return Encoding{
		Charset:   CP037(),
		Sign:      SignEBCDIC,
		ByteOrder: binary.BigEndian,
		Float:     FloatHFP,
	}
}

// MicroFocusASCII returns the encoding of a file written by Micro Focus Visual
// COBOL or COBOL Server on ASCII platforms: ASCII characters, zone 3/7 signs,
// the host's native byte order, and IEEE floats.
//
// Byte order is native because that is the Micro Focus default; a file written
// under the IBM compatibility directives is big-endian, so set
// [Encoding.ByteOrder] to [binary.BigEndian] for one of those.
func MicroFocusASCII() Encoding {
	return Encoding{
		Charset:   ASCII(),
		Sign:      SignASCIIZone37,
		ByteOrder: binary.NativeEndian,
		Float:     FloatIEEE,
	}
}

// GnuCOBOLASCII returns the encoding of a file written by GnuCOBOL with its
// default configuration: ASCII characters, zone 3/7 signs (display-sign),
// big-endian binary, and IEEE floats.
//
// Byte order follows GnuCOBOL's own binary-byteorder setting, whose default is
// big-endian. A build configured with binary-byteorder: native writes the
// host's order instead, so set [Encoding.ByteOrder] to [binary.NativeEndian]
// for one of those — it is the one axis of this bundle that a GnuCOBOL
// configuration routinely moves.
func GnuCOBOLASCII() Encoding {
	return Encoding{
		Charset:   ASCII(),
		Sign:      SignASCIIZone37,
		ByteOrder: binary.BigEndian,
		Float:     FloatIEEE,
	}
}

// ConvertedFromEBCDIC returns the encoding of a mainframe-written file that has
// since been converted to ASCII: ASCII characters, translated-EBCDIC signs,
// big-endian binary, and IBM hexadecimal floats.
//
// No compiler produces this combination — a conversion does, and it is common
// in the field. The character fields were translated and the overpunched signs
// went through the same translation, while the binary and floating-point fields
// kept whatever the mainframe wrote. It is expressible only because the four
// axes are independent.
//
// Packed decimal fields are the hazard in such a file rather than a setting:
// COMP-3 is charset-invariant, so a conversion that touched them corrupted
// them, and no encoding can recover that.
func ConvertedFromEBCDIC() Encoding {
	return Encoding{
		Charset:   ASCII(),
		Sign:      SignTranslatedEBCDIC,
		ByteOrder: binary.BigEndian,
		Float:     FloatHFP,
	}
}

// Justification is which end of a fixed-width alphanumeric field the value sits
// at, and therefore which end carries the space padding.
//
// Unlike the [Encoding] axes, this is declared per field by the copybook rather
// than per file, and its zero value is the COBOL default rather than an error:
// an item with no JUSTIFIED clause is [JustifyLeft].
type Justification int

const (
	// JustifyLeft is the default: the value sits at the left of the field and
	// the padding is on the right.
	JustifyLeft Justification = iota
	// JustifyRight is JUSTIFIED RIGHT: the value sits at the right of the
	// field and the padding is on the left.
	JustifyRight
)

// String implements the [fmt.Stringer] interface.
func (j Justification) String() string {
	switch j {
	case JustifyLeft:
		return "left"
	case JustifyRight:
		return "right"
	}
	return "Justification(" + strconv.Itoa(int(j)) + ")"
}

// Signedness is whether a numeric item carries an operational sign — the S in
// its PICTURE — and therefore which sign value is stored with it.
//
// It is a third, independent thing from the two the SPEC's naming note already
// separates. *Sign convention* ([SignConvention]) is a property of the file in
// hand; *sign position* (LEADING, TRAILING, SEPARATE) is a property of the
// copybook and applies to USAGE DISPLAY only; Signedness is a property of the
// copybook that applies to every numeric usage. `PIC S9(5) COMP-3` is signed
// and stores C or D; `PIC 9(5) COMP-3` is unsigned and stores F. See
// codec/SPEC.md, "Sign".
//
// Unlike [Justification], whose zero value is COBOL's own default, Signedness
// has an invalid zero value and every writer takes it explicitly. There is no
// safe default: writing a signed field as unsigned discards the sign of every
// negative value, and writing an unsigned field as signed puts a C where a
// consumer expects an F. Neither is recoverable from the value being written,
// so the caller states it.
type Signedness int

const (
	// SignednessUnset is the zero value. It names neither, and every writer
	// that takes a Signedness rejects it.
	SignednessUnset Signedness = iota
	// Signed is an item whose PICTURE carries S: it stores C when positive
	// and D when negative.
	Signed
	// Unsigned is an item whose PICTURE has no S: it stores F, and a
	// negative value is rejected rather than silently stored as its
	// absolute value.
	Unsigned
)

// String implements the [fmt.Stringer] interface.
func (s Signedness) String() string {
	switch s {
	case Signed:
		return "signed"
	case Unsigned:
		return "unsigned"
	case SignednessUnset:
		return "unset"
	}
	return "Signedness(" + strconv.Itoa(int(s)) + ")"
}

// valid reports whether s names a member rather than the zero value or an
// out-of-range one.
func (s Signedness) valid() bool {
	return s >= Signed && s <= Unsigned
}

// Digit counts a packed decimal accessor accepts. The limit is the range of
// the Go type the accessor returns and not a property of the field: 9 digits
// is the most that always fits an int32 and 18 the most that always fits an
// int64, which is the same digit-count staircase COBOL uses to pick a binary
// width. 31 is the IBM Enterprise COBOL maximum for a packed item, reachable
// through [Reader.ReadPackedBig] and [Writer.WritePackedBig].
const (
	maxPackedInt32Digits = 9
	maxPackedInt64Digits = 18
	maxPackedDigits      = 31
)

// Packed decimal sign nibbles. A writer emits only these three; a reader also
// accepts packedSignA and packedSignE as positive and packedSignB as negative,
// which is what z/Architecture decimal instructions do. See codec/SPEC.md,
// "Sign nibble".
const (
	packedSignPositive byte = 0x0C
	packedSignNegative byte = 0x0D
	packedSignUnsigned byte = 0x0F
)

// packedWidth reports the byte width of a packed decimal field holding digits
// digits: ceil((digits + 1) / 2), one nibble per digit plus the sign nibble,
// rounded up to a whole byte. It is the whole of the packed size model, and it
// does not depend on scale.
func packedWidth(digits int) int { return (digits + 2) / 2 }

// packedSignIsNegative reports whether a sign nibble means the value is
// negative, rejecting the nibbles 0-9 that mean nothing at all.
//
// The accepted set is wider than the emitted one: A, C, E and F are positive
// and B and D are negative, because z/Architecture decimal instructions accept
// more sign values than they generate and real files carry them.
func packedSignIsNegative(nibble byte) (bool, error) {
	switch nibble {
	case 0x0A, 0x0C, 0x0E, 0x0F:
		return false, nil
	case 0x0B, 0x0D:
		return true, nil
	}
	return false, PackedSignError{Nibble: nibble}
}

// Truncation is the range semantics of a binary (COMP, COMP-4, BINARY,
// COMP-5) item: what the digit count in its PICTURE says about the values the
// item may hold.
//
// It is the IBM TRUNC compiler option, spelled binary-truncate by GnuCOBOL, and
// it is the one binary setting that is a property of the *compiler* rather than
// of the file in hand — which is why it is not an [Encoding] axis. Byte order
// is: the bytes were written by whatever wrote them and are read by something
// else, so it must be declared per file.
//
// Truncation is never passed to an accessor. It is selected by which accessor
// is called, because the two readings are the two families of methods on
// [Reader] and [Writer]:
//
//   - [Reader.ReadBinaryInt16] and friends are TRUNC(STD), the
//     standard-conforming reading: a PIC S9(4) COMP item holds -9999 to 9999
//     and a value outside that range is a [BinaryRangeError].
//   - [Reader.ReadComp5Int16] and friends are TRUNC(BIN): the same two bytes,
//     the full -32768 to 32767 range of the storage. This is what USAGE COMP-5
//     always means and what COMP and COMP-4 mean under TRUNC(BIN) or
//     binary-truncate: no.
//
// Truncation does not change the byte layout, so it never changes how a field
// is decoded — only which decoded values are accepted. That validation is worth
// having: under TRUNC(STD) it is the strongest available detector of a wrong
// [Encoding.ByteOrder], since a byte-swapped value usually overflows the
// PICTURE's decimal range. See codec/SPEC.md, "Range semantics: TRUNC".
type Truncation int

const (
	// TruncUnset is the zero value. It names no semantics; it appears only in
	// a zero-valued [BinaryRangeError], since no accessor takes a Truncation.
	TruncUnset Truncation = iota
	// TruncStd is TRUNC(STD), binary-truncate: yes: the item's range is the
	// decimal range of its PICTURE digit count.
	TruncStd
	// TruncBin is TRUNC(BIN), binary-truncate: no, and USAGE COMP-5: the
	// item's range is the full range of its storage width.
	TruncBin
)

// String implements the [fmt.Stringer] interface.
func (t Truncation) String() string {
	switch t {
	case TruncStd:
		return "trunc-std"
	case TruncBin:
		return "trunc-bin"
	case TruncUnset:
		return "unset"
	}
	return "Truncation(" + strconv.Itoa(int(t)) + ")"
}

// Digit counts a binary accessor accepts. As with the packed accessors the
// limit is the range of the Go type the accessor returns and not a property of
// the field: 4 digits is the most a 2-byte item holds, and a 2-byte item under
// TRUNC(BIN) is the most that always fits an int16. 31 is the IBM Enterprise
// COBOL maximum under ARITH(EXTEND), reachable through [Reader.ReadBinaryBig]
// and [Writer.WriteBinaryBig].
const (
	maxBinaryInt16Digits = 4
	maxBinaryInt32Digits = 9
	maxBinaryInt64Digits = 18
	maxBinaryDigits      = 31
)

// binaryWidth reports the byte width of a binary field holding digits digits:
// 2 bytes through 4 digits, 4 through 9, 8 through 18 and 16 beyond. It is the
// whole of the binary size model, and like [packedWidth] it does not depend on
// scale.
//
// The width is a staircase and not the digit count: PIC 9(5) COMP is *four*
// bytes, not five. Getting the step wrong shifts every field after it in the
// record, which is why the 4/5 and 9/10 boundaries carry their own tests.
//
// This is the IBM Enterprise COBOL table, which is also Micro Focus's and
// GnuCOBOL's binary-size: 2-4-8. A GnuCOBOL build left on its default
// binary-size: 1-2-4-8 gives a 1-2 digit item **one** byte instead of two;
// this package does not implement that variant, and a copybook compiled under
// it desynchronizes at the first such field rather than reading wrongly. See
// codec/SPEC.md, "Binary widths by digit count".
func binaryWidth(digits int) int {
	switch {
	case digits <= 4:
		return 2
	case digits <= 9:
		return 4
	case digits <= 18:
		return 8
	}
	return 16
}

// pow10 holds 10^i for every digit count the fixed-width binary accessors
// reach. 10^18 is the largest power of ten that fits a uint64, which is why
// those accessors stop at 18 digits and the [math/big.Int] ones compute their
// own bound.
var pow10 = func() [maxBinaryInt64Digits + 1]uint64 {
	var t [maxBinaryInt64Digits + 1]uint64
	t[0] = 1
	for i := 1; i < len(t); i++ {
		t[i] = t[i-1] * 10
	}
	return t
}()

// decimalLimit reports 10^digits - 1, the largest magnitude a digits-digit
// item holds under [TruncStd].
func decimalLimit(digits int) *big.Int {
	ten := big.NewInt(10)
	v := new(big.Int).Exp(ten, big.NewInt(int64(digits)), nil)
	return v.Sub(v, big.NewInt(1))
}

// isBigEndian reports whether bo puts the most significant byte first.
//
// The order is probed rather than compared against [binary.BigEndian], because
// [binary.NativeEndian] is a distinct type from both of the named orders and a
// caller may supply an implementation of its own.
func isBigEndian(bo binary.ByteOrder) bool {
	return bo.Uint16([]byte{0x01, 0x00}) == 0x0100
}

// orderBinaryBytes converts b between big-endian order and the order bo
// declares, in place. It is its own inverse — the conversion is a reversal in
// both directions — so the reader and the writer share it.
//
// It exists because [binary.ByteOrder] has no 16-byte accessor: the 19-to-31
// digit fields the [math/big.Int] accessors read and write cannot go through
// Uint64 or PutUint64 the way the narrower ones do.
func orderBinaryBytes(bo binary.ByteOrder, b []byte) {
	if isBigEndian(bo) {
		return
	}
	slices.Reverse(b)
}

// putBinaryUint writes the low 8*len(field) bits of raw into field in the
// order bo declares. field must be 2, 4 or 8 bytes; the 16-byte case belongs
// to the [math/big.Int] writers and goes through [orderBinaryBytes].
func putBinaryUint(bo binary.ByteOrder, field []byte, raw uint64) {
	switch len(field) {
	case 2:
		bo.PutUint16(field, uint16(raw))
	case 4:
		bo.PutUint32(field, uint32(raw))
	default:
		bo.PutUint64(field, raw)
	}
}

// signExtend reinterprets the low 8*width bits of raw as a two's complement
// signed integer, which is what a signed binary item stores over the whole of
// its storage width.
func signExtend(raw uint64, width int) int64 {
	shift := uint(64 - 8*width)
	return int64(raw<<shift) >> shift
}

// Marshaler is implemented by a value that can write itself to a data file.
// Generated record types implement it; the [Writer] it is handed already knows
// the [Encoding], so an implementation never chooses one.
type Marshaler interface {
	MarshalCOBOL(w *Writer) error
}

// Unmarshaler is implemented by a value that can read itself from a data file.
// It is the inverse of [Marshaler], and the [Reader] it is handed likewise
// already knows the [Encoding].
type Unmarshaler interface {
	UnmarshalCOBOL(r *Reader) error
}

// ErrNilReader is returned by [NewReader] when the underlying reader is nil.
var ErrNilReader = errors.New("nil reader")

// ErrNilWriter is returned by [NewWriter] when the underlying writer is nil.
var ErrNilWriter = errors.New("nil writer")

// ErrNilValue is returned by [Marshal] and [Unmarshal] when the value is nil.
var ErrNilValue = errors.New("nil value")

// OffsetError wraps every error a [Reader] or [Writer] returns after
// construction, recording the byte offset at which the failure was detected so
// that a bad byte deep inside a record is diagnosable.
//
// It is a wrapper rather than a field on each error so that the offset is
// stamped in one place and cannot drift. Callers reach past it with errors.Is
// for the underlying cause and errors.As for the offset:
//
//	var oe *codec.OffsetError
//	if errors.As(err, &oe) {
//	        log.Printf("bad byte at offset %d", oe.Offset)
//	}
type OffsetError struct {
	// Offset is the byte position at which the failure was detected,
	// counted from the start of the stream.
	Offset int64
	// Err is the underlying cause.
	Err error
}

// Error implements the [error] interface.
func (e *OffsetError) Error() string {
	return fmt.Sprintf("at byte offset %d: %v", e.Offset, e.Err)
}

// Unwrap implements the interface [errors.Unwrap] expects.
func (e *OffsetError) Unwrap() error { return e.Err }

// EncodingError is returned by [Encoding.Validate], and therefore by
// [NewReader] and [NewWriter], when an axis of the encoding was left undeclared
// or set to a value that names no member.
type EncodingError struct {
	// Field is the name of the offending [Encoding] field.
	Field string
	// Reason says what is wrong with it.
	Reason string
}

// Error implements the [error] interface.
func (e EncodingError) Error() string {
	return fmt.Sprintf("invalid encoding: field %s %s", e.Field, e.Reason)
}

// FieldWidthError is returned when a field width is negative. A width of zero
// is legal and reads or writes nothing.
type FieldWidthError struct {
	// Width is the width that was asked for.
	Width int
}

// Error implements the [error] interface.
func (e FieldWidthError) Error() string {
	return fmt.Sprintf("invalid field width %d: must not be negative", e.Width)
}

// FieldTooLongError is returned when a value does not fit the field it was
// asked to be written into.
//
// Writing truncates nothing: a COBOL MOVE into a short item silently drops
// characters, but a codec doing the same would write a record that no longer
// says what the caller asked it to, so this is loud.
type FieldTooLongError struct {
	// Len is the encoded length of the value, in bytes.
	Len int
	// Width is the width of the field, in bytes.
	Width int
}

// Error implements the [error] interface.
func (e FieldTooLongError) Error() string {
	return fmt.Sprintf("value of %d bytes does not fit a field of %d", e.Len, e.Width)
}

// UnrepresentableRuneError is returned when a character has no byte in the
// declared charset. It has no read-side counterpart: decoding is total, because
// any byte may appear in a PIC X field.
type UnrepresentableRuneError struct {
	// Rune is the character that could not be encoded.
	Rune rune
	// Charset is the name of the charset that has no byte for it.
	Charset string
}

// Error implements the [error] interface.
func (e UnrepresentableRuneError) Error() string {
	return fmt.Sprintf("character %q has no representation in %s", e.Rune, e.Charset)
}

// JustificationError is returned when a [Justification] names no member.
type JustificationError struct {
	// Justification is the value that was passed.
	Justification Justification
}

// Error implements the [error] interface.
func (e JustificationError) Error() string {
	return fmt.Sprintf("invalid justification %d", int(e.Justification))
}

// SignednessError is returned when a [Signedness] names no member, which
// includes the zero value: a writer of a numeric item will not guess whether
// the item's PICTURE carries S.
type SignednessError struct {
	// Signedness is the value that was passed.
	Signedness Signedness
}

// Error implements the [error] interface.
func (e SignednessError) Error() string {
	return fmt.Sprintf("invalid signedness %d: an item is either Signed or Unsigned", int(e.Signedness))
}

// PackedDigitCountError is returned when a packed decimal digit count is
// outside the range the accessor accepts.
//
// The upper bound belongs to the accessor rather than to the field: 9 digits
// for the int32 accessors, 18 for the int64 ones and 31 — the IBM Enterprise
// COBOL maximum — for the [math/big.Int] ones. A field wider than the accessor
// asked for is a call that would have silently overflowed.
type PackedDigitCountError struct {
	// Digits is the digit count that was asked for.
	Digits int
	// Max is the largest digit count this accessor accepts.
	Max int
}

// Error implements the [error] interface.
func (e PackedDigitCountError) Error() string {
	return fmt.Sprintf("invalid packed decimal digit count %d: must be between 1 and %d", e.Digits, e.Max)
}

// PackedPadError is returned when the pad nibble of a packed decimal field is
// not zero.
//
// The pad nibble is the high nibble of the first byte, and it exists only when
// the digit count is even. A non-zero pad is the cheapest available signal
// that the field offset is wrong — that the copybook being read does not
// describe the file in hand — which is why it is validated rather than
// skipped.
type PackedPadError struct {
	// Nibble is the pad nibble that was found, which is between 0x1 and 0xF.
	Nibble byte
}

// Error implements the [error] interface.
func (e PackedPadError) Error() string {
	return fmt.Sprintf("packed decimal pad nibble is %X, not 0", e.Nibble)
}

// PackedDigitError is returned when a digit nibble of a packed decimal field
// holds a value above 9, which no writer produces and which denotes no digit.
type PackedDigitError struct {
	// Nibble is the offending nibble, which is between 0xA and 0xF.
	Nibble byte
}

// Error implements the [error] interface.
func (e PackedDigitError) Error() string {
	return fmt.Sprintf("invalid packed decimal digit nibble %X: digit nibbles are 0-9", e.Nibble)
}

// PackedSignError is returned when the sign nibble of a packed decimal field
// is one of 0-9, none of which names a sign.
//
// A, B, C, D, E and F all name one and are all accepted on read, so this
// rejects exactly the nibbles that cannot have come from a packed field.
type PackedSignError struct {
	// Nibble is the offending nibble, which is between 0x0 and 0x9.
	Nibble byte
}

// Error implements the [error] interface.
func (e PackedSignError) Error() string {
	return fmt.Sprintf("invalid packed decimal sign nibble %X: sign nibbles are A-F", e.Nibble)
}

// PackedRangeError is returned when a value cannot be written into the packed
// decimal field it was given: it has more digits than the field holds, or it is
// negative and the field is [Unsigned].
//
// Both are loud for the reason [FieldTooLongError] is. A COBOL MOVE truncates
// high-order digits and stores a negative value into an unsigned item as its
// absolute value; a codec doing either would write a record that no longer says
// what the caller asked it to.
type PackedRangeError struct {
	// Value is the decimal spelling of the value that did not fit.
	Value string
	// Digits is the number of digits the field holds.
	Digits int
	// Signedness is whether the field carries a sign.
	Signedness Signedness
}

// Error implements the [error] interface.
func (e PackedRangeError) Error() string {
	return fmt.Sprintf("value %s does not fit a %d-digit %s packed decimal field", e.Value, e.Digits, e.Signedness)
}

// BinaryDigitCountError is returned when a binary decimal digit count is
// outside the range the accessor accepts.
//
// The upper bound belongs to the accessor rather than to the field, exactly as
// it does for [PackedDigitCountError]: 4 digits for the int16 accessors, 9 for
// the int32 ones, 18 for the int64 and uint64 ones and 31 — the IBM Enterprise
// COBOL maximum under ARITH(EXTEND) — for the [math/big.Int] ones. The bound is
// what the accessor's return type always holds over the *full* storage width,
// so that a COMP-5 field cannot silently overflow it.
type BinaryDigitCountError struct {
	// Digits is the digit count that was asked for.
	Digits int
	// Max is the largest digit count this accessor accepts.
	Max int
}

// Error implements the [error] interface.
func (e BinaryDigitCountError) Error() string {
	return fmt.Sprintf("invalid binary digit count %d: must be between 1 and %d", e.Digits, e.Max)
}

// BinaryRangeError is returned when a binary value lies outside the range of
// the field it was given.
//
// It arises in both directions, which is not true of [PackedRangeError]:
//
//   - On write, the value does not fit — it has more digits than the PICTURE
//     allows under [TruncStd], it exceeds the storage width under [TruncBin],
//     or it is negative and the field is [Unsigned].
//   - On read under [TruncStd], the *stored* value exceeds the decimal range
//     the PICTURE can express. That is a real signal rather than pedantry: it
//     is the strongest available evidence that [Encoding.ByteOrder] is wrong,
//     since a byte-swapped small number is usually a very large one. A file
//     that legitimately carries such values is a COMP-5 or TRUNC(BIN) file and
//     is read with the [Reader.ReadComp5Int16] family instead.
//
// The offset it is wrapped in is the offset the field *starts* at rather than
// the one it ends at, for the reason a packed nibble error carries the byte
// holding the nibble: a binary field is several bytes wide, and a range error
// is a statement about the whole field.
type BinaryRangeError struct {
	// Value is the decimal spelling of the value that did not fit.
	Value string
	// Digits is the number of digits the field's PICTURE declares.
	Digits int
	// Width is the storage width of the field, in bytes.
	Width int
	// Signedness is whether the field carries a sign.
	Signedness Signedness
	// Truncation is the range semantics the accessor applied.
	Truncation Truncation
}

// Error implements the [error] interface.
func (e BinaryRangeError) Error() string {
	return fmt.Sprintf(
		"value %s does not fit a %d-digit %s binary field of %d bytes under %s",
		e.Value, e.Digits, e.Signedness, e.Width, e.Truncation,
	)
}
