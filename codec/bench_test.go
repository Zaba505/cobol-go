// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"bytes"
	"io"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file is the package's performance baseline. Two rules hold throughout,
// and both exist because the numbers this file replaces were unattributable.
//
// **allocs/op is the assertion; ns/op is orientation.** The allocation count is
// machine independent and reproducible, so it is the figure a change may be
// held to. Wall time is not: it moves with the machine, the toolchain and the
// other tenants of the runner, and nothing should be built on it.
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
// bytes. Read benchmarks build their corpora this way rather than from hex
// literals: the corpus is then by construction a field the package would have
// produced, at whatever width and sign position the case declares, and a
// benchmark cannot drift from the encoding it claims to measure.
func benchField(b *testing.B, enc Encoding, write func(*Writer) error) []byte {
	b.Helper()

	var buf bytes.Buffer
	w, err := NewWriter(&buf, enc)
	require.NoError(b, err)
	require.NoError(b, write(w))
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

// benchAlphaCorpora is the corpus axis of the alphanumeric benchmarks, and it
// is the pair that #108 conflated: two headline figures, 88 allocs/record and
// 100 allocs/record, that were really one shape measured over each of these.
//
// Both are exactly benchAlphaWidth characters, and neither has padding to trim,
// so a case differs from its partner only in how wide the decoded runes are —
// one UTF-8 byte each against two. Both are spellable in both charsets, so the
// 2x2 is a genuine 2x2 rather than four unrelated cases.
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
// fixed rather than parameterised: the cost these benchmarks are about is
// [zonedBytes.digitValue]'s scan, which is a function of the digit values and
// not of the charset, and the charset axis is already pinned by the
// alphanumeric benchmarks.
func benchZonedEncoding() Encoding { return GnuCOBOLASCII() }

// benchZonedCorpus is one digit corpus of a zoned benchmark: a value all of
// whose digits are 0, a mixed one, and one all of whose digits are 9.
//
// This is the axis that made #108's "ReadZonedInt32: 47.73 ns/op" meaningless.
// digitValue scans its digit table, costing digit+1 comparisons, so the nines
// case runs roughly 1.65x the zeros case on the same field width. A single
// scalar figure for this accessor is a figure for whichever of these three the
// author happened to have in hand.
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
	// pair accepts. Both are pinned at the maximum so that the per-digit scan
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

// BenchmarkReadBytes measures [Reader.ReadBytes], which is the make +
// io.ReadFull floor every other accessor sits on: no accessor of this package
// can be cheaper than ReadBytes at its own width, and the difference between
// the two is what that accessor's decoding costs.
//
// Corpus: n bytes cycling 0x00-0xFF, which ReadBytes neither translates nor
// trims. The widths are a 6-byte-ish field, a double word, a short text field
// and a large one.
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
// benchZonedEncoding by the package's own writer. The zeros-to-nines spread is
// the 1.65x that a single quoted ns/op for this accessor hides.
func BenchmarkReadZonedInt32(b *testing.B) {
	enc := benchZonedEncoding()

	for _, sign := range benchZonedSigns {
		b.Run(sign.String(), func(b *testing.B) {
			for _, corpus := range benchZonedCorpora {
				b.Run(corpus.name, func(b *testing.B) {
					field := benchField(b, enc, func(w *Writer) error {
						return w.WriteZonedInt32(corpus.i32, benchZonedInt32Digits, sign)
					})
					r := benchReader(b, enc, field)

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
// scans, so its figures are not comparable with the int32 ones and are not
// meant to be.
//
// Corpus: benchZonedCorpora at benchZonedInt64Digits digits, encoded under
// benchZonedEncoding by the package's own writer.
func BenchmarkReadZonedInt64(b *testing.B) {
	enc := benchZonedEncoding()

	for _, sign := range benchZonedSigns {
		b.Run(sign.String(), func(b *testing.B) {
			for _, corpus := range benchZonedCorpora {
				b.Run(corpus.name, func(b *testing.B) {
					field := benchField(b, enc, func(w *Writer) error {
						return w.WriteZonedInt64(corpus.i64, benchZonedInt64Digits, sign)
					})
					r := benchReader(b, enc, field)

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
			field := benchField(b, enc, func(w *Writer) error {
				return w.WritePackedInt64(v, digits, Signed)
			})
			r := benchReader(b, enc, field)

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

// BenchmarkWriteAlphanumeric is the mirror image of BenchmarkReadAlphanumeric,
// over the same 2x2 of charset and corpus. The [Writer] side has never been
// measured at all, and [Writer.WriteAlphanumericJustified] has the same
// allocate-then-fill shape the reader has.
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
