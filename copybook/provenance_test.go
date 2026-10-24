// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

import (
	"fmt"
	"strings"
	"testing"

	cobol "github.com/Zaba505/cobol-go"
	"github.com/stretchr/testify/require"
)

// The positioned errors Build and NewLayout report against an entry that came
// from a nested member carry the member and the chain of COPY statements that
// reached it, because they carry the entry's own Pos. INNER is copied with a
// REPLACING that shortens a word ahead of each fault, so a column here is the
// column in INNER as written.
func TestPositionedFromNestedMember(t *testing.T) {
	t.Parallel()

	outer := &cobol.LibraryText{Name: "OUTER", Copy: cobol.Pos{Line: 1, Column: 1}}
	inner := &cobol.LibraryText{Name: "INNER", Copy: cobol.Pos{Line: 1, Column: 1, Text: outer}}

	testCases := []struct {
		name   string
		inner  string
		layout bool
		target func() any
		pos    cobol.Pos
	}{
		{
			name:   "build: subordinate item with no record",
			inner:  "05 :T:-ORPHAN PIC X.\n",
			target: func() any { return &LevelSequenceError{} },
			pos:    cobol.Pos{Line: 1, Column: 1, Text: inner},
		},
		{
			name:   "build: picture that does not parse",
			inner:  "01 REC.\n   05 :T:-A PIC SS9.\n",
			target: func() any { return &PictureError{} },
			pos:    cobol.Pos{Line: 2, Column: 13, Text: inner},
		},
		{
			name:   "layout: an elementary display item needs a picture",
			inner:  "01 REC.\n   05 :T:-A PIC X.\n   05 :T:-B.\n",
			layout: true,
			target: func() any { return &LayoutError{} },
			pos:    cobol.Pos{Line: 3, Column: 4, Text: inner},
		},
		{
			name:   "layout: a longer redefines under a strict dialect",
			inner:  "01 REC.\n   05 :T:-A PIC X(4).\n   05 :T:-B REDEFINES T-A PIC X(10).\n",
			layout: true,
			target: func() any { return &RedefinesError{} },
			pos:    cobol.Pos{Line: 3, Column: 4, Text: inner},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			books := cobol.MapCopyBooks(map[string]string{
				"OUTER": "COPY INNER REPLACING ==:T:== BY ==T==.\n",
				"INNER": tc.inner,
			})
			f, err := cobol.Parse(
				strings.NewReader("COPY OUTER.\n"),
				cobol.WithFragment(),
				cobol.WithCopyBooks(books),
			)
			require.NoError(t, err)

			records, err := Build(f.Fragment.Entries)
			if tc.layout {
				require.NoError(t, err)
				require.Len(t, records, 1)
				_, err = NewLayout(records[0], IBMEnterprise())
			}
			wrapped := fmt.Errorf("reading layout: %w", err)

			var p cobol.Positioned
			require.ErrorAs(t, wrapped, &p)
			require.Equal(t, tc.pos, p.Position())
			require.Equal(t, []cobol.Pos{{Line: 1, Column: 1, Text: outer}, {Line: 1, Column: 1}}, p.Position().CopyChain())

			target := tc.target()
			require.ErrorAs(t, wrapped, target)
			require.Equal(t, tc.pos, target.(cobol.Positioned).Position())
		})
	}
}
