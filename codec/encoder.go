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

// Writer writes the fields of a COBOL data file to an [io.Writer], one field at
// a time and in record order. It is the inverse of [Reader]: same shape,
// opposite direction, same encoding.
//
// There is deliberately no usable zero value: a Writer is only obtainable from
// [NewWriter], which requires a complete [Encoding]. A Writer is not safe for
// concurrent use — the write position is state. It does not buffer, so there is
// nothing to flush; a caller wanting buffering wraps its own [io.Writer].
type Writer struct {
	w   io.Writer
	enc Encoding
	off int64
}

// NewWriter returns a [Writer] that writes to w under the given encoding.
//
// enc must declare all four axes; construction fails with an [EncodingError]
// naming the first field that does not. The requirement is the same as
// [NewReader]'s and for the same reason: a file written under a guessed
// encoding is wrong in a way nothing downstream can detect.
func NewWriter(w io.Writer, enc Encoding) (*Writer, error) {
	if w == nil {
		return nil, ErrNilWriter
	}
	if err := enc.Validate(); err != nil {
		return nil, err
	}
	return &Writer{w: w, enc: enc}, nil
}

// Encoding reports the encoding the [Writer] was constructed with.
func (w *Writer) Encoding() Encoding { return w.enc }

// Offset reports how many bytes have been written, which is the position the
// next field will start at. Bytes written by a failed write are counted, so the
// offset after an error is where writing stopped.
func (w *Writer) Offset() int64 { return w.off }

// write emits p. It is the single place the offset advances and the single
// place write errors are stamped with it.
func (w *Writer) write(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	n, err := w.w.Write(p)
	w.off += int64(n)
	if err != nil {
		return &OffsetError{Offset: w.off, Err: err}
	}
	if n < len(p) {
		return &OffsetError{Offset: w.off, Err: io.ErrShortWrite}
	}
	return nil
}

// WriteBytes writes p as it stands, applying no character translation and no
// padding. It is the counterpart of [Reader.ReadBytes]: what that reads at a
// given offset, this writes, byte for byte.
func (w *Writer) WriteBytes(p []byte) error {
	return w.write(p)
}

// WriteAlphanumeric writes s as an alphanumeric (PIC X) or alphabetic (PIC A)
// field exactly n bytes wide, translating it through the encoding's charset and
// padding it on the right with the charset's space.
//
// It is [Writer.WriteAlphanumericJustified] with [JustifyLeft], the COBOL
// default for an item with no JUSTIFIED clause.
func (w *Writer) WriteAlphanumeric(s string, n int) error {
	return w.WriteAlphanumericJustified(s, n, JustifyLeft)
}

// WriteAlphanumericJustified writes s as an alphanumeric field exactly n bytes
// wide, placing the value at the end of the field j names and padding the
// other: padding on the right under [JustifyLeft], on the left under
// [JustifyRight], which is the JUSTIFIED RIGHT clause.
//
// A value that does not fit is a [FieldTooLongError] rather than a truncation,
// and a character with no byte in the charset is an
// [UnrepresentableRuneError]. Both are wrapped in an [OffsetError], and neither
// writes anything: a field is emitted whole or not at all, so a rejected field
// does not desynchronize the record.
func (w *Writer) WriteAlphanumericJustified(s string, n int, j Justification) error {
	if j != JustifyLeft && j != JustifyRight {
		return &OffsetError{Offset: w.off, Err: JustificationError{Justification: j}}
	}
	if n < 0 {
		return &OffsetError{Offset: w.off, Err: FieldWidthError{Width: n}}
	}
	charset := w.enc.Charset
	value := make([]byte, 0, n)
	for _, r := range s {
		b, ok := charset.FromUnicode(r)
		if !ok {
			return &OffsetError{
				Offset: w.off,
				Err:    UnrepresentableRuneError{Rune: r, Charset: charset.Name()},
			}
		}
		value = append(value, b)
	}
	if len(value) > n {
		return &OffsetError{
			Offset: w.off,
			Err:    FieldTooLongError{Len: len(value), Width: n},
		}
	}
	field := bytes.Repeat([]byte{charset.Space()}, n)
	if j == JustifyRight {
		copy(field[n-len(value):], value)
	} else {
		copy(field, value)
	}
	return w.write(field)
}

// WritePackedInt32 writes v as a packed decimal (COMP-3, PACKED-DECIMAL) field
// of digits digits, exactly ceil((digits+1)/2) bytes wide.
//
// digits must be between 1 and 9; see [Reader.ReadPackedInt32] for why the
// bound belongs to the accessor rather than to the field.
//
// s is required and says whether the item's PICTURE carries S, which is what
// selects the sign nibble: C or D when [Signed], F when [Unsigned]. It has no
// default, because neither choice is recoverable from v — see [Signedness].
//
// v is the unscaled integer: a PIC S9(3)V99 COMP-3 item holding -123.45 is
// written as -12345 with digits 5, since V occupies no storage.
func (w *Writer) WritePackedInt32(v int32, digits int, s Signedness) error {
	return w.writePackedInt(int64(v), digits, maxPackedInt32Digits, s)
}

// WritePackedInt64 writes v as a packed decimal field of digits digits,
// exactly ceil((digits+1)/2) bytes wide. digits must be between 1 and 18; the
// 19-to-31 digit range is written with [Writer.WritePackedBig]. s is required,
// as it is on [Writer.WritePackedInt32], and for the same reason.
func (w *Writer) WritePackedInt64(v int64, digits int, s Signedness) error {
	return w.writePackedInt(v, digits, maxPackedInt64Digits, s)
}

// WritePackedBig writes v as a packed decimal field of digits digits, exactly
// ceil((digits+1)/2) bytes wide. digits must be between 1 and 31, the IBM
// Enterprise COBOL maximum. s is required, as it is on
// [Writer.WritePackedInt32], and for the same reason.
//
// A nil v is [ErrNilValue] rather than a zero: an absent number and the number
// zero are different things, and a field is not written from a guess.
func (w *Writer) WritePackedBig(v *big.Int, digits int, s Signedness) error {
	if v == nil {
		return &OffsetError{Offset: w.off, Err: ErrNilValue}
	}
	return w.writePacked(v.String(), v.Sign() < 0, digits, maxPackedDigits, s)
}

// writePackedInt is the shared body of the two integer packed writers, whose
// only difference is the digit count they accept.
func (w *Writer) writePackedInt(v int64, digits, max int, s Signedness) error {
	// Formatted rather than negated, so that the most negative int64 is
	// written like any other value instead of overflowing on its way out.
	return w.writePacked(strconv.FormatInt(v, 10), v < 0, digits, max, s)
}

// writePacked builds and writes one packed decimal field from the decimal
// spelling of a value.
//
// The whole field is validated and built before a byte of it is written, so a
// rejected value writes nothing and cannot leave a half-field behind to
// desynchronize the record.
func (w *Writer) writePacked(text string, negative bool, digits, max int, s Signedness) error {
	if !s.valid() {
		return &OffsetError{Offset: w.off, Err: SignednessError{Signedness: s}}
	}
	if digits < 1 || digits > max {
		return &OffsetError{
			Offset: w.off,
			Err:    PackedDigitCountError{Digits: digits, Max: max},
		}
	}
	magnitude := strings.TrimPrefix(text, "-")
	if len(magnitude) > digits || (negative && s == Unsigned) {
		return &OffsetError{
			Offset: w.off,
			Err:    PackedRangeError{Value: text, Digits: digits, Signedness: s},
		}
	}

	// Nibbles high first. Everything ahead of the digits stays zero, which
	// covers both the high-order zeros the value does not fill and the pad
	// nibble an even digit count leaves over.
	width := packedWidth(digits)
	nibbles := make([]byte, 2*width)
	first := len(nibbles) - 1 - len(magnitude)
	for i := 0; i < len(magnitude); i++ {
		nibbles[first+i] = magnitude[i] - '0'
	}
	switch {
	case s == Unsigned:
		nibbles[len(nibbles)-1] = packedSignUnsigned
	case negative:
		nibbles[len(nibbles)-1] = packedSignNegative
	default:
		nibbles[len(nibbles)-1] = packedSignPositive
	}

	field := make([]byte, width)
	for i := range field {
		field[i] = nibbles[2*i]<<4 | nibbles[2*i+1]
	}
	return w.write(field)
}

// Marshal writes v under the given encoding and returns the bytes it produced.
//
// enc is the first argument and is required, for the reason [Encoding] exists:
// none of its four axes has a default, and every one of them fails silently
// when wrong.
func Marshal(enc Encoding, v Marshaler) ([]byte, error) {
	var buf bytes.Buffer
	w, err := NewWriter(&buf, enc)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrNilValue
	}
	if err := v.MarshalCOBOL(w); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
