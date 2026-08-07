// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

// testRecord is a four-field record exercising every accessor the package
// currently has: two left-justified alphanumeric fields, a JUSTIFIED RIGHT one,
// and a raw byte field. It stands in for the code a generator will emit.
type testRecord struct {
	ID   string
	Name string
	Code string
	Raw  []byte
}

const (
	testRecordIDWidth   = 6
	testRecordNameWidth = 12
	testRecordCodeWidth = 4
	testRecordRawWidth  = 3
)

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
	return w.WriteBytes(r.Raw)
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
	r.Raw, err = rd.ReadBytes(testRecordRawWidth)
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

		// "A12345" | "WIDGET GRIP " | "  42" | 00 01 FF
		data := []byte("A12345WIDGET GRIP   42\x00\x01\xFF")
		require.Len(t, data, testRecordIDWidth+testRecordNameWidth+testRecordCodeWidth+testRecordRawWidth)

		var got testRecord
		require.NoError(t, Unmarshal(GnuCOBOLASCII(), data, &got))
		require.Equal(t, testRecord{
			ID:   "A12345",
			Name: "WIDGET GRIP",
			Code: "42",
			Raw:  []byte{0x00, 0x01, 0xFF},
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
