// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package copybook

import (
	"strings"
	"testing"

	cobol "github.com/Zaba505/cobol-go"
	"github.com/Zaba505/cobol-go/picture"
	"github.com/stretchr/testify/require"
)

// at is shorthand for a source position, so an expected tree reads as a tree
// rather than as a wall of field names.
func at(line, column int) cobol.Pos {
	return cobol.Pos{Line: line, Column: column}
}

// parseFragment parses src as a standalone copybook and returns the flat entry
// list [Build] consumes. Driving the tests through the real parser is what keeps
// the expected trees honest about what the AST actually holds.
func parseFragment(t *testing.T, src string) []*cobol.DataDescriptionEntry {
	t.Helper()

	f, err := cobol.Parse(strings.NewReader(src), cobol.WithFragment())
	require.NoError(t, err)
	require.NotNil(t, f.Fragment)
	return f.Fragment.Entries
}

// pic is the parsed PICTURE character-string an expected [Field] carries.
func pic(t *testing.T, src string, opts ...picture.ParseOption) *picture.Picture {
	t.Helper()

	p, err := picture.Parse(src, opts...)
	require.NoError(t, err)
	return p
}

// values returns the VALUE clause values of an entry, for the expected
// [Condition] of a copybook too large to spell every literal position out in.
func values(t *testing.T, entry *cobol.DataDescriptionEntry) []cobol.ValueSpec {
	t.Helper()

	for _, clause := range entry.Clauses {
		if value, ok := clause.(*cobol.ValueClause); ok {
			return value.Values
		}
	}
	require.FailNow(t, "entry has no VALUE clause")
	return nil
}

func TestBuild(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
		opts []Option
		want func(t *testing.T, entries []*cobol.DataDescriptionEntry) []*Field
	}{
		{
			name: "no entries",
			src:  "",
			want: func(*testing.T, []*cobol.DataDescriptionEntry) []*Field { return nil },
		},
		{
			name: "record of elementary items",
			src: `01 CUSTOMER-RECORD.
   05 CUST-ID PIC 9(6).
   05 CUST-NAME PIC X(20).
`,
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				rec := &Field{
					Pos:   at(1, 1),
					Level: 1,
					Name:  "CUSTOMER-RECORD",
					Kind:  KindGroup,
					Entry: e[0],
				}
				id := &Field{
					Pos:     at(2, 4),
					Level:   5,
					Name:    "CUST-ID",
					Picture: pic(t, "9(6)"),
					Parent:  rec,
					Entry:   e[1],
				}
				name := &Field{
					Pos:     at(3, 4),
					Level:   5,
					Name:    "CUST-NAME",
					Picture: pic(t, "X(20)"),
					Parent:  rec,
					Entry:   e[2],
				}
				rec.Children = []*Field{id, name}
				return []*Field{rec}
			},
		},
		{
			name: "nested groups close at the next lower level",
			src: `01 REC.
   05 ADDRESS.
      10 STREET PIC X(20).
      10 CITY PIC X(10).
   05 PHONE PIC X(10).
`,
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				rec := &Field{Pos: at(1, 1), Level: 1, Name: "REC", Kind: KindGroup, Entry: e[0]}
				addr := &Field{
					Pos:    at(2, 4),
					Level:  5,
					Name:   "ADDRESS",
					Kind:   KindGroup,
					Parent: rec,
					Entry:  e[1],
				}
				street := &Field{
					Pos:     at(3, 7),
					Level:   10,
					Name:    "STREET",
					Picture: pic(t, "X(20)"),
					Parent:  addr,
					Entry:   e[2],
				}
				city := &Field{
					Pos:     at(4, 7),
					Level:   10,
					Name:    "CITY",
					Picture: pic(t, "X(10)"),
					Parent:  addr,
					Entry:   e[3],
				}
				phone := &Field{
					Pos:     at(5, 4),
					Level:   5,
					Name:    "PHONE",
					Picture: pic(t, "X(10)"),
					Parent:  rec,
					Entry:   e[4],
				}
				addr.Children = []*Field{street, city}
				rec.Children = []*Field{addr, phone}
				return []*Field{rec}
			},
		},
		{
			name: "subordinate items need not share one level number",
			src: `01 REC.
   05 GROUP-A.
      10 DEEP PIC X.
      07 SHALLOWER PIC X.
   03 SIBLING PIC X.
`,
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				rec := &Field{Pos: at(1, 1), Level: 1, Name: "REC", Kind: KindGroup, Entry: e[0]}
				groupA := &Field{
					Pos:    at(2, 4),
					Level:  5,
					Name:   "GROUP-A",
					Kind:   KindGroup,
					Parent: rec,
					Entry:  e[1],
				}
				deep := &Field{
					Pos:     at(3, 7),
					Level:   10,
					Name:    "DEEP",
					Picture: pic(t, "X"),
					Parent:  groupA,
					Entry:   e[2],
				}
				// Level 07 is greater than GROUP-A's 05, so it is subordinate
				// to GROUP-A alongside the level 10 rather than beside it.
				shallower := &Field{
					Pos:     at(4, 7),
					Level:   7,
					Name:    "SHALLOWER",
					Picture: pic(t, "X"),
					Parent:  groupA,
					Entry:   e[3],
				}
				// Level 03 is lower than 05, so it closes GROUP-A.
				sibling := &Field{
					Pos:     at(5, 4),
					Level:   3,
					Name:    "SIBLING",
					Picture: pic(t, "X"),
					Parent:  rec,
					Entry:   e[4],
				}
				groupA.Children = []*Field{deep, shallower}
				rec.Children = []*Field{groupA, sibling}
				return []*Field{rec}
			},
		},
		{
			name: "level-77 standalone items stand beside records",
			src: `77 GRAND-TOTAL PIC S9(9)V99 COMP-3.
01 REC.
   05 A PIC X.
77 COUNTER PIC 9(4) BINARY.
`,
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				total := &Field{
					Pos:     at(1, 1),
					Level:   77,
					Name:    "GRAND-TOTAL",
					Picture: pic(t, "S9(9)V99"),
					Usage:   UsageComp3,
					Entry:   e[0],
				}
				rec := &Field{Pos: at(2, 1), Level: 1, Name: "REC", Kind: KindGroup, Entry: e[1]}
				a := &Field{
					Pos:     at(3, 4),
					Level:   5,
					Name:    "A",
					Picture: pic(t, "X"),
					Parent:  rec,
					Entry:   e[2],
				}
				rec.Children = []*Field{a}
				counter := &Field{
					Pos:     at(4, 1),
					Level:   77,
					Name:    "COUNTER",
					Picture: pic(t, "9(4)"),
					Usage:   UsageBinary,
					Entry:   e[3],
				}
				return []*Field{total, rec, counter}
			},
		},
		{
			name: "filler items are retained and marked unnamed",
			src: `01 REC.
   05 A PIC X.
   05 FILLER PIC X(3).
   05 PIC X(2).
`,
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				rec := &Field{Pos: at(1, 1), Level: 1, Name: "REC", Kind: KindGroup, Entry: e[0]}
				a := &Field{
					Pos:     at(2, 4),
					Level:   5,
					Name:    "A",
					Picture: pic(t, "X"),
					Parent:  rec,
					Entry:   e[1],
				}
				// Written FILLER.
				filler := &Field{
					Pos:     at(3, 4),
					Level:   5,
					Filler:  true,
					Picture: pic(t, "X(3)"),
					Parent:  rec,
					Entry:   e[2],
				}
				// Data-name omitted entirely, which COBOL treats as FILLER.
				unnamed := &Field{
					Pos:     at(4, 4),
					Level:   5,
					Filler:  true,
					Picture: pic(t, "X(2)"),
					Parent:  rec,
					Entry:   e[3],
				}
				rec.Children = []*Field{a, filler, unnamed}
				return []*Field{rec}
			},
		},
		{
			name: "condition names attach to the item they follow",
			src: `01 REC.
   05 STATUS-CODE PIC X.
      88 ACTIVE VALUE 'A'.
      88 CLOSED VALUE 'C' 'D'.
   05 AMOUNT PIC 9(3).
      88 IN-RANGE VALUE 1 THRU 100.
`,
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				rec := &Field{Pos: at(1, 1), Level: 1, Name: "REC", Kind: KindGroup, Entry: e[0]}
				status := &Field{
					Pos:     at(2, 4),
					Level:   5,
					Name:    "STATUS-CODE",
					Picture: pic(t, "X"),
					Parent:  rec,
					Entry:   e[1],
				}
				status.Conditions = []*Condition{
					{
						Pos:  at(3, 7),
						Name: "ACTIVE",
						Values: []cobol.ValueSpec{
							{From: &cobol.StringLiteral{Pos: at(3, 23), Value: "'A'"}},
						},
						Parent: status,
						Entry:  e[2],
					},
					{
						Pos:  at(4, 7),
						Name: "CLOSED",
						Values: []cobol.ValueSpec{
							{From: &cobol.StringLiteral{Pos: at(4, 23), Value: "'C'"}},
							{From: &cobol.StringLiteral{Pos: at(4, 27), Value: "'D'"}},
						},
						Parent: status,
						Entry:  e[3],
					},
				}
				amount := &Field{
					Pos:     at(5, 4),
					Level:   5,
					Name:    "AMOUNT",
					Picture: pic(t, "9(3)"),
					Parent:  rec,
					Entry:   e[4],
				}
				amount.Conditions = []*Condition{
					{
						Pos:  at(6, 7),
						Name: "IN-RANGE",
						Values: []cobol.ValueSpec{
							{
								From:    &cobol.NumericLiteral{Pos: at(6, 25), Value: "1"},
								Through: &cobol.NumericLiteral{Pos: at(6, 32), Value: "100"},
							},
						},
						Parent: amount,
						Entry:  e[5],
					},
				}
				rec.Children = []*Field{status, amount}
				return []*Field{rec}
			},
		},
		{
			name: "condition names attach to a group item too",
			src: `01 REC.
   05 KEY-PART.
      10 A PIC X.
      88 WHOLE-KEY-BLANK VALUE SPACES.
`,
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				rec := &Field{Pos: at(1, 1), Level: 1, Name: "REC", Kind: KindGroup, Entry: e[0]}
				keyPart := &Field{
					Pos:    at(2, 4),
					Level:  5,
					Name:   "KEY-PART",
					Kind:   KindGroup,
					Parent: rec,
					Entry:  e[1],
				}
				a := &Field{
					Pos:     at(3, 7),
					Level:   10,
					Name:    "A",
					Picture: pic(t, "X"),
					Parent:  keyPart,
					Entry:   e[2],
				}
				// The condition-name follows A, so it qualifies A rather than
				// the group above it.
				a.Conditions = []*Condition{
					{
						Pos:    at(4, 7),
						Name:   "WHOLE-KEY-BLANK",
						Values: values(t, e[3]),
						Parent: a,
						Entry:  e[3],
					},
				}
				keyPart.Children = []*Field{a}
				rec.Children = []*Field{keyPart}
				return []*Field{rec}
			},
		},
		{
			name: "renames alias a range of the record's fields",
			src: `01 REC.
   05 FIRST-NAME PIC X(10).
   05 MIDDLE-NAME PIC X(10).
   05 LAST-NAME PIC X(10).
   66 WHOLE-NAME RENAMES FIRST-NAME THRU LAST-NAME.
   66 JUST-FIRST RENAMES FIRST-NAME.
`,
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				rec := &Field{Pos: at(1, 1), Level: 1, Name: "REC", Kind: KindGroup, Entry: e[0]}
				first := &Field{
					Pos:     at(2, 4),
					Level:   5,
					Name:    "FIRST-NAME",
					Picture: pic(t, "X(10)"),
					Parent:  rec,
					Entry:   e[1],
				}
				middle := &Field{
					Pos:     at(3, 4),
					Level:   5,
					Name:    "MIDDLE-NAME",
					Picture: pic(t, "X(10)"),
					Parent:  rec,
					Entry:   e[2],
				}
				last := &Field{
					Pos:     at(4, 4),
					Level:   5,
					Name:    "LAST-NAME",
					Picture: pic(t, "X(10)"),
					Parent:  rec,
					Entry:   e[3],
				}
				rec.Children = []*Field{first, middle, last}
				rec.Aliases = []*Alias{
					{
						Pos:     at(5, 4),
						Name:    "WHOLE-NAME",
						From:    first,
						Through: last,
						Fields:  []*Field{first, middle, last},
						Record:  rec,
						Entry:   e[4],
					},
					{
						Pos:    at(6, 4),
						Name:   "JUST-FIRST",
						From:   first,
						Fields: []*Field{first},
						Record: rec,
						Entry:  e[5],
					},
				}
				return []*Field{rec}
			},
		},
		{
			name: "a renamed range covers the fields subordinate to a group in it",
			src: `01 REC.
   05 A PIC X.
   05 GRP.
      10 B PIC X.
      10 C PIC X.
   05 D PIC X.
   66 A-THRU-GRP RENAMES A THRU GRP.
   66 JUST-GRP RENAMES GRP.
`,
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				rec := &Field{Pos: at(1, 1), Level: 1, Name: "REC", Kind: KindGroup, Entry: e[0]}
				a := &Field{Pos: at(2, 4), Level: 5, Name: "A", Picture: pic(t, "X"), Parent: rec, Entry: e[1]}
				grp := &Field{Pos: at(3, 4), Level: 5, Name: "GRP", Kind: KindGroup, Parent: rec, Entry: e[2]}
				b := &Field{Pos: at(4, 7), Level: 10, Name: "B", Picture: pic(t, "X"), Parent: grp, Entry: e[3]}
				c := &Field{Pos: at(5, 7), Level: 10, Name: "C", Picture: pic(t, "X"), Parent: grp, Entry: e[4]}
				d := &Field{Pos: at(6, 4), Level: 5, Name: "D", Picture: pic(t, "X"), Parent: rec, Entry: e[5]}
				grp.Children = []*Field{b, c}
				rec.Children = []*Field{a, grp, d}
				// The range runs to the end of GRP, so B and C are inside it
				// and D is not.
				rec.Aliases = []*Alias{
					{
						Pos:     at(7, 4),
						Name:    "A-THRU-GRP",
						From:    a,
						Through: grp,
						Fields:  []*Field{a, grp, b, c},
						Record:  rec,
						Entry:   e[6],
					},
					// Renaming a group alone covers the group's own storage,
					// which is the storage of its subordinate items.
					{
						Pos:    at(8, 4),
						Name:   "JUST-GRP",
						From:   grp,
						Fields: []*Field{grp, b, c},
						Record: rec,
						Entry:  e[7],
					},
				}
				return []*Field{rec}
			},
		},
		{
			name: "usage is inherited from the group unless overridden",
			src: `01 REC USAGE COMP-3.
   05 A PIC 9(3).
   05 GRP.
      10 B PIC 9(3).
      10 C PIC X(3) DISPLAY.
   05 D USAGE BINARY.
      10 E PIC 9(4).
`,
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				rec := &Field{
					Pos:   at(1, 1),
					Level: 1,
					Name:  "REC",
					Kind:  KindGroup,
					Usage: UsageComp3,
					Entry: e[0],
				}
				a := &Field{
					Pos:     at(2, 4),
					Level:   5,
					Name:    "A",
					Picture: pic(t, "9(3)"),
					Usage:   UsageComp3,
					Parent:  rec,
					Entry:   e[1],
				}
				// A group with no USAGE of its own inherits, and passes the
				// inherited usage on down.
				grp := &Field{
					Pos:    at(3, 4),
					Level:  5,
					Name:   "GRP",
					Kind:   KindGroup,
					Usage:  UsageComp3,
					Parent: rec,
					Entry:  e[2],
				}
				b := &Field{
					Pos:     at(4, 7),
					Level:   10,
					Name:    "B",
					Picture: pic(t, "9(3)"),
					Usage:   UsageComp3,
					Parent:  grp,
					Entry:   e[3],
				}
				c := &Field{
					Pos:     at(5, 7),
					Level:   10,
					Name:    "C",
					Picture: pic(t, "X(3)"),
					Usage:   UsageDisplay,
					Parent:  grp,
					Entry:   e[4],
				}
				// D states its own USAGE, which is then what E inherits.
				d := &Field{
					Pos:    at(6, 4),
					Level:  5,
					Name:   "D",
					Kind:   KindGroup,
					Usage:  UsageBinary,
					Parent: rec,
					Entry:  e[5],
				}
				eField := &Field{
					Pos:     at(7, 7),
					Level:   10,
					Name:    "E",
					Picture: pic(t, "9(4)"),
					Usage:   UsageBinary,
					Parent:  d,
					Entry:   e[6],
				}
				d.Children = []*Field{eField}
				grp.Children = []*Field{b, c}
				rec.Children = []*Field{a, grp, d}
				return []*Field{rec}
			},
		},
		{
			name: "decimal point is comma swaps the roles inside pictures",
			src: `01 REC.
   05 AMOUNT PIC ZZ9,99.
`,
			opts: []Option{WithDecimalPointIsComma()},
			want: func(t *testing.T, e []*cobol.DataDescriptionEntry) []*Field {
				rec := &Field{Pos: at(1, 1), Level: 1, Name: "REC", Kind: KindGroup, Entry: e[0]}
				amount := &Field{
					Pos:     at(2, 4),
					Level:   5,
					Name:    "AMOUNT",
					Picture: pic(t, "ZZ9,99", picture.WithDecimalPointIsComma()),
					Parent:  rec,
					Entry:   e[1],
				}
				rec.Children = []*Field{amount}
				return []*Field{rec}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			entries := parseFragment(t, tc.src)
			records, err := Build(entries, tc.opts...)
			require.NoError(t, err)
			require.Equal(t, tc.want(t, entries), records)
		})
	}
}

// mixedCopybook exercises every construct this package understands in one
// record: groups and elementary items, FILLER written and omitted, a
// condition-name, a RENAMES range, inherited and overridden USAGE, and a
// standalone level-77 item beside the record.
const mixedCopybook = `01 CUSTOMER-RECORD USAGE DISPLAY.
   05 CUST-ID PIC 9(6).
   05 CUST-NAME PIC X(30).
   05 CUST-BALANCE PIC S9(7)V99 COMP-3.
   05 CUST-STATUS PIC X.
      88 STATUS-ACTIVE VALUE 'A'.
      88 STATUS-CLOSED VALUE 'C' 'X'.
   05 FILLER PIC X(2).
   05 CUST-ADDRESS.
      10 ADDR-LINE PIC X(40).
      10 ADDR-ZIP PIC 9(5) COMP.
      10 PIC X(4).
   66 NAME-AND-BALANCE RENAMES CUST-NAME THRU CUST-BALANCE.
77 RECORD-COUNT PIC 9(6) COMP-3.
`

func TestBuildMixedCopybook(t *testing.T) {
	t.Parallel()

	entries := parseFragment(t, mixedCopybook)
	records, err := Build(entries)
	require.NoError(t, err)

	rec := &Field{
		Pos:   at(1, 1),
		Level: 1,
		Name:  "CUSTOMER-RECORD",
		Kind:  KindGroup,
		Usage: UsageDisplay,
		Entry: entries[0],
	}
	id := &Field{
		Pos:     at(2, 4),
		Level:   5,
		Name:    "CUST-ID",
		Picture: pic(t, "9(6)"),
		Parent:  rec,
		Entry:   entries[1],
	}
	name := &Field{
		Pos:     at(3, 4),
		Level:   5,
		Name:    "CUST-NAME",
		Picture: pic(t, "X(30)"),
		Parent:  rec,
		Entry:   entries[2],
	}
	balance := &Field{
		Pos:     at(4, 4),
		Level:   5,
		Name:    "CUST-BALANCE",
		Picture: pic(t, "S9(7)V99"),
		Usage:   UsageComp3,
		Parent:  rec,
		Entry:   entries[3],
	}
	status := &Field{
		Pos:     at(5, 4),
		Level:   5,
		Name:    "CUST-STATUS",
		Picture: pic(t, "X"),
		Parent:  rec,
		Entry:   entries[4],
	}
	status.Conditions = []*Condition{
		{
			Pos:    at(6, 7),
			Name:   "STATUS-ACTIVE",
			Values: values(t, entries[5]),
			Parent: status,
			Entry:  entries[5],
		},
		{
			Pos:    at(7, 7),
			Name:   "STATUS-CLOSED",
			Values: values(t, entries[6]),
			Parent: status,
			Entry:  entries[6],
		},
	}
	filler := &Field{
		Pos:     at(8, 4),
		Level:   5,
		Filler:  true,
		Picture: pic(t, "X(2)"),
		Parent:  rec,
		Entry:   entries[7],
	}
	address := &Field{
		Pos:    at(9, 4),
		Level:  5,
		Name:   "CUST-ADDRESS",
		Kind:   KindGroup,
		Parent: rec,
		Entry:  entries[8],
	}
	line := &Field{
		Pos:     at(10, 7),
		Level:   10,
		Name:    "ADDR-LINE",
		Picture: pic(t, "X(40)"),
		Parent:  address,
		Entry:   entries[9],
	}
	zip := &Field{
		Pos:     at(11, 7),
		Level:   10,
		Name:    "ADDR-ZIP",
		Picture: pic(t, "9(5)"),
		Usage:   UsageComp,
		Parent:  address,
		Entry:   entries[10],
	}
	pad := &Field{
		Pos:     at(12, 7),
		Level:   10,
		Filler:  true,
		Picture: pic(t, "X(4)"),
		Parent:  address,
		Entry:   entries[11],
	}
	address.Children = []*Field{line, zip, pad}
	rec.Children = []*Field{id, name, balance, status, filler, address}
	rec.Aliases = []*Alias{
		{
			Pos:     at(13, 4),
			Name:    "NAME-AND-BALANCE",
			From:    name,
			Through: balance,
			Fields:  []*Field{name, balance},
			Record:  rec,
			Entry:   entries[12],
		},
	}
	count := &Field{
		Pos:     at(14, 1),
		Level:   77,
		Name:    "RECORD-COUNT",
		Picture: pic(t, "9(6)"),
		Usage:   UsageComp3,
		Entry:   entries[13],
	}

	require.Equal(t, []*Field{rec, count}, records)
}

// TestBuildParentLinks checks the upward links independently of the tree
// comparison above, which builds its expected parents by hand and so could in
// principle agree with a Build that never set them.
func TestBuildParentLinks(t *testing.T) {
	t.Parallel()

	entries := parseFragment(t, mixedCopybook)
	records, err := Build(entries)
	require.NoError(t, err)

	require.Nil(t, records[0].Parent)
	require.Nil(t, records[1].Parent, "a level-77 item is a top-level item")

	var walk func(parent *Field)
	walk = func(parent *Field) {
		for _, child := range parent.Children {
			require.Same(t, parent, child.Parent)
			walk(child)
		}
		for _, cond := range parent.Conditions {
			require.Same(t, parent, cond.Parent)
		}
	}
	for _, record := range records {
		walk(record)
		for _, alias := range record.Aliases {
			require.Same(t, record, alias.Record)
		}
	}
}

func TestBuildErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		entries func(t *testing.T) []*cobol.DataDescriptionEntry
		target  any
		message string
	}{
		{
			name: "subordinate item with no record",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "05 ORPHAN PIC X.\n")
			},
			target:  &LevelSequenceError{},
			message: `level-05 item "ORPHAN" at line 1, column 1 has no containing record`,
		},
		{
			name: "subordinate item under a level-77 item",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "77 TOTAL PIC 9(3).\n   05 PART PIC 9.\n")
			},
			target:  &LevelSequenceError{},
			message: `level-05 item "PART" at line 2, column 4 has no containing record; a level-77 item takes no subordinate items`,
		},
		{
			name: "subordinate item under an elementary item",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "01 REC.\n   05 A PIC X.\n      10 B PIC X.\n")
			},
			target:  &LevelSequenceError{},
			message: `level-10 item "B" at line 3, column 7 is subordinate to "A", which has a PICTURE and so is an elementary item`,
		},
		{
			name: "condition name with nothing to qualify",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "88 LONELY VALUE 'A'.\n")
			},
			target:  &LevelSequenceError{},
			message: `level-88 item "LONELY" at line 1, column 1 condition-name has no preceding item to qualify`,
		},
		{
			name: "condition name on a renames entry",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "01 REC.\n   05 A PIC X.\n   66 AL RENAMES A.\n   88 C VALUE 'A'.\n")
			},
			target:  &LevelSequenceError{},
			message: `level-88 item "C" at line 4, column 4 follows a level-66 RENAMES entry; a condition-name on an alias is not represented by this package`,
		},
		{
			name: "renames entry with no record",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "66 NOPE RENAMES A.\n")
			},
			target:  &LevelSequenceError{},
			message: `level-66 item "NOPE" at line 1, column 1 RENAMES entry has no containing record`,
		},
		{
			name: "renames from an unknown field",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "01 REC.\n   05 A PIC X.\n   66 AL RENAMES B.\n")
			},
			target:  &RenamesError{},
			message: `level-66 entry "AL" at line 3, column 4: no field named "B" in "REC"`,
		},
		{
			name: "renames through an unknown field",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "01 REC.\n   05 A PIC X.\n   66 AL RENAMES A THRU B.\n")
			},
			target:  &RenamesError{},
			message: `level-66 entry "AL" at line 3, column 4: no field named "B" in "REC"`,
		},
		{
			name: "renames range running backwards",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "01 REC.\n   05 A PIC X.\n   05 B PIC X.\n   66 AL RENAMES B THRU A.\n")
			},
			target:  &RenamesError{},
			message: `level-66 entry "AL" at line 4, column 4: "A" does not follow "B"`,
		},
		{
			name: "picture that does not parse",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "01 REC.\n   05 A PIC SS9.\n")
			},
			target:  &PictureError{},
			message: `item "A" at line 2, column 9: invalid PICTURE "SS9": symbol S must appear at most once, as the leftmost symbol`,
		},
		{
			// The parser rejects a level number outside 01-49, 66, 77, and 88,
			// so only a hand-built entry reaches this check.
			name: "level number COBOL does not admit",
			entries: func(*testing.T) []*cobol.DataDescriptionEntry {
				return []*cobol.DataDescriptionEntry{
					{Pos: at(1, 1), Level: 50, Name: &cobol.Word{Pos: at(1, 4), Value: "TOO-DEEP"}},
				}
			},
			target:  &LevelError{},
			message: "invalid level number 50 at line 1, column 1: valid levels are 01-49, 66, 77, and 88",
		},
		{
			// COMP-6 is the live case of a usage-type the grammar admits and
			// this package does not map: the parser accepts the spelling so a
			// copybook using it can be read, and Build refuses it here rather
			// than inventing a width for it. Driving this through the real
			// parser is what keeps the two halves of that boundary honest — if
			// this package ever gains a COMP-6 member, this test fails.
			name: "usage type this package does not know",
			entries: func(t *testing.T) []*cobol.DataDescriptionEntry {
				return parseFragment(t, "77 ODD COMP-6.\n")
			},
			target:  &UsageError{},
			message: `item "ODD" at line 1, column 8: unknown USAGE "COMP-6"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			records, err := Build(tc.entries(t))
			require.Nil(t, records)
			require.Error(t, err)
			require.ErrorAs(t, err, tc.target)
			require.EqualError(t, err, tc.message)
		})
	}
}

func TestPictureErrorUnwraps(t *testing.T) {
	t.Parallel()

	entries := parseFragment(t, "01 REC.\n   05 A PIC SS9.\n")
	_, err := Build(entries)

	var picErr PictureError
	require.ErrorAs(t, err, &picErr)
	require.Equal(t, "SS9", picErr.Source)

	var placement picture.SymbolPlacementError
	require.ErrorAs(t, err, &placement, "the picture error underneath stays reachable")
}

func TestKindString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "elementary", KindElementary.String())
	require.Equal(t, "group", KindGroup.String())
}

func TestUsageString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		usage Usage
		want  string
	}{
		{UsageDisplay, "DISPLAY"},
		{UsageBinary, "BINARY"},
		{UsagePackedDecimal, "PACKED-DECIMAL"},
		{UsageComp, "COMP"},
		{UsageComp1, "COMP-1"},
		{UsageComp2, "COMP-2"},
		{UsageComp3, "COMP-3"},
		{UsageComp4, "COMP-4"},
		{UsageComp5, "COMP-5"},
		{UsageIndex, "INDEX"},
		{UsagePointer, "POINTER"},
		{Usage(-1), "DISPLAY"},
	}

	for _, tc := range testCases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.want, tc.usage.String())
		})
	}
}

// TestBuildParsesEveryUsageType pins the mapping from the parser's canonical
// usage-type spellings onto [Usage]: every spelling the grammar admits must
// build, since an unmapped one is a [UsageError] at run time.
func TestBuildParsesEveryUsageType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		clause string
		want   Usage
	}{
		{"DISPLAY", UsageDisplay},
		{"BINARY", UsageBinary},
		{"PACKED-DECIMAL", UsagePackedDecimal},
		{"COMP", UsageComp},
		{"COMP-1", UsageComp1},
		{"COMP-2", UsageComp2},
		{"COMP-3", UsageComp3},
		{"COMP-4", UsageComp4},
		{"COMP-5", UsageComp5},
		{"INDEX", UsageIndex},
		{"POINTER", UsagePointer},
	}

	for _, tc := range testCases {
		t.Run(tc.clause, func(t *testing.T) {
			t.Parallel()

			entries := parseFragment(t, "77 ITEM USAGE "+tc.clause+".\n")
			records, err := Build(entries)
			require.NoError(t, err)
			require.Len(t, records, 1)
			require.Equal(t, tc.want, records[0].Usage)
		})
	}
}
