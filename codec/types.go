// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package codec

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

// Charset is the character set of a data file: the mapping between the bytes
// of an alphanumeric field and the characters they spell.
//
// Charset *translation* is applied to alphanumeric and alphabetic fields and to
// nothing else. Numeric decoding operates on raw byte values, because digit
// bytes (F0-F9 in EBCDIC, 30-39 in ASCII) and overpunched sign zones are
// byte-level facts; routing them through a character translation would lose
// information and make the sign convention unrepresentable.
//
// Which charset is declared still matters to a numeric field, because it is
// what says whether that field's digits are spelled F0-F9 or 30-39 and whether
// a separate sign is 4E/60 or 2B/2D. Those byte values are compared, not
// translated — the distinction this paragraph and the one above it draw.
// See codec/SPEC.md, "Charset as a First-Class Axis".
//
// A Charset is an interface rather than an enum so that nil is detectable:
// [Encoding] must have an invalid zero value in every field. It is also the
// extension point for the code pages this package does not ship, since EBCDIC
// is not one table — cp037, cp500, cp1047 and cp1140 differ in where they put
// the bracket, currency and accent characters.
//
// ToUnicode must be total: every one of the 256 byte values must decode to
// some rune, because any byte may appear in a PIC X field and such fields are
// routinely used to carry binary payloads. FromUnicode may fail, since a
// caller can always ask to write a character the charset has no byte for.
//
// # Make the implementation comparable
//
// Alphanumeric reading is fastest for a charset that Go can compare with ==,
// because the whole ToUnicode mapping is then derived once and shared by every
// [Reader] using that charset. A pointer type qualifies, as does a struct of
// comparable fields; a struct holding a slice, a map, a function or an embedded
// interface does not. Both shipped charsets are empty structs, so every
// [ASCII] equals every other.
//
// A charset that is not comparable still works, produces identical output and
// is not slower than it would have been — it simply reads a field the per-byte
// way rather than through the shared table. Note the trap in the common
// decorator shape: struct{ Charset } embeds an interface and so is not
// comparable, while *struct{ Charset } is.
type Charset interface {
	// Name reports the charset's name, as it would be written in
	// documentation: "ASCII", "cp037".
	Name() string

	// ToUnicode decodes one byte to the character it spells. It is total: it
	// never fails, and never reports a substitution.
	ToUnicode(b byte) rune

	// FromUnicode encodes one character as the byte that spells it,
	// reporting false when the charset has no such byte.
	FromUnicode(r rune) (byte, bool)

	// Space reports the byte that pads a short alphanumeric field: 0x20 in
	// ASCII, 0x40 in EBCDIC.
	Space() byte
}

// ASCII returns the identity character set: byte b decodes to the code point
// numbered b.
//
// Bytes 0x80-0xFF are not ASCII characters. They decode to the equally
// numbered code points rather than to a replacement character, because a
// translation table must be total (codec/SPEC.md, "Alphanumeric and Alphabetic
// Items") — a reader must not fail on an untranslatable byte in a PIC X field.
// The mapping is therefore bijective over all 256 bytes, so alphanumeric data
// round-trips through it unchanged. Use [Reader.ReadBytes] where the bytes are
// a binary payload and no character reading is wanted at all.
func ASCII() Charset { return asciiCharset{} }

type asciiCharset struct{}

func (asciiCharset) Name() string { return "ASCII" }

func (asciiCharset) ToUnicode(b byte) rune { return rune(b) }

func (asciiCharset) FromUnicode(r rune) (byte, bool) {
	if r < 0 || r > 0xFF {
		return 0, false
	}
	return byte(r), true
}

func (asciiCharset) Space() byte { return 0x20 }

// CP037 returns the EBCDIC code page 037 character set, the US/Canada page and
// the default of IBM Enterprise COBOL on z/OS.
//
// cp037 is bijective over all 256 bytes, so alphanumeric data round-trips
// through it unchanged. It is one EBCDIC page among several: cp500, cp1047 and
// cp1140 agree with it on the digits, the letters and the space 0x40 — which is
// why zoned decimal can be specified charset-generically — but differ on '[',
// ']', '!', '^', '~' and the currency symbol.
func CP037() Charset { return cp037Charset{} }

type cp037Charset struct{}

func (cp037Charset) Name() string { return "cp037" }

func (cp037Charset) ToUnicode(b byte) rune { return cp037ToUnicode[b] }

func (cp037Charset) FromUnicode(r rune) (byte, bool) {
	b, ok := cp037FromUnicode[r]
	return b, ok
}

func (cp037Charset) Space() byte { return 0x40 }

// cp037ToUnicode maps each of the 256 cp037 bytes to its code point. The C1
// control characters occupy the positions EBCDIC leaves to controls; the
// printable range is what the table is really for.
var cp037ToUnicode = [256]rune{
	0x00, 0x01, 0x02, 0x03, 0x9C, 0x09, 0x86, 0x7F, // 00-07
	0x97, 0x8D, 0x8E, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, // 08-0F
	0x10, 0x11, 0x12, 0x13, 0x9D, 0x85, 0x08, 0x87, // 10-17
	0x18, 0x19, 0x92, 0x8F, 0x1C, 0x1D, 0x1E, 0x1F, // 18-1F
	0x80, 0x81, 0x82, 0x83, 0x84, 0x0A, 0x17, 0x1B, // 20-27
	0x88, 0x89, 0x8A, 0x8B, 0x8C, 0x05, 0x06, 0x07, // 28-2F
	0x90, 0x91, 0x16, 0x93, 0x94, 0x95, 0x96, 0x04, // 30-37
	0x98, 0x99, 0x9A, 0x9B, 0x14, 0x15, 0x9E, 0x1A, // 38-3F
	' ', 0xA0, 0xE2, 0xE4, 0xE0, 0xE1, 0xE3, 0xE5, // 40-47
	0xE7, 0xF1, 0xA2, '.', '<', '(', '+', '|', // 48-4F
	'&', 0xE9, 0xEA, 0xEB, 0xE8, 0xED, 0xEE, 0xEF, // 50-57
	0xEC, 0xDF, '!', '$', '*', ')', ';', 0xAC, // 58-5F
	'-', '/', 0xC2, 0xC4, 0xC0, 0xC1, 0xC3, 0xC5, // 60-67
	0xC7, 0xD1, 0xA6, ',', '%', '_', '>', '?', // 68-6F
	0xF8, 0xC9, 0xCA, 0xCB, 0xC8, 0xCD, 0xCE, 0xCF, // 70-77
	0xCC, '`', ':', '#', '@', '\'', '=', '"', // 78-7F
	0xD8, 'a', 'b', 'c', 'd', 'e', 'f', 'g', // 80-87
	'h', 'i', 0xAB, 0xBB, 0xF0, 0xFD, 0xFE, 0xB1, // 88-8F
	0xB0, 'j', 'k', 'l', 'm', 'n', 'o', 'p', // 90-97
	'q', 'r', 0xAA, 0xBA, 0xE6, 0xB8, 0xC6, 0xA4, // 98-9F
	0xB5, '~', 's', 't', 'u', 'v', 'w', 'x', // A0-A7
	'y', 'z', 0xA1, 0xBF, 0xD0, 0xDD, 0xDE, 0xAE, // A8-AF
	'^', 0xA3, 0xA5, 0xB7, 0xA9, 0xA7, 0xB6, 0xBC, // B0-B7
	0xBD, 0xBE, '[', ']', 0xAF, 0xA8, 0xB4, 0xD7, // B8-BF
	'{', 'A', 'B', 'C', 'D', 'E', 'F', 'G', // C0-C7
	'H', 'I', 0xAD, 0xF4, 0xF6, 0xF2, 0xF3, 0xF5, // C8-CF
	'}', 'J', 'K', 'L', 'M', 'N', 'O', 'P', // D0-D7
	'Q', 'R', 0xB9, 0xFB, 0xFC, 0xF9, 0xFA, 0xFF, // D8-DF
	'\\', 0xF7, 'S', 'T', 'U', 'V', 'W', 'X', // E0-E7
	'Y', 'Z', 0xB2, 0xD4, 0xD6, 0xD2, 0xD3, 0xD5, // E8-EF
	'0', '1', '2', '3', '4', '5', '6', '7', // F0-F7
	'8', '9', 0xB3, 0xDB, 0xDC, 0xD9, 0xDA, 0x9F, // F8-FF
}

// cp037FromUnicode is the inverse of cp037ToUnicode. The table is bijective, so
// the inverse is total over its 256 code points and loses nothing.
var cp037FromUnicode = func() map[rune]byte {
	m := make(map[rune]byte, len(cp037ToUnicode))
	for b, r := range cp037ToUnicode {
		m[r] = byte(b)
	}
	return m
}()

// alphaTable is the UTF-8 form of a [Charset]'s translation: for each of the
// 256 byte values, the bytes that [strings.Builder.WriteRune] would have
// written for the rune that charset spells it with.
//
// It exists because alphanumeric decoding used to make two calls per byte — an
// interface dispatch to [Charset.ToUnicode] and a [strings.Builder.WriteRune]
// that re-encoded the result — and those two were together the largest single
// cost in decoding a record. The mapping is knowable once: [Charset.ToUnicode]
// is documented total over all 256 byte values, so nothing about a byte's
// translation depends on the field it appears in.
//
// It is emphatically **not** a [256]byte. 128 of cp037's 256 bytes spell runes
// above U+007F, 97 of them inside the printable range the table exists for, and
// [ASCII] is no better — its ToUnicode is rune(b), the identity in *rune* space
// and not in byte space, so byte 0xE9 spells U+00E9, which is two UTF-8 bytes.
// Encoding all 256 values of either charset takes 384 bytes, not 256. A byte
// table, or a string(b) "ASCII is verbatim" shortcut, is silently lossy.
//
// The representation is a packed one so that the inner loop has no branch in
// it. enc[c] holds the UTF-8 encoding of byte c little-endian — encoding byte i
// in bits 8i..8i+7 — and width[c] says how many of those four bytes count. A
// translation therefore writes four bytes per input byte unconditionally and
// advances by the width, rather than switching on how wide the rune turned out
// to be.
type alphaTable struct {
	// enc[c] is the UTF-8 encoding of the rune byte c spells, packed
	// little-endian. Every UTF-8 encoding is at most four bytes, so one
	// uint32 holds any of them.
	enc [256]uint32
	// width[c] is how many bytes of enc[c] are significant, 1 to 4.
	width [256]uint8
	// space[c] reports that byte c decodes to U+0020, which is the padding
	// an alphanumeric field is stripped of.
	//
	// It is deliberately **not** [Charset.Space]. The strip this feeds
	// reproduces strings.TrimRight(s, " ") over the translated string, so
	// what it must find is every byte that spells a space and not the one
	// byte the charset nominates as *the* space: a bijective page can spell
	// U+0020 at more than one byte, and a page whose Space() byte does not
	// decode to U+0020 at all must strip nothing — which is what the
	// accessor did before this table existed. Rewriting this as
	// c == cs.Space() would change both cases.
	space [256]bool
	// max is the widest entry in width. It is what sizes a destination
	// buffer: a field of n bytes decodes to at most n*max bytes, and for
	// both shipped charsets max is 2 rather than 4, so the common case
	// reserves half of what a worst-case assumption would.
	max int
}

// utf8MaxRuneLen is the most bytes one rune occupies in UTF-8, and therefore
// the number [alphaTable.translate] stores per input byte before advancing by
// the real width. It is [utf8.UTFMax], named here so the four-byte
// stores below read as "one rune's worth" rather than as a magic number.
const utf8MaxRuneLen = utf8.UTFMax

// newAlphaTable derives the UTF-8 translation table of cs by calling
// [Charset.ToUnicode] for every one of the 256 byte values.
//
// Derived, never switched on a known page: a caller's own charset — cp500 or
// cp1047 wrapped around encoding/charmap, say — gets a table on exactly the
// same terms the two shipped ones do, which is the property
// TestAlphaTableMatchesWriteRuneForEveryByte pins.
//
// [utf8.EncodeRune] is what makes the result byte-identical to the loop this
// replaced. [strings.Builder.WriteRune] encodes through the same function, so a
// rune that is negative, a surrogate half or above [utf8.MaxRune] becomes
// U+FFFD here exactly as it did there — and nothing in the [Charset] contract
// forbids ToUnicode returning one, since it owes totality and nothing else.
func newAlphaTable(cs Charset) *alphaTable {
	t := new(alphaTable)
	var buf [utf8MaxRuneLen]byte
	for i := range 256 {
		n := utf8.EncodeRune(buf[:], cs.ToUnicode(byte(i)))
		var v uint32
		for j := n - 1; j >= 0; j-- {
			v = v<<8 | uint32(buf[j])
		}
		t.enc[i] = v
		t.width[i] = uint8(n)
		t.space[i] = n == 1 && buf[0] == ' '
		t.max = max(t.max, n)
	}
	return t
}

// trim strips a field's space padding from the end the justification does not
// put the value at: the trailing bytes under [JustifyLeft], the leading ones
// under [JustifyRight].
//
// It runs on the source bytes and answers in them, which is what lets the
// translation that follows both be shorter and land in a smaller buffer. That
// it agrees byte for byte with trimming U+0020 off the translated string is a
// property of UTF-8 rather than of this table: 0x20 is neither a lead byte nor
// a continuation byte, so a translated space is always a whole character and
// never part of one.
func (t *alphaTable) trim(src []byte, j Justification) []byte {
	if j == JustifyRight {
		i := 0
		for i < len(src) && t.space[src[i]] {
			i++
		}
		return src[i:]
	}
	i := len(src)
	for i > 0 && t.space[src[i-1]] {
		i--
	}
	return src[:i]
}

// translate writes the UTF-8 form of src into dst and returns the prefix of
// dst that holds it.
//
// The loop stores a whole rune's worth at every position and advances by the
// real width, so there is no branch on how wide the rune turned out to be and
// no capacity check per byte. That rests on dst being at least
// [alphaTable.fieldCap] of len(src) long: at the last byte the offset is at most
// (len(src)-1)*max, and fieldCap leaves max+utf8MaxRuneLen-1 bytes past it,
// which is at least the four a store writes for every max >= 1.
//
// The precondition is **checked once per field** rather than left to the caller,
// and a short dst is grown rather than refused. It is one comparison against a
// per-byte store that would otherwise be free to run off the end of a buffer
// somebody sized by a different rule — len(src)*max, say, or utf8.UTFMax times
// len(src) against a stale max — and the whole point of the branchless loop is
// that it does not bounds check itself. A caller that sizes dst correctly, which
// [Reader.ReadAlphanumericJustified] does, never allocates here.
//
// The returned prefix is capped so that its capacity is its length. The bytes
// past it are whatever the unconditional stores left there — the tail of a
// previous field, in a reused buffer — so capping is what stops an append in
// some later caller from publishing them, the same hazard [Reader.read] caps
// for. It is not about the string conversion below, which copies either way.
func (t *alphaTable) translate(dst, src []byte) []byte {
	if n := t.fieldCap(len(src)); len(dst) < n {
		dst = make([]byte, n)
	}
	off := 0
	for _, c := range src {
		binary.LittleEndian.PutUint32(dst[off:], t.enc[c])
		off += int(t.width[c])
	}
	return dst[:off:off]
}

// fieldCap is how long the destination of a width-n field must be: every byte
// contributes at most [alphaTable.max] bytes, plus the slack that lets the last
// one be written as a full four-byte store and then trimmed.
//
// The slack is what makes the loop branchless, and it is exactly what the
// bounds check on the final store needs: at the last byte the offset is at most
// (n-1)*max, and (n-1)*max + utf8MaxRuneLen <= n*max + utf8MaxRuneLen - 1 holds
// for every max >= 1.
func (t *alphaTable) fieldCap(n int) int {
	return n*t.max + utf8MaxRuneLen - 1
}

// alphaCache is a bounded, concurrency-safe cache of derived translation
// tables, keyed by the [Charset] each was derived from.
//
// It is a type rather than a pair of package-level variables so that its two
// edges — a hit returning the *same* table and a full cache still returning a
// correct one — are reachable from a test without reaching into package state.
// [alphaTables] is the one instance the package uses.
type alphaCache struct {
	// tables maps Charset to *alphaTable. Only a charset comparableCharset
	// has approved ever reaches it; see alphaCache.tableOf.
	tables sync.Map
	// len tracks how many entries tables holds, because sync.Map has no
	// length and max needs one. It is approximate under concurrent stores —
	// racing LoadOrStores can carry it a few past max — which is acceptable
	// for a bound whose purpose is "not unbounded" rather than "exactly n".
	len atomic.Int64
	// max is the entry ceiling. Past it the cache stops storing and keeps
	// answering: a charset first seen after that point gets a correct table
	// that is simply not retained.
	max int64
}

// alphaTables is the process-wide cache of derived tables, and the reason the
// table is not built anywhere more obvious.
//
// It cannot be built in [NewReader]: TestZonedAccessorsNeverTranslateThroughTheCharset
// counts [Charset.ToUnicode] calls from construction onward and requires zero,
// which is codec/SPEC.md's rule that numeric decoding never routes through the
// charset. Materialising 256 entries at construction breaks that rule the
// moment a Reader exists, whether or not it ever reads an alphanumeric field.
//
// And a per-[Reader] table is amortised over one record, because [Unmarshal]
// builds a Reader per record. #114 measured building it per Reader at 141 to
// 723 ns/op for NewReader and 1 alloc to 3, with the per-record path 1.86x
// slower; the figures quoted on [maxAlphaScratch] are a different run, of this
// implementation, on the machine in this pull request. The table outlives any
// one Reader either way, and the cache is where that fact is written down.
// TestUnmarshalDerivesTheTableOncePerCharset is what holds it true.
var alphaTables = &alphaCache{max: alphaTablesMax}

// alphaTablesMax bounds [alphaTables]. Each entry retains an [alphaTable] —
// 1024 bytes of enc, 256 of width, 256 of space and a word of max, so about
// 1.5KiB — plus whatever the charset itself holds, which puts the ceiling
// around 100KiB. It is far above the two entries a program using the shipped
// charsets occupies, and far above the handful a program wrapping several code
// pages would.
const alphaTablesMax = 64

// alphaTableOf returns the derived table of cs, or **nil** when cs must not be
// cached — which is [Reader.ReadAlphanumericJustified]'s signal to translate
// the charset directly instead.
//
// Nil rather than an uncached table, because an uncached table is the one
// answer that is worse than not having one. Deriving 256 entries costs 256
// [Charset.ToUnicode] calls and a 1.5KiB allocation, and [Unmarshal] builds a
// [Reader] per record — so a table built per Reader for a 22-byte-of-text
// record does an order of magnitude more work per record than the per-byte loop
// it was meant to replace. Returning nil keeps such a charset exactly as fast
// as it was before this table existed, rather than trading a big win for the
// charsets that can be cached against a loss for the ones that cannot.
func alphaTableOf(cs Charset) *alphaTable { return alphaTables.tableOf(cs) }

// tableOf is [alphaTableOf] against a particular cache.
func (c *alphaCache) tableOf(cs Charset) *alphaTable {
	// **The key is an interface, and an interface key is a panic waiting to
	// happen.** [Charset] promises nothing about comparability, so a caller
	// whose charset carries a slice or a map would panic inside the map
	// rather than in any code of their own — and inside sync.Map's store
	// path that panic escapes with its mutex held, which is why the question
	// is answered before the map is touched and never recovered from after.
	if !comparableCharset(cs) {
		return nil
	}
	if t, ok := c.tables.Load(cs); ok {
		return t.(*alphaTable)
	}
	t := newAlphaTable(cs)
	if c.len.Load() >= c.max {
		return t
	}
	actual, loaded := c.tables.LoadOrStore(cs, t)
	if !loaded {
		c.len.Add(1)
	}
	return actual.(*alphaTable)
}

// SignConvention is how an overpunched sign is spelled in the sign-carrying
// byte of a zoned decimal field.
//
// EBCDIC has one convention. ASCII has at least four in production use and they
// are mutually incompatible, so this cannot be modelled as a charset flag.
// Choosing wrong yields silently wrong signs — negative values read as positive
// — with no parse error, which is why it is a required [Encoding] field with an
// invalid zero value. See codec/SPEC.md, "Zoned Sign Conventions".
//
// The conventions are declared here, in the story that fixes the axes; the
// decoding they govern arrives with the zoned decimal accessors.
type SignConvention int

const (
	// SignUnset is the zero value. It is not a convention, and construction
	// fails on it.
	SignUnset SignConvention = iota
	// SignEBCDIC is the one EBCDIC convention: zone C positive, zone D
	// negative, zone F unsigned. Negative digits appear as '}' and 'J'-'R'.
	SignEBCDIC
	// SignASCIIZone37 is the Micro Focus and GnuCOBOL ASCII convention: zone
	// 3 positive, zone 7 negative. Negative digits appear as 'p'-'y'.
	SignASCIIZone37
	// SignTranslatedEBCDIC is what an EBCDIC file converted to ASCII carries:
	// the EBCDIC overpunch bytes put through a character translation, so
	// positive is 0x7B and 0x41-0x49 and negative is 0x7D and 0x4A-0x52. No
	// compiler produces it; a conversion does.
	SignTranslatedEBCDIC
	// SignRealia is the CA-Realia ASCII convention: zone 3 positive, zone 2
	// negative. Negative digits appear as a space and '!'-')'.
	SignRealia
)

// String implements the [fmt.Stringer] interface.
func (s SignConvention) String() string {
	switch s {
	case SignEBCDIC:
		return "ebcdic"
	case SignASCIIZone37:
		return "ascii-zone-3-7"
	case SignTranslatedEBCDIC:
		return "translated-ebcdic"
	case SignRealia:
		return "realia"
	case SignUnset:
		return "unset"
	}
	return "SignConvention(" + strconv.Itoa(int(s)) + ")"
}

// valid reports whether s names a convention rather than the zero value or an
// out-of-range one.
func (s SignConvention) valid() bool {
	return s >= SignEBCDIC && s <= SignRealia
}

// The bytes of a zoned decimal (USAGE DISPLAY) field come from two independent
// places, and keeping them apart is the whole of why the four sign conventions
// are mutually detectable:
//
//   - A plain digit byte is a *charset* fact — F0-F9 in EBCDIC, 30-39 in ASCII
//     — and [zonedBytes] derives it from the declared [Charset], as it does the
//     separate sign bytes 4E/60 and 2B/2D.
//   - The sign-carrying byte of a signed field is a *sign convention* fact and
//     is charset-independent, so [zonedSignTables] holds absolute byte values.
//
// Both are *compared* against a field's bytes, never translated through one.
// Asking a [Charset] for the byte of a character this package names itself is
// the encode direction applied to a constant; putting a file's numeric byte
// through [Charset.ToUnicode] is what codec/SPEC.md, "Charset as a First-Class
// Axis", forbids, because it would lose the overpunch zone that carries the
// sign.

// zonedBytes is the charset half of that split: the byte values the declared
// [Charset] gives the ten digits and the two separate sign characters.
//
// It is derived from the charset rather than switched on a known page, which is
// what lets a caller plug cp500, cp1047 or cp1140 in — as a wrapper around
// golang.org/x/text/encoding/charmap, say — without this package shipping a
// table for each, and without acquiring that dependency itself.
type zonedBytes struct {
	// charset is the charset's name, carried for the errors below.
	charset string
	// digits[d] is the byte spelling the plain (unsigned zone) digit d.
	digits [10]byte
	// digitOf inverts digits: digitOf[b] is one more than the digit byte b
	// spells, and noZonedDigit where it spells none. It is the read
	// direction of a mapping whose write direction is one array index, and
	// it is a table rather than a scan of digits because a scan costs d+1
	// comparisons per byte — which made reading all-nines data measurably
	// slower than reading all-zeros.
	digitOf [256]byte
	// plus and minus are the SIGN SEPARATE bytes, which are
	// charset-sensitive and sign-convention-independent.
	plus, minus byte
}

// noZonedDigit marks a byte spelling no digit in [zonedBytes.digitOf].
//
// It is *zero*, and the digits are stored biased by one so that it can be,
// because the zero value of [zonedBytes] has to reject every byte rather than
// read every byte as a 0. That value is reachable — zonedBytesOf returns it
// beside each of its errors, and a zonedBytes{} literal is one. The scan this
// table replaced rejected 255 of the 256 bytes on such a value; the table
// rejects all 256, differing on 0x00 alone and in the direction that makes an
// unchecked construction error loud. A sentinel above 9 would have inverted
// that, accepting every byte as a zero. It also saves pre-filling 256 bytes on
// every construction.
const noZonedDigit byte = 0

// zonedBytesOf derives the zoned decimal byte values of cs, reporting an
// [UnrepresentableRuneError] naming the first character it has no byte for.
//
// Neither shipped charset can fail this, and no charset that describes a real
// COBOL data file can: an encoding with no digits and no '+' cannot spell a
// numeric item at all, so failing here is better than reading one wrongly.
func zonedBytesOf(cs Charset) (zonedBytes, error) {
	z := zonedBytes{charset: cs.Name()}
	for d := range z.digits {
		b, ok := cs.FromUnicode(rune('0' + d))
		if !ok {
			return zonedBytes{}, UnrepresentableRuneError{Rune: rune('0' + d), Charset: z.charset}
		}
		z.digits[d] = b
	}

	// Invert digits, biased by one so that an unwritten entry is
	// noZonedDigit. The fill runs from 9 down to 0 so that the *lowest*
	// digit wins a byte two digits share, which is what slices.Index gave
	// before this table existed. Charset.FromUnicode is nowhere required to
	// be injective, so a caller's charset may well spell two digits with one
	// byte; a forward fill would then read that byte as the highest of them
	// and silently change what such a file decodes to.
	for d := len(z.digits) - 1; d >= 0; d-- {
		z.digitOf[z.digits[d]] = byte(d) + 1
	}
	for _, sep := range []struct {
		r   rune
		dst *byte
	}{{'+', &z.plus}, {'-', &z.minus}} {
		b, ok := cs.FromUnicode(sep.r)
		if !ok {
			return zonedBytes{}, UnrepresentableRuneError{Rune: sep.r, Charset: z.charset}
		}
		*sep.dst = b
	}
	return z, nil
}

// digitByte returns the byte spelling plain digit d, which is every byte of an
// unsigned field and every non-sign byte of a signed one.
func (z *zonedBytes) digitByte(d byte) (byte, error) {
	if d > 9 {
		return 0, errZonedDigitValue
	}
	return z.digits[d], nil
}

// digitValue reports the digit a plain digit byte spells, rejecting anything
// that is not one rather than coercing it: an EBCDIC F5 read under an ASCII
// charset is a wrong charset, and it is the first zoned field of the first
// record that says so.
func (z *zonedBytes) digitValue(b byte) (byte, error) {
	if d := z.digitOf[b]; d != noZonedDigit {
		return d - 1, nil
	}
	return 0, ZonedDigitError{Byte: b, Charset: z.charset, Zero: z.digits[0], Nine: z.digits[9]}
}

// separateSignByte returns the SIGN SEPARATE byte for a value of the given
// sign. A separate sign is charset-sensitive and convention-independent: 2B/2D
// in ASCII, 4E/60 in EBCDIC.
func (z *zonedBytes) separateSignByte(negative bool) byte {
	if negative {
		return z.minus
	}
	return z.plus
}

// separateSignValue reports whether a SIGN SEPARATE byte means the value is
// negative, rejecting any other byte.
//
// A separate-sign field is the one zoned form carrying no sign-convention
// information at all, which makes it the safest form to write and the form that
// gives a reader nothing to check a convention guess against — so this byte is
// the only thing there is to validate, and it is validated.
// There are two byte values to compare against, so this stays a comparison
// where digitValue became a table: a 256-byte lookup would be slower to build
// and no faster to consult. The order of the arms is load bearing all the same,
// for digitValue's reason — nothing requires Charset.FromUnicode to give '+'
// and '-' different bytes, and a switch takes the first matching arm, so a
// charset spelling both alike reads that byte as positive. That is the same
// answer the digit table's reverse fill preserves: the earlier candidate wins.
func (z *zonedBytes) separateSignValue(b byte) (bool, error) {
	switch b {
	case z.plus:
		return false, nil
	case z.minus:
		return true, nil
	}
	return false, ZonedSeparateSignError{Byte: b, Charset: z.charset, Plus: z.plus, Minus: z.minus}
}

// zonedSignTable is the sign convention half of the split: the absolute byte
// values the sign-carrying digit of a signed zoned field takes under one
// [SignConvention]. See codec/SPEC.md, "Zoned Sign Conventions".
type zonedSignTable struct {
	// positive, negative and unsigned are indexed by the digit 0-9 the byte
	// carries. A writer emits from positive and negative; a reader accepts
	// all three, since an unsigned-zone byte in a signed field is a
	// non-negative value rather than a corruption.
	positive, negative, unsigned [10]byte
	// lenientPositiveZones and lenientNegativeZones are the extra high
	// nibbles a reader accepts and a writer never emits, the low nibble
	// then being the digit. They exist only for [SignEBCDIC], because
	// z/Architecture decimal instructions accept more sign values than they
	// generate; the three ASCII conventions have no equivalent, and
	// admitting extra zones there would destroy the mutual detectability
	// the table below rests on.
	lenientPositiveZones, lenientNegativeZones []byte
}

// zonedSignTables is indexed by [SignConvention]; the [SignUnset] row is the
// zero table and is unreachable, because every entry point checks the
// convention first.
//
// The rows are what makes a wrong convention loud: for every pair of
// conventions, each one's negative bytes are invalid under the other. 7B read
// under [SignASCIIZone37] is a digit nibble of B, D5 under any ASCII convention
// is a zone no ASCII convention uses, and the lenient EBCDIC zones A, B and E
// are likewise untouched by all three.
var zonedSignTables = [...]zonedSignTable{
	SignEBCDIC: {
		positive: [10]byte{0xC0, 0xC1, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9},
		negative: [10]byte{0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9},
		unsigned: [10]byte{0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7, 0xF8, 0xF9},
		// A, C, E and F are positive and B and D negative to a z/Architecture
		// decimal instruction; C, D and F are the three a writer emits.
		lenientPositiveZones: []byte{0xA0, 0xE0},
		lenientNegativeZones: []byte{0xB0},
	},
	SignASCIIZone37: {
		positive: [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
		negative: [10]byte{0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79},
		unsigned: [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
	},
	SignTranslatedEBCDIC: {
		// cp037 C0-C9 are '{ABCDEFGHI' and D0-D9 are '}JKLMNOPQR', so an
		// EBCDIC-to-ASCII text conversion of SignEBCDIC data lands here.
		positive: [10]byte{0x7B, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47, 0x48, 0x49},
		negative: [10]byte{0x7D, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F, 0x50, 0x51, 0x52},
		unsigned: [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
	},
	SignRealia: {
		positive: [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
		// Zone 2: a negative zero is a space, and -1 to -9 are '!' to ')'.
		negative: [10]byte{0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29},
		unsigned: [10]byte{0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39},
	},
}

// zonedSignReading is one entry of [zonedSignReadings]: the digit a
// sign-carrying byte spells under one convention, with zonedSignNegative set
// when that byte makes the field negative, or noZonedSignReading when the byte
// names no digit at all.
type zonedSignReading byte

const (
	// noZonedSignReading marks a byte that is a [ZonedSignError] under the
	// convention indexing its row.
	noZonedSignReading zonedSignReading = 0xFF
	// zonedSignNegative is the sign bit of a reading; the digit is what is
	// left once it is cleared.
	zonedSignNegative zonedSignReading = 0x10
)

// digit reports the digit r spells, which is meaningful only when r is not
// noZonedSignReading.
func (r zonedSignReading) digit() byte { return byte(r &^ zonedSignNegative) }

// negative reports whether r makes the field negative.
func (r zonedSignReading) negative() bool { return r&zonedSignNegative != 0 }

// zonedSignReadings inverts [zonedSignTables]: zonedSignReadings[s][b] is the
// reading of sign-carrying byte b under convention s. The [SignUnset] row is
// the inverse of the zero table and is unreachable for the reason that row is,
// [signByteValue] checking the convention first.
//
// This is a table rather than the four scans it replaces because a rejected
// byte cost all four of them and a negative digit cost up to twenty
// comparisons. It is safe to precompute only because the rows are pairwise
// disjoint except where positive and unsigned coincide — [SignASCIIZone37] and
// [SignRealia], where the two rows agree on the answer as well as on the byte —
// so no byte's reading depends on which arm of the scan reached it first. That
// claim is not left to this comment: TestZonedSignByteValueMatchesTheScan
// checks all four conventions against a transcription of the scan, over every
// one of the 256 byte values.
var zonedSignReadings = buildZonedSignReadings()

// buildZonedSignReadings inverts every row of [zonedSignTables].
//
// The fills run in the *reverse* of the order [signByteValue]'s scan tried
// them — lenient, then unsigned, then positive, then negative — so a later fill
// overwrites an earlier one and the documented precedence survives: a byte in
// two rows reads as it did when the scan stopped at the first. Within a row the
// digits are filled from 9 down to 0 for the same reason slices.Index gave the
// lowest index, though no shipped row holds a duplicate.
//
// The lenient zones are filled only where the low nibble is 0-9, which is the
// scan's own guard, and by re-applying that guard byte by byte rather than by
// deriving bytes from the zone constants. Filling a whole zone would newly
// accept the eighteen bytes whose low nibble is A-F, and those are exactly the
// bytes that make a wrong convention loud.
func buildZonedSignReadings() [len(zonedSignTables)][256]zonedSignReading {
	var rs [len(zonedSignTables)][256]zonedSignReading
	for s := range rs {
		t := &zonedSignTables[s]
		for b := range rs[s] {
			rs[s][b] = noZonedSignReading
		}
		// The lenient zones are transcribed as the scan's own predicate —
		// low nibble a digit, high nibble one of the zones, negative
		// before positive — rather than derived by masking the zone
		// constants down to a high nibble. Masking would silently
		// normalize a malformed row, one whose zone carried a low nibble,
		// into ten bytes the scan matched none of; this arm accepts
		// exactly what the scan accepted for any row, well-formed or not.
		for b := range rs[s] {
			if byte(b)&0x0F > 9 {
				continue
			}
			zone := byte(b) & 0xF0
			switch {
			case slices.Contains(t.lenientNegativeZones, zone):
				rs[s][b] = zonedSignReading(byte(b)&0x0F) | zonedSignNegative
			case slices.Contains(t.lenientPositiveZones, zone):
				rs[s][b] = zonedSignReading(byte(b) & 0x0F)
			}
		}
		for d := len(t.unsigned) - 1; d >= 0; d-- {
			rs[s][t.unsigned[d]] = zonedSignReading(d)
		}
		for d := len(t.positive) - 1; d >= 0; d-- {
			rs[s][t.positive[d]] = zonedSignReading(d)
		}
		for d := len(t.negative) - 1; d >= 0; d-- {
			rs[s][t.negative[d]] = zonedSignReading(d) | zonedSignNegative
		}
	}
	return rs
}

// signByte returns the byte spelling digit d in the sign-carrying position of a
// signed zoned field whose value has the given sign, under convention s.
//
// It emits only the preferred encodings — the C and D zones under
// [SignEBCDIC], never the lenient A, B and E that signByteValue accepts — just
// as the packed writer emits only the C and D nibbles for a signed field. The
// unsigned zone is not reachable from here at all: an item with no S in its
// PICTURE has no sign-carrying byte, and every byte of it comes from
// [zonedBytes.digitByte].
func signByte(s SignConvention, d byte, negative bool) (byte, error) {
	if !s.valid() {
		return 0, EncodingError{Field: "Sign", Reason: "is required and has no default"}
	}
	if d > 9 {
		return 0, errZonedDigitValue
	}
	t := zonedSignTables[s]
	if negative {
		return t.negative[d], nil
	}
	return t.positive[d], nil
}

// signByteValue reports the digit the sign-carrying byte b spells under
// convention s and whether it makes the field negative.
//
// A byte that names no digit under s is a [ZonedSignError] and never a digit:
// coercing it — reading 7B as a 1 because its low nibble is B, say — is what
// turns a wrong convention from a first-record failure into silently wrong
// signs. See codec/SPEC.md, "Validation and detectability".
func signByteValue(s SignConvention, b byte) (digit byte, negative bool, err error) {
	if !s.valid() {
		return 0, false, EncodingError{Field: "Sign", Reason: "is required and has no default"}
	}
	// The precedence this used to scan for — negative, then positive, then
	// the unsigned zone, since an unsigned-zone byte in a signed field is a
	// non-negative value rather than a corruption, and only then the lenient
	// EBCDIC zones — is baked into the fill order of the table. See
	// buildZonedSignReadings.
	r := zonedSignReadings[s][b]
	if r == noZonedSignReading {
		return 0, false, ZonedSignError{Byte: b, Sign: s}
	}
	return r.digit(), r.negative(), nil
}

// zonedCodec is the byte-level half of zoned decimal: the charset facts and the
// sign convention that together say what one field's bytes mean.
//
// It holds no position or width information, because those come from the
// PICTURE and the SIGN clause rather than from the [Encoding] — the two axes
// this type is made of are properties of the *file*. The zoned accessors layer
// the sign position on top of it.
type zonedCodec struct {
	bytes zonedBytes
	sign  SignConvention
}

// newZonedCodec builds the zoned codec of a validated [Encoding].
func newZonedCodec(enc Encoding) (zonedCodec, error) {
	if err := enc.Validate(); err != nil {
		return zonedCodec{}, err
	}
	z, err := zonedBytesOf(enc.Charset)
	if err != nil {
		return zonedCodec{}, err
	}
	return zonedCodec{bytes: z, sign: enc.Sign}, nil
}

// encodeField writes the zoned decimal bytes of ds — one digit per element,
// most significant first, each 0-9 — into dst, which must be the same length.
//
// signAt is the index of the byte the sign is overpunched into: the last byte
// under SIGN IS TRAILING, the first under SIGN IS LEADING, and -1 for an
// unsigned field or one whose sign is SEPARATE, both of which carry the plain
// digit zone throughout. negative is ignored when signAt is negative, since an
// unsigned field has nowhere to record a sign.
//
// dst is left untouched unless the whole field encodes, so a rejected field
// writes nothing.
func (c *zonedCodec) encodeField(dst, ds []byte, signAt int, negative bool) error {
	if len(dst) != len(ds) {
		return errZonedFieldWidth
	}
	if signAt >= len(ds) {
		return errZonedSignPosition
	}
	buf := make([]byte, len(ds))
	for i, d := range ds {
		var (
			b   byte
			err error
		)
		if i == signAt {
			b, err = signByte(c.sign, d, negative)
		} else {
			b, err = c.bytes.digitByte(d)
		}
		if err != nil {
			return err
		}
		buf[i] = b
	}
	copy(dst, buf)
	return nil
}

// decodeField reads the digits of one zoned decimal field, returning them one
// per element with values 0-9, most significant first, together with whether
// the sign-carrying byte made the field negative.
//
// signAt selects the sign-carrying byte as it does for
// [zonedCodec.encodeField], and -1 reads every byte as a plain digit and
// reports a non-negative value.
//
// at is the index within src of the offending byte and is meaningful only
// alongside a non-nil error. A zoned field is several bytes wide, so the caller
// stamps start+at rather than the offset the field ended at — the same reason
// [Reader.readPackedField] stamps the byte holding a bad nibble.
func (c *zonedCodec) decodeField(src []byte, signAt int) (ds []byte, negative bool, at int, err error) {
	if signAt >= len(src) {
		return nil, false, 0, errZonedSignPosition
	}
	ds = make([]byte, len(src))
	for i, b := range src {
		var (
			d   byte
			err error
		)
		if i == signAt {
			d, negative, err = signByteValue(c.sign, b)
		} else {
			d, err = c.bytes.digitValue(b)
		}
		if err != nil {
			return nil, false, i, err
		}
		ds[i] = d
	}
	return ds, negative, 0, nil
}

// SignPosition is where a zoned decimal (USAGE DISPLAY) item keeps its sign:
// whether its PICTURE carries S at all, and what the SIGN clause says about the
// byte the sign lives in.
//
// It is named apart from [SignConvention] because the two are different kinds
// of fact and both are needed to read one field. *Position* comes from the
// copybook — the PICTURE and the SIGN clause — and says which byte carries the
// sign and how wide the field is. *Convention* comes from the file and says how
// that byte is spelled. Neither implies the other, and no file states the
// first.
//
// It subsumes [Signedness] for zoned items, which is why the zoned accessors
// take no Signedness of their own: [SignUnsigned] is an item whose PICTURE has
// no S, and the other four are items that have one. A negative value written
// into a [SignUnsigned] field is a [ZonedRangeError] rather than a silent
// absolute value, exactly as it is for a packed [Unsigned] one.
//
// Like [Signedness] and unlike [Justification], its zero value is invalid
// rather than a default. COBOL's default applies only to an item that already
// has an S — SIGN IS TRAILING — so there is no one position an item with no
// clause at all can be assumed to have, and assuming wrongly shifts every later
// field of the record by the byte a SEPARATE sign takes. See codec/SPEC.md,
// "Sign position".
type SignPosition int

const (
	// SignPositionUnset is the zero value. It names no position, and every
	// zoned accessor rejects it.
	SignPositionUnset SignPosition = iota
	// SignUnsigned is an item whose PICTURE has no S. Every byte carries the
	// plain digit zone, no sign is stored and none is read, so such a field
	// is sign-convention-independent and only charset-sensitive.
	SignUnsigned
	// SignTrailing is SIGN IS TRAILING, the COBOL default for a signed item:
	// the sign is overpunched into the zone of the *last* digit byte, and
	// the field is digits bytes wide.
	SignTrailing
	// SignLeading is SIGN IS LEADING: the sign is overpunched into the zone
	// of the *first* digit byte, and the field is still digits bytes wide.
	SignLeading
	// SignTrailingSeparate is SIGN IS TRAILING SEPARATE CHARACTER: the sign
	// takes a byte of its own, '+' or '-', *after* the digits, so the field
	// is digits+1 bytes wide and every digit byte carries the plain zone.
	SignTrailingSeparate
	// SignLeadingSeparate is SIGN IS LEADING SEPARATE CHARACTER: the same
	// extra byte *before* the digits.
	SignLeadingSeparate
)

// String implements the [fmt.Stringer] interface.
func (s SignPosition) String() string {
	switch s {
	case SignUnsigned:
		return "unsigned"
	case SignTrailing:
		return "trailing"
	case SignLeading:
		return "leading"
	case SignTrailingSeparate:
		return "trailing-separate"
	case SignLeadingSeparate:
		return "leading-separate"
	case SignPositionUnset:
		return "unset"
	}
	return "SignPosition(" + strconv.Itoa(int(s)) + ")"
}

// valid reports whether s names a position rather than the zero value or an
// out-of-range one.
func (s SignPosition) valid() bool {
	return s >= SignUnsigned && s <= SignLeadingSeparate
}

// separate reports whether the sign takes a byte of its own, which is the one
// thing about a sign position that changes a field's width.
func (s SignPosition) separate() bool {
	return s == SignTrailingSeparate || s == SignLeadingSeparate
}

// overpunchAt reports the index within the digit bytes of the one the sign is
// overpunched into, or -1 when no byte is: an unsigned item has no sign, and a
// SEPARATE one keeps it outside the digits. It is the signAt of
// [zonedCodec.encodeField] and [zonedCodec.decodeField].
func (s SignPosition) overpunchAt(digits int) int {
	switch s {
	case SignTrailing:
		return digits - 1
	case SignLeading:
		return 0
	}
	return -1
}

// Digit counts a zoned decimal accessor accepts. As with the packed and binary
// accessors the limit is the range of the Go type the accessor returns and not
// a property of the field: 9 digits is the most that always fits an int32 and
// 18 the most that always fits an int64. 31 is the IBM Enterprise COBOL
// maximum, reachable through [Reader.ReadZonedBig] and [Writer.WriteZonedBig].
const (
	maxZonedInt32Digits = 9
	maxZonedInt64Digits = 18
	maxZonedDigits      = 31
)

// zonedWidth reports the byte width of a zoned decimal field holding digits
// digits under sign position s: one byte per digit, plus one for a SEPARATE
// sign. It is the whole of the zoned size model.
//
// Like [packedWidth] and [binaryWidth] it does not depend on scale, and unlike
// them it is the digit count itself rather than a function of it — a V occupies
// no byte, so PIC S9(7)V99 is nine bytes and PIC S9(7)V99 SIGN LEADING
// SEPARATE is ten.
func zonedWidth(digits int, s SignPosition) int {
	if s.separate() {
		return digits + 1
	}
	return digits
}

// FloatFormat is the representation of COMP-1 and COMP-2 items.
//
// The two formats are incompatible and neither is self-describing: an IBM
// hexadecimal float holding 1.0 reads as 9.0 under IEEE with no signal at all,
// so this is a required [Encoding] field with an invalid zero value. See
// codec/SPEC.md, "Floating Point".
type FloatFormat int

const (
	// FloatUnset is the zero value. It is not a format, and construction
	// fails on it.
	FloatUnset FloatFormat = iota
	// FloatIEEE is IEEE 754 binary32 and binary64, what GnuCOBOL and Micro
	// Focus write and what IBM writes under FLOAT(NATIVE).
	FloatIEEE
	// FloatHFP is IBM hexadecimal floating point, what IBM Enterprise COBOL
	// writes by default.
	FloatHFP
)

// String implements the [fmt.Stringer] interface.
func (f FloatFormat) String() string {
	switch f {
	case FloatIEEE:
		return "ieee-754"
	case FloatHFP:
		return "ibm-hfp"
	case FloatUnset:
		return "unset"
	}
	return "FloatFormat(" + strconv.Itoa(int(f)) + ")"
}

// valid reports whether f names a format rather than the zero value or an
// out-of-range one.
func (f FloatFormat) valid() bool {
	return f >= FloatIEEE && f <= FloatHFP
}

// Encoding is the complete byte-level interpretation of a data file: the four
// independent axes that must be known before a single byte can be read.
//
// Every field is required and every field's zero value is invalid, so an
// Encoding that was never filled in cannot be mistaken for a working one.
// [Encoding.Validate] reports the first field that is missing.
//
// These are independent axes and not one dialect flag. The combination real
// files hit most often and that no compiler produces is a mainframe-written
// file converted to ASCII — ASCII characters, translated-EBCDIC signs,
// big-endian binary — which a boolean "is it EBCDIC" cannot express. The named
// bundles ([IBMEnterprise] and friends) are constructors that fill in all four,
// never defaults the package applies on its own.
type Encoding struct {
	// Charset is the character set of the file. Its translation table is
	// applied to alphanumeric fields; for zoned decimal it fixes which byte
	// values the digit zone and a separate sign take, which are compared
	// rather than translated. See [Charset]. Required; nil is invalid.
	Charset Charset
	// Sign governs how an overpunched zoned decimal sign is spelled.
	// Required; [SignUnset] is invalid.
	Sign SignConvention
	// ByteOrder governs COMP, COMP-4 and COMP-5 binary integers. Required;
	// nil is invalid. Use [binary.BigEndian], [binary.LittleEndian] or
	// [binary.NativeEndian].
	ByteOrder binary.ByteOrder
	// Float governs COMP-1 and COMP-2. Required; [FloatUnset] is invalid.
	Float FloatFormat
}

// Validate reports whether every axis has been declared, returning an
// [EncodingError] naming the first field that has not. [NewReader] and
// [NewWriter] call it, so a caller rarely needs to.
func (e Encoding) Validate() error {
	if e.Charset == nil {
		return EncodingError{Field: "Charset", Reason: "is required and has no default"}
	}
	if e.Sign == SignUnset {
		return EncodingError{Field: "Sign", Reason: "is required and has no default"}
	}
	if !e.Sign.valid() {
		return EncodingError{Field: "Sign", Reason: "has unknown value " + strconv.Itoa(int(e.Sign))}
	}
	if e.ByteOrder == nil {
		return EncodingError{Field: "ByteOrder", Reason: "is required and has no default"}
	}
	if e.Float == FloatUnset {
		return EncodingError{Field: "Float", Reason: "is required and has no default"}
	}
	if !e.Float.valid() {
		return EncodingError{Field: "Float", Reason: "has unknown value " + strconv.Itoa(int(e.Float))}
	}
	return nil
}

// IBMEnterprise returns the encoding of a file written by IBM Enterprise COBOL
// on z/OS with its defaults: cp037 EBCDIC characters, EBCDIC overpunched signs,
// big-endian binary, and IBM hexadecimal floating point.
//
// A site compiling under FLOAT(NATIVE) writes IEEE floats instead; set
// [Encoding.Float] to [FloatIEEE] on the returned value.
func IBMEnterprise() Encoding {
	return Encoding{
		Charset:   CP037(),
		Sign:      SignEBCDIC,
		ByteOrder: binary.BigEndian,
		Float:     FloatHFP,
	}
}

// MicroFocusASCII returns the encoding of a file written by Micro Focus Visual
// COBOL or COBOL Server on ASCII platforms: ASCII characters, zone 3/7 signs,
// the host's native byte order, and IEEE floats.
//
// Byte order is native because that is the Micro Focus default; a file written
// under the IBM compatibility directives is big-endian, so set
// [Encoding.ByteOrder] to [binary.BigEndian] for one of those.
func MicroFocusASCII() Encoding {
	return Encoding{
		Charset:   ASCII(),
		Sign:      SignASCIIZone37,
		ByteOrder: binary.NativeEndian,
		Float:     FloatIEEE,
	}
}

// GnuCOBOLASCII returns the encoding of a file written by GnuCOBOL with its
// default configuration: ASCII characters, zone 3/7 signs (display-sign),
// big-endian binary, and IEEE floats.
//
// Byte order follows GnuCOBOL's own binary-byteorder setting, whose default is
// big-endian. A build configured with binary-byteorder: native writes the
// host's order instead, so set [Encoding.ByteOrder] to [binary.NativeEndian]
// for one of those — it is the one axis of this bundle that a GnuCOBOL
// configuration routinely moves.
func GnuCOBOLASCII() Encoding {
	return Encoding{
		Charset:   ASCII(),
		Sign:      SignASCIIZone37,
		ByteOrder: binary.BigEndian,
		Float:     FloatIEEE,
	}
}

// ConvertedFromEBCDIC returns the encoding of a mainframe-written file that has
// since been converted to ASCII: ASCII characters, translated-EBCDIC signs,
// big-endian binary, and IBM hexadecimal floats.
//
// No compiler produces this combination — a conversion does, and it is common
// in the field. The character fields were translated and the overpunched signs
// went through the same translation, while the binary and floating-point fields
// kept whatever the mainframe wrote. It is expressible only because the four
// axes are independent.
//
// Packed decimal fields are the hazard in such a file rather than a setting:
// COMP-3 is charset-invariant, so a conversion that touched them corrupted
// them, and no encoding can recover that.
func ConvertedFromEBCDIC() Encoding {
	return Encoding{
		Charset:   ASCII(),
		Sign:      SignTranslatedEBCDIC,
		ByteOrder: binary.BigEndian,
		Float:     FloatHFP,
	}
}

// Justification is which end of a fixed-width alphanumeric field the value sits
// at, and therefore which end carries the space padding.
//
// Unlike the [Encoding] axes, this is declared per field by the copybook rather
// than per file, and its zero value is the COBOL default rather than an error:
// an item with no JUSTIFIED clause is [JustifyLeft].
type Justification int

const (
	// JustifyLeft is the default: the value sits at the left of the field and
	// the padding is on the right.
	JustifyLeft Justification = iota
	// JustifyRight is JUSTIFIED RIGHT: the value sits at the right of the
	// field and the padding is on the left.
	JustifyRight
)

// String implements the [fmt.Stringer] interface.
func (j Justification) String() string {
	switch j {
	case JustifyLeft:
		return "left"
	case JustifyRight:
		return "right"
	}
	return "Justification(" + strconv.Itoa(int(j)) + ")"
}

// Signedness is whether a numeric item carries an operational sign — the S in
// its PICTURE — and therefore which sign value is stored with it.
//
// It is a third, independent thing from the two the SPEC's naming note already
// separates. *Sign convention* ([SignConvention]) is a property of the file in
// hand; *sign position* (LEADING, TRAILING, SEPARATE) is a property of the
// copybook and applies to USAGE DISPLAY only; Signedness is a property of the
// copybook that applies to every numeric usage. `PIC S9(5) COMP-3` is signed
// and stores C or D; `PIC 9(5) COMP-3` is unsigned and stores F. See
// codec/SPEC.md, "Sign".
//
// Unlike [Justification], whose zero value is COBOL's own default, Signedness
// has an invalid zero value and every writer takes it explicitly. There is no
// safe default: writing a signed field as unsigned discards the sign of every
// negative value, and writing an unsigned field as signed puts a C where a
// consumer expects an F. Neither is recoverable from the value being written,
// so the caller states it.
type Signedness int

const (
	// SignednessUnset is the zero value. It names neither, and every writer
	// that takes a Signedness rejects it.
	SignednessUnset Signedness = iota
	// Signed is an item whose PICTURE carries S: it stores C when positive
	// and D when negative.
	Signed
	// Unsigned is an item whose PICTURE has no S: it stores F, and a
	// negative value is rejected rather than silently stored as its
	// absolute value.
	Unsigned
)

// String implements the [fmt.Stringer] interface.
func (s Signedness) String() string {
	switch s {
	case Signed:
		return "signed"
	case Unsigned:
		return "unsigned"
	case SignednessUnset:
		return "unset"
	}
	return "Signedness(" + strconv.Itoa(int(s)) + ")"
}

// valid reports whether s names a member rather than the zero value or an
// out-of-range one.
func (s Signedness) valid() bool {
	return s >= Signed && s <= Unsigned
}

// Digit counts a packed decimal accessor accepts. The limit is the range of
// the Go type the accessor returns and not a property of the field: 9 digits
// is the most that always fits an int32 and 18 the most that always fits an
// int64, which is the same digit-count staircase COBOL uses to pick a binary
// width. 31 is the IBM Enterprise COBOL maximum for a packed item, reachable
// through [Reader.ReadPackedBig] and [Writer.WritePackedBig].
const (
	maxPackedInt32Digits = 9
	maxPackedInt64Digits = 18
	maxPackedDigits      = 31
)

// Packed decimal sign nibbles. A writer emits only these three; a reader also
// accepts packedSignA and packedSignE as positive and packedSignB as negative,
// which is what z/Architecture decimal instructions do. See codec/SPEC.md,
// "Sign nibble".
const (
	packedSignPositive byte = 0x0C
	packedSignNegative byte = 0x0D
	packedSignUnsigned byte = 0x0F
)

// packedWidth reports the byte width of a packed decimal field holding digits
// digits: ceil((digits + 1) / 2), one nibble per digit plus the sign nibble,
// rounded up to a whole byte. It is the whole of the packed size model, and it
// does not depend on scale.
func packedWidth(digits int) int { return (digits + 2) / 2 }

// comp6Width reports the byte width of a COMP-6 field holding digits digits:
// ceil(digits / 2), one nibble per digit and nothing else, rounded up to a
// whole byte.
//
// It is deliberately its own function rather than packedWidth with an
// adjustment, because COMP-6 carries no sign nibble at all: PIC 9(4) COMP-6 is
// two bytes where PIC 9(4) COMP-3 is three, and reading one as the other
// desynchronizes the record. See codec/SPEC.md, "Storage Widths".
func comp6Width(digits int) int { return (digits + 1) / 2 }

// packedSignIsNegative reports whether a sign nibble means the value is
// negative, rejecting the nibbles 0-9 that mean nothing at all.
//
// The accepted set is wider than the emitted one: A, C, E and F are positive
// and B and D are negative, because z/Architecture decimal instructions accept
// more sign values than they generate and real files carry them.
func packedSignIsNegative(nibble byte) (bool, error) {
	switch nibble {
	case 0x0A, 0x0C, 0x0E, 0x0F:
		return false, nil
	case 0x0B, 0x0D:
		return true, nil
	}
	return false, PackedSignError{Nibble: nibble}
}

// Truncation is the range semantics of a binary (COMP, COMP-4, BINARY,
// COMP-5) item: what the digit count in its PICTURE says about the values the
// item may hold.
//
// It is the IBM TRUNC compiler option, spelled binary-truncate by GnuCOBOL, and
// it is the one binary setting that is a property of the *compiler* rather than
// of the file in hand — which is why it is not an [Encoding] axis. Byte order
// is: the bytes were written by whatever wrote them and are read by something
// else, so it must be declared per file.
//
// Truncation is never passed to an accessor. It is selected by which accessor
// is called, because the two readings are the two families of methods on
// [Reader] and [Writer]:
//
//   - [Reader.ReadBinaryInt16] and friends are TRUNC(STD), the
//     standard-conforming reading: a PIC S9(4) COMP item holds -9999 to 9999
//     and a value outside that range is a [BinaryRangeError].
//   - [Reader.ReadComp5Int16] and friends are TRUNC(BIN): the same two bytes,
//     the full -32768 to 32767 range of the storage. This is what USAGE COMP-5
//     always means and what COMP and COMP-4 mean under TRUNC(BIN) or
//     binary-truncate: no.
//
// Truncation does not change the byte layout, so it never changes how a field
// is decoded — only which decoded values are accepted. That validation is worth
// having: under TRUNC(STD) it is the strongest available detector of a wrong
// [Encoding.ByteOrder], since a byte-swapped value usually overflows the
// PICTURE's decimal range. See codec/SPEC.md, "Range semantics: TRUNC".
type Truncation int

const (
	// TruncUnset is the zero value. It names no semantics; it appears only in
	// a zero-valued [BinaryRangeError], since no accessor takes a Truncation.
	TruncUnset Truncation = iota
	// TruncStd is TRUNC(STD), binary-truncate: yes: the item's range is the
	// decimal range of its PICTURE digit count.
	TruncStd
	// TruncBin is TRUNC(BIN), binary-truncate: no, and USAGE COMP-5: the
	// item's range is the full range of its storage width.
	TruncBin
)

// String implements the [fmt.Stringer] interface.
func (t Truncation) String() string {
	switch t {
	case TruncStd:
		return "trunc-std"
	case TruncBin:
		return "trunc-bin"
	case TruncUnset:
		return "unset"
	}
	return "Truncation(" + strconv.Itoa(int(t)) + ")"
}

// Digit counts a binary accessor accepts. As with the packed accessors the
// limit is the range of the Go type the accessor returns and not a property of
// the field: 4 digits is the most a 2-byte item holds, and a 2-byte item under
// TRUNC(BIN) is the most that always fits an int16. 31 is the IBM Enterprise
// COBOL maximum under ARITH(EXTEND), reachable through [Reader.ReadBinaryBig]
// and [Writer.WriteBinaryBig].
const (
	maxBinaryInt16Digits = 4
	maxBinaryInt32Digits = 9
	maxBinaryInt64Digits = 18
	maxBinaryDigits      = 31
)

// maxBinaryFieldWidth is the top step of [binaryWidth]'s staircase, the width
// of every field of more than 18 digits and so of the widest binary field the
// package reads. It is named rather than written twice because
// [maxNumericWidth] is derived from it: the staircase is the only place a
// binary width is a literal, and a scratch buffer sized from a second copy of
// that literal would not follow the step if it moved.
const maxBinaryFieldWidth = 16

// binaryWidth reports the byte width of a binary field holding digits digits:
// 2 bytes through 4 digits, 4 through 9, 8 through 18 and 16 beyond. It is the
// whole of the binary size model, and like [packedWidth] it does not depend on
// scale.
//
// The width is a staircase and not the digit count: PIC 9(5) COMP is *four*
// bytes, not five. Getting the step wrong shifts every field after it in the
// record, which is why the 4/5 and 9/10 boundaries carry their own tests.
//
// This is the IBM Enterprise COBOL table, which is also Micro Focus's and
// GnuCOBOL's binary-size: 2-4-8. A GnuCOBOL build left on its default
// binary-size: 1-2-4-8 gives a 1-2 digit item **one** byte instead of two;
// this package does not implement that variant, and a copybook compiled under
// it desynchronizes at the first such field rather than reading wrongly. See
// codec/SPEC.md, "Binary widths by digit count".
func binaryWidth(digits int) int {
	switch {
	case digits <= 4:
		return 2
	case digits <= 9:
		return 4
	case digits <= 18:
		return 8
	}
	return maxBinaryFieldWidth
}

// pow10 holds 10^i for every digit count the fixed-width binary accessors
// reach. 10^18 is the largest power of ten that fits a uint64, which is why
// those accessors stop at 18 digits and the [math/big.Int] ones compute their
// own bound.
var pow10 = func() [maxBinaryInt64Digits + 1]uint64 {
	var t [maxBinaryInt64Digits + 1]uint64
	t[0] = 1
	for i := 1; i < len(t); i++ {
		t[i] = t[i-1] * 10
	}
	return t
}()

// binaryDecimalLimits holds 10^i - 1 for every digit count a binary item may
// declare. It is precomputed for the reason pow10 is: the 19-to-31 digit path
// range-checks every field it reads or writes, and an exponentiation and two
// allocations per field is a hot loop's worth of work for 31 constants.
var binaryDecimalLimits = func() [maxBinaryDigits + 1]*big.Int {
	var t [maxBinaryDigits + 1]*big.Int
	ten := big.NewInt(10)
	one := big.NewInt(1)
	for i := range t {
		v := new(big.Int).Exp(ten, big.NewInt(int64(i)), nil)
		t[i] = v.Sub(v, one)
	}
	return t
}()

// decimalLimit reports 10^digits - 1, the largest magnitude a digits-digit
// item holds under [TruncStd]. digits must already have been validated against
// [maxBinaryDigits].
//
// The returned value is shared and must not be modified: every caller compares
// with it and none of them owns it.
func decimalLimit(digits int) *big.Int { return binaryDecimalLimits[digits] }

// isBigEndian reports whether bo puts the most significant byte first.
//
// The order is probed rather than compared against [binary.BigEndian], because
// [binary.NativeEndian] is a distinct type from both of the named orders and a
// caller may supply an implementation of its own.
func isBigEndian(bo binary.ByteOrder) bool {
	return bo.Uint16([]byte{0x01, 0x00}) == 0x0100
}

// orderBinaryBytes converts b between big-endian order and the order bo
// declares, in place. It is its own inverse — the conversion is a reversal in
// both directions — so the reader and the writer share it.
//
// It exists because [binary.ByteOrder] has no 16-byte accessor: the 19-to-31
// digit fields the [math/big.Int] accessors read and write cannot go through
// Uint64 or PutUint64 the way the narrower ones do.
func orderBinaryBytes(bo binary.ByteOrder, b []byte) {
	if isBigEndian(bo) {
		return
	}
	slices.Reverse(b)
}

// putBinaryUint writes the low 8*len(field) bits of raw into field in the
// order bo declares. field must be 2, 4 or 8 bytes; the 16-byte case belongs
// to the [math/big.Int] writers and goes through [orderBinaryBytes].
func putBinaryUint(bo binary.ByteOrder, field []byte, raw uint64) {
	switch len(field) {
	case 2:
		bo.PutUint16(field, uint16(raw))
	case 4:
		bo.PutUint32(field, uint32(raw))
	default:
		bo.PutUint64(field, raw)
	}
}

// signExtend reinterprets the low 8*width bits of raw as a two's complement
// signed integer, which is what a signed binary item stores over the whole of
// its storage width.
func signExtend(raw uint64, width int) int64 {
	shift := uint(64 - 8*width)
	return int64(raw<<shift) >> shift
}

// comp1Width and comp2Width are the storage widths of the two floating point
// usages. Neither takes a PICTURE — the usage alone fixes the format — so
// unlike every other numeric item their width is not a function of a digit
// count, and there is no digit count to pass an accessor.
const (
	comp1Width = 4
	comp2Width = 8
)

// hfpExponentBias is the bias of an IBM hexadecimal floating point exponent.
// The stored seven-bit field holds the exponent plus 64, base 16, so 0x41
// denotes 16^1 and 0x40 denotes 16^0.
const hfpExponentBias = 64

// hfpMaxExponent is the largest stored exponent a seven-bit field holds, and
// therefore the top of HFP's range: 16^63, since a fraction is always below 1.
const hfpMaxExponent = 0x7F

// hfpFracBits reports the width in bits of the fraction of an HFP field of
// width bytes. The sign takes one bit and the exponent seven; all the rest is
// fraction, which is 24 bits for the short (COMP-1) form and 56 for the long
// (COMP-2) one.
func hfpFracBits(width int) uint { return uint(8*width - 8) }

// floatFromHFP decodes the big-endian bit pattern of an IBM hexadecimal
// floating point field of width bytes to the number it denotes.
//
// The value is ±0.fraction₁₆ × 16^(exponent − 64). The fraction is a base-16
// fraction with an implied radix point to its left and — unlike IEEE — no
// implied leading one, which is why the two formats read each other's bytes as
// plausible wrong numbers rather than as errors.
//
// A zero fraction is zero whatever the exponent and whatever the sign bit,
// which is what makes the all-zero field the true zero. HFP has no negative
// zero, no NaN and no infinity: every bit pattern denotes an ordinary number.
//
// An unnormalized field — one whose leading hex digit is zero — denotes exactly
// what the formula gives and is decoded rather than rejected. Nothing here
// writes one, but z/OS arithmetic produces them.
func floatFromHFP(raw uint64, width int) float64 {
	fracBits := hfpFracBits(width)
	frac := raw & (1<<fracBits - 1)
	if frac == 0 {
		return 0
	}
	exp := int(raw>>fracBits) & hfpMaxExponent
	v := math.Ldexp(float64(frac), 4*(exp-hfpExponentBias)-int(fracBits))
	if (raw>>(fracBits+7))&1 != 0 {
		v = -v
	}
	return v
}

// hfpFromFloat encodes v as the big-endian bit pattern of an IBM hexadecimal
// floating point field of width bytes, normalized so that the leading hex digit
// of the fraction is non-zero.
//
// It reports a [FloatRangeError] rather than storing an approximation in the
// two cases where HFP has nothing to store:
//
//   - A NaN or an infinity. HFP encodes neither, and every pattern it does
//     have already denotes an ordinary number, so there is no bit pattern that
//     would read back as anything but a plausible finite value.
//   - A magnitude outside 16^-65 to 16^63. A float64 reaches far past both
//     ends, and clamping to an infinity or flushing to a zero would be the
//     same silent wrong number one step later.
//
// Normalizing to a hex digit boundary costs up to three bits of fraction —
// HFP's "wobbling precision" — so the fraction is rounded rather than
// truncated. A float64 survives the long form exactly, because 56 bits of
// fraction leave room for the three a 53-bit significand can lose; a float32
// written to the short form may not, since its 24 bits leave none.
func hfpFromFloat(v float64, width int) (uint64, error) {
	rangeErr := func(reason string) error {
		return FloatRangeError{
			Value:  strconv.FormatFloat(v, 'g', -1, 64),
			Format: FloatHFP,
			Width:  width,
			Reason: reason,
		}
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, rangeErr("is not a finite number, and HFP encodes neither NaN nor infinity")
	}
	if v == 0 {
		// True zero is the all-zero field, negative zero included: HFP has no
		// signed zero to preserve one as.
		return 0, nil
	}

	fracBits := hfpFracBits(width)
	f, e := math.Frexp(math.Abs(v)) // |v| = f × 2^e, with f in [0.5, 1)
	// The hex exponent is ceil(e/4), the smallest power of 16 that leaves the
	// fraction below one and so at least 1/16, which is normalization. The
	// shift floors towards minus infinity, which is what a negative e needs.
	hexExp := e >> 2
	if e&3 != 0 {
		hexExp++
	}
	frac := uint64(math.Round(math.Ldexp(f, int(fracBits)+e-4*hexExp)))
	if frac == 1<<fracBits {
		// Rounding carried out of the top hex digit, leaving the fraction at
		// its smallest normalized value one exponent up.
		frac >>= 4
		hexExp++
	}

	exp := hexExp + hfpExponentBias
	if exp > hfpMaxExponent {
		return 0, rangeErr("magnitude is above 16^63, the largest HFP value")
	}
	if exp < 0 {
		return 0, rangeErr("magnitude is below 16^-65, the smallest HFP value")
	}
	raw := uint64(exp)<<fracBits | frac
	if math.Signbit(v) {
		raw |= 1 << (fracBits + 7)
	}
	return raw, nil
}

// Marshaler is implemented by a value that can write itself to a data file.
// Generated record types implement it; the [Writer] it is handed already knows
// the [Encoding], so an implementation never chooses one.
type Marshaler interface {
	MarshalCOBOL(w *Writer) error
}

// Unmarshaler is implemented by a value that can read itself from a data file.
// It is the inverse of [Marshaler], and the [Reader] it is handed likewise
// already knows the [Encoding].
type Unmarshaler interface {
	UnmarshalCOBOL(r *Reader) error
}

// ErrNilReader is returned by [NewReader] when the underlying reader is nil.
var ErrNilReader = errors.New("nil reader")

// ErrNilWriter is returned by [NewWriter] when the underlying writer is nil.
var ErrNilWriter = errors.New("nil writer")

// ErrNilValue is returned by [Marshal] and [Unmarshal] when the value is nil.
var ErrNilValue = errors.New("nil value")

// OffsetError wraps every error a [Reader] or [Writer] returns after
// construction, recording the byte offset at which the failure was detected so
// that a bad byte deep inside a record is diagnosable.
//
// It is a wrapper rather than a field on each error so that the offset is
// stamped in one place and cannot drift. Callers reach past it with errors.Is
// for the underlying cause and errors.As for the offset:
//
//	var oe *codec.OffsetError
//	if errors.As(err, &oe) {
//	        log.Printf("bad byte at offset %d", oe.Offset)
//	}
type OffsetError struct {
	// Offset is the byte position at which the failure was detected,
	// counted from the start of the stream.
	Offset int64
	// Err is the underlying cause.
	Err error
}

// Error implements the [error] interface.
func (e *OffsetError) Error() string {
	return fmt.Sprintf("at byte offset %d: %v", e.Offset, e.Err)
}

// Unwrap implements the interface [errors.Unwrap] expects.
func (e *OffsetError) Unwrap() error { return e.Err }

// EncodingError is returned by [Encoding.Validate], and therefore by
// [NewReader] and [NewWriter], when an axis of the encoding was left undeclared
// or set to a value that names no member.
type EncodingError struct {
	// Field is the name of the offending [Encoding] field.
	Field string
	// Reason says what is wrong with it.
	Reason string
}

// Error implements the [error] interface.
func (e EncodingError) Error() string {
	return fmt.Sprintf("invalid encoding: field %s %s", e.Field, e.Reason)
}

// FieldWidthError is returned when a field width is negative. A width of zero
// is legal and reads or writes nothing.
type FieldWidthError struct {
	// Width is the width that was asked for.
	Width int
}

// Error implements the [error] interface.
func (e FieldWidthError) Error() string {
	return fmt.Sprintf("invalid field width %d: must not be negative", e.Width)
}

// FieldTooLongError is returned when a value does not fit the field it was
// asked to be written into.
//
// Writing truncates nothing: a COBOL MOVE into a short item silently drops
// characters, but a codec doing the same would write a record that no longer
// says what the caller asked it to, so this is loud.
type FieldTooLongError struct {
	// Len is the encoded length of the value, in bytes.
	Len int
	// Width is the width of the field, in bytes.
	Width int
}

// Error implements the [error] interface.
func (e FieldTooLongError) Error() string {
	return fmt.Sprintf("value of %d bytes does not fit a field of %d", e.Len, e.Width)
}

// UnrepresentableRuneError is returned when a character has no byte in the
// declared charset. It has no read-side counterpart: decoding is total, because
// any byte may appear in a PIC X field.
type UnrepresentableRuneError struct {
	// Rune is the character that could not be encoded.
	Rune rune
	// Charset is the name of the charset that has no byte for it.
	Charset string
}

// Error implements the [error] interface.
func (e UnrepresentableRuneError) Error() string {
	return fmt.Sprintf("character %q has no representation in %s", e.Rune, e.Charset)
}

// JustificationError is returned when a [Justification] names no member.
type JustificationError struct {
	// Justification is the value that was passed.
	Justification Justification
}

// Error implements the [error] interface.
func (e JustificationError) Error() string {
	return fmt.Sprintf("invalid justification %d", int(e.Justification))
}

// SignednessError is returned when a [Signedness] names no member, which
// includes the zero value: a writer of a numeric item will not guess whether
// the item's PICTURE carries S.
type SignednessError struct {
	// Signedness is the value that was passed.
	Signedness Signedness
}

// Error implements the [error] interface.
func (e SignednessError) Error() string {
	return fmt.Sprintf("invalid signedness %d: an item is either Signed or Unsigned", int(e.Signedness))
}

// SignPositionError is returned when a [SignPosition] names no position, which
// includes the zero value: a zoned decimal accessor will not guess whether the
// item's PICTURE carries S, nor where its SIGN clause put it.
//
// It is [SignednessError] for USAGE DISPLAY, and the guess it refuses to make
// is wider: a sign position is also what says whether the field is digits bytes
// wide or digits+1.
type SignPositionError struct {
	// SignPosition is the value that was passed.
	SignPosition SignPosition
}

// Error implements the [error] interface.
func (e SignPositionError) Error() string {
	return fmt.Sprintf(
		"invalid sign position %d: an item is SignUnsigned, SignTrailing, SignLeading, SignTrailingSeparate or SignLeadingSeparate",
		int(e.SignPosition),
	)
}

// The three sentinels below guard internal contracts of the zoned helpers, and
// none of them can reach a caller: a caller states a digit count and a sign
// position, and it is this package that turns those into a digit slice, a field
// width and an index. They are unexported and untyped for that reason — a
// caller has nothing to assert on and no way to provoke one. The typed leaves a
// caller does assert on are the three Zoned…Error types below, which describe
// bytes that came out of a file.
var (
	// errZonedDigitValue guards the digit values handed to the zoned
	// encoders being 0-9. They come from a decimal formatting of the number
	// being written.
	errZonedDigitValue = errors.New("invalid zoned decimal digit value: digits are 0-9")
	// errZonedFieldWidth guards a field's byte slice being exactly as wide
	// as its digit slice: a zoned item is one byte per digit.
	errZonedFieldWidth = errors.New("zoned decimal field width does not match its digit count")
	// errZonedSignPosition guards the overpunch index being an index into
	// the field, or -1 for a field that carries no overpunched sign.
	errZonedSignPosition = errors.New("invalid zoned decimal sign position: not an index into the field")
)

// ZonedDigitError is returned when a byte in a plain digit position of a zoned
// decimal (USAGE DISPLAY) field is not a digit in the declared charset.
//
// It is the loudest signal the package has that [Encoding.Charset] is wrong: an
// EBCDIC F5 is not an ASCII digit and an ASCII 35 is not an EBCDIC one, so the
// first zoned field of the first record catches a swapped charset. The byte is
// rejected rather than coerced to its low nibble, which is what would turn that
// failure into a plausible wrong number.
type ZonedDigitError struct {
	// Byte is the offending byte.
	Byte byte
	// Charset is the name of the charset it is not a digit in.
	Charset string
	// Zero and Nine are the bytes that charset does spell 0 and 9 with,
	// which are the ends of a contiguous range in every charset in use.
	Zero, Nine byte
}

// Error implements the [error] interface.
func (e ZonedDigitError) Error() string {
	return fmt.Sprintf("invalid zoned decimal digit byte %#02X: %s digits are %#02X-%#02X", e.Byte, e.Charset, e.Zero, e.Nine)
}

// ZonedSignError is returned when the sign-carrying byte of a signed zoned
// decimal field names no digit under the declared [SignConvention].
//
// The four conventions are mutually detectable at exactly this byte — each
// one's negative bytes are invalid under the other three — so this is what
// makes a wrong convention loud rather than silently wrong signs. A sign byte
// of 7B read under [SignASCIIZone37] is this error and not a digit.
//
// A field that has only ever held non-negative values will not produce it,
// since the three ASCII conventions agree on 30-39: a reader that has seen no
// negative has not confirmed the convention, it has merely not been
// contradicted.
type ZonedSignError struct {
	// Byte is the offending byte.
	Byte byte
	// Sign is the convention it is invalid under.
	Sign SignConvention
}

// Error implements the [error] interface.
func (e ZonedSignError) Error() string {
	return fmt.Sprintf("invalid zoned decimal sign byte %#02X under sign convention %s", e.Byte, e.Sign)
}

// ZonedSeparateSignError is returned when the byte of a SIGN IS SEPARATE
// CHARACTER field is neither the charset's '+' nor its '-'.
//
// The separate sign byte is charset-sensitive and convention-independent — 2B
// and 2D in ASCII, 4E and 60 in EBCDIC — so this rejects a wrong charset and a
// slipped field offset, the only two things it can be.
type ZonedSeparateSignError struct {
	// Byte is the offending byte.
	Byte byte
	// Charset is the name of the charset whose signs it is not one of.
	Charset string
	// Plus and Minus are the bytes that charset spells '+' and '-' with.
	Plus, Minus byte
}

// Error implements the [error] interface.
func (e ZonedSeparateSignError) Error() string {
	return fmt.Sprintf("invalid separate sign byte %#02X: %s spells + as %#02X and - as %#02X", e.Byte, e.Charset, e.Plus, e.Minus)
}

// ZonedDigitCountError is returned when a zoned decimal (USAGE DISPLAY) digit
// count is outside the range the accessor accepts.
//
// The upper bound belongs to the accessor rather than to the field, exactly as
// it does for [PackedDigitCountError]: 9 digits for the int32 accessors, 18 for
// the int64 ones and 31 — the IBM Enterprise COBOL maximum — for the
// [math/big.Int] ones. A field wider than the accessor asked for is a call that
// would have silently overflowed its return type, so it is refused before a
// byte is read.
type ZonedDigitCountError struct {
	// Digits is the digit count that was asked for.
	Digits int
	// Max is the largest digit count this accessor accepts.
	Max int
}

// Error implements the [error] interface.
func (e ZonedDigitCountError) Error() string {
	return fmt.Sprintf("invalid zoned decimal digit count %d: must be between 1 and %d", e.Digits, e.Max)
}

// ZonedRangeError is returned when a value cannot be written into the zoned
// decimal field it was given: it has more digits than the field holds, or it is
// negative and the field is [SignUnsigned].
//
// It is [PackedRangeError] for USAGE DISPLAY and is loud for the same reason. A
// COBOL MOVE truncates high-order digits and stores a negative value into an
// unsigned item as its absolute value; a codec doing either would write a
// record that no longer says what the caller asked it to.
type ZonedRangeError struct {
	// Value is the decimal spelling of the value that did not fit.
	Value string
	// Digits is the number of digits the field holds.
	Digits int
	// Sign is where the field keeps its sign, which is also what says
	// whether it has one at all.
	Sign SignPosition
}

// Error implements the [error] interface.
func (e ZonedRangeError) Error() string {
	return fmt.Sprintf("value %s does not fit a %d-digit %s zoned decimal field", e.Value, e.Digits, e.Sign)
}

// PackedDigitCountError is returned when a packed decimal digit count is
// outside the range the accessor accepts.
//
// The upper bound belongs to the accessor rather than to the field: 9 digits
// for the int32 accessors, 18 for the int64 ones and 31 — the IBM Enterprise
// COBOL maximum — for the [math/big.Int] ones. A field wider than the accessor
// asked for is a call that would have silently overflowed.
type PackedDigitCountError struct {
	// Digits is the digit count that was asked for.
	Digits int
	// Max is the largest digit count this accessor accepts.
	Max int
}

// Error implements the [error] interface.
func (e PackedDigitCountError) Error() string {
	return fmt.Sprintf("invalid packed decimal digit count %d: must be between 1 and %d", e.Digits, e.Max)
}

// PackedPadError is returned when the pad nibble of a packed decimal field is
// not zero.
//
// The pad nibble is the high nibble of the first byte, and it exists only when
// the digit count is even. A non-zero pad is the cheapest available signal
// that the field offset is wrong — that the copybook being read does not
// describe the file in hand — which is why it is validated rather than
// skipped.
type PackedPadError struct {
	// Nibble is the pad nibble that was found, which is between 0x1 and 0xF.
	Nibble byte
}

// Error implements the [error] interface.
func (e PackedPadError) Error() string {
	return fmt.Sprintf("packed decimal pad nibble is %X, not 0", e.Nibble)
}

// PackedDigitError is returned when a digit nibble of a packed decimal field
// holds a value above 9, which no writer produces and which denotes no digit.
type PackedDigitError struct {
	// Nibble is the offending nibble, which is between 0xA and 0xF.
	Nibble byte
}

// Error implements the [error] interface.
func (e PackedDigitError) Error() string {
	return fmt.Sprintf("invalid packed decimal digit nibble %X: digit nibbles are 0-9", e.Nibble)
}

// PackedSignError is returned when the sign nibble of a packed decimal field
// is one of 0-9, none of which names a sign.
//
// A, B, C, D, E and F all name one and are all accepted on read, so this
// rejects exactly the nibbles that cannot have come from a packed field.
type PackedSignError struct {
	// Nibble is the offending nibble, which is between 0x0 and 0x9.
	Nibble byte
}

// Error implements the [error] interface.
func (e PackedSignError) Error() string {
	return fmt.Sprintf("invalid packed decimal sign nibble %X: sign nibbles are A-F", e.Nibble)
}

// PackedRangeError is returned when a value cannot be written into the packed
// decimal field it was given: it has more digits than the field holds, or it is
// negative and the field is [Unsigned].
//
// Both are loud for the reason [FieldTooLongError] is. A COBOL MOVE truncates
// high-order digits and stores a negative value into an unsigned item as its
// absolute value; a codec doing either would write a record that no longer says
// what the caller asked it to.
type PackedRangeError struct {
	// Value is the decimal spelling of the value that did not fit.
	Value string
	// Digits is the number of digits the field holds.
	Digits int
	// Signedness is whether the field carries a sign.
	Signedness Signedness
}

// Error implements the [error] interface.
func (e PackedRangeError) Error() string {
	return fmt.Sprintf("value %s does not fit a %d-digit %s packed decimal field", e.Value, e.Digits, e.Signedness)
}

// BinaryDigitCountError is returned when a binary decimal digit count is
// outside the range the accessor accepts.
//
// The upper bound belongs to the accessor rather than to the field, exactly as
// it does for [PackedDigitCountError]: 4 digits for the int16 accessors, 9 for
// the int32 ones, 18 for the int64 and uint64 ones and 31 — the IBM Enterprise
// COBOL maximum under ARITH(EXTEND) — for the [math/big.Int] ones. The bound is
// what the accessor's return type always holds over the *full* storage width,
// so that a COMP-5 field cannot silently overflow it.
type BinaryDigitCountError struct {
	// Digits is the digit count that was asked for.
	Digits int
	// Max is the largest digit count this accessor accepts.
	Max int
}

// Error implements the [error] interface.
func (e BinaryDigitCountError) Error() string {
	return fmt.Sprintf("invalid binary digit count %d: must be between 1 and %d", e.Digits, e.Max)
}

// BinaryRangeError is returned when a binary value lies outside the range of
// the field it was given.
//
// It arises in both directions, which is not true of [PackedRangeError]:
//
//   - On write, the value does not fit — it has more digits than the PICTURE
//     allows under [TruncStd], it exceeds the storage width under [TruncBin],
//     or it is negative and the field is [Unsigned].
//   - On read under [TruncStd], the *stored* value exceeds the decimal range
//     the PICTURE can express. That is a real signal rather than pedantry: it
//     is the strongest available evidence that [Encoding.ByteOrder] is wrong,
//     since a byte-swapped small number is usually a very large one. A file
//     that legitimately carries such values is a COMP-5 or TRUNC(BIN) file and
//     is read with the [Reader.ReadComp5Int16] family instead.
//
// The offset it is wrapped in is the offset the field *starts* at rather than
// the one it ends at, for the reason a packed nibble error carries the byte
// holding the nibble: a binary field is several bytes wide, and a range error
// is a statement about the whole field.
type BinaryRangeError struct {
	// Value is the decimal spelling of the value that did not fit.
	Value string
	// Digits is the number of digits the field's PICTURE declares.
	Digits int
	// Width is the storage width of the field, in bytes.
	Width int
	// Signedness is whether the field carries a sign.
	Signedness Signedness
	// Truncation is the range semantics the accessor applied.
	Truncation Truncation
}

// Error implements the [error] interface.
func (e BinaryRangeError) Error() string {
	return fmt.Sprintf(
		"value %s does not fit a %d-digit %s binary field of %d bytes under %s",
		e.Value, e.Digits, e.Signedness, e.Width, e.Truncation,
	)
}

// FloatRangeError is returned when a floating point value and the field it was
// given have no number in common. Like [BinaryRangeError] it arises in both
// directions, and every one of its cases belongs to [FloatHFP]:
//
//   - Writing a NaN or an infinity. HFP has no encoding for either — every one
//     of its bit patterns denotes an ordinary number — so there is nothing to
//     store that would not read back as a plausible finite value.
//   - Writing a magnitude outside HFP's range, which is 16^-65 to 16^63.
//   - Reading a COMP-1 field into a float32. HFP's exponent range is far wider
//     than binary32's, so a field may legitimately hold a value that overflows
//     a float32 to infinity or underflows it to zero. Both are reported rather
//     than returned, because HFP produces neither an infinity nor a spurious
//     zero of its own and a caller has no way to tell such a reading from a
//     real one.
//
// [FloatIEEE] never produces one. binary32 and binary64 hold every float32 and
// float64 there is, NaNs and infinities included, so nothing in either
// direction is out of range.
type FloatRangeError struct {
	// Value is the decimal spelling of the value that did not fit.
	Value string
	// Format is the floating point format of the field.
	Format FloatFormat
	// Width is the storage width of the field, in bytes: 4 for COMP-1 and 8
	// for COMP-2.
	Width int
	// Reason says which of the cases above applies.
	Reason string
}

// Error implements the [error] interface.
func (e FloatRangeError) Error() string {
	return fmt.Sprintf(
		"%s floating point value %s in a field of %d bytes: %s",
		e.Format, e.Value, e.Width, e.Reason,
	)
}

// comparableCharset reports whether cs may be used as a map key, which
// [alphaTableOf] must know before it goes anywhere near [alphaTables]: a
// non-comparable key panics inside the map, and inside sync.Map's store path
// that panic escapes while its mutex is held. Recovering from it is therefore
// not an option, so the question has to be answered first.
//
// It is answered from the *type* and answered conservatively. The exact answer
// is reflect.Value.Comparable, which walks the dynamic value and so tells a
// struct{ X any } holding an int from one holding a slice. comparableType
// instead reads any interface-typed field as not comparable, which is wrong only
// in the direction that costs a charset its cached table and can never be wrong
// in the direction that panics.
//
// The choice is about cost, and the cost is per [Reader] and so per record on
// the [Unmarshal] path. The two cases answered inline below are not
// micro-optimisation — between them they are every charset a real program has,
// and they answer in a Kind and a field count where the walk would recurse: 2 ns
// against 5.5 for an empty struct, and against 23.6 for one embedding an
// interface. Memoising the walk per reflect.Type was tried and made no
// measurable difference to BenchmarkUnmarshalRecord, a sync.Map load costing
// about what the walk it replaces does, so there is no such map to keep
// coherent.
func comparableCharset(cs Charset) bool {
	ty := reflect.TypeOf(cs)
	if ty == nil {
		return false
	}
	switch ty.Kind() {
	case reflect.Pointer, reflect.Chan, reflect.UnsafePointer:
		// Nothing to walk: comparing any of these compares the reference
		// and never what it refers to, so it cannot panic. This is the
		// shape a charset carrying state almost always has.
		return true
	case reflect.Struct:
		if ty.NumField() == 0 {
			// Both shipped charsets, and the cheapest answer available.
			return true
		}
	}
	return comparableType(ty)
}

// comparableType is comparableCharset's recursion: reflect.Type.Comparable
// with interface-typed fields ruled out.
//
// reflect.Type.Comparable answers for the type, and an interface type is
// comparable as a type — it is the dynamic value inside it that may not be, so
// a struct carrying one is a value that == accepts or panics on depending on
// what was put in it. Every other kind either is comparable or is not,
// whatever it holds.
func comparableType(t reflect.Type) bool {
	if t == nil {
		return false
	}
	switch t.Kind() {
	case reflect.Interface:
		return false
	case reflect.Array:
		return comparableType(t.Elem())
	case reflect.Struct:
		for i := range t.NumField() {
			if !comparableType(t.Field(i).Type) {
				return false
			}
		}
		return true
	}
	return t.Comparable()
}
