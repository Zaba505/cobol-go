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
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// testRecord is a seven-field record exercising every accessor the package
// currently has: two left-justified alphanumeric fields, a JUSTIFIED RIGHT one,
// a raw byte field, a signed packed decimal field, an unsigned one and a signed
// binary one. It stands in for the code a generator will emit, for
//
//	01 TEST-RECORD.
//	   05 ID     PIC X(6).
//	   05 NAME   PIC X(12).
//	   05 CODE   PIC X(4) JUSTIFIED RIGHT.
//	   05 RAW    PIC X(3).
//	   05 AMOUNT PIC S9(5) COMP-3.
//	   05 QTY    PIC 9(4)  COMP-3.
//	   05 SEQ    PIC S9(4) COMP.
type testRecord struct {
	ID     string
	Name   string
	Code   string
	Raw    []byte
	Amount int64
	Qty    int32
	Seq    int16
}

const (
	testRecordIDWidth      = 6
	testRecordNameWidth    = 12
	testRecordCodeWidth    = 4
	testRecordRawWidth     = 3
	testRecordAmountDigits = 5
	testRecordQtyDigits    = 4
	testRecordSeqDigits    = 4
)

// testRecordWidth is the record's length in bytes, packed and binary fields
// included. SEQ contributes two bytes and not four: a binary field's width is a
// staircase in its digit count, not the digit count itself.
var testRecordWidth = testRecordIDWidth + testRecordNameWidth + testRecordCodeWidth +
	testRecordRawWidth + packedWidth(testRecordAmountDigits) + packedWidth(testRecordQtyDigits) +
	binaryWidth(testRecordSeqDigits)

// MarshalCOBOL implements the [Marshaler] interface.
func (r *testRecord) MarshalCOBOL(w *Writer) error {
	if err := w.WriteAlphanumeric(r.ID, testRecordIDWidth); err != nil {
		return err
	}
	if err := w.WriteAlphanumeric(r.Name, testRecordNameWidth); err != nil {
		return err
	}
	if err := w.WriteAlphanumericJustified(r.Code, testRecordCodeWidth, JustifyRight); err != nil {
		return err
	}
	if err := w.WriteBytes(r.Raw); err != nil {
		return err
	}
	if err := w.WritePackedInt64(r.Amount, testRecordAmountDigits, Signed); err != nil {
		return err
	}
	if err := w.WritePackedInt32(r.Qty, testRecordQtyDigits, Unsigned); err != nil {
		return err
	}
	return w.WriteBinaryInt16(r.Seq, testRecordSeqDigits, Signed)
}

// UnmarshalCOBOL implements the [Unmarshaler] interface.
func (r *testRecord) UnmarshalCOBOL(rd *Reader) error {
	var err error
	if r.ID, err = rd.ReadAlphanumeric(testRecordIDWidth); err != nil {
		return err
	}
	if r.Name, err = rd.ReadAlphanumeric(testRecordNameWidth); err != nil {
		return err
	}
	if r.Code, err = rd.ReadAlphanumericJustified(testRecordCodeWidth, JustifyRight); err != nil {
		return err
	}
	if r.Raw, err = rd.ReadBytes(testRecordRawWidth); err != nil {
		return err
	}
	if r.Amount, err = rd.ReadPackedInt64(testRecordAmountDigits); err != nil {
		return err
	}
	if r.Qty, err = rd.ReadPackedInt32(testRecordQtyDigits); err != nil {
		return err
	}
	r.Seq, err = rd.ReadBinaryInt16(testRecordSeqDigits)
	return err
}

// binaryEncoding returns an encoding whose byte order is bo. Byte order is the
// only [Encoding] axis a binary field reads, and the named bundles cannot be
// used for the little-endian case: [MicroFocusASCII] declares
// [binary.NativeEndian], which is whatever the machine running the test
// happens to be.
func binaryEncoding(bo binary.ByteOrder) Encoding {
	enc := GnuCOBOLASCII()
	enc.ByteOrder = bo
	return enc
}

// binaryOrders is the pair every binary test runs both of: the same field in
// the two orders real files carry it in. A test states the big-endian bytes and
// the little-endian ones are their reversal.
var binaryOrders = []binary.ByteOrder{binary.BigEndian, binary.LittleEndian}

// inByteOrder returns be, which is big-endian, in the order bo declares.
func inByteOrder(bo binary.ByteOrder, be []byte) []byte {
	b := slices.Clone(be)
	orderBinaryBytes(bo, b)
	return b
}

func TestNewReader(t *testing.T) {
	t.Parallel()

	t.Run("nil reader", func(t *testing.T) {
		t.Parallel()

		_, err := NewReader(nil, GnuCOBOLASCII())
		require.ErrorIs(t, err, ErrNilReader)
	})

	t.Run("incomplete encoding names the field", func(t *testing.T) {
		t.Parallel()

		// The zero-value Encoding is the mistake this package exists to
		// make impossible: no Reader comes out of it.
		_, err := NewReader(bytes.NewReader(nil), Encoding{})

		var encErr EncodingError
		require.ErrorAs(t, err, &encErr)
		require.Equal(t, "Charset", encErr.Field)
	})

	t.Run("carries its encoding", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader(nil), IBMEnterprise())
		require.NoError(t, err)
		require.Equal(t, IBMEnterprise(), r.Encoding())
		require.Zero(t, r.Offset())
	})
}

func TestReaderReadAlphanumeric(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		enc     Encoding
		src     []byte
		width   int
		justify Justification
		want    string
	}{
		{
			name:  "ascii strips trailing padding",
			enc:   GnuCOBOLASCII(),
			src:   []byte("ACME  "),
			width: 6,
			want:  "ACME",
		},
		{
			name:  "ascii keeps interior spaces",
			enc:   GnuCOBOLASCII(),
			src:   []byte("A B   "),
			width: 6,
			want:  "A B",
		},
		{
			name:  "ascii full field has nothing to strip",
			enc:   GnuCOBOLASCII(),
			src:   []byte("ACMECO"),
			width: 6,
			want:  "ACMECO",
		},
		{
			name:  "ascii all padding reads empty",
			enc:   GnuCOBOLASCII(),
			src:   []byte("      "),
			width: 6,
			want:  "",
		},
		{
			name:  "zero width reads nothing",
			enc:   GnuCOBOLASCII(),
			src:   []byte("ACME"),
			width: 0,
			want:  "",
		},
		{
			name:  "ebcdic translates and strips 0x40",
			enc:   IBMEnterprise(),
			src:   []byte{0xC1, 0xC3, 0xD4, 0xC5, 0x40, 0x40},
			width: 6,
			want:  "ACME",
		},
		{
			name:    "justified right strips leading padding",
			enc:     GnuCOBOLASCII(),
			src:     []byte("  42"),
			width:   4,
			justify: JustifyRight,
			want:    "42",
		},
		{
			name:    "justified right keeps trailing spaces",
			enc:     GnuCOBOLASCII(),
			src:     []byte(" 42 "),
			width:   4,
			justify: JustifyRight,
			want:    "42 ",
		},
		{
			name:    "ebcdic justified right strips leading 0x40",
			enc:     IBMEnterprise(),
			src:     []byte{0x40, 0x40, 0xF4, 0xF2},
			width:   4,
			justify: JustifyRight,
			want:    "42",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := NewReader(bytes.NewReader(tc.src), tc.enc)
			require.NoError(t, err)

			got, err := r.ReadAlphanumericJustified(tc.width, tc.justify)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, int64(tc.width), r.Offset())
		})
	}

	t.Run("ReadAlphanumeric defaults to left justification", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("ACME  ")), GnuCOBOLASCII())
		require.NoError(t, err)

		got, err := r.ReadAlphanumeric(6)
		require.NoError(t, err)
		require.Equal(t, "ACME", got)
	})
}

func TestReaderReadBytes(t *testing.T) {
	t.Parallel()

	t.Run("returns the padding a string read strips", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("ACME  ")), GnuCOBOLASCII())
		require.NoError(t, err)

		got, err := r.ReadBytes(6)
		require.NoError(t, err)
		require.Equal(t, []byte("ACME  "), got)
	})

	t.Run("passes every byte value through untouched", func(t *testing.T) {
		t.Parallel()

		// A PIC X field routinely carries a binary payload; no byte value in
		// one is invalid and none may be coerced.
		src := make([]byte, 256)
		for i := range src {
			src[i] = byte(i)
		}

		r, err := NewReader(bytes.NewReader(src), IBMEnterprise())
		require.NoError(t, err)

		got, err := r.ReadBytes(len(src))
		require.NoError(t, err)
		require.Equal(t, src, got)
	})

	t.Run("zero width consumes nothing", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("AB")), GnuCOBOLASCII())
		require.NoError(t, err)

		got, err := r.ReadBytes(0)
		require.NoError(t, err)
		require.Empty(t, got)
		require.Zero(t, r.Offset())
	})
}

func TestReaderReadPacked(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		src    []byte
		digits int
		want   int64
	}{
		{
			name:   "odd digits, positive sign C",
			src:    []byte{0x12, 0x34, 0x5C}, // PIC S9(5) COMP-3, +12345
			digits: 5,
			want:   12345,
		},
		{
			name:   "odd digits, negative sign D",
			src:    []byte{0x12, 0x34, 0x5D}, // PIC S9(5) COMP-3, -12345
			digits: 5,
			want:   -12345,
		},
		{
			name:   "odd digits, unsigned sign F",
			src:    []byte{0x12, 0x34, 0x5F}, // PIC 9(5) COMP-3, 12345
			digits: 5,
			want:   12345,
		},
		{
			name:   "even digits, pad nibble then positive",
			src:    []byte{0x01, 0x23, 0x4C}, // PIC S9(4) COMP-3, +1234
			digits: 4,
			want:   1234,
		},
		{
			name:   "even digits, pad nibble then negative",
			src:    []byte{0x01, 0x23, 0x4D}, // PIC S9(4) COMP-3, -1234
			digits: 4,
			want:   -1234,
		},
		{
			name:   "even digits, pad nibble then unsigned",
			src:    []byte{0x01, 0x23, 0x4F}, // PIC 9(4) COMP-3, 1234
			digits: 4,
			want:   1234,
		},
		{
			name:   "single digit fills one byte",
			src:    []byte{0x7C}, // PIC S9(1) COMP-3, +7
			digits: 1,
			want:   7,
		},
		{
			name:   "zero",
			src:    []byte{0x00, 0x00, 0x0C}, // PIC S9(5) COMP-3, +0
			digits: 5,
			want:   0,
		},
		{
			name:   "high-order zeros are not digits of the value",
			src:    []byte{0x00, 0x04, 0x2C}, // PIC S9(5) COMP-3, +42
			digits: 5,
			want:   42,
		},
		{
			name:   "lenient sign A reads positive",
			src:    []byte{0x12, 0x34, 0x5A},
			digits: 5,
			want:   12345,
		},
		{
			name:   "lenient sign E reads positive",
			src:    []byte{0x12, 0x34, 0x5E},
			digits: 5,
			want:   12345,
		},
		{
			name:   "lenient sign B reads negative",
			src:    []byte{0x12, 0x34, 0x5B},
			digits: 5,
			want:   -12345,
		},
		{
			name:   "nine digits, the int32 maximum",
			src:    []byte{0x99, 0x99, 0x99, 0x99, 0x9D}, // PIC S9(9) COMP-3, -999999999
			digits: 9,
			want:   -999999999,
		},
		{
			name:   "eighteen digits, the int64 maximum",
			src:    []byte{0x09, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x9C},
			digits: 18,
			want:   999999999999999999,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// COMP-3 is charset-invariant: the same bytes mean the same
			// number in an EBCDIC file and an ASCII one.
			for _, enc := range []Encoding{IBMEnterprise(), GnuCOBOLASCII()} {
				r, err := NewReader(bytes.NewReader(tc.src), enc)
				require.NoError(t, err)

				got, err := r.ReadPackedInt64(tc.digits)
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
				require.Equal(t, int64(len(tc.src)), r.Offset())
				require.Equal(t, len(tc.src), packedWidth(tc.digits))

				if tc.digits <= maxPackedInt32Digits {
					r32, err := NewReader(bytes.NewReader(tc.src), enc)
					require.NoError(t, err)

					got32, err := r32.ReadPackedInt32(tc.digits)
					require.NoError(t, err)
					require.Equal(t, int32(tc.want), got32)
				}

				rb, err := NewReader(bytes.NewReader(tc.src), enc)
				require.NoError(t, err)

				gotBig, err := rb.ReadPackedBig(tc.digits)
				require.NoError(t, err)
				require.Equal(t, big.NewInt(tc.want), gotBig)
			}
		})
	}
}

func TestReaderReadPackedBig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		src    []byte
		digits int
		want   string
	}{
		{
			name:   "nineteen digits, one past int64",
			src:    []byte{0x12, 0x34, 0x56, 0x78, 0x90, 0x12, 0x34, 0x56, 0x78, 0x9C},
			digits: 19,
			want:   "1234567890123456789",
		},
		{
			name:   "thirty-one digits, the COBOL maximum, negative",
			src:    []byte{0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x9D},
			digits: 31,
			want:   "-9999999999999999999999999999999",
		},
		{
			name:   "thirty digits, the widest even count, unsigned",
			src:    []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0x01, 0x23, 0x45, 0x67, 0x89, 0x01, 0x23, 0x45, 0x67, 0x89, 0x0F},
			digits: 30,
			want:   "123456789012345678901234567890",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := NewReader(bytes.NewReader(tc.src), IBMEnterprise())
			require.NoError(t, err)

			got, err := r.ReadPackedBig(tc.digits)
			require.NoError(t, err)
			require.Equal(t, tc.want, got.String())
			require.Equal(t, int64(packedWidth(tc.digits)), r.Offset())
		})
	}
}

func TestReaderReadPackedErrors(t *testing.T) {
	t.Parallel()

	t.Run("digit nibble above nine", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x12, 0x3A, 0x5C}), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadPackedInt64(5)

		var digitErr PackedDigitError
		require.ErrorAs(t, err, &digitErr)
		require.Equal(t, byte(0x0A), digitErr.Nibble)

		// The offset names the byte the bad nibble sits in, not the end of
		// the field: a corrupt packed field is diagnosable only if the byte
		// holding it can be found.
		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Equal(t, int64(1), offErr.Offset)
	})

	t.Run("digit nibble above nine in the last byte", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x12, 0x34, 0xFC}), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadPackedInt64(5)

		var digitErr PackedDigitError
		require.ErrorAs(t, err, &digitErr)
		require.Equal(t, byte(0x0F), digitErr.Nibble)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Equal(t, int64(2), offErr.Offset)
	})

	t.Run("sign nibble is a digit", func(t *testing.T) {
		t.Parallel()

		// 0-9 in the sign position cannot have come from a packed field.
		r, err := NewReader(bytes.NewReader([]byte{0x12, 0x34, 0x51}), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadPackedInt64(5)

		var signErr PackedSignError
		require.ErrorAs(t, err, &signErr)
		require.Equal(t, byte(0x01), signErr.Nibble)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Equal(t, int64(2), offErr.Offset)
	})

	t.Run("pad nibble is not zero", func(t *testing.T) {
		t.Parallel()

		// The cheapest available signal that the field offset is wrong.
		r, err := NewReader(bytes.NewReader([]byte{0x11, 0x23, 0x4C}), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadPackedInt64(4)

		var padErr PackedPadError
		require.ErrorAs(t, err, &padErr)
		require.Equal(t, byte(0x01), padErr.Nibble)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Zero(t, offErr.Offset)
	})

	t.Run("an odd digit count has no pad nibble to validate", func(t *testing.T) {
		t.Parallel()

		// The high nibble of the first byte is a digit here, not padding.
		r, err := NewReader(bytes.NewReader([]byte{0x12, 0x34, 0x5C}), GnuCOBOLASCII())
		require.NoError(t, err)

		got, err := r.ReadPackedInt64(5)
		require.NoError(t, err)
		require.Equal(t, int64(12345), got)
	})

	t.Run("digit count out of range", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name   string
			digits int
			max    int
			read   func(*Reader, int) error
		}{
			{
				name:   "zero digits",
				digits: 0,
				max:    maxPackedInt64Digits,
				read:   func(r *Reader, d int) error { _, err := r.ReadPackedInt64(d); return err },
			},
			{
				name:   "negative digits",
				digits: -1,
				max:    maxPackedInt64Digits,
				read:   func(r *Reader, d int) error { _, err := r.ReadPackedInt64(d); return err },
			},
			{
				name:   "ten digits overflows an int32",
				digits: 10,
				max:    maxPackedInt32Digits,
				read:   func(r *Reader, d int) error { _, err := r.ReadPackedInt32(d); return err },
			},
			{
				name:   "nineteen digits overflows an int64",
				digits: 19,
				max:    maxPackedInt64Digits,
				read:   func(r *Reader, d int) error { _, err := r.ReadPackedInt64(d); return err },
			},
			{
				name:   "thirty-two digits exceeds COBOL itself",
				digits: 32,
				max:    maxPackedDigits,
				read:   func(r *Reader, d int) error { _, err := r.ReadPackedBig(d); return err },
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				r, err := NewReader(bytes.NewReader(bytes.Repeat([]byte{0x00}, 32)), GnuCOBOLASCII())
				require.NoError(t, err)

				err = tc.read(r, tc.digits)

				var countErr PackedDigitCountError
				require.ErrorAs(t, err, &countErr)
				require.Equal(t, tc.digits, countErr.Digits)
				require.Equal(t, tc.max, countErr.Max)
				// The field was rejected before any byte was consumed, so
				// the record has not desynchronized.
				require.Zero(t, r.Offset())
			})
		}
	})

	t.Run("field cut short", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x12, 0x34}), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadPackedInt64(5)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		require.Equal(t, int64(2), r.Offset())
	})

	t.Run("end of stream reports io.EOF", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader(nil), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadPackedBig(5)
		require.ErrorIs(t, err, io.EOF)
	})
}

// TestReaderReadBinary reads a signed binary field in both byte orders, at
// every width and at both width boundaries. The vectors are SPEC Appendix A.5's
// where it has them.
func TestReaderReadBinary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		be     []byte
		digits int
		want   int64
	}{
		{
			name:   "four digits, positive",
			be:     []byte{0x04, 0xD2}, // PIC S9(4) COMP, +1234
			digits: 4,
			want:   1234,
		},
		{
			name:   "four digits, negative",
			be:     []byte{0xFB, 0x2E}, // PIC S9(4) COMP, -1234
			digits: 4,
			want:   -1234,
		},
		{
			name:   "four digits, zero",
			be:     []byte{0x00, 0x00},
			digits: 4,
			want:   0,
		},
		{
			name:   "four digits, the widest value TRUNC(STD) allows",
			be:     []byte{0x27, 0x0F}, // 9999
			digits: 4,
			want:   9999,
		},
		{
			name:   "four digits, the widest negative value",
			be:     []byte{0xD8, 0xF1}, // -9999
			digits: 4,
			want:   -9999,
		},
		{
			name:   "one digit still occupies two bytes",
			be:     []byte{0x00, 0x07}, // PIC S9(1) COMP, +7
			digits: 1,
			want:   7,
		},
		{
			name:   "four digits is the last two-byte width",
			be:     []byte{0x00, 0x2A}, // PIC S9(4) COMP, +42
			digits: 4,
			want:   42,
		},
		{
			name:   "five digits steps up to four bytes",
			be:     []byte{0x00, 0x00, 0x00, 0x2A}, // PIC S9(5) COMP, +42
			digits: 5,
			want:   42,
		},
		{
			name:   "five digits, the widest value",
			be:     []byte{0x00, 0x01, 0x86, 0x9F}, // 99999
			digits: 5,
			want:   99999,
		},
		{
			name:   "nine digits is the last four-byte width",
			be:     []byte{0x07, 0x5B, 0xCD, 0x15}, // PIC S9(9) COMP, +123456789
			digits: 9,
			want:   123456789,
		},
		{
			name:   "nine digits, negative",
			be:     []byte{0xF8, 0xA4, 0x32, 0xEB}, // -123456789
			digits: 9,
			want:   -123456789,
		},
		{
			name:   "ten digits steps up to eight bytes",
			be:     []byte{0x00, 0x00, 0x00, 0x00, 0x07, 0x5B, 0xCD, 0x15},
			digits: 10,
			want:   123456789,
		},
		{
			name:   "ten digits, the widest value",
			be:     []byte{0x00, 0x00, 0x00, 0x02, 0x54, 0x0B, 0xE3, 0xFF}, // 9999999999
			digits: 10,
			want:   9999999999,
		},
		{
			name:   "eighteen digits, the widest an int64 holds",
			be:     []byte{0x0D, 0xE0, 0xB6, 0xB3, 0xA7, 0x63, 0xFF, 0xFF},
			digits: 18,
			want:   999999999999999999,
		},
		{
			name:   "eighteen digits, negative",
			be:     []byte{0xF2, 0x1F, 0x49, 0x4C, 0x58, 0x9C, 0x00, 0x01},
			digits: 18,
			want:   -999999999999999999,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, bo := range binaryOrders {
				enc := binaryEncoding(bo)
				src := inByteOrder(bo, tc.be)

				r, err := NewReader(bytes.NewReader(src), enc)
				require.NoError(t, err)

				got, err := r.ReadBinaryInt64(tc.digits)
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
				require.Equal(t, int64(len(src)), r.Offset())
				require.Equal(t, len(src), binaryWidth(tc.digits))

				if tc.digits <= maxBinaryInt32Digits {
					r32, err := NewReader(bytes.NewReader(src), enc)
					require.NoError(t, err)

					got32, err := r32.ReadBinaryInt32(tc.digits)
					require.NoError(t, err)
					require.Equal(t, int32(tc.want), got32)
				}
				if tc.digits <= maxBinaryInt16Digits {
					r16, err := NewReader(bytes.NewReader(src), enc)
					require.NoError(t, err)

					got16, err := r16.ReadBinaryInt16(tc.digits)
					require.NoError(t, err)
					require.Equal(t, int16(tc.want), got16)
				}
				if tc.want >= 0 {
					ru, err := NewReader(bytes.NewReader(src), enc)
					require.NoError(t, err)

					gotu, err := ru.ReadBinaryUint64(tc.digits)
					require.NoError(t, err)
					require.Equal(t, uint64(tc.want), gotu)
				}

				rb, err := NewReader(bytes.NewReader(src), enc)
				require.NoError(t, err)

				gotBig, err := rb.ReadBinaryBig(tc.digits)
				require.NoError(t, err)
				require.Equal(t, big.NewInt(tc.want).String(), gotBig.String())

				// The same bytes read under TRUNC(BIN), which validates
				// nothing: the layout is identical, so the value is too.
				rc, err := NewReader(bytes.NewReader(src), enc)
				require.NoError(t, err)

				gotComp5, err := rc.ReadComp5Int64(tc.digits)
				require.NoError(t, err)
				require.Equal(t, tc.want, gotComp5)
			}
		})
	}
}

// TestReaderReadBinaryUnsigned covers the distinction PIC 9(n) draws from
// PIC S9(n): the same bytes, a different reading, and nothing in the file to
// say which was meant.
func TestReaderReadBinaryUnsigned(t *testing.T) {
	t.Parallel()

	t.Run("the same bytes read signed and unsigned", func(t *testing.T) {
		t.Parallel()

		// FF FF is -1 in a PIC S9(4) COMP item and 65535 in a
		// PIC 9(4) COMP-5 one. Only the copybook says which.
		src := []byte{0xFF, 0xFF}

		rs, err := NewReader(bytes.NewReader(src), binaryEncoding(binary.BigEndian))
		require.NoError(t, err)
		signed, err := rs.ReadBinaryInt16(4)
		require.NoError(t, err)
		require.Equal(t, int16(-1), signed)

		ru, err := NewReader(bytes.NewReader(src), binaryEncoding(binary.BigEndian))
		require.NoError(t, err)
		unsigned, err := ru.ReadComp5Uint64(4)
		require.NoError(t, err)
		require.Equal(t, uint64(65535), unsigned)
	})

	t.Run("unsigned within the picture range", func(t *testing.T) {
		t.Parallel()

		for _, bo := range binaryOrders {
			r, err := NewReader(bytes.NewReader(inByteOrder(bo, []byte{0x27, 0x0F})), binaryEncoding(bo))
			require.NoError(t, err)

			got, err := r.ReadBinaryUint64(4)
			require.NoError(t, err)
			require.Equal(t, uint64(9999), got)
		}
	})

	t.Run("the full unsigned eight-byte range", func(t *testing.T) {
		t.Parallel()

		src := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
		r, err := NewReader(bytes.NewReader(src), binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		got, err := r.ReadComp5Uint64(18)
		require.NoError(t, err)
		require.Equal(t, uint64(math.MaxUint64), got)
	})
}

// TestReaderReadBinaryBig covers the 19-to-31 digit range, which is 16 bytes
// wide and which no Go integer type holds.
func TestReaderReadBinaryBig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		be     []byte
		digits int
		want   string
	}{
		{
			name:   "nineteen digits, one past int64",
			be:     []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x11, 0x22, 0x10, 0xF4, 0x7D, 0xE9, 0x81, 0x15},
			digits: 19,
			want:   "1234567890123456789",
		},
		{
			name:   "thirty-one digits, the COBOL maximum",
			be:     []byte{0x00, 0x00, 0x00, 0x7E, 0x37, 0xBE, 0x20, 0x22, 0xC0, 0x91, 0x4B, 0x26, 0x7F, 0xFF, 0xFF, 0xFF},
			digits: 31,
			want:   "9999999999999999999999999999999",
		},
		{
			name:   "thirty-one digits, negative",
			be:     []byte{0xFF, 0xFF, 0xFF, 0x81, 0xC8, 0x41, 0xDF, 0xDD, 0x3F, 0x6E, 0xB4, 0xD9, 0x80, 0x00, 0x00, 0x01},
			digits: 31,
			want:   "-9999999999999999999999999999999",
		},
		{
			name:   "sixteen bytes holding a small value",
			be:     []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0xD2},
			digits: 19,
			want:   "1234",
		},
		{
			name:   "sixteen bytes holding a small negative value",
			be:     []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFB, 0x2E},
			digits: 19,
			want:   "-1234",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, bo := range binaryOrders {
				src := inByteOrder(bo, tc.be)
				r, err := NewReader(bytes.NewReader(src), binaryEncoding(bo))
				require.NoError(t, err)

				got, err := r.ReadBinaryBig(tc.digits)
				require.NoError(t, err)
				require.Equal(t, tc.want, got.String())
				require.Equal(t, int64(16), r.Offset())
			}
		})
	}
}

// TestReaderReadBinaryWidth pins the width staircase itself: PIC 9(5) COMP is
// four bytes and not five, and a wrong step here shifts every later field in
// the record.
func TestReaderReadBinaryWidth(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		digits int
		want   int
	}{
		{digits: 1, want: 2},
		{digits: 2, want: 2},
		{digits: 4, want: 2},
		{digits: 5, want: 4},
		{digits: 9, want: 4},
		{digits: 10, want: 8},
		{digits: 18, want: 8},
		{digits: 19, want: 16},
		{digits: 31, want: 16},
	}

	for _, tc := range testCases {
		t.Run(strconv.Itoa(tc.digits)+" digits", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, binaryWidth(tc.digits))

			src := make([]byte, tc.want)
			r, err := NewReader(bytes.NewReader(src), binaryEncoding(binary.BigEndian))
			require.NoError(t, err)

			_, err = r.ReadBinaryBig(tc.digits)
			require.NoError(t, err)
			require.Equal(t, int64(tc.want), r.Offset())
		})
	}
}

func TestReaderReadBinaryErrors(t *testing.T) {
	t.Parallel()

	t.Run("digit count above the accessor's maximum", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x00, 0x00, 0x00, 0x00}), binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		_, err = r.ReadBinaryInt16(5)

		var countErr BinaryDigitCountError
		require.ErrorAs(t, err, &countErr)
		require.Equal(t, 5, countErr.Digits)
		require.Equal(t, maxBinaryInt16Digits, countErr.Max)
		// Rejected before a byte was consumed, so the record has not
		// desynchronized.
		require.Zero(t, r.Offset())
	})

	t.Run("digit count of zero", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x00, 0x00}), binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		_, err = r.ReadBinaryInt64(0)

		var countErr BinaryDigitCountError
		require.ErrorAs(t, err, &countErr)
		require.Equal(t, 0, countErr.Digits)
	})

	t.Run("digit count above the big maximum", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader(make([]byte, 16)), binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		_, err = r.ReadBinaryBig(32)

		var countErr BinaryDigitCountError
		require.ErrorAs(t, err, &countErr)
		require.Equal(t, maxBinaryDigits, countErr.Max)
	})

	t.Run("field cut short", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x04}), binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		_, err = r.ReadBinaryInt16(4)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Equal(t, int64(1), offErr.Offset)
	})

	t.Run("wrong byte order caught by the TRUNC(STD) range", func(t *testing.T) {
		t.Parallel()

		// 04 D2 is +1234 big-endian, which is what wrote it. Read
		// little-endian the same two bytes are -11772, outside what
		// PIC S9(4) can express — the one detector this package has for a
		// misdeclared Encoding.ByteOrder.
		r, err := NewReader(bytes.NewReader([]byte{0x04, 0xD2}), binaryEncoding(binary.LittleEndian))
		require.NoError(t, err)

		_, err = r.ReadBinaryInt16(4)

		var rangeErr BinaryRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "-11772", rangeErr.Value)
		require.Equal(t, 4, rangeErr.Digits)
		require.Equal(t, 2, rangeErr.Width)
		require.Equal(t, Signed, rangeErr.Signedness)
		require.Equal(t, TruncStd, rangeErr.Truncation)

		// The offset names the byte the field starts at, not the one it
		// ends at: the error is a statement about the whole field.
		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Equal(t, int64(0), offErr.Offset)
	})

	t.Run("the same bytes are legal under TRUNC(BIN)", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x04, 0xD2}), binaryEncoding(binary.LittleEndian))
		require.NoError(t, err)

		got, err := r.ReadComp5Int16(4)
		require.NoError(t, err)
		require.Equal(t, int16(-11772), got)
	})

	t.Run("unsigned value outside the picture range", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0xFF, 0xFF}), binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		_, err = r.ReadBinaryUint64(4)

		var rangeErr BinaryRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "65535", rangeErr.Value)
		require.Equal(t, Unsigned, rangeErr.Signedness)
	})

	t.Run("big value outside the picture range", func(t *testing.T) {
		t.Parallel()

		// 2^127 - 1, which is far above the 31-digit decimal maximum.
		src := []byte{0x7F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
		r, err := NewReader(bytes.NewReader(src), binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		_, err = r.ReadBinaryBig(31)

		var rangeErr BinaryRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, TruncStd, rangeErr.Truncation)

		// Under TRUNC(BIN) the same bytes are an ordinary value.
		rc, err := NewReader(bytes.NewReader(src), binaryEncoding(binary.BigEndian))
		require.NoError(t, err)
		got, err := rc.ReadComp5Big(31)
		require.NoError(t, err)
		require.Equal(t, "170141183460469231731687303715884105727", got.String())
	})
}

func TestReaderOffset(t *testing.T) {
	t.Parallel()

	t.Run("advances field by field", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("ACME  WIDGET      12ZZ")), GnuCOBOLASCII())
		require.NoError(t, err)

		require.Zero(t, r.Offset())

		_, err = r.ReadAlphanumeric(6)
		require.NoError(t, err)
		require.Equal(t, int64(6), r.Offset())

		_, err = r.ReadAlphanumeric(12)
		require.NoError(t, err)
		require.Equal(t, int64(18), r.Offset())

		_, err = r.ReadBytes(4)
		require.NoError(t, err)
		require.Equal(t, int64(22), r.Offset())
	})

	t.Run("counts the bytes a short read consumed", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("ACME")), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadAlphanumeric(10)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		require.Equal(t, int64(4), r.Offset())
	})
}

func TestReaderErrors(t *testing.T) {
	t.Parallel()

	t.Run("field cut short reports where it stopped", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("ACME  WID")), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadAlphanumeric(6)
		require.NoError(t, err)

		_, err = r.ReadAlphanumeric(12)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Equal(t, int64(9), offErr.Offset)
	})

	t.Run("end of stream reports io.EOF", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("ACME  ")), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadAlphanumeric(6)
		require.NoError(t, err)

		// A caller stepping through records detects the last one this way,
		// which is why an exhausted stream is not ErrUnexpectedEOF.
		_, err = r.ReadAlphanumeric(6)
		require.ErrorIs(t, err, io.EOF)
		require.NotErrorIs(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("negative width", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("ACME")), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadBytes(-1)

		var widthErr FieldWidthError
		require.ErrorAs(t, err, &widthErr)
		require.Equal(t, -1, widthErr.Width)
		require.Zero(t, r.Offset())
	})

	t.Run("unknown justification", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("ACME")), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadAlphanumericJustified(4, Justification(9))

		var justErr JustificationError
		require.ErrorAs(t, err, &justErr)
		require.Equal(t, Justification(9), justErr.Justification)
		// The field was rejected before any byte was consumed, so the
		// record has not desynchronized.
		require.Zero(t, r.Offset())
	})
}

func TestUnmarshal(t *testing.T) {
	t.Parallel()

	t.Run("reads a record", func(t *testing.T) {
		t.Parallel()

		// "A12345" | "WIDGET GRIP " | "  42" | 00 01 FF | 12 34 5C | 00 04 2F
		// | 04 D2: SEQ +1234, big-endian as GnuCOBOL writes it by default
		data := []byte("A12345WIDGET GRIP   42\x00\x01\xFF\x12\x34\x5C\x00\x04\x2F\x04\xD2")
		require.Len(t, data, testRecordWidth)

		var got testRecord
		require.NoError(t, Unmarshal(GnuCOBOLASCII(), data, &got))
		require.Equal(t, testRecord{
			ID:     "A12345",
			Name:   "WIDGET GRIP",
			Code:   "42",
			Raw:    []byte{0x00, 0x01, 0xFF},
			Amount: 12345,
			Qty:    42,
			Seq:    1234,
		}, got)
	})

	t.Run("incomplete encoding", func(t *testing.T) {
		t.Parallel()

		var got testRecord
		err := Unmarshal(Encoding{Charset: ASCII(), ByteOrder: binary.BigEndian, Float: FloatIEEE}, nil, &got)

		var encErr EncodingError
		require.ErrorAs(t, err, &encErr)
		require.Equal(t, "Sign", encErr.Field)
	})

	t.Run("nil value", func(t *testing.T) {
		t.Parallel()

		require.ErrorIs(t, Unmarshal(GnuCOBOLASCII(), nil, nil), ErrNilValue)
	})

	t.Run("short record", func(t *testing.T) {
		t.Parallel()

		var got testRecord
		err := Unmarshal(GnuCOBOLASCII(), []byte("A12345WIDGET"), &got)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})
}
