// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"bytes"
	"io"
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
