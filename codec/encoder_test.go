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

// TestWriterWriteZoned writes the test vectors of codec/SPEC.md, Appendix A.1
// and A.2, which the matching reader test decodes: the same numbers, stated as
// bytes in one direction and as values in the other.
func TestWriterWriteZoned(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		v      int64
		digits int
		sign   SignPosition
		enc    Encoding
		want   []byte
	}{
		{
			name:   "ebcdic trailing overpunch, positive",
			v:      12345,
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xC5},
		},
		{
			name:   "ebcdic trailing overpunch, negative",
			v:      -12345,
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xD5},
		},
		{
			name:   "ascii zone 3/7, positive",
			v:      12345,
			digits: 5,
			sign:   SignTrailing,
			enc:    GnuCOBOLASCII(),
			want:   []byte{0x31, 0x32, 0x33, 0x34, 0x35},
		},
		{
			name:   "ascii zone 3/7, negative",
			v:      -12345,
			digits: 5,
			sign:   SignTrailing,
			enc:    GnuCOBOLASCII(),
			want:   []byte{0x31, 0x32, 0x33, 0x34, 0x75}, // "1234u"
		},
		{
			name:   "translated ebcdic, positive",
			v:      12345,
			digits: 5,
			sign:   SignTrailing,
			enc:    ConvertedFromEBCDIC(),
			want:   []byte{0x31, 0x32, 0x33, 0x34, 0x45}, // "1234E"
		},
		{
			name:   "translated ebcdic, negative",
			v:      -12345,
			digits: 5,
			sign:   SignTrailing,
			enc:    ConvertedFromEBCDIC(),
			want:   []byte{0x31, 0x32, 0x33, 0x34, 0x4E}, // "1234N"
		},
		{
			name:   "realia, negative",
			v:      -12345,
			digits: 5,
			sign:   SignTrailing,
			enc:    zonedEncoding(ASCII(), SignRealia),
			want:   []byte{0x31, 0x32, 0x33, 0x34, 0x25}, // "1234%"
		},
		{
			name:   "ebcdic leading overpunch, negative",
			v:      -12345,
			digits: 5,
			sign:   SignLeading,
			enc:    IBMEnterprise(),
			want:   []byte{0xD1, 0xF2, 0xF3, 0xF4, 0xF5},
		},
		{
			name:   "ebcdic unsigned",
			v:      12345,
			digits: 5,
			sign:   SignUnsigned,
			enc:    IBMEnterprise(),
			want:   []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5},
		},
		{
			name:   "ascii unsigned",
			v:      12345,
			digits: 5,
			sign:   SignUnsigned,
			enc:    GnuCOBOLASCII(),
			want:   []byte{0x31, 0x32, 0x33, 0x34, 0x35},
		},
		{
			name:   "ascii leading separate, negative",
			v:      -12345,
			digits: 5,
			sign:   SignLeadingSeparate,
			enc:    GnuCOBOLASCII(),
			want:   []byte{0x2D, 0x31, 0x32, 0x33, 0x34, 0x35}, // "-12345"
		},
		{
			name:   "ebcdic leading separate, negative",
			v:      -12345,
			digits: 5,
			sign:   SignLeadingSeparate,
			enc:    IBMEnterprise(),
			want:   []byte{0x60, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5},
		},
		{
			name:   "ascii trailing separate, positive",
			v:      12345,
			digits: 5,
			sign:   SignTrailingSeparate,
			enc:    GnuCOBOLASCII(),
			want:   []byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x2B}, // "12345+"
		},
		{
			name:   "ebcdic trailing separate, positive",
			v:      12345,
			digits: 5,
			sign:   SignTrailingSeparate,
			enc:    IBMEnterprise(),
			want:   []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0x4E},
		},
		{
			name:   "high-order zero padding",
			v:      42,
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   []byte{0xF0, 0xF0, 0xF0, 0xF4, 0xC2},
		},
		{
			name:   "zero is written positive",
			v:      0,
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   []byte{0xF0, 0xF0, 0xF0, 0xF0, 0xC0},
		},
		{
			name:   "a separate sign of a zero is plus",
			v:      0,
			digits: 3,
			sign:   SignTrailingSeparate,
			enc:    GnuCOBOLASCII(),
			want:   []byte{0x30, 0x30, 0x30, 0x2B}, // "000+"
		},
		{
			name:   "single digit",
			v:      7,
			digits: 1,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   []byte{0xC7},
		},
		{
			name:   "nine digits, the int32 maximum",
			v:      -999999999,
			digits: 9,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   []byte{0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xD9},
		},
		{
			name:   "eighteen digits, the int64 maximum",
			v:      999999999999999999,
			digits: 18,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want: []byte{
				0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9,
				0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xC9,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			w, err := NewWriter(&buf, tc.enc)
			require.NoError(t, err)

			require.NoError(t, w.WriteZonedInt64(tc.v, tc.digits, tc.sign))
			require.Equal(t, tc.want, buf.Bytes())
			require.Equal(t, int64(len(tc.want)), w.Offset())
			require.Equal(t, len(tc.want), zonedWidth(tc.digits, tc.sign))

			if tc.digits <= maxZonedInt32Digits {
				var buf32 bytes.Buffer
				w32, err := NewWriter(&buf32, tc.enc)
				require.NoError(t, err)

				require.NoError(t, w32.WriteZonedInt32(int32(tc.v), tc.digits, tc.sign))
				require.Equal(t, tc.want, buf32.Bytes())
			}

			var bufBig bytes.Buffer
			wb, err := NewWriter(&bufBig, tc.enc)
			require.NoError(t, err)

			require.NoError(t, wb.WriteZonedBig(big.NewInt(tc.v), tc.digits, tc.sign))
			require.Equal(t, tc.want, bufBig.Bytes())
		})
	}
}

func TestWriterWriteZonedBig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		v      string
		digits int
		sign   SignPosition
		enc    Encoding
		want   []byte
	}{
		{
			name:   "nineteen digits, one past int64",
			v:      "-1234567890123456789",
			digits: 19,
			sign:   SignTrailing,
			enc:    GnuCOBOLASCII(),
			want:   []byte("123456789012345678" + "\x79"), // "…8y"
		},
		{
			name:   "thirty-one digits, the COBOL maximum",
			v:      "9999999999999999999999999999999",
			digits: 31,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   append(bytes.Repeat([]byte{0xF9}, 30), 0xC9),
		},
		{
			name:   "thirty-one digits, leading separate",
			v:      "-1234567890123456789012345678901",
			digits: 31,
			sign:   SignLeadingSeparate,
			enc:    GnuCOBOLASCII(),
			want:   append([]byte{0x2D}, []byte("1234567890123456789012345678901")...),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, ok := new(big.Int).SetString(tc.v, 10)
			require.True(t, ok)

			var buf bytes.Buffer
			w, err := NewWriter(&buf, tc.enc)
			require.NoError(t, err)

			require.NoError(t, w.WriteZonedBig(v, tc.digits, tc.sign))
			require.Equal(t, tc.want, buf.Bytes())
		})
	}
}

func TestWriterWriteZonedErrors(t *testing.T) {
	t.Parallel()

	t.Run("value wider than the field", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, IBMEnterprise())
		require.NoError(t, err)

		err = w.WriteZonedInt64(123456, 5, SignTrailing)

		var rangeErr ZonedRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "123456", rangeErr.Value)
		require.Equal(t, 5, rangeErr.Digits)
		require.Equal(t, SignTrailing, rangeErr.Sign)

		// A rejected field writes nothing, so it cannot leave a half-field
		// behind to desynchronize the record.
		require.Empty(t, buf.Bytes())
		require.Zero(t, w.Offset())
	})

	t.Run("negative value into an unsigned field", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, IBMEnterprise())
		require.NoError(t, err)

		err = w.WriteZonedInt64(-1, 5, SignUnsigned)

		var rangeErr ZonedRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "-1", rangeErr.Value)
		require.Equal(t, SignUnsigned, rangeErr.Sign)
		require.Empty(t, buf.Bytes())
	})

	t.Run("the minus sign is not a digit of the value", func(t *testing.T) {
		t.Parallel()

		// -99999 is five digits and fits; a writer counting the '-' would
		// reject it.
		var buf bytes.Buffer
		w, err := NewWriter(&buf, IBMEnterprise())
		require.NoError(t, err)

		require.NoError(t, w.WriteZonedInt64(-99999, 5, SignTrailing))
		require.Equal(t, []byte{0xF9, 0xF9, 0xF9, 0xF9, 0xD9}, buf.Bytes())
	})

	t.Run("the most negative int64", func(t *testing.T) {
		t.Parallel()

		// Nineteen digits, so no int64 accessor holds it: the value is
		// formatted rather than negated, and reports the range it missed
		// instead of overflowing on its way out.
		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WriteZonedInt64(math.MinInt64, maxZonedInt64Digits, SignTrailing)

		var rangeErr ZonedRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "-9223372036854775808", rangeErr.Value)
		require.Empty(t, buf.Bytes())

		// The [math/big.Int] accessor reaches the digit count it needs.
		var wide bytes.Buffer
		wb, err := NewWriter(&wide, GnuCOBOLASCII())
		require.NoError(t, err)

		require.NoError(t, wb.WriteZonedBig(big.NewInt(math.MinInt64), 19, SignTrailing))
		require.Equal(t, []byte("922337203685477580"+"\x78"), wide.Bytes()) // "…0x"
	})

	t.Run("digit count outside the accessor's range", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			digits  int
			write   func(*Writer, int) error
			wantMax int
		}{
			{
				name:   "zero digits",
				digits: 0,
				write: func(w *Writer, d int) error {
					return w.WriteZonedInt64(1, d, SignTrailing)
				},
				wantMax: maxZonedInt64Digits,
			},
			{
				name:   "ten digits into an int32",
				digits: 10,
				write: func(w *Writer, d int) error {
					return w.WriteZonedInt32(1, d, SignTrailing)
				},
				wantMax: maxZonedInt32Digits,
			},
			{
				name:   "nineteen digits into an int64",
				digits: 19,
				write: func(w *Writer, d int) error {
					return w.WriteZonedInt64(1, d, SignTrailing)
				},
				wantMax: maxZonedInt64Digits,
			},
			{
				name:   "thirty-two digits into a big.Int",
				digits: 32,
				write: func(w *Writer, d int) error {
					return w.WriteZonedBig(big.NewInt(1), d, SignTrailing)
				},
				wantMax: maxZonedDigits,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var buf bytes.Buffer
				w, err := NewWriter(&buf, GnuCOBOLASCII())
				require.NoError(t, err)

				err = tc.write(w, tc.digits)

				var countErr ZonedDigitCountError
				require.ErrorAs(t, err, &countErr)
				require.Equal(t, tc.digits, countErr.Digits)
				require.Equal(t, tc.wantMax, countErr.Max)
				require.Empty(t, buf.Bytes())
			})
		}
	})

	t.Run("sign position is required", func(t *testing.T) {
		t.Parallel()

		for _, s := range []SignPosition{SignPositionUnset, SignPosition(99)} {
			var buf bytes.Buffer
			w, err := NewWriter(&buf, GnuCOBOLASCII())
			require.NoError(t, err)

			err = w.WriteZonedInt64(1, 5, s)

			var posErr SignPositionError
			require.ErrorAs(t, err, &posErr)
			require.Equal(t, s, posErr.SignPosition)
			require.Empty(t, buf.Bytes())
		}
	})

	t.Run("nil big.Int", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		require.ErrorIs(t, w.WriteZonedBig(nil, 5, SignTrailing), ErrNilValue)
		require.Empty(t, buf.Bytes())
	})

	t.Run("a charset that cannot spell a numeric field", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, zonedEncoding(partialCharset{}, SignASCIIZone37))
		require.NoError(t, err)

		require.NoError(t, w.WriteAlphanumeric("", 2))

		err = w.WriteZonedInt64(1, 5, SignTrailing)

		var runeErr UnrepresentableRuneError
		require.ErrorAs(t, err, &runeErr)
		require.Equal(t, '0', runeErr.Rune)
	})

	t.Run("a failing writer is reported with its offset", func(t *testing.T) {
		t.Parallel()

		w, err := NewWriter(&failingWriter{}, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WriteZonedInt64(12345, 5, SignTrailing)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.ErrorIs(t, err, errWriteFailed)
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

// TestWriterWriteComp6 is the mirror of TestReaderReadComp6 and shares its
// vectors, the SPEC Appendix A.4 ones included.
func TestWriterWriteComp6(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		value  int64
		digits int
		want   []byte
	}{
		{
			name:   "even digits, no pad nibble",
			value:  1234,
			digits: 4,
			want:   []byte{0x12, 0x34}, // SPEC A.4
		},
		{
			name:   "odd digits write a zero pad nibble",
			value:  123,
			digits: 3,
			want:   []byte{0x01, 0x23}, // SPEC A.4
		},
		{
			name:   "two digits fill one byte",
			value:  42,
			digits: 2,
			want:   []byte{0x42},
		},
		{
			name:   "single digit is one byte with a pad nibble",
			value:  7,
			digits: 1,
			want:   []byte{0x07},
		},
		{
			name:   "zero",
			value:  0,
			digits: 4,
			want:   []byte{0x00, 0x00},
		},
		{
			name:   "value narrower than the field is zero filled",
			value:  42,
			digits: 4,
			want:   []byte{0x00, 0x42},
		},
		{
			name:   "nine digits, the int32 maximum",
			value:  999999999,
			digits: 9,
			want:   []byte{0x09, 0x99, 0x99, 0x99, 0x99},
		},
		{
			name:   "eighteen digits, the int64 maximum",
			value:  999999999999999999,
			digits: 18,
			want:   []byte{0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// COMP-6 is charset-invariant, as COMP-3 is.
			for _, enc := range []Encoding{IBMEnterprise(), GnuCOBOLASCII()} {
				var buf bytes.Buffer
				w, err := NewWriter(&buf, enc)
				require.NoError(t, err)

				require.NoError(t, w.WriteComp6Int64(tc.value, tc.digits))
				require.Equal(t, tc.want, buf.Bytes())
				require.Equal(t, int64(comp6Width(tc.digits)), w.Offset())

				if tc.digits <= maxPackedInt32Digits {
					var buf32 bytes.Buffer
					w32, err := NewWriter(&buf32, enc)
					require.NoError(t, err)

					require.NoError(t, w32.WriteComp6Int32(int32(tc.value), tc.digits))
					require.Equal(t, tc.want, buf32.Bytes())
				}

				var bufBig bytes.Buffer
				wBig, err := NewWriter(&bufBig, enc)
				require.NoError(t, err)

				require.NoError(t, wBig.WriteComp6Big(big.NewInt(tc.value), tc.digits))
				require.Equal(t, tc.want, bufBig.Bytes())
			}
		})
	}
}

func TestWriterWriteComp6Big(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		value  string
		digits int
		want   []byte
	}{
		{
			name:   "nineteen digits, one past int64",
			value:  "1234567890123456789",
			digits: 19,
			want:   []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0x01, 0x23, 0x45, 0x67, 0x89},
		},
		{
			name:   "thirty-one digits, the COBOL maximum",
			value:  "9999999999999999999999999999999",
			digits: 31,
			want: []byte{
				0x09, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
				0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
			},
		},
		{
			name:   "thirty digits, the widest even count",
			value:  "123456789012345678901234567890",
			digits: 30,
			want: []byte{
				0x12, 0x34, 0x56, 0x78, 0x90, 0x12, 0x34,
				0x56, 0x78, 0x90, 0x12, 0x34, 0x56, 0x78, 0x90,
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

			require.NoError(t, w.WriteComp6Big(v, tc.digits))
			require.Equal(t, tc.want, buf.Bytes())
		})
	}
}

func TestWriterWriteComp6Errors(t *testing.T) {
	t.Parallel()

	t.Run("value wider than the field", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WriteComp6Int64(12345, 4)

		var rangeErr PackedRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "12345", rangeErr.Value)
		require.Equal(t, 4, rangeErr.Digits)
		require.Equal(t, Unsigned, rangeErr.Signedness)
		require.Empty(t, buf.Bytes())
		require.Zero(t, w.Offset())
	})

	t.Run("negative value has nowhere to go", func(t *testing.T) {
		t.Parallel()

		// COMP-6 stores no sign at all, so there is no Signedness to pass
		// and every negative value is out of range. Storing the magnitude is
		// what COBOL does and what this package will not do silently.
		writes := []struct {
			name  string
			write func(*Writer) error
			want  string
		}{
			{
				name:  "int32",
				write: func(w *Writer) error { return w.WriteComp6Int32(-42, 5) },
				want:  "-42",
			},
			{
				name:  "int64",
				write: func(w *Writer) error { return w.WriteComp6Int64(-1, 18) },
				want:  "-1",
			},
			{
				name:  "big",
				write: func(w *Writer) error { return w.WriteComp6Big(big.NewInt(-1), 31) },
				want:  "-1",
			},
			{
				name:  "the most negative int64",
				write: func(w *Writer) error { return w.WriteComp6Int64(math.MinInt64, 18) },
				want:  "-9223372036854775808",
			},
		}

		for _, tc := range writes {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				var buf bytes.Buffer
				w, err := NewWriter(&buf, GnuCOBOLASCII())
				require.NoError(t, err)

				err = tc.write(w)

				var rangeErr PackedRangeError
				require.ErrorAs(t, err, &rangeErr)
				require.Equal(t, tc.want, rangeErr.Value)
				require.Equal(t, Unsigned, rangeErr.Signedness)
				require.Empty(t, buf.Bytes())
				require.Zero(t, w.Offset())
			})
		}
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
				write:  func(w *Writer, d int) error { return w.WriteComp6Int64(0, d) },
			},
			{
				name:   "negative digits",
				digits: -1,
				max:    maxPackedInt32Digits,
				write:  func(w *Writer, d int) error { return w.WriteComp6Int32(0, d) },
			},
			{
				name:   "ten digits overflows an int32",
				digits: 10,
				max:    maxPackedInt32Digits,
				write:  func(w *Writer, d int) error { return w.WriteComp6Int32(0, d) },
			},
			{
				name:   "nineteen digits overflows an int64",
				digits: 19,
				max:    maxPackedInt64Digits,
				write:  func(w *Writer, d int) error { return w.WriteComp6Int64(0, d) },
			},
			{
				name:   "thirty-two digits exceeds COBOL itself",
				digits: 32,
				max:    maxPackedDigits,
				write:  func(w *Writer, d int) error { return w.WriteComp6Big(big.NewInt(0), d) },
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

		err = w.WriteComp6Big(nil, 5)
		require.ErrorIs(t, err, ErrNilValue)
		require.Empty(t, buf.Bytes())
	})

	t.Run("underlying writer fails", func(t *testing.T) {
		t.Parallel()

		w, err := NewWriter(&failingWriter{limit: 1}, GnuCOBOLASCII())
		require.NoError(t, err)

		err = w.WriteComp6Int64(1234, 4)
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

// TestWriterWriteFloatIEEE is the direct shape for binary32 and binary64: a
// value in, the exact bytes out, in both byte orders.
func TestWriterWriteFloatIEEE(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		v    float64
		be32 []byte
		be64 []byte
	}{
		{
			name: "one",
			v:    1,
			be32: []byte{0x3F, 0x80, 0x00, 0x00},
			be64: []byte{0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "zero",
			v:    0,
			be32: []byte{0x00, 0x00, 0x00, 0x00},
			be64: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "negative one",
			v:    -1,
			be32: []byte{0xBF, 0x80, 0x00, 0x00},
			be64: []byte{0xBF, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "one and a half",
			v:    1.5,
			be32: []byte{0x3F, 0xC0, 0x00, 0x00},
			be64: []byte{0x3F, 0xF8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "a thirty-second",
			v:    0.03125,
			be32: []byte{0x3D, 0x00, 0x00, 0x00},
			be64: []byte{0x3F, 0xA0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
	}

	for _, tc := range testCases {
		for _, bo := range binaryOrders {
			t.Run(tc.name+", "+bo.String(), func(t *testing.T) {
				t.Parallel()

				enc := floatEncoding(FloatIEEE, bo)

				var buf bytes.Buffer
				w, err := NewWriter(&buf, enc)
				require.NoError(t, err)
				require.NoError(t, w.WriteFloat32(float32(tc.v)))
				require.Equal(t, inByteOrder(bo, tc.be32), buf.Bytes())
				require.Equal(t, int64(comp1Width), w.Offset())

				buf.Reset()
				w, err = NewWriter(&buf, enc)
				require.NoError(t, err)
				require.NoError(t, w.WriteFloat64(tc.v))
				require.Equal(t, inByteOrder(bo, tc.be64), buf.Bytes())
				require.Equal(t, int64(comp2Width), w.Offset())
			})
		}
	}
}

// TestWriterWriteFloatHFP is the direct shape for IBM hexadecimal floating
// point: the same known bit patterns TestReaderReadFloatHFP decodes, written
// from the value side.
func TestWriterWriteFloatHFP(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		v     float64
		short []byte
		long  []byte
	}{
		{
			name:  "zero is the all-zero field",
			v:     0,
			short: []byte{0x00, 0x00, 0x00, 0x00},
			long:  []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "negative zero is the all-zero field too",
			// HFP has no signed zero to preserve one as, so the sign bit is
			// dropped rather than stored on a zero fraction.
			v:     math.Copysign(0, -1),
			short: []byte{0x00, 0x00, 0x00, 0x00},
			long:  []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:  "one",
			v:     1,
			short: []byte{0x41, 0x10, 0x00, 0x00},
			long:  []byte{0x41, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:  "negative one",
			v:     -1,
			short: []byte{0xC1, 0x10, 0x00, 0x00},
			long:  []byte{0xC1, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:  "a half",
			v:     0.5,
			short: []byte{0x40, 0x80, 0x00, 0x00},
			long:  []byte{0x40, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:  "a sixteenth",
			v:     0.0625,
			short: []byte{0x40, 0x10, 0x00, 0x00},
			long:  []byte{0x40, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:  "sixteen",
			v:     16,
			short: []byte{0x42, 0x10, 0x00, 0x00},
			long:  []byte{0x42, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:  "two hundred and fifty-five",
			v:     255,
			short: []byte{0x42, 0xFF, 0x00, 0x00},
			long:  []byte{0x42, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "a thirty-second",
			// The bytes IEEE spells 1.0 with, written from the other side of
			// codec/SPEC.md's worked example.
			v:     0.03125,
			short: []byte{0x3F, 0x80, 0x00, 0x00},
			long:  []byte{0x3F, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "always normalized",
			// 1.0 has an unnormalized spelling, 0.01₁₆ × 16^2, that the reader
			// accepts. The writer emits the normalized one and only that.
			v:     1,
			short: []byte{0x41, 0x10, 0x00, 0x00},
			long:  []byte{0x41, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Little-endian deliberately: HFP is big-endian regardless of the
			// axis, so a writer that consulted it would emit these reversed.
			enc := floatEncoding(FloatHFP, binary.LittleEndian)

			var buf bytes.Buffer
			w, err := NewWriter(&buf, enc)
			require.NoError(t, err)
			require.NoError(t, w.WriteFloat32(float32(tc.v)))
			require.Equal(t, tc.short, buf.Bytes())

			buf.Reset()
			w, err = NewWriter(&buf, enc)
			require.NoError(t, err)
			require.NoError(t, w.WriteFloat64(tc.v))
			require.Equal(t, tc.long, buf.Bytes())
		})
	}
}

func TestWriterWriteFloatErrors(t *testing.T) {
	t.Parallel()

	hfp := floatEncoding(FloatHFP, binary.BigEndian)

	notFinite := []struct {
		name string
		v    float64
	}{
		{name: "nan", v: math.NaN()},
		{name: "positive infinity", v: math.Inf(1)},
		{name: "negative infinity", v: math.Inf(-1)},
	}

	for _, tc := range notFinite {
		t.Run("hfp rejects "+tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			w, err := NewWriter(&buf, hfp)
			require.NoError(t, err)

			err = w.WriteFloat64(tc.v)

			var rangeErr FloatRangeError
			require.ErrorAs(t, err, &rangeErr)
			require.Equal(t, FloatHFP, rangeErr.Format)
			require.Equal(t, comp2Width, rangeErr.Width)
			require.Contains(t, rangeErr.Reason, "not a finite number")
			// A rejected field writes nothing, so the record has not
			// desynchronized.
			require.Zero(t, buf.Len())
			require.Zero(t, w.Offset())

			// The same value is an ordinary binary64 bit pattern under IEEE.
			buf.Reset()
			w, err = NewWriter(&buf, floatEncoding(FloatIEEE, binary.BigEndian))
			require.NoError(t, err)
			require.NoError(t, w.WriteFloat64(tc.v))
			require.Len(t, buf.Bytes(), comp2Width)
		})

		t.Run("hfp rejects "+tc.name+" as comp-1", func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			w, err := NewWriter(&buf, hfp)
			require.NoError(t, err)

			err = w.WriteFloat32(float32(tc.v))

			var rangeErr FloatRangeError
			require.ErrorAs(t, err, &rangeErr)
			require.Equal(t, comp1Width, rangeErr.Width)
			require.Zero(t, buf.Len())
		})
	}

	t.Run("hfp rejects a magnitude above its range", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, hfp)
		require.NoError(t, err)

		// HFP stops at 16^63, about 7.2e75; a float64 runs to 1.8e308.
		err = w.WriteFloat64(1e300)

		var rangeErr FloatRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "1e+300", rangeErr.Value)
		require.Contains(t, rangeErr.Reason, "above 16^63")
		require.Zero(t, buf.Len())
	})

	t.Run("hfp rejects a magnitude below its range", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		w, err := NewWriter(&buf, hfp)
		require.NoError(t, err)

		// HFP stops at 16^-65, about 5.4e-79; a float64 runs down to 5e-324.
		err = w.WriteFloat64(-1e-300)

		var rangeErr FloatRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Contains(t, rangeErr.Reason, "below 16^-65")
		require.Zero(t, buf.Len())
	})

	t.Run("every finite float32 is in HFP's range", func(t *testing.T) {
		t.Parallel()

		// binary32 reaches neither end of HFP's range, so the two bounds above
		// are unreachable from a COMP-1 field and only NaN and infinity are
		// rejected there.
		var buf bytes.Buffer
		w, err := NewWriter(&buf, hfp)
		require.NoError(t, err)

		require.NoError(t, w.WriteFloat32(math.MaxFloat32))
		require.NoError(t, w.WriteFloat32(-math.MaxFloat32))
		require.NoError(t, w.WriteFloat32(math.SmallestNonzeroFloat32))
		require.Equal(t, int64(3*comp1Width), w.Offset())
	})

	t.Run("the very largest HFP long value does not survive a float64", func(t *testing.T) {
		t.Parallel()

		// HFP long carries 56 bits of fraction against a float64's 53, so the
		// three bits below are rounded away on the way out — and at the top of
		// the exponent range that rounding is upward, to 16^63 exactly, which
		// is one ulp past what a fraction below one can spell. It is the one
		// place a value this package decoded cannot be written back, and it is
		// reported rather than clamped.
		r, err := NewReader(bytes.NewReader([]byte{
			0x7F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		}), hfp)
		require.NoError(t, err)
		v, err := r.ReadFloat64()
		require.NoError(t, err)
		require.Equal(t, math.Ldexp(1, 252), v)

		var buf bytes.Buffer
		w, err := NewWriter(&buf, hfp)
		require.NoError(t, err)

		var rangeErr FloatRangeError
		require.ErrorAs(t, w.WriteFloat64(v), &rangeErr)
		require.Contains(t, rangeErr.Reason, "above 16^63")
	})

	t.Run("write failure is reported", func(t *testing.T) {
		t.Parallel()

		w, err := NewWriter(&failingWriter{limit: 0}, floatEncoding(FloatIEEE, binary.BigEndian))
		require.NoError(t, err)

		require.ErrorIs(t, w.WriteFloat32(1), errWriteFailed)
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
			ID:      "A12345",
			Name:    "WIDGET GRIP",
			Code:    "42",
			Raw:     []byte{0x00, 0x01, 0xFF},
			Amount:  12345,
			Qty:     42,
			Units:   7,
			Seq:     1234,
			Rate:    1.5,
			Factor:  2.5,
			Balance: -12345,
			Count:   42,
		})
		require.NoError(t, err)
		// 00 07 is UNITS, a PIC 9(4) COMP-6 item: two bytes to QTY's three,
		// and a leading zero rather than a pad nibble, since the digit count
		// is even. 3F C0 00 00 is RATE as binary32 and 40 04 … FACTOR as
		// binary64, big-endian as GnuCOBOL writes them by default; "1234u" is
		// BALANCE with its sign overpunched into the last digit byte, and
		// "042" is COUNT, an unsigned DISPLAY item padded with a high-order
		// zero.
		require.Equal(t, []byte("A12345WIDGET GRIP   42\x00\x01\xFF\x12\x34\x5C\x00\x04\x2F\x00\x07\x04\xD2"+
			"\x3F\xC0\x00\x00"+"\x40\x04\x00\x00\x00\x00\x00\x00"+"1234u"+"042"), got)
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
			// … | 12 34 5C: AMOUNT +12345 | 00 04 2F: QTY 42 | 00 07: UNITS 7,
			// COMP-6 and so a byte narrower than QTY | 04 D2: SEQ 1234
			// | 3F C0 00 00: RATE 1.5, binary32 | 40 04 …: FACTOR 2.5, binary64
			// | "1234u": BALANCE -12345, zone 3/7 | "042": COUNT 42
			src: []byte("A12345WIDGET GRIP   42\x00\x01\xFF\x12\x34\x5C\x00\x04\x2F\x00\x07\x04\xD2" +
				"\x3F\xC0\x00\x00" + "\x40\x04\x00\x00\x00\x00\x00\x00" + "1234u" + "042"),
		},
		{
			name: "ascii record with empty fields",
			enc:  MicroFocusASCII(),
			// The zero of a signed packed field is C, of an unsigned one F,
			// and of a COMP-6 field nothing at all — its two bytes are all
			// digit nibbles. SEQ is zero, which is the one binary value this
			// encoding's native byte order cannot reorder — and RATE and
			// FACTOR are zero for the same reason, since their IEEE bytes
			// follow that order too. A zoned zero is spelled out digit by
			// digit, and under zone 3/7 a positive sign leaves the last one an
			// ordinary '0'.
			src: []byte("      " + "            " + "    " + "\x00\x00\x00" +
				"\x00\x00\x0C" + "\x00\x00\x0F" + "\x00\x00" + "\x00\x00" +
				"\x00\x00\x00\x00" + "\x00\x00\x00\x00\x00\x00\x00\x00" +
				"00000" + "000"),
		},
		{
			name: "ascii record with full fields",
			enc:  ConvertedFromEBCDIC(),
			// 98 76 5D: AMOUNT -98765 | 09 99 9F: QTY 9999 | 99 99: UNITS 9999,
			// the widest a PIC 9(4) COMP-6 holds and the case where its two
			// bytes are entirely digits | FB 2E: SEQ -1234,
			// big-endian because the mainframe that wrote it was — and for the
			// same reason RATE and FACTOR are HFP, C1 18 …: -1.5 and
			// 42 FF …: 255. A converted file keeps the floating point format
			// the compiler that wrote it used; only the characters changed —
			// which is exactly what "9876N" is: BALANCE -98765 with the
			// EBCDIC overpunch D5 translated to the letter N. "999" is COUNT,
			// filling its three digits.
			src: []byte("A12345WIDGETGRIP123456\xFF\xFE\xFD\x98\x76\x5D\x09\x99\x9F\x99\x99\xFB\x2E" +
				"\xC1\x18\x00\x00" + "\x42\xFF\x00\x00\x00\x00\x00\x00" +
				"9876N" + "999"),
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
				// above: COMP-3 is charset-invariant. The floating point ones
				// are not — these are HFP where the first case is IEEE, and
				// the same twelve bytes mean different numbers in each.
				0x00, 0x01, 0xFF, // raw payload
				0x12, 0x34, 0x5C, // AMOUNT +12345
				0x00, 0x04, 0x2F, // QTY 42
				0x00, 0x07, // UNITS 7, PIC 9(4) COMP-6 — two bytes, no sign nibble
				0x04, 0xD2, // SEQ 1234, big-endian as z/OS writes it
				0x41, 0x18, 0x00, 0x00, // RATE 1.5 as HFP short
				0x41, 0x28, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // FACTOR 2.5 as HFP long
				// The zoned bytes are the ones that are *not* charset
				// invariant: these spell the same numbers as the ASCII case
				// above with EBCDIC digit zones and an EBCDIC overpunch.
				0xF1, 0xF2, 0xF3, 0xF4, 0xD5, // BALANCE -12345
				0xF0, 0xF4, 0xF2, // COUNT 42
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

// TestRoundTripAlphanumericEveryByteValue walks a PIC X field carrying all 256
// byte values through decode → encode → byte-equal, in every charset of
// alphanumericCharsets and under both justifications. It is the round-trip
// shape codec/CLAUDE.md requires of every accessor pair, run over the byte
// range the package's other alphanumeric fixtures never reach.
//
// The two halves of the pair disagree about what a field is made of: the
// reader translates bytes to characters, while the writer ranges over the
// runes of the string it is given. A field decoded to anything other than the
// characters the charset names therefore cannot be written back at all — a
// verbatim decode yields U+FFFD for every byte above U+007F, and no charset
// has a byte for that.
func TestRoundTripAlphanumericEveryByteValue(t *testing.T) {
	t.Parallel()

	for _, tc := range alphanumericCharsets {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, j := range []Justification{JustifyLeft, JustifyRight} {
				t.Run(j.String(), func(t *testing.T) {
					t.Parallel()

					src := allByteValues()
					enc := charsetEncoding(tc.charset)

					r, err := NewReader(bytes.NewReader(src), enc)
					require.NoError(t, err)

					got, err := r.ReadAlphanumericJustified(len(src), j)
					require.NoError(t, err)

					var buf bytes.Buffer
					w, err := NewWriter(&buf, enc)
					require.NoError(t, err)

					err = w.WriteAlphanumericJustified(got, len(src), j)
					if !tc.roundTrip {
						// The case is documented rather than asserted away:
						// the charset cannot spell the field back, and the
						// writer names the character it stopped on instead of
						// substituting one for it.
						var runeErr UnrepresentableRuneError
						require.ErrorAs(t, err, &runeErr)
						require.Equal(t, tc.unrepresentable, runeErr.Rune, tc.why)
						require.Equal(t, tc.charset.Name(), runeErr.Charset)
						require.Zero(t, buf.Len(), "a rejected field wrote part of itself")
						return
					}
					require.NoError(t, err)
					require.Equal(t, src, buf.Bytes())
				})
			}
		})
	}
}

// TestRoundTripAlphanumericPaddedMultiByteField walks the one alphanumeric
// fixture where the justification axis is load-bearing through
// decode → encode → byte-equal.
//
// The all-bytes corpus above cannot do that job: it is exactly as wide as the
// field and carries no space at either end, so the reader's trim and the
// writer's pad are both no-ops on it and the two justifications produce the
// same bytes. Here they do not — the padding sits at the end the justification
// names, and the value between it is two characters that are not one byte
// each, which is where a pad or trim written over the bytes rather than over
// the translated string puts the value in the wrong place.
func TestRoundTripAlphanumericPaddedMultiByteField(t *testing.T) {
	t.Parallel()

	for _, tc := range alphanumericCharsets {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, j := range []Justification{JustifyLeft, JustifyRight} {
				t.Run(j.String(), func(t *testing.T) {
					t.Parallel()

					cs := tc.charset
					src, content := paddedMultiByteField(cs)
					requireMultiByteFixture(t, cs, content)
					enc := charsetEncoding(cs)

					r, err := NewReader(bytes.NewReader(src), enc)
					require.NoError(t, err)

					got, err := r.ReadAlphanumericJustified(len(src), j)
					require.NoError(t, err)
					require.Greater(t, len(got), len([]rune(got)), "the value read back is one byte per character, so the case is not multi-byte")

					var buf bytes.Buffer
					w, err := NewWriter(&buf, enc)
					require.NoError(t, err)

					err = w.WriteAlphanumericJustified(got, len(src), j)
					if !tc.roundTrip {
						// The value's own characters are what this charset
						// cannot spell back, so the field is rejected before
						// the padding is ever reached.
						var runeErr UnrepresentableRuneError
						require.ErrorAs(t, err, &runeErr, tc.why)
						require.Equal(t, cs.Name(), runeErr.Charset)
						require.Zero(t, buf.Len(), "a rejected field wrote part of itself")
						return
					}
					require.NoError(t, err)
					require.Equal(t, src, buf.Bytes())
				})
			}
		})
	}
}

// TestRoundTripPacked walks a packed decimal field both ways at every digit
// count COBOL allows, which is the cheapest check that the width formula, the
// pad nibble and the digit placement agree between the reader and the writer.
// TestRoundTripZoned walks a zoned decimal field both ways at every digit count
// COBOL allows, in every sign position, under every sign convention and in both
// charsets. It is the cheapest check that the width, the sign position and the
// two halves of a zoned byte — the charset's digit zone and the convention's
// sign zone — agree between the reader and the writer.
func TestRoundTripZoned(t *testing.T) {
	t.Parallel()

	encodings := []struct {
		name string
		enc  Encoding
	}{
		{name: "ebcdic", enc: IBMEnterprise()},
		{name: "ascii zone 3/7", enc: GnuCOBOLASCII()},
		{name: "translated ebcdic", enc: ConvertedFromEBCDIC()},
		{name: "realia", enc: zonedEncoding(ASCII(), SignRealia)},
		// A charset no machine ever used, whose digits sit at B0-B9 and
		// whose separate signs at 01 and 02: it fails anything that reached
		// for a hard-coded code page instead of asking the charset.
		{name: "oddball charset", enc: zonedEncoding(oddballCharset{}, SignASCIIZone37)},
	}
	positions := []SignPosition{
		SignUnsigned,
		SignTrailing,
		SignLeading,
		SignTrailingSeparate,
		SignLeadingSeparate,
	}

	for digits := 1; digits <= maxZonedDigits; digits++ {
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

			for _, e := range encodings {
				for _, s := range positions {
					for _, v := range values {
						if v.Sign() < 0 && s == SignUnsigned {
							continue
						}

						var buf bytes.Buffer
						w, err := NewWriter(&buf, e.enc)
						require.NoError(t, err)
						require.NoError(t, w.WriteZonedBig(v, digits, s))
						require.Len(t, buf.Bytes(), zonedWidth(digits, s))

						r, err := NewReader(bytes.NewReader(buf.Bytes()), e.enc)
						require.NoError(t, err)
						got, err := r.ReadZonedBig(digits, s)
						require.NoError(t, err)
						require.Equal(t, v.String(), got.String())

						// The bytes a reader accepts are written back
						// unchanged, which is the direction that catches
						// a sign zone or a digit position drifting.
						var back bytes.Buffer
						w2, err := NewWriter(&back, e.enc)
						require.NoError(t, err)
						require.NoError(t, w2.WriteZonedBig(got, digits, s))
						require.Equal(t, buf.Bytes(), back.Bytes())

						if digits <= maxZonedInt64Digits {
							r64, err := NewReader(bytes.NewReader(buf.Bytes()), e.enc)
							require.NoError(t, err)
							got64, err := r64.ReadZonedInt64(digits, s)
							require.NoError(t, err)
							require.Equal(t, v.Int64(), got64)
						}
						if digits <= maxZonedInt32Digits {
							r32, err := NewReader(bytes.NewReader(buf.Bytes()), e.enc)
							require.NoError(t, err)
							got32, err := r32.ReadZonedInt32(digits, s)
							require.NoError(t, err)
							require.Equal(t, int32(v.Int64()), got32)
						}
					}
				}
			}
		})
	}
}

// TestRoundTripZonedNonCanonicalSigns is the zoned counterpart of
// [TestRoundTripPackedLenientSigns], and it is the one place this round trip is
// deliberately not byte-equal: a reader accepts sign bytes a writer never
// emits, so such a field reads correctly and is written back in the canonical
// spelling.
func TestRoundTripZonedNonCanonicalSigns(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		src       []byte
		enc       Encoding
		want      int64
		wantBytes []byte
	}{
		{
			name:      "lenient ebcdic zone A is written back as C",
			src:       []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xA5},
			enc:       IBMEnterprise(),
			want:      12345,
			wantBytes: []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xC5},
		},
		{
			name:      "lenient ebcdic zone B is written back as D",
			src:       []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xB5},
			enc:       IBMEnterprise(),
			want:      -12345,
			wantBytes: []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xD5},
		},
		{
			name:      "lenient ebcdic zone E is written back as C",
			src:       []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xE5},
			enc:       IBMEnterprise(),
			want:      12345,
			wantBytes: []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xC5},
		},
		{
			name: "an unsigned zone in a signed field is written back signed",
			src:  []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5},
			enc:  IBMEnterprise(),
			// What a signed item holds after a MOVE from an unsigned one:
			// a non-negative value, and a writer states the sign it has.
			want:      12345,
			wantBytes: []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xC5},
		},
		{
			name:      "translated ebcdic keeps its own spelling",
			src:       []byte{0x31, 0x32, 0x33, 0x34, 0x4E},
			enc:       ConvertedFromEBCDIC(),
			want:      -12345,
			wantBytes: []byte{0x31, 0x32, 0x33, 0x34, 0x4E},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := NewReader(bytes.NewReader(tc.src), tc.enc)
			require.NoError(t, err)
			got, err := r.ReadZonedInt64(5, SignTrailing)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)

			var buf bytes.Buffer
			w, err := NewWriter(&buf, tc.enc)
			require.NoError(t, err)
			require.NoError(t, w.WriteZonedInt64(got, 5, SignTrailing))
			require.Equal(t, tc.wantBytes, buf.Bytes())
		})
	}
}

// TestZonedAccessorsNeverTranslateThroughTheCharset is the accessor-level half
// of [TestZonedDecodingNeverTranslatesThroughTheCharset]: charset translation
// belongs to alphanumeric fields, and a numeric byte put through it would lose
// the overpunch zone that carries the sign.
func TestZonedAccessorsNeverTranslateThroughTheCharset(t *testing.T) {
	t.Parallel()

	cs := &countingCharset{Charset: CP037()}
	enc := zonedEncoding(cs, SignEBCDIC)

	var buf bytes.Buffer
	w, err := NewWriter(&buf, enc)
	require.NoError(t, err)
	require.NoError(t, w.WriteZonedInt64(-12345, 5, SignTrailing))
	require.NoError(t, w.WriteZonedInt64(-12345, 5, SignLeadingSeparate))

	r, err := NewReader(bytes.NewReader(buf.Bytes()), enc)
	require.NoError(t, err)
	for _, s := range []SignPosition{SignTrailing, SignLeadingSeparate} {
		got, err := r.ReadZonedInt64(5, s)
		require.NoError(t, err)
		require.Equal(t, int64(-12345), got)
	}

	require.Zero(t, cs.toUnicode.Load(), "a zoned field was translated through the charset")
}

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

// TestRoundTripComp6 walks a COMP-6 field both ways at every digit count COBOL
// allows, which is the cheapest check that the width formula, the pad nibble's
// parity and the digit placement agree between the reader and the writer.
func TestRoundTripComp6(t *testing.T) {
	t.Parallel()

	for digits := 1; digits <= maxPackedDigits; digits++ {
		t.Run(strconv.Itoa(digits)+" digits", func(t *testing.T) {
			t.Parallel()

			// The widest value the field holds, a value with high-order
			// zeros, and zero itself. There is no negation here: a COMP-6
			// field cannot hold one.
			nines, ok := new(big.Int).SetString(strings.Repeat("9", digits), 10)
			require.True(t, ok)
			values := []*big.Int{nines, big.NewInt(0), big.NewInt(7)}

			for _, v := range values {
				var buf bytes.Buffer
				w, err := NewWriter(&buf, IBMEnterprise())
				require.NoError(t, err)
				require.NoError(t, w.WriteComp6Big(v, digits))
				require.Len(t, buf.Bytes(), comp6Width(digits))

				r, err := NewReader(bytes.NewReader(buf.Bytes()), IBMEnterprise())
				require.NoError(t, err)
				got, err := r.ReadComp6Big(digits)
				require.NoError(t, err)
				require.Equal(t, v.String(), got.String())

				// The bytes a reader accepts are written back unchanged,
				// which is the direction that catches a pad nibble drifting.
				var back bytes.Buffer
				w2, err := NewWriter(&back, IBMEnterprise())
				require.NoError(t, err)
				require.NoError(t, w2.WriteComp6Big(got, digits))
				require.Equal(t, buf.Bytes(), back.Bytes())

				if digits <= maxPackedInt64Digits {
					r64, err := NewReader(bytes.NewReader(buf.Bytes()), IBMEnterprise())
					require.NoError(t, err)
					got64, err := r64.ReadComp6Int64(digits)
					require.NoError(t, err)
					require.Equal(t, v.Int64(), got64)
				}
				if digits <= maxPackedInt32Digits {
					r32, err := NewReader(bytes.NewReader(buf.Bytes()), IBMEnterprise())
					require.NoError(t, err)
					got32, err := r32.ReadComp6Int32(digits)
					require.NoError(t, err)
					require.Equal(t, int32(v.Int64()), got32)
				}
			}
		})
	}
}

// TestComp6IsNotComp3 pins the two encodings apart at the byte level. A COMP-3
// field read at a COMP-6 offset must fail rather than yield a plausible number,
// which is the failure mode the SPEC's "MUST NOT be decoded as COMP-3" is
// about.
func TestComp6IsNotComp3(t *testing.T) {
	t.Parallel()

	t.Run("the same value writes different bytes", func(t *testing.T) {
		t.Parallel()

		var comp6 bytes.Buffer
		w6, err := NewWriter(&comp6, GnuCOBOLASCII())
		require.NoError(t, err)
		require.NoError(t, w6.WriteComp6Int64(1234, 4))

		var comp3 bytes.Buffer
		w3, err := NewWriter(&comp3, GnuCOBOLASCII())
		require.NoError(t, err)
		require.NoError(t, w3.WritePackedInt64(1234, 4, Unsigned))

		require.Equal(t, []byte{0x12, 0x34}, comp6.Bytes())
		require.Equal(t, []byte{0x01, 0x23, 0x4F}, comp3.Bytes())
	})

	t.Run("the same digit count at an odd width is caught by the nibbles", func(t *testing.T) {
		t.Parallel()

		// This is the collision the width formulas leave open: packedWidth(5)
		// and comp6Width(5) are both 3, so a copybook that has the usage
		// wrong at an odd digit count reads the same bytes at the same width
		// and the record does not shift. What catches it is the nibbles.
		t.Run("a COMP-3 sign nibble lands in a digit position", func(t *testing.T) {
			t.Parallel()

			// PIC S9(5) COMP-3 holding +1234 is 01 23 4C. Its pad nibble is
			// 0 — the leading digit is zero — so the pad check passes and the
			// digit check on C is the whole of the guarantee.
			for _, sign := range []byte{0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F} {
				src := []byte{0x01, 0x23, 0x40 | sign}
				r, err := NewReader(bytes.NewReader(src), GnuCOBOLASCII())
				require.NoError(t, err)

				_, err = r.ReadComp6Int64(5)

				var digitErr PackedDigitError
				require.ErrorAs(t, err, &digitErr)
				require.Equal(t, sign, digitErr.Nibble)
			}
		})

		t.Run("a non-zero leading digit trips the pad check first", func(t *testing.T) {
			t.Parallel()

			// PIC S9(5) COMP-3 holding +12345 is 12 34 5C. Here the leading
			// digit is non-zero, so the pad check fires before the reader
			// ever reaches the sign nibble.
			r, err := NewReader(bytes.NewReader([]byte{0x12, 0x34, 0x5C}), GnuCOBOLASCII())
			require.NoError(t, err)

			_, err = r.ReadComp6Int64(5)

			var padErr PackedPadError
			require.ErrorAs(t, err, &padErr)
			require.Equal(t, byte(0x01), padErr.Nibble)
		})

		t.Run("an even digit count sharing a width fails on the sign nibble", func(t *testing.T) {
			t.Parallel()

			// PIC S9(5) COMP-3 read as PIC 9(6) COMP-6: three bytes either
			// way, no pad nibble at an even count, so every nibble is a digit
			// position and the sign nibble is out of range.
			for _, sign := range []byte{0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F} {
				src := []byte{0x12, 0x34, 0x50 | sign}
				r, err := NewReader(bytes.NewReader(src), GnuCOBOLASCII())
				require.NoError(t, err)

				_, err = r.ReadComp6Int64(6)

				var digitErr PackedDigitError
				require.ErrorAs(t, err, &digitErr)
				require.Equal(t, sign, digitErr.Nibble)
			}
		})
	})

	t.Run("a COMP-3 field read at the COMP-6 width leaves a byte behind", func(t *testing.T) {
		t.Parallel()

		// PIC 9(4) COMP-3 holding 1234 is 01 23 4F. Read as a PIC 9(4)
		// COMP-6 field it is two bytes, 01 23, whose nibbles are all legal
		// digits — so this one is not caught at the nibble level at all. It
		// is caught by the width: the reader stops a byte short and every
		// later field of the record is out of place, which is the
		// loud-indirectly failure the SPEC's "MUST NOT be decoded as
		// COMP-3" is about.
		src := []byte{0x01, 0x23, 0x4F}
		r, err := NewReader(bytes.NewReader(src), GnuCOBOLASCII())
		require.NoError(t, err)

		got, err := r.ReadComp6Int64(4)
		require.NoError(t, err)
		require.Equal(t, int64(123), got)
		require.Equal(t, int64(2), r.Offset())
	})
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

// floatRoundTripValues are values every float round-trip runs. Each is exact in
// binary32, and each has at most three significant hex digits, so each survives
// the up-to-three bits of fraction that normalizing to a hex digit boundary can
// cost a COMP-1 field.
var floatRoundTripValues = []float64{
	0, 1, -1, 0.5, -0.5, 0.0625, 16, 255, 0.03125, 123.25, 1024, -4096.5,
}

// TestRoundTripFloat is the value-equality direction for both formats and both
// widths: a value written and read back under the same encoding must compare
// equal to what went in.
func TestRoundTripFloat(t *testing.T) {
	t.Parallel()

	for _, format := range []FloatFormat{FloatIEEE, FloatHFP} {
		for _, bo := range binaryOrders {
			t.Run(format.String()+", "+bo.String(), func(t *testing.T) {
				t.Parallel()

				enc := floatEncoding(format, bo)

				for _, v := range floatRoundTripValues {
					var buf bytes.Buffer
					w, err := NewWriter(&buf, enc)
					require.NoError(t, err)
					require.NoError(t, w.WriteFloat32(float32(v)))
					require.NoError(t, w.WriteFloat64(v))
					require.Equal(t, int64(comp1Width+comp2Width), w.Offset())

					r, err := NewReader(bytes.NewReader(buf.Bytes()), enc)
					require.NoError(t, err)
					got32, err := r.ReadFloat32()
					require.NoError(t, err)
					require.Equal(t, float32(v), got32)
					got64, err := r.ReadFloat64()
					require.NoError(t, err)
					require.Equal(t, v, got64)
				}
			})
		}
	}
}

// TestRoundTripFloatDecodeEncode is the byte-equality direction: bytes read out
// of a field and written straight back must reproduce them. For HFP that is a
// real claim rather than a tautology, since the writer always normalizes and
// the reader accepts more patterns than the writer emits — every pattern here
// is one the writer would have produced.
func TestRoundTripFloatDecodeEncode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		format FloatFormat
		short  []byte
		long   []byte
	}{
		{
			name:   "hfp one",
			format: FloatHFP,
			short:  []byte{0x41, 0x10, 0x00, 0x00},
			long:   []byte{0x41, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:   "hfp negative, every fraction digit in use",
			format: FloatHFP,
			short:  []byte{0xC2, 0x7B, 0x40, 0x00},
			long:   []byte{0xC2, 0x7B, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:   "hfp true zero",
			format: FloatHFP,
			short:  []byte{0x00, 0x00, 0x00, 0x00},
			long:   []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name: "hfp fraction filled to the last bit a float64 holds",
			// The long form sits at the top of the exponent range with its last
			// three fraction bits clear, which is what keeps it inside a
			// float64's 53-bit significand — HFP long carries 56. The short
			// form is not at the top, because 16^63 is far past a float32.
			format: FloatHFP,
			short:  []byte{0x41, 0xFF, 0xFF, 0xFF},
			long:   []byte{0x7F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xF8},
		},
		{
			name:   "ieee one",
			format: FloatIEEE,
			short:  []byte{0x3F, 0x80, 0x00, 0x00},
			long:   []byte{0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:   "ieee negative fraction",
			format: FloatIEEE,
			short:  []byte{0xBE, 0xAA, 0xAA, 0xAB},
			long:   []byte{0xBF, 0xD5, 0x55, 0x55, 0x55, 0x55, 0x55, 0x55},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// HFP ignores the byte order axis and IEEE follows it, so stating
			// big-endian is what makes the two rows comparable.
			enc := floatEncoding(tc.format, binary.BigEndian)

			r, err := NewReader(bytes.NewReader(tc.short), enc)
			require.NoError(t, err)
			v32, err := r.ReadFloat32()
			require.NoError(t, err)

			var buf bytes.Buffer
			w, err := NewWriter(&buf, enc)
			require.NoError(t, err)
			require.NoError(t, w.WriteFloat32(v32))
			require.Equal(t, tc.short, buf.Bytes())

			r, err = NewReader(bytes.NewReader(tc.long), enc)
			require.NoError(t, err)
			v64, err := r.ReadFloat64()
			require.NoError(t, err)

			buf.Reset()
			w, err = NewWriter(&buf, enc)
			require.NoError(t, err)
			require.NoError(t, w.WriteFloat64(v64))
			require.Equal(t, tc.long, buf.Bytes())
		})
	}
}

// TestRoundTripFloatAcrossDialects converts a float field from one dialect to
// the other and back, which is the operation a real migration performs: read a
// z/OS file under [IBMEnterprise], write it under [GnuCOBOLASCII], and the
// number must survive. It runs both directions, because the two formats are not
// symmetric — HFP is a base-16 fraction with no implied leading one and IEEE is
// a base-2 significand with one.
func TestRoundTripFloatAcrossDialects(t *testing.T) {
	t.Parallel()

	directions := []struct {
		name string
		from Encoding
		to   Encoding
	}{
		{name: "hfp to ieee", from: IBMEnterprise(), to: GnuCOBOLASCII()},
		{name: "ieee to hfp", from: GnuCOBOLASCII(), to: IBMEnterprise()},
	}

	for _, d := range directions {
		t.Run(d.name, func(t *testing.T) {
			t.Parallel()

			for _, v := range floatRoundTripValues {
				// Write the field as the source dialect spells it.
				var src bytes.Buffer
				w, err := NewWriter(&src, d.from)
				require.NoError(t, err)
				require.NoError(t, w.WriteFloat32(float32(v)))
				require.NoError(t, w.WriteFloat64(v))

				// Read it back and write it as the target dialect spells it.
				r, err := NewReader(bytes.NewReader(src.Bytes()), d.from)
				require.NoError(t, err)
				v32, err := r.ReadFloat32()
				require.NoError(t, err)
				v64, err := r.ReadFloat64()
				require.NoError(t, err)

				var dst bytes.Buffer
				w, err = NewWriter(&dst, d.to)
				require.NoError(t, err)
				require.NoError(t, w.WriteFloat32(v32))
				require.NoError(t, w.WriteFloat64(v64))

				// The bytes differ — that is the whole point of the axis — and
				// the numbers do not. Zero is the one value both formats spell
				// the same way, an all-zero field.
				if v != 0 {
					require.NotEqual(t, src.Bytes(), dst.Bytes())
				}

				r, err = NewReader(bytes.NewReader(dst.Bytes()), d.to)
				require.NoError(t, err)
				got32, err := r.ReadFloat32()
				require.NoError(t, err)
				got64, err := r.ReadFloat64()
				require.NoError(t, err)

				require.Equal(t, float32(v), got32)
				require.Equal(t, v, got64)
			}
		})
	}
}

// TestRoundTripFloatHFPLongIsExact walks the whole of HFP's exponent range and
// asserts that a float64 in it survives the long form unchanged. It is what
// says the documented claim is true: 56 bits of fraction leave room for the
// three that normalizing to a hex digit boundary can cost a 53-bit significand,
// so nothing is rounded on the way in.
func TestRoundTripFloatHFPLongIsExact(t *testing.T) {
	t.Parallel()

	enc := floatEncoding(FloatHFP, binary.BigEndian)

	// A significand using every bit a float64 has, so that any lost bit shows.
	const significand = 1.9999999999999998

	for exp := -259; exp <= 251; exp++ {
		v := math.Ldexp(significand, exp)

		var buf bytes.Buffer
		w, err := NewWriter(&buf, enc)
		require.NoError(t, err)
		require.NoError(t, w.WriteFloat64(v), "exponent %d", exp)
		require.NoError(t, w.WriteFloat64(-v), "exponent %d", exp)

		r, err := NewReader(bytes.NewReader(buf.Bytes()), enc)
		require.NoError(t, err)
		got, err := r.ReadFloat64()
		require.NoError(t, err)
		require.Equal(t, v, got, "exponent %d", exp)
		gotNeg, err := r.ReadFloat64()
		require.NoError(t, err)
		require.Equal(t, -v, gotNeg, "exponent %d", exp)
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
				ID:      "A12345",
				Name:    "WIDGET GRIP",
				Code:    "42",
				Raw:     []byte{0x00, 0x01, 0xFF},
				Amount:  12345,
				Qty:     42,
				Units:   7,
				Seq:     1234,
				Rate:    1.5,
				Factor:  2.5,
				Balance: -12345,
				Count:   42,
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
				Units:  9999,
				Seq:    -1234,
				// HFP, so both are values whose fraction survives being
				// normalized to a hex digit boundary.
				Rate:    -1.5,
				Factor:  -0.03125,
				Balance: 99999,
				Count:   999,
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
				Units:  0,
				Seq:    0,
				// A translated-EBCDIC negative one, which is the letter J.
				Balance: -1,
				Count:   0,
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
				Units:  9999,
				Seq:    9999,
				// IEEE, so the widest value each width holds is in range.
				Rate:    math.MaxFloat32,
				Factor:  math.MaxFloat64,
				Balance: -99999,
				Count:   999,
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
				Units:  1,
				Seq:    -1,
				// A value binary32 cannot spell exactly still round-trips,
				// because it is the same format on the way back.
				Rate:    -0.1,
				Factor:  0.1,
				Balance: -1,
				Count:   1,
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

// TestNewBytesWriter mirrors TestNewWriter over the byte-backed constructor,
// including the case where the two deliberately differ: a nil []byte is an
// empty buffer to append to and not a missing destination, so there is no
// ErrNilWriter case for it to be.
func TestNewBytesWriter(t *testing.T) {
	t.Parallel()

	t.Run("nil buffer is an empty buffer", func(t *testing.T) {
		t.Parallel()

		w, err := NewBytesWriter(nil, GnuCOBOLASCII())
		require.NoError(t, err)
		require.Zero(t, w.Offset())
		require.Nil(t, w.Bytes())

		require.NoError(t, w.WriteAlphanumeric("AB", 4))
		require.Equal(t, []byte("AB  "), w.Bytes())
		require.EqualValues(t, 4, w.Offset())
	})

	t.Run("appends to what the buffer already holds", func(t *testing.T) {
		t.Parallel()

		w, err := NewBytesWriter([]byte("HDR"), GnuCOBOLASCII())
		require.NoError(t, err)

		require.NoError(t, w.WriteAlphanumeric("AB", 4))
		require.Equal(t, []byte("HDRAB  "), w.Bytes())
		require.EqualValues(t, 4, w.Offset(),
			"the offset counts what this Writer wrote, not how long the slice is")
	})

	t.Run("incomplete encoding names the field", func(t *testing.T) {
		t.Parallel()

		_, err := NewBytesWriter(nil, Encoding{
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

		w, err := NewBytesWriter(nil, IBMEnterprise())
		require.NoError(t, err)
		require.Equal(t, IBMEnterprise(), w.Encoding())
		require.Zero(t, w.Offset())
	})
}

// TestNewBytesWriterValidatesExactlyAsNewWriter is
// TestNewBytesReaderValidatesExactlyAsNewReader in the other direction, and
// exists for the same reason: the two constructors share one body, and this
// fails if a later change gives either its own.
func TestNewBytesWriterValidatesExactlyAsNewWriter(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		enc  Encoding
	}{
		{name: "no encoding at all", enc: Encoding{}},
		{
			name: "no sign convention",
			enc:  Encoding{Charset: ASCII(), ByteOrder: binary.BigEndian, Float: FloatIEEE},
		},
		{
			name: "no byte order",
			enc:  Encoding{Charset: ASCII(), Sign: SignASCIIZone37, Float: FloatIEEE},
		},
		{
			name: "no float format",
			enc:  Encoding{Charset: ASCII(), Sign: SignASCIIZone37, ByteOrder: binary.BigEndian},
		},
		{name: "complete encoding", enc: GnuCOBOLASCII()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, streamErr := NewWriter(&bytes.Buffer{}, tc.enc)
			_, bytesErr := NewBytesWriter(nil, tc.enc)
			require.Equal(t, streamErr, bytesErr)
		})
	}
}

// TestWriterReset covers the method's contract clause by clause, as
// TestReaderReset does on the other side: the position restarts, the buffer is
// truncated but its capacity kept, the encoding survives, and a Writer built
// over a stream may be rewound onto a buffer.
func TestWriterReset(t *testing.T) {
	t.Parallel()

	enc := GnuCOBOLASCII()

	t.Run("offset restarts and the encoding survives", func(t *testing.T) {
		t.Parallel()

		w, err := NewBytesWriter(nil, enc)
		require.NoError(t, err)
		require.NoError(t, w.WriteAlphanumeric("AB", 4))

		w.Reset(w.Bytes())
		require.Zero(t, w.Offset())
		require.Empty(t, w.Bytes())
		require.Equal(t, enc, w.Encoding())

		require.NoError(t, w.WriteAlphanumeric("CD", 4))
		require.Equal(t, []byte("CD  "), w.Bytes())
	})

	t.Run("keeps the buffer's capacity", func(t *testing.T) {
		t.Parallel()

		buf := make([]byte, 0, 64)
		w, err := NewBytesWriter(buf, enc)
		require.NoError(t, err)
		require.NoError(t, w.WriteAlphanumeric("AB", 4))

		w.Reset(w.Bytes())
		require.NoError(t, w.WriteAlphanumeric("CD", 4))

		// Same array throughout: the second record was written where the
		// first one was, which is the allocation Reset removes.
		require.Equal(t, 64, cap(w.Bytes()))
		require.Equal(t, []byte("CD  "), w.Bytes())
	})

	t.Run("rewinds a Writer built over a stream", func(t *testing.T) {
		t.Parallel()

		var sink bytes.Buffer
		w, err := NewWriter(&sink, enc)
		require.NoError(t, err)
		require.NoError(t, w.WriteAlphanumeric("AB", 4))
		require.Equal(t, []byte("AB  "), sink.Bytes())
		require.Nil(t, w.Bytes(), "a Writer onto a stream has written nothing to a buffer")

		// The stream is dropped: what follows goes to the buffer, and the
		// stream keeps what it was already given.
		w.Reset(nil)
		require.Zero(t, w.Offset())
		require.NoError(t, w.WriteAlphanumeric("CD", 4))
		require.Equal(t, []byte("CD  "), w.Bytes())
		require.Equal(t, []byte("AB  "), sink.Bytes())
	})

	t.Run("nil drops the caller's buffer", func(t *testing.T) {
		t.Parallel()

		buf := make([]byte, 0, 64)
		w, err := NewBytesWriter(buf, enc)
		require.NoError(t, err)
		require.NoError(t, w.WriteAlphanumeric("AB", 4))

		// The pooling shape: a Writer handed back holds nothing, so the
		// record it last wrote is collectable.
		w.Reset(nil)
		require.Nil(t, w.Bytes())
	})
}

// TestMarshalMatchesTheStreamWriter holds [Marshal]'s byte-backed [Writer] to
// the bytes a [Writer] onto an [io.Writer] produces for the same record. The
// two share every encoding body and differ only in where the bytes land, and
// this is what says so.
func TestMarshalMatchesTheStreamWriter(t *testing.T) {
	t.Parallel()

	for _, enc := range []Encoding{GnuCOBOLASCII(), IBMEnterprise(), MicroFocusASCII()} {
		t.Run(enc.Charset.Name(), func(t *testing.T) {
			t.Parallel()

			rec := testRecord{
				ID:      "A12345",
				Name:    "WIDGET GRIP",
				Code:    "42",
				Raw:     []byte{0x00, 0x01, 0xFF},
				Amount:  -12345,
				Qty:     42,
				Units:   9999,
				Seq:     1234,
				Rate:    1.5,
				Factor:  2.5,
				Balance: -12345,
				Count:   42,
			}

			var sink bytes.Buffer
			w, err := NewWriter(&sink, enc)
			require.NoError(t, err)
			require.NoError(t, rec.MarshalCOBOL(w))

			data, err := Marshal(enc, &rec)
			require.NoError(t, err)
			require.Equal(t, sink.Bytes(), data)
			require.Equal(t, w.Offset(), int64(len(data)))
		})
	}
}

// TestRoundTripReusedReaderAndWriter is the acceptance criterion of #115 in
// both directions at once: one [Writer] and one [Reader], rewound onto each of
// several records with [Writer.Reset] and [Reader.Reset], with every record
// required to survive the ones written and read after it.
//
// The two halves are separate for a reason. The writer's buffer is reused at
// its capacity, so record two is written over record one's bytes; the values
// decoded from record one therefore have to be copies, and would fail here if
// any accessor handed out a window into either the caller's slice or the
// Reader's scratch. Encoding every record before decoding any of them is what
// makes that overwrite happen while the earlier values are still live.
func TestRoundTripReusedReaderAndWriter(t *testing.T) {
	t.Parallel()

	enc := GnuCOBOLASCII()

	want := []testRecord{
		{
			ID:      "A12345",
			Name:    "WIDGET GRIP",
			Code:    "42",
			Raw:     []byte{0x00, 0x01, 0xFF},
			Amount:  -12345,
			Qty:     42,
			Units:   9999,
			Seq:     1234,
			Rate:    1.5,
			Factor:  2.5,
			Balance: -12345,
			Count:   42,
		},
		{
			ID:      "B98765",
			Name:    "SPROCKET",
			Code:    "7",
			Raw:     []byte{0xFE, 0x80, 0x02},
			Amount:  54321,
			Qty:     7,
			Units:   1,
			Seq:     -4321,
			Rate:    -0.25,
			Factor:  -8.75,
			Balance: 54321,
			Count:   7,
		},
		{
			ID:      "C24680",
			Name:    "BRACKET ASSY",
			Code:    "99",
			Raw:     []byte{0x7F, 0x00, 0x01},
			Amount:  1,
			Qty:     1,
			Units:   5,
			Seq:     9999,
			Rate:    0.5,
			Factor:  -1.25,
			Balance: -1,
			Count:   999,
		},
	}

	// One Writer for every record, its buffer reused at capacity. Each
	// record's bytes are copied out because the next Reset writes over
	// them — which is what [Writer.Bytes] documents.
	w, err := NewBytesWriter(nil, enc)
	require.NoError(t, err)

	records := make([][]byte, len(want))
	var scratch []byte
	for i, rec := range want {
		w.Reset(scratch)
		require.NoError(t, rec.MarshalCOBOL(w))
		require.EqualValues(t, testRecordWidth, w.Offset(),
			"the offset restarts per record, so it ends at the record's width")

		scratch = w.Bytes()
		require.Len(t, scratch, testRecordWidth)
		records[i] = append([]byte(nil), scratch...)
	}

	// One Reader for every record, rewound onto each in turn.
	r, err := NewBytesReader(nil, enc)
	require.NoError(t, err)

	got := make([]testRecord, len(want))
	for i, data := range records {
		r.Reset(data)
		require.NoError(t, got[i].UnmarshalCOBOL(r))
		require.EqualValues(t, testRecordWidth, r.Offset())
	}

	require.Equal(t, want, got)
}

// TestBytesWriterGrowsForAFieldWiderThanTheFloor covers the arm of
// [Writer.grow] the record-sized cases never reach: one field wider than both
// twice the buffer's capacity and the 64-byte floor, which is what a PIC X(n)
// comment or a binary payload is. The growth policy is an optimisation, so what
// is asserted is that it changes nothing about the bytes — the field is written
// whole, at the offset it started at, whatever the buffer had to do to hold it.
func TestBytesWriterGrowsForAFieldWiderThanTheFloor(t *testing.T) {
	t.Parallel()

	const width = 200

	w, err := NewBytesWriter(make([]byte, 0, 8), GnuCOBOLASCII())
	require.NoError(t, err)

	require.NoError(t, w.WriteAlphanumeric("ACME", width))
	require.EqualValues(t, width, w.Offset())

	want := append([]byte("ACME"), bytes.Repeat([]byte(" "), width-4)...)
	require.Equal(t, want, w.Bytes())

	// And a second field lands after the first, not over it.
	require.NoError(t, w.WriteAlphanumeric("CO", 4))
	require.Equal(t, append(want, []byte("CO  ")...), w.Bytes())
	require.EqualValues(t, width+4, w.Offset())
}
