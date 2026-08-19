// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSameName pins the comparison every data-name resolution in this package
// routes through, including the two properties a bare strings.EqualFold gives
// that a caller might not expect to be relied on: FILLER is excluded by the
// callers rather than here, and the empty name matches only the empty name.
func TestSameName(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "identical", a: "HEADER", b: "HEADER", want: true},
		{name: "declaration cased, reference upper", a: "Header", b: "HEADER", want: true},
		{name: "declaration upper, reference lower", a: "HEADER", b: "header", want: true},
		{name: "mixed both ways", a: "cUsT-nAmE", b: "CuSt-NaMe", want: true},
		{name: "hyphen is not folded away", a: "CUST-NAME", b: "CUSTNAME", want: false},
		{name: "different names", a: "HEADER", b: "TRAILER", want: false},
		{name: "prefix is not a match", a: "HDR", b: "HDR-TYPE", want: false},
		{name: "empty matches empty", a: "", b: "", want: true},
		{name: "empty matches nothing else", a: "", b: "A", want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, sameName(tc.a, tc.b))
			require.Equal(t, tc.want, sameName(tc.b, tc.a), "the comparison is symmetric")
		})
	}
}

// TestFieldNamePreservesSourceCase is the other half of the change: folding
// happens in the comparison and never in what is stored, so a copybook spelling
// its data-names in mixed case builds fields spelled the way the source spelled
// them. Without this the printer would re-emit a copybook nobody wrote.
func TestFieldNamePreservesSourceCase(t *testing.T) {
	t.Parallel()

	recs := records(t, `01 Customer-Record.
   05 Cust-Id                PIC 9(6).
   05 cust_name              PIC X(20).
   66 Whole-Thing RENAMES CUST-ID THRU CUST_NAME.
`)
	require.Len(t, recs, 1)

	rec := recs[0]
	require.Equal(t, "Customer-Record", rec.Name)
	require.Equal(t, "Cust-Id", rec.Children[0].Name)
	require.Equal(t, "cust_name", rec.Children[1].Name)

	require.Len(t, rec.Aliases, 1)
	require.Equal(t, "Whole-Thing", rec.Aliases[0].Name)
	// The endpoints resolved through the folded comparison, and the fields
	// they resolved to still carry their own spelling.
	require.Equal(t, "Cust-Id", rec.Aliases[0].From.Name)
	require.Equal(t, "cust_name", rec.Aliases[0].Through.Name)
}

// TestRedefinesMatchesNameCaseInsensitively covers the resolution in
// [layouter.redefinedBy]: a REDEFINES clause naming its target in a different
// case from the declaration resolves to it, as every COBOL compiler resolves it.
func TestRedefinesMatchesNameCaseInsensitively(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
	}{
		{
			name: "clause upper, declaration mixed",
			src: `01 R.
   05 Alpha                 PIC X(6).
   05 BETA REDEFINES ALPHA  PIC 9(6).
`,
		},
		{
			name: "clause mixed, declaration upper",
			src: `01 R.
   05 ALPHA                 PIC X(6).
   05 BETA REDEFINES Alpha  PIC 9(6).
`,
		},
		{
			name: "clause lower, declaration upper",
			src: `01 R.
   05 ALPHA                 PIC X(6).
   05 BETA REDEFINES alpha  PIC 9(6).
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := layoutOf(t, tc.src, IBMEnterprise())
			items := l.Items()
			require.Len(t, items, 3)

			alpha, beta := items[1], items[2]
			require.Same(t, alpha, beta.Redefines)
			require.Equal(t, alpha.Offset, beta.Offset)
			require.Equal(t, 6, l.Length, "the overlay adds no storage")
		})
	}
}

// TestRenamesEndpointsMatchNameCaseInsensitively covers [indexOf], which
// resolves the endpoints of a level-66 range.
func TestRenamesEndpointsMatchNameCaseInsensitively(t *testing.T) {
	t.Parallel()

	recs := records(t, `01 REC.
   05 First-Name            PIC X(10).
   05 Middle-Name           PIC X(10).
   05 Last-Name             PIC X(10).
   66 WHOLE-NAME RENAMES FIRST-NAME THRU LAST-NAME.
   66 JUST-MIDDLE RENAMES middle-name.
`)
	require.Len(t, recs, 1)
	rec := recs[0]

	require.Len(t, rec.Aliases, 2)

	whole := rec.Aliases[0]
	require.Equal(t, "First-Name", whole.From.Name)
	require.NotNil(t, whole.Through)
	require.Equal(t, "Last-Name", whole.Through.Name)
	require.Len(t, whole.Fields, 3)

	middle := rec.Aliases[1]
	require.Equal(t, "Middle-Name", middle.From.Name)
	require.Nil(t, middle.Through)
	require.Len(t, middle.Fields, 1)
}

// TestDependingOnMatchesNameCaseInsensitively covers the fifth resolution site,
// [layouter.control]. It is not one of the four the issue named, and it is here
// because the point of routing every site through one helper is that no site
// compares differently: a DEPENDING ON phrase names a data-name exactly as a
// REDEFINES clause does.
func TestDependingOnMatchesNameCaseInsensitively(t *testing.T) {
	t.Parallel()

	l := layoutOf(t, `01 R.
   05 Item-Count            PIC 9(2).
   05 ITEMS OCCURS 1 TO 5 TIMES DEPENDING ON ITEM-COUNT PIC X(3).
`, IBMEnterprise())

	items := l.Find("ITEMS")
	require.NotNil(t, items)
	require.NotNil(t, items.DependingOn)
	require.Equal(t, "Item-Count", items.DependingOn.Field.Name)
	require.Equal(t, 1, items.MinOccurs)
	require.Equal(t, 5, items.MaxOccurs)
}

// TestLayoutFindFoldsCase is the decided behaviour of the public lookup: Find
// matches a data-name the way COBOL matches one, so a caller passing a name read
// out of a record layout document finds the item a REDEFINES clause in the same
// copybook would have resolved to. [Field.Name] is what a caller wanting the
// source spelling reads.
func TestLayoutFindFoldsCase(t *testing.T) {
	t.Parallel()

	l := layoutOf(t, `01 Customer-Record.
   05 FILLER                PIC X(2).
   05 Cust-Group.
      10 Cust-Id            PIC 9(6).
`, IBMEnterprise())

	for _, spelling := range []string{"Cust-Id", "CUST-ID", "cust-id", "cUsT-iD"} {
		item := l.Find(spelling)
		require.NotNil(t, item, "Find(%q)", spelling)
		require.Equal(t, "Cust-Id", item.Field.Name, "Find(%q) returns the declared spelling", spelling)
		require.Equal(t, 2, item.Offset)
	}

	require.NotNil(t, l.Find("customer-record"), "the record itself is findable too")
	require.Nil(t, l.Find("CUST-IDX"), "folding case is not fuzzy matching")
	require.Nil(t, l.Find(""), "a FILLER item has no name to find it by")
}

// TestRedefinesEnclosingOrderingUnderFoldedMatching is acceptance criterion 6:
// the sibling-before-ancestor ordering added in #126 still holds when both scans
// fold case, shadowing case included. A preceding sibling wins whatever case
// either side is written in, and only a name no sibling carries reaches the open
// chain — where a match is the level-rule error rather than a resolution.
func TestRedefinesEnclosingOrderingUnderFoldedMatching(t *testing.T) {
	t.Parallel()

	t.Run("a sibling shadowing its record wins under folded matching", func(t *testing.T) {
		t.Parallel()

		// 01 A / 05 a / 05 B REDEFINES A: the sibling "a" and the
		// enclosing record "A" are now the same name as the clause, and
		// the sibling has to be the one it resolves to. Under exact
		// matching only the record matched, which is the level-rule
		// error — so this case flips outcome with the fold and is the
		// one worth pinning.
		l := layoutOf(t, `01 A.
   05 a                     PIC X(4).
   05 B REDEFINES A         PIC 9(4).
`, IBMEnterprise())

		items := l.Items()
		require.Len(t, items, 3)
		require.Equal(t, "A", items[0].Field.Name)
		require.Equal(t, "a", items[1].Field.Name)
		require.Equal(t, "B", items[2].Field.Name)

		require.Same(t, items[1], items[2].Redefines, "B redefines its sibling, not the record")
		require.Equal(t, 0, items[2].Offset)
		require.Equal(t, 4, l.Length)
	})

	t.Run("the ancestor scan folds case too", func(t *testing.T) {
		t.Parallel()

		// No sibling carries the name, so the open chain is consulted
		// and the entry is subordinate to its own target: the level-rule
		// error #126 added, reached through a spelling that differs from
		// the record's. The diagnostic names the item as it was declared
		// — "REC", not the clause's "rec" — because it is telling the
		// reader which entry to renumber.
		_, err := Build(parseFragment(t, "01 REC.\n   05 GRP.\n      10 X REDEFINES rec PIC X(4).\n"))
		require.Error(t, err)
		require.ErrorAs(t, err, &LevelSequenceError{})
		require.EqualError(t, err, `level-10 item "X" at line 3, column 7 redefines "REC", which its level number makes it subordinate to rather than a sibling of; a REDEFINES entry must be an item of the same group as its target, so write it at its target's level number, 01`)
	})

	t.Run("an unmatched name still reaches the layouter", func(t *testing.T) {
		t.Parallel()

		// Neither scan matches, so Build places the entry as a sibling
		// and NewLayout reports the unresolvable target. Folding case
		// must not turn "no such name" into a match.
		recs := records(t, `01 REC.
   05 ALPHA                 PIC X(4).
   05 B REDEFINES OMEGA     PIC X(4).
`)
		_, err := NewLayout(recs[0], IBMEnterprise())
		require.Error(t, err)
		require.ErrorAs(t, err, &RedefinesError{})
	})
}
