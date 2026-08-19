// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Reader reads the fields of a COBOL data file from an [io.Reader], one field
// at a time and in record order.
//
// There is deliberately no usable zero value: a Reader is only obtainable from
// [NewReader], which requires a complete [Encoding]. A Reader is not safe for
// concurrent use — the read position is state.
type Reader struct {
	r   io.Reader
	enc Encoding
	off int64
	// zoned is the encoding's zoned decimal byte table, derived once rather
	// than per field. zonedErr holds the failure of deriving it, which is
	// reported by the first zoned accessor and by nothing else: a charset
	// that cannot spell a digit or a '+' still reads alphanumeric fields
	// perfectly well, so it is not a reason to refuse a Reader.
	zoned    zonedCodec
	zonedErr error
}

// NewReader returns a [Reader] that reads from r under the given encoding.
//
// enc must declare all four axes; construction fails with an [EncodingError]
// naming the first field that does not. There is no default for any of them,
// because each fails silently when wrong. The named bundles — [IBMEnterprise],
// [MicroFocusASCII], [GnuCOBOLASCII], [ConvertedFromEBCDIC] — expand to a
// complete encoding in one call.
func NewReader(r io.Reader, enc Encoding) (*Reader, error) {
	if r == nil {
		return nil, ErrNilReader
	}
	if err := enc.Validate(); err != nil {
		return nil, err
	}
	zoned, zonedErr := newZonedCodec(enc)
	return &Reader{r: r, enc: enc, zoned: zoned, zonedErr: zonedErr}, nil
}

// Encoding reports the encoding the [Reader] was constructed with.
func (r *Reader) Encoding() Encoding { return r.enc }

// Offset reports how many bytes have been consumed, which is the position of
// the next field in the stream. Bytes consumed by a failed read are counted, so
// the offset after an error is where reading stopped.
func (r *Reader) Offset() int64 { return r.off }

// read consumes exactly n bytes. It is the single place the offset advances and
// the single place read errors are stamped with it.
func (r *Reader) read(n int) ([]byte, error) {
	if n < 0 {
		return nil, &OffsetError{Offset: r.off, Err: FieldWidthError{Width: n}}
	}
	if n == 0 {
		return []byte{}, nil
	}
	buf := make([]byte, n)
	got, err := io.ReadFull(r.r, buf)
	r.off += int64(got)
	if err != nil {
		return nil, &OffsetError{Offset: r.off, Err: err}
	}
	return buf, nil
}

// ReadBytes reads the next n bytes as they stand, applying no character
// translation and stripping no padding.
//
// This is the raw accessor alongside [Reader.ReadAlphanumeric], for the PIC X
// field that carries a binary payload rather than characters, and for any
// caller that needs the bytes a trimmed string cannot reproduce.
//
// The returned slice is the caller's own. It is allocated by the read that
// returns it, and the Reader keeps no reference to it and never writes into it
// again, so it may be retained, modified and handed on without being copied
// first. No accessor of a Reader reuses a buffer, which is what makes that
// safe to depend on rather than an accident of the current implementation.
//
// A short stream is an error: io.EOF when nothing at all was left, and
// [io.ErrUnexpectedEOF] when the field was cut short, both wrapped in an
// [OffsetError]. Reading at the end of a file therefore reports io.EOF, which
// is how a caller stepping through records detects the end of one.
func (r *Reader) ReadBytes(n int) ([]byte, error) {
	return r.read(n)
}

// ReadAlphanumeric reads the next n bytes as an alphanumeric (PIC X) or
// alphabetic (PIC A) field, translating them through the encoding's charset and
// stripping the trailing space padding.
//
// It is [Reader.ReadAlphanumericJustified] with [JustifyLeft], the COBOL
// default for an item with no JUSTIFIED clause.
//
// Trimming is a policy decision this package makes and not a property of the
// data: trailing spaces in a fixed-width field are indistinguishable from
// content, and stripping them is what makes a field read back as the value that
// was written into it. A caller that needs the padding preserved should use
// [Reader.ReadBytes].
//
// Decoding never fails on the bytes themselves. Any byte value may appear in an
// alphanumeric field, so the charset translation is total and an untranslatable
// byte is impossible by construction.
func (r *Reader) ReadAlphanumeric(n int) (string, error) {
	return r.ReadAlphanumericJustified(n, JustifyLeft)
}

// ReadAlphanumericJustified reads the next n bytes as an alphanumeric field
// whose value sits at the end of the field j names, stripping the padding from
// the other end: trailing spaces under [JustifyLeft], leading spaces under
// [JustifyRight], which is the JUSTIFIED RIGHT clause.
//
// The space stripped is the charset's own — 0x20 in ASCII, 0x40 in EBCDIC —
// since translation happens first.
func (r *Reader) ReadAlphanumericJustified(n int, j Justification) (string, error) {
	if j != JustifyLeft && j != JustifyRight {
		return "", &OffsetError{Offset: r.off, Err: JustificationError{Justification: j}}
	}
	b, err := r.read(n)
	if err != nil {
		return "", err
	}
	charset := r.enc.Charset
	var sb strings.Builder
	sb.Grow(n)
	for _, c := range b {
		sb.WriteRune(charset.ToUnicode(c))
	}
	if j == JustifyRight {
		return strings.TrimLeft(sb.String(), " "), nil
	}
	return strings.TrimRight(sb.String(), " "), nil
}

// ReadZonedInt32 reads the next zoned decimal (USAGE DISPLAY) field of digits
// digits as an int32, consuming digits bytes — or digits+1 when s is a
// SEPARATE position, which is the one thing that changes the width.
//
// Zoned decimal is the default USAGE and the most common numeric encoding in
// interchange files: one character byte per digit, most significant first. It
// is a per-field usage and not a file-level mode, so a record routinely mixes
// it with COMP-3, COMP and COMP-1 fields.
//
// s is required and has no default. It says whether the item's PICTURE carries
// S and where the SIGN clause put the sign, neither of which is recoverable
// from the bytes — see [SignPosition].
//
// digits must be between 1 and 9, the most that always fits an int32; a wider
// field is a [ZonedDigitCountError] rather than a silent overflow, and is read
// with [Reader.ReadZonedInt64] or [Reader.ReadZonedBig].
//
// Every byte is validated against the declared [Encoding.Charset] and
// [Encoding.Sign]: a digit byte that is not one is a [ZonedDigitError], a sign
// byte invalid under the convention is a [ZonedSignError], and a SEPARATE sign
// byte that is neither '+' nor '-' is a [ZonedSeparateSignError]. All three
// carry the offset of the *byte* at fault rather than the end of the field.
//
// The return is the unscaled integer. A PICTURE's V occupies no byte and is not
// recoverable from the data, so scale is not a decoding input: PIC S9(3)V99
// holding -123.45 is five bytes reading as -12345. A PICTURE containing an
// actual decimal point is numeric-edited rather than DISPLAY numeric and is out
// of scope here; see codec/SPEC.md, "Numeric-edited de-editing".
func (r *Reader) ReadZonedInt32(digits int, s SignPosition) (int32, error) {
	v, err := r.readZonedInt(digits, maxZonedInt32Digits, s)
	return int32(v), err
}

// ReadZonedInt64 reads the next zoned decimal field of digits digits as an
// int64, consuming digits bytes, or digits+1 under a SEPARATE sign position.
//
// digits must be between 1 and 18, the most that always fits an int64. The 19
// to 31 digit range an IBM item may declare is read with
// [Reader.ReadZonedBig]. s is required; see [Reader.ReadZonedInt32], which also
// says why the return is the unscaled integer.
func (r *Reader) ReadZonedInt64(digits int, s SignPosition) (int64, error) {
	return r.readZonedInt(digits, maxZonedInt64Digits, s)
}

// ReadZonedBig reads the next zoned decimal field of digits digits as a
// [math/big.Int], consuming digits bytes, or digits+1 under a SEPARATE sign
// position.
//
// digits must be between 1 and 31, the IBM Enterprise COBOL maximum. This is
// the accessor for the 19-to-31 digit range no Go integer type holds; below 19
// digits the int32 and int64 accessors say the same thing without allocating.
// s is required; see [Reader.ReadZonedInt32].
func (r *Reader) ReadZonedBig(digits int, s SignPosition) (*big.Int, error) {
	ds, negative, err := r.readZonedDigits(digits, maxZonedDigits, s)
	if err != nil {
		return nil, err
	}
	v := new(big.Int)
	ten := big.NewInt(10)
	d := new(big.Int)
	for _, n := range ds {
		v.Mul(v, ten)
		v.Add(v, d.SetInt64(int64(n)))
	}
	if negative {
		v.Neg(v)
	}
	return v, nil
}

// readZonedInt is the shared body of the two integer zoned accessors, whose
// only difference is the digit count they accept.
func (r *Reader) readZonedInt(digits, max int, s SignPosition) (int64, error) {
	ds, negative, err := r.readZonedDigits(digits, max, s)
	if err != nil {
		return 0, err
	}
	var v int64
	for _, d := range ds {
		v = v*10 + int64(d)
	}
	if negative {
		v = -v
	}
	return v, nil
}

// readZonedDigits reads one zoned decimal field and returns its digits, most
// significant first and one per element with values 0-9, together with whether
// the sign made it negative.
//
// It is the layer that turns a [SignPosition] into a width and an index: the
// separate sign byte, when there is one, is split off here, and everything left
// is the digit bytes [zonedCodec.decodeField] validates.
//
// Every error is stamped with the offset of the byte at fault rather than the
// offset the field ended at, for the reason [Reader.readPackedDigits] stamps
// the byte holding a bad nibble: a zoned field is several bytes wide, and "the
// field ended at offset N" does not say which byte was wrong.
func (r *Reader) readZonedDigits(digits, max int, s SignPosition) ([]byte, bool, error) {
	if !s.valid() {
		return nil, false, &OffsetError{Offset: r.off, Err: SignPositionError{SignPosition: s}}
	}
	if digits < 1 || digits > max {
		return nil, false, &OffsetError{
			Offset: r.off,
			Err:    ZonedDigitCountError{Digits: digits, Max: max},
		}
	}
	if r.zonedErr != nil {
		return nil, false, &OffsetError{Offset: r.off, Err: r.zonedErr}
	}
	start := r.off
	b, err := r.read(zonedWidth(digits, s))
	if err != nil {
		return nil, false, err
	}

	// Split the separate sign byte, if there is one, off the digits. What
	// remains is digits bytes either way, and first is where it begins.
	var (
		negative bool
		first    int
	)
	switch s {
	case SignLeadingSeparate:
		first = 1
		if negative, err = r.zoned.bytes.separateSignValue(b[0]); err != nil {
			return nil, false, &OffsetError{Offset: start, Err: err}
		}
	case SignTrailingSeparate:
		if negative, err = r.zoned.bytes.separateSignValue(b[digits]); err != nil {
			return nil, false, &OffsetError{Offset: start + int64(digits), Err: err}
		}
	}

	ds, overpunched, at, err := r.zoned.decodeField(b[first:first+digits], s.overpunchAt(digits))
	if err != nil {
		return nil, false, &OffsetError{Offset: start + int64(first+at), Err: err}
	}
	if !s.separate() {
		negative = overpunched
	}
	return ds, negative, nil
}

// ReadPackedInt32 reads the next packed decimal (COMP-3, PACKED-DECIMAL) field
// of digits digits as an int32, consuming ceil((digits+1)/2) bytes.
//
// digits must be between 1 and 9, the most that always fits an int32; a wider
// field is a [PackedDigitCountError] rather than a silent overflow, and is read
// with [Reader.ReadPackedInt64] or [Reader.ReadPackedBig].
//
// The return is the unscaled integer. A PICTURE's V and P positions occupy no
// storage and are not recoverable from the bytes, so scale is not a decoding
// input: PIC S9(3)V99 COMP-3 holding -123.45 reads as -12345, and a generator
// emits the scale beside the field as a constant.
func (r *Reader) ReadPackedInt32(digits int) (int32, error) {
	v, err := r.readPackedInt(digits, maxPackedInt32Digits)
	return int32(v), err
}

// ReadPackedInt64 reads the next packed decimal field of digits digits as an
// int64, consuming ceil((digits+1)/2) bytes.
//
// digits must be between 1 and 18, the most that always fits an int64. The 19
// to 31 digit range an IBM packed item may declare is read with
// [Reader.ReadPackedBig]. As with every numeric accessor the return is the
// unscaled integer; see [Reader.ReadPackedInt32].
func (r *Reader) ReadPackedInt64(digits int) (int64, error) {
	return r.readPackedInt(digits, maxPackedInt64Digits)
}

// ReadPackedBig reads the next packed decimal field of digits digits as a
// [math/big.Int], consuming ceil((digits+1)/2) bytes.
//
// digits must be between 1 and 31, the IBM Enterprise COBOL maximum for a
// packed item. This is the accessor for the 19-to-31 digit range no Go integer
// type holds; below 19 digits the int32 and int64 accessors say the same thing
// without allocating. As with every numeric accessor the return is the unscaled
// integer; see [Reader.ReadPackedInt32].
func (r *Reader) ReadPackedBig(digits int) (*big.Int, error) {
	ds, negative, err := r.readPackedDigits(digits, maxPackedDigits)
	if err != nil {
		return nil, err
	}
	v := new(big.Int)
	ten := big.NewInt(10)
	d := new(big.Int)
	for _, n := range ds {
		v.Mul(v, ten)
		v.Add(v, d.SetInt64(int64(n)))
	}
	if negative {
		v.Neg(v)
	}
	return v, nil
}

// readPackedInt is the shared body of the two integer packed accessors, whose
// only difference is the digit count they accept.
func (r *Reader) readPackedInt(digits, max int) (int64, error) {
	ds, negative, err := r.readPackedDigits(digits, max)
	if err != nil {
		return 0, err
	}
	var v int64
	for _, d := range ds {
		v = v*10 + int64(d)
	}
	if negative {
		v = -v
	}
	return v, nil
}

// readPackedDigits reads one packed decimal field and returns its digits, most
// significant first and one per element with values 0-9, together with whether
// the sign nibble made it negative.
//
// Every nibble is validated: the pad, each digit, and the sign. Rejecting them
// is the only defence a reader has against a record whose offsets have slipped,
// and against a file whose packed fields were destroyed by a naive EBCDIC
// character conversion — see codec/SPEC.md, "The COMP-3 conversion trap".
//
// A nibble error carries the offset of the byte the nibble sits in rather than
// the offset the field ended at, which is the one place in this package an
// [OffsetError] is stamped with something other than the current position: a
// bad nibble is diagnosable only if the byte holding it can be found.
func (r *Reader) readPackedDigits(digits, max int) ([]byte, bool, error) {
	if digits < 1 || digits > max {
		return nil, false, &OffsetError{
			Offset: r.off,
			Err:    PackedDigitCountError{Digits: digits, Max: max},
		}
	}
	start := r.off
	b, err := r.read(packedWidth(digits))
	if err != nil {
		return nil, false, err
	}
	// nibbleAt is the offset of the byte holding nibble i, counted from the
	// first byte of the field.
	nibbleAt := func(i int) int64 { return start + int64(i/2) }

	nibbles := make([]byte, 0, 2*len(b))
	for _, c := range b {
		nibbles = append(nibbles, c>>4, c&0x0F)
	}
	// The pad nibble exists exactly when the digit count is even, because
	// digits+1 nibbles is then odd and rounds up to a whole byte.
	if digits%2 == 0 && nibbles[0] != 0 {
		return nil, false, &OffsetError{
			Offset: nibbleAt(0),
			Err:    PackedPadError{Nibble: nibbles[0]},
		}
	}
	first := len(nibbles) - 1 - digits
	ds := nibbles[first : len(nibbles)-1]
	for i, d := range ds {
		if d > 9 {
			return nil, false, &OffsetError{
				Offset: nibbleAt(first + i),
				Err:    PackedDigitError{Nibble: d},
			}
		}
	}
	negative, err := packedSignIsNegative(nibbles[len(nibbles)-1])
	if err != nil {
		return nil, false, &OffsetError{Offset: nibbleAt(len(nibbles) - 1), Err: err}
	}
	return ds, negative, nil
}

// ReadComp6Int32 reads the next COMP-6 field of digits digits as an int32,
// consuming ceil(digits/2) bytes.
//
// COMP-6 is the GnuCOBOL and Micro Focus packed decimal with no sign nibble at
// all: every nibble of the field is a digit nibble, and the item is always
// unsigned. The two encodings are not interchangeable, and they fail
// differently depending on the digit count: at an even count COMP-6 is a byte
// narrower — PIC 9(4) COMP-6 is two bytes where PIC 9(4) COMP-3 is three — so
// reading one as the other shifts every later field of the record, while at an
// odd count the widths coincide and what catches it is the nibbles, a COMP-3
// sign nibble landing where a digit belongs. See codec/SPEC.md, "COMP-6".
//
// There is no [Signedness] to declare, on this side or the writing one, because
// the encoding has nowhere to put a sign. A field whose PICTURE carries S is
// not a COMP-6 field.
//
// digits must be between 1 and 9, the most that always fits an int32; a wider
// field is a [PackedDigitCountError] rather than a silent overflow, and is read
// with [Reader.ReadComp6Int64] or [Reader.ReadComp6Big].
//
// The return is the unscaled integer, for the reason it is on
// [Reader.ReadPackedInt32]: a PICTURE's V and P positions occupy no storage.
func (r *Reader) ReadComp6Int32(digits int) (int32, error) {
	v, err := r.readComp6Int(digits, maxPackedInt32Digits)
	return int32(v), err
}

// ReadComp6Int64 reads the next COMP-6 field of digits digits as an int64,
// consuming ceil(digits/2) bytes.
//
// digits must be between 1 and 18, the most that always fits an int64; wider
// counts are read with [Reader.ReadComp6Big]. As with every numeric accessor
// the return is the unscaled integer; see [Reader.ReadComp6Int32], which also
// says why no [Signedness] is taken.
func (r *Reader) ReadComp6Int64(digits int) (int64, error) {
	return r.readComp6Int(digits, maxPackedInt64Digits)
}

// ReadComp6Big reads the next COMP-6 field of digits digits as a
// [math/big.Int], consuming ceil(digits/2) bytes.
//
// digits must be between 1 and 31, the same upper bound the COMP-3 accessors
// take. This is the accessor for the 19-to-31 digit range no Go integer type
// holds. As with every numeric accessor the return is the unscaled integer; see
// [Reader.ReadComp6Int32].
func (r *Reader) ReadComp6Big(digits int) (*big.Int, error) {
	ds, err := r.readComp6Digits(digits, maxPackedDigits)
	if err != nil {
		return nil, err
	}
	v := new(big.Int)
	ten := big.NewInt(10)
	d := new(big.Int)
	for _, n := range ds {
		v.Mul(v, ten)
		v.Add(v, d.SetInt64(int64(n)))
	}
	return v, nil
}

// readComp6Int is the shared body of the two integer COMP-6 accessors, whose
// only difference is the digit count they accept.
func (r *Reader) readComp6Int(digits, max int) (int64, error) {
	ds, err := r.readComp6Digits(digits, max)
	if err != nil {
		return 0, err
	}
	var v int64
	for _, d := range ds {
		v = v*10 + int64(d)
	}
	return v, nil
}

// readComp6Digits reads one COMP-6 field and returns its digits, most
// significant first and one per element with values 0-9.
//
// It returns no sign, because COMP-6 stores none. That is the whole of the
// difference from [Reader.readPackedDigits], and it is why the two are separate
// bodies rather than one with a flag: there is no sign nibble to skip, so the
// digits run to the very end of the field and the pad nibble appears on the
// opposite parity of digits.
//
// Every nibble is validated as a digit. Nothing in the field may be A-F, so
// none of COMP-3's sign alphabet is accepted anywhere — a value ending in a C
// or an F is a COMP-3 field being read at a COMP-6 offset, which is exactly the
// mistake worth failing on.
//
// A nibble error carries the offset of the byte the nibble sits in, for the
// reason [Reader.readPackedDigits] does.
func (r *Reader) readComp6Digits(digits, max int) ([]byte, error) {
	if digits < 1 || digits > max {
		return nil, &OffsetError{
			Offset: r.off,
			Err:    PackedDigitCountError{Digits: digits, Max: max},
		}
	}
	start := r.off
	b, err := r.read(comp6Width(digits))
	if err != nil {
		return nil, err
	}
	// nibbleAt is the offset of the byte holding nibble i, counted from the
	// first byte of the field.
	nibbleAt := func(i int) int64 { return start + int64(i/2) }

	nibbles := make([]byte, 0, 2*len(b))
	for _, c := range b {
		nibbles = append(nibbles, c>>4, c&0x0F)
	}
	// The pad nibble exists exactly when the digit count is odd, which is the
	// opposite parity from COMP-3: there is no sign nibble making the count
	// up, so an odd digit count is what leaves half a byte over. It is the
	// high nibble of the first byte, as COMP-3's is.
	if digits%2 == 1 && nibbles[0] != 0 {
		return nil, &OffsetError{
			Offset: nibbleAt(0),
			Err:    PackedPadError{Nibble: nibbles[0]},
		}
	}
	first := len(nibbles) - digits
	ds := nibbles[first:]
	for i, d := range ds {
		if d > 9 {
			return nil, &OffsetError{
				Offset: nibbleAt(first + i),
				Err:    PackedDigitError{Nibble: d},
			}
		}
	}
	return ds, nil
}

// ReadBinaryInt16 reads the next binary (COMP, COMP-4, BINARY) field of digits
// digits as a signed int16, consuming 2 bytes.
//
// digits must be between 1 and 4, the digit counts a 2-byte item carries. The
// bytes are two's complement in the order [Encoding.ByteOrder] declares, which
// is required and never inferred: a wrong byte order yields a plausible wrong
// number and never an error.
//
// Range semantics are TRUNC(STD): a stored value whose magnitude the PICTURE
// cannot express is a [BinaryRangeError] rather than a silently over-wide
// reading. That check is what turns a wrong byte order into a first-record
// failure most of the time. Read a COMP-5 field, or a COMP field compiled under
// TRUNC(BIN), with [Reader.ReadComp5Int16] instead; see [Truncation].
//
// As with every numeric accessor the return is the unscaled integer, since a
// PICTURE's V occupies no storage; see [Reader.ReadPackedInt32].
func (r *Reader) ReadBinaryInt16(digits int) (int16, error) {
	v, err := r.readBinaryInt(digits, maxBinaryInt16Digits, TruncStd)
	return int16(v), err
}

// ReadBinaryInt32 reads the next binary field of digits digits as a signed
// int32, consuming 2 bytes for 1 to 4 digits and 4 bytes for 5 to 9.
//
// digits must be between 1 and 9. PIC 9(5) COMP is four bytes and not five —
// the width is a staircase, not the digit count. Range semantics are
// TRUNC(STD); see [Reader.ReadBinaryInt16].
func (r *Reader) ReadBinaryInt32(digits int) (int32, error) {
	v, err := r.readBinaryInt(digits, maxBinaryInt32Digits, TruncStd)
	return int32(v), err
}

// ReadBinaryInt64 reads the next binary field of digits digits as a signed
// int64, consuming 2, 4 or 8 bytes by the digit count.
//
// digits must be between 1 and 18. The 19-to-31 digit range an ARITH(EXTEND)
// item may declare is 16 bytes wide and is read with [Reader.ReadBinaryBig].
// Range semantics are TRUNC(STD); see [Reader.ReadBinaryInt16].
func (r *Reader) ReadBinaryInt64(digits int) (int64, error) {
	return r.readBinaryInt(digits, maxBinaryInt64Digits, TruncStd)
}

// ReadBinaryUint64 reads the next binary field of digits digits as an unsigned
// uint64, consuming 2, 4 or 8 bytes by the digit count.
//
// This is the accessor for an item whose PICTURE has no S. The distinction is
// not cosmetic and is not recoverable from the bytes: FF FF is 65535 read as an
// unsigned 2-byte item and -1 read as a signed one, so which accessor is called
// is what says which the copybook declared.
//
// digits must be between 1 and 18. Range semantics are TRUNC(STD); see
// [Reader.ReadBinaryInt16]. A PIC 9(4) COMP-5 item holding 65535 is outside the
// four-digit decimal range and is read with [Reader.ReadComp5Uint64].
func (r *Reader) ReadBinaryUint64(digits int) (uint64, error) {
	return r.readBinaryUint(digits, maxBinaryInt64Digits, TruncStd)
}

// ReadBinaryBig reads the next binary field of digits digits as a
// [math/big.Int], consuming 2, 4, 8 or 16 bytes by the digit count.
//
// digits must be between 1 and 31. This is the accessor for the 19-to-31 digit
// range that IBM Enterprise COBOL allows under ARITH(EXTEND) and that no Go
// integer type holds; below 19 digits the fixed-width accessors say the same
// thing without allocating. Range semantics are TRUNC(STD); see
// [Reader.ReadBinaryInt16].
//
// The bytes are read as two's complement, so the value is signed. An unsigned
// item of that width reads identically under TRUNC(STD), because 10^31 is far
// below the 2^127 at which the sign bit of a 16-byte field turns on; the
// readings part company only for a COMP-5 field, which is what
// [Reader.ReadComp5Big] documents.
func (r *Reader) ReadBinaryBig(digits int) (*big.Int, error) {
	return r.readBinaryBig(digits, TruncStd)
}

// ReadComp5Int16 reads the next COMP-5 field of digits digits as a signed
// int16, consuming 2 bytes.
//
// It is [Reader.ReadBinaryInt16] with TRUNC(BIN) range semantics: the value may
// use the full -32768 to 32767 range of the storage rather than the decimal
// range of the PICTURE, so no range validation is performed at all. Use it for
// USAGE COMP-5, which always means this, and for COMP or COMP-4 compiled under
// TRUNC(BIN) or GnuCOBOL's binary-truncate: no. See [Truncation].
//
// COMP-5 is defined as *native* byte order on the platform that wrote it, which
// is a fact about the file and is declared through [Encoding.ByteOrder] like
// any other: this accessor does not assume one.
func (r *Reader) ReadComp5Int16(digits int) (int16, error) {
	v, err := r.readBinaryInt(digits, maxBinaryInt16Digits, TruncBin)
	return int16(v), err
}

// ReadComp5Int32 reads the next COMP-5 field of digits digits as a signed
// int32, consuming 2 or 4 bytes. It is [Reader.ReadBinaryInt32] with TRUNC(BIN)
// range semantics; see [Reader.ReadComp5Int16].
func (r *Reader) ReadComp5Int32(digits int) (int32, error) {
	v, err := r.readBinaryInt(digits, maxBinaryInt32Digits, TruncBin)
	return int32(v), err
}

// ReadComp5Int64 reads the next COMP-5 field of digits digits as a signed
// int64, consuming 2, 4 or 8 bytes. It is [Reader.ReadBinaryInt64] with
// TRUNC(BIN) range semantics; see [Reader.ReadComp5Int16].
func (r *Reader) ReadComp5Int64(digits int) (int64, error) {
	return r.readBinaryInt(digits, maxBinaryInt64Digits, TruncBin)
}

// ReadComp5Uint64 reads the next COMP-5 field of digits digits as an unsigned
// uint64, consuming 2, 4 or 8 bytes. It is [Reader.ReadBinaryUint64] with
// TRUNC(BIN) range semantics; see [Reader.ReadComp5Int16].
//
// This is the accessor a PIC 9(4) COMP-5 item holding 65535 needs: those two
// FF bytes are legal there and are outside the range TRUNC(STD) allows.
func (r *Reader) ReadComp5Uint64(digits int) (uint64, error) {
	return r.readBinaryUint(digits, maxBinaryInt64Digits, TruncBin)
}

// ReadComp5Big reads the next COMP-5 field of digits digits as a
// [math/big.Int], consuming 2, 4, 8 or 16 bytes. It is [Reader.ReadBinaryBig]
// with TRUNC(BIN) range semantics; see [Reader.ReadComp5Int16].
//
// The bytes are read as two's complement over the full storage width, so a
// 16-byte field with its top bit set reads as a negative number. An unsigned
// 16-byte COMP-5 item carrying a value that large has no accessor here; below
// 8 bytes, [Reader.ReadComp5Uint64] is the unsigned reading.
func (r *Reader) ReadComp5Big(digits int) (*big.Int, error) {
	return r.readBinaryBig(digits, TruncBin)
}

// readBinaryField reads one binary field of at most 8 bytes, returning its
// bytes as a raw unsigned integer together with the field's width and the
// offset it began at.
func (r *Reader) readBinaryField(digits, max int) (raw uint64, width int, start int64, err error) {
	if digits < 1 || digits > max {
		return 0, 0, r.off, &OffsetError{
			Offset: r.off,
			Err:    BinaryDigitCountError{Digits: digits, Max: max},
		}
	}
	width = binaryWidth(digits)
	start = r.off
	b, err := r.read(width)
	if err != nil {
		return 0, 0, start, err
	}
	switch width {
	case 2:
		raw = uint64(r.enc.ByteOrder.Uint16(b))
	case 4:
		raw = uint64(r.enc.ByteOrder.Uint32(b))
	default:
		raw = r.enc.ByteOrder.Uint64(b)
	}
	return raw, width, start, nil
}

// readBinaryInt is the shared body of the signed fixed-width accessors, whose
// only differences are the digit count they accept and the truncation mode they
// validate under.
func (r *Reader) readBinaryInt(digits, max int, t Truncation) (int64, error) {
	raw, width, start, err := r.readBinaryField(digits, max)
	if err != nil {
		return 0, err
	}
	v := signExtend(raw, width)
	// TRUNC(BIN) confines a value to its storage width and nothing else, and
	// the width is what was just read, so there is nothing left to check.
	if t == TruncStd {
		limit := int64(pow10[digits] - 1)
		if v < -limit || v > limit {
			return 0, &OffsetError{
				Offset: start,
				Err: BinaryRangeError{
					Value:      strconv.FormatInt(v, 10),
					Digits:     digits,
					Width:      width,
					Signedness: Signed,
					Truncation: t,
				},
			}
		}
	}
	return v, nil
}

// readBinaryUint is readBinaryInt for an item whose PICTURE has no S: the same
// bytes read as an unsigned magnitude rather than as two's complement.
func (r *Reader) readBinaryUint(digits, max int, t Truncation) (uint64, error) {
	raw, width, start, err := r.readBinaryField(digits, max)
	if err != nil {
		return 0, err
	}
	if t == TruncStd && raw > pow10[digits]-1 {
		return 0, &OffsetError{
			Offset: start,
			Err: BinaryRangeError{
				Value:      strconv.FormatUint(raw, 10),
				Digits:     digits,
				Width:      width,
				Signedness: Unsigned,
				Truncation: t,
			},
		}
	}
	return raw, nil
}

// readBinaryBig is the shared body of the two [math/big.Int] accessors. It is
// separate from readBinaryInt because [binary.ByteOrder] has no 16-byte
// accessor: the widest fields are ordered a byte at a time.
func (r *Reader) readBinaryBig(digits int, t Truncation) (*big.Int, error) {
	if digits < 1 || digits > maxBinaryDigits {
		return nil, &OffsetError{
			Offset: r.off,
			Err:    BinaryDigitCountError{Digits: digits, Max: maxBinaryDigits},
		}
	}
	width := binaryWidth(digits)
	start := r.off
	b, err := r.read(width)
	if err != nil {
		return nil, err
	}
	orderBinaryBytes(r.enc.ByteOrder, b)

	v := new(big.Int).SetBytes(b)
	if b[0]&0x80 != 0 {
		// Two's complement: the stored bits are the value plus 2^(8*width).
		v.Sub(v, new(big.Int).Lsh(big.NewInt(1), uint(8*width)))
	}
	if t == TruncStd && v.CmpAbs(decimalLimit(digits)) > 0 {
		return nil, &OffsetError{
			Offset: start,
			Err: BinaryRangeError{
				Value:      v.String(),
				Digits:     digits,
				Width:      width,
				Signedness: Signed,
				Truncation: t,
			},
		}
	}
	return v, nil
}

// ReadFloat32 reads the next floating point (COMP-1) field as a float32,
// consuming 4 bytes.
//
// COMP-1 takes no PICTURE — the usage alone fixes the format — so this
// accessor takes no digit count and there is no scale to apply. It is the one
// numeric family in this package whose width is not a function of a digit
// count.
//
// What the four bytes mean comes entirely from [Encoding.Float], which is
// required and never inferred. Under [FloatIEEE] they are binary32 in the order
// [Encoding.ByteOrder] declares. Under [FloatHFP] they are IBM hexadecimal
// floating point and big-endian *regardless* of that axis: HFP predates any
// little-endian IBM platform and has no little-endian form, so byte order is
// not a question a COMP-1 field in that format asks.
//
// The two formats read each other's bytes without complaint. IEEE 1.0 is
// 3F 80 00 00 and reads as 0.03125 under HFP; HFP 1.0 is 41 10 00 00 and reads
// as 9.0 under IEEE. Neither is an error, a NaN or an out-of-range value — they
// are plausible numbers that pass every check downstream, which is why the axis
// has no default.
//
// HFP's exponent range is far wider than binary32's, so a COMP-1 field may
// legitimately hold a value no float32 expresses. That is a [FloatRangeError]
// rather than an infinity or a zero; see [FloatRangeError] for why it is not
// simply returned.
func (r *Reader) ReadFloat32() (float32, error) {
	start := r.off
	b, err := r.read(comp1Width)
	if err != nil {
		return 0, err
	}
	if r.enc.Float == FloatIEEE {
		return math.Float32frombits(r.enc.ByteOrder.Uint32(b)), nil
	}
	v := floatFromHFP(uint64(binary.BigEndian.Uint32(b)), comp1Width)
	f := float32(v)
	reason := ""
	switch {
	case math.IsInf(float64(f), 0):
		reason = "overflows a float32"
	case f == 0 && v != 0:
		reason = "underflows a float32 to zero"
	default:
		return f, nil
	}
	return 0, &OffsetError{
		Offset: start,
		Err: FloatRangeError{
			Value:  strconv.FormatFloat(v, 'g', -1, 64),
			Format: FloatHFP,
			Width:  comp1Width,
			Reason: reason,
		},
	}
}

// ReadFloat64 reads the next floating point (COMP-2) field as a float64,
// consuming 8 bytes.
//
// It is [Reader.ReadFloat32] one width up, and everything that accessor says
// about [Encoding.Float], about HFP being big-endian regardless of
// [Encoding.ByteOrder], and about the two formats reading each other silently
// holds here unchanged.
//
// It has no range failure. HFP's range, 16^-65 to 16^63, sits well inside a
// float64's, so every COMP-2 field decodes to a number. HFP long carries 56
// bits of fraction against a float64's 53, so a value using the last three is
// rounded to nearest on the way out — the one place in this package a decoded
// number is not exact.
func (r *Reader) ReadFloat64() (float64, error) {
	b, err := r.read(comp2Width)
	if err != nil {
		return 0, err
	}
	if r.enc.Float == FloatIEEE {
		return math.Float64frombits(r.enc.ByteOrder.Uint64(b)), nil
	}
	return floatFromHFP(binary.BigEndian.Uint64(b), comp2Width), nil
}

// Unmarshal reads data into v under the given encoding.
//
// enc is the first argument and is required, for the reason [Encoding] exists:
// none of its four axes has a default, and every one of them fails silently
// when wrong.
//
// Bytes left over after v has read what it wants are not an error — a data file
// is a sequence of records, and a record type reads its own length.
func Unmarshal(enc Encoding, data []byte, v Unmarshaler) error {
	r, err := NewReader(bytes.NewReader(data), enc)
	if err != nil {
		return err
	}
	if v == nil {
		return ErrNilValue
	}
	return v.UnmarshalCOBOL(r)
}
