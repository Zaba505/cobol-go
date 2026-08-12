# codec Package - Claude Memory

The `codec` package reads and writes the **bytes of a COBOL data file**. The
root package's tokenizer/parser/printer pattern documented in the top-level
`CLAUDE.md` does **not** apply here: this is a **binary** file library, laid out
as `types.go` / `decoder.go` / `encoder.go`.

```
bytes ── Reader ─► Go values ── Writer ─► bytes
   │         │                      │
   │     decoder.go             encoder.go
   └───────────── types.go ─────────────┘
```

`codec/SPEC.md` is the normative contract. Its **MUST**/**SHOULD** language is a
requirement on this package, not a suggestion to weigh against ergonomics, and
Appendix C maps each of its sections to the story that implements it.

## The rule that matters: no default encoding, ever

[Encoding] carries the four axes — `Charset`, `Sign`, `ByteOrder`, `Float` —
and **every one is required with an invalid zero value**. `Charset` and
`binary.ByteOrder` are interfaces so nil is detectable; `SignConvention` and
`FloatFormat` have `…Unset` zero values.

All four fail *silently* when wrong: the wrong value yields a plausible but
incorrect number rather than an error, at any layer, with no way for the caller
to discover it. So:

- Never add a default, a fallback, or an inference from the bytes to any axis.
- Never let a new constructor take fewer than a complete `Encoding`.
- The named bundles (`IBMEnterprise`, `MicroFocusASCII`, `GnuCOBOLASCII`,
  `ConvertedFromEBCDIC`) are values a caller passes, never something the package
  applies on its own. A new bundle must fill in all four fields and must pass
  `Encoding.Validate` — `TestDialects` asserts that.

## Standard library only

Non-test files import **nothing outside `std`**, and
`TestPackageImportsOnlyStandardLibrary` fails the build if that stops being
true. Generated file libraries link this package alone; they must never pull in
the parser in the root package. Test files may use `testify`, as the rest of the
module does.

Anything needing `picture` or `copybook` belongs on the *generator* side, not
here. Numeric accessors take `digits` but **not** `scale`: byte width never
depends on scale, so the return is the unscaled integer and the generator emits
the scale as a constant.

Their `digits` bound belongs to the **accessor**, not to the field: 4 for the
int16 accessors, 9 for the int32 ones, 18 for the int64 and uint64 ones, 31 —
the IBM maximum — for the `math/big.Int` ones. A wider count is a
`PackedDigitCountError`, a `BinaryDigitCountError` or a `ZonedDigitCountError`
rather than a silent overflow.

Numeric **writers** additionally take a `Signedness` (`Signed`/`Unsigned`) with
an invalid zero value. Whether the PICTURE carries `S` selects the stored sign
value and is not recoverable from the value being written, so it is stated per
call and never defaulted — the same argument as the `Encoding` axes, one level
down. A negative value written into an `Unsigned` field is a `PackedRangeError`,
not a silent absolute value.

The zoned accessors are the exception and are stricter rather than looser: a
`SignPosition` takes the place of the `Signedness` and is required on the
**reading** side too, because it is also what says whether the field is `digits`
bytes wide or `digits+1`. See below.

## Errors: one wrapper, typed leaves

Every error returned after construction is wrapped once in
`*OffsetError{Offset, Err}`, so a bad byte deep inside a record is diagnosable.
The offset is stamped in exactly one place per direction — `Reader.read` and
`Writer.write`, the only two methods that move `off` — so it cannot drift.
Never construct an `OffsetError` outside those paths without a reason, and never
add a second offset field to a leaf error.

There is one standing exception, and it is the shape any later one should take:
`Reader.readPackedDigits` stamps a bad nibble with the offset of the **byte
holding it**, computed from the field's start offset in a single `nibbleAt`
helper. A packed field is several bytes wide, and "the field ended at offset N"
does not say which byte was corrupt.

The second such exception is `BinaryRangeError`, stamped with the offset the
field **starts** at rather than the one it ends at, for the same reason: a
binary field is several bytes wide and a range error is a statement about the
whole field, most often that `Encoding.ByteOrder` is wrong.

The third is `FloatRangeError`, stamped by `Reader.ReadFloat32` with the offset
the field **starts** at for the same reason as `BinaryRangeError`.

Leaves are typed values (`EncodingError`, `FieldWidthError`,
`FieldTooLongError`, `UnrepresentableRuneError`, `JustificationError`,
`SignednessError`, `SignPositionError`, `ZonedDigitError`, `ZonedSignError`,
`ZonedSeparateSignError`, `ZonedDigitCountError`, `ZonedRangeError`,
`PackedDigitCountError`, `PackedPadError`,
`PackedDigitError`, `PackedSignError`, `PackedRangeError`,
`BinaryDigitCountError`, `BinaryRangeError`, `FloatRangeError`) or stdlib
sentinels
(`io.EOF`, `io.ErrUnexpectedEOF`, `io.ErrShortWrite`, `ErrNilValue`). Callers
use `errors.Is` for the cause and `errors.As` for the offset; tests assert both.

Loud beats silent everywhere except one documented place: **alphanumeric
decoding must never fail**. Any byte may appear in a `PIC X` field, such fields
carry binary payloads, so `Charset.ToUnicode` is total over all 256 bytes.
Numeric decoding is the opposite — reject bytes invalid under the declared
setting rather than coercing them.

A rejected field writes **nothing**. Validate first, build the whole field, then
write it, so a failure cannot leave a half-field behind and desynchronize the
record.

## Packed decimal: COMP-3 and COMP-6 are separate bodies on purpose

`COMP-6` (GnuCOBOL, Micro Focus) is packed decimal with **no sign nibble**, so
`comp6Width` is `ceil(digits / 2)` against `packedWidth`'s `ceil((digits+1)/2)`
and the pad nibble lands on the **opposite parity** — odd digit counts, not
even. `readComp6Digits`/`writeComp6` are their own bodies rather than
`readPackedDigits`/`writePacked` with a flag: the digits run to the end of the
field, there is no sign to return or to emit, and the two differences interact.

Three consequences to preserve:

- The `Comp6` accessors take **no `Signedness`**, on either side. The encoding
  has nowhere to record one, so it is not a value to declare — this is the one
  numeric family where the argument's absence is the point. A negative value is
  a `PackedRangeError{Signedness: Unsigned}` on write.
- **No nibble may be `A`–`F`**, including the low nibble of the last byte. None
  of `COMP-3`'s sign alphabet is accepted, which is what makes a `COMP-3` field
  read at a `COMP-6` offset a `PackedDigitError` rather than a wrong number.
- The errors are the packed ones (`PackedDigitCountError`, `PackedPadError`,
  `PackedDigitError`, `PackedRangeError`) reused, not a parallel set. Only the
  widths and the parity differ.

The two widths **coincide at every odd digit count** — `packedWidth(5)` and
`comp6Width(5)` are both 3 — and differ by a byte only at even ones. So a
mis-declared usage desynchronizes the record half the time and is otherwise
invisible except through the nibbles. The **"no `A`–`F` anywhere" check is what
guarantees it is caught** — a COMP-3 sign nibble always lands in a digit
position — while the pad check only fires when the leading digit happens to be
non-zero. Both stay: the pad check is the cheap offset signal, the digit check
is the guarantee. `TestPackedAndComp6Widths` pins the width relationship; do
not "simplify" either width formula against the other.

## Binary items: two families, one axis each

Binary (COMP, COMP-4, BINARY, COMP-5) has two forks and they are **not** the
same kind of thing, so they are declared in two different places:

- **Byte order** is a property of the *file* and comes from
  `Encoding.ByteOrder`. Never hard-code big-endian, never default it, never
  infer it from the bytes.
- **Range semantics** (`TRUNC(STD)` vs `TRUNC(BIN)`/`COMP-5`) is a property of
  the *compiler*, so it is not an `Encoding` axis. It is selected by which
  accessor is called: `ReadBinaryInt16` and friends are `TRUNC(STD)` and
  validate what they read against the PICTURE's decimal range, `ReadComp5Int16`
  and friends are `TRUNC(BIN)` and validate nothing beyond the storage width.
  That read-side validation is deliberate — it is the only detector the package
  has for a wrong byte order — and it must not be dropped to make a value
  "just read".

Width is `binaryWidth`, a staircase (2/4/8/16) in the digit count and never the
digit count itself. GnuCOBOL's `binary-size: 1-2-4-8` one-byte variant is not
implemented; a copybook compiled under it desynchronizes rather than reading
wrongly, which the SPEC classifies as loud-indirectly.

Signedness on the read side is the *method name* (`ReadBinaryUint64` is the
unsigned reading of the same bytes); on the write side it is the `Signedness`
argument, as it is for packed.

## Floating point: one axis, and one exception to every other rule

`COMP-1` (4 bytes) and `COMP-2` (8) take **no PICTURE**, so `ReadFloat32`,
`ReadFloat64`, `WriteFloat32` and `WriteFloat64` take **no `digits` and no
`Signedness`** — the two arguments every other numeric accessor requires. Do not
add either "for consistency": there is no digit count to declare, no scale, and
no `S` clause to select a sign convention from.

`Encoding.Float` is the whole of the fork, and it is the package's cleanest
example of a silent failure: every bit pattern is valid in both formats. IEEE
`1.0` is `3F 80 00 00` and reads as `0.03125` under HFP; HFP `1.0` is
`41 10 00 00` and reads as `9.0` under IEEE.

**HFP ignores `Encoding.ByteOrder`** and is always big-endian — it predates any
little-endian IBM platform. IEEE follows the axis. The float tests state the
byte order axis as little-endian in the HFP cases precisely so that a reader or
writer that started consulting it would fail them.

The conversion lives in `hfpFromFloat` / `floatFromHFP` in `types.go`:
sign-magnitude, a 7-bit excess-64 **base-16** exponent, and a normalized base-16
fraction with **no implied leading one**. Three consequences worth keeping in
mind:

- Normalizing to a hex digit boundary costs up to 3 bits of fraction. A float64
  survives HFP long exactly (56 bits leave room for the 3); a float32 may not
  survive HFP short (24 leave none), so round-trip fixtures use values with at
  most three significant hex digits.
- HFP has no NaN, no infinity and no negative zero, and its range (16^±64) is
  neither a subset nor a superset of a float64's. Anything unrepresentable is a
  `FloatRangeError` and writes nothing — never an infinity, never a flush to
  zero.
- The reader accepts **unnormalized** fields (z/OS arithmetic produces them);
  the writer emits only normalized ones. That asymmetry is deliberate and is why
  byte-equality fixtures use patterns the writer would have produced.

## Charsets

`Charset` is an interface, not an enum, because nil must be detectable and
because EBCDIC is not one table — cp037, cp500, cp1047 and cp1140 differ in
where they put brackets, currency and accents. `ASCII()` and `CP037()` ship
here, and they are the **only** two that ever will: another page is a caller's
own implementation (`encoding/charmap` wrapped, say), which is what keeps the
std-only promise above from being a limitation.

Both shipped tables are **bijective over all 256 bytes**, which is what lets
alphanumeric data round-trip unchanged; `TestCharsetIsTotalAndBijective`
asserts it for every charset, so a new one gets that check for free by being
added to its table.

Charset *translation* applies to **alphanumeric fields only**. Numeric decoding
works on raw byte values — digit zones and overpunch signs are byte-level facts,
and routing them through a character translation makes the sign convention
unrepresentable. The declared charset still matters to a numeric field: it says
whether the digits are `F0`–`F9` or `30`–`39` and whether a separate sign is
`4E`/`60` or `2B`/`2D`. Compare those byte values; do not call `ToUnicode` on
them. `TestZonedDecodingNeverTranslatesThroughTheCharset` counts the
translations a zoned field performs and requires zero, and
`TestZonedAccessorsNeverTranslateThroughTheCharset` does the same one layer up,
through `Reader` and `Writer`.

## Zoned bytes: two halves that must not merge

The bytes of a `USAGE DISPLAY` field come from two independent places, and
keeping them apart is the whole of why the four sign conventions are mutually
detectable:

- **Plain digit bytes are a charset fact.** `zonedBytesOf(cs)` derives them, and
  the separate sign bytes, by asking the `Charset` for the bytes of `'0'`–`'9'`,
  `'+'` and `'-'`. Derived, never switched on a known page — that is what lets a
  plugged-in cp1047 work, and `oddballCharset` in the tests (digits at `B0`–`B9`)
  is there to fail anything hard-coded.
- **The sign-carrying byte is a sign-convention fact and is charset
  independent.** `zonedSignTables` holds absolute byte values, one row per
  convention, transcribed from `SPEC.md`'s digit-by-digit table.

`signByte` / `signByteValue` are the byte-level pair, `zonedCodec.encodeField` /
`decodeField` the field-level one; `signAt` is the index the sign is
overpunched into, and `-1` means unsigned or `SEPARATE` (plain zone throughout).
`decodeField` returns the **index within the field** of the offending byte
alongside its error, for the caller to stamp as `start+at` — the same reason
`readPackedDigits` reports the byte holding a bad nibble.

Two asymmetries to preserve:

- **Lenient reading is EBCDIC's alone.** Zones `A`, `C`, `E`, `F` read positive
  and `B`, `D` negative, because z/Architecture accepts more than it generates;
  a writer emits only `C`, `D`, `F`. All the lenient zones are above `0x9F`, so
  this widens `SignEBCDIC` without overlapping any ASCII convention. Do **not**
  add a lenient set to the ASCII three: that would destroy the mutual
  detectability `TestZonedSignConventionsAreMutuallyDetectable` asserts.
- **An unsigned-zone byte in the sign position is a non-negative value**, not a
  corruption — a signed item holds one after a `MOVE` from an unsigned one.

## Zoned accessors: position is the copybook's half

`SignPosition` (`SignUnsigned`, `SignTrailing`, `SignLeading`,
`SignTrailingSeparate`, `SignLeadingSeparate`, with an invalid
`SignPositionUnset` zero) is the *copybook* half of a zoned field, where
`SignConvention` is the *file* half. It is passed to every zoned accessor in
both directions, and it does three things at once: it says whether the PICTURE
carries `S` — so it replaces `Signedness` here rather than joining it — it
selects the overpunched byte through `SignPosition.overpunchAt`, and it fixes
the width through `zonedWidth`, which is `digits` plus one only for a `SEPARATE`
sign. Do not give it a default: assuming `TRAILING` for a field that was
`SEPARATE` shifts every later field of the record by a byte.

`Reader.readZonedDigits` and `Writer.writeZoned` are the two field-level bodies.
They split the separate sign byte off, hand the rest to `zonedCodec`, and stamp
`start+first+at` so an error names the byte at fault — `first` being 1 under
`SignLeadingSeparate`, which is the one offset arithmetic worth re-checking when
this code changes.

The `zonedCodec` itself is derived **once**, in `NewReader`/`NewWriter`, and any
failure of deriving it is held in `zonedErr` and reported by the first zoned
accessor. Deriving it cannot fail the constructor: a charset with no `'+'`
cannot spell a numeric field but reads `PIC X` ones perfectly well.

## Testing style

Table-driven, `t.Parallel()` at both the function and each subtest, `require`
rather than `assert`, hex byte literals with a comment naming the field. Two
round-trip shapes, both required for every accessor pair:

1. **decode → encode → byte-equal**, which proves the trimming a reader does is
   undone by the padding a writer does.
2. **encode → decode → value-equal**.

`testRecord` in `decoder_test.go` is the shared fixture standing in for
generated code; extend it rather than growing a second one.
