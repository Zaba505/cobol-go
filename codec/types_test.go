// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"encoding/binary"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestEncodingValidate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		enc       Encoding
		wantField string
	}{
		{
			name:      "zero value names the charset first",
			enc:       Encoding{},
			wantField: "Charset",
		},
		{
			name:      "missing charset",
			enc:       Encoding{Sign: SignEBCDIC, ByteOrder: binary.BigEndian, Float: FloatHFP},
			wantField: "Charset",
		},
		{
			name:      "missing sign convention",
			enc:       Encoding{Charset: CP037(), ByteOrder: binary.BigEndian, Float: FloatHFP},
			wantField: "Sign",
		},
		{
			name:      "unknown sign convention",
			enc:       Encoding{Charset: CP037(), Sign: SignConvention(99), ByteOrder: binary.BigEndian, Float: FloatHFP},
			wantField: "Sign",
		},
		{
			name:      "missing byte order",
			enc:       Encoding{Charset: CP037(), Sign: SignEBCDIC, Float: FloatHFP},
			wantField: "ByteOrder",
		},
		{
			name:      "missing float format",
			enc:       Encoding{Charset: CP037(), Sign: SignEBCDIC, ByteOrder: binary.BigEndian},
			wantField: "Float",
		},
		{
			name:      "unknown float format",
			enc:       Encoding{Charset: CP037(), Sign: SignEBCDIC, ByteOrder: binary.BigEndian, Float: FloatFormat(42)},
			wantField: "Float",
		},
		{
			name: "missing binary size",
			enc: Encoding{
				Charset:   CP037(),
				Sign:      SignEBCDIC,
				ByteOrder: binary.BigEndian,
				Float:     FloatHFP,
			},
			wantField: "Binary",
		},
		{
			name: "unknown binary size",
			enc: Encoding{
				Charset:   CP037(),
				Sign:      SignEBCDIC,
				ByteOrder: binary.BigEndian,
				Float:     FloatHFP,
				Binary:    BinarySize(42),
			},
			wantField: "Binary",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.enc.Validate()

			var encErr EncodingError
			require.ErrorAs(t, err, &encErr)
			require.Equal(t, tc.wantField, encErr.Field)
		})
	}

	t.Run("complete encoding validates", func(t *testing.T) {
		t.Parallel()

		enc := Encoding{
			Charset:   ASCII(),
			Sign:      SignASCIIZone37,
			ByteOrder: binary.LittleEndian,
			Float:     FloatIEEE,
			Binary:    BinarySizeSmallest,
		}
		require.NoError(t, enc.Validate())
	})
}

func TestDialects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		enc  Encoding
		want Encoding
	}{
		{
			name: "ibm enterprise",
			enc:  IBMEnterprise(),
			want: Encoding{
				Charset:   CP037(),
				Sign:      SignEBCDIC,
				ByteOrder: binary.BigEndian,
				Float:     FloatHFP,
				Binary:    BinarySize248,
			},
		},
		{
			name: "micro focus ascii",
			enc:  MicroFocusASCII(),
			want: Encoding{
				Charset:   ASCII(),
				Sign:      SignASCIIZone37,
				ByteOrder: binary.NativeEndian,
				Float:     FloatIEEE,
				Binary:    BinarySize248,
			},
		},
		{
			name: "gnucobol ascii",
			enc:  GnuCOBOLASCII(),
			want: Encoding{
				Charset:   ASCII(),
				Sign:      SignASCIIZone37,
				ByteOrder: binary.BigEndian,
				Float:     FloatIEEE,
				Binary:    BinarySize1248,
			},
		},
		{
			name: "converted from ebcdic",
			enc:  ConvertedFromEBCDIC(),
			want: Encoding{
				Charset:   ASCII(),
				Sign:      SignTranslatedEBCDIC,
				ByteOrder: binary.BigEndian,
				Float:     FloatHFP,
				Binary:    BinarySize248,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.enc)
			// A bundle is only useful if it is complete: that is the whole
			// point of naming one rather than filling in fields by hand.
			require.NoError(t, tc.enc.Validate())
		})
	}
}

func TestSignConventionString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		s    SignConvention
		want string
	}{
		{name: "unset", s: SignUnset, want: "unset"},
		{name: "ebcdic", s: SignEBCDIC, want: "ebcdic"},
		{name: "ascii zone 3/7", s: SignASCIIZone37, want: "ascii-zone-3-7"},
		{name: "translated ebcdic", s: SignTranslatedEBCDIC, want: "translated-ebcdic"},
		{name: "realia", s: SignRealia, want: "realia"},
		{name: "out of range", s: SignConvention(99), want: "SignConvention(99)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.s.String())
		})
	}
}

func TestSignPositionString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		s    SignPosition
		want string
	}{
		{name: "unset", s: SignPositionUnset, want: "unset"},
		{name: "unsigned", s: SignUnsigned, want: "unsigned"},
		{name: "trailing", s: SignTrailing, want: "trailing"},
		{name: "leading", s: SignLeading, want: "leading"},
		{name: "trailing separate", s: SignTrailingSeparate, want: "trailing-separate"},
		{name: "leading separate", s: SignLeadingSeparate, want: "leading-separate"},
		{name: "out of range", s: SignPosition(99), want: "SignPosition(99)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.s.String())
			require.Equal(t, tc.s != SignPositionUnset && tc.s != SignPosition(99), tc.s.valid())
		})
	}
}

// TestZonedWidth pins the whole of the zoned size model: one byte per digit,
// and one more only when the sign has a byte of its own. Getting the SEPARATE
// step wrong shifts every field after it in the record.
func TestZonedWidth(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		sign        SignPosition
		wantWidth   int
		wantOverpun int
	}{
		{name: "unsigned", sign: SignUnsigned, wantWidth: 5, wantOverpun: -1},
		{name: "trailing overpunch", sign: SignTrailing, wantWidth: 5, wantOverpun: 4},
		{name: "leading overpunch", sign: SignLeading, wantWidth: 5, wantOverpun: 0},
		{name: "trailing separate", sign: SignTrailingSeparate, wantWidth: 6, wantOverpun: -1},
		{name: "leading separate", sign: SignLeadingSeparate, wantWidth: 6, wantOverpun: -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// PIC S9(5), or PIC S9(3)V99: a V occupies no byte, so scale
			// never enters the width.
			require.Equal(t, tc.wantWidth, zonedWidth(5, tc.sign))
			require.Equal(t, tc.wantOverpun, tc.sign.overpunchAt(5))
			require.Equal(t, tc.wantWidth > 5, tc.sign.separate())
		})
	}
}

// TestPackedAndComp6Widths pins the two packed width formulas against each
// other. They differ by a byte at every *even* digit count and coincide at
// every odd one, which is the fact a reader of either encoding has to know: at
// an odd digit count a mis-declared usage does not shift the record, so nothing
// but the nibbles catches it — and the pad nibble sits on the opposite parity
// precisely so that they do.
func TestPackedAndComp6Widths(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		digits     int
		wantPacked int
		wantComp6  int
	}{
		{digits: 1, wantPacked: 1, wantComp6: 1},
		{digits: 2, wantPacked: 2, wantComp6: 1},
		{digits: 3, wantPacked: 2, wantComp6: 2},
		{digits: 4, wantPacked: 3, wantComp6: 2},
		{digits: 5, wantPacked: 3, wantComp6: 3},
		{digits: 18, wantPacked: 10, wantComp6: 9},
		{digits: 31, wantPacked: 16, wantComp6: 16},
	}

	for _, tc := range testCases {
		t.Run(strconv.Itoa(tc.digits)+" digits", func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.wantPacked, packedWidth(tc.digits))
			require.Equal(t, tc.wantComp6, comp6Width(tc.digits))

			if tc.digits%2 == 0 {
				require.Equal(t, packedWidth(tc.digits)-1, comp6Width(tc.digits))
			} else {
				require.Equal(t, packedWidth(tc.digits), comp6Width(tc.digits))
			}
		})
	}
}

func TestBinarySizeString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		b    BinarySize
		want string
	}{
		{name: "unset", b: BinarySizeUnset, want: "unset"},
		{name: "2-4-8", b: BinarySize248, want: "2-4-8"},
		{name: "1-2-4-8", b: BinarySize1248, want: "1-2-4-8"},
		{name: "smallest", b: BinarySizeSmallest, want: "1--8"},
		{name: "full", b: BinarySizeFull, want: "full"},
		{name: "out of range", b: BinarySize(42), want: "BinarySize(42)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// The four named spellings are GnuCOBOL's own binary-size values,
			// and copybook.BinarySize returns the same strings for the same
			// members; copybook's TestBinarySizeAgreesWithCodec is what holds
			// the two together.
			require.Equal(t, tc.want, tc.b.String())
		})
	}
}

func TestFloatFormatString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		f    FloatFormat
		want string
	}{
		{name: "unset", f: FloatUnset, want: "unset"},
		{name: "ieee", f: FloatIEEE, want: "ieee-754"},
		{name: "hfp", f: FloatHFP, want: "ibm-hfp"},
		{name: "out of range", f: FloatFormat(42), want: "FloatFormat(42)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.f.String())
		})
	}
}

func TestJustificationString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "left", JustifyLeft.String())
	require.Equal(t, "right", JustifyRight.String())
	require.Equal(t, "Justification(7)", Justification(7).String())
	// The zero value is the COBOL default rather than an error, unlike the
	// four Encoding axes.
	require.Equal(t, JustifyLeft, Justification(0))
}

// shippedCharsets is every charset this package exports, and the single place
// a new one is added to the suite: the bijectivity walk below and the
// alphanumeric walks in decoder_test.go and encoder_test.go all run over it,
// so a code page added to the package is covered by construction rather than
// by remembering to extend three tables.
var shippedCharsets = []struct {
	name      string
	charset   Charset
	wantName  string
	wantSpace byte
}{
	{name: "ascii", charset: ASCII(), wantName: "ASCII", wantSpace: 0x20},
	{name: "cp037", charset: CP037(), wantName: "cp037", wantSpace: 0x40},
}

func TestCharsetIsTotalAndBijective(t *testing.T) {
	t.Parallel()

	for _, tc := range shippedCharsets {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.wantName, tc.charset.Name())
			require.Equal(t, tc.wantSpace, tc.charset.Space())
			require.Equal(t, ' ', tc.charset.ToUnicode(tc.charset.Space()))

			// Decoding must be total and reversible over all 256 bytes: any
			// byte may appear in a PIC X field, so a reader must never fail
			// on one, and a byte that does not survive the round trip would
			// corrupt a binary payload carried in such a field.
			seen := make(map[rune]byte, 256)
			for i := range 256 {
				b := byte(i)
				r := tc.charset.ToUnicode(b)

				prev, dup := seen[r]
				require.Falsef(t, dup, "byte %#02x and byte %#02x both decode to %q", b, prev, r)
				seen[r] = b

				got, ok := tc.charset.FromUnicode(r)
				require.Truef(t, ok, "byte %#02x decodes to %q which does not encode back", b, r)
				require.Equalf(t, b, got, "byte %#02x round-tripped to %#02x", b, got)
			}

			// The other direction over the same 256 characters. A bijection
			// makes this the same statement, which is the point: asserting it
			// both ways is what would catch a table that had stopped being
			// one.
			for r := range seen {
				b, ok := tc.charset.FromUnicode(r)
				require.Truef(t, ok, "character %q has no byte", r)
				require.Equalf(t, r, tc.charset.ToUnicode(b), "character %q round-tripped through byte %#02x", r, b)
			}
			require.Len(t, seen, 256)
		})
	}
}

func TestCP037KnownCodePoints(t *testing.T) {
	t.Parallel()

	// The bytes this package's later stories depend on: the digit zone F, the
	// overpunch letters, the separate sign bytes, and the '{' / '}' that a
	// zero with an EBCDIC sign spells.
	testCases := []struct {
		name string
		text string
		want []byte
	}{
		{name: "digits are zone F", text: "0123456789", want: []byte{0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9}},
		{name: "A through I", text: "ABCDEFGHI", want: []byte{0xC1, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9}},
		{name: "J through R", text: "JKLMNOPQR", want: []byte{0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9}},
		{name: "S through Z", text: "STUVWXYZ", want: []byte{0xE2, 0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9}},
		{name: "separate sign bytes", text: "+-", want: []byte{0x4E, 0x60}},
		{name: "braces", text: "{}", want: []byte{0xC0, 0xD0}},
		{name: "space", text: " ", want: []byte{0x40}},
	}

	charset := CP037()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := make([]byte, 0, len(tc.want))
			for _, r := range tc.text {
				b, ok := charset.FromUnicode(r)
				require.Truef(t, ok, "character %q has no cp037 byte", r)
				got = append(got, b)
			}
			require.Equal(t, tc.want, got)

			for i, b := range tc.want {
				require.Equal(t, rune(tc.text[i]), charset.ToUnicode(b))
			}
		})
	}
}

func TestASCIIRejectsUnrepresentableRunes(t *testing.T) {
	t.Parallel()

	_, ok := ASCII().FromUnicode('€')
	require.False(t, ok)

	_, ok = CP037().FromUnicode('€')
	require.False(t, ok)
}

// allSignConventions is every convention a file may declare, in declaration
// order. The mutual-detectability tests below walk it against itself.
var allSignConventions = []SignConvention{
	SignEBCDIC,
	SignASCIIZone37,
	SignTranslatedEBCDIC,
	SignRealia,
}

func TestZonedBytesOf(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		charset    Charset
		wantDigits [10]byte
		wantPlus   byte
		wantMinus  byte
	}{
		{
			name:       "ascii digits are zone 3 and signs are 2B/2D",
			charset:    ASCII(),
			wantDigits: [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
			wantPlus:   0x2B,
			wantMinus:  0x2D,
		},
		{
			name:       "cp037 digits are zone F and signs are 4E/60",
			charset:    CP037(),
			wantDigits: [10]byte{0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9},
			wantPlus:   0x4E,
			wantMinus:  0x60,
		},
		{
			// Nothing about the two shipped pages is hard-coded: a caller's
			// own charset is asked the same questions.
			name:       "a caller's charset supplies its own bytes",
			charset:    oddballCharset{},
			wantDigits: [10]byte{0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7, 0xB8, 0xB9},
			wantPlus:   0x01,
			wantMinus:  0x02,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			z, err := zonedBytesOf(tc.charset)
			require.NoError(t, err)
			require.Equal(t, tc.charset.Name(), z.charset)
			require.Equal(t, tc.wantDigits, z.digits)
			require.Equal(t, tc.wantPlus, z.plus)
			require.Equal(t, tc.wantMinus, z.minus)

			for d := range 10 {
				b, err := z.digitByte(byte(d))
				require.NoError(t, err)
				require.Equal(t, tc.wantDigits[d], b)

				got, err := z.digitValue(b)
				require.NoError(t, err)
				require.Equal(t, byte(d), got)
			}

			require.Equal(t, tc.wantPlus, z.separateSignByte(false))
			require.Equal(t, tc.wantMinus, z.separateSignByte(true))

			negative, err := z.separateSignValue(tc.wantPlus)
			require.NoError(t, err)
			require.False(t, negative)

			negative, err = z.separateSignValue(tc.wantMinus)
			require.NoError(t, err)
			require.True(t, negative)
		})
	}
}

func TestZonedBytesOfRejectsUnusableCharset(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		charset  Charset
		wantRune rune
	}{
		{name: "no digits", charset: partialCharset{}, wantRune: '0'},
		{name: "digits but no plus", charset: partialCharset{digits: true}, wantRune: '+'},
		{name: "digits and plus but no minus", charset: partialCharset{digits: true, plus: true}, wantRune: '-'},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := zonedBytesOf(tc.charset)

			var runeErr UnrepresentableRuneError
			require.ErrorAs(t, err, &runeErr)
			require.Equal(t, tc.wantRune, runeErr.Rune)
			require.Equal(t, "partial", runeErr.Charset)
		})
	}
}

// digitValueByScan is the linear scan [zonedBytes.digitValue] used before it
// became a table lookup, transcribed unchanged. It is the reference the test
// below checks the table against, byte for byte and error for error, so the
// claim that the table *is* the scan stays legible now that the scan is gone.
func digitValueByScan(z *zonedBytes, b byte) (byte, error) {
	if d := slices.Index(z.digits[:], b); d >= 0 {
		return byte(d), nil
	}
	return 0, ZonedDigitError{Byte: b, Charset: z.charset, Zero: z.digits[0], Nine: z.digits[9]}
}

func TestZonedDigitValueMatchesTheScan(t *testing.T) {
	t.Parallel()

	// The two collapsing charsets are the ones that matter. Charset.FromUnicode
	// is nowhere required to be injective, so a caller may hand over a charset
	// spelling several digits with one byte; the scan read such a byte as the
	// *lowest* of them, and the inverse table has to agree or files that
	// decoded one way yesterday decode another way today.
	testCases := []struct {
		name    string
		charset Charset
	}{
		{name: "ascii", charset: ASCII()},
		{name: "cp037", charset: CP037()},
		{name: "a caller's own charset", charset: oddballCharset{}},
		{name: "a charset folding digits in pairs", charset: collapsingCharset{fold: 2}},
		{name: "a charset spelling every digit alike", charset: collapsingCharset{fold: 10}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			z, err := zonedBytesOf(tc.charset)
			require.NoError(t, err)

			for b := range 256 {
				gotDigit, gotErr := z.digitValue(byte(b))
				wantDigit, wantErr := digitValueByScan(&z, byte(b))

				require.Equalf(t, wantErr, gotErr, "byte %#02X", b)
				require.Equalf(t, wantDigit, gotDigit, "byte %#02X", b)
			}
		})
	}
}

func TestZonedDigitValueOnAnUnbuiltTableRejectsEverything(t *testing.T) {
	t.Parallel()

	// The zero value of zonedBytes is reachable — zonedBytesOf returns it
	// beside each of its errors, and a zonedBytes{} literal is one — so what
	// it does matters even though every in-package path guards on the error
	// first. A table whose unwritten entry meant "digit 0" would read all 256
	// bytes as a zero off an unbuilt table; storing digits biased by one is
	// what makes the unwritten entry mean "no digit" instead.
	//
	// This is the one place the table deliberately does *not* reproduce the
	// scan. The scan read 0x00 as a 0 here, because an unbuilt digits array
	// spells every digit 0x00 and slices.Index found digit 0 at index 0; the
	// table rejects it. One byte fewer accepted, on a value that only exists
	// when construction has already failed, is the direction a decoder should
	// differ in — and it is the direction that makes an unchecked error loud
	// instead of quiet.
	var z zonedBytes

	for b := range 256 {
		_, err := z.digitValue(byte(b))
		require.Equalf(t, ZonedDigitError{Byte: byte(b)}, err, "byte %#02X", b)
	}

	// The scan, for contrast, accepted exactly one of them.
	_, err := digitValueByScan(&z, 0x00)
	require.NoError(t, err)
}

func TestZonedSeparateSignValueWhenPlusAndMinusCollide(t *testing.T) {
	t.Parallel()

	// Nothing requires a charset to spell '+' and '-' differently either, and
	// the same rule applies: the first candidate wins, so a shared byte is
	// positive. Reading it as negative would flip the sign of every value in a
	// separate-sign field.
	z, err := zonedBytesOf(collapsingCharset{fold: 1, signsCollide: true})
	require.NoError(t, err)
	require.Equal(t, z.plus, z.minus)

	negative, err := z.separateSignValue(z.plus)
	require.NoError(t, err)
	require.False(t, negative)

	// The write direction is unaffected: it answers from the sign, not from
	// the byte, so it still spells both signs — alike, as the charset asked.
	require.Equal(t, z.plus, z.separateSignByte(false))
	require.Equal(t, z.minus, z.separateSignByte(true))

	// And no other byte has become a sign.
	for b := range 256 {
		if byte(b) == z.plus {
			continue
		}
		_, err := z.separateSignValue(byte(b))
		require.Equalf(t, ZonedSeparateSignError{
			Byte: byte(b), Charset: z.charset, Plus: z.plus, Minus: z.minus,
		}, err, "byte %#02X", b)
	}
}

// signByteValueByScan is the four-pass linear scan [signByteValue] used before
// it became a table lookup, transcribed unchanged. It is what
// TestZonedSignByteValueMatchesTheScan checks the built table against.
func signByteValueByScan(s SignConvention, b byte) (digit byte, negative bool, err error) {
	if !s.valid() {
		return 0, false, EncodingError{Field: "Sign", Reason: "is required and has no default"}
	}
	t := zonedSignTables[s]
	if d := slices.Index(t.negative[:], b); d >= 0 {
		return byte(d), true, nil
	}
	if d := slices.Index(t.positive[:], b); d >= 0 {
		return byte(d), false, nil
	}
	if d := slices.Index(t.unsigned[:], b); d >= 0 {
		return byte(d), false, nil
	}
	if b&0x0F <= 9 {
		if slices.Contains(t.lenientNegativeZones, b&0xF0) {
			return b & 0x0F, true, nil
		}
		if slices.Contains(t.lenientPositiveZones, b&0xF0) {
			return b & 0x0F, false, nil
		}
	}
	return 0, false, ZonedSignError{Byte: b, Sign: s}
}

func TestZonedSignByteValueMatchesTheScan(t *testing.T) {
	t.Parallel()

	// Exhaustive on purpose: four conventions by 256 byte values is 1024
	// cases, and moving signByteValue to a precomputed table moves the
	// documented precedence — negative, positive, unsigned, then the lenient
	// EBCDIC zones — and the low-nibble guard on those zones into a build
	// loop. Precedence and guard are cheap to get subtly wrong and expensive
	// to notice: the failure mode is a byte no convention should accept being
	// read as a digit, which is the mutual detectability the whole sign model
	// rests on.
	for _, sign := range allSignConventions {
		t.Run(sign.String(), func(t *testing.T) {
			t.Parallel()

			for b := range 256 {
				gotDigit, gotNegative, gotErr := signByteValue(sign, byte(b))
				wantDigit, wantNegative, wantErr := signByteValueByScan(sign, byte(b))

				require.Equalf(t, wantErr, gotErr, "byte %#02X", b)
				require.Equalf(t, wantDigit, gotDigit, "byte %#02X", b)
				require.Equalf(t, wantNegative, gotNegative, "byte %#02X", b)
			}
		})
	}

	t.Run("unset", func(t *testing.T) {
		t.Parallel()

		// The zero row of the table is built like any other and is unreachable
		// for the reason it always was: the convention is checked first.
		for b := range 256 {
			_, _, err := signByteValue(SignUnset, byte(b))
			require.Equalf(t, EncodingError{Field: "Sign", Reason: "is required and has no default"}, err, "byte %#02X", b)
		}
	})
}

func TestZonedDigitByteRejectsWrongCharset(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		charset  Charset
		b        byte
		wantZero byte
		wantNine byte
	}{
		{name: "ebcdic digit under ascii", charset: ASCII(), b: 0xF5, wantZero: 0x30, wantNine: 0x39},
		{name: "ascii digit under cp037", charset: CP037(), b: 0x35, wantZero: 0xF0, wantNine: 0xF9},
		{name: "overpunched sign byte in a plain digit position", charset: ASCII(), b: 0x75, wantZero: 0x30, wantNine: 0x39},
		{name: "letter under ascii", charset: ASCII(), b: 'A', wantZero: 0x30, wantNine: 0x39},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			z, err := zonedBytesOf(tc.charset)
			require.NoError(t, err)

			_, err = z.digitValue(tc.b)

			var digitErr ZonedDigitError
			require.ErrorAs(t, err, &digitErr)
			require.Equal(t, tc.b, digitErr.Byte)
			require.Equal(t, tc.charset.Name(), digitErr.Charset)
			require.Equal(t, tc.wantZero, digitErr.Zero)
			require.Equal(t, tc.wantNine, digitErr.Nine)
		})
	}
}

func TestZonedSeparateSignRejectsOtherBytes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		charset Charset
		b       byte
	}{
		{name: "ebcdic plus under ascii", charset: ASCII(), b: 0x4E},
		{name: "ascii plus under cp037", charset: CP037(), b: 0x2B},
		{name: "a digit is not a sign", charset: ASCII(), b: 0x35},
		{name: "a space is not a sign", charset: ASCII(), b: 0x20},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			z, err := zonedBytesOf(tc.charset)
			require.NoError(t, err)

			_, err = z.separateSignValue(tc.b)

			var sepErr ZonedSeparateSignError
			require.ErrorAs(t, err, &sepErr)
			require.Equal(t, tc.b, sepErr.Byte)
			require.Equal(t, tc.charset.Name(), sepErr.Charset)
			require.Equal(t, z.plus, sepErr.Plus)
			require.Equal(t, z.minus, sepErr.Minus)
		})
	}
}

func TestZonedSignByteTables(t *testing.T) {
	t.Parallel()

	// codec/SPEC.md, "Zoned Sign Conventions", digit by digit. A writer emits
	// exactly these bytes; the round trip below asserts a reader reads them
	// back as the digit and sign they were written from.
	testCases := []struct {
		name         string
		sign         SignConvention
		positive     [10]byte
		negative     [10]byte
		wantUnsigned [10]byte
	}{
		{
			name:         "ebcdic",
			sign:         SignEBCDIC,
			positive:     [10]byte{0xC0, 0xC1, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9},
			negative:     [10]byte{0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9},
			wantUnsigned: [10]byte{0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9},
		},
		{
			name:         "ascii zone 3/7",
			sign:         SignASCIIZone37,
			positive:     [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
			negative:     [10]byte{0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79},
			wantUnsigned: [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
		},
		{
			name:         "translated ebcdic",
			sign:         SignTranslatedEBCDIC,
			positive:     [10]byte{'{', 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'I'},
			negative:     [10]byte{'}', 'J', 'K', 'L', 'M', 'N', 'O', 'P', 'Q', 'R'},
			wantUnsigned: [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
		},
		{
			name:         "realia",
			sign:         SignRealia,
			positive:     [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
			negative:     [10]byte{' ', '!', '"', '#', '$', '%', '&', '\'', '(', ')'},
			wantUnsigned: [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for d := range 10 {
				for _, want := range []struct {
					negative bool
					b        byte
				}{
					{negative: false, b: tc.positive[d]},
					{negative: true, b: tc.negative[d]},
				} {
					got, err := signByte(tc.sign, byte(d), want.negative)
					require.NoErrorf(t, err, "digit %d negative=%v", d, want.negative)
					require.Equalf(t, want.b, got, "digit %d negative=%v", d, want.negative)

					gotDigit, gotNegative, err := signByteValue(tc.sign, got)
					require.NoErrorf(t, err, "byte %#02X", got)
					require.Equalf(t, byte(d), gotDigit, "byte %#02X", got)
					require.Equalf(t, want.negative, gotNegative, "byte %#02X", got)
				}

				// An unsigned-zone byte in the sign position reads as a
				// non-negative digit rather than as a corruption: a signed
				// item routinely holds one after a MOVE from an unsigned one.
				gotDigit, gotNegative, err := signByteValue(tc.sign, tc.wantUnsigned[d])
				require.NoErrorf(t, err, "unsigned byte %#02X", tc.wantUnsigned[d])
				require.Equal(t, byte(d), gotDigit)
				require.False(t, gotNegative)
			}
		})
	}
}

func TestZonedSignConventionsAreMutuallyDetectable(t *testing.T) {
	t.Parallel()

	// codec/SPEC.md, "Validation and detectability": for every pair of
	// conventions, each one's negative bytes are invalid under the other. That
	// property is what turns a wrong SignConvention from silently flipped
	// signs into a failure at the first negative value.
	for _, sign := range allSignConventions {
		t.Run(sign.String(), func(t *testing.T) {
			t.Parallel()

			for d := range 10 {
				b, err := signByte(sign, byte(d), true)
				require.NoError(t, err)

				for _, other := range allSignConventions {
					if other == sign {
						continue
					}
					_, _, err := signByteValue(other, b)

					var signErr ZonedSignError
					require.ErrorAsf(t, err, &signErr, "negative %d is %#02X under %s, which %s must reject", d, b, sign, other)
					require.Equal(t, b, signErr.Byte)
					require.Equal(t, other, signErr.Sign)
				}
			}
		})
	}
}

func TestZonedSignByteAgainstSpecDetectabilityTable(t *testing.T) {
	t.Parallel()

	// The worked table from codec/SPEC.md, "Validation and detectability",
	// transcribed row for row. want is nil where the byte must be rejected.
	type reading struct {
		digit    byte
		negative bool
	}
	testCases := []struct {
		name     string
		b        byte
		readings map[SignConvention]*reading
	}{
		{
			name: "D5 is EBCDIC -5 and nothing else",
			b:    0xD5,
			readings: map[SignConvention]*reading{
				SignEBCDIC: {digit: 5, negative: true},
			},
		},
		{
			name: "75 is zone 3/7 -5 and nothing else",
			b:    0x75,
			readings: map[SignConvention]*reading{
				SignASCIIZone37: {digit: 5, negative: true},
			},
		},
		{
			name: "4E is translated -5 and nothing else",
			b:    0x4E,
			readings: map[SignConvention]*reading{
				SignTranslatedEBCDIC: {digit: 5, negative: true},
			},
		},
		{
			name: "25 is Realia -5 and nothing else",
			b:    0x25,
			readings: map[SignConvention]*reading{
				SignRealia: {digit: 5, negative: true},
			},
		},
		{
			name: "35 is a positive or unsigned 5 everywhere but EBCDIC",
			b:    0x35,
			readings: map[SignConvention]*reading{
				SignASCIIZone37:      {digit: 5},
				SignTranslatedEBCDIC: {digit: 5},
				SignRealia:           {digit: 5},
			},
		},
		{
			name: "7B is a translated +0 and is not a digit under zone 3/7",
			b:    0x7B,
			readings: map[SignConvention]*reading{
				SignTranslatedEBCDIC: {digit: 0},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, sign := range allSignConventions {
				digit, negative, err := signByteValue(sign, tc.b)

				want, ok := tc.readings[sign]
				if !ok {
					var signErr ZonedSignError
					require.ErrorAsf(t, err, &signErr, "%s must reject %#02X", sign, tc.b)
					continue
				}
				require.NoErrorf(t, err, "%s must read %#02X", sign, tc.b)
				require.Equalf(t, want.digit, digit, "%s reading of %#02X", sign, tc.b)
				require.Equalf(t, want.negative, negative, "%s reading of %#02X", sign, tc.b)
			}
		})
	}
}

func TestZonedSignByteLenientEBCDIC(t *testing.T) {
	t.Parallel()

	// z/Architecture decimal instructions accept more sign values than they
	// generate, and real files carry them. The lenient zones are EBCDIC's
	// alone: they are all above 0x9F, so widening SignEBCDIC this far still
	// overlaps none of the three ASCII conventions.
	testCases := []struct {
		name         string
		b            byte
		wantDigit    byte
		wantNegative bool
	}{
		{name: "zone A is positive", b: 0xA7, wantDigit: 7},
		{name: "zone B is negative", b: 0xB7, wantDigit: 7, wantNegative: true},
		{name: "zone C is positive", b: 0xC7, wantDigit: 7},
		{name: "zone D is negative", b: 0xD7, wantDigit: 7, wantNegative: true},
		{name: "zone E is positive", b: 0xE7, wantDigit: 7},
		{name: "zone F is positive", b: 0xF7, wantDigit: 7},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			digit, negative, err := signByteValue(SignEBCDIC, tc.b)
			require.NoError(t, err)
			require.Equal(t, tc.wantDigit, digit)
			require.Equal(t, tc.wantNegative, negative)

			// The lenient zones are read but never written, exactly as the
			// packed writer emits only the C, D and F nibbles.
			written, err := signByte(SignEBCDIC, tc.wantDigit, tc.wantNegative)
			require.NoError(t, err)
			if tc.wantNegative {
				require.Equal(t, byte(0xD0|tc.wantDigit), written)
			} else {
				require.Equal(t, byte(0xC0|tc.wantDigit), written)
			}

			// A digit nibble above 9 is not a digit however lenient the zone.
			_, _, err = signByteValue(SignEBCDIC, tc.b&0xF0|0x0A)
			var signErr ZonedSignError
			require.ErrorAs(t, err, &signErr)
		})
	}
}

func TestZonedSignByteRejectsUndeclaredConvention(t *testing.T) {
	t.Parallel()

	for _, sign := range []SignConvention{SignUnset, SignConvention(99)} {
		t.Run(sign.String(), func(t *testing.T) {
			t.Parallel()

			_, err := signByte(sign, 5, true)
			var encErr EncodingError
			require.ErrorAs(t, err, &encErr)
			require.Equal(t, "Sign", encErr.Field)

			_, _, err = signByteValue(sign, 0x35)
			require.ErrorAs(t, err, &encErr)
			require.Equal(t, "Sign", encErr.Field)
		})
	}
}

func TestZonedDigitValueGuard(t *testing.T) {
	t.Parallel()

	z, err := zonedBytesOf(ASCII())
	require.NoError(t, err)

	_, err = z.digitByte(10)
	require.ErrorIs(t, err, errZonedDigitValue)

	_, err = signByte(SignEBCDIC, 0x0A, false)
	require.ErrorIs(t, err, errZonedDigitValue)
}

func TestNewZonedCodecRequiresCompleteEncoding(t *testing.T) {
	t.Parallel()

	_, err := newZonedCodec(Encoding{})
	var encErr EncodingError
	require.ErrorAs(t, err, &encErr)
	require.Equal(t, "Charset", encErr.Field)

	_, err = newZonedCodec(Encoding{Charset: ASCII(), ByteOrder: binary.BigEndian, Float: FloatIEEE, Binary: BinarySize248})
	require.ErrorAs(t, err, &encErr)
	require.Equal(t, "Sign", encErr.Field)

	c, err := newZonedCodec(IBMEnterprise())
	require.NoError(t, err)
	require.Equal(t, SignEBCDIC, c.sign)
	require.Equal(t, "cp037", c.bytes.charset)
}

func TestZonedFieldRoundTrip(t *testing.T) {
	t.Parallel()

	realia := Encoding{Charset: ASCII(), Sign: SignRealia, ByteOrder: binary.LittleEndian, Float: FloatIEEE, Binary: BinarySize248}

	// signAt is the index the sign is overpunched into: the last byte under
	// SIGN IS TRAILING, the first under SIGN IS LEADING, -1 for an unsigned
	// field and for one whose sign is SEPARATE.
	testCases := []struct {
		name     string
		enc      Encoding
		digits   []byte
		signAt   int
		negative bool
		want     []byte
	}{
		{
			name:   "ebcdic unsigned",
			enc:    IBMEnterprise(),
			digits: []byte{1, 2, 3, 4, 5},
			signAt: -1,
			want:   []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xF5},
		},
		{
			name:   "ebcdic trailing positive",
			enc:    IBMEnterprise(),
			digits: []byte{1, 2, 3, 4, 5},
			signAt: 4,
			want:   []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xC5},
		},
		{
			name:     "ebcdic trailing negative",
			enc:      IBMEnterprise(),
			digits:   []byte{1, 2, 3, 4, 5},
			signAt:   4,
			negative: true,
			want:     []byte{0xF1, 0xF2, 0xF3, 0xF4, 0xD5},
		},
		{
			name:     "ebcdic leading negative",
			enc:      IBMEnterprise(),
			digits:   []byte{1, 2, 3, 4, 5},
			signAt:   0,
			negative: true,
			want:     []byte{0xD1, 0xF2, 0xF3, 0xF4, 0xF5},
		},
		{
			name:     "ebcdic negative zero is a closing brace",
			enc:      IBMEnterprise(),
			digits:   []byte{0},
			signAt:   0,
			negative: true,
			want:     []byte{0xD0},
		},
		{
			name:     "micro focus trailing negative",
			enc:      MicroFocusASCII(),
			digits:   []byte{1, 2, 3, 4, 5},
			signAt:   4,
			negative: true,
			want:     []byte{'1', '2', '3', '4', 'u'},
		},
		{
			name:   "gnucobol trailing positive",
			enc:    GnuCOBOLASCII(),
			digits: []byte{1, 2, 3, 4, 5},
			signAt: 4,
			want:   []byte{'1', '2', '3', '4', '5'},
		},
		{
			name:     "converted from ebcdic trailing negative",
			enc:      ConvertedFromEBCDIC(),
			digits:   []byte{1, 2, 3, 4, 5},
			signAt:   4,
			negative: true,
			want:     []byte{'1', '2', '3', '4', 'N'},
		},
		{
			name:   "converted from ebcdic leading positive",
			enc:    ConvertedFromEBCDIC(),
			digits: []byte{1, 2, 3, 4, 5},
			signAt: 0,
			want:   []byte{'A', '2', '3', '4', '5'},
		},
		{
			name:     "realia trailing negative",
			enc:      realia,
			digits:   []byte{1, 2, 3, 4, 5},
			signAt:   4,
			negative: true,
			want:     []byte{'1', '2', '3', '4', '%'},
		},
		{
			name:     "realia negative zero is a space",
			enc:      realia,
			digits:   []byte{9, 0},
			signAt:   1,
			negative: true,
			want:     []byte{'9', ' '},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, err := newZonedCodec(tc.enc)
			require.NoError(t, err)

			// encode -> byte-equal
			got := make([]byte, len(tc.want))
			require.NoError(t, c.encodeField(got, tc.digits, tc.signAt, tc.negative))
			require.Equal(t, tc.want, got)

			// decode -> value-equal
			ds, negative, _, err := c.decodeField(tc.want, tc.signAt)
			require.NoError(t, err)
			require.Equal(t, tc.digits, ds)
			require.Equal(t, tc.negative, negative)

			// decode -> encode -> byte-equal, the shape that catches a reader
			// whose trimming a writer does not undo.
			again := make([]byte, len(tc.want))
			require.NoError(t, c.encodeField(again, ds, tc.signAt, negative))
			require.Equal(t, tc.want, again)
		})
	}
}

func TestZonedFieldRoundTripsEverySignConvention(t *testing.T) {
	t.Parallel()

	// Every convention, every digit in the sign position, both signs, and both
	// sign positions. This is the exhaustive statement the worked fixtures
	// above only sample.
	for _, sign := range allSignConventions {
		t.Run(sign.String(), func(t *testing.T) {
			t.Parallel()

			charset := ASCII()
			if sign == SignEBCDIC {
				charset = CP037()
			}
			c, err := newZonedCodec(Encoding{
				Charset:   charset,
				Sign:      sign,
				ByteOrder: binary.BigEndian,
				Float:     FloatIEEE,
				Binary:    BinarySize248,
			})
			require.NoError(t, err)

			for d := range 10 {
				for _, negative := range []bool{false, true} {
					for _, signAt := range []int{0, 2} {
						digits := []byte{7, 8, 9}
						digits[signAt] = byte(d)

						field := make([]byte, len(digits))
						require.NoError(t, c.encodeField(field, digits, signAt, negative))

						// The sign rides in the sign-carrying byte and costs no
						// storage, so the field is exactly as wide as an
						// unsigned one.
						require.Len(t, field, len(digits))

						ds, gotNegative, _, err := c.decodeField(field, signAt)
						require.NoErrorf(t, err, "field %#v", field)
						require.Equal(t, digits, ds)
						require.Equal(t, negative, gotNegative)
					}
				}
			}
		})
	}
}

func TestZonedFieldRejectsBytesOfAnotherConvention(t *testing.T) {
	t.Parallel()

	// A field written under one convention, read under another. The digit
	// bytes agree wherever the charset does; it is the sign-carrying byte that
	// says the reader was told the wrong thing.
	for _, wrote := range allSignConventions {
		for _, reads := range allSignConventions {
			if wrote == reads {
				continue
			}
			t.Run(wrote.String()+" read as "+reads.String(), func(t *testing.T) {
				t.Parallel()

				charsetOf := func(s SignConvention) Charset {
					if s == SignEBCDIC {
						return CP037()
					}
					return ASCII()
				}
				encodingOf := func(s SignConvention) Encoding {
					return Encoding{
						Charset:   charsetOf(s),
						Sign:      s,
						ByteOrder: binary.BigEndian,
						Float:     FloatIEEE,
						Binary:    BinarySize248,
					}
				}

				writer, err := newZonedCodec(encodingOf(wrote))
				require.NoError(t, err)
				reader, err := newZonedCodec(encodingOf(reads))
				require.NoError(t, err)

				field := make([]byte, 3)
				require.NoError(t, writer.encodeField(field, []byte{1, 2, 3}, 2, true))

				_, _, at, err := reader.decodeField(field, 2)
				require.Errorf(t, err, "field %#v decoded under %s without complaint", field, reads)

				// A wrong charset is caught at the first digit byte; a wrong
				// sign convention at the sign-carrying byte. Either way the
				// index says which byte to look at.
				if charsetOf(wrote) == charsetOf(reads) {
					var signErr ZonedSignError
					require.ErrorAs(t, err, &signErr)
					require.Equal(t, field[2], signErr.Byte)
					require.Equal(t, 2, at)
				} else {
					var digitErr ZonedDigitError
					require.ErrorAs(t, err, &digitErr)
					require.Equal(t, field[0], digitErr.Byte)
					require.Equal(t, 0, at)
				}
			})
		}
	}
}

func TestZonedFieldShapeGuards(t *testing.T) {
	t.Parallel()

	c, err := newZonedCodec(MicroFocusASCII())
	require.NoError(t, err)

	// A zoned item is one byte per digit, and the overpunch index is an index
	// into the field or -1. Neither is a caller's to get wrong — the accessors
	// derive both from the digit count and the SIGN clause — so these are
	// unexported sentinels rather than typed leaves describing a file's bytes.
	require.ErrorIs(t, c.encodeField(make([]byte, 2), []byte{1, 2, 3}, -1, false), errZonedFieldWidth)
	require.ErrorIs(t, c.encodeField(make([]byte, 3), []byte{1, 2, 3}, 3, false), errZonedSignPosition)

	_, _, _, err = c.decodeField([]byte("123"), 3)
	require.ErrorIs(t, err, errZonedSignPosition)
}

func TestZonedFieldWritesNothingWhenRejected(t *testing.T) {
	t.Parallel()

	// A rejected field writes nothing, so a failure cannot leave half a field
	// behind and desynchronize the record.
	c, err := newZonedCodec(MicroFocusASCII())
	require.NoError(t, err)

	dst := []byte{0xEE, 0xEE, 0xEE}
	err = c.encodeField(dst, []byte{1, 0x0B, 3}, 2, false)
	require.ErrorIs(t, err, errZonedDigitValue)
	require.Equal(t, []byte{0xEE, 0xEE, 0xEE}, dst)
}

func TestZonedDecodingNeverTranslatesThroughTheCharset(t *testing.T) {
	t.Parallel()

	// codec/SPEC.md, "Charset as a First-Class Axis": charset translation is
	// for alphanumeric fields only. A numeric byte routed through ToUnicode
	// would lose the overpunch zone that carries the sign, so the zoned paths
	// compare byte values the charset supplied and never translate one.
	cs := &countingCharset{Charset: CP037()}
	c, err := newZonedCodec(Encoding{
		Charset:   cs,
		Sign:      SignEBCDIC,
		ByteOrder: binary.BigEndian,
		Float:     FloatHFP,
		Binary:    BinarySize248,
	})
	require.NoError(t, err)

	field := make([]byte, 3)
	require.NoError(t, c.encodeField(field, []byte{1, 2, 3}, 2, true))
	require.Equal(t, []byte{0xF1, 0xF2, 0xD3}, field)

	ds, negative, _, err := c.decodeField(field, 2)
	require.NoError(t, err)
	require.Equal(t, []byte{1, 2, 3}, ds)
	require.True(t, negative)

	require.Zero(t, cs.toUnicode.Load(), "a zoned field was translated through the charset")
}

// countingCharset counts the character translations a code path performs, so a
// test can assert that a numeric one performs none.
type countingCharset struct {
	Charset
	toUnicode atomic.Int64
}

func (c *countingCharset) ToUnicode(b byte) rune {
	c.toUnicode.Add(1)
	return c.Charset.ToUnicode(b)
}

// oddballCharset is a charset no machine ever used: its digits sit at B0-B9 and
// its signs at 01 and 02. Nothing but a caller's own table could put them
// there, which is what makes it a test that the zoned byte values come from the
// charset rather than from a hard-coded pair of code pages.
type oddballCharset struct{}

func (oddballCharset) Name() string { return "oddball" }

func (oddballCharset) ToUnicode(b byte) rune {
	switch {
	case b >= 0xB0 && b <= 0xB9:
		return rune('0' + b - 0xB0)
	case b == 0x01:
		return '+'
	case b == 0x02:
		return '-'
	}
	return rune(b)
}

func (oddballCharset) FromUnicode(r rune) (byte, bool) {
	switch {
	case r >= '0' && r <= '9':
		return byte(0xB0 + r - '0'), true
	case r == '+':
		return 0x01, true
	case r == '-':
		return 0x02, true
	}
	return 0, false
}

func (oddballCharset) Space() byte { return 0x20 }

// collapsingCharset is a charset whose FromUnicode is deliberately not
// injective. fold digits share each byte, and when signsCollide is set '+' and
// '-' share one too. Nothing in the [Charset] contract forbids either, so the
// zoned inverse mappings have to say which of the colliding characters a shared
// byte reads back as — and have to keep saying the same thing.
type collapsingCharset struct {
	fold         int
	signsCollide bool
}

func (collapsingCharset) Name() string { return "collapsing" }

func (collapsingCharset) ToUnicode(b byte) rune { return rune(b) }

func (c collapsingCharset) FromUnicode(r rune) (byte, bool) {
	switch {
	case r >= '0' && r <= '9':
		return byte(0x30 + int(r-'0')/c.fold), true
	case r == '+':
		return 0x2B, true
	case r == '-':
		if c.signsCollide {
			return 0x2B, true
		}
		return 0x2D, true
	}
	return 0, false
}

func (collapsingCharset) Space() byte { return 0x20 }

// partialCharset spells only as much as its fields admit, standing in for a
// caller's charset that cannot describe a numeric field at all.
type partialCharset struct {
	digits bool
	plus   bool
}

func (partialCharset) Name() string { return "partial" }

func (partialCharset) ToUnicode(b byte) rune { return rune(b) }

func (c partialCharset) FromUnicode(r rune) (byte, bool) {
	switch {
	case c.digits && r >= '0' && r <= '9':
		return byte(r), true
	case c.plus && r == '+':
		return byte(r), true
	}
	return 0, false
}

func (partialCharset) Space() byte { return 0x20 }

func TestOffsetErrorUnwraps(t *testing.T) {
	t.Parallel()

	err := &OffsetError{Offset: 17, Err: FieldTooLongError{Len: 9, Width: 4}}

	var tooLong FieldTooLongError
	require.ErrorAs(t, err, &tooLong)
	require.Equal(t, 9, tooLong.Len)
	require.Equal(t, 4, tooLong.Width)

	var offErr *OffsetError
	require.ErrorAs(t, error(err), &offErr)
	require.Equal(t, int64(17), offErr.Offset)
	require.Contains(t, err.Error(), "offset 17")
}

// TestPackageImportsOnlyStandardLibrary pins the promise this package makes:
// generated file libraries link codec and nothing else, never the COBOL source
// parser. Test files are exempt — testify is a test dependency, not one a
// generated library inherits.
func TestPackageImportsOnlyStandardLibrary(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	fset := token.NewFileSet()
	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		checked++

		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.ImportsOnly)
		require.NoError(t, err)

		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			// A standard library import path has no dot in its first
			// element; every other module's does, this one's included.
			first, _, _ := strings.Cut(path, "/")
			require.NotContainsf(t, first, ".", "%s imports %q, which is outside the standard library", name, path)
		}
	}
	require.NotZero(t, checked, "no non-test Go files were checked")
}

// alphaTableCharsets is every charset the derived translation table is walked
// over: the ones the package ships, the caller-supplied oddballCharset, and
// three whose whole purpose is to sit outside what a code page would do.
//
// The last three are not in alphanumericCharsets, which drives the encoder's
// round-trip walks as well as the reader's: none of them has a FromUnicode
// worth round-tripping through, and a table that decodes correctly is what is
// under test here rather than a charset a file could be written in. What they
// pin is the three shapes a rune can take that a code page never produces —
// three UTF-8 bytes, four, and no valid encoding at all.
var alphaTableCharsets = func() []struct {
	name    string
	charset Charset
} {
	table := make([]struct {
		name    string
		charset Charset
	}, 0, len(shippedCharsets)+6)
	for _, sc := range shippedCharsets {
		table = append(table, struct {
			name    string
			charset Charset
		}{name: sc.name, charset: sc.charset})
	}
	return append(table,
		struct {
			name    string
			charset Charset
		}{name: "oddball", charset: oddballCharset{}},
		struct {
			name    string
			charset Charset
		}{name: "wide", charset: wideCharset{}},
		struct {
			name    string
			charset Charset
		}{name: "invalid runes", charset: invalidRuneCharset{}},
		struct {
			name    string
			charset Charset
		}{name: "not comparable", charset: nonComparableCharset{pad: []byte{1}}},
		struct {
			name    string
			charset Charset
		}{name: "embedded interface", charset: embeddingCharset{Charset: CP037()}},
		struct {
			name    string
			charset Charset
		}{name: "map field", charset: mapCharset{alias: map[rune]rune{'A': 'Z'}}},
	)
}()

// TestAlphaTableMatchesWriteRuneForEveryByte is the equivalence the derived
// table exists to be held to: for every one of the 256 byte values of every
// charset, the packed entry must encode exactly what
// [strings.Builder.WriteRune] would have written for that byte's rune.
//
// It is stated against WriteRune rather than against utf8.AppendRune — which
// is what the table is built with — because WriteRune is the call the table
// replaced. The two agree by definition today; asserting the one the accessor
// used to make is what would catch them ceasing to.
func TestAlphaTableMatchesWriteRuneForEveryByte(t *testing.T) {
	t.Parallel()

	for _, tc := range alphaTableCharsets {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			table := newAlphaTable(tc.charset)
			wantMax := 0
			for i := range 256 {
				var sb strings.Builder
				sb.WriteRune(tc.charset.ToUnicode(byte(i)))
				want := sb.String()

				var packed [utf8MaxRuneLen]byte
				binary.LittleEndian.PutUint32(packed[:], table.enc[i])
				got := string(packed[:table.width[i]])
				require.Equalf(t, want, got, "byte %#02x of %s", i, tc.charset.Name())
				wantMax = max(wantMax, len(want))
			}
			require.Equal(t, wantMax, table.max, "max is not the widest entry")
			require.GreaterOrEqual(t, table.max, 1, "a rune encodes to at least one byte")
			require.LessOrEqual(t, table.max, utf8MaxRuneLen)
		})
	}
}

// TestAlphaTableAppendFieldMatchesWriteRune walks the table's field-level
// entry point over a field carrying all 256 byte values, which is where the
// branchless four-byte store and the width-driven advance meet.
//
// The per-byte equality above cannot catch an off-by-one in that advance: an
// entry can be right in isolation and still be laid down over its neighbour.
func TestAlphaTableAppendFieldMatchesWriteRune(t *testing.T) {
	t.Parallel()

	for _, tc := range alphaTableCharsets {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := allByteValues()

			var sb strings.Builder
			for _, b := range src {
				sb.WriteRune(tc.charset.ToUnicode(b))
			}
			want := sb.String()

			table := newAlphaTable(tc.charset)
			// A scratch reused at its widest is the state a second field
			// meets, so every width below runs into the bytes the one before
			// it left there.
			scratch := make([]byte, table.fieldCap(len(src)))
			got := table.translate(scratch, src)
			require.Equal(t, want, string(got))
			require.Equal(t, len(got), cap(got), "the translated prefix was not capped")

			// Every width the table might see, so the reserve is exercised at
			// the boundaries rather than only at 256.
			for _, n := range []int{0, 1, 2, 3, 255} {
				var head strings.Builder
				for _, b := range src[:n] {
					head.WriteRune(tc.charset.ToUnicode(b))
				}
				require.Equalf(t, head.String(), string(table.translate(scratch[:table.fieldCap(n)], src[:n])),
					"field width %d", n)
			}
		})
	}
}

// TestAlphaCacheSharesOneTablePerCharset pins the cache's whole reason for
// existing: the table outlives any one [Reader], because [Unmarshal] builds a
// Reader per record and a table built per Reader would be amortised over a
// single one.
//
// It runs against a cache of its own rather than against [alphaTables], so it
// asserts nothing about global state. Through the package-level cache the same
// assertion is order dependent — every distinct pointer-typed charset any test
// in the suite reads with is a permanent entry, and past alphaTablesMax the
// sharing property legitimately stops holding — which would make this fail for
// reasons that have nothing to do with what it checks.
func TestAlphaCacheSharesOneTablePerCharset(t *testing.T) {
	t.Parallel()

	c := &alphaCache{max: alphaTablesMax}
	require.Same(t, c.tableOf(ASCII()), c.tableOf(ASCII()))
	require.Same(t, c.tableOf(CP037()), c.tableOf(CP037()))
	require.NotSame(t, c.tableOf(ASCII()), c.tableOf(CP037()))
	require.Equal(t, int64(2), c.len.Load())
}

// TestAlphaCacheKeepsAnsweringWhenFull walks the bound. A cached entry keeps its
// charset reachable for the life of the program, so the cache has a ceiling —
// and the branch past that ceiling has to keep returning correct tables rather
// than nil or a shared wrong one.
//
// A cache of its own is what makes this reachable at all: through
// [alphaTables] the branch needs 64 distinct charsets and leaves the process
// cache full for every test after it.
func TestAlphaCacheKeepsAnsweringWhenFull(t *testing.T) {
	t.Parallel()

	c := &alphaCache{max: 2}
	first := c.tableOf(ASCII())
	require.Same(t, first, c.tableOf(ASCII()))
	require.NotNil(t, c.tableOf(CP037()))
	require.Equal(t, int64(2), c.len.Load())

	// Full. A charset first seen now gets a correct table that is simply not
	// retained, so two lookups are two tables and both translate properly.
	third := c.tableOf(oddballCharset{})
	fourth := c.tableOf(oddballCharset{})
	require.NotNil(t, third)
	require.NotSame(t, third, fourth, "the cache stored past its bound")
	require.Equal(t, newAlphaTable(oddballCharset{}).enc, third.enc)
	require.Equal(t, int64(2), c.len.Load())

	// And the entries that were already in keep being shared.
	require.Same(t, first, c.tableOf(ASCII()))
}

// TestAlphaTableOfRefusesACharsetThatCannotBeAKey is the hazard of keying a
// cache by an interface. [Charset] promises nothing about comparability, so a
// caller whose charset carries a slice, a map or a function would panic the map
// lookup rather than any code of their own — and inside sync.Map's store path
// that panic would escape with its mutex held, so it cannot be recovered from
// either.
//
// The answer is nil, which [Reader.ReadAlphanumericJustified] reads as "no
// table, translate per byte". An *uncached table* was the first answer and is
// worse than none: it costs 256 translations and a 1.5KiB allocation per
// [Reader], and Unmarshal builds one per record.
func TestAlphaTableOfRefusesACharsetThatCannotBeAKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		charset Charset
		want    bool
	}{
		{name: "slice field", charset: nonComparableCharset{pad: []byte{1}}},
		{name: "embedded interface", charset: embeddingCharset{Charset: ASCII()}},
		{name: "map field", charset: mapCharset{}},
		{name: "empty struct", charset: ASCII(), want: true},
		{name: "pointer to an embedded interface", charset: &embeddingCharset{Charset: ASCII()}, want: true},
		{name: "pointer to a slice field", charset: &nonComparableCharset{pad: []byte{1}}, want: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// reflect.Value.Comparable is the exact answer and is what the
			// type walk has to agree with wherever it says yes. Where it
			// says no the walk may be stricter, which is the safe
			// direction; the cases above state which is which.
			exact := reflect.ValueOf(tc.charset).Comparable()
			require.Equal(t, tc.want, comparableCharset(tc.charset))
			if comparableCharset(tc.charset) {
				require.True(t, exact, "the walk approved a value == would panic on")
			}

			c := &alphaCache{max: alphaTablesMax}
			var got *alphaTable
			require.NotPanics(t, func() { got = c.tableOf(tc.charset) })
			if !tc.want {
				require.Nil(t, got, "a charset that cannot be a key got a table")
				require.Zero(t, c.len.Load())
				return
			}
			require.NotNil(t, got)
		})
	}
}

// TestUnmarshalDerivesTheTableOncePerCharset is the assertion the cache exists
// for, and the one that fails if the table ever goes back to being per-[Reader].
//
// [Unmarshal] builds a Reader per record, so a per-Reader table would translate
// 256 byte values *per record*: the count below would be 256 times the record
// count rather than 256 in total. Every other test of the table would pass
// either way, because they all read through one Reader and so through the
// per-Reader memo rather than through the cache.
func TestUnmarshalDerivesTheTableOncePerCharset(t *testing.T) {
	t.Parallel()

	// A pointer, so it is comparable and therefore cacheable, and distinct
	// from every other charset the suite uses.
	cs := &countingCharset{Charset: CP037()}
	enc := charsetEncoding(cs)

	// Two alphanumeric fields, because a record with one would still pass if
	// the table were resolved per *field* rather than per Reader.
	data := []byte{0xC1, 0xC2, 0xC3, 0xF1, 0xF2}

	const records = 20
	for range records {
		var rec twoTextFields
		require.NoError(t, Unmarshal(enc, data, &rec))
		require.Equal(t, "ABC", rec.first)
		require.Equal(t, "12", rec.second)
	}
	require.Equal(t, int64(256), cs.toUnicode.Load(),
		"the table was derived more than once across %d records", records)
}

// twoTextFields is the smallest record that exercises the per-record path: two
// alphanumeric fields and nothing else, so the only translation is theirs.
type twoTextFields struct {
	first, second string
}

func (r *twoTextFields) UnmarshalCOBOL(rd *Reader) error {
	var err error
	if r.first, err = rd.ReadAlphanumeric(3); err != nil {
		return err
	}
	r.second, err = rd.ReadAlphanumeric(2)
	return err
}

// wideCharset spells runes no code page would: three UTF-8 bytes for one half
// of the byte space and four for the other. It exists because both shipped
// charsets top out at two, so nothing else in the suite reaches the wide
// entries of the packed table or the reserve that sizes a field for them.
type wideCharset struct{}

func (wideCharset) Name() string { return "wide" }

func (wideCharset) ToUnicode(b byte) rune {
	switch {
	case b == 0x20:
		return ' '
	case b < 0x80:
		// Three bytes: the CJK block, well above the two-byte ceiling.
		return rune(0x4E00 + int(b))
	default:
		// Four bytes: a supplementary plane, the widest UTF-8 gets.
		return rune(0x1F000 + int(b))
	}
}

func (wideCharset) FromUnicode(r rune) (byte, bool) {
	switch {
	case r == ' ':
		return 0x20, true
	case r >= 0x4E00 && r < 0x4E80:
		return byte(r - 0x4E00), true
	case r >= 0x1F080 && r <= 0x1F0FF:
		return byte(r - 0x1F000), true
	}
	return 0, false
}

func (wideCharset) Space() byte { return 0x20 }

// invalidRuneCharset spells values that are not characters at all: a negative
// one, a surrogate half, and one above [utf8.MaxRune].
//
// Nothing in the [Charset] contract forbids them — ToUnicode owes totality and
// nothing else — and [strings.Builder.WriteRune] answers each with U+FFFD. The
// table has to answer identically, which is a property of building it with
// [utf8.EncodeRune] rather than one this package implements.
type invalidRuneCharset struct{}

func (invalidRuneCharset) Name() string { return "invalid-runes" }

func (invalidRuneCharset) ToUnicode(b byte) rune {
	switch b {
	case 0x01:
		return -1
	case 0x02:
		return 0xD800 // a leading surrogate, unencodable on its own
	case 0x03:
		return 0xDFFF // a trailing surrogate
	case 0x04:
		return utf8.MaxRune + 1
	case 0x05:
		return rune(0x7FFFFFFF)
	}
	return rune(b)
}

func (invalidRuneCharset) FromUnicode(r rune) (byte, bool) {
	if r < 0 || r > 0xFF {
		return 0, false
	}
	return byte(r), true
}

func (invalidRuneCharset) Space() byte { return 0x20 }

// nonComparableCharset is a charset that cannot be a map key: pad is a slice,
// so == on two of them panics. It is [ASCII] in every other respect.
//
// A caller's charset holding a slice is not far-fetched — a table read from a
// file, or a fallback list — and nothing in the [Charset] documentation warns
// against one, which is why the cache has to cope rather than the caller.
type nonComparableCharset struct {
	pad []byte
}

func (nonComparableCharset) Name() string { return "non-comparable" }

func (nonComparableCharset) ToUnicode(b byte) rune { return rune(b) }

func (nonComparableCharset) FromUnicode(r rune) (byte, bool) {
	if r < 0 || r > 0xFF {
		return 0, false
	}
	return byte(r), true
}

func (nonComparableCharset) Space() byte { return 0x20 }

// embeddingCharset is the decorator shape a caller reaches for first: embed
// [Charset], override what differs. Embedding an *interface* is what makes it
// not comparable, which is worth a fixture of its own because nothing about the
// shape looks like it carries a slice — and because the pointer to it is
// comparable, so the remedy is one character.
type embeddingCharset struct {
	Charset
}

func (embeddingCharset) Name() string { return "embedding" }

// mapCharset carries a map, the other way a struct stops being comparable.
type mapCharset struct {
	alias map[rune]rune
}

func (mapCharset) Name() string { return "map" }

func (c mapCharset) ToUnicode(b byte) rune {
	if r, ok := c.alias[rune(b)]; ok {
		return r
	}
	return rune(b)
}

func (mapCharset) FromUnicode(r rune) (byte, bool) {
	if r < 0 || r > 0xFF {
		return 0, false
	}
	return byte(r), true
}

func (mapCharset) Space() byte { return 0x20 }
