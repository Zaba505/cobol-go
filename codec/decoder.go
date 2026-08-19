// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"encoding/binary"
	"io"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Reader reads the fields of a COBOL data file from an [io.Reader], or from a
// []byte the caller already holds, one field at a time and in record order.
//
// There is deliberately no usable zero value: a Reader is only obtainable from
// [NewReader] or [NewBytesReader], each of which requires a complete
// [Encoding]. A Reader is not safe for concurrent use — the read position is
// state, and so are the scratch buffers every field is read into, which makes
// concurrent use silent corruption of a field rather than a racy counter.
//
// A Reader over bytes can be rewound onto the next record with [Reader.Reset],
// which keeps everything the [Encoding] derived. That is what makes one Reader
// per *file* — or one pooled across a fleet of them — the cheap way to step
// through records, rather than one per record.
type Reader struct {
	// r is the stream a Reader built by [NewReader] reads, and is unused by
	// the byte-backed Readers — those built by [NewBytesReader] or rewound
	// by [Reader.Reset] — whose source is data instead.
	r io.Reader
	// fromBytes says which of the two it is, and it is a field rather than a
	// nil check on either of them because the zero value of a Reader has to
	// stay unusable. A Reader nobody constructed has a nil r and a nil data,
	// and "nil r means read the bytes" would read that as an empty record and
	// answer [io.EOF] — a plausible answer, from a Reader whose [Encoding]
	// was never validated. With this field it takes the stream arm instead
	// and fails on the nil [io.Reader] exactly as it did before there was a
	// second kind of source.
	fromBytes bool
	// data is the *unread remainder* of a byte-backed Reader's source: the
	// caller's own slice, held rather than copied, resliced as fields are
	// consumed. It is a field on the struct rather than a *bytes.Reader
	// behind r because the point of the byte-backed path is that a record
	// costs no allocation at all — wrapping the slice would put one back,
	// per record, exactly where [Reader.Reset] exists to remove it.
	//
	// Consuming by reslicing rather than by indexing at off is what keeps
	// off a pure counter of bytes read. The two are equal today, but off is
	// what [Reader.Offset] and every [OffsetError] mean, and the first
	// accessor to move one without the other would otherwise turn this
	// field into an out-of-range index in the middle of a decode.
	//
	// The Reader holds the slice until the next Reset and no longer; see
	// Reset for what that means for a caller reusing its buffer.
	data []byte
	enc  Encoding
	off  int64
	// zoned is the encoding's zoned decimal byte table, derived once rather
	// than per field. zonedErr holds the failure of deriving it, which is
	// reported by the first zoned accessor and by nothing else: a charset
	// that cannot spell a digit or a '+' still reads alphanumeric fields
	// perfectly well, so it is not a reason to refuse a Reader.
	zoned    zonedCodec
	zonedErr error
	// num is the scratch every field narrow enough to fit it is read into,
	// and wide is the growable one everything else is read into. Both are
	// reused across fields and neither is ever returned to a caller; see
	// [Reader.read] for why one buffer is an array on the struct and the
	// other a slice, and [Reader.ReadBytes] for the one accessor that opts
	// out of both.
	//
	// wide grows to the widest field asked of it and never shrinks, so a
	// Reader that has read one PIC X(32760) field holds 32KB until it is
	// dropped. That is the trade a reused buffer is: shrinking it would
	// reintroduce the allocation on the next wide field. [Unmarshal] builds
	// a Reader per record and drops it, so it never holds one for long; a
	// caller keeping one across records with [Reader.Reset] is choosing
	// that capacity, which is the same choice as keeping the buffer.
	num  [maxNumericWidth]byte
	wide []byte
	// alpha is the charset's UTF-8 translation table, and alphaNum and
	// alphaWide are the scratch the translated bytes are written into —
	// the same fixed-array-plus-growable-slice pair num and wide are, for
	// the same reason.
	//
	// alpha is looked up on the first alphanumeric read and not before.
	// Deriving a table calls [Charset.ToUnicode] 256 times, and
	// TestZonedAccessorsNeverTranslateThroughTheCharset requires a Reader
	// that reads only numeric fields to make no such call at all — which is
	// codec/SPEC.md's rule that numeric decoding never routes through the
	// charset, asserted from construction onward. Lazy is what keeps that
	// true; see [alphaTables] for why the table itself is not per-Reader.
	//
	// It stays nil for a charset that cannot be cached, which is a working
	// state and not an unresolved one — alphaLookedUp is what tells those
	// apart, so a charset with no table is not looked up again per field.
	alpha         *alphaTable
	alphaLookedUp bool
	alphaNum      [maxAlphaScratch]byte
	alphaWide     []byte
}

// maxNumericWidth is the width of [Reader.num], the fixed scratch every
// numeric field is read into. It is the widest field any numeric accessor will
// read, derived from the package's own digit-count maxima rather than written
// down: a 32-byte zoned field of 31 digits with a SEPARATE sign, against 16
// bytes for packed, COMP-6 and binary and 4 and 8 for the two floating point
// widths.
//
// Deriving it is what keeps a raised maximum from turning a legal field into a
// smashed buffer. It is *not* what keeps it from panicking — [Reader.read]
// falls back to [Reader.wide] for anything longer, so an accessor admitting a
// wider field than this const anticipated costs an allocation and nothing
// else. 31 digits is a dialect ceiling (codec/SPEC.md, "18, or 31 with
// ARITH(EXTEND)"), not a fact about COBOL, so the day it moves is a day this
// const has to be free to move with it.
//
// TestNumericScratchFitsEveryNumericUsage pins the derivation against the
// width functions themselves, which is the check that catches a family whose
// width stops being one of the terms below.
const maxNumericWidth = max(
	maxZonedDigits+1,      // zonedWidth(maxZonedDigits, a SEPARATE sign position)
	(maxPackedDigits+2)/2, // packedWidth(maxPackedDigits)
	(maxPackedDigits+1)/2, // comp6Width(maxPackedDigits)
	maxBinaryFieldWidth,   // binaryWidth(maxBinaryDigits)
	comp1Width,
	comp2Width,
)

// maxAlphaScratch is the size of [Reader.alphaNum], the fixed scratch an
// alphanumeric field's translated bytes are written into. It is
// [maxNumericWidth]: the two reused scratches of a [Reader] are the same size,
// so the numeric side reads a 32-byte field without touching the heap and the
// alphanumeric side translates into 32 bytes without touching it either.
//
// Unlike maxNumericWidth this is a *policy* number that happens to be derived,
// and it cannot be anything else: PIC X(n) is bounded by the record rather than
// by a digit ceiling, so there is no width to derive one from. What pins it is
// the other end. A [Reader] is what [Unmarshal] builds per record, so every byte
// of the struct is paid once per record, and a bigger array buys a wider
// heap-free field at the cost of every record — including the records whose
// fields would have fitted anyway. A sweep over BenchmarkNewReader and
// BenchmarkUnmarshalRecord found the crossover well below a 64-byte field: at
// 259 bytes, enough for one under any charset, both benchmarks were slower than
// the code this replaces on the strength of the struct alone. **Do not grow this
// array without re-running that pair**; it is not free, and the two benchmarks
// disagree by design about which way it should go.
//
// At the value chosen the struct costs NewReader 262 -> 272 ns/op, one
// allocation either way, and the per-record path still comes out ahead:
// 628 -> 615 for a cacheable charset. [Reader.readAlphanumericPerByte] documents
// the one case that does not.
//
// How wide a *field* this covers is charset-dependent, because a rune is one to
// four UTF-8 bytes: 14 bytes under either shipped charset and 7 under one
// mapping into a supplementary plane; see [alphaTable.fieldCap]. It covers far
// more than that in practice, because the padding is stripped before the
// translation is written — a PIC X(30) name holding eleven characters needs 25
// bytes here, not 63. A field whose *value* is still wider is not a fault and
// not a panic: it falls to [Reader.alphaWide], exactly as a numeric field wider
// than maxNumericWidth falls to [Reader.wide], at the cost of one allocation
// that is then reused at that size.
const maxAlphaScratch = maxNumericWidth

// NewReader returns a [Reader] that reads from r under the given encoding.
//
// enc must declare all five axes; construction fails with an [EncodingError]
// naming the first field that does not. There is no default for any of them,
// because each fails silently when wrong. The named bundles — [IBMEnterprise],
// [MicroFocusASCII], [GnuCOBOLASCII], [ConvertedFromEBCDIC] — expand to a
// complete encoding in one call.
func NewReader(r io.Reader, enc Encoding) (*Reader, error) {
	if r == nil {
		return nil, ErrNilReader
	}
	rd, err := newReader(enc)
	if err != nil {
		return nil, err
	}
	rd.r = r
	return rd, nil
}

// NewBytesReader returns a [Reader] that reads the record in data under the
// given encoding. It is [NewReader] over bytes the caller already holds, and it
// is what [Unmarshal] builds.
//
// enc is validated exactly as [NewReader] validates it, and construction fails
// with the same [EncodingError] naming the same field. The one difference is
// what it does *not* reject: a nil data is an empty record and not an error,
// because a nil slice is a slice of no bytes rather than a missing source. Such
// a Reader is at end of input, and the first field asked of it fails with
// [io.EOF] — the same answer [NewReader] over an empty stream gives.
//
// The Reader holds data rather than copying it; see [Reader.Reset] for the
// lifetime that implies.
func NewBytesReader(data []byte, enc Encoding) (*Reader, error) {
	rd, err := newReader(enc)
	if err != nil {
		return nil, err
	}
	rd.fromBytes = true
	rd.data = data
	return rd, nil
}

// newReader builds the half of a [Reader] that comes from the encoding —
// which is exactly the half [Reader.Reset] keeps — and leaves the source to
// its caller. Both constructors go through it so that the validation they
// promise to share cannot drift into two spellings of it.
func newReader(enc Encoding) (*Reader, error) {
	if err := enc.Validate(); err != nil {
		return nil, err
	}
	zoned, zonedErr := newZonedCodec(enc)
	return &Reader{enc: enc, zoned: zoned, zonedErr: zonedErr}, nil
}

// Reset rewinds r onto data, so that the next field read is data's first byte
// and [Reader.Offset] reports 0 again.
//
// Everything derived from the [Encoding] survives — the zoned decimal byte
// tables, the alphanumeric translation table, and the scratch buffers every
// field is read into. The Encoding itself cannot change; a different one needs
// a different Reader, because an encoding that could be swapped under a
// half-read record is the silent failure [Encoding] exists to make impossible.
//
// This is what lets one Reader serve a whole file, or a pool serve a fleet of
// them:
//
//	r := pool.Get().(*codec.Reader)
//	defer func() { r.Reset(nil); pool.Put(r) }()
//	for _, rec := range records {
//		r.Reset(rec)
//		if err := v.UnmarshalCOBOL(r); err != nil {
//			return err
//		}
//	}
//
// **The Reader holds data; it does not copy it.** The slice is retained until
// the next Reset and no longer, so a caller reusing one buffer per record must
// not refill it while the Reader is still reading, and a caller returning a
// Reader to a pool passes nil to drop the reference. Nothing a read *returns*
// views data — every accessor decodes through the Reader's own scratch, and
// [Reader.ReadBytes] allocates — so values from earlier records survive both
// the next Reset and a later write into the caller's buffer.
//
// Reset works on a Reader built by [NewReader] too: the stream is dropped and
// the bytes take its place.
func (r *Reader) Reset(data []byte) {
	r.fromBytes = true
	r.r = nil
	r.data = data
	r.off = 0
}

// Encoding reports the encoding the [Reader] was constructed with.
func (r *Reader) Encoding() Encoding { return r.enc }

// Offset reports how many bytes have been consumed, which is the position of
// the next field in the stream. Bytes consumed by a failed read are counted, so
// the offset after an error is where reading stopped.
func (r *Reader) Offset() int64 { return r.off }

// read consumes exactly n bytes into a buffer the Reader owns and reuses, and
// returns it. The bytes are valid until the next read and no further: every
// caller of this method must consume them before it returns, and one that
// hands them to its own caller must use [Reader.readOwned] instead.
//
// Reuse is the whole point of it. A fresh make per field is not expensive
// because it is a make — an identical one whose result does not escape costs
// nothing — it is expensive because returning the slice forces it onto the
// heap, one allocation per field before any value exists.
//
// Two buffers, because the two families are bounded differently. A numeric
// field is bounded by [maxNumericWidth], so [Reader.num] is an array inline in
// the struct: no allocation ever and no pointer to chase. A PIC X field is
// bounded only by the record, so anything wider goes through [Reader.wide],
// which grows on demand and is then reused at that size. The wide path is also
// the fallback that keeps a numeric field wider than the array an allocation
// rather than a panic.
//
// Both buffers are sliced with a **full slice expression**, so the returned
// slice's capacity is the field width and not the buffer's. A slice into a
// reused buffer with spare capacity is an append away from writing over the
// bytes of the next field, silently and with no bounds panic; capping it makes
// "these n bytes and no more" a fact the runtime enforces rather than a
// promise this comment makes.
func (r *Reader) read(n int) ([]byte, error) {
	if err := r.checkWidth(n); err != nil {
		return nil, err
	}
	var buf []byte
	if n <= maxNumericWidth {
		buf = r.num[:n:n]
	} else {
		if cap(r.wide) < n {
			r.wide = make([]byte, n)
		}
		buf = r.wide[:n:n]
	}
	if err := r.readInto(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// readOwned consumes exactly n bytes into a freshly allocated buffer that
// becomes the caller's. It is [Reader.read] for the one accessor whose bytes
// escape, and it allocates on purpose; see [Reader.ReadBytes].
func (r *Reader) readOwned(n int) ([]byte, error) {
	if err := r.checkWidth(n); err != nil {
		return nil, err
	}
	buf := make([]byte, n)
	if err := r.readInto(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// checkWidth rejects a negative field width. It is the one precondition both
// buffer policies share, held here rather than written out in each of them for
// the reason the offset is stamped in one place: two copies of a precondition
// are two copies to keep in step, and the reusing path and the owning one are
// exactly the pair that must not drift.
func (r *Reader) checkWidth(n int) error {
	if n < 0 {
		return &OffsetError{Offset: r.off, Err: FieldWidthError{Width: n}}
	}
	return nil
}

// readInto fills buf exactly. It is the single place the offset advances and
// the single place read errors are stamped with it, which is what keeps the
// offset from drifting between the owning and the reusing path above — and
// now between the streaming source and the byte-backed one, which is why the
// two arms are here rather than in two methods.
//
// The byte-backed arm is [io.ReadFull]'s contract done by hand: a short fill is
// [io.ErrUnexpectedEOF], and one that copied nothing at all is [io.EOF], so a
// record ending exactly on a field boundary reads the same either way. It
// *copies* into buf rather than handing back a window onto data, which costs
// the copy and buys two invariants: no accessor can hand out a view into the
// caller's slice, and [Reader.read]'s promise that its result is overwritten by
// the next read stays true of every source. What it does reslice is its own
// [Reader.data], so the source shrinks as it is consumed and off stays a pure
// counter rather than an index into it.
//
// Which arm runs is [Reader.fromBytes] and never a nil check, so that a Reader
// nobody constructed still fails on its nil [io.Reader] instead of reading as
// an empty record.
func (r *Reader) readInto(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	var (
		got int
		err error
	)
	if r.fromBytes {
		got = copy(buf, r.data)
		r.data = r.data[got:]
		if got < len(buf) {
			err = io.ErrUnexpectedEOF
			if got == 0 {
				err = io.EOF
			}
		}
	} else {
		got, err = io.ReadFull(r.r, buf)
	}
	r.off += int64(got)
	if err != nil {
		return &OffsetError{Offset: r.off, Err: err}
	}
	return nil
}

// alphaScratch returns a buffer of exactly n bytes for a translation to be
// written into. It is [Reader.read]'s buffer policy applied to the other side
// of the translation: the fixed array for anything that fits it, the growable
// slice for anything that does not, and no allocation at all in the common
// case.
//
// The two policies need separate buffers because both are live at once — the
// source bytes of a wide field are sitting in [Reader.wide] while the
// translation is being written.
func (r *Reader) alphaScratch(n int) []byte {
	if n <= len(r.alphaNum) {
		return r.alphaNum[:n]
	}
	if cap(r.alphaWide) < n {
		r.alphaWide = make([]byte, n)
	}
	return r.alphaWide[:n]
}

// ReadBytes reads the next n bytes as they stand, applying no character
// translation and stripping no padding.
//
// This is the raw accessor alongside [Reader.ReadAlphanumeric], for the PIC X
// field that carries a binary payload rather than characters, and for any
// caller that needs the bytes a trimmed string cannot reproduce.
//
// The returned slice is the caller's own. This method allocates it for the one
// call that returns it, and neither retains a reference to it nor writes into
// it again, so it may be kept, modified and handed on without being copied
// first. It is not a view into a buffer the Reader reuses, which is the
// property a caller holding one across later reads depends on.
//
// A short stream is an error: io.EOF when nothing at all was left, and
// [io.ErrUnexpectedEOF] when the field was cut short, both wrapped in an
// [OffsetError]. Reading at the end of a file therefore reports io.EOF, which
// is how a caller stepping through records detects the end of one.
func (r *Reader) ReadBytes(n int) ([]byte, error) {
	return r.readOwned(n)
}

// ReadAlphanumeric reads the next n bytes as an alphanumeric (PIC X) or
// alphabetic (PIC A) field, translating them through the encoding's charset and
// stripping the trailing space padding.
//
// It is [Reader.ReadAlphanumericJustified] with [JustifyLeft], the COBOL
// default for an item with no JUSTIFIED clause.
//
// Trimming is a policy decision this package makes and not a property of the
// data: trailing spaces in a fixed-width field are indistinguishable from
// content, and stripping them is what makes a field read back as the value that
// was written into it. A caller that needs the padding preserved should use
// [Reader.ReadBytes].
//
// Decoding never fails on the bytes themselves. Any byte value may appear in an
// alphanumeric field, so the charset translation is total and an untranslatable
// byte is impossible by construction.
func (r *Reader) ReadAlphanumeric(n int) (string, error) {
	return r.ReadAlphanumericJustified(n, JustifyLeft)
}

// ReadAlphanumericJustified reads the next n bytes as an alphanumeric field
// whose value sits at the end of the field j names, stripping the padding from
// the other end: trailing spaces under [JustifyLeft], leading spaces under
// [JustifyRight], which is the JUSTIFIED RIGHT clause.
//
// The space stripped is the charset's own — 0x20 in ASCII, 0x40 in EBCDIC —
// since translation happens first.
func (r *Reader) ReadAlphanumericJustified(n int, j Justification) (string, error) {
	if j != JustifyLeft && j != JustifyRight {
		return "", &OffsetError{Offset: r.off, Err: JustificationError{Justification: j}}
	}
	b, err := r.read(n)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		// Nothing to translate, and so no reason to look a table up: a
		// zero-width field must not be what makes a Reader derive 256
		// entries, and must not be a charset translation at all.
		return "", nil
	}
	if !r.alphaLookedUp {
		r.alpha, r.alphaLookedUp = alphaTableOf(r.enc.Charset), true
	}
	t := r.alpha
	if t == nil {
		return r.readAlphanumericPerByte(b, j), nil
	}
	// The padding comes off the *source* bytes, before translation rather
	// than after it, and the two are the same operation. The space stripped
	// is U+0020 and the bytes stripped are the ones that spell it, which is
	// exactly the correspondence [alphaTable.space] records; and a stripped
	// U+0020 can never have been part of a wider character, because 0x20 is
	// neither a UTF-8 lead byte nor a continuation byte.
	//
	// Doing it first is what makes the fixed scratch worth having. Fields in
	// a real record are mostly padding — a PIC X(30) name holding eleven
	// characters is the normal case, not the exception — so trimming first
	// keeps a field far wider than [maxAlphaScratch] off the growable buffer,
	// and translates only the bytes that survive.
	src := t.trim(b, j)
	return string(t.translate(r.alphaScratch(t.fieldCap(len(src))), src)), nil
}

// readAlphanumericPerByte is the translation of a field whose charset has no
// derived table: one [Charset.ToUnicode] and one [strings.Builder.WriteRune] per
// byte, which is what this package did for every charset before the table
// existed.
//
// It is reached only for a charset that cannot be a map key and so cannot be
// cached — see [alphaTableOf], which explains why no table at all beats a table
// built per [Reader]. It is a separate method rather than a branch inline in
// [Reader.ReadAlphanumericJustified] so that the table path, which is the path
// nearly every program takes, is not sharing a stack frame with a loop it never
// runs.
//
// **This is the one path that is slower than before the table existed**, and by
// a constant rather than by a factor: BenchmarkUnmarshalRecord/uncached is
// 665 -> 735 ns/op, about 10%. The cost is the [Reader] carrying a table pointer
// and a scratch it will not use, the comparability answer being reached once per
// Reader, and the branch above. None of it is recoverable while the table is
// cached per charset and the [Reader] is built per record, and the remedy
// belongs to the caller rather than here: a comparable charset — a pointer will
// do — takes the table path and is 2.1x faster instead. [Charset] says so.
func (r *Reader) readAlphanumericPerByte(b []byte, j Justification) string {
	charset := r.enc.Charset
	var sb strings.Builder
	sb.Grow(len(b))
	for _, c := range b {
		sb.WriteRune(charset.ToUnicode(c))
	}
	if j == JustifyRight {
		return strings.TrimLeft(sb.String(), " ")
	}
	return strings.TrimRight(sb.String(), " ")
}

// ReadZonedInt32 reads the next zoned decimal (USAGE DISPLAY) field of digits
// digits as an int32, consuming digits bytes — or digits+1 when s is a
// SEPARATE position, which is the one thing that changes the width.
//
// Zoned decimal is the default USAGE and the most common numeric encoding in
// interchange files: one character byte per digit, most significant first. It
// is a per-field usage and not a file-level mode, so a record routinely mixes
// it with COMP-3, COMP and COMP-1 fields.
//
// s is required and has no default. It says whether the item's PICTURE carries
// S and where the SIGN clause put the sign, neither of which is recoverable
// from the bytes — see [SignPosition].
//
// digits must be between 1 and 9, the most that always fits an int32; a wider
// field is a [ZonedDigitCountError] rather than a silent overflow, and is read
// with [Reader.ReadZonedInt64] or [Reader.ReadZonedBig].
//
// Every byte is validated against the declared [Encoding.Charset] and
// [Encoding.Sign]: a digit byte that is not one is a [ZonedDigitError], a sign
// byte invalid under the convention is a [ZonedSignError], and a SEPARATE sign
// byte that is neither '+' nor '-' is a [ZonedSeparateSignError]. All three
// carry the offset of the *byte* at fault rather than the end of the field.
//
// The return is the unscaled integer. A PICTURE's V occupies no byte and is not
// recoverable from the data, so scale is not a decoding input: PIC S9(3)V99
// holding -123.45 is five bytes reading as -12345. A PICTURE containing an
// actual decimal point is numeric-edited rather than DISPLAY numeric and is out
// of scope here; see codec/SPEC.md, "Numeric-edited de-editing".
func (r *Reader) ReadZonedInt32(digits int, s SignPosition) (int32, error) {
	v, err := r.readZonedInt(digits, maxZonedInt32Digits, s)
	return int32(v), err
}

// ReadZonedInt64 reads the next zoned decimal field of digits digits as an
// int64, consuming digits bytes, or digits+1 under a SEPARATE sign position.
//
// digits must be between 1 and 18, the most that always fits an int64. The 19
// to 31 digit range an IBM item may declare is read with
// [Reader.ReadZonedBig]. s is required; see [Reader.ReadZonedInt32], which also
// says why the return is the unscaled integer.
func (r *Reader) ReadZonedInt64(digits int, s SignPosition) (int64, error) {
	return r.readZonedInt(digits, maxZonedInt64Digits, s)
}

// ReadZonedBig reads the next zoned decimal field of digits digits as a
// [math/big.Int], consuming digits bytes, or digits+1 under a SEPARATE sign
// position.
//
// digits must be between 1 and 31, the IBM Enterprise COBOL maximum. This is
// the accessor for the 19-to-31 digit range no Go integer type holds; below 19
// digits the int32 and int64 accessors say the same thing without allocating.
// s is required; see [Reader.ReadZonedInt32].
func (r *Reader) ReadZonedBig(digits int, s SignPosition) (*big.Int, error) {
	ds, negative, err := r.readZonedDigits(digits, maxZonedDigits, s)
	if err != nil {
		return nil, err
	}
	v := new(big.Int)
	ten := big.NewInt(10)
	d := new(big.Int)
	for _, n := range ds {
		v.Mul(v, ten)
		v.Add(v, d.SetInt64(int64(n)))
	}
	if negative {
		v.Neg(v)
	}
	return v, nil
}

// readZonedInt is the shared body of the two integer zoned accessors, whose
// only difference is the digit count they accept.
func (r *Reader) readZonedInt(digits, max int, s SignPosition) (int64, error) {
	ds, negative, err := r.readZonedDigits(digits, max, s)
	if err != nil {
		return 0, err
	}
	var v int64
	for _, d := range ds {
		v = v*10 + int64(d)
	}
	if negative {
		v = -v
	}
	return v, nil
}

// readZonedDigits reads one zoned decimal field and returns its digits, most
// significant first and one per element with values 0-9, together with whether
// the sign made it negative.
//
// It is the layer that turns a [SignPosition] into a width and an index: the
// separate sign byte, when there is one, is split off here, and everything left
// is the digit bytes [zonedCodec.decodeField] validates.
//
// Every error is stamped with the offset of the byte at fault rather than the
// offset the field ended at, for the reason [Reader.readPackedField] stamps
// the byte holding a bad nibble: a zoned field is several bytes wide, and "the
// field ended at offset N" does not say which byte was wrong.
func (r *Reader) readZonedDigits(digits, max int, s SignPosition) ([]byte, bool, error) {
	if !s.valid() {
		return nil, false, &OffsetError{Offset: r.off, Err: SignPositionError{SignPosition: s}}
	}
	if digits < 1 || digits > max {
		return nil, false, &OffsetError{
			Offset: r.off,
			Err:    ZonedDigitCountError{Digits: digits, Max: max},
		}
	}
	if r.zonedErr != nil {
		return nil, false, &OffsetError{Offset: r.off, Err: r.zonedErr}
	}
	start := r.off
	b, err := r.read(zonedWidth(digits, s))
	if err != nil {
		return nil, false, err
	}

	// Split the separate sign byte, if there is one, off the digits. What
	// remains is digits bytes either way, and first is where it begins.
	var (
		negative bool
		first    int
	)
	switch s {
	case SignLeadingSeparate:
		first = 1
		if negative, err = r.zoned.bytes.separateSignValue(b[0]); err != nil {
			return nil, false, &OffsetError{Offset: start, Err: err}
		}
	case SignTrailingSeparate:
		if negative, err = r.zoned.bytes.separateSignValue(b[digits]); err != nil {
			return nil, false, &OffsetError{Offset: start + int64(digits), Err: err}
		}
	}

	ds, overpunched, at, err := r.zoned.decodeField(b[first:first+digits], s.overpunchAt(digits))
	if err != nil {
		return nil, false, &OffsetError{Offset: start + int64(first+at), Err: err}
	}
	if !s.separate() {
		negative = overpunched
	}
	return ds, negative, nil
}

// ReadPackedInt32 reads the next packed decimal (COMP-3, PACKED-DECIMAL) field
// of digits digits as an int32, consuming ceil((digits+1)/2) bytes.
//
// digits must be between 1 and 9, the most that always fits an int32; a wider
// field is a [PackedDigitCountError] rather than a silent overflow, and is read
// with [Reader.ReadPackedInt64] or [Reader.ReadPackedBig].
//
// The return is the unscaled integer. A PICTURE's V and P positions occupy no
// storage and are not recoverable from the bytes, so scale is not a decoding
// input: PIC S9(3)V99 COMP-3 holding -123.45 reads as -12345, and a generator
// emits the scale beside the field as a constant.
func (r *Reader) ReadPackedInt32(digits int) (int32, error) {
	v, err := r.readPackedInt(digits, maxPackedInt32Digits)
	return int32(v), err
}

// ReadPackedInt64 reads the next packed decimal field of digits digits as an
// int64, consuming ceil((digits+1)/2) bytes.
//
// digits must be between 1 and 18, the most that always fits an int64. The 19
// to 31 digit range an IBM packed item may declare is read with
// [Reader.ReadPackedBig]. As with every numeric accessor the return is the
// unscaled integer; see [Reader.ReadPackedInt32].
func (r *Reader) ReadPackedInt64(digits int) (int64, error) {
	return r.readPackedInt(digits, maxPackedInt64Digits)
}

// ReadPackedBig reads the next packed decimal field of digits digits as a
// [math/big.Int], consuming ceil((digits+1)/2) bytes.
//
// digits must be between 1 and 31, the IBM Enterprise COBOL maximum for a
// packed item. This is the accessor for the 19-to-31 digit range no Go integer
// type holds; below 19 digits the int32 and int64 accessors say the same thing
// without allocating. As with every numeric accessor the return is the unscaled
// integer; see [Reader.ReadPackedInt32].
func (r *Reader) ReadPackedBig(digits int) (*big.Int, error) {
	b, first, negative, err := r.readPackedField(digits, maxPackedDigits)
	if err != nil {
		return nil, err
	}
	v := new(big.Int)
	ten := big.NewInt(10)
	d := new(big.Int)
	for i := 0; i < digits; i++ {
		v.Mul(v, ten)
		v.Add(v, d.SetInt64(int64(nibbleOf(b, first+i))))
	}
	if negative {
		v.Neg(v)
	}
	return v, nil
}

// readPackedInt is the shared body of the two integer packed accessors, whose
// only difference is the digit count they accept.
//
// It folds the field's nibbles straight into the result rather than unpacking
// them into a slice first, which is what makes an integer packed read allocate
// nothing at all; see [Reader.readPackedField].
func (r *Reader) readPackedInt(digits, max int) (int64, error) {
	b, first, negative, err := r.readPackedField(digits, max)
	if err != nil {
		return 0, err
	}
	var v int64
	for i := 0; i < digits; i++ {
		v = v*10 + int64(nibbleOf(b, first+i))
	}
	if negative {
		v = -v
	}
	return v, nil
}

// nibbleOf returns nibble i of b, counting the high nibble of the first byte as
// nibble zero.
//
// It is what stands in for the unpacked nibble slice the packed bodies used to
// build: a field's digits are a contiguous run of nibble indices, so every
// caller walks that run and folds it, and no intermediate slice exists to
// allocate. Indexing one byte at a time rather than loading several is
// deliberate. [Reader.read] hands back a buffer sliced to the field's own
// width, so a load wider than the field either runs off the end of an owned
// slice or, behind the reused buffer, quietly reads the *next* field's bytes
// and reports a neighbour as this field's bad nibble — a fault that shows up
// only in a file long enough to have a next field.
func nibbleOf(b []byte, i int) byte {
	if i%2 == 0 {
		return b[i/2] >> 4
	}
	return b[i/2] & 0x0F
}

// readPackedField reads one packed decimal field and validates every nibble of
// it, returning the field's own bytes together with the index of the nibble
// holding its most significant digit and whether the sign nibble made it
// negative.
//
// The digits are nibbles first through first+digits-1 of b, read with
// [nibbleOf]. Returning them that way rather than as a slice of one digit per
// element is what removes the allocation the integer accessors used to pay:
// each of them folds the run into a number as it walks it, and [big.Int] is the
// only thing any packed read now puts on the heap.
//
// b views the buffer the [Reader] reuses and is valid only until the next read,
// so every caller must consume it before returning — none of them returns
// anything that views it.
//
// Every nibble is validated: the pad, each digit, and the sign. Rejecting them
// is the only defence a reader has against a record whose offsets have slipped,
// and against a file whose packed fields were destroyed by a naive EBCDIC
// character conversion — see codec/SPEC.md, "The COMP-3 conversion trap".
//
// A nibble error carries the offset of the byte the nibble sits in rather than
// the offset the field ended at, which is the one place in this package an
// [OffsetError] is stamped with something other than the current position: a
// bad nibble is diagnosable only if the byte holding it can be found.
//
// The order the nibbles are checked in is normative rather than incidental, and
// must not be rearranged. A corrupt field usually carries more than one bad
// nibble — 91.7% of the three-byte values a PIC S9(4) field can hold are
// invalid in more than one of the three roles at once — so which fault is
// reported, and which byte the offset names, is decided by this order and by
// nothing else. It is field order: the pad nibble, then the digit nibbles from
// most significant to least, then the sign, which makes the reported offset the
// earliest byte that went wrong. See codec/SPEC.md, "Fault precedence".
func (r *Reader) readPackedField(digits, max int) (b []byte, first int, negative bool, err error) {
	if digits < 1 || digits > max {
		return nil, 0, false, &OffsetError{
			Offset: r.off,
			Err:    PackedDigitCountError{Digits: digits, Max: max},
		}
	}
	start := r.off
	b, err = r.read(packedWidth(digits))
	if err != nil {
		return nil, 0, false, err
	}
	// nibbleAt is the offset of the byte holding nibble i, counted from the
	// first byte of the field.
	nibbleAt := func(i int) int64 { return start + int64(i/2) }

	// The pad nibble exists exactly when the digit count is even, because
	// digits+1 nibbles is then odd and rounds up to a whole byte. It is
	// nibble zero, the high nibble of the first byte.
	if digits%2 == 0 {
		if pad := nibbleOf(b, 0); pad != 0 {
			return nil, 0, false, &OffsetError{
				Offset: nibbleAt(0),
				Err:    PackedPadError{Nibble: pad},
			}
		}
	}
	// The sign takes the last nibble of the field, so the digits end one
	// nibble short of it.
	sign := 2*len(b) - 1
	first = sign - digits
	for i := 0; i < digits; i++ {
		if d := nibbleOf(b, first+i); d > 9 {
			return nil, 0, false, &OffsetError{
				Offset: nibbleAt(first + i),
				Err:    PackedDigitError{Nibble: d},
			}
		}
	}
	negative, err = packedSignIsNegative(nibbleOf(b, sign))
	if err != nil {
		return nil, 0, false, &OffsetError{Offset: nibbleAt(sign), Err: err}
	}
	return b, first, negative, nil
}

// ReadComp6Int32 reads the next COMP-6 field of digits digits as an int32,
// consuming ceil(digits/2) bytes.
//
// COMP-6 is the GnuCOBOL and Micro Focus packed decimal with no sign nibble at
// all: every nibble of the field is a digit nibble, and the item is always
// unsigned. The two encodings are not interchangeable, and they fail
// differently depending on the digit count: at an even count COMP-6 is a byte
// narrower — PIC 9(4) COMP-6 is two bytes where PIC 9(4) COMP-3 is three — so
// reading one as the other shifts every later field of the record, while at an
// odd count the widths coincide and what catches it is the nibbles, a COMP-3
// sign nibble landing where a digit belongs. See codec/SPEC.md, "COMP-6".
//
// There is no [Signedness] to declare, on this side or the writing one, because
// the encoding has nowhere to put a sign. A field whose PICTURE carries S is
// not a COMP-6 field.
//
// digits must be between 1 and 9, the most that always fits an int32; a wider
// field is a [PackedDigitCountError] rather than a silent overflow, and is read
// with [Reader.ReadComp6Int64] or [Reader.ReadComp6Big].
//
// The return is the unscaled integer, for the reason it is on
// [Reader.ReadPackedInt32]: a PICTURE's V and P positions occupy no storage.
func (r *Reader) ReadComp6Int32(digits int) (int32, error) {
	v, err := r.readComp6Int(digits, maxPackedInt32Digits)
	return int32(v), err
}

// ReadComp6Int64 reads the next COMP-6 field of digits digits as an int64,
// consuming ceil(digits/2) bytes.
//
// digits must be between 1 and 18, the most that always fits an int64; wider
// counts are read with [Reader.ReadComp6Big]. As with every numeric accessor
// the return is the unscaled integer; see [Reader.ReadComp6Int32], which also
// says why no [Signedness] is taken.
func (r *Reader) ReadComp6Int64(digits int) (int64, error) {
	return r.readComp6Int(digits, maxPackedInt64Digits)
}

// ReadComp6Big reads the next COMP-6 field of digits digits as a
// [math/big.Int], consuming ceil(digits/2) bytes.
//
// digits must be between 1 and 31, the same upper bound the COMP-3 accessors
// take. This is the accessor for the 19-to-31 digit range no Go integer type
// holds. As with every numeric accessor the return is the unscaled integer; see
// [Reader.ReadComp6Int32].
func (r *Reader) ReadComp6Big(digits int) (*big.Int, error) {
	b, first, err := r.readComp6Field(digits, maxPackedDigits)
	if err != nil {
		return nil, err
	}
	v := new(big.Int)
	ten := big.NewInt(10)
	d := new(big.Int)
	for i := 0; i < digits; i++ {
		v.Mul(v, ten)
		v.Add(v, d.SetInt64(int64(nibbleOf(b, first+i))))
	}
	return v, nil
}

// readComp6Int is the shared body of the two integer COMP-6 accessors, whose
// only difference is the digit count they accept.
//
// It folds the field's nibbles straight into the result, for the reason
// [Reader.readPackedInt] does.
func (r *Reader) readComp6Int(digits, max int) (int64, error) {
	b, first, err := r.readComp6Field(digits, max)
	if err != nil {
		return 0, err
	}
	var v int64
	for i := 0; i < digits; i++ {
		v = v*10 + int64(nibbleOf(b, first+i))
	}
	return v, nil
}

// readComp6Field reads one COMP-6 field and validates every nibble of it,
// returning the field's own bytes together with the index of the nibble holding
// its most significant digit.
//
// The digits are nibbles first through first+digits-1 of b, read with
// [nibbleOf], and b views the reused buffer under the same rule
// [Reader.readPackedField] returns it under.
//
// It reports no sign, because COMP-6 stores none. That is the whole of the
// difference from [Reader.readPackedField], and it is why the two are separate
// bodies rather than one with a flag: there is no sign nibble to skip, so the
// digits run to the very end of the field and the pad nibble appears on the
// opposite parity of digits.
//
// Every nibble is validated as a digit. Nothing in the field may be A-F, so
// none of COMP-3's sign alphabet is accepted anywhere — a value ending in a C
// or an F is a COMP-3 field being read at a COMP-6 offset, which is exactly the
// mistake worth failing on.
//
// A nibble error carries the offset of the byte the nibble sits in, for the
// reason [Reader.readPackedField] does, and the nibbles are checked in field
// order — pad, then digits from most significant to least — for the reason it
// does too. That precedence is normative here as well; see codec/SPEC.md,
// "Fault precedence".
func (r *Reader) readComp6Field(digits, max int) (b []byte, first int, err error) {
	if digits < 1 || digits > max {
		return nil, 0, &OffsetError{
			Offset: r.off,
			Err:    PackedDigitCountError{Digits: digits, Max: max},
		}
	}
	start := r.off
	b, err = r.read(comp6Width(digits))
	if err != nil {
		return nil, 0, err
	}
	// nibbleAt is the offset of the byte holding nibble i, counted from the
	// first byte of the field.
	nibbleAt := func(i int) int64 { return start + int64(i/2) }

	// The pad nibble exists exactly when the digit count is odd, which is the
	// opposite parity from COMP-3: there is no sign nibble making the count
	// up, so an odd digit count is what leaves half a byte over. It is the
	// high nibble of the first byte, as COMP-3's is.
	if digits%2 == 1 {
		if pad := nibbleOf(b, 0); pad != 0 {
			return nil, 0, &OffsetError{
				Offset: nibbleAt(0),
				Err:    PackedPadError{Nibble: pad},
			}
		}
	}
	// There is no sign nibble, so the digits run to the last nibble of the
	// field inclusive.
	first = 2*len(b) - digits
	for i := 0; i < digits; i++ {
		if d := nibbleOf(b, first+i); d > 9 {
			return nil, 0, &OffsetError{
				Offset: nibbleAt(first + i),
				Err:    PackedDigitError{Nibble: d},
			}
		}
	}
	return b, first, nil
}

// ReadBinaryInt16 reads the next binary (COMP, COMP-4, BINARY) field of digits
// digits, consuming the bytes [Encoding.Binary] gives that digit count — 2
// under the usual staircase, 1 under [BinarySize1248] or [BinarySizeSmallest]
// below three digits, 8 under [BinarySizeFull].
//
// digits must be between 1 and 4, the digit counts an int16 carries. The bytes
// are two's complement in the order [Encoding.ByteOrder] declares, which is
// required and never inferred: a wrong byte order yields a plausible wrong
// number and never an error. How many bytes there are is [Encoding.Binary]'s to
// say and is inferred no more than the order is: a wrong staircase shifts every
// field after this one.
//
// Range semantics are TRUNC(STD): a stored value whose magnitude the PICTURE
// cannot express is a [BinaryRangeError] rather than a silently over-wide
// reading. That check is what turns a wrong byte order into a first-record
// failure most of the time. Read a COMP-5 field, or a COMP field compiled under
// TRUNC(BIN), with [Reader.ReadComp5Int16] instead; see [Truncation].
//
// As with every numeric accessor the return is the unscaled integer, since a
// PICTURE's V occupies no storage; see [Reader.ReadPackedInt32].
func (r *Reader) ReadBinaryInt16(digits int) (int16, error) {
	v, err := r.readBinaryInt(digits, maxBinaryInt16Digits, 16, TruncStd)
	return int16(v), err
}

// ReadBinaryInt32 reads the next binary field of digits digits as a signed
// int32, consuming the bytes [Encoding.Binary] gives that digit count.
//
// digits must be between 1 and 9. PIC 9(5) COMP is four bytes and not five —
// the width is a staircase, not the digit count, and which staircase is
// [Encoding.Binary]'s to say. Range semantics are TRUNC(STD); see
// [Reader.ReadBinaryInt16].
func (r *Reader) ReadBinaryInt32(digits int) (int32, error) {
	v, err := r.readBinaryInt(digits, maxBinaryInt32Digits, 32, TruncStd)
	return int32(v), err
}

// ReadBinaryInt64 reads the next binary field of digits digits as a signed
// int64, consuming the 1 to 8 bytes [Encoding.Binary] gives that digit count.
//
// digits must be between 1 and 18. The 19-to-31 digit range an ARITH(EXTEND)
// item may declare is 16 bytes wide under every staircase and is read with
// [Reader.ReadBinaryBig]. Range semantics are TRUNC(STD); see
// [Reader.ReadBinaryInt16].
func (r *Reader) ReadBinaryInt64(digits int) (int64, error) {
	return r.readBinaryInt(digits, maxBinaryInt64Digits, 64, TruncStd)
}

// ReadBinaryUint64 reads the next binary field of digits digits as an unsigned
// uint64, consuming the 1 to 8 bytes [Encoding.Binary] gives that digit
// count.
//
// This is the accessor for an item whose PICTURE has no S. The distinction is
// not cosmetic and is not recoverable from the bytes: FF FF is 65535 read as an
// unsigned 2-byte item and -1 read as a signed one, so which accessor is called
// is what says which the copybook declared.
//
// digits must be between 1 and 18. Range semantics are TRUNC(STD); see
// [Reader.ReadBinaryInt16]. A PIC 9(4) COMP-5 item holding 65535 is outside the
// four-digit decimal range and is read with [Reader.ReadComp5Uint64].
func (r *Reader) ReadBinaryUint64(digits int) (uint64, error) {
	return r.readBinaryUint(digits, maxBinaryInt64Digits, TruncStd)
}

// ReadBinaryBig reads the next binary field of digits digits as a
// [math/big.Int], consuming the 1 to 8 bytes [Encoding.Binary] gives that digit
// count, or 16 beyond eighteen digits.
//
// digits must be between 1 and 31. This is the accessor for the 19-to-31 digit
// range that IBM Enterprise COBOL allows under ARITH(EXTEND) and that no Go
// integer type holds; below 19 digits the fixed-width accessors say the same
// thing without allocating. Range semantics are TRUNC(STD); see
// [Reader.ReadBinaryInt16].
//
// The bytes are read as two's complement, so the value is signed. An unsigned
// item of that width reads identically under TRUNC(STD), because 10^31 is far
// below the 2^127 at which the sign bit of a 16-byte field turns on; the
// readings part company only for a COMP-5 field, which is what
// [Reader.ReadComp5Big] documents.
func (r *Reader) ReadBinaryBig(digits int) (*big.Int, error) {
	return r.readBinaryBig(digits, TruncStd)
}

// ReadComp5Int16 reads the next COMP-5 field of digits digits as a signed
// int16, consuming the bytes [Encoding.Binary] gives that digit count.
//
// It is [Reader.ReadBinaryInt16] with TRUNC(BIN) range semantics: the value may
// use the full range of the storage rather than the decimal range of the
// PICTURE, so no range validation against the PICTURE is performed at all. Use
// it for USAGE COMP-5, which always means this, and for COMP or COMP-4 compiled
// under TRUNC(BIN) or GnuCOBOL's binary-truncate: no. See [Truncation].
//
// The full range of the storage is not always an int16's. Under
// [BinarySizeFull] the field is eight bytes however few digits it declares, and
// a value it legally holds may be too wide for the int16 this returns — that is
// a [BinaryAccessorRangeError], never a truncation, and the remedy is
// [Reader.ReadComp5Int64].
//
// COMP-5 is defined as *native* byte order on the platform that wrote it, which
// is a fact about the file and is declared through [Encoding.ByteOrder] like
// any other: this accessor does not assume one.
func (r *Reader) ReadComp5Int16(digits int) (int16, error) {
	v, err := r.readBinaryInt(digits, maxBinaryInt16Digits, 16, TruncBin)
	return int16(v), err
}

// ReadComp5Int32 reads the next COMP-5 field of digits digits as a signed
// int32, consuming the bytes [Encoding.Binary] gives that digit count. It is
// [Reader.ReadBinaryInt32] with TRUNC(BIN) range semantics; see
// [Reader.ReadComp5Int16], including for the staircase that can put a value
// beyond an int32 in a field this accessor accepts.
func (r *Reader) ReadComp5Int32(digits int) (int32, error) {
	v, err := r.readBinaryInt(digits, maxBinaryInt32Digits, 32, TruncBin)
	return int32(v), err
}

// ReadComp5Int64 reads the next COMP-5 field of digits digits as a signed
// int64, consuming the 1 to 8 bytes [Encoding.Binary] gives that digit count.
// It is [Reader.ReadBinaryInt64] with TRUNC(BIN) range semantics; see
// [Reader.ReadComp5Int16].
func (r *Reader) ReadComp5Int64(digits int) (int64, error) {
	return r.readBinaryInt(digits, maxBinaryInt64Digits, 64, TruncBin)
}

// ReadComp5Uint64 reads the next COMP-5 field of digits digits as an unsigned
// uint64, consuming the 1 to 8 bytes [Encoding.Binary] gives that digit count.
// It is [Reader.ReadBinaryUint64] with
// TRUNC(BIN) range semantics; see [Reader.ReadComp5Int16].
//
// This is the accessor a PIC 9(4) COMP-5 item holding 65535 needs: those two
// FF bytes are legal there and are outside the range TRUNC(STD) allows.
func (r *Reader) ReadComp5Uint64(digits int) (uint64, error) {
	return r.readBinaryUint(digits, maxBinaryInt64Digits, TruncBin)
}

// ReadComp5Big reads the next COMP-5 field of digits digits as a
// [math/big.Int], consuming the 1 to 8 bytes [Encoding.Binary] gives that digit
// count, or 16 beyond eighteen digits. It is [Reader.ReadBinaryBig] with
// TRUNC(BIN) range semantics; see [Reader.ReadComp5Int16].
//
// The bytes are read as two's complement over the full storage width, so a
// 16-byte field with its top bit set reads as a negative number. An unsigned
// 16-byte COMP-5 item carrying a value that large has no accessor here; below
// 8 bytes, [Reader.ReadComp5Uint64] is the unsigned reading.
func (r *Reader) ReadComp5Big(digits int) (*big.Int, error) {
	return r.readBinaryBig(digits, TruncBin)
}

// readBinaryField reads one binary field of at most 8 bytes, returning its
// bytes as a raw unsigned integer together with the field's width and the
// offset it began at.
//
// The width comes from [Encoding.Binary], the staircase the file was compiled
// under, and is never assumed: under [BinarySizeSmallest] it may be any of 1
// through 8, which is why the bytes go through [binaryUint] rather than
// straight to a [binary.ByteOrder] accessor.
func (r *Reader) readBinaryField(digits, max int) (raw uint64, width int, start int64, err error) {
	if digits < 1 || digits > max {
		return 0, 0, r.off, &OffsetError{
			Offset: r.off,
			Err:    BinaryDigitCountError{Digits: digits, Max: max},
		}
	}
	width = r.enc.Binary.width(digits)
	start = r.off
	b, err := r.read(width)
	if err != nil {
		return 0, 0, start, err
	}
	return binaryUint(r.enc.ByteOrder, b), width, start, nil
}

// readBinaryInt is the shared body of the signed fixed-width accessors, whose
// only differences are the digit count they accept, the bit width of the Go
// type they return and the truncation mode they validate under.
func (r *Reader) readBinaryInt(digits, max, bits int, t Truncation) (int64, error) {
	raw, width, start, err := r.readBinaryField(digits, max)
	if err != nil {
		return 0, err
	}
	v := signExtend(raw, width)
	// TRUNC(BIN) confines a value to its storage width and nothing else, and
	// the width is what was just read, so there is nothing left to check
	// against the PICTURE.
	if t == TruncStd {
		limit := int64(pow10[digits] - 1)
		if v < -limit || v > limit {
			return 0, &OffsetError{
				Offset: start,
				Err: BinaryRangeError{
					Value:      strconv.FormatInt(v, 10),
					Digits:     digits,
					Width:      width,
					Signedness: Signed,
					Truncation: t,
				},
			}
		}
	}
	// What is left to check is the accessor's own type. A staircase that
	// makes the field wider than that type — [BinarySizeFull] does, for
	// every count an int16 or int32 accessor takes — puts values in range
	// for the field and out of range for the caller's variable, and the
	// caller must hear about that rather than receive the low bytes.
	if bits < 64 && v != signExtend(uint64(v), bits/8) {
		return 0, &OffsetError{
			Offset: start,
			Err: BinaryAccessorRangeError{
				Value: strconv.FormatInt(v, 10),
				Width: width,
				Bits:  bits,
			},
		}
	}
	return v, nil
}

// readBinaryUint is readBinaryInt for an item whose PICTURE has no S: the same
// bytes read as an unsigned magnitude rather than as two's complement.
func (r *Reader) readBinaryUint(digits, max int, t Truncation) (uint64, error) {
	raw, width, start, err := r.readBinaryField(digits, max)
	if err != nil {
		return 0, err
	}
	if t == TruncStd && raw > pow10[digits]-1 {
		return 0, &OffsetError{
			Offset: start,
			Err: BinaryRangeError{
				Value:      strconv.FormatUint(raw, 10),
				Digits:     digits,
				Width:      width,
				Signedness: Unsigned,
				Truncation: t,
			},
		}
	}
	return raw, nil
}

// readBinaryBig is the shared body of the two [math/big.Int] accessors. It is
// separate from readBinaryInt because [binary.ByteOrder] has no 16-byte
// accessor: the widest fields are ordered a byte at a time.
func (r *Reader) readBinaryBig(digits int, t Truncation) (*big.Int, error) {
	if digits < 1 || digits > maxBinaryDigits {
		return nil, &OffsetError{
			Offset: r.off,
			Err:    BinaryDigitCountError{Digits: digits, Max: maxBinaryDigits},
		}
	}
	width := r.enc.Binary.width(digits)
	start := r.off
	b, err := r.read(width)
	if err != nil {
		return nil, err
	}
	orderBinaryBytes(r.enc.ByteOrder, b)

	v := new(big.Int).SetBytes(b)
	if b[0]&0x80 != 0 {
		// Two's complement: the stored bits are the value plus 2^(8*width).
		v.Sub(v, new(big.Int).Lsh(big.NewInt(1), uint(8*width)))
	}
	if t == TruncStd && v.CmpAbs(decimalLimit(digits)) > 0 {
		return nil, &OffsetError{
			Offset: start,
			Err: BinaryRangeError{
				Value:      v.String(),
				Digits:     digits,
				Width:      width,
				Signedness: Signed,
				Truncation: t,
			},
		}
	}
	return v, nil
}

// ReadFloat32 reads the next floating point (COMP-1) field as a float32,
// consuming 4 bytes.
//
// COMP-1 takes no PICTURE — the usage alone fixes the format — so this
// accessor takes no digit count and there is no scale to apply. It is the one
// numeric family in this package whose width is not a function of a digit
// count.
//
// What the four bytes mean comes entirely from [Encoding.Float], which is
// required and never inferred. Under [FloatIEEE] they are binary32 in the order
// [Encoding.ByteOrder] declares. Under [FloatHFP] they are IBM hexadecimal
// floating point and big-endian *regardless* of that axis: HFP predates any
// little-endian IBM platform and has no little-endian form, so byte order is
// not a question a COMP-1 field in that format asks.
//
// The two formats read each other's bytes without complaint. IEEE 1.0 is
// 3F 80 00 00 and reads as 0.03125 under HFP; HFP 1.0 is 41 10 00 00 and reads
// as 9.0 under IEEE. Neither is an error, a NaN or an out-of-range value — they
// are plausible numbers that pass every check downstream, which is why the axis
// has no default.
//
// HFP's exponent range is far wider than binary32's, so a COMP-1 field may
// legitimately hold a value no float32 expresses. That is a [FloatRangeError]
// rather than an infinity or a zero; see [FloatRangeError] for why it is not
// simply returned.
func (r *Reader) ReadFloat32() (float32, error) {
	start := r.off
	b, err := r.read(comp1Width)
	if err != nil {
		return 0, err
	}
	if r.enc.Float == FloatIEEE {
		return math.Float32frombits(r.enc.ByteOrder.Uint32(b)), nil
	}
	v := floatFromHFP(uint64(binary.BigEndian.Uint32(b)), comp1Width)
	f := float32(v)
	reason := ""
	switch {
	case math.IsInf(float64(f), 0):
		reason = "overflows a float32"
	case f == 0 && v != 0:
		reason = "underflows a float32 to zero"
	default:
		return f, nil
	}
	return 0, &OffsetError{
		Offset: start,
		Err: FloatRangeError{
			Value:  strconv.FormatFloat(v, 'g', -1, 64),
			Format: FloatHFP,
			Width:  comp1Width,
			Reason: reason,
		},
	}
}

// ReadFloat64 reads the next floating point (COMP-2) field as a float64,
// consuming 8 bytes.
//
// It is [Reader.ReadFloat32] one width up, and everything that accessor says
// about [Encoding.Float], about HFP being big-endian regardless of
// [Encoding.ByteOrder], and about the two formats reading each other silently
// holds here unchanged.
//
// It has no range failure. HFP's range, 16^-65 to 16^63, sits well inside a
// float64's, so every COMP-2 field decodes to a number. HFP long carries 56
// bits of fraction against a float64's 53, so a value using the last three is
// rounded to nearest on the way out — the one place in this package a decoded
// number is not exact.
func (r *Reader) ReadFloat64() (float64, error) {
	b, err := r.read(comp2Width)
	if err != nil {
		return 0, err
	}
	if r.enc.Float == FloatIEEE {
		return math.Float64frombits(r.enc.ByteOrder.Uint64(b)), nil
	}
	return floatFromHFP(binary.BigEndian.Uint64(b), comp2Width), nil
}

// Unmarshal reads data into v under the given encoding.
//
// enc is the first argument and is required, for the reason [Encoding] exists:
// none of its five axes has a default, and every one of them fails silently
// when wrong.
//
// Bytes left over after v has read what it wants are not an error — a data file
// is a sequence of records, and a record type reads its own length.
//
// It builds a [Reader] per record and drops it, which is one allocation and no
// wrapper around data. A caller stepping through many records can do better
// still by keeping one Reader and calling [Reader.Reset] per record: the
// encoding's derived tables and the scratch buffers are then paid once for the
// file rather than once for the record.
func Unmarshal(enc Encoding, data []byte, v Unmarshaler) error {
	r, err := NewBytesReader(data, enc)
	if err != nil {
		return err
	}
	if v == nil {
		return ErrNilValue
	}
	return v.UnmarshalCOBOL(r)
}
