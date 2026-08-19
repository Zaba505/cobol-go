# COBOL Data Representation Reference

## Overview

This document is the single source of truth for the **byte-level representation
of COBOL data items** — how a value described by a copybook is laid out in the
bytes of a data file. It is the reference the `picture`, `copybook`, and `codec`
packages are all built against, so that the three implement one agreed reading
of vendor documentation rather than three independent ones.

It is **distinct from the root [`SPEC.md`](../SPEC.md)**, which specifies COBOL
*source syntax* — the tokens and grammar of a program. The root spec deliberately
defers everything below:

- The **"PICTURE determines category"** bullet of the root spec's *Semantics*
  section states that a PICTURE string's symbols fix the item's *category*, but
  not how digits, scale, or sign are derived from it.
- The **"`USAGE` default is `DISPLAY`"** bullet, immediately below it, states
  that `USAGE` "change[s] the stored encoding but not the logical value" —
  without saying what any stored encoding actually is.

Both bullets now link here. They are cited by name rather than by line number
because a line number into a 1200-line document rots the moment either file is
edited — as the two references originally written here did, within this very
commit.

This document fills exactly that gap. Source syntax questions belong in the root
spec; byte-layout questions belong here.

### Scope

In scope: the byte layout of elementary data items for every `USAGE` a data file
can carry — `DISPLAY` (zoned decimal and character), `PACKED-DECIMAL`/`COMP-3`,
`BINARY`/`COMP`/`COMP-4`/`COMP-5`, `COMP-1`/`COMP-2` — across **both ASCII and
EBCDIC** files, together with the dialect and per-file settings that change those
layouts.

Out of scope, with reasons, in [Out of Scope](#out-of-scope).

### ASCII is a target, not a footnote

**Character set is a first-class axis of this specification, not an EBCDIC
default with an ASCII footnote.** ASCII-encoded copybook data files are a direct
user use case: files produced by GnuCOBOL and Micro Focus on Linux and Windows,
and mainframe-written files that have since been converted, are ASCII in the
field. Every encoding below states explicitly whether it is charset-sensitive,
and where ASCII has no single convention this document enumerates the competing
ones rather than picking one silently.

ASCII is in fact the *harder* of the two charsets for zoned decimal: EBCDIC has
one universal sign convention and ASCII has at least four incompatible ones in
production use. See [Zoned Sign Conventions](#zoned-sign-conventions).

### Governing sources

- **ISO/IEC 1989:2014 / ISO/IEC 1989:2023**, *Programming language COBOL* — the
  normative standard for PICTURE, `USAGE`, and `SIGN`. It fixes the *logical*
  model (digits, scale, sign) and leaves most byte-level detail
  implementor-defined, which is why the rest of this list is needed.
  <https://www.iso.org/standard/51416.html>
- **IBM Enterprise COBOL for z/OS Language Reference** and **Programming Guide**
  — the reference for EBCDIC zoned decimal, packed decimal, big-endian binary,
  `TRUNC`, and hexadecimal floating point.
- **z/Architecture Principles of Operation**, chapter 8 (Decimal Instructions)
  and chapter 9 (Floating-Point) — normative for which sign nibbles the hardware
  accepts and for the HFP format.
- **GnuCOBOL** `config/*.conf` (notably `binary-byteorder`, `binary-size`,
  `binary-truncate`, `display-sign`) and its `.ttbl` translation tables
  (`ebcdic500_ascii7bit`, `ebcdic500_ascii8bit`, `ebcdic500_latin1`, …). These
  are the most useful *primary enumeration of the knobs*: GnuCOBOL exposes as
  configuration exactly the axes that differ between dialects.
  <https://gnucobol.sourceforge.io/>
- **Micro Focus Visual COBOL / COBOL Server** documentation — the reference for
  the zone-3/7 ASCII sign convention and for `COMP` byte-order directives.

> **Ambiguity:** The ISO standard is paywalled and, for byte layout, largely
> silent by design. Where vendors differ this document does **not** choose a
> winner; it records the fork as a setting and states who takes which side. That
> is the whole point — a library that picks a side silently is the failure mode
> [Failure Modes](#failure-modes-silent-vs-loud) exists to prevent.

### Conformance language

**MUST**, **MUST NOT**, **SHOULD**, and **MAY** are normative requirements on the
`codec` package (see #72, #73, #74, #75, #76, #77). Everything else is
descriptive. A requirement written here is not a suggestion a downstream story
may weigh against ergonomics.

---

## The Four Axes of an Encoding

Four settings must be known before a single byte of a data file can be
interpreted. None of them is recoverable from the file itself with certainty,
and **every one of them fails silently when wrong** — a wrong value yields a
plausible but incorrect result rather than an error.

| Axis | Values | What it governs |
|---|---|---|
| **Charset** | EBCDIC (cp037, cp500, cp1047, cp1140, …), ASCII/identity | Alphanumeric character data; the digit zone of zoned decimal; the byte values of a separate sign |
| **Sign convention** | `SignEBCDIC`, `SignASCIIZone37`, `SignTranslatedEBCDIC`, `SignRealia` | How an overpunched sign is spelled in a zoned decimal byte |
| **Byte order** | Big-endian, little-endian (native) | `COMP`/`COMP-4`/`COMP-5` binary integers |
| **Float format** | IEEE-754, IBM HFP | `COMP-1`/`COMP-2` |

These are **independent axes, not one "dialect" flag.** The combination that real
files hit most often and that no compiler produces is a mainframe-written file
converted to ASCII: ASCII characters, *translated-EBCDIC* signs, big-endian
binary. A boolean "is it EBCDIC" cannot express it.

### Normative: caller-declared, no default

> `codec` **MUST** require all four axes from the caller and **MUST NOT** supply
> a default for any of them. Each **MUST** have an invalid zero value, and
> construction **MUST** fail with a typed error naming the missing axis. Named
> bundles (`IBMEnterprise()`, `MicroFocusASCII()`, `GnuCOBOLASCII()`,
> `ConvertedFromEBCDIC()`) **MAY** be provided as constructors that expand to a
> complete setting of all four — so that a caller *states* an assumption in one
> call — but a bundle **MUST NOT** be applied implicitly.

This is a requirement on #72 and #77, not a style preference. The justification
is in [Failure Modes](#failure-modes-silent-vs-loud): a library that guesses any
of these four can return wrong data with no indication at any layer, and the
caller has no way to discover it. Requiring the declaration converts an
undetectable data-corruption bug into a compile-time or construction-time
question.

---

## From PICTURE to Attributes

Every layout rule below is expressed in terms of four attributes derived from the
PICTURE character-string and the `SIGN` clause. Deriving them is the job of the
`picture` package (#71); this section is the contract.

### Category

Per the root spec's Semantics section, the set of symbols present fixes the
category:

| Symbols present | Category | Has a numeric value |
|---|---|---|
| only `9` `S` `V` `P` | **numeric** | yes |
| only `A` | **alphabetic** | no |
| `X`, or a mix including `X` and `9` | **alphanumeric** | no |
| `9` plus editing symbols (`Z * , . + - CR DB $ B 0 /`) | **numeric-edited** | yes, but de-editing is out of scope |
| `X`/`A` plus `B` `0` `/` | **alphanumeric-edited** | no |

Only **numeric** items have a `USAGE` other than `DISPLAY` in any meaningful
sense: a `USAGE` clause on a non-numeric item is either an error or ignored,
depending on dialect. All of [Zoned Decimal](#zoned-decimal-usage-display)
onward concerns numeric items; non-numeric items are covered in
[Alphanumeric and Alphabetic](#alphanumeric-and-alphabetic-items).

### Digits

Expand every repeat count first: `9(5)` ≡ `99999`, `S9(3)V9(2)` ≡ `S999V99`.

- **`digits`** = the number of `9` symbols. These are the *stored* digit
  positions and are what every width formula below is a function of.
- `P` positions are digit positions of the *value* but occupy **no storage** and
  are **not** counted in `digits`.
- `V` and `S` are not digit positions and occupy no storage (`S` occupies one
  byte only under `SIGN … SEPARATE`).

The standard limits `digits` to 18; IBM Enterprise COBOL with `ARITH(EXTEND)`
and GnuCOBOL both raise this to 31.

### Scale

Scale is defined by assigning an **exponent** to each digit position:

1. Locate the assumed decimal point. If `V` is present, the point is at `V`. If
   `V` is absent and the picture ends in a run of `P`, the point is immediately
   to the **right** of that run. If `V` is absent and the picture *begins* with a
   run of `P`, the point is immediately to the **left** of that run. Otherwise
   the point is at the right end.
2. The digit position immediately left of the point has exponent `0`, the next
   one left `1`, and so on; immediately right of the point is `-1`, then `-2`.
   `P` positions are counted here exactly like `9` positions.
3. **`scale` = −(exponent of the rightmost `9`)**, so that

       value = unscaled_integer × 10^(−scale)

   where `unscaled_integer` is the stored digits read as one integer.

| PICTURE | `digits` | `scale` | +12345 stored as | Value |
|---|---|---|---|---|
| `9(5)` | 5 | 0 | `12345` | 12345 |
| `9(3)V99` | 5 | 2 | `12345` | 123.45 |
| `V9(5)` | 5 | 5 | `12345` | 0.12345 |
| `9(5)PPP` | 5 | −3 | `12345` | 12345000 |
| `PPP9(5)` | 5 | 8 | `12345` | 0.00012345 |
| `9(5)P` | 5 | −1 | `12345` | 123450 |

The two properties that matter downstream: **`scale` never affects the number of
bytes** (`V` and `P` occupy no storage), and **`scale` is not recoverable from
the bytes**. A decoder therefore returns the unscaled integer and carries the
scale alongside; it is not a decoding input. This is why `codec`'s numeric
methods take `digits` but not `scale` (#72).

### Sign

- `S` present → the item is **signed** and carries an operational sign.
- `S` absent → the item is **unsigned**. Its stored representation carries the
  *unsigned* sign value for its encoding (zone `F`/`3`, packed nibble `F`); a
  negative value stored into it is stored as its absolute value.
- The `SIGN IS LEADING | TRAILING [SEPARATE CHARACTER]` clause applies **only**
  to `USAGE DISPLAY` signed numeric items. Default is `TRAILING`, non-separate.
  It has no effect on `COMP-3`, `COMP`, `COMP-1`, or `COMP-2`.

> **Naming.** *Sign position* (`LEADING`/`TRAILING`/`SEPARATE`) is a property of
> the copybook and comes from the PICTURE and `SIGN` clause. *Sign convention*
> (`SignEBCDIC`, `SignASCIIZone37`, …) is a property of the **file in hand** and
> cannot be read from the copybook at all. Both are needed and they are different
> axes; the naming split is normative on #73 and #77.

---

## Storage Widths

Width in bytes as a function of `digits` and `USAGE`. This is the whole of the
size model for elementary items.

| `USAGE` | Aliases | Width in bytes | Charset-sensitive |
|---|---|---|---|
| `DISPLAY`, numeric | — | `digits`, **+1** if `SIGN … SEPARATE` | **yes** |
| `DISPLAY`, alphanumeric/alphabetic | — | number of character positions | **yes** |
| `DISPLAY`, numeric-edited | — | number of character positions of the edited picture | **yes** |
| `PACKED-DECIMAL` | `COMP-3`, `COMPUTATIONAL-3` | `ceil((digits + 1) / 2)` | no |
| `COMP-6` (GnuCOBOL, Micro Focus) | — | `ceil(digits / 2)` — packed, **no sign nibble** | no |
| `BINARY` | `COMP`, `COMP-4`, `COMPUTATIONAL` | 2 / 4 / 8 by digit count (below) | no |
| `COMP-5` | native binary | same widths, different range semantics | no |
| `COMP-1` | `FLOAT-SHORT` | 4 | no |
| `COMP-2` | `FLOAT-LONG` | 8 | no |
| `INDEX` | — | 4 (IBM); platform-dependent elsewhere | no |
| `POINTER` | — | 4 (AMODE 31) or 8 (AMODE 64); platform pointer width | no |

### Binary widths by digit count

| `digits` | IBM Enterprise COBOL | GnuCOBOL `binary-size: 1-2-4-8` | GnuCOBOL `binary-size: 2-4-8` |
|---|---|---|---|
| 1–2 | 2 | **1** | 2 |
| 3–4 | 2 | 2 | 2 |
| 5–9 | 4 | 4 | 4 |
| 10–18 | 8 | 8 | 8 |
| 19–31 | 16 (`ARITH(EXTEND)`) | 16 | 16 |

The 1–2 digit row is a real, silent fork: `PIC S9(2) COMP` is **two** bytes under
IBM and **one** byte under GnuCOBOL's default `binary-size`. Because it changes
the width, it desynchronizes every field after it in the record — which makes it
one of the few silent settings that fails *loudly* in practice, via a record
length mismatch. GnuCOBOL also offers `binary-size: 1--8` (smallest width that
holds `digits`) and `binary-size: full` (always 8).

### Group items, OCCURS, and SYNCHRONIZED

- A group item's width is the sum of its subordinate items' widths. A group is
  always treated as alphanumeric when moved or compared, whatever its members'
  usages.
- `OCCURS n` multiplies the item's width by `n`. `OCCURS DEPENDING ON` makes the
  record's length variable; see #80.
- `SYNCHRONIZED` inserts **slack bytes** before an item to align it to its
  natural boundary, and may insert trailing slack in a group under `OCCURS`.
  Slack bytes are part of the record and change every subsequent offset. Layout
  computation including slack is #79's problem; this document specifies the
  elementary encodings only, and a reader **MUST NOT** assume a record is the
  simple sum of its fields' widths when `SYNCHRONIZED` is present anywhere.

---

## Charset as a First-Class Axis

Character set applies to *some* of the encodings below and not others, and the
line between them is the source of the single worst data-corruption trap in this
document.

| Encoding | Charset-sensitive | Why |
|---|---|---|
| Alphanumeric / alphabetic `DISPLAY` | **yes** | The bytes *are* characters |
| Numeric-edited `DISPLAY` | **yes** | Digits and insertion characters are characters |
| Zoned decimal `DISPLAY` | **yes** | Digit bytes are the *characters* `0`–`9`: `F0`–`F9` in EBCDIC, `30`–`39` in ASCII |
| Separate sign byte | **yes** | `+`/`-` are `2B`/`2D` in ASCII, `4E`/`60` in EBCDIC |
| Packed decimal `COMP-3` | **no** | Nibbles, never characters — identical bytes in both |
| Binary `COMP`/`COMP-4`/`COMP-5` | **no** | Two's complement integers |
| Floating point `COMP-1`/`COMP-2` | **no** | IEEE or HFP bit fields |
| `INDEX`, `POINTER` | **no** | Machine addresses; not valid interchange data at all |

> **Normative.** Charset translation **MUST** be applied to alphanumeric fields
> only. Numeric decoding **MUST** operate on raw byte values: digit bytes
> (`F0`–`F9` vs `30`–`39`) and overpunch zones are byte-level facts, not
> character-level ones, and routing them through a character translation both
> loses information and makes the sign convention unrepresentable. (#77)

### The mixed-record reality

Because half the table is charset-sensitive and half is not, **"an ASCII file" is
not a well-formed description of a record.** A record written on z/OS and
converted to ASCII has ASCII character fields, ASCII-with-translated-EBCDIC-signs
zoned fields, and packed and binary fields that either survived unchanged or were
destroyed — see [the COMP-3 trap](#the-comp-3-conversion-trap). A consumer
**MUST** be able to describe those independently, which is why charset and sign
convention are separate axes.

---

## Zoned Decimal (`USAGE DISPLAY`)

A signed or unsigned numeric item held as one character per digit. This is the
default `USAGE` and the most common numeric encoding in interchange files.

### Layout

`digits` bytes, most significant digit first. Each byte is two nibbles:

```
  high nibble        low nibble
 ┌───────────┬───────────┐
 │   zone    │   digit   │
 └───────────┴───────────┘
```

- The **digit nibble** is the BCD digit `0`–`9`. A digit nibble of `A`–`F` is
  invalid in every convention.
- The **zone nibble** is the charset's digit zone on every byte except the one
  carrying the sign: `F` in EBCDIC (so `F0`–`F9` = `0`–`9`), `3` in ASCII (so
  `30`–`39` = `0`–`9`).
- On an **unsigned** item (`S` absent) every byte carries the plain digit zone;
  no sign is stored and none is read. An unsigned zoned field is therefore
  **sign-convention-independent** and only charset-sensitive.

### Sign position

On a signed item, exactly one byte carries the sign, selected by the `SIGN`
clause:

| Clause | Sign byte | Width |
|---|---|---|
| `SIGN IS TRAILING` (default) | last byte | `digits` |
| `SIGN IS LEADING` | first byte | `digits` |
| `SIGN IS TRAILING SEPARATE CHARACTER` | an extra byte appended | `digits + 1` |
| `SIGN IS LEADING SEPARATE CHARACTER` | an extra byte prepended | `digits + 1` |

Non-separate signs are **overpunched**: the sign replaces the zone nibble of the
digit byte it lands on, so the sign costs no storage. Which zone value means what
is the sign convention.

### Zoned Sign Conventions

**EBCDIC has one convention. ASCII has at least four in production use, and they
are mutually incompatible.** This cannot be modelled as a boolean charset flag;
it is a selectable set, and choosing wrong yields silently wrong *signs* —
negative values read as positive — with no parse error.

Zone nibble (or whole byte) for the sign-carrying digit, by convention:

| Convention | Positive | Negative | Unsigned | Negative digits appear as |
|---|---|---|---|---|
| **`SignEBCDIC`** | zone `C` (`C0`–`C9`) | zone `D` (`D0`–`D9`) | zone `F` | `}`, `J`–`R` in EBCDIC |
| **`SignASCIIZone37`** | zone `3` (`30`–`39`) | zone `7` (`70`–`79`) | zone `3` | `p`–`y` |
| **`SignTranslatedEBCDIC`** | `7B`, `41`–`49` | `7D`, `4A`–`52` | `30`–`39` | `}`, `J`–`R` in ASCII |
| **`SignRealia`** | zone `3` (`30`–`39`) | zone `2` (`20`–`29`) | zone `3` | space, `!`–`)` |

Digit-by-digit, for the sign-carrying byte:

| Digit | EBCDIC + | EBCDIC − | Zone37 + | Zone37 − | Translated + | Translated − | Realia + | Realia − |
|---|---|---|---|---|---|---|---|---|
| 0 | `C0` | `D0` | `30` | `70` | `7B` `{` | `7D` `}` | `30` | `20` |
| 1 | `C1` | `D1` | `31` | `71` | `41` `A` | `4A` `J` | `31` | `21` |
| 2 | `C2` | `D2` | `32` | `72` | `42` `B` | `4B` `K` | `32` | `22` |
| 3 | `C3` | `D3` | `33` | `73` | `43` `C` | `4C` `L` | `33` | `23` |
| 4 | `C4` | `D4` | `34` | `74` | `44` `D` | `4D` `M` | `34` | `24` |
| 5 | `C5` | `D5` | `35` | `75` | `45` `E` | `4E` `N` | `35` | `25` |
| 6 | `C6` | `D6` | `36` | `76` | `46` `F` | `4F` `O` | `36` | `26` |
| 7 | `C7` | `D7` | `37` | `77` | `47` `G` | `50` `P` | `37` | `27` |
| 8 | `C8` | `D8` | `38` | `78` | `48` `H` | `51` `Q` | `38` | `28` |
| 9 | `C9` | `D9` | `39` | `79` | `49` `I` | `52` `R` | `39` | `29` |

Where each is found:

- **`SignEBCDIC`** — every EBCDIC file. IBM Enterprise COBOL, and any file
  written on z/OS, i, or VSE. Universal: there is no competing EBCDIC
  convention.
- **`SignASCIIZone37`** — Micro Focus, Microsoft COBOL, and GnuCOBOL when
  compiling for ASCII (`display-sign` / `-fsign=ASCII`). This is the *native*
  ASCII convention: a program that never saw a mainframe writes this.
- **`SignTranslatedEBCDIC`** — what an EBCDIC→ASCII **text conversion** produces
  from `SignEBCDIC` data, because cp037 `C0`–`C9` are `{ABCDEFGHI` and `D0`–`D9`
  are `}JKLMNOPQR`. Also what PL/I writes on ASCII platforms. A file with this
  convention was written by a mainframe and converted, and its packed fields are
  suspect ([see below](#the-comp-3-conversion-trap)).
- **`SignRealia`** — CA Realia COBOL. Rarer, but distinctive enough that
  supporting it costs one table row and misreading it corrupts every negative
  value.

#### Lenient reading (EBCDIC only)

z/Architecture decimal instructions accept more sign values than they generate.
For **`SignEBCDIC`** a reader **MAY** accept, and a strict reader **SHOULD**
report:

| Zone nibble | Meaning | Generated by |
|---|---|---|
| `A`, `C`, `E`, `F` | positive | `C` preferred; `F` for unsigned |
| `B`, `D` | negative | `D` preferred |

A writer **MUST** emit only `C` (positive), `D` (negative), and `F` (unsigned).
The lenient set applies to `SignEBCDIC` and to packed decimal; the three ASCII
conventions have no equivalent, and admitting extra zones there would destroy the
mutual detectability described below.

### `SIGN SEPARATE`

The sign occupies its own byte, and the digit bytes all carry the plain digit
zone. The separate sign byte is **charset-sensitive but convention-independent**:

| Sign | ASCII | EBCDIC |
|---|---|---|
| `+` | `2B` | `4E` |
| `-` | `2D` | `60` |

`SIGN … SEPARATE` requires `S` in the PICTURE. A separate-sign field is the one
zoned form that carries no sign-convention information at all — which makes it
both the safest form to write and the form that gives a reader nothing to
validate a convention guess against.

### Validation and detectability

> **Normative.** A reader **MUST** reject bytes that are invalid under the
> declared convention rather than coercing them. A sign byte of `7B` (`{`) read
> under `SignASCIIZone37` is not a valid digit, and **MUST** be a typed,
> offset-carrying error rather than a digit. (#77)

That requirement is what makes most wrong-convention mistakes loud. The four
conventions are **mutually detectable at the signed digit byte**: for every pair,
each convention's negative digit bytes are invalid under the other. Note that
this holds under the lenient EBCDIC reading too — the
[lenient set](#lenient-reading-ebcdic-only) admits extra *zone nibbles*
(`A`, `B`, `E`) that no ASCII convention uses, so it widens what `SignEBCDIC`
accepts without overlapping any of the other three.

| Byte seen | `SignEBCDIC` | `SignASCIIZone37` | `SignTranslatedEBCDIC` | `SignRealia` |
|---|---|---|---|---|
| `D5` (EBCDIC −5) | −5 | **invalid** (zone `D`) | **invalid** | **invalid** |
| `75` (`u`, zone37 −5) | **invalid** (zone `7`) | −5 | **invalid** | **invalid** |
| `4E` (`N`, translated −5) | **invalid** (digit nibble `E`) | **invalid** (zone `4`) | −5 | **invalid** |
| `25` (`%`, Realia −5) | **invalid** (zone `2`) | **invalid** (zone `2`) | **invalid** | −5 |
| `35` (`5`) | **invalid** (zone `3`) | +5 | unsigned 5 | +5 |

Two consequences worth stating plainly:

1. **A wrong charset on zoned data is loud**, because EBCDIC's `F0`–`F9` are not
   valid ASCII digit bytes and vice versa. The first zoned field of the first
   record catches it.
2. **Where two conventions agree they also decode identically.** `SignRealia`
   and `SignASCIIZone37` share their positive and unsigned encodings, so a file
   containing no negatives is indistinguishable *and* correctly decoded under
   either. Indistinguishability is only harmful where the readings differ, and
   for these four they never do.

The genuinely silent case is therefore narrow: an **unsigned** or
**`SIGN SEPARATE`** field carries no convention-specific byte at all, so no
reading of it can be wrong on this axis — and a signed field only reveals the
convention once a **negative** value appears, since the three ASCII conventions
all read `30`–`39` as a non-negative digit. A reader that has seen only
positives has not yet confirmed anything; it has merely not yet been
contradicted.

---

## Packed Decimal (`COMP-3` / `PACKED-DECIMAL`)

Two digits per byte, sign in the final low nibble.

### Layout

`ceil((digits + 1) / 2)` bytes. Reading the nibbles left to right: an optional
pad nibble, then `digits` digit nibbles, then one sign nibble.

```
  PIC S9(5) COMP-3, value +12345   →  1  2 | 3  4 | 5  C
                                      ─────┼──────┼─────
                                       12  |  34  |  5C

  PIC S9(4) COMP-3, value -1234    →  0  1 | 2  3 | 4  D
                                      ─────┼──────┼─────
                                       01  |  23  |  4D
                                       ↑ pad nibble
```

- **The pad nibble exists when `digits` is even**, because `digits + 1` nibbles
  is then odd and rounds up to a whole byte. It is the **high** nibble of the
  **first** byte.
- A writer **MUST** set the pad nibble to `0`. A reader **SHOULD** validate that
  it is `0` and report a typed error otherwise: a non-zero pad is the cheapest
  available signal that the field offset is wrong.
- Every digit nibble **MUST** be `0`–`9`; `A`–`F` in a digit position is a typed
  error.

### Sign nibble

| Nibble | Meaning | Emitted |
|---|---|---|
| `C` | positive | yes, preferred |
| `D` | negative | yes, preferred |
| `F` | unsigned (`S` absent) | yes |
| `A`, `E` | positive | accepted on read only |
| `B` | negative | accepted on read only |
| `0`–`9` | **invalid** | — |

A writer **MUST** emit `C`, `D`, or `F`. A reader **MUST** reject `0`–`9`; the
lenient set above mirrors z/Architecture decimal-instruction behaviour.

### `COMP-6`

`COMP-6` (GnuCOBOL, Micro Focus) is packed decimal with **no sign nibble at all**:
`ceil(digits / 2)` bytes, always unsigned, pad nibble when `digits` is odd. It is
a different encoding and **MUST NOT** be decoded as `COMP-3`.

Reading the nibbles left to right: an optional pad nibble, then `digits` digit
nibbles, and nothing after them.

```
  PIC 9(4) COMP-6, value 1234      →  1  2 | 3  4
                                      ─────┼─────
                                       12  |  34

  PIC 9(3) COMP-6, value 123       →  0  1 | 2  3
                                      ─────┼─────
                                       01  |  23
                                       ↑ pad nibble
```

- **The pad nibble exists when `digits` is odd** — the opposite parity from
  `COMP-3`, because no sign nibble makes the count up. It is the **high** nibble
  of the **first** byte, in the same place `COMP-3` puts its own.
- A writer **MUST** set the pad nibble to `0`. A reader **SHOULD** validate that
  it is `0` and report a typed error otherwise, exactly as for `COMP-3` and for
  the same reason: a non-zero pad is the cheapest available signal that the field
  offset is wrong.
- Every nibble of the digit run **MUST** be `0`–`9`. `A`–`F` anywhere in the
  field is a typed error, so none of `COMP-3`'s sign alphabet — `C`, `D`, `F`,
  and the lenient `A`, `B`, `E` — is accepted in the low nibble of the last byte.
  That is what turns a `COMP-3` field read at a `COMP-6` offset into a loud
  failure rather than a wrong number.
- A negative value is not encodable and **MUST** be rejected by a writer rather
  than stored as its magnitude.

The two widths **coincide at every odd digit count** and differ by a byte at
every even one: `ceil((d+1)/2)` and `ceil(d/2)` are both 3 for `d = 5`. So a
copybook that has the usage wrong shifts the record only half the time, and at
an odd digit count nothing but the nibbles can catch it.

They do catch it, and the **digit check** is what guarantees that. A
`PIC S9(5) COMP-3` field read as `PIC 9(5) COMP-6` puts its sign nibble — always
one of `A`–`F` — where a digit belongs, so the digit check fires on every such
field. The pad check fires as well whenever the value's leading digit is
non-zero, but it is not what the guarantee rests on: `01 23 4C` presents a pad
nibble of `0` and is rejected on the `C` alone.

### Fault precedence

A corrupt packed field almost never carries exactly one bad nibble. Of the
16,777,216 three-byte values a `PIC S9(4) COMP-3` field can hold, **15,384,000 —
91.7% — are invalid in more than one of the three roles at once**: a bad pad
*and* a bad digit, a bad digit *and* a bad sign, or all three. For
`PIC S9(5) COMP-3`, which has no pad nibble, the figure is 9,485,760, or 56.5%.
So for the large majority of the genuinely corrupt fields this validation exists
to catch, "the error the reader reports" is not determined by the checks above —
only by the order they are applied in.

**That order is normative.** A reader that finds more than one invalid nibble in
a `COMP-3` or `COMP-6` field **MUST** report the **earliest one in field order**:
nibbles read left to right, which is the pad nibble, then the digit nibbles from
most significant to least, then — for `COMP-3` — the sign nibble. The error type
follows from the role of the nibble reported, and the offset it carries is the
offset of the byte holding that nibble.

Three consequences, stated so that they cannot be read as accidents of an
implementation:

- A non-zero pad nibble beside a bad digit is a **pad** error, not a digit error.
- A bad digit beside an invalid sign nibble is a **digit** error, not a sign
  error.
- Of several bad digits, the **first** is reported — the most significant — not
  the last and not whichever is cheapest to find.

A reader that declines the pad check — it is a **SHOULD**, not a **MUST** — still
applies the checks it does perform in this order.

The reason to pin this rather than leave it to the implementation is that the
offset is already normative. [Failure Modes](#normative-consequences) requires
every such error to carry the byte offset at which the fault was found, and that
requirement says nothing at all if a field with faults at bytes 0 and 2 may name
either one. The two things that corrupt a packed field — a record whose offsets
have slipped, and a naive text conversion — both damage it from some point
onward, so the earliest bad byte is the one nearest to where the record actually
went wrong; naming a later byte points the diagnosis past the evidence. The rule
also survives reimplementation cheaply: "earliest in field order" is what a
nibble-parallel check reporting its lowest set lane produces on its own, so a
branchless rewrite has nothing to reproduce by hand.

### Charset invariance — and why that is a trap

**`COMP-3` is charset-invariant: a packed field holds identical bytes in an
"ASCII" file and an "EBCDIC" file.** No translation is applied to it, ever,
because its bytes are nibble pairs and were never characters.

This is a trap, not a convenience.

#### The COMP-3 conversion trap

A record containing packed fields that is put through a **naive, non-copybook-aware
EBCDIC→ASCII text conversion** — `iconv`, an FTP transfer in text mode, a
"convert this file to ASCII" utility — is **silently corrupted**. The translator
rewrites bytes that were never characters.

Worked example. `PIC S9(5) COMP-3`, value +12345, stored as:

```
  12 34 5C
```

Under a cp037→ISO-8859-1 text conversion, `5C` is EBCDIC `*`, which the
translator faithfully rewrites to ASCII `2A`. The bytes `12` and `34` are EBCDIC
control characters and are rewritten to whatever the table maps them to. The
field becomes:

```
  ?? ?? 2A
```

Decoded as `COMP-3`, the final byte now reads digit `2`, sign nibble `A` —
**and `A` is a valid positive sign under the lenient rule**. The field decodes
without error as **+1234 2**. A plausible number. No exception, no warning, no
way to tell from the value that anything happened.

Three things follow, and this document states them so that consumers validate
rather than assume:

1. **A file described as "ASCII" is often only ASCII in its character fields.**
   The description is about the character data; it says nothing about whether the
   packed fields survived.
2. **A converted file may carry already-damaged packed fields**, and no setting
   of any axis in this document can recover them. The damage happened before the
   reader saw the file.
3. A strict reader is the only defence available: rejecting non-`0` pad nibbles,
   digit nibbles `A`–`F`, and sign nibbles `0`–`9` turns *most* corrupted packed
   fields into loud errors. The example above is the residual case that slips
   through — which is why "validate the pad nibble" is a **SHOULD** on the
   reader rather than an optimisation.

The correct conversion of a record containing packed fields is copybook-aware:
translate the character fields, copy the packed and binary fields byte for byte.
A file that was converted otherwise should be treated as suspect.

---

## Binary (`COMP` / `COMP-4` / `BINARY` / `COMP-5`)

Two's complement integers, width per
[Binary widths by digit count](#binary-widths-by-digit-count).

### Byte order — an explicit fork

| Platform / compiler | Byte order |
|---|---|
| IBM Enterprise COBOL (z/OS, i, VSE) | **big-endian**, always |
| GnuCOBOL | `binary-byteorder: big-endian \| native` — a dialect config setting |
| Micro Focus on x86 | **native (little-endian)** in its own dialect; big-endian under IBM-compatibility directives |

`COMP-5` is defined as *native* byte order on every platform that documents it,
so on z/OS `COMP-5` is big-endian and on x86 it is little-endian. `COMP` and
`COMP-4` are the ones that vary by dialect setting.

Byte order **MUST** be a caller-declared axis. It is **weakly detectable**: a
value whose byte-swapped reading exceeds the PICTURE's decimal range is a strong
signal under `TRUNC(STD)` (see below), and real data is heavily biased toward
small magnitudes, so most binary fields are mostly zero in their high-order
bytes. Neither is a guarantee — `0100` big-endian is 256 and little-endian is 1,
both in range for `PIC S9(4)` — so detection is a diagnostic **MAY**, never a
substitute for the declaration.

### Range semantics: `TRUNC`

Two incompatible readings of what a binary item's PICTURE means:

| Setting | Range of `PIC S9(4) COMP` | Notes |
|---|---|---|
| `TRUNC(STD)` | −9999 … 9999 | Standard-conforming: value truncated to `digits` decimal digits on store |
| `TRUNC(BIN)`, `COMP-5` | −32768 … 32767 | Full range of the 2-byte storage |
| `TRUNC(OPT)` | unpredictable | Compiler assumes values fit; **do not rely on it** |

GnuCOBOL spells this `binary-truncate: yes` (≡ `TRUNC(STD)`) / `no`
(≡ `TRUNC(BIN)`).

`TRUNC` does not change the byte layout, so it does not change *decoding*. It
changes two other things:

- **Validation.** Under `TRUNC(STD)` a reader **MAY** report `|value| ≥ 10^digits`
  as a range error; that same value is legitimate under `TRUNC(BIN)`. This is the
  strongest available byte-order detector, and it is only available under
  `TRUNC(STD)`.
- **Encoding.** A writer targeting `TRUNC(STD)` **MUST** reject or truncate
  values outside the decimal range, matching what the compiler would have stored.

### Sign

An item without `S` is unsigned and holds the absolute value; decode it as an
unsigned integer of the storage width. An item with `S` is two's complement over
the full storage width.

---

## Floating Point (`COMP-1` / `COMP-2`)

`COMP-1` is 4 bytes, `COMP-2` is 8. A `PICTURE` is not permitted on either — the
usage alone fixes the format — so `digits` and `scale` do not apply.

### Two incompatible formats

| Format | Where | `COMP-1` | `COMP-2` |
|---|---|---|---|
| **IEEE-754** | Distributed platforms: GnuCOBOL, Micro Focus, everything writing ASCII files. Enterprise COBOL 6 under `FLOAT(NATIVE)` | binary32 | binary64 |
| **IBM HFP** (hexadecimal floating point) | z/OS, the default for Enterprise COBOL under `FLOAT(HEX)` | short (4) | long (8) |

As a practical rule: **ASCII/distributed files are effectively always IEEE and
z/OS files are effectively always HFP.** That correlation is strong enough to be
useful and weak enough that it **MUST NOT** be inferred — Enterprise COBOL 6's
`FLOAT(NATIVE)` writes IEEE into an otherwise thoroughly EBCDIC file.

### IBM HFP layout

Big-endian, and inherently so — HFP predates any little-endian IBM platform.

```
  short (COMP-1), 4 bytes:
   ┌─┬───────────┬────────────────────────────────────┐
   │S│ exponent  │            fraction                │
   │1│  7 bits   │              24 bits               │
   └─┴───────────┴────────────────────────────────────┘

  long (COMP-2), 8 bytes: same sign and exponent, 56-bit fraction
```

- Exponent is **excess-64, base 16**: the stored value is `exp - 64`.
- Fraction is a base-16 fraction with an implied radix point to its left — there
  is no implied leading 1 as in IEEE.
- `value = ±0.fraction₁₆ × 16^(exponent − 64)`
- True zero is all-zero bytes. HFP has **no NaN and no infinity** encoding.

### IEEE layout

Standard binary32 / binary64. Byte order follows the platform: big-endian on
z/OS, native on distributed platforms — a float in a little-endian file is
little-endian. In practice float format and byte order move together, but they
are declared separately because HFP is always big-endian regardless.

### Why this is silent

Neither format can detect the other, because every bit pattern is valid in both.
Worked example, the value 1.0 as `COMP-1`:

| Written as | Bytes | Read as IEEE | Read as HFP |
|---|---|---|---|
| IEEE 1.0 | `3F 80 00 00` | **1.0** | 0.03125 |
| HFP 1.0 | `41 10 00 00` | **9.0** | **1.0** |

An HFP 1.0 read as IEEE is 9.0. Not an error, not a NaN, not out of range — a
plausible number that will pass every sanity check downstream. This is the
cleanest illustration in this document of why the four axes are caller-declared.

The only weak signals available: HFP produces no NaN or infinity, so IEEE
exponent bits of all-ones are evidence of a misread; and HFP's exponent range
(16^±64) is far wider than binary32's, so HFP data read as IEEE clusters into
implausible magnitudes. Both are diagnostics a reader **MAY** offer. Neither is
detection.

---

## Alphanumeric and Alphabetic Items

`PIC X(n)`, `PIC A(n)`, and group items. Width is `n` bytes, one byte per
character position. **Charset-sensitive**: translation applies here and only
here.

- **Padding** is with the charset's space (`20` ASCII, `40` EBCDIC). A value
  shorter than the field is padded on the **right** by default, and on the
  **left** under `JUSTIFIED RIGHT`.
- Trailing spaces are indistinguishable from content. Whether a reader trims them
  is a `codec` policy decision (#72), not a property of the data.
- **Any** byte value may appear in an alphanumeric field, including bytes with no
  character in the declared charset. A translation table **MUST** be total: a
  reader **MUST NOT** fail on an untranslatable byte in an alphanumeric field,
  because such fields are routinely used to carry binary payloads. This is the
  one place in this document where coercion beats rejection, and the reason is
  that "invalid" is not a meaningful category for `PIC X`.
- A `codec` **SHOULD** offer a raw-bytes accessor alongside the translated-string
  accessor for exactly that case.

EBCDIC translation is **not** one table. cp037 (US/Canada), cp500
(international), cp1047 (Latin-1/Open Edition), and cp1140 (cp037 with the euro)
differ in the placement of `[`, `]`, `!`, `^`, `~`, and the currency symbol.
GnuCOBOL ships several (`ebcdic500_ascii7bit`, `ebcdic500_ascii8bit`,
`ebcdic500_latin1`, …) precisely because there is no single right answer. The
**digits `F0`–`F9`, the letters, and the space `40` are identical across all of
them**, which is why zoned decimal can be specified charset-generically above
while alphanumeric data cannot.

---

## Dialect Matrix

### By compiler

| Setting | IBM Enterprise COBOL | GnuCOBOL | Micro Focus |
|---|---|---|---|
| Default charset | EBCDIC (cp037/cp1047) | ASCII (EBCDIC via `.ttbl`) | ASCII |
| Zoned sign convention | `SignEBCDIC` | `SignASCIIZone37` (`display-sign`) | `SignASCIIZone37` |
| `SIGN SEPARATE` bytes | `4E`/`60` | `2B`/`2D` | `2B`/`2D` |
| Binary byte order | big-endian | `binary-byteorder: big-endian \| native` | native; big-endian under IBM directives |
| Binary widths | 2/4/8 | `binary-size` (`1-2-4-8` default) | 2/4/8, directive-dependent |
| Binary range | `TRUNC(STD\|BIN\|OPT)` | `binary-truncate: yes \| no` | directive-dependent |
| `COMP-1`/`COMP-2` | HFP (`FLOAT(HEX)`); IEEE under `FLOAT(NATIVE)` | IEEE | IEEE |
| `COMP-3` layout | identical | identical | identical |
| `COMP-6` | not supported | supported | supported |
| Max digits | 18, or 31 with `ARITH(EXTEND)` | 18, or 31 | 18, or 31 |

### By compiler vs by file — the distinction that matters

Some of the above is a property of the **compiler that will consume the data**.
Some is a property of the **file in hand**, and cannot be answered by knowing
which compiler is involved at all:

| Setting | Property of | Why |
|---|---|---|
| **Charset** | **the file** | A mainframe-written file may have been converted after it was written |
| **Zoned sign convention** | **the file** | `SignTranslatedEBCDIC` is produced by no compiler — it is produced by a *conversion* |
| **Binary byte order** | **the file** | Written by whatever wrote it, read by something else |
| **Float format** | **the file** | Same |
| `TRUNC` semantics | the compiler | Affects what values can be present, not how bytes are read |
| Binary widths | the compiler | Fixed at compile time, baked into the record layout |
| `COMP-6` support | the compiler | The copybook either uses it or does not |

The top four are why a "dialect" is a *bundle of defaults a caller may choose*
and never a property the library can infer. `ConvertedFromEBCDIC()` — ASCII
characters, translated-EBCDIC signs, big-endian binary — is a real and common
combination that **no compiler produces**, and it is only expressible because
these are independent axes.

---

## Failure Modes: Silent vs Loud

A setting is **silently-failing** when a wrong value yields a plausible but
incorrect value rather than an error. A setting is **loudly-failing** when a
wrong value produces bytes that cannot be valid, so the reader can report it.

This classification is the reason for the normative rules above, and it is a
requirement on `codec`: **every silently-failing setting is caller-declared with
no default** (#72, #77), and **every silently-failing setting that can be turned
into a loud one by validation MUST be** (#73, #74, #75).

### Classification

| Setting / condition | Class | Detectable from the bytes? |
|---|---|---|
| **Charset** (zoned numeric fields) | silent | **Yes, fully.** EBCDIC `F0`–`F9` are invalid ASCII digit bytes and `30`–`39` are invalid EBCDIC ones. Caught at the first zoned field |
| **Charset** (alphanumeric fields) | silent | **No.** Every byte is legal in `PIC X`; a wrong table yields mojibake, not an error |
| **Zoned sign convention** | silent | **Yes, at any signed non-separate field with a negative value.** All four conventions are mutually invalid at the sign byte ([table](#validation-and-detectability)). **No** for unsigned or `SIGN SEPARATE` fields — but those carry no convention-specific bytes, so no reading of them can be wrong |
| **Binary byte order** | silent | **Weakly.** A swapped value exceeding `10^digits` is conclusive under `TRUNC(STD)` only; small values with a zero low byte are undetectable |
| **Float format** (IEEE vs HFP) | silent | **Weakly.** HFP emits no NaN/Inf, so IEEE all-ones exponents are evidence; magnitude clustering is a heuristic. HFP 1.0 reads as IEEE 9.0 with no signal |
| **`TRUNC` semantics** | silent on read | **Weakly.** An absolute value ≥ `10^digits` proves the file is not `TRUNC(STD)`; the converse proves nothing |
| **Binary width** (`binary-size`) | **loud, indirectly** | Changes every subsequent field offset, so it desynchronizes the record and surfaces as a length mismatch or a nibble validation failure |
| **Wrong copybook / wrong offsets** | **loud, indirectly** | Same mechanism: packed and zoned validation fails almost immediately once offsets slip |
| **Corrupted `COMP-3`** (the conversion trap) | mostly loud | Pad-nibble, digit-nibble, and sign-nibble validation catch most cases. The residual silent case is real ([above](#the-comp-3-conversion-trap)) |
| Invalid digit nibble in packed/zoned | **loud** | Always. `A`–`F` in a digit position is never valid |
| Invalid sign nibble (`0`–`9`) in packed | **loud** | Always |
| Non-zero packed pad nibble | **loud** | Always, if validated — a **SHOULD** on the reader |
| Separate sign byte not `+`/`-` | **loud** | Always |
| Record shorter than the layout | **loud** | Always |

### Normative consequences

1. `codec` **MUST NOT** default any of charset, sign convention, byte order, or
   float format. (#72, #77)
2. `codec` **MUST** reject bytes invalid under the declared setting rather than
   coercing them, for zoned digits, zoned signs, packed digits, packed signs, and
   separate sign bytes. Every such error **MUST** carry the byte offset at which
   it was found, so that a bad byte deep inside a record is diagnosable. Where a
   packed field carries more than one fault — the common case, not the corner one
   — which fault that offset names is fixed by
   [Fault precedence](#fault-precedence). (#72, #73, #74, #77, #110)
3. `codec` **SHOULD** validate the packed pad nibble and **SHOULD** offer
   `TRUNC(STD)` range validation, because those are the two checks that convert
   otherwise-silent misconfiguration into a first-record failure.
4. Alphanumeric decoding is the documented exception: it **MUST NOT** fail on an
   untranslatable byte ([above](#alphanumeric-and-alphabetic-items)).

The goal these add up to: **as many wrong settings as possible fail at the first
bad record rather than at the end of a quarter.**

---

## Out of Scope

### Numeric-edited de-editing

Reading a numeric-edited item (`PIC ZZ,ZZ9.99-`, `PIC $$$,$$9.99CR`) back into a
number is **not specified here and not required of `codec`.**

Reason: editing is a *presentation* transform and is not reliably invertible.
Check protection (`*`) destroys leading digits, floating insertion (`$$$`,
`+++`) makes the sign and digit positions depend on the value's magnitude, `B`
and `/` insertion is ambiguous against data, and `BLANK WHEN ZERO` maps a whole
range of values onto spaces. De-editing needs the full parsed PICTURE and a set
of policy decisions about ambiguous input, which places it in the `picture`
package's territory, above `codec`, and behind its own story. Numeric-edited
items in a record are readable as **alphanumeric** — their width is well-defined
in [Storage Widths](#storage-widths) — which is enough to keep record offsets
correct.

### `national` / UTF-16 items

`PIC N(n)`, `USAGE NATIONAL`, `NSYMBOL(NATIONAL)`, and national-edited items are
**out of scope**.

Reason: they are UTF-16 (UCS-2 on older compilers), which makes them a
two-bytes-per-character encoding with its own byte-order question, its own
surrogate handling, and its own set of conversion tables — a materially
different problem from the single-byte charset axis this document specifies, and
one that would pull in `golang.org/x/text`-class tables that `codec` is required
not to depend on (#72). The target use case is single-byte copybook data files.
A national item's *width* is `2n` bytes, which is stated here only so that record
offsets remain computable when one appears.

### Also out of scope

- **`INDEX` and `POINTER`** as interchange data. Their widths are recorded in
  [Storage Widths](#storage-widths) so offsets can be computed, but their
  contents are machine addresses and subscript values with no meaning outside
  the process that wrote them.
- **`SYNCHRONIZED` slack-byte computation.** Specified as a caveat only; layout
  is #79.
- **Record formats** — RECFM=FB/VB, RDW headers, block descriptors, line
  terminators. These concern how records are delimited in a file, not how a data
  item is laid out in a record.
- **`OCCURS DEPENDING ON`** resolution (#80).

---

## Appendix A: Test Vectors

Concrete bytes for the encodings above, intended as shared fixtures for #73–#77.
`⎵` is a space.

### A.1 Zoned decimal, `PIC S9(5)`, `SIGN TRAILING` (default), non-separate

| Value | Charset | Convention | Bytes | Renders as |
|---|---|---|---|---|
| +12345 | EBCDIC | `SignEBCDIC` | `F1 F2 F3 F4 C5` | `1234E` |
| −12345 | EBCDIC | `SignEBCDIC` | `F1 F2 F3 F4 D5` | `1234N` |
| +12345 | ASCII | `SignASCIIZone37` | `31 32 33 34 35` | `12345` |
| −12345 | ASCII | `SignASCIIZone37` | `31 32 33 34 75` | `1234u` |
| +12345 | ASCII | `SignTranslatedEBCDIC` | `31 32 33 34 45` | `1234E` |
| −12345 | ASCII | `SignTranslatedEBCDIC` | `31 32 33 34 4E` | `1234N` |
| +12345 | ASCII | `SignRealia` | `31 32 33 34 35` | `12345` |
| −12345 | ASCII | `SignRealia` | `31 32 33 34 25` | `1234%` |

### A.2 Zoned decimal, unsigned and separate

| Item | Value | Charset | Bytes |
|---|---|---|---|
| `PIC 9(5)` | 12345 | EBCDIC | `F1 F2 F3 F4 F5` |
| `PIC 9(5)` | 12345 | ASCII | `31 32 33 34 35` |
| `PIC S9(5) SIGN LEADING SEPARATE` | −12345 | ASCII | `2D 31 32 33 34 35` |
| `PIC S9(5) SIGN LEADING SEPARATE` | −12345 | EBCDIC | `60 F1 F2 F3 F4 F5` |
| `PIC S9(5) SIGN TRAILING SEPARATE` | +12345 | ASCII | `31 32 33 34 35 2B` |
| `PIC S9(5) SIGN TRAILING SEPARATE` | +12345 | EBCDIC | `F1 F2 F3 F4 F5 4E` |
| `PIC S9(5) SIGN LEADING` (overpunch) | −12345 | EBCDIC | `D1 F2 F3 F4 F5` |

### A.3 Negative tests — each byte invalid under the named convention

| Field bytes | Read under | Expected |
|---|---|---|
| `31 32 33 34 7B` | `SignASCIIZone37` | error: digit nibble `B` at offset 4 |
| `31 32 33 34 75` | `SignTranslatedEBCDIC` | error: invalid sign byte at offset 4 |
| `31 32 33 34 25` | `SignASCIIZone37` | error: invalid zone `2` at offset 4 |
| `F1 F2 F3 F4 D5` | any ASCII convention | error at offset 0 (zone `F` is not a digit) |
| `31 32 33 34 35` | `SignEBCDIC` | error at offset 0 (zone `3` is not a digit) |

### A.4 Packed decimal — charset-invariant, identical bytes in both charsets

| Item | Value | Bytes |
|---|---|---|
| `PIC S9(5) COMP-3` | +12345 | `12 34 5C` |
| `PIC S9(5) COMP-3` | −12345 | `12 34 5D` |
| `PIC 9(5) COMP-3` | 12345 | `12 34 5F` |
| `PIC S9(4) COMP-3` | −1234 | `01 23 4D` |
| `PIC S9(4) COMP-3` | +1234 | `01 23 4C` |
| `PIC S9(3)V99 COMP-3` | −123.45 | `12 34 5D` (identical to `S9(5)`; scale is not stored) |
| `PIC 9(4) COMP-6` | 1234 | `12 34` |
| `PIC 9(3) COMP-6` | 123 | `01 23` (leading pad nibble; odd digit count) |

Negative tests: `12 34 5A` decodes as +12345 under the lenient rule; `12 34 55`
is an error (sign nibble `5`); `1A 34 5C` is an error (digit nibble `A`);
`F2 34 5C` is an error under a strict reader (non-zero pad nibble for
`PIC S9(4)`).

`COMP-6` negative tests: `12 3C` is an error for `PIC 9(4) COMP-6` (digit nibble
`C`; there is no sign nibble to accept it); `F1 23` is an error for
`PIC 9(3) COMP-6` (non-zero pad nibble); and −1 cannot be written into any
`COMP-6` field.

### A.5 Binary

| Item | Value | Big-endian | Little-endian |
|---|---|---|---|
| `PIC S9(4) COMP` | 1234 | `04 D2` | `D2 04` |
| `PIC S9(4) COMP` | −1234 | `FB 2E` | `2E FB` |
| `PIC S9(4) COMP` | 0 | `00 00` | `00 00` |
| `PIC S9(9) COMP` | 123456789 | `07 5B CD 15` | `15 CD 5B 07` |
| `PIC 9(4) COMP` | 65535 | `FF FF` | `FF FF` (`TRUNC(BIN)`/`COMP-5` only) |

Byte-order detection: `04 D2` read little-endian is 53764, which exceeds the
`PIC S9(4)` decimal range and is conclusive under `TRUNC(STD)`. `01 00` read
either way (256 or 1) is in range both ways and is **not** detectable.

### A.6 Floating point, value 1.0

| Item | Format | Bytes |
|---|---|---|
| `COMP-1` | IEEE binary32, big-endian | `3F 80 00 00` |
| `COMP-1` | IEEE binary32, little-endian | `00 00 80 3F` |
| `COMP-1` | IBM HFP short | `41 10 00 00` |
| `COMP-2` | IEEE binary64, big-endian | `3F F0 00 00 00 00 00 00` |
| `COMP-2` | IBM HFP long | `41 10 00 00 00 00 00 00` |

Cross-format misreads, for the silent-failure tests: `41 10 00 00` as IEEE is
**9.0**; `3F 80 00 00` as HFP is **0.03125**. Both decode without error.

### A.7 A mixed record

`01 TXN. 05 ID PIC X(4). 05 AMT PIC S9(5) COMP-3. 05 QTY PIC S9(4) COMP.
05 NAME PIC X(6).` — 4 + 3 + 2 + 6 = 15 bytes, `ID` = `A123`, `AMT` = −12345,
`QTY` = 1234, `NAME` = `WIDGET`.

| Encoding | Bytes |
|---|---|
| `IBMEnterprise()` (EBCDIC, big-endian) | `C1 F1 F2 F3` `12 34 5D` `04 D2` `E6 C9 C4 C7 C5 E3` |
| `MicroFocusASCII()` (ASCII, little-endian) | `41 31 32 33` `12 34 5D` `D2 04` `57 49 44 47 45 54` |
| `ConvertedFromEBCDIC()` (ASCII chars, translated signs, big-endian) | `41 31 32 33` `12 34 5D` `04 D2` `57 49 44 47 45 54` |

Note that the `COMP-3` field is byte-identical in all three rows — and that the
third row is what a *correct*, copybook-aware conversion of the first produces.
A naive text conversion of the first row would have rewritten the `12 34 5D`
bytes along with everything else.

---

## Appendix B: EBCDIC Overpunch Cross-Reference (cp037)

The origin of `SignTranslatedEBCDIC`: what a cp037→ASCII text conversion does to
a zoned sign byte.

| EBCDIC byte | cp037 character | ASCII byte | Digit | Sign |
|---|---|---|---|---|
| `C0` | `{` | `7B` | 0 | + |
| `C1`–`C9` | `A`–`I` | `41`–`49` | 1–9 | + |
| `D0` | `}` | `7D` | 0 | − |
| `D1`–`D9` | `J`–`R` | `4A`–`52` | 1–9 | − |
| `F0`–`F9` | `0`–`9` | `30`–`39` | 0–9 | unsigned |
| `4E` | `+` | `2B` | — | separate + |
| `60` | `-` | `2D` | — | separate − |
| `40` | space | `20` | — | pad |

Two collisions worth naming so they are not mistaken for errors:

- ASCII `4E` is the translated-EBCDIC **negative 5**, and EBCDIC `4E` is the
  separate-sign **`+`**. Different charsets, no ambiguity within one reading, but
  the same two hex digits appear in this document meaning both.
- ASCII `2B`/`2D` are the separate-sign bytes and are also perfectly ordinary
  `+`/`-` characters in an alphanumeric field. Nothing distinguishes them but the
  copybook.

---

## Appendix C: Mapping to Stories

| Section | Implemented by |
|---|---|
| [From PICTURE to Attributes](#from-picture-to-attributes) | #71 `picture` |
| [Storage Widths](#storage-widths) | #71, #79 `copybook` |
| [The Four Axes](#the-four-axes-of-an-encoding), alphanumeric | #72 `codec` |
| [Charset as a First-Class Axis](#charset-as-a-first-class-axis), [Zoned Sign Conventions](#zoned-sign-conventions) | #77 `codec` |
| [Zoned Decimal](#zoned-decimal-usage-display) | #73 `codec` |
| [Packed Decimal](#packed-decimal-comp-3--packed-decimal) | #74 `codec` |
| [`COMP-6`](#comp-6) | #99 `codec` |
| [Fault precedence](#fault-precedence) | #110 `codec` |
| [Binary](#binary-comp--comp-4--binary--comp-5) | #75 `codec` |
| [Floating Point](#floating-point-comp-1--comp-2) | #76 `codec` |
| Record tree, offsets, `OCCURS DEPENDING ON` | #78, #79, #80 `copybook` |

## Appendix D: Related Standards

- ISO/IEC 1989:2014 / ISO/IEC 1989:2023 — *Programming language COBOL*.
  <https://www.iso.org/standard/51416.html>
- IBM Enterprise COBOL for z/OS — *Language Reference* and *Programming Guide*.
  <https://www.ibm.com/docs/en/cobol-zos>
- z/Architecture *Principles of Operation*, ch. 8 (Decimal Instructions), ch. 9
  (Floating-Point Overview). <https://www.ibm.com/docs/en/zos>
- GnuCOBOL Programmer's Guide and `config/*.conf` dialect files.
  <https://gnucobol.sourceforge.io/>
- IEEE 754-2019 — *Standard for Floating-Point Arithmetic*.
- The root [`SPEC.md`](../SPEC.md) — COBOL **source syntax**, the companion to
  this document.
