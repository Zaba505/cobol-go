// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// span is one expected row of a layout: where a field starts, how wide one
// occurrence of it is, and how far apart its occurrences are.
//
// stride and occurs default to length and 1, so a table states them only where
// slack or an OCCURS clause makes them interesting — which is exactly where the
// arithmetic is worth reading twice.
type span struct {
	name   string
	offset int
	length int
	stride int
	occurs int
	slack  int
}

// records builds the record tree of a copybook source, driving the real parser
// so the layouts are computed from an AST no test hand-assembled.
func records(t *testing.T, src string) []*Field {
	t.Helper()

	recs, err := Build(parseFragment(t, src))
	require.NoError(t, err)
	return recs
}

// layoutOf builds the layout of the first record of src under d.
func layoutOf(t *testing.T, src string, d Dialect) *Layout {
	t.Helper()

	recs := records(t, src)
	require.NotEmpty(t, recs)

	l, err := NewLayout(recs[0], d)
	require.NoError(t, err)
	return l
}

// requireSpans asserts the layout's items, in the pre-order [Layout.Items]
// returns them in, against the expected rows.
func requireSpans(t *testing.T, l *Layout, want []span) {
	t.Helper()

	items := l.Items()
	require.Len(t, items, len(want), "number of items")

	for i, w := range want {
		got := items[i]
		stride, occurs := w.stride, w.occurs
		if stride == 0 {
			stride = w.length
		}
		if occurs == 0 {
			occurs = 1
		}

		require.Equal(t, w.name, got.Field.Name, "item %d: name", i)
		require.Equal(t, w.offset, got.Offset, "%s: offset", w.name)
		require.Equal(t, w.length, got.Length, "%s: length", w.name)
		require.Equal(t, stride, got.Stride, "%s: stride", w.name)
		require.Equal(t, occurs, got.Occurs, "%s: occurs", w.name)
		require.Equal(t, w.slack, got.Slack, "%s: slack", w.name)
		require.Equal(t, stride*occurs, got.Total(), "%s: total", w.name)
		require.Equal(t, w.offset+stride*occurs, got.End(), "%s: end", w.name)
	}
}

func TestNewLayout(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		src        string
		dialect    Dialect
		want       []span
		wantLength int
	}{
		{
			// The worked example of issue #79: the record every other
			// case here is a variation on. ADDR-ZIP is the one worth
			// re-reading — PIC 9(5) COMP is four bytes, not five, and
			// a fifth byte there would shift every following record.
			name: "worked example mixing display, packed, binary and occurs",
			src: `01 CUSTOMER-RECORD.
   05 CUST-ID PIC 9(6).
   05 CUST-NAME PIC X(30).
   05 CUST-BALANCE PIC S9(7)V99 COMP-3.
   05 CUST-STATUS PIC X.
      88 STATUS-ACTIVE VALUE 'A'.
   05 CUST-ADDRESS OCCURS 3 TIMES.
      10 ADDR-LINE PIC X(40).
      10 ADDR-ZIP PIC 9(5) COMP.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "CUSTOMER-RECORD", offset: 0, length: 174},
				{name: "CUST-ID", offset: 0, length: 6},
				{name: "CUST-NAME", offset: 6, length: 30},
				{name: "CUST-BALANCE", offset: 36, length: 5},
				{name: "CUST-STATUS", offset: 41, length: 1},
				{name: "CUST-ADDRESS", offset: 42, length: 44, occurs: 3},
				{name: "ADDR-LINE", offset: 42, length: 40},
				{name: "ADDR-ZIP", offset: 82, length: 4},
			},
			wantLength: 174,
		},
		{
			// The golden record: DISPLAY, COMP-3, COMP, OCCURS and
			// REDEFINES in one layout, which is where the four
			// arithmetics have to agree with each other rather than
			// only with themselves. TXN-CARD covers TXN-BODY exactly,
			// so the record is the same length whichever way it is
			// read.
			name: "golden record mixing display, comp-3, comp, occurs and redefines",
			src: `01 TXN-RECORD.
   05 TXN-ID PIC 9(8).
   05 TXN-KIND PIC X.
   05 TXN-AMOUNT PIC S9(9)V99 COMP-3.
   05 TXN-COUNT PIC S9(4) COMP.
   05 TXN-BODY PIC X(24).
   05 TXN-CARD REDEFINES TXN-BODY.
      10 CARD-NUMBER PIC X(16).
      10 CARD-EXPIRY PIC 9(4) COMP.
      10 FILLER PIC X(6).
   05 TXN-TAGS OCCURS 4 TIMES.
      10 TAG-CODE PIC X(3).
      10 TAG-VALUE PIC S9(5) COMP-3.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "TXN-RECORD", offset: 0, length: 65},
				{name: "TXN-ID", offset: 0, length: 8},
				{name: "TXN-KIND", offset: 8, length: 1},
				{name: "TXN-AMOUNT", offset: 9, length: 6},
				{name: "TXN-COUNT", offset: 15, length: 2},
				{name: "TXN-BODY", offset: 17, length: 24},
				{name: "TXN-CARD", offset: 17, length: 24},
				{name: "CARD-NUMBER", offset: 17, length: 16},
				{name: "CARD-EXPIRY", offset: 33, length: 2},
				{name: "", offset: 35, length: 6},
				{name: "TXN-TAGS", offset: 41, length: 6, occurs: 4},
				{name: "TAG-CODE", offset: 41, length: 3},
				{name: "TAG-VALUE", offset: 44, length: 3},
			},
			wantLength: 65,
		},
		{
			name: "binary width is a staircase in the digit count",
			src: `01 R.
   05 A PIC S9(4) COMP.
   05 B PIC S9(5) COMP.
   05 C PIC S9(9) COMP.
   05 D PIC S9(10) COMP.
   05 E PIC S9(18) COMP.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 26},
				{name: "A", offset: 0, length: 2},
				{name: "B", offset: 2, length: 4},
				{name: "C", offset: 6, length: 4},
				{name: "D", offset: 10, length: 8},
				{name: "E", offset: 18, length: 8},
			},
			wantLength: 26,
		},
		{
			name: "binary is two bytes at one digit under ibm",
			src: `01 R.
   05 A PIC S9(2) COMP.
   05 B PIC X(3).
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 5},
				{name: "A", offset: 0, length: 2},
				{name: "B", offset: 2, length: 3},
			},
			wantLength: 5,
		},
		{
			// The 1-2 digit row is the silent fork between the two
			// staircases, and it moves every following field.
			name: "binary is one byte at one digit under gnucobol",
			src: `01 R.
   05 A PIC S9(2) COMP.
   05 B PIC X(3).
`,
			dialect: GnuCOBOL(),
			want: []span{
				{name: "R", offset: 0, length: 4},
				{name: "A", offset: 0, length: 1},
				{name: "B", offset: 1, length: 3},
			},
			wantLength: 4,
		},
		{
			name: "packed decimal is one nibble per digit plus a sign nibble",
			src: `01 R.
   05 A PIC S9(1) COMP-3.
   05 B PIC S9(3) COMP-3.
   05 C PIC S9(7)V99 COMP-3.
   05 D PIC 9(18) PACKED-DECIMAL.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 18},
				{name: "A", offset: 0, length: 1},
				{name: "B", offset: 1, length: 2},
				{name: "C", offset: 3, length: 5},
				{name: "D", offset: 8, length: 10},
			},
			wantLength: 18,
		},
		{
			name: "v and p occupy no storage",
			src: `01 R.
   05 A PIC S9(3)V99.
   05 B PIC 9(5)PPP.
   05 C PIC PPP9(5).
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 15},
				{name: "A", offset: 0, length: 5},
				{name: "B", offset: 5, length: 5},
				{name: "C", offset: 10, length: 5},
			},
			wantLength: 15,
		},
		{
			name: "numeric-edited items are as wide as they print",
			src: `01 R.
   05 A PIC ZZ,ZZ9.99.
   05 B PIC X(2).
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 11},
				{name: "A", offset: 0, length: 9},
				{name: "B", offset: 9, length: 2},
			},
			wantLength: 11,
		},
		{
			name: "sign separate costs a byte and is inherited from a group",
			src: `01 R.
   05 A PIC S9(5) SIGN LEADING SEPARATE.
   05 B PIC S9(5).
   05 G SIGN IS TRAILING SEPARATE CHARACTER.
      10 G1 PIC S9(3).
      10 G2 PIC 9(3).
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 18},
				{name: "A", offset: 0, length: 6},
				{name: "B", offset: 6, length: 5},
				{name: "G", offset: 11, length: 7},
				{name: "G1", offset: 11, length: 4},
				{name: "G2", offset: 15, length: 3},
			},
			wantLength: 18,
		},
		{
			name: "occurs multiplies an elementary item",
			src: `01 R.
   05 A PIC X(4) OCCURS 5 TIMES.
   05 B PIC X.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 21},
				{name: "A", offset: 0, length: 4, occurs: 5},
				{name: "B", offset: 20, length: 1},
			},
			wantLength: 21,
		},
		{
			name: "nested occurs multiplies the subtree",
			src: `01 R.
   05 T OCCURS 3 TIMES.
      10 U PIC X(2).
      10 V OCCURS 4 TIMES.
         15 W PIC X(3).
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 42},
				{name: "T", offset: 0, length: 14, occurs: 3},
				{name: "U", offset: 0, length: 2},
				{name: "V", offset: 2, length: 3, occurs: 4},
				{name: "W", offset: 2, length: 3},
			},
			wantLength: 42,
		},
		{
			name: "redefines overlays the redefined item at the same offset",
			src: `01 R.
   05 A PIC X(10).
   05 B REDEFINES A.
      10 B1 PIC X(4).
      10 B2 PIC X(6).
   05 C PIC X(2).
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 12},
				{name: "A", offset: 0, length: 10},
				{name: "B", offset: 0, length: 10},
				{name: "B1", offset: 0, length: 4},
				{name: "B2", offset: 4, length: 6},
				{name: "C", offset: 10, length: 2},
			},
			wantLength: 12,
		},
		{
			name: "a shorter redefines leaves the group's extent alone",
			src: `01 R.
   05 A PIC X(10).
   05 B REDEFINES A PIC X(4).
   05 C PIC X(2).
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 12},
				{name: "A", offset: 0, length: 10},
				{name: "B", offset: 0, length: 4},
				{name: "C", offset: 10, length: 2},
			},
			wantLength: 12,
		},
		{
			name: "several items may redefine the same one",
			src: `01 R.
   05 A PIC X(6).
   05 B REDEFINES A PIC X(6).
   05 C REDEFINES A PIC X(6).
   05 D PIC X.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 7},
				{name: "A", offset: 0, length: 6},
				{name: "B", offset: 0, length: 6},
				{name: "C", offset: 0, length: 6},
				{name: "D", offset: 6, length: 1},
			},
			wantLength: 7,
		},
		{
			// Under a lenient dialect a longer redefining item
			// extends the group it sits in rather than being an
			// error, and C starts past the end of the longer of the
			// two overlaid items rather than inside B.
			name: "a longer redefines extends the group under a lenient dialect",
			src: `01 R.
   05 A PIC X(4).
   05 B REDEFINES A PIC X(10).
   05 C PIC X(2).
`,
			dialect: GnuCOBOL(),
			want: []span{
				{name: "R", offset: 0, length: 12},
				{name: "A", offset: 0, length: 4},
				{name: "B", offset: 0, length: 10},
				{name: "C", offset: 10, length: 2},
			},
			wantLength: 12,
		},
		{
			name: "synchronized inserts slack before a binary item",
			src: `01 R.
   05 A PIC X.
   05 B PIC S9(4) COMP SYNC.
   05 C PIC X.
   05 D PIC S9(9) COMP SYNC.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 12},
				{name: "A", offset: 0, length: 1},
				{name: "B", offset: 2, length: 2, slack: 1},
				{name: "C", offset: 4, length: 1},
				{name: "D", offset: 8, length: 4, slack: 3},
			},
			wantLength: 12,
		},
		{
			name: "synchronized has no effect on display and packed items",
			src: `01 R.
   05 A PIC X.
   05 B PIC S9(5) SYNC.
   05 C PIC S9(5) COMP-3 SYNC.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 9},
				{name: "A", offset: 0, length: 1},
				{name: "B", offset: 1, length: 5},
				{name: "C", offset: 6, length: 3},
			},
			wantLength: 9,
		},
		{
			// The same copybook under a dialect that syntax-checks
			// SYNCHRONIZED and lays the record out as though it were
			// absent.
			name: "synchronized is ignored under gnucobol",
			src: `01 R.
   05 A PIC X.
   05 B PIC S9(4) COMP SYNC.
   05 C PIC X.
   05 D PIC S9(9) COMP SYNC.
`,
			dialect: GnuCOBOL(),
			want: []span{
				{name: "R", offset: 0, length: 8},
				{name: "A", offset: 0, length: 1},
				{name: "B", offset: 1, length: 2},
				{name: "C", offset: 3, length: 1},
				{name: "D", offset: 4, length: 4},
			},
			wantLength: 8,
		},
		{
			// A group under OCCURS holding a SYNCHRONIZED item is
			// aligned as a whole and padded at the end, so that every
			// occurrence is laid out identically. Aligning V inside
			// each occurrence separately would not do that.
			name: "a group under occurs is aligned and gets trailing slack",
			src: `01 R.
   05 A PIC X.
   05 T OCCURS 3 TIMES.
      10 U PIC X.
      10 V PIC S9(9) COMP SYNC.
   05 Z PIC X.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 29},
				{name: "A", offset: 0, length: 1},
				{name: "T", offset: 4, length: 8, occurs: 3, slack: 3},
				{name: "U", offset: 4, length: 1},
				{name: "V", offset: 8, length: 4, slack: 3},
				{name: "Z", offset: 28, length: 1},
			},
			wantLength: 29,
		},
		{
			// A width that is not a multiple of its own boundary — as
			// BinarySizeSmallest's 3, 5, 6 and 7-byte items are not —
			// is where trailing slack would show up if it were applied
			// to a field that occurs once. It must not be: alignment
			// is bytes skipped *before* an item, and padding a single
			// occurrence would push C out by a byte that no item's
			// Slack accounts for.
			name: "a single occurrence takes no trailing slack",
			src: `01 R.
   05 A PIC X.
   05 B PIC S9(6) COMP SYNC.
   05 C PIC X.
`,
			dialect: Dialect{
				Binary: BinarySizeSmallest, Sync: SyncAligned, Redefines: RedefinesStrict,
				IndexWidth: 4, PointerWidth: 8,
			},
			want: []span{
				{name: "R", offset: 0, length: 8},
				{name: "A", offset: 0, length: 1},
				{name: "B", offset: 4, length: 3, slack: 3},
				{name: "C", offset: 7, length: 1},
			},
			wantLength: 8,
		},
		{
			// The same item under OCCURS does take it, because every
			// occurrence after the first has to start on the boundary.
			name: "several occurrences of an odd width take trailing slack",
			src: `01 R.
   05 A PIC X.
   05 B PIC S9(6) COMP SYNC OCCURS 3 TIMES.
   05 C PIC X.
`,
			dialect: Dialect{
				Binary: BinarySizeSmallest, Sync: SyncAligned, Redefines: RedefinesStrict,
				IndexWidth: 4, PointerWidth: 8,
			},
			want: []span{
				{name: "R", offset: 0, length: 17},
				{name: "A", offset: 0, length: 1},
				{name: "B", offset: 4, length: 3, stride: 4, occurs: 3, slack: 3},
				{name: "C", offset: 16, length: 1},
			},
			wantLength: 17,
		},
		{
			name: "floating point, index and pointer widths come from the usage",
			src: `01 R.
   05 A COMP-1.
   05 B COMP-2.
   05 C INDEX.
   05 D POINTER.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 20},
				{name: "A", offset: 0, length: 4},
				{name: "B", offset: 4, length: 8},
				{name: "C", offset: 12, length: 4},
				{name: "D", offset: 16, length: 4},
			},
			wantLength: 20,
		},
		{
			name: "a pointer is eight bytes on a sixty-four bit build",
			src: `01 R.
   05 A POINTER.
   05 B PIC X.
`,
			dialect: GnuCOBOL(),
			want: []span{
				{name: "R", offset: 0, length: 9},
				{name: "A", offset: 0, length: 8},
				{name: "B", offset: 8, length: 1},
			},
			wantLength: 9,
		},
		{
			name: "usage inherited from a group fixes the width",
			src: `01 R.
   05 G COMP-3.
      10 G1 PIC S9(5).
      10 G2 PIC S9(5) DISPLAY.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 8},
				{name: "G", offset: 0, length: 8},
				{name: "G1", offset: 0, length: 3},
				{name: "G2", offset: 3, length: 5},
			},
			wantLength: 8,
		},
		{
			name: "a level-77 item is a record of one field",
			src: `77 STANDALONE PIC S9(4) COMP.
`,
			dialect:    IBMEnterprise(),
			want:       []span{{name: "STANDALONE", offset: 0, length: 2}},
			wantLength: 2,
		},
		{
			name: "filler items occupy storage and carry no name",
			src: `01 R.
   05 FILLER PIC X(3).
   05 A PIC X.
`,
			dialect: IBMEnterprise(),
			want: []span{
				{name: "R", offset: 0, length: 4},
				{name: "", offset: 0, length: 3},
				{name: "A", offset: 3, length: 1},
			},
			wantLength: 4,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := layoutOf(t, tc.src, tc.dialect)
			requireSpans(t, l, tc.want)
			require.Equal(t, tc.wantLength, l.Length, "record length")
			require.Equal(t, tc.dialect, l.Dialect)
			require.Equal(t, l.Record.Total(), l.Length, "Layout.Length is the record's total")
		})
	}
}

func TestNewLayoutErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		src      string
		dialect  Dialect
		target   any
		contains string
	}{
		{
			name: "occurs depending on has no static layout",
			src: `01 R.
   05 N PIC 9(2).
   05 A PIC X(3) OCCURS 1 TO 5 TIMES DEPENDING ON N.
`,
			dialect:  IBMEnterprise(),
			target:   &OccursError{},
			contains: "OCCURS DEPENDING ON",
		},
		{
			name: "occurs with a range and no depending on has no fixed length",
			src: `01 R.
   05 A PIC X(3) OCCURS 1 TO 5 TIMES.
`,
			dialect:  IBMEnterprise(),
			target:   &OccursError{},
			contains: "no fixed length",
		},
		{
			name: "an elementary display item needs a picture",
			src: `01 R.
   05 A PIC X.
   05 B.
`,
			dialect:  IBMEnterprise(),
			target:   &LayoutError{},
			contains: "no PICTURE clause",
		},
		{
			name: "a binary item needs a picture",
			src: `01 R.
   05 A COMP.
`,
			dialect:  IBMEnterprise(),
			target:   &LayoutError{},
			contains: "USAGE COMP item with no PICTURE clause",
		},
		{
			name: "a packed item needs a numeric picture",
			src: `01 R.
   05 A PIC X(5) COMP-3.
`,
			dialect:  IBMEnterprise(),
			target:   &LayoutError{},
			contains: "alphanumeric rather than numeric",
		},
		{
			name: "a binary item needs digit positions",
			src: `01 R.
   05 A PIC ZZZ COMP.
`,
			dialect:  IBMEnterprise(),
			target:   &LayoutError{},
			contains: "numeric-edited rather than numeric",
		},
		{
			name: "redefines must name a preceding item of the same group",
			src: `01 R.
   05 A PIC X(4).
   05 B REDEFINES NOWHERE PIC X(4).
`,
			dialect:  IBMEnterprise(),
			target:   &RedefinesError{},
			contains: `no preceding item named "NOWHERE"`,
		},
		{
			name: "redefines may not name an item that follows it",
			src: `01 R.
   05 B REDEFINES A PIC X(4).
   05 A PIC X(4).
`,
			dialect:  IBMEnterprise(),
			target:   &RedefinesError{},
			contains: `no preceding item named "A"`,
		},
		{
			// The strict rule is the standard's, and it is what stops
			// a redefinition silently lengthening the record.
			name: "a longer redefines is rejected under a strict dialect",
			src: `01 R.
   05 A PIC X(4).
   05 B REDEFINES A PIC X(10).
`,
			dialect:  IBMEnterprise(),
			target:   &RedefinesError{},
			contains: "occupies 10 bytes, more than the 4 bytes",
		},
		{
			name: "occurs zero times is not an occurrence count",
			src: `01 R.
   05 A PIC X OCCURS 0 TIMES.
`,
			dialect:  IBMEnterprise(),
			target:   &OccursError{},
			contains: "not a positive whole number",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recs := records(t, tc.src)
			require.NotEmpty(t, recs)

			l, err := NewLayout(recs[0], tc.dialect)
			require.Nil(t, l)
			require.Error(t, err)
			require.ErrorAs(t, err, tc.target)
			require.Contains(t, err.Error(), tc.contains)
		})
	}
}

func TestNewLayoutRequiresARecord(t *testing.T) {
	t.Parallel()

	l, err := NewLayout(nil, IBMEnterprise())
	require.Nil(t, l)
	require.ErrorIs(t, err, ErrNilRecord)
}

// TestNewLayoutRequiresACompleteDialect is the layout-side counterpart of
// codec's rule that no axis of an encoding has a default: a dialect field left
// out shifts every following field of the record, so it is reported rather than
// filled in.
func TestNewLayoutRequiresACompleteDialect(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		dialect Dialect
		field   string
	}{
		{
			name:    "empty dialect",
			dialect: Dialect{},
			field:   "Binary",
		},
		{
			name:    "no sync mode",
			dialect: Dialect{Binary: BinarySize248, Redefines: RedefinesStrict, IndexWidth: 4, PointerWidth: 4},
			field:   "Sync",
		},
		{
			name:    "no redefines rule",
			dialect: Dialect{Binary: BinarySize248, Sync: SyncAligned, IndexWidth: 4, PointerWidth: 4},
			field:   "Redefines",
		},
		{
			name:    "no index width",
			dialect: Dialect{Binary: BinarySize248, Sync: SyncAligned, Redefines: RedefinesStrict, PointerWidth: 4},
			field:   "IndexWidth",
		},
		{
			name:    "no pointer width",
			dialect: Dialect{Binary: BinarySize248, Sync: SyncAligned, Redefines: RedefinesStrict, IndexWidth: 4},
			field:   "PointerWidth",
		},
		{
			name: "index width beyond any machine datum",
			dialect: Dialect{
				Binary: BinarySize248, Sync: SyncAligned, Redefines: RedefinesStrict,
				IndexWidth: 40, PointerWidth: 4,
			},
			field: "IndexWidth",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			recs := records(t, "01 R PIC X.\n")
			require.Len(t, recs, 1)

			l, err := NewLayout(recs[0], tc.dialect)
			require.Nil(t, l)

			var dialectErr DialectError
			require.ErrorAs(t, err, &dialectErr)
			require.Equal(t, tc.field, dialectErr.Field)
		})
	}
}

// TestDialects asserts that every shipped bundle is complete, so that a caller
// passing one never has to fill a field in itself.
func TestDialects(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		dialect Dialect
	}{
		{name: "ibm enterprise", dialect: IBMEnterprise()},
		{name: "gnucobol", dialect: GnuCOBOL()},
		{name: "micro focus", dialect: MicroFocus()},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.NoError(t, tc.dialect.Validate())
		})
	}
}

func TestBinarySizeWidth(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		size   BinarySize
		widths map[int]int
	}{
		{
			name: "2-4-8",
			size: BinarySize248,
			widths: map[int]int{
				1: 2, 2: 2, 4: 2, 5: 4, 9: 4, 10: 8, 18: 8, 19: 16, 31: 16,
			},
		},
		{
			name: "1-2-4-8",
			size: BinarySize1248,
			widths: map[int]int{
				1: 1, 2: 1, 3: 2, 4: 2, 5: 4, 9: 4, 10: 8, 18: 8, 19: 16,
			},
		},
		{
			name: "1--8",
			size: BinarySizeSmallest,
			widths: map[int]int{
				1: 1, 2: 1, 3: 2, 4: 2, 5: 3, 6: 3, 7: 4, 9: 4,
				10: 5, 11: 5, 12: 6, 14: 6, 15: 7, 16: 7, 17: 8, 18: 8, 19: 16,
			},
		},
		{
			name: "full",
			size: BinarySizeFull,
			widths: map[int]int{
				1: 8, 4: 8, 9: 8, 18: 8, 19: 16,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.name, tc.size.String())
			for digits, want := range tc.widths {
				require.Equal(t, want, tc.size.width(digits), "%d digits", digits)
			}
		})
	}
}

func TestItemOccurrenceOffset(t *testing.T) {
	t.Parallel()

	l := layoutOf(t, `01 R.
   05 A PIC X.
   05 T OCCURS 3 TIMES.
      10 U PIC X.
      10 V PIC S9(9) COMP SYNC.
`, IBMEnterprise())

	table := l.Find("T")
	require.NotNil(t, table)

	for n, want := range []int{4, 12, 20} {
		got, err := table.OccurrenceOffset(n)
		require.NoError(t, err)
		require.Equal(t, want, got, "occurrence %d", n)
	}

	for _, n := range []int{-1, 3} {
		_, err := table.OccurrenceOffset(n)
		var occErr OccurrenceError
		require.ErrorAs(t, err, &occErr)
		require.Equal(t, 3, occErr.Occurs)
		require.Equal(t, n, occErr.Index)
	}

	// A field that occurs once has exactly one occurrence, at its own
	// offset.
	single := l.Find("A")
	require.NotNil(t, single)

	offset, err := single.OccurrenceOffset(0)
	require.NoError(t, err)
	require.Equal(t, 0, offset)

	_, err = single.OccurrenceOffset(1)
	require.Error(t, err)
}

func TestLayoutFind(t *testing.T) {
	t.Parallel()

	l := layoutOf(t, `01 R.
   05 FILLER PIC X(2).
   05 G.
      10 A PIC X(3).
`, IBMEnterprise())

	item := l.Find("A")
	require.NotNil(t, item)
	require.Equal(t, 2, item.Offset)
	require.Equal(t, l.Find("G"), item.Parent)
	require.Equal(t, l.Record, item.Parent.Parent)

	require.Nil(t, l.Find("NOWHERE"))
	require.Nil(t, l.Find(""), "a FILLER item has no name to find it by")
}

// TestLayoutOfSubtree pins the documented behaviour that a field from inside a
// record lays out with itself at offset zero, which is what makes a group's
// layout reusable where the same group appears in several records.
func TestLayoutOfSubtree(t *testing.T) {
	t.Parallel()

	recs := records(t, `01 R.
   05 A PIC X(7).
   05 G.
      10 G1 PIC X(3).
      10 G2 PIC X(4).
`)
	require.Len(t, recs, 1)

	group := recs[0].Children[1]
	require.Equal(t, "G", group.Name)

	l, err := NewLayout(group, IBMEnterprise())
	require.NoError(t, err)
	requireSpans(t, l, []span{
		{name: "G", offset: 0, length: 7},
		{name: "G1", offset: 0, length: 3},
		{name: "G2", offset: 3, length: 4},
	})
	require.Equal(t, 7, l.Length)
}

// TestRedefinesLinksTheItemItOverlays asserts the back-reference every consumer
// needs to tell an overlay apart from a field of its own.
func TestRedefinesLinksTheItemItOverlays(t *testing.T) {
	t.Parallel()

	l := layoutOf(t, `01 R.
   05 A PIC X(6).
   05 B REDEFINES A PIC 9(6).
`, IBMEnterprise())

	a, b := l.Find("A"), l.Find("B")
	require.NotNil(t, a)
	require.NotNil(t, b)
	require.Nil(t, a.Redefines)
	require.Same(t, a, b.Redefines)
	require.Equal(t, a.Offset, b.Offset)
}

func TestLayoutStringers(t *testing.T) {
	t.Parallel()

	require.Equal(t, "unset", BinarySizeUnset.String())
	require.Equal(t, "2-4-8", BinarySize248.String())
	require.Equal(t, "unset", SyncUnset.String())
	require.Equal(t, "ignored", SyncIgnored.String())
	require.Equal(t, "aligned", SyncAligned.String())
	require.Equal(t, "unset", RedefinesUnset.String())
	require.Equal(t, "strict", RedefinesStrict.String())
	require.Equal(t, "lenient", RedefinesLenient.String())
}
