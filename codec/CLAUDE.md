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

## Errors: one wrapper, typed leaves

Every error returned after construction is wrapped once in
`*OffsetError{Offset, Err}`, so a bad byte deep inside a record is diagnosable.
The offset is stamped in exactly one place per direction — `Reader.read` and
`Writer.write`, the only two methods that move `off` — so it cannot drift.
Never construct an `OffsetError` outside those paths without a reason, and never
add a second offset field to a leaf error.

Leaves are typed values (`EncodingError`, `FieldWidthError`,
`FieldTooLongError`, `UnrepresentableRuneError`, `JustificationError`) or
stdlib sentinels (`io.EOF`, `io.ErrUnexpectedEOF`, `io.ErrShortWrite`). Callers
use `errors.Is` for the cause and `errors.As` for the offset; tests assert both.

Loud beats silent everywhere except one documented place: **alphanumeric
decoding must never fail**. Any byte may appear in a `PIC X` field, such fields
carry binary payloads, so `Charset.ToUnicode` is total over all 256 bytes.
Numeric decoding is the opposite — reject bytes invalid under the declared
setting rather than coercing them.

A rejected field writes **nothing**. Validate first, build the whole field, then
write it, so a failure cannot leave a half-field behind and desynchronize the
record.

## Charsets

`Charset` is an interface, not an enum, because nil must be detectable and
because EBCDIC is not one table — cp037, cp500, cp1047 and cp1140 differ in
where they put brackets, currency and accents. `ASCII()` and `CP037()` ship
here; more pages arrive with the charset story.

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
them.

## Testing style

Table-driven, `t.Parallel()` at both the function and each subtest, `require`
rather than `assert`, hex byte literals with a comment naming the field. Two
round-trip shapes, both required for every accessor pair:

1. **decode → encode → byte-equal**, which proves the trimming a reader does is
   undone by the padding a writer does.
2. **encode → decode → value-equal**.

`testRecord` in `decoder_test.go` is the shared fixture standing in for
generated code; extend it rather than growing a second one.
