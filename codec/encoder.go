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

// Writer writes the fields of a COBOL data file to an [io.Writer], or appends
// them to a []byte the caller already holds, one field at a time and in record
// order. It is the inverse of [Reader]: same shape, opposite direction, same
// encoding.
//
// There is deliberately no usable zero value: a Writer is only obtainable from
// [NewWriter] or [NewBytesWriter], each of which requires a complete
// [Encoding]. A Writer is not safe for concurrent use — the write position is
// state.
//
// It does not buffer an [io.Writer], and must not be made to: a Writer has no
// Flush and no Close, so bytes held back from the stream would be lost by every
// caller that simply stops writing. A byte-backed Writer is the other thing —
// the bytes it appends *are* its output, handed over by [Writer.Bytes] — and
// [Writer.Reset] rewinds one onto the next record without allocating, which is
// what makes one Writer per file, or a pool of them, the cheap way to encode a
// sequence of records.
type Writer struct {
	// w is the stream a Writer built by [NewWriter] writes to, and is nil
	// for exactly the byte-backed Writers — those built by [NewBytesWriter]
	// or rewound by [Writer.Reset] — which append to buf instead.
	// [NewWriter] rejects a nil [io.Writer], so nil here is unambiguous.
	w io.Writer
	// buf is what a byte-backed Writer appends to and what [Writer.Bytes]
	// returns. It is the caller's own slice, reused at its capacity from
	// [Writer.Reset] onward, which is where the per-record buffer
	// [Marshal] used to allocate went.
	buf []byte
	enc Encoding
	off int64
	// zoned and zonedErr are the encoding's zoned decimal byte table and the
	// failure of deriving it, for the reason [Reader] carries them: the
	// table is derived once rather than per field, and a charset that cannot
	// spell a digit is refused by the first zoned field rather than by the
	// constructor, since alphanumeric fields do not need one.
	zoned    zonedCodec
	zonedErr error
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
	wr, err := newWriter(enc)
	if err != nil {
		return nil, err
	}
	wr.w = w
	return wr, nil
}

// NewBytesWriter returns a [Writer] that appends to buf under the given
// encoding, and whose output is read back with [Writer.Bytes]. It is
// [NewWriter] over a buffer the caller already holds, and it is the writing
// side of [NewBytesReader].
//
// buf is appended to as it stands, so a caller passing a slice with bytes in it
// gets those bytes back in front of the record; passing buf[:0] reuses the
// capacity and keeps nothing. [Writer.Offset] counts from 0 either way, since
// it reports what this Writer has written and not how long the slice is.
//
// enc is validated exactly as [NewWriter] validates it, and construction fails
// with the same [EncodingError] naming the same field. The one difference is
// what it does *not* reject: a nil buf is an empty buffer to append to and not
// an error, because a nil slice is a slice of no bytes rather than a missing
// destination. That is what [Marshal] passes.
func NewBytesWriter(buf []byte, enc Encoding) (*Writer, error) {
	wr, err := newWriter(enc)
	if err != nil {
		return nil, err
	}
	wr.buf = buf
	return wr, nil
}

// newWriter builds the half of a [Writer] that comes from the encoding —
// which is exactly the half [Writer.Reset] keeps — and leaves the destination
// to its caller, for the reason [newReader] does the same.
func newWriter(enc Encoding) (*Writer, error) {
	if err := enc.Validate(); err != nil {
		return nil, err
	}
	zoned, zonedErr := newZonedCodec(enc)
	return &Writer{enc: enc, zoned: zoned, zonedErr: zonedErr}, nil
}

// Reset rewinds w onto buf, truncating it to no length so that the next field
// written is the record's first byte and [Writer.Offset] reports 0 again. The
// capacity survives, which is the point: a Writer reset onto the buffer it
// filled last time writes the next record into the same bytes.
//
// It is [Reader.Reset] in the other direction, and everything derived from the
// [Encoding] survives it in the same way. The Encoding itself cannot change;
// a different one needs a different Writer.
//
//	w := pool.Get().(*codec.Writer)
//	defer func() { w.Reset(nil); pool.Put(w) }()
//	for _, rec := range records {
//		w.Reset(scratch)
//		if err := rec.MarshalCOBOL(w); err != nil {
//			return err
//		}
//		if _, err := f.Write(w.Bytes()); err != nil {
//			return err
//		}
//		scratch = w.Bytes()
//	}
//
// **The Writer holds buf; it does not copy it.** The slice is retained until
// the next Reset and no longer, so a caller returning a Writer to a pool passes
// nil to drop the reference — and a caller that keeps writing through the
// Writer must expect the bytes [Writer.Bytes] returned to be overwritten, since
// they are the same array.
//
// Reset works on a Writer built by [NewWriter] too: the stream is dropped and
// the buffer takes its place. That is a change of destination and not of
// position, so a Writer half way through a record on a stream is not one to
// reset.
func (w *Writer) Reset(buf []byte) {
	w.w = nil
	w.buf = buf[:0]
	w.off = 0
}

// Bytes returns what has been written to a byte-backed [Writer] — one built by
// [NewBytesWriter] or rewound by [Writer.Reset] — including anything the buffer
// it was given already held.
//
// The slice is the Writer's own, valid until the next write or [Writer.Reset]
// and no longer, in the way [bytes.Buffer.Bytes] is. A caller keeping the
// record past that point copies it; [Marshal] can hand its result straight over
// because it drops the Writer.
//
// A Writer over an [io.Writer] has written nothing here and returns nil: its
// bytes went to the stream.
func (w *Writer) Bytes() []byte { return w.buf }

// Encoding reports the encoding the [Writer] was constructed with.
func (w *Writer) Encoding() Encoding { return w.enc }

// Offset reports how many bytes have been written, which is the position the
// next field will start at. Bytes written by a failed write are counted, so the
// offset after an error is where writing stopped.
func (w *Writer) Offset() int64 { return w.off }

// write emits p. It is the single place the offset advances and the single
// place write errors are stamped with it — for the streaming destination and
// the byte-backed one alike, which is why the two arms are here rather than in
// two methods.
//
// Appending cannot fail: there is no short write and no error to stamp, so the
// byte-backed arm is the offset and nothing else.
func (w *Writer) write(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	if w.w == nil {
		if cap(w.buf)-len(w.buf) < len(p) {
			w.grow(len(p))
		}
		w.buf = append(w.buf, p...)
		w.off += int64(len(p))
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

// smallRecordBuffer is the capacity a byte-backed [Writer] jumps to on its
// first field rather than growing into a byte at a time. It is
// [bytes.Buffer]'s own bootstrap size and it is here for the same reason:
// a record is written field by field, so a buffer growing by what append asks
// for reaches a record's width in four or five allocations where one will do.
// [Marshal] is what makes that a per-record cost rather than a per-file one.
const smallRecordBuffer = 64

// grow makes room in buf for n more bytes, geometrically and with a floor, so
// that a record's fields cost one allocation between them rather than one
// each. It is only reached on the byte-backed path, and only when the caller's
// own capacity — the whole point of [Writer.Reset] — has run out.
func (w *Writer) grow(n int) {
	next := 2 * cap(w.buf)
	if next < smallRecordBuffer {
		next = smallRecordBuffer
	}
	if need := len(w.buf) + n; next < need {
		next = need
	}
	buf := make([]byte, len(w.buf), next)
	copy(buf, w.buf)
	w.buf = buf
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

// WriteZonedInt32 writes v as a zoned decimal (USAGE DISPLAY) field of digits
// digits, exactly digits bytes wide — or digits+1 when s is a SEPARATE
// position. It is the counterpart of [Reader.ReadZonedInt32].
//
// digits must be between 1 and 9; see [Reader.ReadZonedInt32] for why the bound
// belongs to the accessor rather than to the field.
//
// s is required and, unlike the packed and binary writers, it takes the place
// of a [Signedness] rather than joining one: [SignUnsigned] is an item whose
// PICTURE has no S, and the other four both declare the S and say which byte
// carries it. Neither fact is recoverable from v, so both are stated per call
// and never defaulted; see [SignPosition].
//
// A value with more digits than the field holds, or a negative one written into
// a [SignUnsigned] field, is a [ZonedRangeError] and writes nothing — a whole
// field is emitted or none of it, so a rejected value cannot desynchronize the
// record. A value with fewer digits is padded on the left with zeros, which is
// what a COBOL MOVE stores.
//
// v is the unscaled integer: a PIC S9(3)V99 item holding -123.45 is written as
// -12345 with digits 5, since V occupies no byte.
func (w *Writer) WriteZonedInt32(v int32, digits int, s SignPosition) error {
	return w.writeZonedInt(int64(v), digits, maxZonedInt32Digits, s)
}

// WriteZonedInt64 writes v as a zoned decimal field of digits digits, exactly
// digits bytes wide, or digits+1 under a SEPARATE sign position. digits must be
// between 1 and 18; the 19-to-31 digit range is written with
// [Writer.WriteZonedBig]. s is required, as it is on [Writer.WriteZonedInt32],
// and for the same reason.
func (w *Writer) WriteZonedInt64(v int64, digits int, s SignPosition) error {
	return w.writeZonedInt(v, digits, maxZonedInt64Digits, s)
}

// WriteZonedBig writes v as a zoned decimal field of digits digits, exactly
// digits bytes wide, or digits+1 under a SEPARATE sign position. digits must be
// between 1 and 31, the IBM Enterprise COBOL maximum. s is required; see
// [Writer.WriteZonedInt32].
//
// A nil v is [ErrNilValue] rather than a zero, as it is on
// [Writer.WritePackedBig]: an absent number and the number zero are different
// things, and a field is not written from a guess.
func (w *Writer) WriteZonedBig(v *big.Int, digits int, s SignPosition) error {
	if v == nil {
		return &OffsetError{Offset: w.off, Err: ErrNilValue}
	}
	return w.writeZoned(v.String(), v.Sign() < 0, digits, maxZonedDigits, s)
}

// writeZonedInt is the shared body of the two integer zoned writers, whose only
// difference is the digit count they accept.
func (w *Writer) writeZonedInt(v int64, digits, max int, s SignPosition) error {
	// Formatted rather than negated, so that the most negative int64 is
	// written like any other value instead of overflowing on its way out.
	return w.writeZoned(strconv.FormatInt(v, 10), v < 0, digits, max, s)
}

// writeZoned builds and writes one zoned decimal field from the decimal
// spelling of a value.
//
// The whole field is validated and built before a byte of it is written, so a
// rejected value writes nothing and cannot leave a half-field behind to
// desynchronize the record.
func (w *Writer) writeZoned(text string, negative bool, digits, max int, s SignPosition) error {
	if !s.valid() {
		return &OffsetError{Offset: w.off, Err: SignPositionError{SignPosition: s}}
	}
	if digits < 1 || digits > max {
		return &OffsetError{
			Offset: w.off,
			Err:    ZonedDigitCountError{Digits: digits, Max: max},
		}
	}
	if w.zonedErr != nil {
		return &OffsetError{Offset: w.off, Err: w.zonedErr}
	}
	magnitude := strings.TrimPrefix(text, "-")
	if len(magnitude) > digits || (negative && s == SignUnsigned) {
		return &OffsetError{
			Offset: w.off,
			Err:    ZonedRangeError{Value: text, Digits: digits, Sign: s},
		}
	}

	// Digit values, high first. Everything ahead of the value stays zero,
	// which is the high-order zero padding a COBOL MOVE stores.
	ds := make([]byte, digits)
	first := digits - len(magnitude)
	for i := 0; i < len(magnitude); i++ {
		ds[first+i] = magnitude[i] - '0'
	}

	// The separate sign byte, where there is one, sits outside the digits;
	// every other position is a digit byte, signed or not.
	field := make([]byte, zonedWidth(digits, s))
	digitBytes := field
	switch s {
	case SignLeadingSeparate:
		field[0] = w.zoned.bytes.separateSignByte(negative)
		digitBytes = field[1:]
	case SignTrailingSeparate:
		field[digits] = w.zoned.bytes.separateSignByte(negative)
		digitBytes = field[:digits]
	}
	if err := w.zoned.encodeField(digitBytes, ds, s.overpunchAt(digits), negative); err != nil {
		return &OffsetError{Offset: w.off, Err: err}
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

// WriteComp6Int32 writes v as a COMP-6 field of digits digits, exactly
// ceil(digits/2) bytes wide.
//
// COMP-6 is packed decimal with no sign nibble at all, so it is narrower than
// the COMP-3 of the same PICTURE and it takes no [Signedness]: there is nowhere
// in the field to record one. See [Reader.ReadComp6Int32].
//
// A negative v is a [PackedRangeError] rather than something encodable, for the
// reason a negative value written into an [Unsigned] packed field is: the
// encoding cannot express it, and writing its absolute value would produce a
// record that no longer says what the caller asked it to.
//
// digits must be between 1 and 9; see [Reader.ReadPackedInt32] for why the
// bound belongs to the accessor rather than to the field. v is the unscaled
// integer, since V occupies no storage.
func (w *Writer) WriteComp6Int32(v int32, digits int) error {
	return w.writeComp6Int(int64(v), digits, maxPackedInt32Digits)
}

// WriteComp6Int64 writes v as a COMP-6 field of digits digits, exactly
// ceil(digits/2) bytes wide. digits must be between 1 and 18; the 19-to-31
// digit range is written with [Writer.WriteComp6Big]. A negative v is a
// [PackedRangeError], as it is on [Writer.WriteComp6Int32].
func (w *Writer) WriteComp6Int64(v int64, digits int) error {
	return w.writeComp6Int(v, digits, maxPackedInt64Digits)
}

// WriteComp6Big writes v as a COMP-6 field of digits digits, exactly
// ceil(digits/2) bytes wide. digits must be between 1 and 31. A negative v is a
// [PackedRangeError], as it is on [Writer.WriteComp6Int32].
//
// A nil v is [ErrNilValue] rather than a zero, for the reason it is on
// [Writer.WritePackedBig]: an absent number and the number zero are different
// things.
func (w *Writer) WriteComp6Big(v *big.Int, digits int) error {
	if v == nil {
		return &OffsetError{Offset: w.off, Err: ErrNilValue}
	}
	return w.writeComp6(v.String(), v.Sign() < 0, digits, maxPackedDigits)
}

// writeComp6Int is the shared body of the two integer COMP-6 writers, whose
// only difference is the digit count they accept.
func (w *Writer) writeComp6Int(v int64, digits, max int) error {
	// Formatted rather than negated, for the reason writePackedInt does it:
	// the most negative int64 has no negation, and it is rejected below like
	// any other negative value rather than overflowing on its way out.
	return w.writeComp6(strconv.FormatInt(v, 10), v < 0, digits, max)
}

// writeComp6 builds and writes one COMP-6 field from the decimal spelling of a
// value.
//
// The whole field is validated and built before a byte of it is written, so a
// rejected value writes nothing and cannot leave a half-field behind to
// desynchronize the record.
func (w *Writer) writeComp6(text string, negative bool, digits, max int) error {
	if digits < 1 || digits > max {
		return &OffsetError{
			Offset: w.off,
			Err:    PackedDigitCountError{Digits: digits, Max: max},
		}
	}
	magnitude := strings.TrimPrefix(text, "-")
	if len(magnitude) > digits || negative {
		return &OffsetError{
			Offset: w.off,
			Err:    PackedRangeError{Value: text, Digits: digits, Signedness: Unsigned},
		}
	}

	// Nibbles high first. Everything ahead of the digits stays zero, which
	// covers both the high-order zeros the value does not fill and the pad
	// nibble an odd digit count leaves over — the opposite parity from
	// COMP-3, since there is no sign nibble making the count up.
	width := comp6Width(digits)
	nibbles := make([]byte, 2*width)
	first := len(nibbles) - len(magnitude)
	for i := 0; i < len(magnitude); i++ {
		nibbles[first+i] = magnitude[i] - '0'
	}

	field := make([]byte, width)
	for i := range field {
		field[i] = nibbles[2*i]<<4 | nibbles[2*i+1]
	}
	return w.write(field)
}

// WriteBinaryInt16 writes v as a binary (COMP, COMP-4, BINARY) field of digits
// digits, exactly 2 bytes wide.
//
// digits must be between 1 and 4. The bytes are two's complement in the order
// [Encoding.ByteOrder] declares, which is required and never inferred.
//
// s is required and says whether the item's PICTURE carries S, for the reason
// it is required on [Writer.WritePackedInt32]: a negative value written into an
// [Unsigned] field is a [BinaryRangeError] rather than a silent absolute value.
//
// Range semantics are TRUNC(STD): a value outside the decimal range of digits
// digits is rejected, which is what the compiler's own store would have
// truncated it to. Write a COMP-5 field, or a COMP field compiled under
// TRUNC(BIN), with [Writer.WriteComp5Int16] instead; see [Truncation].
//
// v is the unscaled integer: a PIC S9(3)V99 COMP item holding -123.45 is
// written as -12345 with digits 5, since V occupies no storage.
func (w *Writer) WriteBinaryInt16(v int16, digits int, s Signedness) error {
	return w.writeBinaryInt(int64(v), digits, maxBinaryInt16Digits, s, TruncStd)
}

// WriteBinaryInt32 writes v as a binary field of digits digits, 2 bytes wide
// for 1 to 4 digits and 4 bytes wide for 5 to 9. digits must be between 1 and
// 9, and s is required; see [Writer.WriteBinaryInt16].
func (w *Writer) WriteBinaryInt32(v int32, digits int, s Signedness) error {
	return w.writeBinaryInt(int64(v), digits, maxBinaryInt32Digits, s, TruncStd)
}

// WriteBinaryInt64 writes v as a binary field of digits digits, 2, 4 or 8 bytes
// wide by the digit count. digits must be between 1 and 18; the 19-to-31 digit
// range is written with [Writer.WriteBinaryBig]. s is required; see
// [Writer.WriteBinaryInt16].
func (w *Writer) WriteBinaryInt64(v int64, digits int, s Signedness) error {
	return w.writeBinaryInt(v, digits, maxBinaryInt64Digits, s, TruncStd)
}

// WriteBinaryUint64 writes v as a binary field of digits digits, 2, 4 or 8
// bytes wide by the digit count. digits must be between 1 and 18.
//
// s is required here too, and it is not implied by the argument type: a uint64
// cannot be negative, but [Signed] still selects the narrower range, since a
// signed 2-byte item stops at 32767 where an unsigned one runs to 65535. Under
// TRUNC(STD) both are further confined to the PICTURE's decimal range; see
// [Writer.WriteBinaryInt16].
func (w *Writer) WriteBinaryUint64(v uint64, digits int, s Signedness) error {
	return w.writeBinaryUint(v, digits, maxBinaryInt64Digits, s, TruncStd)
}

// WriteBinaryBig writes v as a binary field of digits digits, 2, 4, 8 or 16
// bytes wide by the digit count. digits must be between 1 and 31, the IBM
// Enterprise COBOL maximum under ARITH(EXTEND). s is required; see
// [Writer.WriteBinaryInt16].
//
// A nil v is [ErrNilValue] rather than a zero, as it is on
// [Writer.WritePackedBig]: an absent number and the number zero are different
// things, and a field is not written from a guess.
func (w *Writer) WriteBinaryBig(v *big.Int, digits int, s Signedness) error {
	return w.writeBinaryBig(v, digits, s, TruncStd)
}

// WriteComp5Int16 writes v as a COMP-5 field of digits digits, exactly 2 bytes
// wide.
//
// It is [Writer.WriteBinaryInt16] with TRUNC(BIN) range semantics: the value
// may use the full range of the storage rather than the decimal range of the
// PICTURE. Use it for USAGE COMP-5, and for COMP or COMP-4 compiled under
// TRUNC(BIN) or GnuCOBOL's binary-truncate: no. See [Truncation].
func (w *Writer) WriteComp5Int16(v int16, digits int, s Signedness) error {
	return w.writeBinaryInt(int64(v), digits, maxBinaryInt16Digits, s, TruncBin)
}

// WriteComp5Int32 writes v as a COMP-5 field of digits digits, 2 or 4 bytes
// wide. It is [Writer.WriteBinaryInt32] with TRUNC(BIN) range semantics; see
// [Writer.WriteComp5Int16].
func (w *Writer) WriteComp5Int32(v int32, digits int, s Signedness) error {
	return w.writeBinaryInt(int64(v), digits, maxBinaryInt32Digits, s, TruncBin)
}

// WriteComp5Int64 writes v as a COMP-5 field of digits digits, 2, 4 or 8 bytes
// wide. It is [Writer.WriteBinaryInt64] with TRUNC(BIN) range semantics; see
// [Writer.WriteComp5Int16].
func (w *Writer) WriteComp5Int64(v int64, digits int, s Signedness) error {
	return w.writeBinaryInt(v, digits, maxBinaryInt64Digits, s, TruncBin)
}

// WriteComp5Uint64 writes v as a COMP-5 field of digits digits, 2, 4 or 8 bytes
// wide. It is [Writer.WriteBinaryUint64] with TRUNC(BIN) range semantics; see
// [Writer.WriteComp5Int16].
//
// This is what a PIC 9(4) COMP-5 item holding 65535 needs: FF FF is legal there
// and outside the range TRUNC(STD) allows.
func (w *Writer) WriteComp5Uint64(v uint64, digits int, s Signedness) error {
	return w.writeBinaryUint(v, digits, maxBinaryInt64Digits, s, TruncBin)
}

// WriteComp5Big writes v as a COMP-5 field of digits digits, 2, 4, 8 or 16
// bytes wide. It is [Writer.WriteBinaryBig] with TRUNC(BIN) range semantics;
// see [Writer.WriteComp5Int16].
//
// An [Unsigned] field is written as a magnitude over the full storage width, so
// a 16-byte one may carry a value with its top bit set. [Reader.ReadComp5Big]
// reads two's complement and would report such a value as negative; below 8
// bytes, [Reader.ReadComp5Uint64] is the unsigned reading that recovers it.
func (w *Writer) WriteComp5Big(v *big.Int, digits int, s Signedness) error {
	return w.writeBinaryBig(v, digits, s, TruncBin)
}

// binaryField validates the arguments every binary writer shares and reports
// the field's storage width.
func (w *Writer) binaryField(digits, max int, s Signedness) (int, error) {
	if !s.valid() {
		return 0, &OffsetError{Offset: w.off, Err: SignednessError{Signedness: s}}
	}
	if digits < 1 || digits > max {
		return 0, &OffsetError{
			Offset: w.off,
			Err:    BinaryDigitCountError{Digits: digits, Max: max},
		}
	}
	return binaryWidth(digits), nil
}

// writeBinaryInt is the shared body of the signed fixed-width writers, whose
// only differences are the digit count they accept and the truncation mode they
// range-check under.
//
// The value is checked and the whole field built before a byte of it is
// written, so a rejected value writes nothing and cannot leave a half-field
// behind to desynchronize the record.
func (w *Writer) writeBinaryInt(v int64, digits, max int, s Signedness, t Truncation) error {
	width, err := w.binaryField(digits, max, s)
	if err != nil {
		return err
	}
	if !binaryIntFits(v, digits, width, s, t) {
		return &OffsetError{
			Offset: w.off,
			Err: BinaryRangeError{
				Value:      strconv.FormatInt(v, 10),
				Digits:     digits,
				Width:      width,
				Signedness: s,
				Truncation: t,
			},
		}
	}
	field := make([]byte, width)
	putBinaryUint(w.enc.ByteOrder, field, uint64(v))
	return w.write(field)
}

// writeBinaryUint is writeBinaryInt for a value that cannot be negative. The
// stored bytes are the same for any value both can express; what differs is the
// upper bound, which for an [Unsigned] item under [TruncBin] is one bit wider.
func (w *Writer) writeBinaryUint(v uint64, digits, max int, s Signedness, t Truncation) error {
	width, err := w.binaryField(digits, max, s)
	if err != nil {
		return err
	}
	if !binaryUintFits(v, digits, width, s, t) {
		return &OffsetError{
			Offset: w.off,
			Err: BinaryRangeError{
				Value:      strconv.FormatUint(v, 10),
				Digits:     digits,
				Width:      width,
				Signedness: s,
				Truncation: t,
			},
		}
	}
	field := make([]byte, width)
	putBinaryUint(w.enc.ByteOrder, field, v)
	return w.write(field)
}

// writeBinaryBig is the shared body of the two [math/big.Int] writers. It is
// separate from writeBinaryInt because [binary.ByteOrder] has no 16-byte
// accessor: the widest fields are ordered a byte at a time.
func (w *Writer) writeBinaryBig(v *big.Int, digits int, s Signedness, t Truncation) error {
	if v == nil {
		return &OffsetError{Offset: w.off, Err: ErrNilValue}
	}
	width, err := w.binaryField(digits, maxBinaryDigits, s)
	if err != nil {
		return err
	}
	if !binaryBigFits(v, digits, width, s, t) {
		return &OffsetError{
			Offset: w.off,
			Err: BinaryRangeError{
				Value:      v.String(),
				Digits:     digits,
				Width:      width,
				Signedness: s,
				Truncation: t,
			},
		}
	}

	// Two's complement: a negative value is stored as itself plus 2^(8*width),
	// which is exactly the bit pattern the reader sign-extends back.
	magnitude := v
	if v.Sign() < 0 {
		magnitude = new(big.Int).Add(v, new(big.Int).Lsh(big.NewInt(1), uint(8*width)))
	}
	b := magnitude.Bytes()
	field := make([]byte, width)
	copy(field[width-len(b):], b)
	orderBinaryBytes(w.enc.ByteOrder, field)
	return w.write(field)
}

// WriteFloat32 writes v as a floating point (COMP-1) field, exactly 4 bytes
// wide. It is the counterpart of [Reader.ReadFloat32].
//
// There is no digit count and no [Signedness] to pass: COMP-1 takes no PICTURE,
// so there is no digit count to declare and no S clause to select a sign
// convention from — a floating point item always carries its own sign.
//
// The bytes written come entirely from [Encoding.Float]. Under [FloatIEEE] they
// are binary32 in the order [Encoding.ByteOrder] declares; under [FloatHFP]
// they are IBM hexadecimal floating point and big-endian regardless of that
// axis, for the reason [Reader.ReadFloat32] gives.
//
// Under [FloatHFP] a NaN or an infinity is a [FloatRangeError] and writes
// nothing: HFP has no encoding for either, so there is no bit pattern that
// would read back as anything but a plausible finite number. Every finite
// float32 is in range, since binary32 reaches neither end of HFP's. It may lose
// up to three bits of precision to hex normalization, which is HFP's own
// wobble and not this package's rounding; see [hfpFromFloat].
func (w *Writer) WriteFloat32(v float32) error {
	field := make([]byte, comp1Width)
	if w.enc.Float == FloatIEEE {
		w.enc.ByteOrder.PutUint32(field, math.Float32bits(v))
		return w.write(field)
	}
	raw, err := hfpFromFloat(float64(v), comp1Width)
	if err != nil {
		return &OffsetError{Offset: w.off, Err: err}
	}
	binary.BigEndian.PutUint32(field, uint32(raw))
	return w.write(field)
}

// WriteFloat64 writes v as a floating point (COMP-2) field, exactly 8 bytes
// wide. It is [Writer.WriteFloat32] one width up and takes the same arguments —
// which is to say only the value — for the same reasons.
//
// Under [FloatHFP] a NaN, an infinity, or a magnitude outside 16^-65 to 16^63
// is a [FloatRangeError] and writes nothing. Unlike the COMP-1 case the range
// bounds are reachable here: a float64 runs to 1.8e308 where HFP stops at about
// 7.2e75, and down to 5e-324 where HFP stops at about 5.4e-79. Every float64
// that is in range is stored exactly, since HFP long's 56 bits of fraction
// leave room for the three that hex normalization can cost a 53-bit
// significand.
func (w *Writer) WriteFloat64(v float64) error {
	field := make([]byte, comp2Width)
	if w.enc.Float == FloatIEEE {
		w.enc.ByteOrder.PutUint64(field, math.Float64bits(v))
		return w.write(field)
	}
	raw, err := hfpFromFloat(v, comp2Width)
	if err != nil {
		return &OffsetError{Offset: w.off, Err: err}
	}
	binary.BigEndian.PutUint64(field, raw)
	return w.write(field)
}

// binaryIntFits reports whether v can be stored in a binary field of the given
// digit count and width under the given signedness and truncation mode.
func binaryIntFits(v int64, digits, width int, s Signedness, t Truncation) bool {
	if s == Unsigned && v < 0 {
		return false
	}
	if t == TruncStd {
		limit := int64(pow10[digits] - 1)
		return v >= -limit && v <= limit
	}
	if width >= 8 {
		// Every int64 fits eight bytes, signed or not.
		return true
	}
	if s == Unsigned {
		return v <= int64(1)<<(8*width)-1
	}
	return v >= -(int64(1)<<(8*width-1)) && v <= int64(1)<<(8*width-1)-1
}

// binaryUintFits is binaryIntFits for a value that cannot be negative.
func binaryUintFits(v uint64, digits, width int, s Signedness, t Truncation) bool {
	if t == TruncStd {
		return v <= pow10[digits]-1
	}
	if s == Signed {
		if width >= 8 {
			return v <= math.MaxInt64
		}
		return v <= uint64(1)<<(8*width-1)-1
	}
	if width >= 8 {
		return true
	}
	return v <= uint64(1)<<(8*width)-1
}

// binaryBigFits is binaryIntFits for the widths no Go integer type covers.
func binaryBigFits(v *big.Int, digits, width int, s Signedness, t Truncation) bool {
	if s == Unsigned && v.Sign() < 0 {
		return false
	}
	if t == TruncStd {
		return v.CmpAbs(decimalLimit(digits)) <= 0
	}
	one := big.NewInt(1)
	if s == Unsigned {
		max := new(big.Int).Sub(new(big.Int).Lsh(one, uint(8*width)), one)
		return v.Cmp(max) <= 0
	}
	max := new(big.Int).Sub(new(big.Int).Lsh(one, uint(8*width-1)), one)
	min := new(big.Int).Neg(new(big.Int).Lsh(one, uint(8*width-1)))
	return v.Cmp(min) >= 0 && v.Cmp(max) <= 0
}

// Marshal writes v under the given encoding and returns the bytes it produced.
//
// enc is the first argument and is required, for the reason [Encoding] exists:
// none of its four axes has a default, and every one of them fails silently
// when wrong.
//
// The returned slice is the caller's own: the [Writer] that produced it is
// dropped here and nothing else holds it. A caller encoding many records can
// keep one Writer and call [Writer.Reset] per record instead, which reuses both
// the Writer and the buffer that would otherwise be allocated for each of them.
func Marshal(enc Encoding, v Marshaler) ([]byte, error) {
	w, err := NewBytesWriter(nil, enc)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrNilValue
	}
	if err := v.MarshalCOBOL(w); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}
