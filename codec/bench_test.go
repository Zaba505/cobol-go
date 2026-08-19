// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"bytes"
	"io"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file is the package's performance baseline. Two rules hold throughout,
// and both exist because the numbers this file replaces were unattributable.
//
// **allocs/op is the figure worth comparing; ns/op is orientation.** The
// allocation count is machine independent and reproducible, so it is the figure
// a change may be held to. Wall time is not: it moves with the machine, the
// toolchain and the other tenants of the runner, and nothing should be built on
// it.
//
// Neither figure is *enforced*, and the wording above is deliberate about that.
// Nothing here asserts an allocation count, there is no recorded baseline, and
// by the decision below CI never executes these functions. "Worth comparing" is
// a claim about which number means something when a human compares two runs,
// not a mechanism. The mechanism, if one is ever wanted, is a
// [testing.AllocsPerRun] guard over a handful of accessors: that would be an
// ordinary test, would run in CI, and would catch an allocation regression. It
// is out of scope here because a count pinned in a test moves with the
// toolchain and needs an owner.
//
// **Every benchmark fixes and documents its corpus.** A number that cannot be
// attributed to data is not a measurement of anything. The accessors here are
// corpus sensitive in ways that are easy to miss — [zonedBytes.digitValue]
// scans, so a zoned field of nines costs more comparisons than one of zeros,
// and an alphanumeric field whose bytes decode to non-ASCII runes costs two
// UTF-8 bytes per character where an ASCII one costs one — so a result quoted
// without its corpus can be off by a factor of 1.65 while looking precise.
// Where a benchmark takes a parameter, it is because measurements at two values
// of it are not comparable.
//
// # CI does not run these, by decision
//
// .github/workflows/ci.yml runs `go test -race -covermode=atomic ./...` and
// passes no -bench flag, so these functions compile on every pull request and
// execute on none of them. That is the intended arrangement rather than an
// oversight:
//
//   - the CI job runs under -race, which instruments every memory access and
//     makes both timings and allocation counts unrepresentative of a normal
//     build. A benchmark job would have to be a second job without it.
//   - a gate needs something to compare against: a stored baseline, benchstat,
//     and a runner whose variance is known. GitHub's shared runners are shared,
//     so a threshold on ns/op would flap and be disabled within a month.
//
// So the benchmarks are documentation of intent and a local tool. Compiling
// them on every pull request is the part CI does contribute: a benchmark that
// no longer builds is caught immediately. Run them by hand against a change:
//
//	go test ./codec/ -run '^$' -bench . -benchmem
//	go test ./codec/ -run '^$' -bench BenchmarkReadZoned -benchmem -count 10
//
// Revisit this if the module ever gains a dedicated runner; the missing pieces
// are a baseline file and benchstat, not these functions.

// benchCorpusCopies is how many copies of a field or record each corpus holds.
// The corpus is cycled rather than regenerated, so it only has to be long
// enough that wrapping is rare and short enough to stay in cache.
const benchCorpusCopies = 64

// repeatReader is an [io.Reader] that yields the bytes of a fixed corpus over
// and over, wrapping at the end. It exists so that a read benchmark measures
// the accessor and nothing else: it allocates nothing, never reports io.EOF,
// and needs no per-iteration reconstruction of the [Reader] under test.
//
// Every corpus below is a whole number of copies of the field being read, so
// wrapping lands on a field boundary and no read ever straddles the seam with
// a differently aligned one.
type repeatReader struct {
	buf []byte
	off int
}

func (r *repeatReader) Read(p []byte) (int, error) {
	if len(r.buf) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.buf[r.off:])
	r.off += n
	if r.off == len(r.buf) {
		r.off = 0
	}
	return n, nil
}

// benchReader returns a [Reader] over benchCorpusCopies copies of field, under
// the given encoding.
func benchReader(b *testing.B, enc Encoding, field []byte) *Reader {
	b.Helper()

	r, err := NewReader(&repeatReader{buf: bytes.Repeat(field, benchCorpusCopies)}, enc)
	require.NoError(b, err)
	return r
}

// benchWriter returns a [Writer] onto [io.Discard], so a write benchmark
// measures the encoder rather than the sink.
func benchWriter(b *testing.B, enc Encoding) *Writer {
	b.Helper()

	w, err := NewWriter(io.Discard, enc)
	require.NoError(b, err)
	return w
}

// benchField encodes one field with the package's own [Writer] and returns its
// bytes, checking that it got exactly wantWidth of them. Read benchmarks build
// their corpora this way rather than from hex literals: the corpus is then by
// construction a field the package would have produced, at whatever width and
// sign position the case declares, and a benchmark cannot drift from the
// encoding it claims to measure.
//
// wantWidth is not a formality. It is this helper that stands between a corpus
// and a field-width misalignment — a corpus a byte narrower than the field
// being read makes every read after the first straddle a boundary, and the
// benchmark would still run and still report a number. Stating the width at the
// call site pins the width arithmetic (zonedWidth's SEPARATE +1, packedWidth's
// ceil((digits+1)/2)) rather than assuming it, and it pins the [Writer]'s
// unbuffered contract: Writer.write goes straight to the underlying
// [io.Writer], with no Flush or Close to call, so the buffer holds the whole
// field the moment write returns. Were that ever to change, this length check
// is what would fail.
func benchField(b *testing.B, enc Encoding, wantWidth int, write func(*Writer) error) []byte {
	b.Helper()

	var buf bytes.Buffer
	w, err := NewWriter(&buf, enc)
	require.NoError(b, err)
	require.NoError(b, write(w))
	require.Len(b, buf.Bytes(), wantWidth)
	return buf.Bytes()
}

// benchBytes returns n bytes cycling 0x00-0xFF. [Reader.ReadBytes] applies no
// translation and strips no padding, so the byte values cannot affect its cost;
// cycling the full range is a corpus that says so rather than one that happens
// to be printable.
func benchBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i % 256)
	}
	return b
}

// benchTextBytes encodes text one byte per rune through cs, which is what a
// fixed-width alphanumeric field holds.
func benchTextBytes(b *testing.B, cs Charset, text string) []byte {
	b.Helper()

	out := make([]byte, 0, len(text))
	for _, r := range text {
		c, ok := cs.FromUnicode(r)
		require.Truef(b, ok, "%s cannot spell %q", cs.Name(), r)
		out = append(out, c)
	}
	return out
}

// benchDigits returns the first n digits of 123456789012345678 as an int64: a
// mixed digit corpus that is defined identically at every width, so a result at
// one digit count is at least stated in the same terms as one at another. It is
// still not comparable across widths, which is why the digit count is a
// benchmark parameter rather than a constant.
func benchDigits(b *testing.B, n int) int64 {
	b.Helper()

	const digits = "123456789012345678"
	require.LessOrEqual(b, n, len(digits))
	v, err := strconv.ParseInt(digits[:n], 10, 64)
	require.NoError(b, err)
	return v
}

// requireReadsBack consumes one field from the corpus under test and requires
// it to decode to want, before the timed loop starts.
//
// Without it a read benchmark asserts only that its corpus parses, not that it
// holds what its name claims: a "nines" corpus that benchField had somehow
// built as zeros would run happily and its number would be attributed to the
// wrong data, which is the single failure this whole file exists to prevent.
//
// Consuming a field is safe and costs nothing measurable. Every corpus is a
// whole number of copies of the field, and repeatReader wraps only at the
// corpus end, so reads stay aligned to field boundaries however many are taken
// before b.ResetTimer.
func requireReadsBack[T comparable](b *testing.B, want T, read func() (T, error)) {
	b.Helper()

	got, err := read()
	require.NoError(b, err)
	require.Equal(b, want, got)
}

// benchAlphaWidth is the width of every alphanumeric benchmark's field, in
// bytes and therefore in characters. It matches testRecord's NAME field.
const benchAlphaWidth = 12

// benchAlphaCharsets is the charset axis of the alphanumeric benchmarks: the
// two tables this package ships. Only this axis varies — each case takes
// [GnuCOBOLASCII] and replaces its charset — because the other three
// [Encoding] axes are not consulted by an alphanumeric field at all.
var benchAlphaCharsets = []struct {
	name string
	cs   Charset
}{
	{name: "ascii", cs: ASCII()},
	{name: "cp037", cs: CP037()},
}

// benchAlphaCorpora is the corpus axis of the alphanumeric benchmarks. It is
// the field-level mechanism behind a record-level conflation in #108, where 88
// allocs/record and 100 allocs/record were quoted for what was described as one
// shape: the two runs differed in whether the record's alphanumeric fields
// decoded to ASCII runes. These corpora are one field each and so cannot
// themselves produce an allocs/record figure — they isolate the cause, and
// BenchmarkDecodeRecord is where a record-level number comes from.
//
// Both are exactly benchAlphaWidth characters, and neither has padding to trim,
// so a case differs from its partner only in how wide the decoded runes are —
// one UTF-8 byte each against two.
//
// Both are spellable in both charsets, which is what makes this a genuine 2x2
// rather than four unrelated cases, and that rests on a documented property of
// each table rather than on luck. [ASCII] is the identity over all 256 bytes
// and not a 7-bit table, so every rune below U+0100 has a byte; cp037 is
// bijective over all 256 bytes and carries these particular accented letters at
// 0x42-0x53. benchTextBytes fails the benchmark rather than substituting a
// byte, so a table that stopped covering them would be loud.
var benchAlphaCorpora = []struct {
	name string
	text string
}{
	{name: "ascii-runes", text: "WIDGETGRIP12"},
	{name: "non-ascii-runes", text: "âäàáãåçñ¢éêë"},
}

// benchZonedSigns is the sign position axis of the zoned benchmarks: all five,
// because the position selects the field's width as well as where the sign
// byte sits, and results at two of them are therefore not comparable.
var benchZonedSigns = []SignPosition{
	SignUnsigned,
	SignTrailing,
	SignLeading,
	SignTrailingSeparate,
	SignLeadingSeparate,
}

// benchZonedEncoding is the encoding every zoned benchmark runs under. It is
// fixed rather than parameterised: the cost these benchmarks are about is the
// per-byte digit and sign classification, which is a function of the digit
// values and not of the charset — [zonedBytes.digitOf] is one table lookup
// whatever bytes the charset put in it — and the charset axis is already
// pinned by the alphanumeric benchmarks.
func benchZonedEncoding() Encoding { return GnuCOBOLASCII() }

// benchZonedCorpus is one digit corpus of a zoned benchmark: a value all of
// whose digits are 0, a mixed one, and one all of whose digits are 9.
//
// This is the axis that made #108's "ReadZonedInt32: 47.73 ns/op" meaningless.
// digitValue used to scan its digit table, costing digit+1 comparisons, so the
// nines case ran roughly 1.65x the zeros case on the same field width. A single
// scalar figure for that accessor was a figure for whichever of these three the
// author happened to have in hand.
//
// #112 replaced both scans with tables and the read side is now flat across the
// three: 59ns at nine digits and 78ns at eighteen, whatever the digits are. The
// axis stays all the same. It is what *showed* the data dependence, it is what
// would show one coming back, and the write side is corpus sensitive still, for
// its own reason — see BenchmarkWriteZonedInt64.
//
// Every value is non-negative, so a case differs from its neighbours in the
// digits and not in the sign byte; the sign position axis is what varies that.
type benchZonedCorpus struct {
	name string
	i32  int32
	i64  int64
}

// benchZonedCorpora holds the three corpora at each accessor's widest digit
// count: 9 for the int32 accessors, 18 for the int64 ones.
var benchZonedCorpora = []benchZonedCorpus{
	{name: "zeros", i32: 0, i64: 0},
	{name: "mixed", i32: 123456789, i64: 123456789012345678},
	{name: "nines", i32: 999999999, i64: 999999999999999999},
}

const (
	// benchZonedInt32Digits is the widest digit count ReadZonedInt32 and
	// WriteZonedInt32 accept, and benchZonedInt64Digits the widest the int64
	// pair accepts. Both are pinned at the maximum so that the per-digit cost
	// these benchmarks exist to expose is at its most visible.
	benchZonedInt32Digits = 9
	benchZonedInt64Digits = 18
)

// benchPackedDigits is the digit count axis of the packed benchmarks. A packed
// field's width is ceil((digits+1)/2), so these five run 1, 4, 5, 8 and 10
// bytes wide and a result at one is not a baseline for another. 1 and 18 are
// the ends of what ReadPackedInt64 accepts; 7, 9 and 15 sit at the interesting
// widths in between, 9 being the point past which the value stops fitting an
// int32.
var benchPackedDigits = []int{1, 7, 9, 15, 18}

// benchRecordEncoding is the encoding the whole-record benchmarks run under.
// It is ASCII with IEEE floats — the GnuCOBOL default — rather than
// [IBMEnterprise], so that the charset translation and the float conversion are
// each at their cheapest and the figure is a floor for the record rather than a
// blend of two dialects.
func benchRecordEncoding() Encoding { return GnuCOBOLASCII() }

// benchRecord is the whole-record corpus: testRecord with every field carrying
// a representative value. It is one record shape and one set of values, stated
// here once, so that the decode and encode benchmarks below are measuring the
// same thing and the allocs/record either reports is attributable.
//
// Amount and Balance are negative because a sign is a byte a reader has to
// classify; Units is at its four-digit maximum; Name and Code are shorter than
// their fields, so the padding a writer adds and a reader trims is on the path.
// Every rune is ASCII: the two-byte-rune cost is measured by
// BenchmarkReadAlphanumeric, where it can be attributed, rather than folded in
// here.
func benchRecord() testRecord {
	return testRecord{
		ID:      "A12345",
		Name:    "WIDGET GRIP",
		Code:    "42",
		Raw:     []byte{0x00, 0x01, 0xFF},
		Amount:  -12345,
		Qty:     42,
		Units:   9999,
		Seq:     1234,
		Rate:    1.5,
		Factor:  2.5,
		Balance: -12345,
		Count:   42,
	}
}

// BenchmarkNewReader measures construction alone, which is one of the two
// figures that decide where a derived per-charset table may live.
//
// It is here because the pull in the two directions is real: anything
// materialised in [NewReader] is paid by every [Unmarshal] call, since
// Unmarshal builds a Reader per record, while anything left to the first read
// is paid by a Reader that does not reuse it. #114 measured a 256-entry
// translation table built here at 141 -> 723 ns/op and 1 -> 3 allocs/op, which
// is what sent it to [alphaTables] instead. That pair of figures is that
// issue's run on that author's machine; the ones on [maxAlphaScratch] are this
// implementation on the machine of the pull request that landed it, and the two
// baselines are not comparable to each other.
//
// Corpus: none — the [io.Reader] is never read from. The encoding is
// [IBMEnterprise] rather than [GnuCOBOLASCII] because construction derives the
// zoned byte tables from the charset, and cp037's are the more expensive pair
// to derive.
func BenchmarkNewReader(b *testing.B) {
	enc := IBMEnterprise()
	src := bytes.NewReader(nil)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r, err := NewReader(src, enc)
		if err != nil {
			b.Fatal(err)
		}
		runtime.KeepAlive(r)
	}
}

// BenchmarkReadBytes measures [Reader.ReadBytes], which is the make +
// io.ReadFull floor every other accessor sits on: no accessor of this package
// can be cheaper than ReadBytes at its own width, and the difference between
// the two is what that accessor's decoding costs.
//
// Corpus: n bytes cycling 0x00-0xFF, which ReadBytes neither translates nor
// trims. The widths are a word, a double word, a short text field and a large
// one: 4 and 8 are what a binary field occupies, 40 is a name, and 256 is a
// comment or a binary payload.
func BenchmarkReadBytes(b *testing.B) {
	for _, n := range []int{4, 8, 40, 256} {
		b.Run("n="+strconv.Itoa(n), func(b *testing.B) {
			r := benchReader(b, GnuCOBOLASCII(), benchBytes(n))

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := r.ReadBytes(n); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkReadAlphanumeric measures [Reader.ReadAlphanumeric] over the 2x2 of
// charset and corpus, which is the pairing #108 conflated. The four cases are
// the same twelve-character field width throughout, so the only things varying
// are the translation table and how many UTF-8 bytes the decoded runes occupy.
//
// Corpus: benchAlphaCorpora, twelve characters with no padding to trim.
func BenchmarkReadAlphanumeric(b *testing.B) {
	for _, cs := range benchAlphaCharsets {
		b.Run(cs.name, func(b *testing.B) {
			enc := GnuCOBOLASCII()
			enc.Charset = cs.cs

			for _, corpus := range benchAlphaCorpora {
				b.Run(corpus.name, func(b *testing.B) {
					require.Len(b, []rune(corpus.text), benchAlphaWidth)
					r := benchReader(b, enc, benchTextBytes(b, cs.cs, corpus.text))

					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := r.ReadAlphanumeric(benchAlphaWidth); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// BenchmarkReadZonedInt32 measures [Reader.ReadZonedInt32] over every
// [SignPosition] and each of the three digit corpora, at the accessor's widest
// digit count.
//
// Corpus: benchZonedCorpora at benchZonedInt32Digits digits, encoded under
// benchZonedEncoding by the package's own writer. The zeros-to-nines spread was
// the 1.65x that a single quoted ns/op for this accessor hid; since #112 the
// three read alike, and that they do is the thing worth re-measuring.
func BenchmarkReadZonedInt32(b *testing.B) {
	enc := benchZonedEncoding()

	for _, sign := range benchZonedSigns {
		b.Run(sign.String(), func(b *testing.B) {
			for _, corpus := range benchZonedCorpora {
				b.Run(corpus.name, func(b *testing.B) {
					field := benchField(b, enc, zonedWidth(benchZonedInt32Digits, sign), func(w *Writer) error {
						return w.WriteZonedInt32(corpus.i32, benchZonedInt32Digits, sign)
					})
					r := benchReader(b, enc, field)
					requireReadsBack(b, corpus.i32, func() (int32, error) {
						return r.ReadZonedInt32(benchZonedInt32Digits, sign)
					})

					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := r.ReadZonedInt32(benchZonedInt32Digits, sign); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// BenchmarkReadZonedInt64 is BenchmarkReadZonedInt32 at benchZonedInt64Digits
// digits, the widest the int64 accessor takes. Twice the digits is twice the
// per-byte lookups, so its figures are not comparable with the int32 ones and
// are not meant to be.
//
// Corpus: benchZonedCorpora at benchZonedInt64Digits digits, encoded under
// benchZonedEncoding by the package's own writer.
func BenchmarkReadZonedInt64(b *testing.B) {
	enc := benchZonedEncoding()

	for _, sign := range benchZonedSigns {
		b.Run(sign.String(), func(b *testing.B) {
			for _, corpus := range benchZonedCorpora {
				b.Run(corpus.name, func(b *testing.B) {
					field := benchField(b, enc, zonedWidth(benchZonedInt64Digits, sign), func(w *Writer) error {
						return w.WriteZonedInt64(corpus.i64, benchZonedInt64Digits, sign)
					})
					r := benchReader(b, enc, field)
					requireReadsBack(b, corpus.i64, func() (int64, error) {
						return r.ReadZonedInt64(benchZonedInt64Digits, sign)
					})

					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if _, err := r.ReadZonedInt64(benchZonedInt64Digits, sign); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// BenchmarkReadPackedInt64 measures [Reader.ReadPackedInt64] at each of
// benchPackedDigits. The digit count is a parameter and not a constant because
// a packed field's width is a function of it: a figure at 18 digits is a figure
// about ten bytes, and comparing it with a baseline taken at 7 compares five
// bytes with ten.
//
// Corpus: the first n digits of 123456789012345678, signed, encoded under
// [IBMEnterprise] — packed decimal is charset invariant, so the encoding is
// stated only because an [Encoding] is required.
func BenchmarkReadPackedInt64(b *testing.B) {
	enc := IBMEnterprise()

	for _, digits := range benchPackedDigits {
		b.Run("digits="+strconv.Itoa(digits), func(b *testing.B) {
			v := benchDigits(b, digits)
			field := benchField(b, enc, packedWidth(digits), func(w *Writer) error {
				return w.WritePackedInt64(v, digits, Signed)
			})
			r := benchReader(b, enc, field)
			requireReadsBack(b, v, func() (int64, error) { return r.ReadPackedInt64(digits) })

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := r.ReadPackedInt64(digits); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkDecodeRecord measures a whole testRecord read through one [Reader],
// which is how a file is read: the Reader is constructed once and the record is
// decoded many times. One iteration is one record, so ns/op is per record and
// allocs/op is allocs per record — the two figures come from the same run and
// cannot be quoted from different corpora, which is the failure this benchmark
// exists to make impossible.
//
// The destination is one testRecord reused across iterations, which is how a
// caller stepping through a file writes the loop. It does not flatter the
// allocs/record figure: every accessor assigns a freshly allocated value —
// [Reader.ReadBytes] documents that the slice it returns is the caller's own
// and is not a view into anything the Reader reuses — so there is no capacity
// on the struct for a later record to decode into. Moving the declaration
// inside the loop would change the figure by the one struct, not by the
// nineteen field allocations.
//
// Corpus: benchRecord under benchRecordEncoding.
func BenchmarkDecodeRecord(b *testing.B) {
	enc := benchRecordEncoding()

	fixture := benchRecord()
	data, err := Marshal(enc, &fixture)
	require.NoError(b, err)
	require.Len(b, data, testRecordWidth)

	r := benchReader(b, enc, data)

	var rec testRecord
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := rec.UnmarshalCOBOL(r); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "records/s")
}

// BenchmarkUnmarshalRecord measures the per-record path a caller stepping
// through a file actually takes: [Unmarshal] builds a [Reader] over one
// record's bytes and drops it, so construction is paid once per record rather
// than once per file.
//
// It is the counterweight to BenchmarkDecodeRecord, which reuses one Reader
// across every iteration and therefore amortises construction to nothing. The
// two disagree by exactly what a Reader costs to build, and a change that
// moves work into construction to save it per field is visible here and
// nowhere else.
//
// The charset axis is the second thing it exists for, and it is not a charset
// axis in the sense BenchmarkReadAlphanumeric has one — both cases below spell
// the record identically. What differs is whether the charset can be a map key,
// and so whether its translation table is shared across records or absent: a
// comparable charset reads through [alphaTables], one that is not comparable
// reads per byte (see alphaTableOf). That is a cliff only this benchmark can
// see, because it is the only one that builds a [Reader] per iteration, and the
// "uncached" case is here to show the fallback costs about what the per-byte
// loop always cost rather than 256 translations and a table per record.
//
// Corpus: benchRecord under benchRecordEncoding, the same record
// BenchmarkDecodeRecord reads, so the difference between the two figures is
// attributable to the Reader and not to the data.
func BenchmarkUnmarshalRecord(b *testing.B) {
	for _, cs := range benchUnmarshalCharsets {
		b.Run(cs.name, func(b *testing.B) {
			enc := benchRecordEncoding()
			enc.Charset = cs.cs
			require.Equal(b, cs.comparable, comparableCharset(cs.cs),
				"this case does not exercise the cacheability it names")

			fixture := benchRecord()
			data, err := Marshal(enc, &fixture)
			require.NoError(b, err)
			require.Len(b, data, testRecordWidth)

			var rec testRecord
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := Unmarshal(enc, data, &rec); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "records/s")
		})
	}
}

// benchUnmarshalCharsets is BenchmarkUnmarshalRecord's cacheability axis: the
// shipped [ASCII], which is an empty struct and so a usable map key, and the
// same table wrapped in a struct that embeds the [Charset] interface, which is
// the idiomatic decorator shape and is not comparable. Both decode the record
// to identical values.
var benchUnmarshalCharsets = []struct {
	name       string
	cs         Charset
	comparable bool
}{
	{name: "cached", cs: ASCII(), comparable: true},
	{name: "uncached", cs: benchEmbeddingCharset{Charset: ASCII()}},
}

// benchEmbeddingCharset is [ASCII] behind an embedded interface, which is what
// makes it non-comparable and so uncacheable. It is separate from
// embeddingCharset in types_test.go so that a change to that fixture cannot
// silently move this benchmark's meaning.
type benchEmbeddingCharset struct {
	Charset
}

func (benchEmbeddingCharset) Name() string { return "ascii-embedded" }

// BenchmarkWriteAlphanumeric is the mirror image of BenchmarkReadAlphanumeric,
// over the same 2x2 of charset and corpus. The [Writer] side has never been
// measured at all, and [Writer.WriteAlphanumeric] — which is
// [Writer.WriteAlphanumericJustified] at [JustifyLeft], so the shared body is
// what is measured — has the same allocate-then-fill shape the reader has.
//
// Corpus: benchAlphaCorpora, twelve characters written into a
// benchAlphaWidth-byte field, so no padding is added and the case differs from
// its partner only in the runes.
func BenchmarkWriteAlphanumeric(b *testing.B) {
	for _, cs := range benchAlphaCharsets {
		b.Run(cs.name, func(b *testing.B) {
			enc := GnuCOBOLASCII()
			enc.Charset = cs.cs

			for _, corpus := range benchAlphaCorpora {
				b.Run(corpus.name, func(b *testing.B) {
					require.Len(b, []rune(corpus.text), benchAlphaWidth)
					w := benchWriter(b, enc)

					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := w.WriteAlphanumeric(corpus.text, benchAlphaWidth); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// BenchmarkWritePackedInt64 is the mirror image of BenchmarkReadPackedInt64,
// at the same digit counts and over the same corpus.
func BenchmarkWritePackedInt64(b *testing.B) {
	enc := IBMEnterprise()

	for _, digits := range benchPackedDigits {
		b.Run("digits="+strconv.Itoa(digits), func(b *testing.B) {
			v := benchDigits(b, digits)
			w := benchWriter(b, enc)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := w.WritePackedInt64(v, digits, Signed); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkWriteZonedInt64 is the mirror image of BenchmarkReadZonedInt64, over
// the same sign positions and the same three digit corpora.
//
// There is deliberately no WriteZonedInt32 counterpart, where the read side has
// both widths. [Writer.WriteZonedInt32] and [Writer.WriteZonedInt64] are one
// body — both are writeZonedInt with a different digit ceiling — so the int32
// writer has no code of its own for a benchmark to cover, and the int64 case
// exercises that shared body at the wider field. The read side is not symmetric
// with this because readZonedDigits' per-byte classification is where the
// corpus sensitivity lived, and its cost is a function of the digit count.
// [zonedCodec.encodeField] has the reader's allocate-then-fill shape, and the
// writer turns out to be corpus sensitive in its own way rather than in the
// reader's: it is handed the value and not the digit bytes, so what varies with
// the corpus is the decimal formatting ahead of the field, and the all-zeros
// case is measurably cheaper than the other two. Pinning all three is what
// stops a writer figure being quoted from whichever was to hand.
func BenchmarkWriteZonedInt64(b *testing.B) {
	enc := benchZonedEncoding()

	for _, sign := range benchZonedSigns {
		b.Run(sign.String(), func(b *testing.B) {
			for _, corpus := range benchZonedCorpora {
				b.Run(corpus.name, func(b *testing.B) {
					w := benchWriter(b, enc)

					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						if err := w.WriteZonedInt64(corpus.i64, benchZonedInt64Digits, sign); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// BenchmarkEncodeRecord is the mirror image of BenchmarkDecodeRecord: the same
// record written through one [Writer], one iteration to a record, reporting
// records/s and allocs/record from the same run.
//
// It writes through a Writer onto [io.Discard] rather than through [Marshal],
// so the figure is the encoder's own and not the encoder plus a buffer that
// grows — the same asymmetry as on the read side, where a file is decoded
// through one long-lived Reader.
func BenchmarkEncodeRecord(b *testing.B) {
	rec := benchRecord()
	w := benchWriter(b, benchRecordEncoding())

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := rec.MarshalCOBOL(w); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "records/s")
}
