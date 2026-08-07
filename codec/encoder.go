// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"bytes"
	"io"
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
