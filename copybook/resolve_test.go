// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// odoSrc is the record the resolution tests are measured against: a two-byte
// count, a three-byte table of one to five occurrences, and a trailer that moves
// with the table.
const odoSrc = `01 R.
   05 N PIC 9(2).
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
   05 TAIL PIC X(4).
`

// asciiRecord builds a record whose first two bytes are the ASCII zoned decimal
// spelling of n, padded out past the longest layout the tests use.
func asciiRecord(n int) []byte {
	return append([]byte(fmt.Sprintf("%02d", n)), make([]byte, 32)...)
}

func TestNewLayoutVariable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		src       string
		want      []span
		minLength int
		maxLength int
	}{
		{
			name: "occurs depending on lays the table out at its maximum",
			src:  odoSrc,
			want: []span{
				{name: "R", offset: 0, length: 21},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 3, occurs: 5,
					minOccurs: 1, maxOccurs: 5, dependsOn: "N",
				},
				{name: "TAIL", offset: 17, length: 4},
			},
			minLength: 9,
			maxLength: 21,
		},
		{
			name: "a single occurrence count is the maximum of a depending on table",
			src: `01 R.
   05 N PIC 9(2).
   05 A PIC X(3) OCCURS 4 TIMES DEPENDING ON N.
`,
			want: []span{
				{name: "R", offset: 0, length: 14},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 3, occurs: 4,
					minOccurs: 1, maxOccurs: 4, dependsOn: "N",
				},
			},
			minLength: 5,
			maxLength: 14,
		},
		{
			name: "a group table depends on a field of an earlier group",
			src: `01 R.
   05 HDR.
      10 N PIC 9(3).
   05 T OCCURS 1 TO 3 TIMES DEPENDING ON N.
      10 CODE PIC X(2).
      10 QTY PIC 9(4) COMP-3.
`,
			want: []span{
				{name: "R", offset: 0, length: 18},
				{name: "HDR", offset: 0, length: 3},
				{name: "N", offset: 0, length: 3},
				{
					name: "T", offset: 3, length: 5, occurs: 3,
					minOccurs: 1, maxOccurs: 3, dependsOn: "N",
				},
				{name: "CODE", offset: 3, length: 2},
				{name: "QTY", offset: 5, length: 3},
			},
			minLength: 8,
			maxLength: 18,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := layoutOf(t, tc.src, IBMEnterprise())
			require.True(t, l.Variable, "the record is variable-length")
			require.Zero(t, l.Length, "a variable-length record advertises no fixed length")
			require.Equal(t, tc.minLength, l.MinLength, "minimum record length")
			require.Equal(t, tc.maxLength, l.MaxLength, "maximum record length")
			requireSpans(t, l, tc.want)
		})
	}
}

func TestNewLayoutResolvesTheControllingField(t *testing.T) {
	t.Parallel()

	l := layoutOf(t, odoSrc, IBMEnterprise())

	a, n := l.Find("A"), l.Find("N")
	require.NotNil(t, a)
	require.NotNil(t, n)
	require.Same(t, n, a.DependingOn, "the table's count comes from N's item")
	require.Same(t, n.Field, a.DependingOn.Field, "the table's count comes from N's field")
	require.Nil(t, n.DependingOn, "an ordinary item depends on nothing")
}

func TestLayoutResolve(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		src    string
		data   []byte
		want   []span
		length int
	}{
		{
			name: "the minimum occurrence count",
			src:  odoSrc,
			data: asciiRecord(1),
			want: []span{
				{name: "R", offset: 0, length: 9},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 3, occurs: 1,
					minOccurs: 1, maxOccurs: 5, dependsOn: "N",
				},
				{name: "TAIL", offset: 5, length: 4},
			},
			length: 9,
		},
		{
			name: "a mid-range occurrence count",
			src:  odoSrc,
			data: asciiRecord(3),
			want: []span{
				{name: "R", offset: 0, length: 15},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 3, occurs: 3,
					minOccurs: 1, maxOccurs: 5, dependsOn: "N",
				},
				{name: "TAIL", offset: 11, length: 4},
			},
			length: 15,
		},
		{
			name: "the maximum occurrence count",
			src:  odoSrc,
			data: asciiRecord(5),
			want: []span{
				{name: "R", offset: 0, length: 21},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 3, occurs: 5,
					minOccurs: 1, maxOccurs: 5, dependsOn: "N",
				},
				{name: "TAIL", offset: 17, length: 4},
			},
			length: 21,
		},
		{
			name: "an EBCDIC controlling field reads the same as an ASCII one",
			src:  odoSrc,
			data: append([]byte{0xF0, 0xF4}, make([]byte, 32)...),
			want: []span{
				{name: "R", offset: 0, length: 18},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 3, occurs: 4,
					minOccurs: 1, maxOccurs: 5, dependsOn: "N",
				},
				{name: "TAIL", offset: 14, length: 4},
			},
			length: 18,
		},
		{
			name: "a signed controlling field carries its sign in the last digit byte",
			src: `01 R.
   05 N PIC S9(2).
   05 A PIC X(2) OCCURS 1 TO 20 TIMES DEPENDING ON N.
`,
			// EBCDIC F1 C2: a positive 12, the sign overpunched
			// into the zone of the last digit.
			data: append([]byte{0xF1, 0xC2}, make([]byte, 32)...),
			want: []span{
				{name: "R", offset: 0, length: 26},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 2, occurs: 12,
					minOccurs: 1, maxOccurs: 20, dependsOn: "N",
				},
			},
			length: 26,
		},
		{
			name: "a leading sign is overpunched into the first digit byte",
			src: `01 R.
   05 N PIC S9(2) SIGN LEADING.
   05 A PIC X(2) OCCURS 1 TO 20 TIMES DEPENDING ON N.
`,
			// EBCDIC C1 F2: a positive 12, the sign overpunched
			// into the zone of the *first* digit.
			data: append([]byte{0xC1, 0xF2}, make([]byte, 32)...),
			want: []span{
				{name: "R", offset: 0, length: 26},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 2, occurs: 12,
					minOccurs: 1, maxOccurs: 20, dependsOn: "N",
				},
			},
			length: 26,
		},
		{
			name: "a separate sign byte is not a digit of the count",
			src: `01 R.
   05 N PIC S9(2) SIGN LEADING SEPARATE.
   05 A PIC X(2) OCCURS 1 TO 9 TIMES DEPENDING ON N.
`,
			data: append([]byte("+03"), make([]byte, 32)...),
			want: []span{
				{name: "R", offset: 0, length: 9},
				{name: "N", offset: 0, length: 3},
				{
					name: "A", offset: 3, length: 2, occurs: 3,
					minOccurs: 1, maxOccurs: 9, dependsOn: "N",
				},
			},
			length: 9,
		},
		{
			name: "a packed decimal controlling field is nibbles rather than characters",
			src: `01 R.
   05 N PIC 9(3) COMP-3.
   05 A PIC X(4) OCCURS 1 TO 9 TIMES DEPENDING ON N.
   05 TAIL PIC X.
`,
			data: append([]byte{0x00, 0x2F}, make([]byte, 32)...),
			want: []span{
				{name: "R", offset: 0, length: 11},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 4, occurs: 2,
					minOccurs: 1, maxOccurs: 9, dependsOn: "N",
				},
				{name: "TAIL", offset: 10, length: 1},
			},
			length: 11,
		},
		{
			name: "an even packed digit count pads its leading nibble",
			src: `01 R.
   05 N PIC 9(4) COMP-3.
   05 A PIC X(2) OCCURS 1 TO 9 TIMES DEPENDING ON N.
`,
			// PIC 9(4) COMP-3 is three bytes: a pad nibble, four
			// digit nibbles and a sign nibble.
			data: append([]byte{0x00, 0x00, 0x3F}, make([]byte, 32)...),
			want: []span{
				{name: "R", offset: 0, length: 9},
				{name: "N", offset: 0, length: 3},
				{
					name: "A", offset: 3, length: 2, occurs: 3,
					minOccurs: 1, maxOccurs: 9, dependsOn: "N",
				},
			},
			length: 9,
		},
		{
			name: "a comp-6 controlling field carries no sign nibble",
			src: `01 R.
   05 N PIC 9(3) COMP-6.
   05 A PIC X(4) OCCURS 1 TO 9 TIMES DEPENDING ON N.
   05 TAIL PIC X.
`,
			// PIC 9(3) COMP-6 is two bytes: one pad nibble and three
			// digit nibbles. packedValue reads the same two bytes as
			// a three-digit 000 whose sign nibble is 2 — no sign at
			// all, so it refuses a record COMP-6 reads fine. Hence
			// its own arm in readCount.
			data: append([]byte{0x00, 0x02}, make([]byte, 32)...),
			want: []span{
				{name: "R", offset: 0, length: 11},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 4, occurs: 2,
					minOccurs: 1, maxOccurs: 9, dependsOn: "N",
				},
				{name: "TAIL", offset: 10, length: 1},
			},
			length: 11,
		},
		{
			name: "a comp-6 count is its nibbles most significant first",
			src: `01 R.
   05 N PIC 9(3) COMP-6.
   05 A PIC X(2) OCCURS 1 TO 12 TIMES DEPENDING ON N.
`,
			// Nibbles 0, 1, 2 read as 12 and not as 2 or 21: the one
			// count in this file whose value needs every nibble in
			// the right order and place.
			data: append([]byte{0x00, 0x12}, make([]byte, 32)...),
			want: []span{
				{name: "R", offset: 0, length: 26},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 2, occurs: 12,
					minOccurs: 1, maxOccurs: 12, dependsOn: "N",
				},
			},
			length: 26,
		},
		{
			name: "an even comp-6 digit count fills its bytes exactly",
			src: `01 R.
   05 N PIC 9(4) COMP-6.
   05 A PIC X(2) OCCURS 1 TO 9 TIMES DEPENDING ON N.
`,
			// PIC 9(4) COMP-6 is two bytes of four digit nibbles and
			// no pad, where PIC 9(4) COMP-3 is three.
			data: append([]byte{0x00, 0x03}, make([]byte, 32)...),
			want: []span{
				{name: "R", offset: 0, length: 8},
				{name: "N", offset: 0, length: 2},
				{
					name: "A", offset: 2, length: 2, occurs: 3,
					minOccurs: 1, maxOccurs: 9, dependsOn: "N",
				},
			},
			length: 8,
		},
		{
			name: "the fields of a resolved group table sit under every occurrence",
			src: `01 R.
   05 N PIC 9(2).
   05 T OCCURS 1 TO 4 TIMES DEPENDING ON N.
      10 CODE PIC X(2).
      10 QTY PIC 9(4) COMP-3.
   05 TAIL PIC X(2).
`,
			data: asciiRecord(2),
			want: []span{
				{name: "R", offset: 0, length: 14},
				{name: "N", offset: 0, length: 2},
				{
					name: "T", offset: 2, length: 5, occurs: 2,
					minOccurs: 1, maxOccurs: 4, dependsOn: "N",
				},
				{name: "CODE", offset: 2, length: 2},
				{name: "QTY", offset: 4, length: 3},
				{name: "TAIL", offset: 12, length: 2},
			},
			length: 14,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := layoutOf(t, tc.src, IBMEnterprise())
			require.True(t, l.Variable)

			got, err := l.Resolve(tc.data)
			require.NoError(t, err)
			require.False(t, got.Variable, "a resolved layout is no longer variable")
			require.Equal(t, tc.length, got.Length, "resolved record length")
			require.Equal(t, tc.length, got.MinLength, "a resolved layout has one length")
			require.Equal(t, tc.length, got.MaxLength, "a resolved layout has one length")
			require.Equal(t, l.Dialect, got.Dialect)
			requireSpans(t, got, tc.want)
		})
	}
}

// TestLayoutResolveEmptyTable covers OCCURS 0 TO n, whose table may resolve to no
// occurrences at all and so contribute nothing to the record. Its items are
// asserted by hand rather than as spans, because a span's zero occurrence count
// is the "unset, default to one" of the table above and cannot say zero.
func TestLayoutResolveEmptyTable(t *testing.T) {
	t.Parallel()

	l := layoutOf(t, `01 R.
   05 N PIC 9(2).
   05 A PIC X(3) OCCURS 0 TO 2 TIMES DEPENDING ON N.
   05 TAIL PIC X.
`, IBMEnterprise())
	require.True(t, l.Variable)
	require.Equal(t, 3, l.MinLength, "the shortest record holds no occurrences")
	require.Equal(t, 9, l.MaxLength)
	require.Equal(t, 0, l.Find("A").MinOccurs, "the table may occur no times")

	got, err := l.Resolve(asciiRecord(0))
	require.NoError(t, err)
	require.Equal(t, 3, got.Length, "an empty table contributes no bytes")

	a := got.Find("A")
	require.Equal(t, 0, a.Occurs, "the table resolved to no occurrences")
	require.Equal(t, 3, a.Length, "one occurrence would still be three bytes wide")
	require.Equal(t, 0, a.Total(), "no occurrences occupy no storage")
	require.Equal(t, 2, got.Find("TAIL").Offset, "the trailer sits where the table starts")

	_, err = a.OccurrenceOffset(0)
	require.ErrorAs(t, err, &OccurrenceError{}, "an empty table has no occurrence to offset")
}

func TestLayoutResolveOccurrenceOffsets(t *testing.T) {
	t.Parallel()

	l := layoutOf(t, odoSrc, IBMEnterprise())
	got, err := l.Resolve(asciiRecord(3))
	require.NoError(t, err)

	a := got.Find("A")
	require.NotNil(t, a)
	for i, want := range []int{2, 5, 8} {
		off, err := a.OccurrenceOffset(i)
		require.NoError(t, err)
		require.Equal(t, want, off, "occurrence %d", i)
	}

	_, err = a.OccurrenceOffset(3)
	require.ErrorAs(t, err, &OccurrenceError{}, "an occurrence past the resolved count is out of range")
}

func TestLayoutResolveFixedRecord(t *testing.T) {
	t.Parallel()

	l := layoutOf(t, `01 R.
   05 A PIC X(3).
   05 B PIC 9(4) COMP-3.
`, IBMEnterprise())
	require.False(t, l.Variable)
	require.Equal(t, 6, l.Length)
	require.Equal(t, l.Length, l.MinLength)
	require.Equal(t, l.Length, l.MaxLength)

	// A record with no variable table is already resolved, so no bytes are
	// read and the layout in hand is the answer.
	got, err := l.Resolve(nil)
	require.NoError(t, err)
	require.Same(t, l, got)

	got, err = l.ResolveCounts(nil)
	require.NoError(t, err)
	require.Same(t, l, got)
}

func TestLayoutResolveCounts(t *testing.T) {
	t.Parallel()

	// A binary controlling field is the case Resolve refuses: its bytes mean
	// different numbers under different byte orders, so the count is the
	// caller's to read.
	l := layoutOf(t, `01 R.
   05 N PIC 9(4) COMP.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
   05 TAIL PIC X(2).
`, IBMEnterprise())
	require.True(t, l.Variable)

	got, err := l.ResolveCounts(map[*Field]int{l.Find("N").Field: 4})
	require.NoError(t, err)
	require.False(t, got.Variable)
	require.Equal(t, 16, got.Length)
	requireSpans(t, got, []span{
		{name: "R", offset: 0, length: 16},
		{name: "N", offset: 0, length: 2},
		{
			name: "A", offset: 2, length: 3, occurs: 4,
			minOccurs: 1, maxOccurs: 5, dependsOn: "N",
		},
		{name: "TAIL", offset: 14, length: 2},
	})
}

func TestLayoutResolveErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      string
		data     []byte
		counts   map[string]int
		target   any
		contains string
	}{
		{
			name: "the controlling field must be defined before the table",
			src: `01 R.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
   05 N PIC 9(2).
`,
			data:     asciiRecord(2),
			target:   &DependingError{},
			contains: `item "N" is defined after the table it controls`,
		},
		{
			name: "the controlling field must be defined at all",
			src: `01 R.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON NOWHERE.
`,
			data:     asciiRecord(2),
			target:   &DependingError{},
			contains: `no item named "NOWHERE" is defined before the table it controls`,
		},
		{
			name: "the controlling field may not be a group item",
			src: `01 R.
   05 N.
      10 M PIC 9(2).
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     asciiRecord(2),
			target:   &DependingError{},
			contains: "is a group item rather than an integer",
		},
		{
			name: "the controlling field must be numeric",
			src: `01 R.
   05 N PIC X(2).
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     asciiRecord(2),
			target:   &DependingError{},
			contains: "alphanumeric rather than numeric",
		},
		{
			name: "the controlling field must have a picture to be an integer",
			src: `01 R.
   05 N COMP-1.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     asciiRecord(2),
			target:   &DependingError{},
			contains: "has no PICTURE clause and so no integer value",
		},
		{
			name: "the controlling field must be an integer",
			src: `01 R.
   05 N PIC 9V9.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     asciiRecord(2),
			target:   &DependingError{},
			contains: "is not an integer",
		},
		{
			name: "a range that ends below where it starts is no range",
			src: `01 R.
   05 N PIC 9(2).
   05 A PIC X(3) OCCURS 5 TO 3 TIMES DEPENDING ON N.
`,
			data:     asciiRecord(4),
			target:   &OccursError{},
			contains: "ends below where it starts",
		},
		{
			name: "a single occurrence count of zero admits no table",
			src: `01 R.
   05 N PIC 9(2).
   05 A PIC X(3) OCCURS 0 TIMES DEPENDING ON N.
`,
			data:     asciiRecord(0),
			target:   &OccursError{},
			contains: "admits no occurrences at all",
		},
		{
			name:     "a count above the declared maximum is out of range",
			src:      odoSrc,
			data:     asciiRecord(6),
			target:   &DependingError{},
			contains: "occurrence count 6 is outside the 1 to 5",
		},
		{
			name: "a count below the declared minimum is out of range",
			src: `01 R.
   05 N PIC 9(2).
   05 A PIC X(3) OCCURS 2 TO 5 TIMES DEPENDING ON N.
`,
			data:     asciiRecord(1),
			target:   &DependingError{},
			contains: "occurrence count 1 is outside the 2 to 5",
		},
		{
			name:     "the record must reach the controlling field",
			src:      odoSrc,
			data:     []byte{0x30},
			target:   &DependingError{},
			contains: "ends at byte 2 of a record only 1 bytes long",
		},
		{
			name:     "an empty record reaches no controlling field",
			src:      odoSrc,
			data:     []byte{},
			target:   &DependingError{},
			contains: "ends at byte 2 of a record only 0 bytes long",
		},
		{
			name:     "a controlling field holding spaces is not a count",
			src:      odoSrc,
			data:     append([]byte("  "), make([]byte, 32)...),
			target:   &DependingError{},
			contains: "which is no unsigned digit",
		},
		{
			name: "a negative controlling field is not a count",
			src: `01 R.
   05 N PIC S9(2).
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			// EBCDIC F0 D2: a negative 2.
			data:     append([]byte{0xF0, 0xD2}, make([]byte, 32)...),
			target:   &DependingError{},
			contains: "which is no unsigned digit",
		},
		{
			name: "a negative packed controlling field is not a count",
			src: `01 R.
   05 N PIC S9(3) COMP-3.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     append([]byte{0x00, 0x2D}, make([]byte, 32)...),
			target:   &DependingError{},
			contains: "whose sign nibble is negative",
		},
		{
			name: "a negative separate sign is not a count",
			src: `01 R.
   05 N PIC S9(2) SIGN LEADING SEPARATE.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     append([]byte("-03"), make([]byte, 32)...),
			target:   &DependingError{},
			contains: "byte 0 is 0x2d, a negative sign",
		},
		{
			name: "a trailing separate sign is checked at its own end",
			src: `01 R.
   05 N PIC S9(2) SIGN TRAILING SEPARATE.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			// EBCDIC 60 is the '-' of a separate sign.
			data:     append([]byte{0xF0, 0xF3, 0x60}, make([]byte, 32)...),
			target:   &DependingError{},
			contains: "byte 2 is 0x60, a negative sign",
		},
		{
			name: "a separate sign byte that is neither plus nor minus is not a count",
			src: `01 R.
   05 N PIC S9(2) SIGN LEADING SEPARATE.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     append([]byte("x03"), make([]byte, 32)...),
			target:   &DependingError{},
			contains: "which is no separate sign",
		},
		{
			name: "a leading overpunched sign is not accepted at the other end",
			src: `01 R.
   05 N PIC S9(2) SIGN LEADING.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			// F0 C3 overpunches the last byte, which is where a
			// TRAILING sign goes and not a LEADING one.
			data:     append([]byte{0xF0, 0xC3}, make([]byte, 32)...),
			target:   &DependingError{},
			contains: "which is no unsigned digit",
		},
		{
			name: "a packed pad nibble holding a digit is not a count",
			src: `01 R.
   05 N PIC 9(4) COMP-3.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     append([]byte{0x10, 0x00, 0x3F}, make([]byte, 32)...),
			target:   &DependingError{},
			contains: "whose high nibble pads a 4-digit value and is not zero",
		},
		{
			name: "a packed controlling field holding no sign nibble is not a count",
			src: `01 R.
   05 N PIC 9(3) COMP-3.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     append([]byte{0x00, 0x23}, make([]byte, 32)...),
			target:   &DependingError{},
			contains: "whose low nibble is no sign",
		},
		{
			name: "a comp-6 pad nibble holding a digit is not a count",
			src: `01 R.
   05 N PIC 9(3) COMP-6.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     append([]byte{0x10, 0x02}, make([]byte, 32)...),
			target:   &DependingError{},
			contains: "whose high nibble pads a 3-digit value and is not zero",
		},
		{
			name: "a comp-6 sign nibble is a digit position and must hold a digit",
			src: `01 R.
   05 N PIC 9(4) COMP-6.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			// C is the positive sign nibble COMP-3 would accept
			// here; under COMP-6 the last nibble is the last digit.
			data:     append([]byte{0x00, 0x2C}, make([]byte, 32)...),
			target:   &DependingError{},
			contains: "whose low nibble is no digit",
		},
		{
			name: "a binary controlling field is not read without a byte order",
			src: `01 R.
   05 N PIC 9(4) COMP.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			data:     append([]byte{0x00, 0x02}, make([]byte, 32)...),
			target:   &DependingError{},
			contains: "whose value depends on the byte order of the file it came from",
		},
		{
			name:     "a count must be supplied for every controlling field",
			src:      odoSrc,
			counts:   map[string]int{},
			target:   &DependingError{},
			contains: `no occurrence count was supplied for controlling item "N"`,
		},
		{
			name:     "a supplied count is held to the declared range too",
			src:      odoSrc,
			counts:   map[string]int{"N": 9},
			target:   &DependingError{},
			contains: "occurrence count 9 is outside the 1 to 5",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recs := records(t, tc.src)
			require.NotEmpty(t, recs)

			l, err := NewLayout(recs[0], IBMEnterprise())
			if err == nil {
				if tc.counts != nil {
					_, err = l.ResolveCounts(countsByName(t, l, tc.counts))
				} else {
					_, err = l.Resolve(tc.data)
				}
			}

			require.Error(t, err)
			require.ErrorAs(t, err, tc.target)
			require.Contains(t, err.Error(), tc.contains)
		})
	}
}

// TestUnsignedPackedValue drives the COMP-6 reader directly, which is the only
// way to reach its two width guards: readCount only ever hands it a slice whose
// length came from layouter.width, so a byte count that is no width for the
// digit count cannot arise there. Pinning them here keeps them honest rather
// than merely unreachable, and pins the nibble order the resolve tests exercise
// only through a layout.
func TestUnsignedPackedValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		b       []byte
		digits  int
		want    int64
		wantErr string
	}{
		{name: "even digits fill their bytes", b: []byte{0x12, 0x34}, digits: 4, want: 1234},
		{name: "odd digits pad the leading nibble", b: []byte{0x01, 0x23}, digits: 3, want: 123},
		{name: "a single digit", b: []byte{0x07}, digits: 1, want: 7},
		{name: "every nibble carries its place", b: []byte{0x10, 0x00, 0x00}, digits: 6, want: 100000},
		{name: "the last nibble is a digit and not a sign", b: []byte{0x00, 0x09}, digits: 4, want: 9},
		{name: "zero", b: []byte{0x00, 0x00}, digits: 4, want: 0},
		{
			name: "no bytes hold no digits",
			b:    []byte{}, digits: 4,
			wantErr: "holds no digit positions",
		},
		{
			name: "too few bytes for the digit count",
			b:    []byte{0x12}, digits: 4,
			wantErr: "holds 1 bytes, which is no 4-digit unsigned packed value",
		},
		{
			name: "too many bytes for the digit count",
			b:    []byte{0x00, 0x12, 0x34}, digits: 3,
			wantErr: "holds 3 bytes, which is no 3-digit unsigned packed value",
		},
		{
			name: "a pad nibble holding a digit",
			b:    []byte{0x91, 0x23}, digits: 3,
			wantErr: "byte 0 is 0x91, whose high nibble pads a 3-digit value and is not zero",
		},
		{
			name: "a high nibble that is no digit",
			b:    []byte{0xA1, 0x23}, digits: 4,
			wantErr: "byte 0 is 0xa1, whose high nibble is no digit",
		},
		{
			name: "a low nibble that is no digit",
			b:    []byte{0x12, 0x3C}, digits: 4,
			wantErr: "byte 1 is 0x3c, whose low nibble is no digit",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := unsignedPackedValue(tc.b, tc.digits)
			if tc.wantErr != "" {
				require.Error(t, err)
				require.EqualError(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// countsByName keys a table of occurrence counts by the controlling field the
// layout resolved, which is what [Layout.ResolveCounts] takes.
func countsByName(t *testing.T, l *Layout, byName map[string]int) map[*Field]int {
	t.Helper()

	counts := make(map[*Field]int, len(byName))
	for name, n := range byName {
		item := l.Find(name)
		require.NotNil(t, item, "no item named %q", name)
		counts[item.Field] = n
	}
	return counts
}
