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

[Encoding] carries the five axes — `Charset`, `Sign`, `ByteOrder`, `Float`,
`Binary` — and **every one is required with an invalid zero value**. `Charset`
and `binary.ByteOrder` are interfaces so nil is detectable; `SignConvention`,
`FloatFormat` and `BinarySize` have `…Unset` zero values.

All five fail *silently* when wrong: the wrong value yields a plausible but
incorrect number rather than an error, at any layer, with no way for the caller
to discover it. `Binary` fails one step worse — it changes how many *bytes* a
binary field occupies, so the record shifts rather than one value going wrong,
and a `Reader` cannot detect that because it never knows a record's length. So:

- Never add a default, a fallback, or an inference from the bytes to any axis.
- Never let a new constructor take fewer than a complete `Encoding`.
- The named bundles (`IBMEnterprise`, `MicroFocusASCII`, `GnuCOBOLASCII`,
  `ConvertedFromEBCDIC`) are values a caller passes, never something the package
  applies on its own. A new bundle must fill in all five fields and must pass
  `Encoding.Validate` — `TestDialects` asserts that. `GnuCOBOLASCII` is
  `BinarySize1248`, because that is GnuCOBOL's default `binary-size`; every
  other bundle is `BinarySize248`.

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
The offset is stamped in exactly one place per direction — `Reader.readInto`
and `Writer.write`, the only two methods that move `off` — so it cannot drift.
`Reader.read` and `Reader.readOwned` are the two buffer policies above
`readInto` (below) and neither touches `off` itself, which is what keeps the
owning path and the reusing one from drifting apart.
Never construct an `OffsetError` outside those paths without a reason, and never
add a second offset field to a leaf error.

There is one standing exception, and it is the shape any later one should take:
`Reader.readPackedField` stamps a bad nibble with the offset of the **byte
holding it**, computed from the field's start offset in a single `nibbleAt`
helper. A packed field is several bytes wide, and "the field ended at offset N"
does not say which byte was corrupt.

**Which** bad nibble that is, when there is more than one, is normative and not
an artefact of the scan: both `readPackedField` and `readComp6Field` check
nibbles in **field order** — pad, digits most significant first, then (COMP-3
only) the sign — and report the **earliest** fault, so the offset names the
first byte that went wrong. Most corrupt fields carry several faults at once, so
this is the common path rather than a corner of it; `SPEC.md`'s "Fault
precedence" states it and `TestPackedFaultPrecedence` /
`TestComp6FaultPrecedence` pin it. Do not reorder either body's checks, and do
not report the last bad digit instead of the first.

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
`BinaryDigitCountError`, `BinaryRangeError`, `BinaryAccessorRangeError`,
`FloatRangeError`) or stdlib
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

## Reading buffers: reused by default, allocated on request

`Reader.readInto` fills a buffer the caller supplies and is the only thing that
moves the offset, and `Reader.checkWidth` is the one precondition both policies
share, held in one place for the same reason. Two methods sit on them and they
are the whole of the buffer policy:

- **`read(n)` returns a buffer the `Reader` owns and reuses.** Its bytes are
  valid until the next read and no further, so every caller must consume them
  before returning. This is the path all but one accessor takes.
- **`readOwned(n)` allocates.** `ReadBytes` is its only caller, because
  `ReadBytes` hands the slice to the user; its doc comment promises the slice is
  the caller's own, and that promise is why the method cannot share the reused
  buffer. Do not "optimize" it onto `read`.

The reused side is two buffers, because the two families are bounded
differently:

- `Reader.num` is a **fixed array inline in the struct**, sized
  `maxNumericWidth` — a `max` over `zonedWidth`, `packedWidth`, `comp6Width`,
  `binaryWidth` and the two float widths at each family's own digit maximum. It
  is **derived, never written as `32`**: 31 digits is a dialect ceiling
  (`SPEC.md`, "18, or 31 with `ARITH(EXTEND)`") rather than a fact about COBOL,
  and a literal would smash the buffer the day it moved.
  `TestNumericScratchFitsEveryNumericUsage` checks the const against the width
  functions, and checks it for *equality* with their maximum so it cannot
  silently become oversized either.
- `Reader.wide` is a **growable slice** for anything longer. `PIC X(n)` is
  bounded only by the record, so alphanumeric fields wider than the array land
  here, and so — deliberately — would a numeric field wider than the array, at
  the cost of one allocation instead of a panic. Raising a digit maximum can
  therefore never be a runtime fault on a legal field.

Why it matters at all: a fresh `make` per field is not expensive because it is a
`make`. An identical one whose result does not escape is stack allocated and
free. It is expensive because `read` *returned* the slice, forcing it onto the
heap — one allocation per field before any value exists. Removing the escape
took `BenchmarkDecodeRecord` from 20 allocs/record to 9 and 507 ns to 399, and
#116 took it to 6 by folding the packed and COMP-6 nibbles rather than
unpacking them into a slice.

**Both buffers are sliced with a full slice expression** — `r.num[:n:n]`, not
`r.num[:n]`. A slice into a reused buffer carrying spare capacity is one
`append` in one callee away from writing over the next field's bytes, silently
and with no bounds panic; capping makes "these `n` bytes and no more" something
the runtime enforces. Do not drop the third index.

Four invariants a change here must keep, all pinned by tests:

- **Nothing an accessor returns may view the reused buffer.**
  `zonedCodec.decodeField` allocates its own, and the packed and COMP-6 bodies
  return only numbers — `readPackedField`/`readComp6Field` hand back the
  reused buffer itself, and every caller folds it into an `int64` or a
  `big.Int` before returning, which is what makes this hold today (#116).
  `TestReadValuesDoNotAliasTheReusedBuffer` reads two records with one `Reader`
  and requires the first record's values to survive the second — but on its own
  that test cannot fail, because every value `testRecord` holds is a string or a
  number and Go's own string conversion would copy the bytes anyway. Two tests
  give it teeth: `TestReadResultIsVolatile` states the premise directly (what
  `read` returns *is* overwritten by the next read, and its capacity *is* the
  field width), and `TestReadBytesIsTheOnlyAccessorHandingOutBytes` reflects
  over the exported methods and fails the moment a second `[]byte`-returning
  accessor appears — which is the regression the two-record test would sit
  through.
- **`ReadBytes` allocates.** It is the only accessor on `readOwned`, and its doc
  comment promises the caller owns the slice.
- **No read-ahead.** `read` slices to `n` and never to the buffer's capacity, so
  the `io.Reader` still holds every byte no field asked for.
  `TestReaderReadsNoFurtherThanTheFieldsAsked` requires that, reading its wide
  field with `ReadAlphanumeric` rather than `ReadBytes` so the growable buffer
  — the one whose capacity outlives the field — is actually on the path.
- **The buffer does not escape.** `TestReadingDoesNotAllocate` requires
  **zero** allocations from `ReadBinaryInt16`, `ReadFloat32` and `ReadFloat64`,
  each of which returns a value with no backing array of its own, so any
  allocation at all is the field's bytes reaching the heap. See the benchmarks
  section for why zero is assertable where a count is not.

## Sources and destinations: a stream, or bytes the caller holds (#115, #131)

A `Reader` reads an `io.Reader` **or** a `[]byte`, and a `Writer` writes an
`io.Writer` **or** appends to a `[]byte`. Which one is decided by a single
field being nil — `Reader.r`, `Writer.w` — which is unambiguous because
`NewReader`/`NewWriter` reject a nil interface. `NewBytesReader` and
`NewBytesWriter` are the byte-backed constructors, and `Unmarshal`/`Marshal`
are their first callers.

- **The byte source is a slice on the struct, not a `*bytes.Reader` behind the
  interface.** Wrapping puts back the per-record allocation the whole story
  exists to remove: `Unmarshal` went 11 -> 10 allocs/record and 607 -> 520 ns
  by not wrapping. The cost is 25 bytes of struct, which took `Reader` from 512
  to 544 and so into the next size class — `BenchmarkNewReader` 266 -> 274 ns,
  512 -> 576 B, one allocation either way. That trade is disclosed rather than
  smoothed over, exactly as the `maxAlphaScratch` one above it is, and the pair
  of benchmarks that disagree about it are the same pair. Every ns/op figure in
  this section is one machine's, measured on the pull request that landed the
  change; the allocation counts are the ones to compare.
- **Which arm runs is an explicit `fromBytes`/`toBytes` field, never a nil check
  on `r`/`w`.** This is the one thing about the two-source design that is not
  obvious and cost a review round. "Nil stream means read the bytes" makes the
  **zero value usable**: a `Reader{}` nobody constructed reads as an empty
  record and answers `io.EOF`, and a `Writer{}` goes further and *succeeds*,
  appending bytes under an `Encoding` with none of its five axes set. Both are
  exactly the silent failure the no-default rule at the top of this file exists
  to prevent. With the field they take the stream arm and fail on the nil
  interface, as they did before there were two kinds of source, and
  `TestZeroValueIsStillUnusable` pins that.
- **`Reader.data` is the unread remainder and is consumed by reslicing**, not
  indexed at `off`. The two are equal today, but `off` is what `Offset` and
  every `OffsetError` mean by "bytes read", and indexing a slice with it welds
  that meaning to "position in the source": the first accessor to move one
  without the other turns the read path into an out-of-range panic mid-decode.
  Reslicing keeps `off` a pure counter.
- **Both arms live inside `readInto` and `write`.** Those are the two methods
  that move `off` and stamp `OffsetError`, and the rule that they are the only
  ones is what keeps a second source from becoming a second error-stamping
  path. `TestReaderResetErrorsMatchTheStream` holds the byte arm to the
  stream's errors, offsets included: a short field is `io.ErrUnexpectedEOF` and
  an empty one `io.EOF`, which is `io.ReadFull`'s contract restated by hand.
- **The byte arm copies into the scratch; it does not reslice the source.**
  Reslicing would be faster and would break both of the package's aliasing
  invariants at once — an accessor could hand out a window into the caller's
  record, and `read`'s promise that its result dies at the next read would stop
  being true of every source. `TestReaderResetReadsSeveralRecords` overwrites
  every record after decoding them all and requires the values to stand.
- **`Reset` truncates its buffer where `NewBytesWriter` appends to it.** A
  rewind has to: `Offset` reporting 0 means the next byte written is the first
  byte `Bytes()` returns. The constructor is a destination rather than a rewind,
  so it keeps what the caller put there — a header, say. Both doc comments say
  so and point at each other, because the argument and its name are identical
  and only the semantics differ.
- **`Reset` on a stream-backed `Writer` is a change of destination**, and the
  doc says plainly that nothing further reaches the stream and that a pool of
  stream `Writer`s is not what this method makes possible. It cannot be made to
  report that: `Reset` has no error return, and this package raises no panics.
  Pool the byte-backed ones, build one `Writer` per stream.
- **`Reset` keeps everything the `Encoding` derived** — the zoned tables, the
  alpha table lookup, the scratch buffers, the writer's capacity — and the
  `Encoding` itself cannot change. A swappable encoding under a half-read
  record is the silent failure this package exists to prevent, so a different
  one needs a different `Reader`. This is what makes pooling worth it:
  `BenchmarkResetDecodeRecord` is 268 ns/9 allocs against `Unmarshal`'s
  520/10, and `BenchmarkResetEncodeRecord` 393/14 against `Marshal`'s 631/16.
- **The retention is documented and tested, not incidental.** Both sides hold
  the caller's slice until the next `Reset` and no longer; `Reset(nil)` is what
  a pooled codec passes on the way back, and `TestReaderReset` pins both halves
  of that.
- **The rewind exists on both sources, and the stream one is called
  `ResetStream` (#131).** `Reset(data []byte)` served only a caller holding a
  record's bytes; a caller reading from a stream — `cpybkc`'s delimited and
  line-sequential framings, which hand `codec` the same `*bufio.Reader` they
  draw the framing from — had no way to reuse a codec at all, so the
  amortisation `#115` exists to offer was unreachable from the input type most
  callers start with. `ResetStream(io.Reader)` and `ResetStream(io.Writer)`
  close that, and they keep exactly what `Reset` keeps.
  - **The name.** Go cannot overload, and `Reset` is taken by the byte-shaped
    half, so the stream half needed a second name. `ResetReader`/`ResetWriter`
    would mirror `bufio.Reader.Reset(io.Reader)`'s *argument* while stuttering
    against the receiver — `Reader.ResetReader` reads as "reset the reader's
    reader" — and would be **two** names for one operation, one per side, where
    the operation is identical. `ResetStream` is one name on both types, says
    what changes rather than what the argument is, and pairs with `Reset` as
    "onto a stream" against "onto bytes". It matches nothing in the standard
    library, which is the cost, and it is the smaller cost. Both `Reset` doc
    comments name it, because a caller holding an `io.Reader` finds `Reset`
    first and would otherwise conclude the package cannot serve them.
  - **A nil argument means the hand-back, on all four methods.** `Reset(nil)` was
    already the pooling shape — hold nothing, keep nothing of the caller's alive
    — and `ResetStream(nil)` is spelled as exactly that: it delegates to
    `Reset(nil)`, so the codec falls into the byte-backed arm over an empty
    source. A read then answers `io.EOF` and a write goes to a buffer nobody
    reads, rather than either panicking on a nil interface. That is what keeps
    the answer from being "panics on the next read" without inventing a third
    state, and `TestZeroValueIsStillUnusable` is untouched by it: a codec
    *nobody constructed* still takes the stream arm and still panics, because
    its `Encoding` was never validated and nothing must make it look workable.
  - **Rewinding onto the same stream is the point**, and it is what makes
    `Offset` and every `OffsetError` record-relative on a stream, as they
    already were on bytes. Nothing is read ahead — the `Reader` does not buffer,
    which `#108` measured and declined — so the caller's framing reads the byte
    after the last field. `TestReaderResetStream` pins that with a `bufio.Reader`
    shared between the codec and the test's own delimiter read, which is the
    adopter's shape.
  - **Each `ResetStream` drops the other arm's source** — `data = nil`,
    `buf = nil` — rather than leaving it behind an unused discriminator. The
    retention promise is "until the next rewind and no longer", and a rewind
    onto a stream is a rewind.
  - **What it is worth**, measured on the pull request that landed it (go1.26.2,
    AMD Ryzen 9 5950X, `-benchtime 2s -count 5`, `benchRecord` under
    `GnuCOBOLASCII`): `BenchmarkResetStreamDecodeRecord` 321 ns / 37 B / 6
    allocs against `BenchmarkStreamDecodeRecord`'s 580 / 613 / 7, and
    `BenchmarkResetStreamEncodeRecord` 403 / 80 / 14 against
    `BenchmarkStreamEncodeRecord`'s 623 / 528 / 15. The allocation removed is
    the codec itself — one per record, and the 576-byte `Reader` is most of the
    bytes; the ns/op gap is that allocation plus the encoding-derived work no
    longer redone. It lands the stream-shaped caller on the same figures the
    byte-shaped one has had since #115.
- **The `Writer` is still not buffered.** It has no `Flush` and no `Close`, so
  holding bytes back from an `io.Writer` would truncate any caller that just
  stops writing. A byte-backed `Writer` is a different thing: the bytes it
  appends *are* its output, handed over by `Bytes()`. What it does own is a
  growth policy — `Writer.grow`, geometric with a 64-byte floor — because
  `append`'s own growth reaches a record's width in four allocations where one
  will do, and `Marshal` pays that per record. Dropping the floor regresses
  `BenchmarkMarshalRecord` from 16 allocs/record to 19, which is worse than the
  `bytes.Buffer` it replaced. Both arms of that policy are pinned by tests
  rather than by the benchmark alone — `TestBytesWriterFirstFieldJumpsToTheFloor`
  and `TestBytesWriterGrowsForAFieldWiderThanTheFloor` — because CI runs the
  tests and never the benchmarks.

## Packed decimal: COMP-3 and COMP-6 are separate bodies on purpose

`COMP-6` (GnuCOBOL, Micro Focus) is packed decimal with **no sign nibble**, so
`comp6Width` is `ceil(digits / 2)` against `packedWidth`'s `ceil((digits+1)/2)`
and the pad nibble lands on the **opposite parity** — odd digit counts, not
even. `readComp6Field`/`writeComp6` are their own bodies rather than
`readPackedField`/`writePacked` with a flag: the digits run to the end of the
field, there is no sign to return or to emit, and the two differences interact.

Neither body unpacks the field into a slice of digits. `nibbleOf(b, i)` reads
nibble `i` of the field's own bytes, the digits are the contiguous run
`first` … `first+digits-1`, and each caller folds that run as it walks it, so an
integer packed or COMP-6 read allocates nothing at all, and the `Big` accessors
are the only ones in either family still reaching the heap — for the `big.Int`
they return and the temporaries it is folded with, never for the field (#116). Index a nibble at a time
rather than loading a word: `Reader.read` returns a buffer sliced to the
field's width, so a wider load runs off an owned slice or, behind the reused
buffer, reads the *next* field's bytes and reports a neighbour as this field's
bad nibble. `TestPackedReadsNoFurtherThanItsOwnBytes` reads the narrowest field
of each usage behind a poisoned scratch and pins that.

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

Width has a **third** fork, and unlike the other two it is an `Encoding` axis:

- **The width staircase** is `Encoding.Binary`, a `BinarySize`. There are four —
  `2-4-8`, `1-2-4-8`, `1--8` and `full` — and they disagree from the first digit
  count: `PIC S9(2) COMP` is two bytes under the first and one under the second.
  `BinarySize.width(digits)` is the whole of the size model, and **nothing
  outside a test may call `binaryWidth`**, which is now only the *bound*
  `maxNumericWidth`'s derivation is checked against (`BinarySizeFull`'s
  staircase, the widest of the four) and reads 8 for a two-digit item. Every
  byte path called it before the axis existed, so that is the mistake to expect;
  `TestBinaryWidthIsNotOnAnyBytePath` walks the AST of every non-test file and
  fails on a call, rather than leaving the rule to a doc comment.
  `Reader.readBinaryField`, `Reader.readBinaryBig` and `Writer.binaryField` are
  the three places the axis is consulted, and encode and decode move together or
  a round trip silently changes a record's length.

`BinarySize.width`'s `default:` arm returns `maxBinaryFieldWidth` and not the
2-4-8 staircase. It is unreachable — `Encoding.Validate` rejects an unset or
unknown axis — and answering with a plausible width would make it a default in
everything but name, which is what the axis exists to prevent. Sixteen bytes
fails loudly instead. Do not "simplify" it back to a fallthrough.

`codec.BinarySize` mirrors `copybook.BinarySize` member for member and width for
width, and cannot be the same type because this package imports only `std`.
`copybook`'s `TestBinarySizeAgreesWithCodec` pins the two against each other at
every digit count 1–31, measuring codec's side through `Writer.Offset` so the
agreement is asserted over public behaviour rather than over a copied table. A
change to either staircase that is not made to both fails there.

`BinarySizeSmallest` is the one that gives 3, 5, 6 and 7-byte fields, which is
why `binaryUint`/`putBinaryUint` have a path for widths `binary.ByteOrder` has
no accessor for, and `BinarySizeFull` is the one that can make a field wider than
the Go type an accessor returns — that is `BinaryAccessorRangeError`, a read-side
error saying the bytes are fine and the accessor is too narrow, as against
`BinaryRangeError` saying the bytes are wrong for the field.

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

## The alphanumeric translation table: derived once, cached per charset

`ReadAlphanumericJustified` does **not** call `ToUnicode` per byte. It goes
through `alphaTable`, the charset's translation materialised as UTF-8: `enc[c]`
packs the encoding of byte `c` into a `uint32` little-endian and `width[c]` says
how many of those four bytes count, so the inner loop stores a whole rune's
worth unconditionally and advances by the width — no branch, no interface
dispatch, no per-rune re-encode. `BenchmarkReadAlphanumeric` measured 2.1x on its
ASCII-mapping corpus and 2.9x on its multi-byte one, and `BenchmarkDecodeRecord`
went 403 to 333 ns.

Five things about it are load bearing, and all five cost something to
rediscover:

- **It is not a `[256]byte`, and there is no "ASCII is verbatim" shortcut.**
  128 of cp037's bytes spell runes above U+007F, and `ASCII()` is no better —
  its `ToUnicode` is `rune(b)`, the identity in *rune* space, so 0xE9 spells
  U+00E9 and costs two UTF-8 bytes. Encoding all 256 values of either charset
  takes 384 bytes. `TestAlphaTableMatchesWriteRuneForEveryByte` and
  `TestReadAlphanumericMatchesWriteRuneUnderEveryCharset` hold the result
  byte-identical to the `ToUnicode` + `WriteRune` loop it replaced, over every
  byte of every charset in the suite — including one whose runes need four UTF-8
  bytes and one whose `ToUnicode` returns values that are not characters at all.
- **It is built lazily, on the first alphanumeric read, and never in
  `NewReader`.** `TestZonedAccessorsNeverTranslateThroughTheCharset` counts
  `ToUnicode` calls from construction onward and requires **zero**;
  materialising 256 entries at construction breaks that the moment a `Reader`
  exists. `TestNewReaderTranslatesNothing` states the premise on its own.
- **It is cached per `Charset` in `alphaTables`, not per `Reader`.** `Unmarshal`
  builds a `Reader` per record, so a per-`Reader` table is amortised over one
  record and the per-record path regresses. The key is an interface, which is a
  panic waiting to happen: `Charset` promises nothing about comparability, so
  `comparableCharset` answers from the type — reading an interface-typed field
  as *not* comparable, conservatively — before anything reaches the map. Do not
  replace it with `reflect.Value.Comparable`: it is the exact answer, but it
  walks per *value* rather than per type and allocates, and this runs once per
  `Reader` and so once per record. Do not wrap the map access in a `recover`
  either, because `sync.Map`'s store path panics with its mutex held. The cache
  is an `alphaCache` value rather than loose package variables so that both of
  its edges — a hit returning the same table, and a full cache still returning a
  correct one — are testable without touching process state.
- **Padding comes off the source bytes, before translation.** `alphaTable.trim`
  strips the bytes that spell U+0020 — every such byte, never `Charset.Space()`,
  a distinction the field's own doc comment spells out. It agrees with trimming
  the translated string because 0x20 is neither a UTF-8 lead byte nor a
  continuation byte. Doing it first is what makes the 32-byte inline scratch
  worth having: a real field is mostly padding, so a `PIC X(30)` name holding
  eleven characters needs 25 bytes of scratch and never reaches the growable
  buffer.
- **A charset that cannot be cached gets no table, not an uncached one.**
  `alphaTableOf` returns nil and `Reader.readAlphanumericPerByte` does what this
  package always did. An uncached table was the first design and is the one
  answer worse than none: 256 `ToUnicode` calls and a 1.5KiB allocation *per
  record*, against about 22 calls for the loop it replaces. What is left is a
  measured 10% on `BenchmarkUnmarshalRecord/uncached` — the struct growth and one
  branch — and the remedy is the caller's: the **public** `Charset` doc comment
  now tells them to make the implementation comparable, and warns that
  `struct{ Charset }` is not while `*struct{ Charset }` is.

The scratch follows the `num`/`wide` policy exactly — `Reader.alphaNum` is a
fixed array of `maxAlphaScratch` bytes, `Reader.alphaWide` the growable
fallback. `maxAlphaScratch` is `maxNumericWidth`, and it is a **policy** number:
`PIC X(n)` has no width to derive one from, and past this size the struct costs
`Unmarshal`'s per-record path more than the translation saves it. Both
directions are benchmarked — `BenchmarkNewReader` and
`BenchmarkUnmarshalRecord` — because they pull opposite ways, and a change here
is not landable without running both.

`NewReader` is 3.5% slower than before this change (262 -> 272 ns/op, one
allocation either way) because the `Reader` grew by one allocation size class.
That is the floor for caching anything on a `Reader` at all; it is disclosed
rather than smoothed over, and the per-record path it buys pays for it — 403 ->
333 ns on the record decode, 628 -> 615 on `Unmarshal`.

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

**Both inverses are precomputed tables, and their fill order is the semantics**
(#112). `zonedBytes.digitOf` inverts `digits`, and `zonedSignReadings` inverts
every row of `zonedSignTables`; both replaced a `slices.Index` scan whose cost
grew with the digit, which made reading all-nines data 1.65x the cost of reading
all-zeros. Four things about them are load bearing rather than incidental:

- **Each fill runs backwards.** `slices.Index` returned the *first* match, so
  the last write into the table has to be the earliest candidate. For digits
  that is digit 9 down to 0, because `Charset.FromUnicode` is nowhere required
  to be injective and a caller's charset may spell two digits with one byte. For
  signs it is the reverse of the scan's four passes — lenient, unsigned,
  positive, negative — so the documented precedence survives. `separateSignValue`
  stays a two-armed switch for the same reason in miniature: the first arm wins,
  so a charset spelling `'+'` and `'-'` alike reads that byte as positive.
- **`digitOf` stores digits biased by one, so its sentinel can be zero.** The
  zero value of `zonedBytes` is reachable — `zonedBytesOf` returns it beside
  each of its errors — and an unwritten entry therefore has to mean "no digit"
  rather than "digit 0", or an unbuilt table reads every byte as a zero. It is
  the one place the table deliberately differs from the scan, which read `0x00`
  as a 0 there.
- **The lenient EBCDIC zones are filled only where the low nibble is 0-9**, and
  the guard is re-applied byte by byte rather than by masking the zone constants
  down to a high nibble, so a malformed row is not silently normalized into ten
  bytes the scan matched none of. Filling a whole zone would newly accept
  eighteen bytes that are rejected today, and those are exactly the bytes that
  make a wrong convention loud.
- **`TestZonedSignByteValueMatchesTheScan` and `TestZonedDigitValueMatchesTheScan`
  keep a transcription of the scan and check the tables against it**, over all
  256 byte values and, for signs, all four conventions. A precomputed table is
  only as auditable as the thing it is checked against; the scan is gone from
  the package and stays in the tests for that.

The receivers of `zonedBytes` and `zonedCodec` are pointers because of this: a
value receiver would copy the 256-byte table on every call and hand the win
straight back.

`signByte` / `signByteValue` are the byte-level pair, `zonedCodec.encodeField` /
`decodeField` the field-level one; `signAt` is the index the sign is
overpunched into, and `-1` means unsigned or `SEPARATE` (plain zone throughout).
`decodeField` returns the **index within the field** of the offending byte
alongside its error, for the caller to stamp as `start+at` — the same reason
`readPackedField` reports the byte holding a bad nibble.

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

## Benchmarks: allocs/op is the figure worth comparing, and CI does not run them

`bench_test.go` is the package's performance baseline. Two rules, both of which
exist because the figures it replaces could not be attributed to any data:

- **allocs/op is the figure worth comparing; ns/op is orientation.** The
  allocation count is machine independent and reproducible, so a change may be
  held to it. Wall time moves with the machine and the toolchain, so nothing
  may. Neither is *enforced* here — nothing asserts a count, there is no stored
  baseline, and CI does not run these (below). Pinning a *count* in a test
  remains a deliberate non-goal: it moves with the toolchain and wants an owner.
  Pinning **zero** does not, and `TestReadingDoesNotAllocate` does exactly that
  for five accessors, because zero is zero on every toolchain and is the one
  number that says a buffer did not escape. `TestResetDoesNotAllocate` pins the
  same number for `Reset` and `ResetStream` on both sides. Those two are the package's only tests
  without `t.Parallel()`, since `testing.AllocsPerRun` sets `GOMAXPROCS` to 1
  and panics when called from a parallel test.
- **Every benchmark fixes and documents its corpus**, and every parameter is a
  parameter because results at two of its values are *not* comparable. A zoned
  field of nines used to cost more than one of zeros, because
  `zonedBytes.digitValue` scanned — #112 made it a table and the three corpora
  now read alike, which is a result the axis still has to be there to show; an
  alphanumeric field whose bytes decode to non-ASCII runes costs two
  UTF-8 bytes a character where an ASCII one costs one; a packed field's width
  is a function of its digit count. Those three are exactly the conflations that
  put wrong numbers in #108.

**CI does not run them, by decision** (#111). `.github/workflows/ci.yml` passes
no `-bench`, so the functions compile on every pull request and execute on none
— which is the point CI does contribute, since a benchmark that stops building
is caught at once. Making them a gate would need a second job without `-race`
(which distorts both timings and allocation counts), a stored baseline,
benchstat, and a runner whose variance is known; GitHub's shared runners are
shared. Run them by hand against a change:

```
go test ./codec/ -run '^$' -bench . -benchmem
```

Extend `testRecord` rather than adding a second fixture here too — the whole
record benchmarks decode and encode it, so a new field is measured for free.

## Performance: what landed, and what the next round waits on (#108)

The research in [#108](https://github.com/Zaba505/cobol-go/discussions/108)
measured where decode time goes and produced eight stories, #109–#116, all
merged. Everything that landed is contract-preserving: the zoned inverse tables
(#112), the reused read scratch (#113), the alphanumeric table derived per
charset (#114), `Reset` and the byte-backed constructors on both sides (#115),
and folding the packed and COMP-6 nibbles rather than unpacking them into a
slice (#116) — with #109 and #110 pinning the alphanumeric byte range and the
nibble fault precedence *first*, so the refactors had a contract to be held to,
and #111 supplying the baseline the package did not have.

Two things were deliberately left, and the reasons are written down here so that
neither is re-argued from first principles:

- **`Append`-style accessors over a caller-owned arena.** Removing the remaining
  string allocations is worth about 1.3x on top of what landed — the last
  quarter, not the 12x #108 quoted. That figure came from returning strings that
  alias a reused buffer, which was measured doing exactly what this package's
  buffer invariants forbid: a previously returned value came back spelling
  another field's bytes. The shape that would work is
  `AppendAlphanumeric(dst []byte, …)` following `strconv.AppendInt`, with the
  arena's lifetime owned by the caller — and it is new exported API, so it is not
  free the way #112–#116 were.
- **SIMD.** After the scalar work in front of it, the only genuinely
  vector-shaped work left is the charset translate: roughly a third of what
  remains, projecting to about 1.5x end to end against the 4x the scalar work
  was worth. As of Go 1.26, `simd/archsimd` is behind `GOEXPERIMENT=simd` and
  every file in it is `_amd64.go`, so there is no portable route — and this
  package imports only `std`.

**Neither waits on a benchmark. Both wait on a named adopter's CPU profile**
showing `codec` at **≥40% of on-CPU samples**, over **≥30s** of run time, with
**≥1000 samples**, with the record layout contributed as `testdata` so the claim
is reproducible here.

Each clause of that bar is there because #108 was triggered by something that
failed it:

- **A profile, not a `time` split and not a records/s figure.** The workload that
  prompted the exercise was quoted at ~457,000 records/s end to end against a
  decode-only 405,298 records/s. A program cannot outrun its own subroutine, so
  those two are not measurements of the same thing and decode's actual share was
  never established. The profile that started it had three samples in it.
- **40%, because Amdahl bounds what the work can be worth.** Against a perfect 4x
  on the codec share alone: 40% caps end-to-end at 1.43x, 30% at 1.29x, 20% at
  1.18x. Below the bar the ceiling is smaller than the risk of the change.
- **The layout as `testdata`, because that is the form #24 and #82 were closed in
  favour of** — acquire no corpus, rely on adopter reports. An adopter
  decode-bound enough to clear this bar is that report, and their layout is the
  fixture the package still does not have.

Until then the answer to "could `codec` be faster" is yes, by about 1.3x with new
API and about 1.5x more with assembly, and neither is worth the contract.
