// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"bytes"
	"io"
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
	return &Reader{r: r, enc: enc}, nil
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
