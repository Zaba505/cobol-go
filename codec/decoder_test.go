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

// testRecord is an eleven-field record exercising every accessor family the
// package currently has: two left-justified alphanumeric fields, a JUSTIFIED
// RIGHT one, a raw byte field, a signed packed decimal field, an unsigned one,
// a signed binary one, the two floating point widths, a signed zoned decimal
// field and an unsigned one. It stands in for the code a generator will emit,
// for
//
//	01 TEST-RECORD.
//	   05 ID      PIC X(6).
//	   05 NAME    PIC X(12).
//	   05 CODE    PIC X(4) JUSTIFIED RIGHT.
//	   05 RAW     PIC X(3).
//	   05 AMOUNT  PIC S9(5) COMP-3.
//	   05 QTY     PIC 9(4)  COMP-3.
//	   05 SEQ     PIC S9(4) COMP.
//	   05 RATE    COMP-1.
//	   05 FACTOR  COMP-2.
//	   05 BALANCE PIC S9(5) DISPLAY.
//	   05 COUNT   PIC 9(3)  DISPLAY.
//
// RATE and FACTOR carry no PICTURE, because COMP-1 and COMP-2 do not take one.
//
// That one record mixes DISPLAY, COMP-3, COMP, COMP-1 and COMP-2 fields is the
// point rather than an accident: USAGE is a property of each item, so a
// generator computes every field's width from its own usage and no file is in
// one numeric mode.
type testRecord struct {
	ID      string
	Name    string
	Code    string
	Raw     []byte
	Amount  int64
	Qty     int32
	Seq     int16
	Rate    float32
	Factor  float64
	Balance int64
	Count   int32
}

const (
	testRecordIDWidth       = 6
	testRecordNameWidth     = 12
	testRecordCodeWidth     = 4
	testRecordRawWidth      = 3
	testRecordAmountDigits  = 5
	testRecordQtyDigits     = 4
	testRecordSeqDigits     = 4
	testRecordBalanceDigits = 5
	testRecordCountDigits   = 3
)

// testRecordBalanceSign is the sign position of BALANCE: the COBOL default for
// an item whose PICTURE carries S, overpunched into the last digit byte.
const testRecordBalanceSign = SignTrailing

// testRecordWidth is the record's length in bytes, zoned, packed, binary and
// floating point fields included. SEQ contributes two bytes and not four: a
// binary field's width is a staircase in its digit count, not the digit count
// itself. BALANCE contributes five and not six, because a TRAILING sign is
// overpunched rather than given a byte. RATE and FACTOR contribute a fixed four
// and eight, since neither has a digit count for a width to depend on.
var testRecordWidth = testRecordIDWidth + testRecordNameWidth + testRecordCodeWidth +
	testRecordRawWidth + packedWidth(testRecordAmountDigits) + packedWidth(testRecordQtyDigits) +
	binaryWidth(testRecordSeqDigits) + comp1Width + comp2Width +
	zonedWidth(testRecordBalanceDigits, testRecordBalanceSign) +
	zonedWidth(testRecordCountDigits, SignUnsigned)

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
	if err := w.WriteBinaryInt16(r.Seq, testRecordSeqDigits, Signed); err != nil {
		return err
	}
	if err := w.WriteFloat32(r.Rate); err != nil {
		return err
	}
	if err := w.WriteFloat64(r.Factor); err != nil {
		return err
	}
	if err := w.WriteZonedInt64(r.Balance, testRecordBalanceDigits, testRecordBalanceSign); err != nil {
		return err
	}
	return w.WriteZonedInt32(r.Count, testRecordCountDigits, SignUnsigned)
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
	if r.Seq, err = rd.ReadBinaryInt16(testRecordSeqDigits); err != nil {
		return err
	}
	if r.Rate, err = rd.ReadFloat32(); err != nil {
		return err
	}
	if r.Factor, err = rd.ReadFloat64(); err != nil {
		return err
	}
	if r.Balance, err = rd.ReadZonedInt64(testRecordBalanceDigits, testRecordBalanceSign); err != nil {
		return err
	}
	r.Count, err = rd.ReadZonedInt32(testRecordCountDigits, SignUnsigned)
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

// floatEncoding returns an encoding whose floating point format is f and whose
// byte order is bo. Those are the only two axes a COMP-1 or COMP-2 field reads,
// and the named bundles pin the pair together — every one of them is either
// IEEE with a platform order or HFP big-endian — where the point of these tests
// is that the two axes are independent.
func floatEncoding(f FloatFormat, bo binary.ByteOrder) Encoding {
	enc := GnuCOBOLASCII()
	enc.Float = f
	enc.ByteOrder = bo
	return enc
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

// zonedEncoding returns an encoding whose charset is cs and whose sign
// convention is s. Those are the only two axes a zoned decimal field reads, and
// the named bundles cannot express every pairing of them: [SignRealia] has no
// bundle at all, and the point of these tests is that the charset and the
// convention are independent axes rather than one dialect flag.
func zonedEncoding(cs Charset, s SignConvention) Encoding {
	enc := GnuCOBOLASCII()
	enc.Charset = cs
	enc.Sign = s
	return enc
}

// TestReaderReadZoned walks the test vectors of codec/SPEC.md, Appendix A.1 and
// A.2, in both charsets and every sign convention: the same number spelled the
// way each combination spells it.
func TestReaderReadZoned(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		src    []byte
		digits int
		sign   SignPosition
		enc    Encoding
		want   int64
	}{
		{
			name:   "ebcdic trailing overpunch, positive",
			src:    []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xC5}, // PIC S9(5) DISPLAY, +12345
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   12345,
		},
		{
			name:   "ebcdic trailing overpunch, negative",
			src:    []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xD5}, // PIC S9(5) DISPLAY, -12345
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   -12345,
		},
		{
			name:   "ascii zone 3/7, positive",
			src:    []byte{0x31, 0x32, 0x33, 0x34, 0x35}, // "12345"
			digits: 5,
			sign:   SignTrailing,
			enc:    GnuCOBOLASCII(),
			want:   12345,
		},
		{
			name:   "ascii zone 3/7, negative",
			src:    []byte{0x31, 0x32, 0x33, 0x34, 0x75}, // "1234u"
			digits: 5,
			sign:   SignTrailing,
			enc:    GnuCOBOLASCII(),
			want:   -12345,
		},
		{
			name:   "translated ebcdic, positive",
			src:    []byte{0x31, 0x32, 0x33, 0x34, 0x45}, // "1234E"
			digits: 5,
			sign:   SignTrailing,
			enc:    ConvertedFromEBCDIC(),
			want:   12345,
		},
		{
			name:   "translated ebcdic, negative",
			src:    []byte{0x31, 0x32, 0x33, 0x34, 0x4E}, // "1234N"
			digits: 5,
			sign:   SignTrailing,
			enc:    ConvertedFromEBCDIC(),
			want:   -12345,
		},
		{
			name:   "realia, positive",
			src:    []byte{0x31, 0x32, 0x33, 0x34, 0x35}, // "12345"
			digits: 5,
			sign:   SignTrailing,
			enc:    zonedEncoding(ASCII(), SignRealia),
			want:   12345,
		},
		{
			name:   "realia, negative",
			src:    []byte{0x31, 0x32, 0x33, 0x34, 0x25}, // "1234%"
			digits: 5,
			sign:   SignTrailing,
			enc:    zonedEncoding(ASCII(), SignRealia),
			want:   -12345,
		},
		{
			name:   "realia negative zero is zero",
			src:    []byte{0x20}, // a space, which is how Realia spells -0
			digits: 1,
			sign:   SignTrailing,
			enc:    zonedEncoding(ASCII(), SignRealia),
			want:   0,
		},
		{
			name:   "ebcdic leading overpunch, negative",
			src:    []byte{0xD1, 0xF2, 0xF3, 0xF4, 0xF5}, // PIC S9(5) SIGN LEADING
			digits: 5,
			sign:   SignLeading,
			enc:    IBMEnterprise(),
			want:   -12345,
		},
		{
			name:   "ascii leading overpunch, negative",
			src:    []byte{0x71, 0x32, 0x33, 0x34, 0x35}, // "q2345"
			digits: 5,
			sign:   SignLeading,
			enc:    GnuCOBOLASCII(),
			want:   -12345,
		},
		{
			name:   "ebcdic unsigned",
			src:    []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5}, // PIC 9(5) DISPLAY
			digits: 5,
			sign:   SignUnsigned,
			enc:    IBMEnterprise(),
			want:   12345,
		},
		{
			name:   "ascii unsigned",
			src:    []byte{0x31, 0x32, 0x33, 0x34, 0x35}, // PIC 9(5) DISPLAY
			digits: 5,
			sign:   SignUnsigned,
			enc:    GnuCOBOLASCII(),
			want:   12345,
		},
		{
			name:   "ascii leading separate, negative",
			src:    []byte{0x2D, 0x31, 0x32, 0x33, 0x34, 0x35}, // "-12345"
			digits: 5,
			sign:   SignLeadingSeparate,
			enc:    GnuCOBOLASCII(),
			want:   -12345,
		},
		{
			name:   "ebcdic leading separate, negative",
			src:    []byte{0x60, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5},
			digits: 5,
			sign:   SignLeadingSeparate,
			enc:    IBMEnterprise(),
			want:   -12345,
		},
		{
			name:   "ascii trailing separate, positive",
			src:    []byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x2B}, // "12345+"
			digits: 5,
			sign:   SignTrailingSeparate,
			enc:    GnuCOBOLASCII(),
			want:   12345,
		},
		{
			name:   "ebcdic trailing separate, positive",
			src:    []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0x4E},
			digits: 5,
			sign:   SignTrailingSeparate,
			enc:    IBMEnterprise(),
			want:   12345,
		},
		{
			name: "a separate sign is convention-independent",
			src:  []byte{0x2D, 0x31, 0x32, 0x33, 0x34, 0x35},
			// Realia rather than zone 3/7, on bytes that carry nothing
			// either convention could disagree about.
			digits: 5,
			sign:   SignLeadingSeparate,
			enc:    zonedEncoding(ASCII(), SignRealia),
			want:   -12345,
		},
		{
			name:   "unsigned zone in a signed field reads non-negative",
			src:    []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5}, // after a MOVE from an unsigned item
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   12345,
		},
		{
			name:   "lenient ebcdic zone A reads positive",
			src:    []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xA5},
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   12345,
		},
		{
			name:   "lenient ebcdic zone B reads negative",
			src:    []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xB5},
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   -12345,
		},
		{
			name:   "single digit",
			src:    []byte{0xC7}, // PIC S9(1) DISPLAY, +7
			digits: 1,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   7,
		},
		{
			name:   "zero",
			src:    []byte{0xF0, 0xF0, 0xF0, 0xF0, 0xC0},
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   0,
		},
		{
			name:   "high-order zeros are not digits of the value",
			src:    []byte{0xF0, 0xF0, 0xF0, 0xF4, 0xC2}, // +42
			digits: 5,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   42,
		},
		{
			name:   "nine digits, the int32 maximum",
			src:    []byte{0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xD9},
			digits: 9,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   -999999999,
		},
		{
			name: "eighteen digits, the int64 maximum",
			src: []byte{
				0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9,
				0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xF9, 0xC9,
			},
			digits: 18,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   999999999999999999,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := NewReader(bytes.NewReader(tc.src), tc.enc)
			require.NoError(t, err)

			got, err := r.ReadZonedInt64(tc.digits, tc.sign)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, int64(len(tc.src)), r.Offset())

			// A DISPLAY field is one byte per digit, plus one for a
			// SEPARATE sign and nothing else: no V and no scale.
			require.Equal(t, len(tc.src), zonedWidth(tc.digits, tc.sign))

			if tc.digits <= maxZonedInt32Digits {
				r32, err := NewReader(bytes.NewReader(tc.src), tc.enc)
				require.NoError(t, err)

				got32, err := r32.ReadZonedInt32(tc.digits, tc.sign)
				require.NoError(t, err)
				require.Equal(t, int32(tc.want), got32)
			}

			rb, err := NewReader(bytes.NewReader(tc.src), tc.enc)
			require.NoError(t, err)

			gotBig, err := rb.ReadZonedBig(tc.digits, tc.sign)
			require.NoError(t, err)
			require.Equal(t, big.NewInt(tc.want), gotBig)
		})
	}
}

func TestReaderReadZonedBig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		src    []byte
		digits int
		sign   SignPosition
		enc    Encoding
		want   string
	}{
		{
			name:   "nineteen digits, one past int64",
			src:    []byte("123456789012345678" + "\x79"), // "…8y", zone 7 negative 9
			digits: 19,
			sign:   SignTrailing,
			enc:    GnuCOBOLASCII(),
			want:   "-1234567890123456789",
		},
		{
			name: "thirty-one digits, the COBOL maximum",
			src: append(
				bytes.Repeat([]byte{0xF9}, 30),
				0xC9, // the sign-carrying digit, positive
			),
			digits: 31,
			sign:   SignTrailing,
			enc:    IBMEnterprise(),
			want:   "9999999999999999999999999999999",
		},
		{
			name:   "thirty-one digits, unsigned",
			src:    []byte("1234567890123456789012345678901"),
			digits: 31,
			sign:   SignUnsigned,
			enc:    GnuCOBOLASCII(),
			want:   "1234567890123456789012345678901",
		},
		{
			name:   "thirty-one digits, leading separate",
			src:    append([]byte{0x2D}, []byte("1234567890123456789012345678901")...),
			digits: 31,
			sign:   SignLeadingSeparate,
			enc:    GnuCOBOLASCII(),
			want:   "-1234567890123456789012345678901",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := NewReader(bytes.NewReader(tc.src), tc.enc)
			require.NoError(t, err)

			got, err := r.ReadZonedBig(tc.digits, tc.sign)
			require.NoError(t, err)
			require.Equal(t, tc.want, got.String())
			require.Equal(t, int64(zonedWidth(tc.digits, tc.sign)), r.Offset())
		})
	}
}

// TestReaderReadZonedRejectsOtherConventions walks codec/SPEC.md, Appendix A.3:
// every row is a field that is valid under some convention and must be refused
// under the one declared, which is what makes a wrong convention loud rather
// than silently wrong signs.
func TestReaderReadZonedRejectsOtherConventions(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      []byte
		enc      Encoding
		wantByte byte
		wantAt   int64
	}{
		{
			name:     "translated positive zero under zone 3/7",
			src:      []byte{0x31, 0x32, 0x33, 0x34, 0x7B}, // "1234{"
			enc:      GnuCOBOLASCII(),
			wantByte: 0x7B,
			wantAt:   4,
		},
		{
			name:     "zone 3/7 negative under translated ebcdic",
			src:      []byte{0x31, 0x32, 0x33, 0x34, 0x75}, // "1234u"
			enc:      ConvertedFromEBCDIC(),
			wantByte: 0x75,
			wantAt:   4,
		},
		{
			name:     "realia negative under zone 3/7",
			src:      []byte{0x31, 0x32, 0x33, 0x34, 0x25}, // "1234%"
			enc:      GnuCOBOLASCII(),
			wantByte: 0x25,
			wantAt:   4,
		},
		{
			name:     "ebcdic negative under zone 3/7",
			src:      []byte{0x31, 0x32, 0x33, 0x34, 0xD5},
			enc:      GnuCOBOLASCII(),
			wantByte: 0xD5,
			wantAt:   4,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := NewReader(bytes.NewReader(tc.src), tc.enc)
			require.NoError(t, err)

			_, err = r.ReadZonedInt64(5, SignTrailing)

			var signErr ZonedSignError
			require.ErrorAs(t, err, &signErr)
			require.Equal(t, tc.wantByte, signErr.Byte)
			require.Equal(t, tc.enc.Sign, signErr.Sign)

			// The offset names the byte at fault, not the end of the field.
			var offErr *OffsetError
			require.ErrorAs(t, err, &offErr)
			require.Equal(t, tc.wantAt, offErr.Offset)
		})
	}
}

func TestReaderReadZonedErrors(t *testing.T) {
	t.Parallel()

	t.Run("wrong charset is caught at the first digit", func(t *testing.T) {
		t.Parallel()

		// codec/SPEC.md, A.3: EBCDIC digits read under an ASCII charset.
		// Nothing about the field is plausible, and offset 0 says so.
		r, err := NewReader(bytes.NewReader([]byte{0xF1, 0xF2, 0xF3, 0xF4, 0xD5}), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadZonedInt64(5, SignTrailing)

		var digitErr ZonedDigitError
		require.ErrorAs(t, err, &digitErr)
		require.Equal(t, byte(0xF1), digitErr.Byte)
		require.Equal(t, "ASCII", digitErr.Charset)
		require.Equal(t, byte(0x30), digitErr.Zero)
		require.Equal(t, byte(0x39), digitErr.Nine)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Zero(t, offErr.Offset)
	})

	t.Run("the other wrong charset is caught the same way", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x31, 0x32, 0x33, 0x34, 0x35}), IBMEnterprise())
		require.NoError(t, err)

		_, err = r.ReadZonedInt64(5, SignTrailing)

		var digitErr ZonedDigitError
		require.ErrorAs(t, err, &digitErr)
		require.Equal(t, byte(0x31), digitErr.Byte)
		require.Equal(t, "cp037", digitErr.Charset)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Zero(t, offErr.Offset)
	})

	t.Run("a non-digit inside the field names its own byte", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x31, 0x32, 0x3A, 0x34, 0x35}), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadZonedInt64(5, SignTrailing)

		var digitErr ZonedDigitError
		require.ErrorAs(t, err, &digitErr)
		require.Equal(t, byte(0x3A), digitErr.Byte)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Equal(t, int64(2), offErr.Offset)
	})

	t.Run("a leading separate sign shifts the digit offsets", func(t *testing.T) {
		t.Parallel()

		// The bad byte is the third digit, which is the fourth byte of the
		// field: the sign byte ahead of the digits is counted too.
		r, err := NewReader(bytes.NewReader([]byte{0x2D, 0x31, 0x32, 0x3A, 0x34, 0x35}), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadZonedInt64(5, SignLeadingSeparate)

		var digitErr ZonedDigitError
		require.ErrorAs(t, err, &digitErr)
		require.Equal(t, byte(0x3A), digitErr.Byte)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Equal(t, int64(3), offErr.Offset)
	})

	t.Run("a digit byte in the leading sign position", func(t *testing.T) {
		t.Parallel()

		// An overpunched LEADING sign under EBCDIC: F1 is the unsigned zone,
		// which is a non-negative 1 and not an error, but 31 is not a byte
		// cp037 spells any digit with.
		r, err := NewReader(bytes.NewReader([]byte{0x31, 0xF2, 0xF3, 0xF4, 0xF5}), IBMEnterprise())
		require.NoError(t, err)

		_, err = r.ReadZonedInt64(5, SignLeading)

		var signErr ZonedSignError
		require.ErrorAs(t, err, &signErr)
		require.Equal(t, byte(0x31), signErr.Byte)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Zero(t, offErr.Offset)
	})

	t.Run("separate sign byte that is neither plus nor minus", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name   string
			src    []byte
			sign   SignPosition
			wantAt int64
		}{
			{
				name:   "leading",
				src:    []byte{0x2A, 0x31, 0x32, 0x33, 0x34, 0x35}, // '*'
				sign:   SignLeadingSeparate,
				wantAt: 0,
			},
			{
				name:   "trailing",
				src:    []byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x2A},
				sign:   SignTrailingSeparate,
				wantAt: 5,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				r, err := NewReader(bytes.NewReader(tc.src), GnuCOBOLASCII())
				require.NoError(t, err)

				_, err = r.ReadZonedInt64(5, tc.sign)

				var sepErr ZonedSeparateSignError
				require.ErrorAs(t, err, &sepErr)
				require.Equal(t, byte(0x2A), sepErr.Byte)
				require.Equal(t, byte(0x2B), sepErr.Plus)
				require.Equal(t, byte(0x2D), sepErr.Minus)

				var offErr *OffsetError
				require.ErrorAs(t, err, &offErr)
				require.Equal(t, tc.wantAt, offErr.Offset)
			})
		}
	})

	t.Run("digit count outside the accessor's range", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name    string
			digits  int
			read    func(*Reader, int) error
			wantMax int
		}{
			{
				name:   "zero digits",
				digits: 0,
				read: func(r *Reader, d int) error {
					_, err := r.ReadZonedInt64(d, SignTrailing)
					return err
				},
				wantMax: maxZonedInt64Digits,
			},
			{
				name:   "ten digits into an int32",
				digits: 10,
				read: func(r *Reader, d int) error {
					_, err := r.ReadZonedInt32(d, SignTrailing)
					return err
				},
				wantMax: maxZonedInt32Digits,
			},
			{
				name:   "nineteen digits into an int64",
				digits: 19,
				read: func(r *Reader, d int) error {
					_, err := r.ReadZonedInt64(d, SignTrailing)
					return err
				},
				wantMax: maxZonedInt64Digits,
			},
			{
				name:   "thirty-two digits into a big.Int",
				digits: 32,
				read: func(r *Reader, d int) error {
					_, err := r.ReadZonedBig(d, SignTrailing)
					return err
				},
				wantMax: maxZonedDigits,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				r, err := NewReader(bytes.NewReader(bytes.Repeat([]byte{0x30}, 64)), GnuCOBOLASCII())
				require.NoError(t, err)

				err = tc.read(r, tc.digits)

				var countErr ZonedDigitCountError
				require.ErrorAs(t, err, &countErr)
				require.Equal(t, tc.digits, countErr.Digits)
				require.Equal(t, tc.wantMax, countErr.Max)

				// A field that would have overflowed is refused before a
				// byte of it is consumed.
				require.Zero(t, r.Offset())
			})
		}
	})

	t.Run("sign position is required", func(t *testing.T) {
		t.Parallel()

		for _, s := range []SignPosition{SignPositionUnset, SignPosition(99)} {
			r, err := NewReader(bytes.NewReader([]byte("12345")), GnuCOBOLASCII())
			require.NoError(t, err)

			_, err = r.ReadZonedInt64(5, s)

			var posErr SignPositionError
			require.ErrorAs(t, err, &posErr)
			require.Equal(t, s, posErr.SignPosition)
			require.Zero(t, r.Offset())
		}
	})

	t.Run("short field", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("123")), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadZonedInt64(5, SignTrailing)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("a separate sign is part of the width", func(t *testing.T) {
		t.Parallel()

		// Five digit bytes are there; the sign byte is not, and a field is
		// short when any byte of it is.
		r, err := NewReader(bytes.NewReader([]byte("12345")), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadZonedInt64(5, SignTrailingSeparate)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("end of file", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader(nil), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadZonedInt64(5, SignTrailing)
		require.ErrorIs(t, err, io.EOF)
	})

	t.Run("a charset that cannot spell a numeric field", func(t *testing.T) {
		t.Parallel()

		// Reported by the first zoned field rather than by NewReader: such a
		// charset still reads alphanumeric fields perfectly well.
		r, err := NewReader(bytes.NewReader([]byte("12345")), zonedEncoding(partialCharset{}, SignASCIIZone37))
		require.NoError(t, err)

		s, err := r.ReadAlphanumeric(2)
		require.NoError(t, err)
		require.Equal(t, "12", s)

		_, err = r.ReadZonedInt64(3, SignTrailing)

		var runeErr UnrepresentableRuneError
		require.ErrorAs(t, err, &runeErr)
		require.Equal(t, '0', runeErr.Rune)
	})
}

// TestReaderReadMixedUsageRecord decodes the record of codec/SPEC.md, Appendix
// A.7, with a USAGE DISPLAY field added to it. It is the guarantee that usage
// is a property of each item rather than a mode of the file: one record holds a
// DISPLAY field, a COMP-3 field and a COMP field, each read at its own width.
func TestReaderReadMixedUsageRecord(t *testing.T) {
	t.Parallel()

	// 01 TXN.
	//    05 ID   PIC X(4).
	//    05 BAL  PIC S9(5)       DISPLAY.
	//    05 AMT  PIC S9(5)       COMP-3.
	//    05 QTY  PIC S9(4)       COMP.
	//    05 NAME PIC X(6).
	// 4 + 5 + 3 + 2 + 6 = 20 bytes.
	testCases := []struct {
		name string
		enc  Encoding
		src  []byte
	}{
		{
			name: "ibm enterprise",
			enc:  IBMEnterprise(),
			src: []byte{
				0xC1, 0xF1, 0xF2, 0xF3, // ID   "A123"
				0xF1, 0xF2, 0xF3, 0xF4, 0xD5, // BAL  -12345, EBCDIC overpunch
				0x12, 0x34, 0x5D, // AMT  -12345, packed
				0x04, 0xD2, // QTY  1234, big-endian
				0xE6, 0xC9, 0xC4, 0xC7, 0xC5, 0xE3, // NAME "WIDGET"
			},
		},
		{
			name: "gnucobol ascii",
			enc:  GnuCOBOLASCII(),
			src: []byte{
				0x41, 0x31, 0x32, 0x33, // ID   "A123"
				0x31, 0x32, 0x33, 0x34, 0x75, // BAL  -12345, zone 3/7 overpunch
				0x12, 0x34, 0x5D, // AMT  -12345, packed
				0x04, 0xD2, // QTY  1234, big-endian
				0x57, 0x49, 0x44, 0x47, 0x45, 0x54, // NAME "WIDGET"
			},
		},
		{
			name: "converted from ebcdic",
			enc:  ConvertedFromEBCDIC(),
			src: []byte{
				0x41, 0x31, 0x32, 0x33, // ID   "A123"
				0x31, 0x32, 0x33, 0x34, 0x4E, // BAL  -12345, translated overpunch
				0x12, 0x34, 0x5D, // AMT  -12345, packed
				0x04, 0xD2, // QTY  1234, big-endian
				0x57, 0x49, 0x44, 0x47, 0x45, 0x54, // NAME "WIDGET"
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := NewReader(bytes.NewReader(tc.src), tc.enc)
			require.NoError(t, err)

			id, err := r.ReadAlphanumeric(4)
			require.NoError(t, err)
			require.Equal(t, "A123", id)

			bal, err := r.ReadZonedInt64(5, SignTrailing)
			require.NoError(t, err)
			require.Equal(t, int64(-12345), bal)

			amt, err := r.ReadPackedInt64(5)
			require.NoError(t, err)
			require.Equal(t, int64(-12345), amt)

			qty, err := r.ReadBinaryInt16(4)
			require.NoError(t, err)
			require.Equal(t, int16(1234), qty)

			name, err := r.ReadAlphanumeric(6)
			require.NoError(t, err)
			require.Equal(t, "WIDGET", name)

			require.Equal(t, int64(len(tc.src)), r.Offset())

			// The DISPLAY field is spelled differently in every row above
			// and the COMP-3 field identically in all three: a zoned sign
			// is charset- and convention-sensitive where a packed one is
			// neither. See codec/SPEC.md, "Charset invariance".
			require.Equal(t, []byte{0x12, 0x34, 0x5D}, tc.src[9:12])
		})
	}
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

// TestReaderReadFloatIEEE reads binary32 and binary64 in both byte orders. The
// bytes are stated big-endian and reversed for the little-endian subtest, which
// is the axis a COMP-1 field does read — unlike HFP, which is big-endian
// whatever [Encoding.ByteOrder] says.
func TestReaderReadFloatIEEE(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		be32 []byte
		be64 []byte
		want float64
	}{
		{
			name: "one",
			be32: []byte{0x3F, 0x80, 0x00, 0x00},
			be64: []byte{0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want: 1,
		},
		{
			name: "zero",
			be32: []byte{0x00, 0x00, 0x00, 0x00},
			be64: []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want: 0,
		},
		{
			name: "negative one",
			be32: []byte{0xBF, 0x80, 0x00, 0x00},
			be64: []byte{0xBF, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want: -1,
		},
		{
			name: "one and a half",
			be32: []byte{0x3F, 0xC0, 0x00, 0x00},
			be64: []byte{0x3F, 0xF8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want: 1.5,
		},
		{
			name: "a thirty-second",
			be32: []byte{0x3D, 0x00, 0x00, 0x00},
			be64: []byte{0x3F, 0xA0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want: 0.03125,
		},
		{
			name: "nine",
			be32: []byte{0x41, 0x10, 0x00, 0x00},
			be64: []byte{0x40, 0x22, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want: 9,
		},
	}

	for _, tc := range testCases {
		for _, bo := range binaryOrders {
			t.Run(tc.name+", "+bo.String(), func(t *testing.T) {
				t.Parallel()

				enc := floatEncoding(FloatIEEE, bo)

				r, err := NewReader(bytes.NewReader(inByteOrder(bo, tc.be32)), enc)
				require.NoError(t, err)
				got32, err := r.ReadFloat32()
				require.NoError(t, err)
				require.Equal(t, float32(tc.want), got32)
				require.Equal(t, int64(comp1Width), r.Offset())

				r, err = NewReader(bytes.NewReader(inByteOrder(bo, tc.be64)), enc)
				require.NoError(t, err)
				got64, err := r.ReadFloat64()
				require.NoError(t, err)
				require.Equal(t, tc.want, got64)
				require.Equal(t, int64(comp2Width), r.Offset())
			})
		}
	}
}

// TestReaderReadFloatHFP reads known IBM hexadecimal floating point bit
// patterns. Between them they cover the three parts of the format the
// conversion has to get right: the sign bit, the excess-64 base-16 exponent and
// the normalized fraction with no implied leading one.
func TestReaderReadFloatHFP(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		short []byte
		long  []byte
		want  float64
	}{
		{
			name: "true zero is the all-zero field",
			// Exponent and fraction alike; HFP has no implied leading one to
			// make a zero fraction denote anything else.
			short: []byte{0x00, 0x00, 0x00, 0x00},
			long:  []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  0,
		},
		{
			name: "one",
			// 0.1₁₆ × 16^1: exponent 0x41 is 65, which is 1 in excess-64.
			short: []byte{0x41, 0x10, 0x00, 0x00},
			long:  []byte{0x41, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  1,
		},
		{
			name: "negative one is the same field with the sign bit set",
			// The top bit of 0x41 is the sign, so the exponent reads 0x41
			// either way: HFP is sign-magnitude, not two's complement.
			short: []byte{0xC1, 0x10, 0x00, 0x00},
			long:  []byte{0xC1, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  -1,
		},
		{
			name: "a half",
			// 0.8₁₆ × 16^0: the fraction is a base-16 fraction, so its leading
			// digit 8 is a half rather than the 1.0 an IEEE significand starts
			// from.
			short: []byte{0x40, 0x80, 0x00, 0x00},
			long:  []byte{0x40, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  0.5,
		},
		{
			name: "a sixteenth is the smallest normalized fraction",
			// 0.1₁₆ × 16^0. Any smaller fraction would leave the leading hex
			// digit zero, which is what normalization forbids.
			short: []byte{0x40, 0x10, 0x00, 0x00},
			long:  []byte{0x40, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  0.0625,
		},
		{
			name: "sixteen steps the exponent, not the fraction",
			// 0.1₁₆ × 16^2 — the same fraction as one, one exponent up.
			short: []byte{0x42, 0x10, 0x00, 0x00},
			long:  []byte{0x42, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  16,
		},
		{
			name: "two hundred and fifty-five fills both fraction digits",
			// 0.FF₁₆ × 16^2.
			short: []byte{0x42, 0xFF, 0x00, 0x00},
			long:  []byte{0x42, 0xFF, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  255,
		},
		{
			name: "a fraction below one sixteenth borrows an exponent",
			// 0.8₁₆ × 16^-1: exponent 0x3F is 63, which is -1 in excess-64.
			// These are also the bytes IEEE spells 1.0 with, which is the
			// worked example in codec/SPEC.md.
			short: []byte{0x3F, 0x80, 0x00, 0x00},
			long:  []byte{0x3F, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  0.03125,
		},
		{
			name: "a value using every fraction digit of the short form",
			// 0.7B4₁₆ × 16^2.
			short: []byte{0x42, 0x7B, 0x40, 0x00},
			long:  []byte{0x42, 0x7B, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  123.25,
		},
		{
			name: "an unnormalized field decodes rather than failing",
			// 0.01₁₆ × 16^2 is 1.0 written with a zero leading digit. Nothing
			// here writes one, but z/OS arithmetic produces them, and the
			// formula gives the right answer without any special case.
			short: []byte{0x42, 0x01, 0x00, 0x00},
			long:  []byte{0x42, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			want:  1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The byte order axis is stated little-endian deliberately: HFP is
			// big-endian regardless of it, and a reader that consulted the axis
			// would read every one of these backwards.
			enc := floatEncoding(FloatHFP, binary.LittleEndian)

			r, err := NewReader(bytes.NewReader(tc.short), enc)
			require.NoError(t, err)
			got32, err := r.ReadFloat32()
			require.NoError(t, err)
			require.Equal(t, float32(tc.want), got32)
			require.Equal(t, int64(comp1Width), r.Offset())

			r, err = NewReader(bytes.NewReader(tc.long), enc)
			require.NoError(t, err)
			got64, err := r.ReadFloat64()
			require.NoError(t, err)
			require.Equal(t, tc.want, got64)
			require.Equal(t, int64(comp2Width), r.Offset())
		})
	}
}

// TestReaderReadFloatHFPExtremes reads the two ends of HFP's range, which is
// far wider than binary32's at both of them. A float64 takes them; a float32
// does not, and reports rather than returning an infinity or a zero.
func TestReaderReadFloatHFPExtremes(t *testing.T) {
	t.Parallel()

	enc := floatEncoding(FloatHFP, binary.BigEndian)

	t.Run("the largest short value overflows a float32", func(t *testing.T) {
		t.Parallel()

		// 0.FFFFFF₁₆ × 16^63, about 7.2e75, against a float32 that stops at
		// about 3.4e38.
		r, err := NewReader(bytes.NewReader([]byte{0x7F, 0xFF, 0xFF, 0xFF}), enc)
		require.NoError(t, err)

		_, err = r.ReadFloat32()

		var rangeErr FloatRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, FloatHFP, rangeErr.Format)
		require.Equal(t, comp1Width, rangeErr.Width)
		require.Equal(t, "overflows a float32", rangeErr.Reason)

		// The offset names the byte the field starts at, not the one it ends
		// at: the error is a statement about the whole field.
		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Zero(t, offErr.Offset)
	})

	t.Run("the smallest short value underflows a float32", func(t *testing.T) {
		t.Parallel()

		// 0.1₁₆ × 16^-64, about 5.4e-79, against a float32 whose smallest
		// subnormal is about 1.4e-45.
		r, err := NewReader(bytes.NewReader([]byte{0x00, 0x10, 0x00, 0x00}), enc)
		require.NoError(t, err)

		_, err = r.ReadFloat32()

		var rangeErr FloatRangeError
		require.ErrorAs(t, err, &rangeErr)
		require.Equal(t, "underflows a float32 to zero", rangeErr.Reason)
	})

	t.Run("a float64 takes both ends", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{
			0x7F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
			0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}), enc)
		require.NoError(t, err)

		// 16^63 and 16^-65, the two ends of HFP's range. The largest reads as
		// 2^252 exactly rather than one ulp below it, because HFP long's 56
		// bits of fraction are three more than a float64 holds.
		largest, err := r.ReadFloat64()
		require.NoError(t, err)
		require.Equal(t, math.Ldexp(1, 252), largest)

		smallest, err := r.ReadFloat64()
		require.NoError(t, err)
		require.Equal(t, math.Ldexp(1, -260), smallest)
	})
}

// TestReaderReadFloatDialectBoundary is the silent-failure test: the same four
// bytes decode to a different, entirely plausible number under the other
// format, with no error and nothing a caller could check. It is the table in
// codec/SPEC.md, "Why this is silent", and the whole argument for
// [Encoding.Float] having no default.
func TestReaderReadFloatDialectBoundary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      []byte
		asIEEE   float32
		asHFP    float32
		src64    []byte
		asIEEE64 float64
		asHFP64  float64
	}{
		{
			name:     "IEEE one read as HFP is a thirty-second",
			src:      []byte{0x3F, 0x80, 0x00, 0x00},
			asIEEE:   1,
			asHFP:    0.03125,
			src64:    []byte{0x3F, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			asIEEE64: 1,
			asHFP64:  0.05859375,
		},
		{
			name:     "HFP one read as IEEE is nine",
			src:      []byte{0x41, 0x10, 0x00, 0x00},
			asIEEE:   9,
			asHFP:    1,
			src64:    []byte{0x41, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			asIEEE64: 262144,
			asHFP64:  1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			read32 := func(f FloatFormat) float32 {
				r, err := NewReader(bytes.NewReader(tc.src), floatEncoding(f, binary.BigEndian))
				require.NoError(t, err)
				v, err := r.ReadFloat32()
				require.NoError(t, err)
				return v
			}
			read64 := func(f FloatFormat) float64 {
				r, err := NewReader(bytes.NewReader(tc.src64), floatEncoding(f, binary.BigEndian))
				require.NoError(t, err)
				v, err := r.ReadFloat64()
				require.NoError(t, err)
				return v
			}

			// Both readings succeed. Neither is a NaN, an infinity or an
			// out-of-range value; the wrong one is simply a wrong number.
			require.Equal(t, tc.asIEEE, read32(FloatIEEE))
			require.Equal(t, tc.asHFP, read32(FloatHFP))
			require.Equal(t, tc.asIEEE64, read64(FloatIEEE))
			require.Equal(t, tc.asHFP64, read64(FloatHFP))
		})
	}
}

func TestReaderReadFloatErrors(t *testing.T) {
	t.Parallel()

	t.Run("an unset float format is rejected at construction", func(t *testing.T) {
		t.Parallel()

		// There is no default, because reading HFP bytes as IEEE — or the
		// reverse — produces a plausible wrong number rather than an error, so
		// the axis has to be settled before a byte is read rather than at the
		// first COMP-1 field.
		enc := GnuCOBOLASCII()
		enc.Float = FloatUnset

		_, err := NewReader(bytes.NewReader(nil), enc)

		var encErr EncodingError
		require.ErrorAs(t, err, &encErr)
		require.Equal(t, "Float", encErr.Field)

		_, err = NewWriter(&bytes.Buffer{}, enc)
		require.ErrorAs(t, err, &encErr)
		require.Equal(t, "Float", encErr.Field)
	})

	t.Run("comp-1 field cut short", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x41, 0x10}), floatEncoding(FloatHFP, binary.BigEndian))
		require.NoError(t, err)

		_, err = r.ReadFloat32()
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)

		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Equal(t, int64(2), offErr.Offset)
	})

	t.Run("comp-2 field cut short", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x41, 0x10, 0x00, 0x00}), floatEncoding(FloatIEEE, binary.BigEndian))
		require.NoError(t, err)

		_, err = r.ReadFloat64()
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("end of stream reports io.EOF", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader(nil), floatEncoding(FloatIEEE, binary.BigEndian))
		require.NoError(t, err)

		_, err = r.ReadFloat32()
		require.ErrorIs(t, err, io.EOF)
		require.NotErrorIs(t, err, io.ErrUnexpectedEOF)
	})

	t.Run("IEEE carries NaN and infinity through unchanged", func(t *testing.T) {
		t.Parallel()

		// binary32 and binary64 have encodings for both, so nothing here is
		// out of range; only HFP has to reject them.
		src := []byte{
			0x7F, 0x80, 0x00, 0x00, // +Inf, binary32
			0xFF, 0xF0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // -Inf, binary64
			0x7F, 0xC0, 0x00, 0x00, // NaN, binary32
		}
		r, err := NewReader(bytes.NewReader(src), floatEncoding(FloatIEEE, binary.BigEndian))
		require.NoError(t, err)

		inf32, err := r.ReadFloat32()
		require.NoError(t, err)
		require.True(t, math.IsInf(float64(inf32), 1))

		inf64, err := r.ReadFloat64()
		require.NoError(t, err)
		require.True(t, math.IsInf(inf64, -1))

		nan32, err := r.ReadFloat32()
		require.NoError(t, err)
		require.True(t, math.IsNaN(float64(nan32)))
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
		// | 3F C0 00 00: RATE 1.5 as binary32 | 40 04 …: FACTOR 2.5 as binary64
		// | "1234u": BALANCE -12345, zone 3/7 overpunch | "042": COUNT 42
		data := []byte("A12345WIDGET GRIP   42\x00\x01\xFF\x12\x34\x5C\x00\x04\x2F\x04\xD2" +
			"\x3F\xC0\x00\x00" + "\x40\x04\x00\x00\x00\x00\x00\x00" + "1234u" + "042")
		require.Len(t, data, testRecordWidth)

		var got testRecord
		require.NoError(t, Unmarshal(GnuCOBOLASCII(), data, &got))
		require.Equal(t, testRecord{
			ID:      "A12345",
			Name:    "WIDGET GRIP",
			Code:    "42",
			Raw:     []byte{0x00, 0x01, 0xFF},
			Amount:  12345,
			Qty:     42,
			Seq:     1234,
			Rate:    1.5,
			Factor:  2.5,
			Balance: -12345,
			Count:   42,
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
