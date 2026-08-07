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
	"strings"
	"testing"

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

func TestCharsetIsTotalAndBijective(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		charset   Charset
		wantName  string
		wantSpace byte
	}{
		{name: "ascii", charset: ASCII(), wantName: "ASCII", wantSpace: 0x20},
		{name: "cp037", charset: CP037(), wantName: "cp037", wantSpace: 0x40},
	}

	for _, tc := range testCases {
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
