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
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// testRecord is a twelve-field record exercising every accessor family the
// package currently has: two left-justified alphanumeric fields, a JUSTIFIED
// RIGHT one, a raw byte field, a signed packed decimal field, an unsigned one,
// a COMP-6 one, a signed binary one, the two floating point widths, a signed
// zoned decimal field and an unsigned one. It stands in for the code a
// generator will emit, for
//
//	01 TEST-RECORD.
//	   05 ID      PIC X(6).
//	   05 NAME    PIC X(12).
//	   05 CODE    PIC X(4) JUSTIFIED RIGHT.
//	   05 RAW     PIC X(3).
//	   05 AMOUNT  PIC S9(5) COMP-3.
//	   05 QTY     PIC 9(4)  COMP-3.
//	   05 UNITS   PIC 9(4)  COMP-6.
//	   05 SEQ     PIC S9(4) COMP.
//	   05 RATE    COMP-1.
//	   05 FACTOR  COMP-2.
//	   05 BALANCE PIC S9(5) DISPLAY.
//	   05 COUNT   PIC 9(3)  DISPLAY.
//
// RATE and FACTOR carry no PICTURE, because COMP-1 and COMP-2 do not take one.
//
// QTY and UNITS carry the same PICTURE and are deliberately adjacent: QTY is
// three bytes and UNITS two, because COMP-6 stores no sign nibble. Reading
// either at the other's width shifts every field after it.
//
// That one record mixes DISPLAY, COMP-3, COMP-6, COMP, COMP-1 and COMP-2 fields
// is the point rather than an accident: USAGE is a property of each item, so a
// generator computes every field's width from its own usage and no file is in
// one numeric mode.
type testRecord struct {
	ID      string
	Name    string
	Code    string
	Raw     []byte
	Amount  int64
	Qty     int32
	Units   int32
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
	testRecordUnitsDigits   = 4
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
// itself. UNITS contributes two where QTY, of the same PICTURE, contributes
// three, because COMP-6 has no sign nibble. BALANCE contributes five and not
// six, because a TRAILING sign is overpunched rather than given a byte. RATE
// and FACTOR contribute a fixed four and eight, since neither has a digit count
// for a width to depend on.
var testRecordWidth = testRecordIDWidth + testRecordNameWidth + testRecordCodeWidth +
	testRecordRawWidth + packedWidth(testRecordAmountDigits) + packedWidth(testRecordQtyDigits) +
	comp6Width(testRecordUnitsDigits) +
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
	if err := w.WriteComp6Int32(r.Units, testRecordUnitsDigits); err != nil {
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
	if r.Units, err = rd.ReadComp6Int32(testRecordUnitsDigits); err != nil {
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

// charsetEncoding returns an encoding whose charset is cs. The charset is the
// only axis an alphanumeric field reads — neither the sign convention, the
// float format nor the byte order touches one — so the walks below vary that
// axis alone and leave the rest at a bundle's values.
func charsetEncoding(cs Charset) Encoding {
	enc := GnuCOBOLASCII()
	enc.Charset = cs
	return enc
}

// alphanumericCharset is one row of the table the alphanumeric walks run over.
type alphanumericCharset struct {
	name    string
	charset Charset
	// roundTrip says a PIC X field of arbitrary bytes survives
	// decode → encode → byte-equal under this charset, which needs it to be
	// bijective over the whole byte space. Where it is not, why says why, and
	// unrepresentable is the first character of the all-bytes corpus the
	// writer has no byte for.
	roundTrip       bool
	unrepresentable rune
	why             string
}

// alphanumericCharsets is every charset the alphanumeric accessors are walked
// over byte for byte: every charset the package ships, taken from
// shippedCharsets so that a code page added there is walked by construction,
// plus the caller-supplied oddballCharset, which is in the table so that an
// implementation hard-coded to a shipped code page fails the walk rather than
// passes it.
var alphanumericCharsets = func() []alphanumericCharset {
	table := make([]alphanumericCharset, 0, len(shippedCharsets)+1)
	for _, sc := range shippedCharsets {
		// Every charset this package ships is bijective over all 256 bytes —
		// TestCharsetIsTotalAndBijective is what holds that true — so a PIC X
		// field of arbitrary bytes round-trips under each of them.
		table = append(table, alphanumericCharset{name: sc.name, charset: sc.charset, roundTrip: true})
	}
	return append(table, alphanumericCharset{
		name:    "oddball",
		charset: oddballCharset{},
		// A caller's charset owes this package a total ToUnicode and nothing
		// more, and oddball's FromUnicode spells only the digits and the two
		// signs — so U+0000, the first character of the decoded corpus, has no
		// byte to be written back as. Nor is its decoding injective: 0x01 and
		// 0x2B both decode to '+', so even a total FromUnicode could not
		// reproduce the field. A PIC X field of arbitrary bytes is therefore
		// unrepresentable under such a charset, which is a property of the
		// charset and not a defect of the writer; [Reader.ReadBytes] is what
		// carries a binary payload under one.
		unrepresentable: 0x00,
		why:             "oddball spells only the digits, '+' and '-'",
	})
}()

// wantUTF8Lens is the length in bytes of the decoded 256-byte corpus under
// each charset, keyed by the charset's own name. The lengths are stated rather
// than derived, because they are the property these walks exist to pin: a
// translation emitting one byte per input byte — string(b), or a [256]byte
// table — reads 256 for every charset here, and every one of them maps some
// byte above U+007F, where a character costs two bytes once written as UTF-8.
//
// A charset added to alphanumericCharsets with no length here fails the walk
// rather than being quietly walked without the pin.
var wantUTF8Lens = map[string]int{
	"ASCII":   384,
	"cp037":   384,
	"oddball": 374,
}

// allByteValues is a PIC X field carrying every byte value in order, which is
// the corpus the alphanumeric walks below run over. Under every charset in
// alphanumericCharsets it begins and ends with a byte that does not decode to
// a space, so the padding strip is a no-op on it and cannot mask a difference
// — the walks assert that rather than assume it.
func allByteValues() []byte {
	src := make([]byte, 256)
	for i := range src {
		src[i] = byte(i)
	}
	return src
}

// paddedMultiByteField is a six-byte PIC X field whose value is the two
// characters cs spells for 0x80 and 0xFF, padded either side with two of the
// charset's space bytes. It is the fixture where the justification axis is
// load-bearing: the trim and the pad run on opposite ends, and the value's
// characters are not one byte each, so an implementation working over the
// source bytes rather than over the translated string takes the wrong amount
// off one end. It returns the field and the value it carries.
func paddedMultiByteField(cs Charset) (src []byte, content string) {
	return []byte{cs.Space(), cs.Space(), 0x80, 0xFF, cs.Space(), cs.Space()},
		string([]rune{cs.ToUnicode(0x80), cs.ToUnicode(0xFF)})
}

// requireMultiByteFixture states the preconditions paddedMultiByteField's
// value carries, rather than leaving a charset that violates one to fail with
// a diff of two unprintable strings and no reason.
//
// The padding is a byte and the trim runs on characters, so the two only meet
// where the charset's space byte decodes to U+0020; and the value is only a
// multi-byte case where its characters do not fit in one UTF-8 byte.
func requireMultiByteFixture(t *testing.T, cs Charset, content string) {
	t.Helper()

	require.Equalf(t, ' ', cs.ToUnicode(cs.Space()),
		"%s's space byte does not decode to U+0020, so a field padded with it has no padding to strip",
		cs.Name())
	for _, r := range content {
		require.NotEqualf(t, ' ', r, "a value character of %s decodes to a space, so the field is all padding", cs.Name())
		require.Greaterf(t, r, rune(0x7F), "a value character of %s is one byte, so the case is not multi-byte", cs.Name())
	}
}

// TestReaderReadAlphanumericTranslatesEveryByte reads a PIC X field carrying
// all 256 byte values and requires the result to be byte-identical to the
// charset's own translation of those bytes. Every other alphanumeric fixture
// in the package spells printable characters, so the 128 byte values where a
// translating implementation and a verbatim one disagree are otherwise
// untested end to end — and a PIC X field routinely carries a binary payload
// (codec/SPEC.md, "Alphanumeric and Alphabetic Items"), which makes the high
// half of the byte space data rather than a theoretical case.
func TestReaderReadAlphanumericTranslatesEveryByte(t *testing.T) {
	t.Parallel()

	for _, tc := range alphanumericCharsets {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := allByteValues()

			// The reference is the accessor's specification and reads nothing
			// of its implementation: translate each byte through the charset,
			// then encode the character it names as UTF-8.
			var sb strings.Builder
			for _, b := range src {
				sb.WriteRune(tc.charset.ToUnicode(b))
			}
			want := sb.String()

			wantLen, ok := wantUTF8Lens[tc.charset.Name()]
			require.Truef(t, ok, "no decoded length stated for %s; add one to wantUTF8Lens", tc.charset.Name())
			require.Equal(t, wantLen, len(want))
			require.Greaterf(t, len(want), len(src),
				"%s decodes this corpus one byte per byte, so it cannot catch a verbatim read",
				tc.charset.Name())

			// The strip takes off U+0020 rather than the charset's space
			// byte, since it runs on the translated string — so both are what
			// the corpus has to avoid at its ends for the equality below to be
			// exact under either justification.
			require.Equal(t, want, strings.Trim(want, " "),
				"corpus begins or ends with a space, so the padding strip would hide a difference")
			require.NotEqual(t, tc.charset.Space(), src[0])
			require.NotEqual(t, tc.charset.Space(), src[len(src)-1])

			for _, j := range []Justification{JustifyLeft, JustifyRight} {
				t.Run(j.String(), func(t *testing.T) {
					t.Parallel()

					r, err := NewReader(bytes.NewReader(src), charsetEncoding(tc.charset))
					require.NoError(t, err)

					got, err := r.ReadAlphanumericJustified(len(src), j)
					require.NoError(t, err)
					require.Equal(t, want, got)
					require.Equal(t, int64(len(src)), r.Offset())

					// Stated apart from the equality above because it is the
					// mutation this walk was written for: string(b) and a
					// [256]byte table both produce exactly this.
					require.NotEqual(t, string(src), got, "field was read verbatim rather than translated")
				})
			}

			t.Run("default justification", func(t *testing.T) {
				t.Parallel()

				r, err := NewReader(bytes.NewReader(src), charsetEncoding(tc.charset))
				require.NoError(t, err)

				got, err := r.ReadAlphanumeric(len(src))
				require.NoError(t, err)
				require.Equal(t, want, got)
			})
		})
	}
}

// TestReaderReadAlphanumericTrimsAroundMultiByteContent pins the padding strip
// against a value whose characters are not one byte each. The trim runs on the
// translated string rather than on the bytes read, so a strip written over the
// source bytes would take the wrong number of characters off the end that
// carries the padding.
func TestReaderReadAlphanumericTrimsAroundMultiByteContent(t *testing.T) {
	t.Parallel()

	for _, tc := range alphanumericCharsets {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cs := tc.charset
			src, content := paddedMultiByteField(cs)
			requireMultiByteFixture(t, cs, content)

			testCases := []struct {
				justify Justification
				want    string
			}{
				{justify: JustifyLeft, want: "  " + content},
				{justify: JustifyRight, want: content + "  "},
			}
			for _, jc := range testCases {
				t.Run(jc.justify.String(), func(t *testing.T) {
					t.Parallel()

					r, err := NewReader(bytes.NewReader(src), charsetEncoding(cs))
					require.NoError(t, err)

					got, err := r.ReadAlphanumericJustified(len(src), jc.justify)
					require.NoError(t, err)
					require.Equal(t, jc.want, got)
				})
			}
		})
	}
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

	t.Run("returns a slice the caller owns", func(t *testing.T) {
		t.Parallel()

		// The doc comment promises the returned slice is the caller's own and
		// is not a view into a buffer the Reader reuses. Two short reads at
		// disjoint offsets would not pin that — a slice into a shared read
		// window satisfies them — so the field is held across enough further
		// reading to force any plausible internal buffer to be recycled, and
		// its capacity is checked as well, since a window would hand back a
		// slice with room after it.
		const held, rest = 4, 64 * 1024
		src := append([]byte("ABCD"), bytes.Repeat([]byte("x"), rest)...)

		r, err := NewReader(bytes.NewReader(src), GnuCOBOLASCII())
		require.NoError(t, err)

		first, err := r.ReadBytes(held)
		require.NoError(t, err)
		require.Equal(t, []byte("ABCD"), first)
		require.Equal(t, held, cap(first), "the returned slice has room after it, so it is a window into a larger buffer")

		first[0] = 'Z'
		for range rest / held {
			_, err := r.ReadBytes(held)
			require.NoError(t, err)
		}

		require.Equal(t, []byte("ZBCD"), first, "a later read wrote into a slice an earlier one returned")
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

// TestReaderReadComp6 reads a COMP-6 field at both digit parities. The 9(4)
// vector is SPEC Appendix A.4's; the 9(3) one is the odd-count vector added
// beside it, which is where the pad nibble shows up.
func TestReaderReadComp6(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		src    []byte
		digits int
		want   int64
	}{
		{
			name:   "even digits, no pad nibble",
			src:    []byte{0x12, 0x34}, // PIC 9(4) COMP-6, 1234 — SPEC A.4
			digits: 4,
			want:   1234,
		},
		{
			name:   "odd digits, leading pad nibble",
			src:    []byte{0x01, 0x23}, // PIC 9(3) COMP-6, 123 — SPEC A.4
			digits: 3,
			want:   123,
		},
		{
			name:   "two digits fill one byte",
			src:    []byte{0x42}, // PIC 9(2) COMP-6, 42
			digits: 2,
			want:   42,
		},
		{
			name:   "single digit is one byte with a pad nibble",
			src:    []byte{0x07}, // PIC 9(1) COMP-6, 7
			digits: 1,
			want:   7,
		},
		{
			name:   "zero",
			src:    []byte{0x00, 0x00},
			digits: 4,
			want:   0,
		},
		{
			name:   "high-order zeros are not digits of the value",
			src:    []byte{0x00, 0x42},
			digits: 4,
			want:   42,
		},
		{
			name:   "nine digits, the int32 maximum",
			src:    []byte{0x09, 0x99, 0x99, 0x99, 0x99}, // PIC 9(9) COMP-6
			digits: 9,
			want:   999999999,
		},
		{
			name:   "eighteen digits, the int64 maximum",
			src:    []byte{0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99},
			digits: 18,
			want:   999999999999999999,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// COMP-6 is charset-invariant for the reason COMP-3 is: its
			// bytes are nibble pairs and were never characters.
			for _, enc := range []Encoding{IBMEnterprise(), GnuCOBOLASCII()} {
				r, err := NewReader(bytes.NewReader(tc.src), enc)
				require.NoError(t, err)

				got, err := r.ReadComp6Int64(tc.digits)
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
				require.Equal(t, int64(len(tc.src)), r.Offset())
				require.Equal(t, len(tc.src), comp6Width(tc.digits))

				if tc.digits <= maxPackedInt32Digits {
					r32, err := NewReader(bytes.NewReader(tc.src), enc)
					require.NoError(t, err)

					got32, err := r32.ReadComp6Int32(tc.digits)
					require.NoError(t, err)
					require.Equal(t, int32(tc.want), got32)
				}

				rb, err := NewReader(bytes.NewReader(tc.src), enc)
				require.NoError(t, err)

				gotBig, err := rb.ReadComp6Big(tc.digits)
				require.NoError(t, err)
				require.Equal(t, big.NewInt(tc.want), gotBig)
			}
		})
	}
}

func TestReaderReadComp6Big(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		src    []byte
		digits int
		want   string
	}{
		{
			name:   "nineteen digits, one past int64",
			src:    []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0x01, 0x23, 0x45, 0x67, 0x89},
			digits: 19,
			want:   "1234567890123456789",
		},
		{
			name: "thirty-one digits, the COBOL maximum",
			src: []byte{
				0x09, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
				0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99, 0x99,
			},
			digits: 31,
			want:   "9999999999999999999999999999999",
		},
		{
			name: "thirty digits, the widest even count",
			src: []byte{
				0x12, 0x34, 0x56, 0x78, 0x90, 0x12, 0x34,
				0x56, 0x78, 0x90, 0x12, 0x34, 0x56, 0x78, 0x90,
			},
			digits: 30,
			want:   "123456789012345678901234567890",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := NewReader(bytes.NewReader(tc.src), IBMEnterprise())
			require.NoError(t, err)

			got, err := r.ReadComp6Big(tc.digits)
			require.NoError(t, err)
			require.Equal(t, tc.want, got.String())
			require.Equal(t, int64(comp6Width(tc.digits)), r.Offset())
		})
	}
}

func TestReaderReadComp6Errors(t *testing.T) {
	t.Parallel()

	t.Run("digit nibble above nine", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte{0x1A, 0x34}), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadComp6Int64(4)

		var digitErr PackedDigitError
		require.ErrorAs(t, err, &digitErr)
		require.Equal(t, byte(0x0A), digitErr.Nibble)

		// The offset names the byte the bad nibble sits in, not the end of
		// the field.
		var offErr *OffsetError
		require.ErrorAs(t, err, &offErr)
		require.Zero(t, offErr.Offset)
	})

	t.Run("the low nibble of the last byte is a digit, not a sign", func(t *testing.T) {
		t.Parallel()

		signNibbles := []byte{0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F}
		for _, n := range signNibbles {
			// COMP-3 would read this as a sign. COMP-6 has no sign nibble,
			// so every one of them is a digit nibble out of range — which is
			// what turns a COMP-3 field read at a COMP-6 offset into a loud
			// failure rather than a plausible number.
			r, err := NewReader(bytes.NewReader([]byte{0x12, 0x30 | n}), GnuCOBOLASCII())
			require.NoError(t, err)

			_, err = r.ReadComp6Int64(4)

			var digitErr PackedDigitError
			require.ErrorAs(t, err, &digitErr)
			require.Equal(t, n, digitErr.Nibble)

			var offErr *OffsetError
			require.ErrorAs(t, err, &offErr)
			require.Equal(t, int64(1), offErr.Offset)
		}
	})

	t.Run("pad nibble is not zero", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name   string
			src    []byte
			digits int
			want   byte
		}{
			{
				// The cheapest available signal that the field offset is
				// wrong, validated here exactly as it is for COMP-3.
				name:   "pad nibble is not even a digit",
				src:    []byte{0xF1, 0x23},
				digits: 3,
				want:   0x0F,
			},
			{
				// The case a mis-offset field actually produces, and the
				// one that would otherwise decode as a plausible number:
				// 91 23 would read as 123 if the pad check were relaxed to
				// the digit check's `> 9`. The check is `!= 0` for exactly
				// this reason.
				name:   "pad nibble is a legal digit",
				src:    []byte{0x91, 0x23},
				digits: 3,
				want:   0x09,
			},
			{
				name:   "single digit with a digit-valued pad nibble",
				src:    []byte{0x51},
				digits: 1,
				want:   0x05,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				r, err := NewReader(bytes.NewReader(tc.src), GnuCOBOLASCII())
				require.NoError(t, err)

				_, err = r.ReadComp6Int64(tc.digits)

				var padErr PackedPadError
				require.ErrorAs(t, err, &padErr)
				require.Equal(t, tc.want, padErr.Nibble)

				var offErr *OffsetError
				require.ErrorAs(t, err, &offErr)
				require.Zero(t, offErr.Offset)
			})
		}
	})

	t.Run("an even digit count has no pad nibble to validate", func(t *testing.T) {
		t.Parallel()

		// The opposite parity from COMP-3: with no sign nibble making the
		// count up, an even digit count fills the field exactly.
		r, err := NewReader(bytes.NewReader([]byte{0x12, 0x34}), GnuCOBOLASCII())
		require.NoError(t, err)

		got, err := r.ReadComp6Int64(4)
		require.NoError(t, err)
		require.Equal(t, int64(1234), got)
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
				read:   func(r *Reader, d int) error { _, err := r.ReadComp6Int64(d); return err },
			},
			{
				name:   "negative digits",
				digits: -1,
				max:    maxPackedInt64Digits,
				read:   func(r *Reader, d int) error { _, err := r.ReadComp6Int64(d); return err },
			},
			{
				name:   "ten digits overflows an int32",
				digits: 10,
				max:    maxPackedInt32Digits,
				read:   func(r *Reader, d int) error { _, err := r.ReadComp6Int32(d); return err },
			},
			{
				name:   "nineteen digits overflows an int64",
				digits: 19,
				max:    maxPackedInt64Digits,
				read:   func(r *Reader, d int) error { _, err := r.ReadComp6Int64(d); return err },
			},
			{
				name:   "thirty-two digits exceeds COBOL itself",
				digits: 32,
				max:    maxPackedDigits,
				read:   func(r *Reader, d int) error { _, err := r.ReadComp6Big(d); return err },
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

		// Two bytes is a whole PIC 9(4) COMP-6 field and one byte short of a
		// PIC 9(5) one: the width the reader wanted is ceil(digits/2), and
		// the boundary is where the error appears.
		r, err := NewReader(bytes.NewReader([]byte{0x12, 0x34}), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadComp6Int64(5)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
		require.Equal(t, int64(2), r.Offset())

		r4, err := NewReader(bytes.NewReader([]byte{0x12, 0x34}), GnuCOBOLASCII())
		require.NoError(t, err)

		got, err := r4.ReadComp6Int64(4)
		require.NoError(t, err)
		require.Equal(t, int64(1234), got)
	})

	t.Run("end of stream reports io.EOF", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader(nil), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.ReadComp6Big(5)
		require.ErrorIs(t, err, io.EOF)
	})
}

// packedFault names which of the three nibble roles a packed field's reported
// fault belongs to. A multi-fault test row states the role it expects rather
// than asserting it in a closure, so the table reads as the precedence rule it
// pins: pad, then digits most significant first, then sign.
type packedFault int

const (
	faultPad packedFault = iota
	faultDigit
	faultSign
)

// requirePackedFault asserts that err is the packed nibble error for the given
// role, carrying the given nibble, stamped with the offset of the byte holding
// that nibble. Asserting the role is what rejects a reordered scan: a reader
// that checked digits before the pad returns a PackedDigitError here, and
// [require.ErrorAs] for PackedPadError fails on it.
func requirePackedFault(t *testing.T, err error, want packedFault, nibble byte, offset int64) {
	t.Helper()

	switch want {
	case faultPad:
		var padErr PackedPadError
		require.ErrorAs(t, err, &padErr)
		require.Equal(t, nibble, padErr.Nibble)
	case faultDigit:
		var digitErr PackedDigitError
		require.ErrorAs(t, err, &digitErr)
		require.Equal(t, nibble, digitErr.Nibble)
	case faultSign:
		var signErr PackedSignError
		require.ErrorAs(t, err, &signErr)
		require.Equal(t, nibble, signErr.Nibble)
	default:
		t.Fatalf("unknown packed fault %d", want)
	}

	// The offset names the byte holding the offending nibble, not the byte the
	// field ended at, which is what makes nibbleAt's arithmetic load bearing
	// rather than decorative.
	var offErr *OffsetError
	require.ErrorAs(t, err, &offErr)
	require.Equal(t, offset, offErr.Offset)
}

// runFaultPrecedenceCase reads src with each of the given accessors and
// requires the reported fault to be the expected one.
//
// It runs every case twice: once with the field at the start of the record, and
// once behind a short prefix consumed by an earlier read. nibbleAt is
// fieldStart+i/2, and a field beginning at offset 0 exercises only the second
// term — an implementation that dropped the field's start offset altogether
// would pass a table that never moved the field. The prefix is an odd number of
// bytes so that a start offset folded into the nibble index rather than added
// to the byte index shows up as well.
func runFaultPrecedenceCase(
	t *testing.T,
	readers map[string]func(*Reader, int) error,
	src []byte,
	digits int,
	want packedFault,
	wantNibble byte,
	wantOffset int64,
) {
	t.Helper()

	starts := []struct {
		name string
		at   int64
	}{
		{name: "at the start of the record", at: 0},
		{name: "after a three-byte field", at: 3},
	}

	for _, start := range starts {
		t.Run(start.name, func(t *testing.T) {
			t.Parallel()

			for name, read := range readers {
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					record := append(bytes.Repeat([]byte{0xFF}, int(start.at)), src...)
					r, err := NewReader(bytes.NewReader(record), GnuCOBOLASCII())
					require.NoError(t, err)

					if start.at > 0 {
						_, err = r.ReadBytes(int(start.at))
						require.NoError(t, err)
						require.Equal(t, start.at, r.Offset())
					}

					err = read(r, digits)
					requirePackedFault(t, err, want, wantNibble, start.at+wantOffset)
				})
			}
		})
	}
}

// TestPackedFaultPrecedence pins which fault a COMP-3 field carrying more than
// one of them reports, and which byte the offset names.
//
// This is the common case rather than a corner of it: of the 16,777,216
// three-byte values a PIC S9(4) COMP-3 field can hold, 15,384,000 — 91.7% — are
// invalid in more than one of the three roles at once. SPEC.md's "Fault
// precedence" makes the answer normative, and it is field order: the pad
// nibble, then the digit nibbles from most significant to least, then the sign.
// Every row below is chosen so that at least one of the three reorderings the
// rule forbids — digits before the pad, the sign before the digits, the last
// bad digit instead of the first — returns something this table rejects.
func TestPackedFaultPrecedence(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		src        []byte
		digits     int
		want       packedFault
		wantNibble byte
		wantOffset int64
	}{
		{
			// Pad A at nibble 0, digit B at nibble 3. Checking the digits
			// first would report B at offset 1.
			name:       "bad pad beats a bad digit",
			src:        []byte{0xA1, 0x2B, 0x4C},
			digits:     4,
			want:       faultPad,
			wantNibble: 0x0A,
			wantOffset: 0,
		},
		{
			// Pad F, leading digit A, sign 3: every role is wrong at once,
			// which is the shape a field read at a slipped offset usually has.
			name:       "bad pad beats everything else in the field",
			src:        []byte{0xFA, 0x23, 0x43},
			digits:     4,
			want:       faultPad,
			wantNibble: 0x0F,
			wantOffset: 0,
		},
		{
			// Digit A at nibble 2, sign 7. Checking the sign first would
			// report a PackedSignError at offset 2.
			name:       "bad digit beats a bad sign",
			src:        []byte{0x01, 0xA3, 0x47},
			digits:     4,
			want:       faultDigit,
			wantNibble: 0x0A,
			wantOffset: 1,
		},
		{
			// Digits A and B, in the first two bytes. Reporting the last bad
			// digit would give B at offset 1.
			name:       "the first bad digit beats the second",
			src:        []byte{0x0A, 0xB3, 0x4C},
			digits:     4,
			want:       faultDigit,
			wantNibble: 0x0A,
			wantOffset: 0,
		},
		{
			// Both bad digits are past the first byte, so the offset is
			// nibbleAt's arithmetic rather than the field's start offset.
			name:       "the first bad digit beats the second away from the field start",
			src:        []byte{0x01, 0x2C, 0xDC},
			digits:     4,
			want:       faultDigit,
			wantNibble: 0x0C,
			wantOffset: 1,
		},
		{
			// The bad digit and the bad sign share the final byte, so the
			// offsets coincide and only the error type separates the two
			// orderings.
			name:       "a bad digit in the final byte beats the sign beside it",
			src:        []byte{0x01, 0x23, 0xE5},
			digits:     4,
			want:       faultDigit,
			wantNibble: 0x0E,
			wantOffset: 2,
		},
		{
			// Four bytes, so the reported byte is neither the first nor the
			// last: faults at nibbles 5, 6 and 7 (bytes 2, 3 and 3).
			name:       "the first bad digit beats the rest in a four-byte field",
			src:        []byte{0x01, 0x23, 0x4B, 0xC2},
			digits:     6,
			want:       faultDigit,
			wantNibble: 0x0B,
			wantOffset: 2,
		},
		{
			// An odd digit count has no pad nibble, so the high nibble of the
			// first byte is the most significant digit. A reader that checked
			// nibble 0 as a pad regardless of parity would report a
			// PackedPadError here.
			name:       "at an odd digit count the leading nibble is a digit, and beats the sign",
			src:        []byte{0xA2, 0x34, 0x51},
			digits:     5,
			want:       faultDigit,
			wantNibble: 0x0A,
			wantOffset: 0,
		},
		{
			// The boundary that gives the two rows above their meaning: with
			// nothing earlier wrong, the sign check does fire, and it fires
			// at the last byte.
			name:       "the sign is reported when nothing before it is wrong",
			src:        []byte{0x01, 0x23, 0x45},
			digits:     4,
			want:       faultSign,
			wantNibble: 0x05,
			wantOffset: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Every accessor shares readPackedField, so the precedence must
			// not depend on which one the generated code calls.
			runFaultPrecedenceCase(t, map[string]func(*Reader, int) error{
				"ReadPackedInt32": func(r *Reader, d int) error { _, err := r.ReadPackedInt32(d); return err },
				"ReadPackedInt64": func(r *Reader, d int) error { _, err := r.ReadPackedInt64(d); return err },
				"ReadPackedBig":   func(r *Reader, d int) error { _, err := r.ReadPackedBig(d); return err },
			}, tc.src, tc.digits, tc.want, tc.wantNibble, tc.wantOffset)
		})
	}
}

// TestComp6FaultPrecedence is TestPackedFaultPrecedence's other half. COMP-6
// puts its pad nibble on the opposite parity — odd digit counts, not even — and
// has no sign role at all, so the same rule has to be pinned against a
// different layout rather than assumed to carry over from the COMP-3 body: the
// two are separate functions, and until this test only one of them had its scan
// order held in place.
func TestComp6FaultPrecedence(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		src        []byte
		digits     int
		want       packedFault
		wantNibble byte
		wantOffset int64
	}{
		{
			// Pad 9 — a legal digit value, which is why the pad check is
			// != 0 rather than > 9 — beside digit A at nibble 2.
			name:       "bad pad beats a bad digit",
			src:        []byte{0x91, 0xA3},
			digits:     3,
			want:       faultPad,
			wantNibble: 0x09,
			wantOffset: 0,
		},
		{
			// COMP-6 rejects A-F in every nibble, the pad included, so the
			// pad role gets a nibble out of the digit range as well as the
			// digit-valued one above. A pad check spelled > 9 would pass the
			// row above and this one, and fail the row below.
			name:       "a pad nibble in A-F beats a bad digit",
			src:        []byte{0xC1, 0xA3},
			digits:     3,
			want:       faultPad,
			wantNibble: 0x0C,
			wantOffset: 0,
		},
		{
			// The COMP-3-read-at-a-COMP-6-offset shape: a sign nibble sits in
			// the last digit position, and the pad is wrong as well. The pad
			// is the earlier fault and wins.
			name:       "bad pad beats a COMP-3 sign nibble in the last position",
			src:        []byte{0x11, 0x2C},
			digits:     3,
			want:       faultPad,
			wantNibble: 0x01,
			wantOffset: 0,
		},
		{
			// Digits A and B in different bytes. Reporting the last bad digit
			// would give B at offset 1.
			name:       "the first bad digit beats the second",
			src:        []byte{0x0A, 0xB3},
			digits:     3,
			want:       faultDigit,
			wantNibble: 0x0A,
			wantOffset: 0,
		},
		{
			// An even digit count has no pad nibble here, the opposite parity
			// from COMP-3, so nibble 0 is the most significant digit. A reader
			// that checked it as a pad would report a PackedPadError.
			name:       "at an even digit count the leading nibble is a digit",
			src:        []byte{0xC2, 0x3D},
			digits:     4,
			want:       faultDigit,
			wantNibble: 0x0C,
			wantOffset: 0,
		},
		{
			// Pad valid, so the offset comes from nibbleAt rather than from
			// the field's start: faults at nibbles 2 and 5, bytes 1 and 2.
			name:       "the first bad digit beats the second away from the field start",
			src:        []byte{0x01, 0xA3, 0x4B},
			digits:     5,
			want:       faultDigit,
			wantNibble: 0x0A,
			wantOffset: 1,
		},
		{
			// Three bytes with no pad nibble at all, faults at nibbles 3 and
			// 4 — adjacent nibbles that straddle a byte boundary, so the two
			// orderings differ in offset as well as in nibble.
			name:       "the first bad digit beats the second across a byte boundary",
			src:        []byte{0x12, 0x3E, 0xF6},
			digits:     6,
			want:       faultDigit,
			wantNibble: 0x0E,
			wantOffset: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			runFaultPrecedenceCase(t, map[string]func(*Reader, int) error{
				"ReadComp6Int32": func(r *Reader, d int) error { _, err := r.ReadComp6Int32(d); return err },
				"ReadComp6Int64": func(r *Reader, d int) error { _, err := r.ReadComp6Int64(d); return err },
				"ReadComp6Big":   func(r *Reader, d int) error { _, err := r.ReadComp6Big(d); return err },
			}, tc.src, tc.digits, tc.want, tc.wantNibble, tc.wantOffset)
		})
	}
}

// TestPackedReadsNoFurtherThanItsOwnBytes pins the bound the folded nibble
// walk depends on: a packed or COMP-6 read looks at the bytes of its own field
// and at nothing else.
//
// It runs the narrowest field of each usage — one digit, one byte for both —
// because that is where the margin is largest: the reused scratch is
// maxNumericWidth bytes and [Reader.read] slices one of them out of it, so
// anything reading a machine word instead of a nibble would take fifteen bytes
// that belong to the field before. Those bytes are poison here, the nines and
// A-F nibbles of a wide field read immediately beforehand, so a read that
// strayed into them returns a wrong number or a spurious PackedDigitError
// rather than passing by luck on a zeroed buffer.
//
// Both sources are covered. They differ in where the field's bytes come from —
// [io.ReadFull] against a copy out of the caller's slice — and only the
// byte-backed one has a real neighbour byte after the field, which is what a
// wide load would reach on a file rather than on a fixture.
func TestPackedReadsNoFurtherThanItsOwnBytes(t *testing.T) {
	t.Parallel()

	// A wide field of every nibble the validators reject, read first so that
	// it is what the scratch buffer holds when the narrow field lands in it.
	poison := bytes.Repeat([]byte{0x9A, 0xBC, 0xDE, 0xF9}, maxNumericWidth/4)
	require.Len(t, poison, maxNumericWidth)

	usages := []struct {
		name string
		// field is the narrowest field of this usage: one digit, holding 7.
		field []byte
		read  map[string]func(r *Reader) (string, error)
	}{
		{
			name:  "comp-3",
			field: []byte{0x7C},
			read: map[string]func(r *Reader) (string, error){
				"ReadPackedInt32": func(r *Reader) (string, error) {
					v, err := r.ReadPackedInt32(1)
					return strconv.FormatInt(int64(v), 10), err
				},
				"ReadPackedInt64": func(r *Reader) (string, error) {
					v, err := r.ReadPackedInt64(1)
					return strconv.FormatInt(v, 10), err
				},
				"ReadPackedBig": func(r *Reader) (string, error) {
					v, err := r.ReadPackedBig(1)
					if err != nil {
						return "", err
					}
					return v.String(), nil
				},
			},
		},
		{
			name:  "comp-6",
			field: []byte{0x07},
			read: map[string]func(r *Reader) (string, error){
				"ReadComp6Int32": func(r *Reader) (string, error) {
					v, err := r.ReadComp6Int32(1)
					return strconv.FormatInt(int64(v), 10), err
				},
				"ReadComp6Int64": func(r *Reader) (string, error) {
					v, err := r.ReadComp6Int64(1)
					return strconv.FormatInt(v, 10), err
				},
				"ReadComp6Big": func(r *Reader) (string, error) {
					v, err := r.ReadComp6Big(1)
					if err != nil {
						return "", err
					}
					return v.String(), nil
				},
			},
		},
	}

	sources := []struct {
		name string
		open func(t *testing.T, data []byte) *Reader
	}{
		{
			name: "stream",
			open: func(t *testing.T, data []byte) *Reader {
				t.Helper()

				r, err := NewReader(bytes.NewReader(data), IBMEnterprise())
				require.NoError(t, err)
				return r
			},
		},
		{
			name: "bytes",
			open: func(t *testing.T, data []byte) *Reader {
				t.Helper()

				r, err := NewBytesReader(data, IBMEnterprise())
				require.NoError(t, err)
				return r
			},
		},
	}

	// The bytes after the field, which no read of it may consume or look at.
	trailer := bytes.Repeat([]byte{0xFF}, 16)

	for _, u := range usages {
		t.Run(u.name, func(t *testing.T) {
			t.Parallel()

			for _, src := range sources {
				t.Run(src.name, func(t *testing.T) {
					t.Parallel()

					for name, read := range u.read {
						t.Run(name, func(t *testing.T) {
							t.Parallel()

							data := slices.Concat(poison, u.field, trailer)
							r := src.open(t, data)

							// Fill the scratch with the poison, as a
							// preceding field of a record would.
							// ReadAlphanumeric and not ReadBytes:
							// ReadBytes takes the allocating path and
							// would leave the scratch untouched.
							_, err := r.ReadAlphanumeric(len(poison))
							require.NoError(t, err)

							got, err := read(r)
							require.NoError(t, err)
							require.Equal(t, "7", got, "the value came from outside the field")
							require.Equal(t, int64(len(poison)+len(u.field)), r.Offset(),
								"the read consumed more than the field's own bytes")

							rest, err := r.ReadBytes(len(trailer))
							require.NoError(t, err)
							require.Equal(t, trailer, rest, "the bytes after the field were disturbed")
						})
					}
				})
			}
		})
	}
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
		// | 00 07: UNITS 7, PIC 9(4) COMP-6 — two bytes where QTY's identical
		// PICTURE takes three, since COMP-6 has no sign nibble
		// | 04 D2: SEQ +1234, big-endian as GnuCOBOL writes it by default
		// | 3F C0 00 00: RATE 1.5 as binary32 | 40 04 …: FACTOR 2.5 as binary64
		// | "1234u": BALANCE -12345, zone 3/7 overpunch | "042": COUNT 42
		data := []byte("A12345WIDGET GRIP   42\x00\x01\xFF\x12\x34\x5C\x00\x04\x2F\x00\x07\x04\xD2" +
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
			Units:   7,
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

// TestNumericScratchFitsEveryNumericUsage pins [maxNumericWidth] against the
// width functions themselves, at each family's own digit-count maximum.
//
// The const is a const because an array size has to be one, so it restates the
// width formulas as constant arithmetic instead of calling them. That
// restatement is the thing worth checking: it is derived from the maxima, so
// raising one moves it, but nothing in the language makes it follow a change to
// a *formula*. A family whose width stops being the term the const carries —
// a fifth numeric usage, a COMP-6 whose parity changes, a binary staircase with
// a wider top step — fails here rather than at the first field that overruns.
func TestNumericScratchFitsEveryNumericUsage(t *testing.T) {
	t.Parallel()

	widest := []struct {
		name  string
		width int
	}{
		{name: "zoned unsigned", width: zonedWidth(maxZonedDigits, SignUnsigned)},
		{name: "zoned trailing", width: zonedWidth(maxZonedDigits, SignTrailing)},
		{name: "zoned leading", width: zonedWidth(maxZonedDigits, SignLeading)},
		{name: "zoned trailing separate", width: zonedWidth(maxZonedDigits, SignTrailingSeparate)},
		{name: "zoned leading separate", width: zonedWidth(maxZonedDigits, SignLeadingSeparate)},
		{name: "packed", width: packedWidth(maxPackedDigits)},
		{name: "comp-6", width: comp6Width(maxPackedDigits)},
		{name: "binary", width: binaryWidth(maxBinaryDigits)},
		{name: "comp-1", width: comp1Width},
		{name: "comp-2", width: comp2Width},
	}

	got := 0
	for _, w := range widest {
		require.LessOrEqualf(t, w.width, maxNumericWidth,
			"the widest legal %s field does not fit the numeric scratch", w.name)
		got = max(got, w.width)
	}

	// Equality and not just "fits": a scratch wider than the widest field is
	// dead bytes on every Reader, and one derived from a term that has stopped
	// being anybody's maximum would still pass the loop above.
	require.Equal(t, maxNumericWidth, got,
		"maxNumericWidth is not the widest numeric field the package reads")
}

// TestReadFallsBackForFieldsWiderThanTheScratch covers the two properties the
// growable buffer carries: a field wider than [maxNumericWidth] is served
// rather than panicking or being truncated, and the buffer it is served from is
// reused at that width afterwards.
//
// It reaches [Reader.read] directly because no accessor can currently ask for
// an over-wide *numeric* field — every numeric accessor rejects the digit count
// first — and that is exactly the case the fallback exists for: 31 digits is a
// dialect ceiling rather than a fact about COBOL, so the fallback has to be
// there before the maximum moves, not after.
func TestReadFallsBackForFieldsWiderThanTheScratch(t *testing.T) {
	t.Parallel()

	widths := []int{maxNumericWidth + 1, 4 * maxNumericWidth}

	for _, n := range widths {
		t.Run("n="+strconv.Itoa(n), func(t *testing.T) {
			t.Parallel()

			want := make([]byte, n)
			for i := range want {
				want[i] = byte(i)
			}

			r, err := NewReader(bytes.NewReader(want), GnuCOBOLASCII())
			require.NoError(t, err)

			got, err := r.read(n)
			require.NoError(t, err)
			require.Equal(t, want, got)
			require.Equal(t, int64(n), r.Offset())
		})
	}

	t.Run("grown buffer is reused at that width", func(t *testing.T) {
		t.Parallel()

		const n = maxNumericWidth + 1

		r, err := NewReader(bytes.NewReader(make([]byte, 3*n)), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.read(n)
		require.NoError(t, err)
		grown := cap(r.wide)
		require.GreaterOrEqual(t, grown, n)

		for i := 0; i < 2; i++ {
			_, err = r.read(n)
			require.NoError(t, err)
			require.Equal(t, grown, cap(r.wide),
				"the growable read buffer was reallocated at a width it already held")
		}
	})

	t.Run("a negative width is still a FieldWidthError", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader(nil), GnuCOBOLASCII())
		require.NoError(t, err)

		_, err = r.read(-1)
		var widthErr FieldWidthError
		require.ErrorAs(t, err, &widthErr)
		require.Equal(t, -1, widthErr.Width)
		require.Zero(t, r.Offset())
	})
}

// TestReadValuesDoNotAliasTheReusedBuffer is the property that makes reusing a
// read buffer safe at all: nothing an accessor returns may be a view into it.
//
// It reads two records with one [Reader] and asserts every value of the first
// is unchanged after the second has been read over the same bytes. The two
// records differ in every field, so an aliased return would show up as the
// second record's value appearing in the first record's variable rather than as
// a subtle corruption.
func TestReadValuesDoNotAliasTheReusedBuffer(t *testing.T) {
	t.Parallel()

	enc := GnuCOBOLASCII()

	first := testRecord{
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
	second := testRecord{
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
	}

	var data []byte
	for _, rec := range []testRecord{first, second} {
		b, err := Marshal(enc, &rec)
		require.NoError(t, err)
		require.Len(t, b, testRecordWidth)
		data = append(data, b...)
	}

	r, err := NewReader(bytes.NewReader(data), enc)
	require.NoError(t, err)

	var gotFirst, gotSecond testRecord
	require.NoError(t, gotFirst.UnmarshalCOBOL(r))
	require.Equal(t, first, gotFirst)

	require.NoError(t, gotSecond.UnmarshalCOBOL(r))
	require.Equal(t, second, gotSecond)

	// The whole point: the first record's values still read as they did before
	// the second record was read over the same buffer.
	require.Equal(t, first, gotFirst)
}

// TestReadBytesReturnsACallerOwnedSlice pins the exception [Reader.ReadBytes]
// is, and that its doc comment promises: it allocates, so its result neither
// aliases the buffer the other accessors reuse nor the result of an earlier
// call, and a caller may write into it.
func TestReadBytesReturnsACallerOwnedSlice(t *testing.T) {
	t.Parallel()

	// A width inside the numeric scratch, which is where an accidental
	// alias would come from.
	const n = 4
	require.LessOrEqual(t, n, maxNumericWidth)

	r, err := NewReader(bytes.NewReader([]byte("ABCDEFGHIJKL")), GnuCOBOLASCII())
	require.NoError(t, err)

	first, err := r.ReadBytes(n)
	require.NoError(t, err)
	require.Equal(t, []byte("ABCD"), first)

	second, err := r.ReadBytes(n)
	require.NoError(t, err)
	require.NotSame(t, &first[0], &second[0], "two ReadBytes results share a backing array")
	require.Equal(t, []byte("ABCD"), first)

	// Writing into a returned slice is the caller's right and must not reach
	// anything the Reader later reads.
	second[0] = 0xFF
	third, err := r.ReadAlphanumeric(n)
	require.NoError(t, err)
	require.Equal(t, "IJKL", third)
	require.Equal(t, []byte("ABCD"), first)
	require.Equal(t, []byte{0xFF, 'F', 'G', 'H'}, second)
}

// TestReaderReadsNoFurtherThanTheFieldsAsked pins the absence of read-ahead. A
// reusable buffer is exactly the change that could quietly introduce some — a
// buffer sized for the widest numeric field could be filled to its width rather
// than to the field's — and a Reader that consumed more than it was asked for
// would leave the caller unable to read the rest of the stream any other way.
func TestReaderReadsNoFurtherThanTheFieldsAsked(t *testing.T) {
	t.Parallel()

	// A wide alphanumeric field, so the growable buffer is on the path too,
	// followed by narrow fields served from the fixed array.
	const wide = 3 * maxNumericWidth
	data := bytes.Repeat([]byte("X"), wide)
	data = append(data, "ACME  "...)
	data = append(data, "TRAILING BYTES"...)

	src := bytes.NewReader(data)
	r, err := NewReader(src, GnuCOBOLASCII())
	require.NoError(t, err)

	// ReadAlphanumeric and not ReadBytes: ReadBytes takes the allocating path
	// and would never touch the growable buffer, which is the one whose
	// capacity outlives the field and so the one read-ahead could creep into.
	_, err = r.ReadAlphanumeric(wide)
	require.NoError(t, err)
	require.Equal(t, len(data)-wide, src.Len())

	_, err = r.ReadAlphanumeric(6)
	require.NoError(t, err)
	require.Equal(t, len(data)-wide-6, src.Len())
	require.Equal(t, int64(wide+6), r.Offset())

	// What is left is readable and is exactly what was never asked for.
	rest, err := io.ReadAll(src)
	require.NoError(t, err)
	require.Equal(t, []byte("TRAILING BYTES"), rest)
}

// TestReadResultIsVolatile pins the premise every accessor on the reusing path
// depends on: what [Reader.read] returns is a window into the Reader and is
// overwritten by the next read.
//
// It is the other half of TestReadValuesDoNotAliasTheReusedBuffer, and it is
// here because that test cannot fail on its own — every value testRecord holds
// is a string or a number, and Go's own string conversion would copy the bytes
// even if an accessor tried to hand the scratch out. Stating the volatility
// directly is what makes "no returned value aliases it" a claim about the
// accessors rather than a claim about string conversion.
func TestReadResultIsVolatile(t *testing.T) {
	t.Parallel()

	widths := []struct {
		name string
		n    int
	}{
		{name: "fixed array", n: 4},
		{name: "growable buffer", n: maxNumericWidth + 1},
	}

	for _, w := range widths {
		t.Run(w.name, func(t *testing.T) {
			t.Parallel()

			first := bytes.Repeat([]byte{0xAA}, w.n)
			second := bytes.Repeat([]byte{0x55}, w.n)

			r, err := NewReader(bytes.NewReader(append(slices.Clone(first), second...)), GnuCOBOLASCII())
			require.NoError(t, err)

			held, err := r.read(w.n)
			require.NoError(t, err)
			require.Equal(t, first, held)

			_, err = r.read(w.n)
			require.NoError(t, err)
			require.Equal(t, second, held,
				"read returned a buffer the next read did not reuse; the accessors' no-alias property is then untested rather than true")
		})
	}

	t.Run("the returned slice cannot be appended into the next field", func(t *testing.T) {
		t.Parallel()

		const n = 4

		r, err := NewReader(bytes.NewReader(bytes.Repeat([]byte{0x01}, 4*n)), GnuCOBOLASCII())
		require.NoError(t, err)

		held, err := r.read(n)
		require.NoError(t, err)
		require.Equal(t, n, cap(held),
			"read returned spare capacity, so an append would write over the bytes of the next field")
	})
}

// TestReadBytesIsTheOnlyAccessorHandingOutBytes guards the boundary the whole
// buffer-reuse design rests on: exactly one exported accessor returns a
// []byte, and it is the one that allocates.
//
// TestReadValuesDoNotAliasTheReusedBuffer checks the accessors that exist. This
// checks the ones that do not yet: a second []byte-returning accessor added on
// the reusing path would hand a caller a window into the Reader that is
// overwritten by the next field, and every existing test would still pass. It
// fails here instead, at the moment the method is added.
func TestReadBytesIsTheOnlyAccessorHandingOutBytes(t *testing.T) {
	t.Parallel()

	byteSlice := reflect.TypeOf([]byte(nil))

	var got []string
	rt := reflect.TypeOf(&Reader{})
	for i := 0; i < rt.NumMethod(); i++ {
		m := rt.Method(i)
		for j := 0; j < m.Type.NumOut(); j++ {
			if m.Type.Out(j) == byteSlice {
				got = append(got, m.Name)
				break
			}
		}
	}

	require.Equal(t, []string{"ReadBytes"}, got,
		"an accessor other than ReadBytes returns a []byte; it must read through readOwned, not read")
}

// TestReadingDoesNotAllocate is the enforceable form of the change this test
// file was extended for: the read buffer no longer escapes, so an accessor that
// decodes into a value allocates nothing at all.
//
// The two packed cases were the second half of that change: the integer packed
// and COMP-6 accessors used to unpack every nibble of the field into a slice
// the fold then walked once and dropped, and it escaped because the function
// building it returned it. They fold the field's own nibbles now, so they
// belong beside the accessors that never had an intermediate at all.
//
// It asserts zero rather than pinning a count. A count moves with the toolchain
// and wants an owner — codec/CLAUDE.md says so of the benchmarks, and that
// still holds — but zero is zero on every toolchain, and it is the one number
// that says the buffer did not escape. Every accessor here returns a value with
// no backing array of its own, so any allocation at all is the field's bytes
// reaching the heap.
//
// It is one of the package's two tests without t.Parallel(), at either level,
// TestResetDoesNotAllocate being the other. [testing.AllocsPerRun] sets
// GOMAXPROCS to 1 for the duration of its run and panics outright when called
// from a parallel test, so the choice is that exception or no allocation
// assertion at all.
func TestReadingDoesNotAllocate(t *testing.T) {
	enc := GnuCOBOLASCII()

	testCases := []struct {
		name  string
		field []byte
		read  func(r *Reader) error
	}{
		{
			name:  "binary",
			field: []byte{0x04, 0xD2},
			read: func(r *Reader) error {
				_, err := r.ReadBinaryInt16(testRecordSeqDigits)
				return err
			},
		},
		{
			// Five digits, so the field carries a pad nibble, three
			// bytes and a sign — every nibble role the fold validates.
			name:  "comp-3",
			field: []byte{0x12, 0x34, 0x5C},
			read: func(r *Reader) error {
				_, err := r.ReadPackedInt64(5)
				return err
			},
		},
		{
			// Four digits, the even count at which COMP-6 has no pad
			// nibble and is a byte narrower than the COMP-3 above.
			name:  "comp-6",
			field: []byte{0x12, 0x34},
			read: func(r *Reader) error {
				_, err := r.ReadComp6Int64(4)
				return err
			},
		},
		{
			name:  "comp-1",
			field: []byte{0x3F, 0xC0, 0x00, 0x00},
			read: func(r *Reader) error {
				_, err := r.ReadFloat32()
				return err
			},
		},
		{
			name:  "comp-2",
			field: []byte{0x40, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			read: func(r *Reader) error {
				_, err := r.ReadFloat64()
				return err
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r, err := NewReader(&repeatReader{buf: bytes.Repeat(tc.field, 4096)}, enc)
			require.NoError(t, err)

			require.NoError(t, tc.read(r))

			allocs := testing.AllocsPerRun(100, func() {
				if err := tc.read(r); err != nil {
					t.Error(err)
				}
			})
			require.Zero(t, allocs, "the field's bytes reached the heap")
		})
	}
}

// TestReadAlphanumericMatchesWriteRuneUnderEveryCharset is the accessor-level
// half of TestAlphaTableAppendFieldMatchesWriteRune: whatever the derived
// table is worth, what [Reader.ReadAlphanumericJustified] returns has to be
// what the [Charset.ToUnicode] plus [strings.Builder.WriteRune] loop it
// replaced returned, for every byte value and under both justifications.
//
// It runs over alphaTableCharsets rather than alphanumericCharsets, so the
// three charsets whose runes a code page would never spell — three UTF-8
// bytes, four, and not a character at all — are on the accessor's path and not
// only on the table's.
func TestReadAlphanumericMatchesWriteRuneUnderEveryCharset(t *testing.T) {
	t.Parallel()

	for _, tc := range alphaTableCharsets {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := allByteValues()

			var sb strings.Builder
			for _, b := range src {
				sb.WriteRune(tc.charset.ToUnicode(b))
			}
			translated := sb.String()
			require.Greater(t, len(translated), len(src),
				"this charset decodes the corpus one byte per byte, so a verbatim read would pass")

			for _, j := range []Justification{JustifyLeft, JustifyRight} {
				t.Run(j.String(), func(t *testing.T) {
					t.Parallel()

					// The reference trims the *translated* string, which is
					// what the accessor documents: the space stripped is
					// U+0020 and not the charset's space byte.
					want := strings.TrimRight(translated, " ")
					if j == JustifyRight {
						want = strings.TrimLeft(translated, " ")
					}

					r, err := NewReader(bytes.NewReader(src), charsetEncoding(tc.charset))
					require.NoError(t, err)

					got, err := r.ReadAlphanumericJustified(len(src), j)
					require.NoError(t, err)
					require.Equal(t, want, got)
				})
			}
		})
	}
}

// TestReadAlphanumericReusesItsScratchAcrossFields walks a wide field, then a
// narrow one, then a wide one again through a single [Reader], which is where
// a translation scratch reused across fields shows a stale tail.
//
// The scratch is sized for the widest field the Reader has seen and never
// shrunk — the policy [Reader.wide] already follows — so a narrow field
// decoded into it is surrounded by the previous field's bytes on both sides.
// Only the prefix the width says is meaningful may reach the caller.
func TestReadAlphanumericReusesItsScratchAcrossFields(t *testing.T) {
	t.Parallel()

	for _, tc := range alphaTableCharsets {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cs := tc.charset
			wide := allByteValues()
			// A narrow field of bytes the charset spells as something other
			// than a space, so nothing is trimmed and the whole of what the
			// accessor returns is compared.
			narrow := []byte{0x41, 0x42, 0x43}

			var stream []byte
			stream = append(stream, wide...)
			stream = append(stream, narrow...)
			stream = append(stream, wide...)

			translate := func(b []byte) string {
				var sb strings.Builder
				for _, c := range b {
					sb.WriteRune(cs.ToUnicode(c))
				}
				return strings.TrimRight(sb.String(), " ")
			}

			r, err := NewReader(bytes.NewReader(stream), charsetEncoding(cs))
			require.NoError(t, err)

			first, err := r.ReadAlphanumeric(len(wide))
			require.NoError(t, err)
			require.Equal(t, translate(wide), first)

			middle, err := r.ReadAlphanumeric(len(narrow))
			require.NoError(t, err)
			require.Equal(t, translate(narrow), middle)

			last, err := r.ReadAlphanumeric(len(wide))
			require.NoError(t, err)
			require.Equal(t, translate(wide), last)

			// The first field's value survives the two after it, which is the
			// aliasing half: a returned string that viewed the scratch would
			// have been overwritten twice by now.
			require.Equal(t, translate(wide), first)
		})
	}
}

// TestNewReaderTranslatesNothing is TestZonedAccessorsNeverTranslateThroughTheCharset's
// premise stated on its own: constructing a [Reader] performs no character
// translation at all, so the derived table cannot be built there however
// tempting a fully built Reader is.
//
// It is separate because that test proves the rule through two zoned
// accessors, and would still pass if the table were built in [NewReader] and
// the zoned paths merely did not consult it. Counting from construction with
// no read at all is what forbids the eager table.
func TestNewReaderTranslatesNothing(t *testing.T) {
	t.Parallel()

	cs := &countingCharset{Charset: CP037()}
	r, err := NewReader(bytes.NewReader(allByteValues()), charsetEncoding(cs))
	require.NoError(t, err)
	require.Zero(t, cs.toUnicode.Load(), "constructing a Reader translated characters")

	// And the first alphanumeric read is where the table is derived: 256
	// translations for the table, and none per byte after it.
	_, err = r.ReadAlphanumeric(16)
	require.NoError(t, err)
	require.Equal(t, int64(256), cs.toUnicode.Load())

	_, err = r.ReadAlphanumeric(16)
	require.NoError(t, err)
	require.Equal(t, int64(256), cs.toUnicode.Load(), "a second field re-derived the table")
}

// TestReadAlphanumericTrimsPaddingBeforeTranslating walks the two cases the
// order of the trim and the translation is visible in: a field far wider than
// [maxAlphaScratch] whose *value* is not, and a field that is nothing but
// padding.
//
// The order is an implementation choice with no visible effect by design —
// trimming the source bytes that spell U+0020 and trimming U+0020 off the
// translated string are the same operation — so what pins it is the pair of
// answers, under both justifications and under a charset whose space byte is
// not 0x20.
func TestReadAlphanumericTrimsPaddingBeforeTranslating(t *testing.T) {
	t.Parallel()

	for _, tc := range alphaTableCharsets {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cs := tc.charset
			require.Equalf(t, ' ', cs.ToUnicode(cs.Space()),
				"%s's space byte does not decode to U+0020", cs.Name())

			const width = 200
			value := []byte{0x41, 0x42, 0x43}
			var content strings.Builder
			for _, c := range value {
				content.WriteRune(cs.ToUnicode(c))
			}

			testCases := []struct {
				name    string
				field   []byte
				justify Justification
				want    string
			}{
				{
					name:    "padded right",
					field:   append(slices.Clone(value), bytes.Repeat([]byte{cs.Space()}, width-len(value))...),
					justify: JustifyLeft,
					want:    content.String(),
				},
				{
					name:    "padded left",
					field:   append(bytes.Repeat([]byte{cs.Space()}, width-len(value)), value...),
					justify: JustifyRight,
					want:    content.String(),
				},
				{
					name:    "all padding",
					field:   bytes.Repeat([]byte{cs.Space()}, width),
					justify: JustifyLeft,
					want:    "",
				},
				{
					name:    "all padding, right justified",
					field:   bytes.Repeat([]byte{cs.Space()}, width),
					justify: JustifyRight,
					want:    "",
				},
			}

			for _, c := range testCases {
				t.Run(c.name, func(t *testing.T) {
					t.Parallel()

					require.Len(t, c.field, width)

					r, err := NewReader(bytes.NewReader(c.field), charsetEncoding(cs))
					require.NoError(t, err)

					got, err := r.ReadAlphanumericJustified(width, c.justify)
					require.NoError(t, err)
					require.Equal(t, c.want, got)
					require.Equal(t, int64(width), r.Offset(), "the whole field was consumed")
				})
			}
		})
	}
}

// TestReadAlphanumericZeroWidthTranslatesNothing pins the one field width that
// has no bytes to translate. A zero-width PIC X item is legal to ask for — a
// generated record layout can carry an OCCURS 0 group — and it must be an empty
// string, must consume nothing, and must not be what makes a [Reader] derive a
// 256-entry translation table.
func TestReadAlphanumericZeroWidthTranslatesNothing(t *testing.T) {
	t.Parallel()

	for _, j := range []Justification{JustifyLeft, JustifyRight} {
		t.Run(j.String(), func(t *testing.T) {
			t.Parallel()

			cs := &countingCharset{Charset: CP037()}
			r, err := NewReader(bytes.NewReader(allByteValues()), charsetEncoding(cs))
			require.NoError(t, err)

			got, err := r.ReadAlphanumericJustified(0, j)
			require.NoError(t, err)
			require.Empty(t, got)
			require.Zero(t, r.Offset(), "a zero-width field consumed bytes")
			require.Zero(t, cs.toUnicode.Load(), "a zero-width field translated characters")

			// The next field still reads correctly, so the short circuit is
			// not a state the Reader gets stuck in.
			next, err := r.ReadAlphanumericJustified(4, j)
			require.NoError(t, err)
			require.Equal(t, string([]rune{
				cs.ToUnicode(0x00), cs.ToUnicode(0x01),
				cs.ToUnicode(0x02), cs.ToUnicode(0x03),
			}), next)
		})
	}
}

// TestNewBytesReader mirrors TestNewReader over the byte-backed constructor,
// including the one case where the two deliberately differ: a nil []byte is a
// record of no bytes and not a missing source, so it is not the ErrNilReader
// case and there is nothing for it to be.
func TestNewBytesReader(t *testing.T) {
	t.Parallel()

	t.Run("nil data is an empty record", func(t *testing.T) {
		t.Parallel()

		r, err := NewBytesReader(nil, GnuCOBOLASCII())
		require.NoError(t, err)
		require.Zero(t, r.Offset())

		_, err = r.ReadBytes(1)
		require.ErrorIs(t, err, io.EOF)
	})

	t.Run("incomplete encoding names the field", func(t *testing.T) {
		t.Parallel()

		_, err := NewBytesReader(nil, Encoding{})

		var encErr EncodingError
		require.ErrorAs(t, err, &encErr)
		require.Equal(t, "Charset", encErr.Field)
	})

	t.Run("carries its encoding", func(t *testing.T) {
		t.Parallel()

		r, err := NewBytesReader([]byte("A12345"), IBMEnterprise())
		require.NoError(t, err)
		require.Equal(t, IBMEnterprise(), r.Encoding())
		require.Zero(t, r.Offset())
	})
}

// TestNewBytesReaderValidatesExactlyAsNewReader is the assertion behind
// [NewBytesReader]'s doc comment that it validates what [NewReader] validates
// and fails with the same error: the two constructors share one body, and this
// is what fails if a later change gives either its own.
//
// It compares the errors themselves rather than their kinds, so an
// [EncodingError] naming a different field is a failure and not a pass.
func TestNewBytesReaderValidatesExactlyAsNewReader(t *testing.T) {
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

			_, streamErr := NewReader(bytes.NewReader(nil), tc.enc)
			_, bytesErr := NewBytesReader(nil, tc.enc)
			require.Equal(t, streamErr, bytesErr)
		})
	}
}

// TestReaderReset covers the method's contract one clause at a time: the
// position restarts, the encoding and everything derived from it survives, the
// source may be swapped for another record's bytes, and a Reader built over a
// stream may be rewound onto bytes.
func TestReaderReset(t *testing.T) {
	t.Parallel()

	enc := GnuCOBOLASCII()

	t.Run("offset restarts and the encoding survives", func(t *testing.T) {
		t.Parallel()

		data := []byte("ABCDEF")
		r, err := NewBytesReader(data, enc)
		require.NoError(t, err)

		_, err = r.ReadBytes(4)
		require.NoError(t, err)
		require.EqualValues(t, 4, r.Offset())

		r.Reset(data)
		require.Zero(t, r.Offset())
		require.Equal(t, enc, r.Encoding())

		got, err := r.ReadBytes(4)
		require.NoError(t, err)
		require.Equal(t, []byte("ABCD"), got)
	})

	t.Run("rewinds a Reader built over a stream", func(t *testing.T) {
		t.Parallel()

		r, err := NewReader(bytes.NewReader([]byte("STREAM")), enc)
		require.NoError(t, err)

		_, err = r.ReadBytes(3)
		require.NoError(t, err)

		// The stream is dropped: what follows comes from the bytes, and
		// from their first byte.
		r.Reset([]byte("BYTES!"))
		require.Zero(t, r.Offset())

		got, err := r.ReadBytes(6)
		require.NoError(t, err)
		require.Equal(t, []byte("BYTES!"), got)
	})

	t.Run("nil drops the caller's bytes", func(t *testing.T) {
		t.Parallel()

		r, err := NewBytesReader([]byte("ABCDEF"), enc)
		require.NoError(t, err)

		// The pooling shape: a Reader handed back holds nothing, so the
		// record it last read is collectable.
		r.Reset(nil)
		_, err = r.ReadBytes(1)
		require.ErrorIs(t, err, io.EOF)
	})

	t.Run("holds the slice rather than copying it", func(t *testing.T) {
		t.Parallel()

		data := []byte("ABCDEF")
		r, err := NewBytesReader(data, enc)
		require.NoError(t, err)

		// Reset takes the slice, so a write into it before the read is
		// seen by the read. This is the documented lifetime stated as a
		// test: a caller refilling one buffer per record must not do it
		// while the Reader is still reading.
		r.Reset(data)
		copy(data, "ZZZ")

		got, err := r.ReadBytes(6)
		require.NoError(t, err)
		require.Equal(t, []byte("ZZZDEF"), got)
	})
}

// TestReaderResetReadsSeveralRecords is the acceptance criterion of #115 in the
// reading direction: one [Reader] carried across several records, each read
// after a [Reader.Reset], with the values of the earlier records required to
// survive the later ones.
//
// It is TestReadValuesDoNotAliasTheReusedBuffer one level up. That test reads
// two records from one stream, so the only thing that could alias is the
// Reader's own scratch; this one reads each record from a slice the caller
// holds, so a value aliasing *the caller's bytes* would fail here and pass
// there.
func TestReaderResetReadsSeveralRecords(t *testing.T) {
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
			ID:      "C00001",
			Name:    "",
			Code:    "",
			Raw:     []byte{0x00, 0x00, 0x00},
			Amount:  0,
			Qty:     0,
			Units:   0,
			Seq:     0,
			Rate:    0,
			Factor:  0,
			Balance: 0,
			Count:   0,
		},
	}

	records := make([][]byte, len(want))
	for i, rec := range want {
		b, err := Marshal(enc, &rec)
		require.NoError(t, err)
		require.Len(t, b, testRecordWidth)
		records[i] = b
	}

	r, err := NewBytesReader(nil, enc)
	require.NoError(t, err)

	got := make([]testRecord, len(want))
	for i, data := range records {
		r.Reset(data)
		require.NoError(t, got[i].UnmarshalCOBOL(r))
		require.EqualValues(t, testRecordWidth, r.Offset(),
			"the offset restarts per record, so it ends at the record's width and not at the file's")
		require.Equal(t, want[i], got[i])
	}

	// The whole point: no record's values were disturbed by the records
	// read after it, through the Reader's scratch or through the caller's
	// slices.
	require.Equal(t, want, got)

	// Nor does a value view the bytes it was decoded from: overwriting
	// every record leaves every value as it was.
	for _, data := range records {
		for i := range data {
			data[i] = 0xFF
		}
	}
	require.Equal(t, want, got)
}

// TestReaderResetErrorsMatchTheStream holds the byte-backed source to the
// errors the [io.Reader] one produces, which is what keeps [Reader.Reset] from
// being a second error-stamping path. The offsets are the interesting half: a
// record that runs out mid-field must report the byte it stopped at, counted
// from the Reset and not from the file.
func TestReaderResetErrorsMatchTheStream(t *testing.T) {
	t.Parallel()

	enc := GnuCOBOLASCII()

	testCases := []struct {
		name string
		data []byte
		// drain reads the record dry before the read under test, so the
		// failing read starts at the end of the data rather than at its
		// beginning.
		drain bool
		want  error
	}{
		{
			name: "empty record is EOF at offset zero",
			data: nil,
			want: io.EOF,
		},
		{
			name: "short record is an unexpected EOF at what it held",
			data: []byte{0x00, 0x01},
			want: io.ErrUnexpectedEOF,
		},
		{
			// The boundary the hand-written EOF branch exists for: a
			// record that ended exactly on a field boundary is EOF and
			// not a short read, so a record read to its end and asked
			// for one more field answers what a stream answers.
			name:  "a record ended exactly is EOF at its width",
			data:  []byte{0x00, 0x01, 0x02, 0x03},
			drain: true,
			want:  io.EOF,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stream, err := NewReader(bytes.NewReader(tc.data), enc)
			require.NoError(t, err)

			r, err := NewBytesReader(nil, enc)
			require.NoError(t, err)
			r.Reset(tc.data)

			if tc.drain {
				_, err = stream.ReadBytes(len(tc.data))
				require.NoError(t, err)
				_, err = r.ReadBytes(len(tc.data))
				require.NoError(t, err)
			}

			_, streamErr := stream.ReadBytes(4)
			_, resetErr := r.ReadBytes(4)

			require.ErrorIs(t, resetErr, tc.want)
			require.Equal(t, streamErr, resetErr)

			var offErr *OffsetError
			require.ErrorAs(t, resetErr, &offErr)
			require.EqualValues(t, len(tc.data), offErr.Offset,
				"the offset names where reading stopped, counted from the Reset")
			require.EqualValues(t, len(tc.data), r.Offset())
			require.Equal(t, stream.Offset(), r.Offset())
		})
	}
}

// TestResetDoesNotAllocate is #115's claim in the enforceable form
// TestReadingDoesNotAllocate uses: rewinding a [Reader] or a [Writer] onto the
// next record allocates nothing at all, which is the whole of why a pooled
// codec beats one built per record.
//
// It asserts zero rather than pinning a count, for the reason codec/CLAUDE.md
// gives: a count moves with the toolchain, and zero does not. Zero here means
// the record's source is held rather than wrapped, and that the writer's buffer
// is reused at its capacity rather than reallocated.
//
// It is the second of the package's two tests without t.Parallel(), at either
// level. [testing.AllocsPerRun] sets GOMAXPROCS to 1 for the duration of its
// run and panics outright when called from a parallel test.
func TestResetDoesNotAllocate(t *testing.T) {
	enc := GnuCOBOLASCII()

	t.Run("reader", func(t *testing.T) {
		field := []byte{0x04, 0xD2}

		r, err := NewBytesReader(field, enc)
		require.NoError(t, err)

		read := func() {
			r.Reset(field)
			if _, err := r.ReadBinaryInt16(testRecordSeqDigits); err != nil {
				t.Error(err)
			}
		}
		read()

		require.Zero(t, testing.AllocsPerRun(100, read),
			"rewinding a Reader onto a record allocates")
	})

	t.Run("writer", func(t *testing.T) {
		field := []byte{0x04, 0xD2}

		w, err := NewBytesWriter(make([]byte, 0, len(field)), enc)
		require.NoError(t, err)
		buf := w.Bytes()

		write := func() {
			w.Reset(buf)
			if err := w.WriteBytes(field); err != nil {
				t.Error(err)
			}
		}
		write()

		require.Zero(t, testing.AllocsPerRun(100, write),
			"rewinding a Writer onto its own buffer allocates")
	})
}

// TestZeroValueIsStillUnusable pins the invariant both type doc comments assert
// and that a second kind of source could quietly have taken away: a [Reader] or
// a [Writer] nobody constructed does not work.
//
// It is not a hypothetical. Discriminating on a nil [io.Reader] rather than on
// [Reader.fromBytes] would make the zero Reader read as an *empty record* and
// answer [io.EOF] — a plausible answer from a Reader whose [Encoding] was never
// validated — and the zero Writer would go further and *succeed*, appending
// bytes under an encoding with none of its four axes set. Both panic instead,
// exactly as they did before there was a byte-backed path, and the panic is the
// nil interface being used rather than anything this package raises.
func TestZeroValueIsStillUnusable(t *testing.T) {
	t.Parallel()

	t.Run("reader", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			_, _ = (&Reader{}).ReadBytes(1) //nolint:errcheck // the panic is the assertion
		}, "a Reader nobody constructed read as an empty record instead of failing")
	})

	t.Run("writer", func(t *testing.T) {
		t.Parallel()

		require.Panics(t, func() {
			_ = (&Writer{}).WriteBytes([]byte{0x00}) //nolint:errcheck // the panic is the assertion
		}, "a Writer nobody constructed wrote bytes under an unvalidated encoding")
	})
}
