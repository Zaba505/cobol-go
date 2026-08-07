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
			ID:   "A12345",
			Name: "WIDGET GRIP",
			Code: "42",
			Raw:  []byte{0x00, 0x01, 0xFF},
		})
		require.NoError(t, err)
		require.Equal(t, []byte("A12345WIDGET GRIP   42\x00\x01\xFF"), got)
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
			src:  []byte("A12345WIDGET GRIP   42\x00\x01\xFF"),
		},
		{
			name: "ascii record with empty fields",
			enc:  MicroFocusASCII(),
			src:  []byte("      " + "            " + "    " + "\x00\x00\x00"),
		},
		{
			name: "ascii record with full fields",
			enc:  ConvertedFromEBCDIC(),
			src:  []byte("A12345WIDGETGRIP123456\xFF\xFE\xFD"),
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
				0x00, 0x01, 0xFF, // raw payload
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
				ID:   "A12345",
				Name: "WIDGET GRIP",
				Code: "42",
				Raw:  []byte{0x00, 0x01, 0xFF},
			},
		},
		{
			name: "ebcdic",
			enc:  IBMEnterprise(),
			rec: testRecord{
				ID:   "A12345",
				Name: "WIDGET GRIP",
				Code: "42",
				Raw:  []byte{0x00, 0x01, 0xFF},
			},
		},
		{
			name: "converted from ebcdic",
			enc:  ConvertedFromEBCDIC(),
			rec: testRecord{
				ID:   "B0",
				Name: "",
				Code: "",
				Raw:  []byte{0x40, 0x20, 0x00},
			},
		},
		{
			name: "fields filling their width exactly",
			enc:  MicroFocusASCII(),
			rec: testRecord{
				ID:   "A12345",
				Name: "WIDGETGRIP12",
				Code: "3456",
				Raw:  []byte{0xFF, 0xFE, 0xFD},
			},
		},
		{
			name: "interior spaces survive",
			enc:  GnuCOBOLASCII(),
			rec: testRecord{
				ID:   "A B C",
				Name: "W  I  D",
				Code: "4 2",
				Raw:  []byte{0x01, 0x02, 0x03},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := Marshal(tc.enc, &tc.rec)
			require.NoError(t, err)
			require.Len(t, data, testRecordIDWidth+testRecordNameWidth+testRecordCodeWidth+testRecordRawWidth)

			var got testRecord
			require.NoError(t, Unmarshal(tc.enc, data, &got))
			require.Equal(t, tc.rec, got)
		})
	}
}
