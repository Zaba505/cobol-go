// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"bytes"
	"encoding/binary"
	"io"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// testRecord is a six-field record exercising every accessor the package
// currently has: two left-justified alphanumeric fields, a JUSTIFIED RIGHT one,
// a raw byte field, a signed packed decimal field and an unsigned one. It
// stands in for the code a generator will emit, for
//
//	01 TEST-RECORD.
//	   05 ID     PIC X(6).
//	   05 NAME   PIC X(12).
//	   05 CODE   PIC X(4) JUSTIFIED RIGHT.
//	   05 RAW    PIC X(3).
//	   05 AMOUNT PIC S9(5) COMP-3.
//	   05 QTY    PIC 9(4)  COMP-3.
type testRecord struct {
	ID     string
	Name   string
	Code   string
	Raw    []byte
	Amount int64
	Qty    int32
}

const (
	testRecordIDWidth      = 6
	testRecordNameWidth    = 12
	testRecordCodeWidth    = 4
	testRecordRawWidth     = 3
	testRecordAmountDigits = 5
	testRecordQtyDigits    = 4
)

// testRecordWidth is the record's length in bytes, packed fields included.
var testRecordWidth = testRecordIDWidth + testRecordNameWidth + testRecordCodeWidth +
	testRecordRawWidth + packedWidth(testRecordAmountDigits) + packedWidth(testRecordQtyDigits)

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
	return w.WritePackedInt32(r.Qty, testRecordQtyDigits, Unsigned)
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
	r.Qty, err = rd.ReadPackedInt32(testRecordQtyDigits)
	return err
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
		data := []byte("A12345WIDGET GRIP   42\x00\x01\xFF\x12\x34\x5C\x00\x04\x2F")
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
