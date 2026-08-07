// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"bytes"
	"io"
	"math/big"
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
