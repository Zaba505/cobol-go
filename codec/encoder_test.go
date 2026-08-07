// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// failingWriter fails after letting the first limit bytes through, so that a
// write error can be observed with a known offset.
type failingWriter struct {
	limit int
	n     int
}

var errWriteFailed = errors.New("write failed")

func (w *failingWriter) Write(p []byte) (int, error) {
	remaining := w.limit - w.n
	if remaining <= 0 {
		return 0, errWriteFailed
	}
	if len(p) <= remaining {
		w.n += len(p)
		return len(p), nil
	}
	w.n = w.limit
	return remaining, errWriteFailed
}

func TestNewWriter(t *testing.T) {
	t.Parallel()

	t.Run("nil writer", func(t *testing.T) {
		t.Parallel()

		_, err := NewWriter(nil, GnuCOBOLASCII())
		require.ErrorIs(t, err, ErrNilWriter)
	})

	t.Run("incomplete encoding names the field", func(t *testing.T) {
		t.Parallel()

		_, err := NewWriter(&bytes.Buffer{}, Encoding{
			Charset:   ASCII(),
			Sign:      SignASCIIZone37,
			ByteOrder: binary.BigEndian,
		})

		var encErr EncodingError
		require.ErrorAs(t, err, &encErr)
		require.Equal(t, "Float", encErr.Field)
	})

	t.Run("carries its encoding", func(t *testing.T) {
		t.Parallel()

		w, err := NewWriter(&bytes.Buffer{}, IBMEnterprise())
		require.NoError(t, err)
		require.Equal(t, IBMEnterprise(), w.Encoding())
		require.Zero(t, w.Offset())
	})
}

func TestWriterWriteAlphanumeric(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		enc     Encoding
		value   string
		width   int
		justify Justification
		want    []byte
	}{
		{
			name:  "ascii pads on the right",
			enc:   GnuCOBOLASCII(),
			value: "ACME",
			width: 6,
			want:  []byte("ACME  "),
		},
		{
			name:  "ascii exact fit pads nothing",
			enc:   GnuCOBOLASCII(),
			value: "ACMECO",
			width: 6,
			want:  []byte("ACMECO"),
		},
		{
			name:  "empty value is all padding",
			enc:   GnuCOBOLASCII(),
			value: "",
			width: 4,
			want:  []byte("    "),
		},
		{
			name:  "zero width writes nothing",
			enc:   GnuCOBOLASCII(),
			value: "",
			width: 0,
			want:  nil, // nothing reached the buffer at all
		},
		{
			name:  "ebcdic translates and pads with 0x40",
			enc:   IBMEnterprise(),
			value: "ACME",
			width: 6,
			want:  []byte{0xC1, 0xC3, 0xD4, 0xC5, 0x40, 0x40},
		},
		{
			name:    "justified right pads on the left",
			enc:     GnuCOBOLASCII(),
			value:   "42",
			width:   4,
			justify: JustifyRight,
			want:    []byte("  42"),
		},
		{
			name:    "ebcdic justified right pads on the left with 0x40",
			enc:     IBMEnterprise(),
			value:   "42",
			width:   4,
			justify: JustifyRight,
			want:    []byte{0x40, 0x40, 0xF4, 0xF2},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			w, err := NewWriter(&buf, tc.enc)
			require.NoError(t, err)

			require.NoError(t, w.WriteAlphanumericJustified(tc.value, tc.width, tc.justify))
			require.Equal(t, tc.want, buf.Bytes())
			require.Equal(t, int64(tc.width), w.Offset())
		})
	}

	t.Run("WriteAlphanumeric defaults to left justification", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		require.NoError(t, w.WriteAlphanumeric("ACME", 6))
		require.Equal(t, []byte("ACME  "), buf.Bytes())
	})
}

func TestWriterWriteBytes(t *testing.T) {
	t.Parallel()

	t.Run("writes what it is given, untranslated", func(t *testing.T) {
		t.Parallel()

		src := make([]byte, 256)
		for i := range src {
			src[i] = byte(i)
		}

		var buf bytes.Buffer
		// An EBCDIC encoding must not touch these: they are a payload, not
		// characters.
		w, err := NewWriter(&buf, IBMEnterprise())
		require.NoError(t, err)

		require.NoError(t, w.WriteBytes(src))
		require.Equal(t, src, buf.Bytes())
		require.Equal(t, int64(256), w.Offset())
	})

	t.Run("empty write is a no-op", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		require.NoError(t, w.WriteBytes(nil))
		require.Zero(t, w.Offset())
		require.Empty(t, buf.Bytes())
	})
}

func TestWriterWritePacked(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		value      int64
		digits     int
		signedness Signedness
		want       []byte
	}{
		{
			name:       "odd digits, positive writes sign C",
			value:      12345,
			digits:     5,
			signedness: Signed,
			want:       []byte{0x12, 0x34, 0x5C},
		},
		{
			name:       "odd digits, negative writes sign D",
			value:      -12345,
			digits:     5,
			signedness: Signed,
			want:       []byte{0x12, 0x34, 0x5D},
		},
		{
			name:       "odd digits, unsigned writes sign F",
			value:      12345,
			digits:     5,
			signedness: Unsigned,
			want:       []byte{0x12, 0x34, 0x5F},
		},
		{
			name:       "even digits write a zero pad nibble",
			value:      -1234,
			digits:     4,
			signedness: Signed,
			want:       []byte{0x01, 0x23, 0x4D},
		},
		{
			name:       "even digits, unsigned",
			value:      1234,
			digits:     4,
			signedness: Unsigned,
			want:       []byte{0x01, 0x23, 0x4F},
		},
		{
			name:       "value narrower than the field is zero filled",
			value:      42,
			digits:     5,
			signedness: Signed,
			want:       []byte{0x00, 0x04, 0x2C},
		},
		{
			name:       "zero is positive, not unsigned",
			value:      0,
			digits:     5,
			signedness: Signed,
			want:       []byte{0x00, 0x00, 0x0C},
		},
		{
			name:       "unsigned zero",
			value:      0,
			digits:     4,
			signedness: Unsigned,
			want:       []byte{0x00, 0x00, 0x0F},
		},
		{
			name:       "single digit fills one byte",
			value:      -7,
			digits:     1,
			signedness: Signed,
			want:       []byte{0x7D},
		},
		{
			name:       "nine digits, the int32 maximum",
			value:      999999999,
			digits:     9,
			signedness: Signed,
			want:       []byte{0x99, 0x99, 0x99, 0x99, 0x9C},
		},
		{
			name:       "eighteen digits, the int64 maximum",
			value:      -999999999999999999,
			digits:     18,
			signedness: Signed,
			want:       []byte{0x09, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x9D},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// COMP-3 is charset-invariant: the same value writes the same
			// bytes under an EBCDIC encoding and an ASCII one.
			for _, enc := range []Encoding{IBMEnterprise(), GnuCOBOLASCII()} {
				var buf bytes.Buffer
				w, err := NewWriter(&buf, enc)
				require.NoError(t, err)

				require.NoError(t, w.WritePackedInt64(tc.value, tc.digits, tc.signedness))
				require.Equal(t, tc.want, buf.Bytes())
				require.Equal(t, int64(packedWidth(tc.digits)), w.Offset())

				if tc.digits <= maxPackedInt32Digits {
					var buf32 bytes.Buffer
					w32, err := NewWriter(&buf32, enc)
					require.NoError(t, err)

					require.NoError(t, w32.WritePackedInt32(int32(tc.value), tc.digits, tc.signedness))
					require.Equal(t, tc.want, buf32.Bytes())
				}

				var bufBig bytes.Buffer
				wBig, err := NewWriter(&bufBig, enc)
				require.NoError(t, err)

				require.NoError(t, wBig.WritePackedBig(big.NewInt(tc.value), tc.digits, tc.signedness))
				require.Equal(t, tc.want, bufBig.Bytes())
			}
		})
	}
}

func TestWriterWritePackedBig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		value      string
		digits     int
		signedness Signedness
		want       []byte
	}{
		{
			name:       "nineteen digits, one past int64",
			value:      "1234567890123456789",
			digits:     19,
			signedness: Signed,
			want:       []byte{0x12, 0x34, 0x56, 0x78, 0x90, 0x12, 0x34, 0x56, 0x78, 0x9C},
		},
		{
			name:       "thirty-one digits, the COBOL maximum, negative",
			value:      "-9999999999999999999999999999999",
			digits:     31,
			signedness: Signed,
			want: []byte{
				0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
				0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x9D,
			},
		},
		{
			name:       "thirty digits, the widest even count, unsigned",
			value:      "123456789012345678901234567890",
			digits:     30,
			signedness: Unsigned,
			want: []byte{
				0x01, 0x23, 0x45, 0x67, 0x89, 0x01, 0x23, 0x45,
				0x67, 0x89, 0x01, 0x23, 0x45, 0x67, 0x89, 0x0F,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, ok := new(big.Int).SetString(tc.value, 10)
			require.True(t, ok)

			var buf bytes.Buffer
			w, err := NewWriter(&buf, IBMEnterprise())
			require.NoError(t, err)

			require.NoError(t, w.WritePackedBig(v, tc.digits, tc.signedness))
			require.Equal(t, tc.want, buf.Bytes())
		})
	}
}

func TestWriterWritePackedErrors(t *testing.T) {
	t.Parallel()

	t.Run("value wider than the field", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		// A COBOL MOVE would drop the high-order digit; a codec doing the
		// same would write a record that no longer says what it was given.
		err = w.WritePackedInt64(123456, 5, Signed)

		var rangeErr PackedRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "123456", rangeErr.Value)
		require.Equal(t, 5, rangeErr.Digits)
		require.Equal(t, Signed, rangeErr.Signedness)
		require.Empty(t, buf.Bytes())
		require.Zero(t, w.Offset())
	})

	t.Run("negative value in an unsigned field", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		// Storing it as its absolute value is what COBOL does and what this
		// package will not do silently.
		err = w.WritePackedInt32(-42, 5, Unsigned)

		var rangeErr PackedRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "-42", rangeErr.Value)
		require.Equal(t, Unsigned, rangeErr.Signedness)
		require.Empty(t, buf.Bytes())
	})

	t.Run("negative big value in an unsigned field", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WritePackedBig(big.NewInt(-1), 31, Unsigned)

		var rangeErr PackedRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "-1", rangeErr.Value)
		require.Empty(t, buf.Bytes())
	})

	t.Run("signedness left unset", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		// Whether the PICTURE carries S is not recoverable from the value,
		// so the zero value is rejected rather than guessed at.
		err = w.WritePackedInt64(1, 5, SignednessUnset)

		var signErr SignednessError
		require.ErrorAs(t, err, &signErr)
		require.Equal(t, SignednessUnset, signErr.Signedness)
		require.Empty(t, buf.Bytes())
	})

	t.Run("signedness out of range", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WritePackedInt32(1, 5, Signedness(9))

		var signErr SignednessError
		require.ErrorAs(t, err, &signErr)
		require.Equal(t, Signedness(9), signErr.Signedness)
		require.Empty(t, buf.Bytes())
	})

	t.Run("digit count out of range", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name   string
			digits int
			max    int
			write  func(*Writer, int) error
		}{
			{
				name:   "zero digits",
				digits: 0,
				max:    maxPackedInt64Digits,
				write:  func(w *Writer, d int) error { return w.WritePackedInt64(0, d, Signed) },
			},
			{
				name:   "negative digits",
				digits: -1,
				max:    maxPackedInt32Digits,
				write:  func(w *Writer, d int) error { return w.WritePackedInt32(0, d, Signed) },
			},
			{
				name:   "ten digits overflows an int32",
				digits: 10,
				max:    maxPackedInt32Digits,
				write:  func(w *Writer, d int) error { return w.WritePackedInt32(0, d, Signed) },
			},
			{
				name:   "nineteen digits overflows an int64",
				digits: 19,
				max:    maxPackedInt64Digits,
				write:  func(w *Writer, d int) error { return w.WritePackedInt64(0, d, Signed) },
			},
			{
				name:   "thirty-two digits exceeds COBOL itself",
				digits: 32,
				max:    maxPackedDigits,
				write:  func(w *Writer, d int) error { return w.WritePackedBig(big.NewInt(0), d, Signed) },
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var buf bytes.Buffer
				w, err := NewWriter(&buf, GnuCOBOLASCII())
				require.NoError(t, err)

				err = tc.write(w, tc.digits)

				var countErr PackedDigitCountError
				require.ErrorAs(t, err, &countErr)
				require.Equal(t, tc.digits, countErr.Digits)
				require.Equal(t, tc.max, countErr.Max)
				require.Empty(t, buf.Bytes())
				require.Zero(t, w.Offset())
			})
		}
	})

	t.Run("nil big value", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		// An absent number and the number zero are different things.
		err = w.WritePackedBig(nil, 5, Signed)
		require.ErrorIs(t, err, ErrNilValue)
		require.Empty(t, buf.Bytes())
	})

	t.Run("the most negative int64 is written like any other value", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		// -9223372036854775808 has no positive counterpart, so a writer that
		// negated it would report the wrong magnitude.
		err = w.WritePackedInt64(math.MinInt64, 18, Signed)

		var rangeErr PackedRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "-9223372036854775808", rangeErr.Value)

		// 19 digits is past the int64 accessor's own bound, so the value is
		// written through the big one, magnitude intact.
		require.NoError(t, w.WritePackedBig(big.NewInt(math.MinInt64), 19, Signed))
		require.Equal(t, []byte{0x92, 0x23, 0x37, 0x20, 0x36, 0x85, 0x47, 0x75, 0x80, 0x8D}, buf.Bytes())
	})

	t.Run("underlying writer fails", func(t *testing.T) {
		t.Parallel()

		w, err := NewWriter(&failingWriter{limit: 1}, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WritePackedInt64(12345, 5, Signed)
		require.ErrorIs(t, err, errWriteFailed)
	})
}

// TestWriterWriteBinary writes a signed binary field in both byte orders, at
// every width and at both width boundaries. It is the mirror of
// TestReaderReadBinary and shares its vectors.
func TestWriterWriteBinary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		v      int64
		digits int
		want   []byte // big-endian; the little-endian case is its reversal
	}{
		{
			name:   "four digits, positive",
			v:      1234,
			digits: 4,
			want:   []byte{0x04, 0xD2},
		},
		{
			name:   "four digits, negative",
			v:      -1234,
			digits: 4,
			want:   []byte{0xFB, 0x2E},
		},
		{
			name:   "four digits, zero",
			v:      0,
			digits: 4,
			want:   []byte{0x00, 0x00},
		},
		{
			name:   "four digits, the widest value TRUNC(STD) allows",
			v:      9999,
			digits: 4,
			want:   []byte{0x27, 0x0F},
		},
		{
			name:   "four digits, the widest negative value",
			v:      -9999,
			digits: 4,
			want:   []byte{0xD8, 0xF1},
		},
		{
			name:   "one digit still occupies two bytes",
			v:      7,
			digits: 1,
			want:   []byte{0x00, 0x07},
		},
		{
			name:   "five digits steps up to four bytes",
			v:      42,
			digits: 5,
			want:   []byte{0x00, 0x00, 0x00, 0x2A},
		},
		{
			name:   "nine digits is the last four-byte width",
			v:      123456789,
			digits: 9,
			want:   []byte{0x07, 0x5B, 0xCD, 0x15},
		},
		{
			name:   "nine digits, negative",
			v:      -123456789,
			digits: 9,
			want:   []byte{0xF8, 0xA4, 0x32, 0xEB},
		},
		{
			name:   "ten digits steps up to eight bytes",
			v:      123456789,
			digits: 10,
			want:   []byte{0x00, 0x00, 0x00, 0x00, 0x07, 0x5B, 0xCD, 0x15},
		},
		{
			name:   "eighteen digits, the widest an int64 holds",
			v:      999999999999999999,
			digits: 18,
			want:   []byte{0x0D, 0xE0, 0xB6, 0xB3, 0xA7, 0x63, 0xFF, 0xFF},
		},
		{
			name:   "eighteen digits, negative",
			v:      -999999999999999999,
			digits: 18,
			want:   []byte{0xF2, 0x1F, 0x49, 0x4C, 0x58, 0x9C, 0x00, 0x01},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, bo := range binaryOrders {
				enc := binaryEncoding(bo)
				want := inByteOrder(bo, tc.want)

				var buf bytes.Buffer
				w, err := NewWriter(&buf, enc)
				require.NoError(t, err)
				require.NoError(t, w.WriteBinaryInt64(tc.v, tc.digits, Signed))
				require.Equal(t, want, buf.Bytes())
				require.Equal(t, int64(binaryWidth(tc.digits)), w.Offset())

				if tc.digits <= maxBinaryInt32Digits {
					var buf32 bytes.Buffer
					w32, err := NewWriter(&buf32, enc)
					require.NoError(t, err)
					require.NoError(t, w32.WriteBinaryInt32(int32(tc.v), tc.digits, Signed))
					require.Equal(t, want, buf32.Bytes())
				}
				if tc.digits <= maxBinaryInt16Digits {
					var buf16 bytes.Buffer
					w16, err := NewWriter(&buf16, enc)
					require.NoError(t, err)
					require.NoError(t, w16.WriteBinaryInt16(int16(tc.v), tc.digits, Signed))
					require.Equal(t, want, buf16.Bytes())
				}
				if tc.v >= 0 {
					var bufu bytes.Buffer
					wu, err := NewWriter(&bufu, enc)
					require.NoError(t, err)
					require.NoError(t, wu.WriteBinaryUint64(uint64(tc.v), tc.digits, Unsigned))
					require.Equal(t, want, bufu.Bytes())
				}

				var bufBig bytes.Buffer
				wb, err := NewWriter(&bufBig, enc)
				require.NoError(t, err)
				require.NoError(t, wb.WriteBinaryBig(big.NewInt(tc.v), tc.digits, Signed))
				require.Equal(t, want, bufBig.Bytes())

				// TRUNC(BIN) changes which values are accepted and never
				// the bytes a value is written as.
				var bufComp5 bytes.Buffer
				wc, err := NewWriter(&bufComp5, enc)
				require.NoError(t, err)
				require.NoError(t, wc.WriteComp5Int64(tc.v, tc.digits, Signed))
				require.Equal(t, want, bufComp5.Bytes())
			}
		})
	}
}

// TestWriterWriteBinaryUnsigned covers the values only an unsigned item holds,
// which is the half of the storage range TRUNC(BIN) opens up.
func TestWriterWriteBinaryUnsigned(t *testing.T) {
	t.Parallel()

	t.Run("the full two-byte unsigned range under COMP-5", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		// PIC 9(4) COMP-5 holding 65535, SPEC Appendix A.5's last row.
		require.NoError(t, w.WriteComp5Uint64(65535, 4, Unsigned))
		require.Equal(t, []byte{0xFF, 0xFF}, buf.Bytes())
	})

	t.Run("the same value is out of range signed", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		err = w.WriteComp5Uint64(65535, 4, Signed)

		var rangeErr BinaryRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, Signed, rangeErr.Signedness)
		require.Empty(t, buf.Bytes())
	})

	t.Run("a negative value is refused by an unsigned field", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		// A COBOL MOVE would have stored the absolute value; this is loud.
		err = w.WriteBinaryInt16(-1, 4, Unsigned)

		var rangeErr BinaryRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "-1", rangeErr.Value)
		require.Equal(t, Unsigned, rangeErr.Signedness)
		require.Empty(t, buf.Bytes())
	})
}

// TestWriterWriteBinaryBig covers the 19-to-31 digit range, which is 16 bytes
// wide and which no Go integer type holds.
func TestWriterWriteBinaryBig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		v      string
		digits int
		want   []byte
	}{
		{
			name:   "nineteen digits, one past int64",
			v:      "1234567890123456789",
			digits: 19,
			want:   []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x11, 0x22, 0x10, 0xF4, 0x7D, 0xE9, 0x81, 0x15},
		},
		{
			name:   "thirty-one digits, the COBOL maximum",
			v:      "9999999999999999999999999999999",
			digits: 31,
			want:   []byte{0x00, 0x00, 0x00, 0x7E, 0x37, 0xBE, 0x20, 0x22, 0xC0, 0x91, 0x4B, 0x26, 0x7F, 0xFF, 0xFF, 0xFF},
		},
		{
			name:   "thirty-one digits, negative",
			v:      "-9999999999999999999999999999999",
			digits: 31,
			want:   []byte{0xFF, 0xFF, 0xFF, 0x81, 0xC8, 0x41, 0xDF, 0xDD, 0x3F, 0x6E, 0xB4, 0xD9, 0x80, 0x00, 0x00, 0x01},
		},
		{
			name:   "a small value still fills sixteen bytes",
			v:      "1234",
			digits: 19,
			want:   []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0xD2},
		},
		{
			name:   "a small negative value is sign-extended to sixteen bytes",
			v:      "-1234",
			digits: 19,
			want:   []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFB, 0x2E},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, ok := new(big.Int).SetString(tc.v, 10)
			require.True(t, ok)

			for _, bo := range binaryOrders {
				var buf bytes.Buffer
				w, err := NewWriter(&buf, binaryEncoding(bo))
				require.NoError(t, err)

				require.NoError(t, w.WriteBinaryBig(v, tc.digits, Signed))
				require.Equal(t, inByteOrder(bo, tc.want), buf.Bytes())
			}
		})
	}
}

func TestWriterWriteBinaryErrors(t *testing.T) {
	t.Parallel()

	t.Run("value with more digits than the picture", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		// 10000 fits the two bytes but not the four digits, which is what
		// TRUNC(STD) means and what the compiler would have truncated.
		err = w.WriteBinaryInt16(10000, 4, Signed)

		var rangeErr BinaryRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "10000", rangeErr.Value)
		require.Equal(t, 4, rangeErr.Digits)
		require.Equal(t, 2, rangeErr.Width)
		require.Equal(t, TruncStd, rangeErr.Truncation)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Zero(t, offErr.Offset)
		// Nothing was written: a rejected field must not desynchronize the
		// record it sits in.
		require.Empty(t, buf.Bytes())
		require.Zero(t, w.Offset())
	})

	t.Run("the same value is in range under TRUNC(BIN)", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		require.NoError(t, w.WriteComp5Int16(10000, 4, Signed))
		require.Equal(t, []byte{0x27, 0x10}, buf.Bytes())
	})

	t.Run("value beyond the storage width under TRUNC(BIN)", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		// Two bytes signed stop at 32767, whatever the truncation mode.
		err = w.WriteComp5Int32(40000, 4, Signed)

		var rangeErr BinaryRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, TruncBin, rangeErr.Truncation)
		require.Empty(t, buf.Bytes())
	})

	t.Run("digit count above the accessor's maximum", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		err = w.WriteBinaryInt32(1, 10, Signed)

		var countErr BinaryDigitCountError
		require.ErrorAs(t, err, &countErr)
		require.Equal(t, 10, countErr.Digits)
		require.Equal(t, maxBinaryInt32Digits, countErr.Max)
		require.Empty(t, buf.Bytes())
	})

	t.Run("digit count of zero", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		err = w.WriteBinaryBig(big.NewInt(1), 0, Signed)

		var countErr BinaryDigitCountError
		require.ErrorAs(t, err, &countErr)
		require.Equal(t, 0, countErr.Digits)
	})

	t.Run("unset signedness", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		// Whether the PICTURE carries S is not recoverable from the value,
		// so it is stated per call and never defaulted.
		err = w.WriteBinaryInt16(1, 4, SignednessUnset)

		var signErr SignednessError
		require.ErrorAs(t, err, &signErr)
		require.Equal(t, SignednessUnset, signErr.Signedness)
		require.Empty(t, buf.Bytes())
	})

	t.Run("unknown signedness", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		err = w.WriteBinaryUint64(1, 4, Signedness(9))

		var signErr SignednessError
		require.ErrorAs(t, err, &signErr)
		require.Empty(t, buf.Bytes())
	})

	t.Run("nil big value", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		require.ErrorIs(t, w.WriteBinaryBig(nil, 19, Signed), ErrNilValue)
		require.Empty(t, buf.Bytes())
	})

	t.Run("underlying writer fails", func(t *testing.T) {
		t.Parallel()

		w, err := NewWriter(&failingWriter{limit: 0}, binaryEncoding(binary.BigEndian))
		require.NoError(t, err)

		err = w.WriteBinaryInt16(1, 4, Signed)
		require.ErrorIs(t, err, errWriteFailed)
	})
}

func TestWriterOffset(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w, err := NewWriter(&buf, GnuCOBOLASCII())
	require.NoError(t, err)

	require.Zero(t, w.Offset())
	require.NoError(t, w.WriteAlphanumeric("A12345", 6))
	require.Equal(t, int64(6), w.Offset())
	require.NoError(t, w.WriteAlphanumeric("WIDGET", 12))
	require.Equal(t, int64(18), w.Offset())
	require.NoError(t, w.WriteBytes([]byte{0x00, 0x01}))
	require.Equal(t, int64(20), w.Offset())
}

func TestWriterErrors(t *testing.T) {
	t.Parallel()

	t.Run("value longer than the field", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WriteAlphanumeric("ACME CORPORATION", 6)

		var tooLong FieldTooLongError
		require.ErrorAs(t, err, &tooLong)
		require.Equal(t, 16, tooLong.Len)
		require.Equal(t, 6, tooLong.Width)
		// Nothing was written: a rejected field must not desynchronize the
		// record it sits in.
		require.Empty(t, buf.Bytes())
		require.Zero(t, w.Offset())
	})

	t.Run("character the charset cannot spell", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, IBMEnterprise())
		require.NoError(t, err)

		err = w.WriteAlphanumeric("10 €", 8)

		var badRune UnrepresentableRuneError
		require.ErrorAs(t, err, &badRune)
		require.Equal(t, '€', badRune.Rune)
		require.Equal(t, "cp037", badRune.Charset)
		require.Empty(t, buf.Bytes())
	})

	t.Run("negative width", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WriteAlphanumeric("", -1)

		var widthErr FieldWidthError
		require.ErrorAs(t, err, &widthErr)
		require.Equal(t, -1, widthErr.Width)
	})

	t.Run("unknown justification", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WriteAlphanumericJustified("A", 4, Justification(9))

		var justErr JustificationError
		require.ErrorAs(t, err, &justErr)
		require.Empty(t, buf.Bytes())
	})

	t.Run("underlying writer fails mid-record", func(t *testing.T) {
		t.Parallel()

		w, err := NewWriter(&failingWriter{limit: 8}, GnuCOBOLASCII())
		require.NoError(t, err)

		require.NoError(t, w.WriteAlphanumeric("A12345", 6))
		err = w.WriteAlphanumeric("WIDGET", 12)
		require.ErrorIs(t, err, errWriteFailed)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Equal(t, int64(8), offErr.Offset)
		require.Equal(t, int64(8), w.Offset())
	})

	t.Run("short write is reported", func(t *testing.T) {
		t.Parallel()

		// A writer that accepts fewer bytes than it was given without
		// reporting an error still lost data.
		w, err := NewWriter(&shortWriter{limit: 4}, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WriteAlphanumeric("ACME", 6)
		require.ErrorIs(t, err, io.ErrShortWrite)
	})
}

// shortWriter accepts at most limit bytes per call and reports no error, the
// contract violation io.ErrShortWrite exists for.
type shortWriter struct{ limit int }

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		return w.limit, nil
	}
	return len(p), nil
}

func TestMarshal(t *testing.T) {
	t.Parallel()

	t.Run("writes a record", func(t *testing.T) {
		t.Parallel()

		got, err := Marshal(GnuCOBOLASCII(), &testRecord{
			ID:     "A12345",
			Name:   "WIDGET GRIP",
			Code:   "42",
			Raw:    []byte{0x00, 0x01, 0xFF},
			Amount: 12345,
			Qty:    42,
			Seq:    1234,
		})
		require.NoError(t, err)
		require.Equal(t, []byte("A12345WIDGET GRIP   42\x00\x01\xFF\x12\x34\x5C\x00\x04\x2F\x04\xD2"), got)
	})

	t.Run("incomplete encoding", func(t *testing.T) {
		t.Parallel()

		_, err := Marshal(Encoding{Sign: SignEBCDIC, ByteOrder: binary.BigEndian, Float: FloatHFP}, &testRecord{})

		var encErr EncodingError
		require.ErrorAs(t, err, &encErr)
		require.Equal(t, "Charset", encErr.Field)
	})

	t.Run("nil value", func(t *testing.T) {
		t.Parallel()

		_, err := Marshal(GnuCOBOLASCII(), nil)
		require.ErrorIs(t, err, ErrNilValue)
	})
}

// TestRoundTripDecodeEncode is the byte-equality direction: bytes read out of a
// record and written straight back must reproduce the record exactly, padding
// included. It is what proves the trimming a reader does is undone by the
// padding a writer does.
func TestRoundTripDecodeEncode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		enc  Encoding
		src  []byte
	}{
		{
			name: "ascii record",
			enc:  GnuCOBOLASCII(),
			// … | 12 34 5C: AMOUNT +12345 | 00 04 2F: QTY 42 | 04 D2: SEQ 1234
			src: []byte("A12345WIDGET GRIP   42\x00\x01\xFF\x12\x34\x5C\x00\x04\x2F\x04\xD2"),
		},
		{
			name: "ascii record with empty fields",
			enc:  MicroFocusASCII(),
			// The zero of a signed packed field is C, of an unsigned one F.
			// SEQ is zero, which is the one binary value this encoding's
			// native byte order cannot reorder.
			src: []byte("      " + "            " + "    " + "\x00\x00\x00" +
				"\x00\x00\x0C" + "\x00\x00\x0F" + "\x00\x00"),
		},
		{
			name: "ascii record with full fields",
			enc:  ConvertedFromEBCDIC(),
			// 98 76 5D: AMOUNT -98765 | 09 99 9F: QTY 9999 | FB 2E: SEQ -1234,
			// big-endian because the mainframe that wrote it was
			src: []byte("A12345WIDGETGRIP123456\xFF\xFE\xFD\x98\x76\x5D\x09\x99\x9F\xFB\x2E"),
		},
		{
			name: "ebcdic record",
			enc:  IBMEnterprise(),
			src: append(
				[]byte{
					0xC1, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, // "A12345"
					0xE6, 0xC9, 0xC4, 0xC7, 0xC5, 0xE3, 0x40, 0x40, 0x40, 0x40, 0x40, 0x40, // "WIDGET      "
					0x40, 0x40, 0xF4, 0xF2, // "  42"
				},
				// The packed bytes are byte-identical to the ASCII case
				// above: COMP-3 is charset-invariant.
				0x00, 0x01, 0xFF, // raw payload
				0x12, 0x34, 0x5C, // AMOUNT +12345
				0x00, 0x04, 0x2F, // QTY 42
				0x04, 0xD2, // SEQ 1234, big-endian as z/OS writes it
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var rec testRecord
			require.NoError(t, Unmarshal(tc.enc, tc.src, &rec))

			got, err := Marshal(tc.enc, &rec)
			require.NoError(t, err)
			require.Equal(t, tc.src, got)
		})
	}
}

// TestRoundTripPacked walks a packed decimal field both ways at every digit
// count COBOL allows, which is the cheapest check that the width formula, the
// pad nibble and the digit placement agree between the reader and the writer.
func TestRoundTripPacked(t *testing.T) {
	t.Parallel()

	signednesses := []Signedness{Signed, Unsigned}

	for digits := 1; digits <= maxPackedDigits; digits++ {
		t.Run(strconv.Itoa(digits)+" digits", func(t *testing.T) {
			t.Parallel()

			// The widest value the field holds, its negation, a value with
			// high-order zeros, and zero itself.
			nines, ok := new(big.Int).SetString(strings.Repeat("9", digits), 10)
			require.True(t, ok)
			values := []*big.Int{
				nines,
				new(big.Int).Neg(nines),
				big.NewInt(0),
				big.NewInt(7),
			}

			for _, s := range signednesses {
				for _, v := range values {
					if v.Sign() < 0 && s == Unsigned {
						continue
					}

					var buf bytes.Buffer
					w, err := NewWriter(&buf, IBMEnterprise())
					require.NoError(t, err)
					require.NoError(t, w.WritePackedBig(v, digits, s))
					require.Len(t, buf.Bytes(), packedWidth(digits))

					r, err := NewReader(bytes.NewReader(buf.Bytes()), IBMEnterprise())
					require.NoError(t, err)
					got, err := r.ReadPackedBig(digits)
					require.NoError(t, err)
					require.Equal(t, v.String(), got.String())

					// The bytes a reader accepts are written back
					// unchanged, which is the direction that catches a
					// pad nibble or a sign nibble drifting.
					var back bytes.Buffer
					w2, err := NewWriter(&back, IBMEnterprise())
					require.NoError(t, err)
					require.NoError(t, w2.WritePackedBig(got, digits, s))
					require.Equal(t, buf.Bytes(), back.Bytes())

					if digits <= maxPackedInt64Digits {
						r64, err := NewReader(bytes.NewReader(buf.Bytes()), IBMEnterprise())
						require.NoError(t, err)
						got64, err := r64.ReadPackedInt64(digits)
						require.NoError(t, err)
						require.Equal(t, v.Int64(), got64)
					}
					if digits <= maxPackedInt32Digits {
						r32, err := NewReader(bytes.NewReader(buf.Bytes()), IBMEnterprise())
						require.NoError(t, err)
						got32, err := r32.ReadPackedInt32(digits)
						require.NoError(t, err)
						require.Equal(t, int32(v.Int64()), got32)
					}
				}
			}
		})
	}
}

// TestRoundTripPackedLenientSigns is the one place the round trip is
// deliberately not byte-equal: a reader accepts six sign nibbles and a writer
// emits three, so a field carrying A, B or E reads correctly and is written
// back in the canonical spelling.
func TestRoundTripPackedLenientSigns(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		src        []byte
		signedness Signedness
		want       int64
		wantBytes  []byte
	}{
		{
			name:       "A is written back as C",
			src:        []byte{0x12, 0x34, 0x5A},
			signedness: Signed,
			want:       12345,
			wantBytes:  []byte{0x12, 0x34, 0x5C},
		},
		{
			name:       "B is written back as D",
			src:        []byte{0x12, 0x34, 0x5B},
			signedness: Signed,
			want:       -12345,
			wantBytes:  []byte{0x12, 0x34, 0x5D},
		},
		{
			name:       "E is written back as C",
			src:        []byte{0x12, 0x34, 0x5E},
			signedness: Signed,
			want:       12345,
			wantBytes:  []byte{0x12, 0x34, 0x5C},
		},
		{
			name:       "F survives when the field is unsigned",
			src:        []byte{0x12, 0x34, 0x5F},
			signedness: Unsigned,
			want:       12345,
			wantBytes:  []byte{0x12, 0x34, 0x5F},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := NewReader(bytes.NewReader(tc.src), IBMEnterprise())
			require.NoError(t, err)
			got, err := r.ReadPackedInt64(5)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)

			var buf bytes.Buffer
			w, err := NewWriter(&buf, IBMEnterprise())
			require.NoError(t, err)
			require.NoError(t, w.WritePackedInt64(got, 5, tc.signedness))
			require.Equal(t, tc.wantBytes, buf.Bytes())
		})
	}
}

// TestRoundTripBinary walks a binary field both ways at every digit count COBOL
// allows, in both byte orders, under both truncation modes and both
// signednesses. It is the cheapest check that the width staircase, the byte
// order and the two's complement agree between the reader and the writer, and
// it is what covers the 4/5 and 9/10 width boundaries at every value extreme.
func TestRoundTripBinary(t *testing.T) {
	t.Parallel()

	for digits := 1; digits <= maxBinaryDigits; digits++ {
		t.Run(strconv.Itoa(digits)+" digits", func(t *testing.T) {
			t.Parallel()

			width := binaryWidth(digits)
			one := big.NewInt(1)
			stdLimit := decimalLimit(digits)
			binMax := new(big.Int).Sub(new(big.Int).Lsh(one, uint(8*width-1)), one)
			binMin := new(big.Int).Neg(new(big.Int).Lsh(one, uint(8*width-1)))

			// Each mode is exercised at the extremes of its own range: the
			// PICTURE's decimal limits under TRUNC(STD), the storage width's
			// under TRUNC(BIN).
			modes := []struct {
				name    string
				values  []*big.Int
				write   func(w *Writer, v *big.Int, s Signedness) error
				read    func(r *Reader) (*big.Int, error)
				read64  func(r *Reader) (int64, error)
				readU64 func(r *Reader) (uint64, error)
			}{
				{
					name:   "trunc-std",
					values: []*big.Int{stdLimit, new(big.Int).Neg(stdLimit), big.NewInt(0), big.NewInt(7)},
					write: func(w *Writer, v *big.Int, s Signedness) error {
						return w.WriteBinaryBig(v, digits, s)
					},
					read:    func(r *Reader) (*big.Int, error) { return r.ReadBinaryBig(digits) },
					read64:  func(r *Reader) (int64, error) { return r.ReadBinaryInt64(digits) },
					readU64: func(r *Reader) (uint64, error) { return r.ReadBinaryUint64(digits) },
				},
				{
					name:   "trunc-bin",
					values: []*big.Int{binMax, binMin, big.NewInt(0), big.NewInt(7)},
					write: func(w *Writer, v *big.Int, s Signedness) error {
						return w.WriteComp5Big(v, digits, s)
					},
					read:    func(r *Reader) (*big.Int, error) { return r.ReadComp5Big(digits) },
					read64:  func(r *Reader) (int64, error) { return r.ReadComp5Int64(digits) },
					readU64: func(r *Reader) (uint64, error) { return r.ReadComp5Uint64(digits) },
				},
			}

			for _, mode := range modes {
				for _, s := range []Signedness{Signed, Unsigned} {
					for _, v := range mode.values {
						if v.Sign() < 0 && s == Unsigned {
							continue
						}
						for _, bo := range binaryOrders {
							enc := binaryEncoding(bo)

							var buf bytes.Buffer
							w, err := NewWriter(&buf, enc)
							require.NoError(t, err, mode.name)
							require.NoError(t, mode.write(w, v, s), mode.name)
							require.Len(t, buf.Bytes(), width)

							r, err := NewReader(bytes.NewReader(buf.Bytes()), enc)
							require.NoError(t, err)
							got, err := mode.read(r)
							require.NoError(t, err, mode.name)
							require.Equal(t, v.String(), got.String(), mode.name)

							// The bytes a reader accepts are written back
							// unchanged, which is the direction that catches
							// a byte order or a sign extension drifting.
							var back bytes.Buffer
							w2, err := NewWriter(&back, enc)
							require.NoError(t, err)
							require.NoError(t, mode.write(w2, got, s))
							require.Equal(t, buf.Bytes(), back.Bytes())

							if digits <= maxBinaryInt64Digits {
								r64, err := NewReader(bytes.NewReader(buf.Bytes()), enc)
								require.NoError(t, err)
								got64, err := mode.read64(r64)
								require.NoError(t, err, mode.name)
								require.Equal(t, v.Int64(), got64)
							}
							if digits <= maxBinaryInt64Digits && v.Sign() >= 0 {
								ru, err := NewReader(bytes.NewReader(buf.Bytes()), enc)
								require.NoError(t, err)
								gotU, err := mode.readU64(ru)
								require.NoError(t, err, mode.name)
								require.Equal(t, v.Uint64(), gotU)
							}
						}
					}
				}
			}
		})
	}
}

// TestRoundTripBinaryUnsignedFullWidth is the half of the storage range only an
// unsigned COMP-5 item reaches: the values above the signed maximum, which the
// uint64 accessors are the reading of.
func TestRoundTripBinaryUnsignedFullWidth(t *testing.T) {
	t.Parallel()

	for digits := 1; digits <= maxBinaryInt64Digits; digits++ {
		t.Run(strconv.Itoa(digits)+" digits", func(t *testing.T) {
			t.Parallel()

			width := binaryWidth(digits)
			max := uint64(math.MaxUint64)
			if width < 8 {
				max = uint64(1)<<(8*width) - 1
			}

			for _, bo := range binaryOrders {
				enc := binaryEncoding(bo)

				var buf bytes.Buffer
				w, err := NewWriter(&buf, enc)
				require.NoError(t, err)
				require.NoError(t, w.WriteComp5Uint64(max, digits, Unsigned))
				require.Len(t, buf.Bytes(), width)
				require.Equal(t, bytes.Repeat([]byte{0xFF}, width), buf.Bytes())

				r, err := NewReader(bytes.NewReader(buf.Bytes()), enc)
				require.NoError(t, err)
				got, err := r.ReadComp5Uint64(digits)
				require.NoError(t, err)
				require.Equal(t, max, got)
			}
		})
	}
}

// TestRoundTripEncodeDecode is the value-equality direction: a record written
// and read back must compare equal to what went in.
func TestRoundTripEncodeDecode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		enc  Encoding
		rec  testRecord
	}{
		{
			name: "ascii",
			enc:  GnuCOBOLASCII(),
			rec: testRecord{
				ID:     "A12345",
				Name:   "WIDGET GRIP",
				Code:   "42",
				Raw:    []byte{0x00, 0x01, 0xFF},
				Amount: 12345,
				Qty:    42,
				Seq:    1234,
			},
		},
		{
			name: "ebcdic",
			enc:  IBMEnterprise(),
			rec: testRecord{
				ID:     "A12345",
				Name:   "WIDGET GRIP",
				Code:   "42",
				Raw:    []byte{0x00, 0x01, 0xFF},
				Amount: -12345,
				Qty:    42,
				Seq:    -1234,
			},
		},
		{
			name: "converted from ebcdic",
			enc:  ConvertedFromEBCDIC(),
			rec: testRecord{
				ID:     "B0",
				Name:   "",
				Code:   "",
				Raw:    []byte{0x40, 0x20, 0x00},
				Amount: 0,
				Qty:    0,
				Seq:    0,
			},
		},
		{
			name: "fields filling their width exactly",
			enc:  MicroFocusASCII(),
			rec: testRecord{
				ID:     "A12345",
				Name:   "WIDGETGRIP12",
				Code:   "3456",
				Raw:    []byte{0xFF, 0xFE, 0xFD},
				Amount: -99999,
				Qty:    9999,
				Seq:    9999,
			},
		},
		{
			name: "interior spaces survive",
			enc:  GnuCOBOLASCII(),
			rec: testRecord{
				ID:     "A B C",
				Name:   "W  I  D",
				Code:   "4 2",
				Raw:    []byte{0x01, 0x02, 0x03},
				Amount: -1,
				Qty:    1,
				Seq:    -1,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := Marshal(tc.enc, &tc.rec)
			require.NoError(t, err)
			require.Len(t, data, testRecordWidth)

			var got testRecord
			require.NoError(t, Unmarshal(tc.enc, data, &got))
			require.Equal(t, tc.rec, got)
		})
	}
}
