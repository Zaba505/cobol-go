// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import (
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CopyBookResolver resolves the text-name of a COPY statement to the library
// text — the copybook — that it names.
//
// name is the text-name as written in the source, with the delimiters of a
// literal text-name already removed; library is the library-name from an OF or
// IN phrase, or "" when the statement gave none. Whether the source wrote the
// name as a COBOL word or as an alphanumeric literal is *not* passed on — the
// standard makes the first case-insensitive and the second exact, but a resolver
// that reads a case-sensitive filesystem cannot honor both from one string, and
// splitting the interface to say which was written would push that dilemma onto
// every implementation. So the interface takes no position and the two resolvers
// this package ships match case-insensitively throughout, which is what makes
// COPY custrec find a mainframe library's CUSTREC. A resolver that must
// distinguish the two can be written as a [CopyBookFunc] over any rule it
// likes.
//
// The returned bytes are the copybook's source text in the same reference format
// as the source doing the copying — [FSCopyBooks] and [MapCopyBooks] both return
// it unchanged. A resolver reports a copybook it does not have by returning an
// error wrapping [fs.ErrNotExist]; [Parse] wraps whatever comes back in a
// [CopyBookNotFoundError] that carries the position of the COPY statement.
type CopyBookResolver interface {
	ResolveCopyBook(name, library string) ([]byte, error)
}

// CopyBookFunc adapts an ordinary function to the [CopyBookResolver] interface.
type CopyBookFunc func(name, library string) ([]byte, error)

// ResolveCopyBook implements [CopyBookResolver].
func (f CopyBookFunc) ResolveCopyBook(name, library string) ([]byte, error) {
	return f(name, library)
}

// defaultCopyBookSuffixes are the filename extensions [FSCopyBooks] tries when a
// caller names none: the text-name exactly as written first, then the .cpy
// extension copybooks conventionally carry, in both cases.
var defaultCopyBookSuffixes = []string{"", ".cpy", ".CPY"}

// FSCopyBooks returns a [CopyBookResolver] that reads copybooks from fsys. It
// covers the filesystem (os.DirFS), an embedded library (embed.FS), and anything
// else that implements [fs.FS] with one implementation, since that is the only
// thing the three have to have in common.
//
// A text-name is resolved to a file by trying each suffix in turn — the
// text-name as written, then ".cpy", then ".CPY" when the caller names none —
// against the text-name as written, upper-cased and lower-cased, in that order.
// Trying all three cases is what lets a source that says COPY custrec find
// CUSTREC.cpy in a library lifted off a mainframe, where COBOL's
// case-insensitivity did not survive the transfer to a case-sensitive
// filesystem. It applies to a literal text-name too — the spelling as written is
// tried first, so an exact match always wins, but a literal that matches nothing
// exactly still falls back to the other casings.
//
// A library-name becomes a directory: COPY CUSTREC OF PAYLIB reads
// PAYLIB/CUSTREC.cpy, again trying each casing of the directory name.
func FSCopyBooks(fsys fs.FS, suffixes ...string) CopyBookResolver {
	if len(suffixes) == 0 {
		suffixes = defaultCopyBookSuffixes
	}
	return CopyBookFunc(func(name, library string) ([]byte, error) {
		// A candidate that is simply absent is not the answer, so the search
		// carries on; a candidate that exists but cannot be read is, and it is
		// reported rather than reduced to "not found" by the next candidate.
		var readErr error
		for _, dir := range caseVariants(library) {
			for _, base := range caseVariants(name) {
				for _, suffix := range suffixes {
					p := base + suffix
					if dir != "" {
						p = path.Join(dir, p)
					}
					if !fs.ValidPath(p) {
						continue
					}
					switch b, err := fs.ReadFile(fsys, p); {
					case err == nil:
						return b, nil
					case !errors.Is(err, fs.ErrNotExist) && readErr == nil:
						readErr = err
					}
				}
			}
		}
		if readErr != nil {
			return nil, fmt.Errorf("copybook %q: %w", copyBookKey(name, library), readErr)
		}
		return nil, fmt.Errorf("copybook %q: %w", copyBookKey(name, library), fs.ErrNotExist)
	})
}

// MapCopyBooks returns a [CopyBookResolver] serving copybooks held in memory.
// Keys are text-names, matched without regard to case; a copybook belonging to a
// named library is keyed "LIBRARY.TEXT-NAME". A library-qualified COPY looks for
// the qualified key first and falls back to the bare text-name, so a library that
// has only one copy of each copybook does not have to spell the library out.
//
// The map is copied, so a later change to it does not change what the resolver
// serves.
func MapCopyBooks(books map[string]string) CopyBookResolver {
	index := make(map[string]string, len(books))
	for k, v := range books {
		index[strings.ToUpper(k)] = v
	}
	return CopyBookFunc(func(name, library string) ([]byte, error) {
		if library != "" {
			if text, ok := index[copyBookKey(name, library)]; ok {
				return []byte(text), nil
			}
		}
		if text, ok := index[strings.ToUpper(name)]; ok {
			return []byte(text), nil
		}
		return nil, fmt.Errorf("copybook %q: %w", copyBookKey(name, library), fs.ErrNotExist)
	})
}

// caseVariants returns the spellings of a COBOL word a case-sensitive lookup
// should try: as written, upper-cased and lower-cased, with duplicates dropped so
// an already upper-case name costs one lookup rather than three. An empty name
// yields a single empty spelling, so a caller can loop over it unconditionally.
func caseVariants(s string) []string {
	if s == "" {
		return []string{""}
	}
	variants := []string{s}
	for _, v := range []string{strings.ToUpper(s), strings.ToLower(s)} {
		if !slices.Contains(variants, v) {
			variants = append(variants, v)
		}
	}
	return variants
}

// copyBookKey is the upper-cased, library-qualified identity of a copybook. It
// keys [MapCopyBooks] and, more importantly, the stack that detects a COPY cycle:
// COBOL words are case-insensitive, so COPY custrec and COPY CUSTREC name the
// same copybook and must count as the same node of the cycle.
func copyBookKey(name, library string) string {
	if library == "" {
		return strings.ToUpper(name)
	}
	return strings.ToUpper(library) + "." + strings.ToUpper(name)
}

// CopyBookNotFoundError is returned when a [CopyBookResolver] cannot supply the
// copybook a COPY statement names. Pos is the position of the COPY keyword; Err
// is the resolver's own error, which wraps [fs.ErrNotExist] for the resolvers
// this package provides.
type CopyBookNotFoundError struct {
	Pos     Pos
	Name    string
	Library string
	Err     error
}

// Error implements the [error] interface.
func (e CopyBookNotFoundError) Error() string {
	return fmt.Sprintf("cannot resolve copybook %q at line %d, column %d: %v",
		copyBookKey(e.Name, e.Library), e.Pos.Line, e.Pos.Column, e.Err)
}

// Unwrap returns the resolver's error, so a caller can test for [fs.ErrNotExist]
// with errors.Is.
func (e CopyBookNotFoundError) Unwrap() error { return e.Err }

// CopyBookCycleError is returned when a COPY statement names a copybook that is
// already being copied — directly, or through any number of intervening
// copybooks. Expanding it would not terminate. Pos is the position of the
// offending COPY keyword and Stack is the chain of copybooks currently open,
// outermost first, with the repeated one appended.
type CopyBookCycleError struct {
	Pos     Pos
	Name    string
	Library string
	Stack   []string
}

// Error implements the [error] interface.
func (e CopyBookCycleError) Error() string {
	return fmt.Sprintf("copybook cycle at line %d, column %d: %s",
		e.Pos.Line, e.Pos.Column, strings.Join(e.Stack, " -> "))
}

// MissingCopyBookResolverError is returned when a source contains a COPY
// statement but [Parse] was given no [CopyBookResolver] to expand it with. Pos is
// the position of the COPY keyword.
//
// COPY is a text-manipulation statement: there is no AST node for it, because by
// the time the parser runs the statement has been replaced by the text it names.
// A parse with no resolver therefore cannot carry the statement forward in any
// form, and saying so is better than the unexpected-token error the bare word
// COPY would otherwise produce somewhere inside a data description entry.
type MissingCopyBookResolverError struct {
	Pos     Pos
	Name    string
	Library string
}

// Error implements the [error] interface.
func (e MissingCopyBookResolverError) Error() string {
	return fmt.Sprintf("COPY %s at line %d, column %d needs a copybook resolver; pass WithCopyBooks to Parse",
		copyBookKey(e.Name, e.Library), e.Pos.Line, e.Pos.Column)
}

// copyStatement is a parsed COPY statement. It is not an AST node: COPY is
// replaced by the library text it names before the parser ever runs, so nothing
// of it survives into the [File]. Pos is the position of the COPY keyword, kept
// for the errors reported while resolving and expanding it.
type copyStatement struct {
	Pos       Pos
	Name      string
	Library   string
	Replacing []copyReplacement
}

// copyReplacement is one "From BY To" pair of a REPLACING phrase. Both sides are
// held as text — the content of a pseudo-text operand, or the lexeme of a
// single-token one — because the replacement is applied to the copybook's text
// before that text is tokenized.
type copyReplacement struct {
	From string
	To   string
}

// copyParser reads a COPY statement off the token stream that the expander is
// walking. It gives the statement's action loop the same shape the AST parser's
// constructs have — one named action per state — over the pull function the
// expander already holds.
//
// Comments met inside the statement are set aside in comments rather than
// dropped; the expander re-emits them ahead of the copied text, so a comment
// written between COPY and its terminating period survives into the AST.
type copyParser struct {
	pull     func() (Token, error, bool)
	peeked   *Token
	comments []Token
}

// next returns the next non-comment token, buffering any comment it passes.
// expected names the token types the caller would have accepted, so a stream
// that ends mid-statement reports what that state was waiting for rather than
// one fixed guess.
func (c *copyParser) next(expected ...TokenType) (Token, error) {
	if c.peeked != nil {
		tok := *c.peeked
		c.peeked = nil
		return tok, nil
	}
	for {
		tok, err, ok := c.pull()
		if err != nil {
			return Token{}, err
		}
		if !ok {
			return Token{}, UnexpectedEndOfTokensError{Expected: expected}
		}
		if tok.Type == TokenComment {
			c.comments = append(c.comments, tok)
			continue
		}
		return tok, nil
	}
}

// peek returns the next non-comment token without consuming it, reporting
// [UnexpectedEndOfTokensError] over expected when the stream is exhausted. A
// COPY statement always has more to read — at the very least its terminating
// period — so there is no "peeked past the end" case to report separately.
func (c *copyParser) peek(expected ...TokenType) (Token, error) {
	if c.peeked != nil {
		return *c.peeked, nil
	}
	tok, err := c.next(expected...)
	if err != nil {
		return Token{}, err
	}
	c.peeked = &tok
	return tok, nil
}

// expect consumes the next token, requiring its type to be one of types. It is
// the copy statement's counterpart of [parser.expect]: every place the COPY
// grammar requires a particular token goes through it rather than inlining the
// type check, so the expectation reported on failure is the one that state
// actually had.
func (c *copyParser) expect(types ...TokenType) (Token, error) {
	tok, err := c.next(types...)
	if err != nil {
		return Token{}, err
	}
	if !slices.Contains(types, tok.Type) {
		return Token{}, UnexpectedTokenError{Expected: types, Actual: tok}
	}
	return tok, nil
}

// expectKeyword consumes the next token, requiring an identifier whose spelling
// is one of kw, compared without regard to case as COBOL reserved words are.
func (c *copyParser) expectKeyword(kw ...string) (Token, error) {
	tok, err := c.expect(TokenIdentifier)
	if err != nil {
		return Token{}, err
	}
	if !keywordIs(tok, kw...) {
		return Token{}, UnexpectedKeywordError{Expected: kw, Actual: tok}
	}
	return tok, nil
}

// expectPeriod consumes the separator period terminating the COPY statement.
func (c *copyParser) expectPeriod() (Token, error) {
	tok, err := c.expect(TokenSymbol)
	if err != nil {
		return Token{}, err
	}
	if !isPeriod(tok) {
		return Token{}, UnexpectedTokenError{Expected: []TokenType{TokenSymbol}, Actual: tok}
	}
	return tok, nil
}

// copyNameTokens are the token types a text-name or a library-name may be: a
// COBOL word, or an alphanumeric literal for a name a COBOL word cannot spell.
var copyNameTokens = []TokenType{TokenIdentifier, TokenString}

// copyOperandTokens are the token types a REPLACING operand may be.
var copyOperandTokens = []TokenType{TokenPseudoText, TokenIdentifier, TokenString, TokenNumber}

// copyAction is one state of the COPY statement's action loop: it reads some
// tokens, records what it read on st, and returns the next state, or nil to
// finish (nil with an error to fail).
type copyAction func(c *copyParser, st *copyStatement) (copyAction, error)

// parseCopyStatement runs the action loop for a COPY statement whose COPY
// keyword was already consumed at pos:
//
//	COPY text-name [ (OF | IN) library-name ] [ REPLACING { operand BY operand }… ] .
//
// (SPEC.md §"COPY and Pseudo-Text".)
func parseCopyStatement(c *copyParser, pos Pos) (*copyStatement, error) {
	st := &copyStatement{Pos: pos}
	var err error
	for action := parseCopyName; action != nil && err == nil; {
		action, err = action(c, st)
	}
	if err != nil {
		return nil, err
	}
	return st, nil
}

// parseCopyName reads the text-name naming the copybook: a COBOL word, or an
// alphanumeric literal for a name a COBOL word cannot spell (a lower-case or
// dotted filename, most often).
func parseCopyName(c *copyParser, st *copyStatement) (copyAction, error) {
	tok, err := c.expect(copyNameTokens...)
	if err != nil {
		return nil, err
	}
	st.Name = copyTextName(tok)
	return parseCopyLibrary, nil
}

// parseCopyLibrary reads the optional OF/IN library-name phrase. What may stand
// here instead is REPLACING or the terminating period, so an exhausted stream
// reports both a word and a symbol as acceptable.
func parseCopyLibrary(c *copyParser, st *copyStatement) (copyAction, error) {
	tok, err := c.peek(TokenIdentifier, TokenSymbol)
	if err != nil {
		return nil, err
	}
	if !keywordIs(tok, "OF", "IN") {
		return parseCopyReplacing, nil
	}
	if _, err := c.expectKeyword("OF", "IN"); err != nil {
		return nil, err
	}
	name, err := c.expect(copyNameTokens...)
	if err != nil {
		return nil, err
	}
	st.Library = copyTextName(name)
	return parseCopyReplacing, nil
}

// parseCopyReplacing reads the optional REPLACING keyword, dispatching to the
// operand pairs when it is there and to the terminating period when it is not.
func parseCopyReplacing(c *copyParser, st *copyStatement) (copyAction, error) {
	tok, err := c.peek(TokenIdentifier, TokenSymbol)
	if err != nil {
		return nil, err
	}
	if !keywordIs(tok, "REPLACING") {
		return parseCopyEnd, nil
	}
	if _, err := c.expectKeyword("REPLACING"); err != nil {
		return nil, err
	}
	return parseCopyReplacementFrom, nil
}

// parseCopyReplacementFrom reads the operand a replacement matches on.
func parseCopyReplacementFrom(c *copyParser, st *copyStatement) (copyAction, error) {
	tok, err := c.expect(copyOperandTokens...)
	if err != nil {
		return nil, err
	}
	st.Replacing = append(st.Replacing, copyReplacement{From: copyOperandText(tok)})
	return parseCopyReplacementBy, nil
}

// parseCopyReplacementBy reads the BY separating a replacement's two operands.
func parseCopyReplacementBy(c *copyParser, st *copyStatement) (copyAction, error) {
	if _, err := c.expectKeyword("BY"); err != nil {
		return nil, err
	}
	return parseCopyReplacementTo, nil
}

// parseCopyReplacementTo reads the operand a replacement substitutes in, then
// dispatches: another operand continues the REPLACING phrase, and anything else
// ends it. A REPLACING phrase is a list with no separator, so the only thing that
// ends it is a token that cannot begin an operand — in practice the terminating
// period.
func parseCopyReplacementTo(c *copyParser, st *copyStatement) (copyAction, error) {
	tok, err := c.expect(copyOperandTokens...)
	if err != nil {
		return nil, err
	}
	st.Replacing[len(st.Replacing)-1].To = copyOperandText(tok)

	next, err := c.peek(append(copyOperandTokens, TokenSymbol)...)
	if err != nil {
		return nil, err
	}
	if slices.Contains(copyOperandTokens, next.Type) {
		return parseCopyReplacementFrom, nil
	}
	return parseCopyEnd, nil
}

// parseCopyEnd consumes the separator period terminating the COPY statement. The
// period belongs to the statement and is not passed on: the library text
// logically replaces the whole COPY statement, the terminating period included,
// so a copybook supplies its own sentence and entry terminators.
func parseCopyEnd(c *copyParser, _ *copyStatement) (copyAction, error) {
	if _, err := c.expectPeriod(); err != nil {
		return nil, err
	}
	return nil, nil
}

// copyTextName extracts the name a text-name or library-name token carries: a
// COBOL word as written, or the content of an alphanumeric literal with its
// delimiters removed. The token's type is already one of [copyNameTokens], since
// every caller reaches it through [copyParser.expect].
func copyTextName(tok Token) string {
	if tok.Type == TokenString {
		return trimLiteralDelimiters(string(tok.Value))
	}
	return string(tok.Value)
}

// copyOperandText extracts the text a REPLACING operand contributes. A
// pseudo-text operand contributes its normalized content; a word, literal or
// number contributes its lexeme, delimiters included, since a literal operand
// matches the literal as it is written in the copybook rather than its value.
func copyOperandText(tok Token) string {
	return string(tok.Value)
}

// trimLiteralDelimiters removes the matching quotation marks around an
// alphanumeric literal's lexeme and undoubles any escaped delimiter inside it.
func trimLiteralDelimiters(s string) string {
	if len(s) < 2 {
		return s
	}
	delim := s[0]
	if delim != '"' && delim != '\'' {
		return s
	}
	if s[len(s)-1] != delim {
		return s
	}
	inner := s[1 : len(s)-1]
	return strings.ReplaceAll(inner, string([]byte{delim, delim}), string(delim))
}

// expandCopy returns src with every COPY statement replaced by the tokens of the
// library text it names — the COBOL text-manipulation facility, run as a pass
// over the token stream between [Tokenize] and the parser.
//
// A copybook is expanded in three steps: the resolver supplies its text, the COPY
// statement's REPLACING phrase is applied to that text as a substitution over its
// text words, and only then is the result tokenized. Doing the substitution on
// text rather than on tokens is what makes the pervasive ==:TAG:== idiom work:
// :TAG:-CUSTOMER-ID is four text words, the replacement covers the three that
// spell :TAG:, and the hyphenated remainder is left touching whatever went in,
// producing the single word CUST-CUSTOMER-ID that a token-level substitution
// could not have formed.
//
// The tokens of a copybook are then run through expandCopy again, so a copybook
// may itself COPY. stack carries the copybooks currently open, keyed
// case-insensitively; a text-name already on it is a cycle and is reported as a
// [CopyBookCycleError] rather than expanded forever.
//
// Positions in copied tokens are positions within the copybook — after
// replacement — not within the source that copied it. There is nowhere in a
// [Pos] to record which file a line belongs to, and a position naming the COPY
// statement for every copied token would collapse a whole record layout onto one
// point.
func expandCopy(src iter.Seq2[Token, error], cfg copyConfig, stack []string) iter.Seq2[Token, error] {
	return func(yield func(Token, error) bool) {
		pull, stop := iter.Pull2(src)
		defer stop()

		c := &copyParser{pull: pull}
		for {
			tok, ok, err := nextRawToken(c)
			if err != nil {
				yield(Token{}, err)
				return
			}
			if !ok {
				return // stream exhausted
			}
			if !keywordIs(tok, "COPY") {
				if !yield(tok, nil) {
					return
				}
				continue
			}
			if !expandOne(c, tok.Pos, cfg, stack, yield) {
				return
			}
		}
	}
}

// nextRawToken pulls the next token of the stream being scanned for COPY
// statements, comments included: outside a COPY statement every token is passed
// straight through. The bool reports whether a token was produced, false once the
// stream is cleanly exhausted.
func nextRawToken(c *copyParser) (Token, bool, error) {
	if c.peeked != nil {
		tok := *c.peeked
		c.peeked = nil
		return tok, true, nil
	}
	tok, err, ok := c.pull()
	if err != nil {
		return Token{}, false, err
	}
	return tok, ok, nil
}

// expandOne resolves, replaces and yields one COPY statement whose COPY keyword
// was consumed at pos. It reports false when iteration should stop — because the
// consumer stopped, or because an error was yielded.
func expandOne(c *copyParser, pos Pos, cfg copyConfig, stack []string, yield func(Token, error) bool) bool {
	c.comments = nil
	st, err := parseCopyStatement(c, pos)
	if err != nil {
		yield(Token{}, err)
		return false
	}
	// A comment written inside the COPY statement has no copied text to lead, so
	// it is emitted ahead of the copybook and leads the copybook's first node.
	for _, comment := range c.comments {
		if !yield(comment, nil) {
			return false
		}
	}
	if cfg.resolver == nil {
		yield(Token{}, MissingCopyBookResolverError{Pos: pos, Name: st.Name, Library: st.Library})
		return false
	}

	key := copyBookKey(st.Name, st.Library)
	for _, open := range stack {
		if open == key {
			yield(Token{}, CopyBookCycleError{
				Pos:     pos,
				Name:    st.Name,
				Library: st.Library,
				Stack:   append(append([]string{}, stack...), key),
			})
			return false
		}
	}

	text, err := cfg.resolver.ResolveCopyBook(st.Name, st.Library)
	if err != nil {
		yield(Token{}, CopyBookNotFoundError{
			Pos:     pos,
			Name:    st.Name,
			Library: st.Library,
			Err:     err,
		})
		return false
	}

	// The copybook's tokens get their own listing-directive pass rather than
	// sharing the copying source's: recognition is line-relative, and a copied
	// token's position is a position within the copybook, so one filter spanning
	// the seam would read the copybook's line 1 as a continuation of the line the
	// COPY statement sat on.
	replaced := applyReplacing(string(text), st.Replacing)
	inner := expandCopy(
		skipListingDirectives(Tokenize(strings.NewReader(replaced), cfg.tokenizeOptions...)),
		cfg,
		append(append([]string{}, stack...), key),
	)
	for tok, err := range inner {
		if err != nil {
			yield(Token{}, err)
			return false
		}
		if !yield(tok, nil) {
			return false
		}
	}
	return true
}

// copyConfig is what expandCopy needs beyond the token stream: the resolver that
// supplies library text, and the tokenizer options a copybook is read with. A
// copybook is read in the same reference format as the source that copies it,
// which is what makes a fixed-format program's fixed-format copybooks work with
// no extra configuration.
type copyConfig struct {
	resolver        CopyBookResolver
	tokenizeOptions []TokenizeOption
}

// applyReplacing performs a COPY statement's REPLACING substitution on a
// copybook's text.
//
// Both sides of a replacement are text, but the match is made over *text words*
// — COBOL's lexical unit for text manipulation — so a pattern can never match
// part of a word. The matched words' byte span is what gets replaced, so
// everything around it, spacing included, survives untouched; that is what keeps
// :TAG: and the -CUSTOMER-ID that follows it welded together after :TAG: is
// replaced.
//
// Scanning resumes after the substituted text rather than re-reading it, so a
// replacement whose result contains its own pattern substitutes once, not
// forever. Where several replacements could match at one position the first one
// written wins, and a replacement with an empty pattern is skipped, since it
// could only ever match nothing.
func applyReplacing(src string, replacements []copyReplacement) string {
	if len(replacements) == 0 {
		return src
	}
	patterns := make([][]textWord, len(replacements))
	for i, r := range replacements {
		patterns[i] = textWords(r.From)
	}

	words := textWords(src)
	var b strings.Builder
	copied := 0
	for i := 0; i < len(words); {
		matched := false
		for p, pattern := range patterns {
			if len(pattern) == 0 || !matchTextWords(words, i, pattern) {
				continue
			}
			b.WriteString(src[copied:words[i].start])
			b.WriteString(replacements[p].To)
			copied = words[i+len(pattern)-1].end
			i += len(pattern)
			matched = true
			break
		}
		if !matched {
			i++
		}
	}
	b.WriteString(src[copied:])
	return b.String()
}

// textWord is one COBOL text word within a source text: the byte span it
// occupies and the text it spells.
type textWord struct {
	start int
	end   int
	text  string
}

// textWords splits src into the text words COPY REPLACING matches over. A text
// word is a separator or a character-string, never a space: the separators
// ( ) : , ; . stand alone, an alphanumeric literal is one word delimiters
// included, and every other maximal run of non-space, non-separator characters is
// one word.
//
// A period, comma or semicolon is a separator only when a space or the end of the
// text follows it, which is what keeps the 3 . 14 a naive split would make of a
// numeric literal from existing: 3.14 is one text word, while the period ending
// PIC X(10). is its own.
func textWords(src string) []textWord {
	var words []textWord
	i := 0
	for i < len(src) {
		r, size := utf8.DecodeRuneInString(src[i:])
		switch {
		case unicode.IsSpace(r):
			i += size
		case r == '"' || r == '\'':
			start := i
			i = endOfLiteralWord(src, i, r)
			words = append(words, textWord{start: start, end: i, text: src[start:i]})
		case isSeparatorTextWord(src, i, r):
			words = append(words, textWord{start: i, end: i + size, text: src[i : i+size]})
			i += size
		default:
			start := i
			for i < len(src) {
				r2, size2 := utf8.DecodeRuneInString(src[i:])
				if unicode.IsSpace(r2) || r2 == '"' || r2 == '\'' || isSeparatorTextWord(src, i, r2) {
					break
				}
				i += size2
			}
			words = append(words, textWord{start: start, end: i, text: src[start:i]})
		}
	}
	return words
}

// endOfLiteralWord returns the byte offset just past the alphanumeric literal
// opening at start with delimiter delim, treating a doubled delimiter as an
// escaped one. An unterminated literal runs to the end of the text; the tokenizer
// reports it when the copied text is read, so nothing is gained by rejecting it
// twice.
func endOfLiteralWord(src string, start int, delim rune) int {
	i := start + utf8.RuneLen(delim)
	for i < len(src) {
		r, size := utf8.DecodeRuneInString(src[i:])
		i += size
		if r != delim {
			continue
		}
		if i < len(src) {
			if r2, size2 := utf8.DecodeRuneInString(src[i:]); r2 == delim {
				i += size2
				continue
			}
		}
		return i
	}
	return len(src)
}

// isSeparatorTextWord reports whether the rune r at offset i in src is a
// separator standing as a text word of its own. The parentheses and the colon
// always are; the period, comma and semicolon are only when whitespace or the end
// of the text follows, so the period inside a numeric literal stays part of it.
func isSeparatorTextWord(src string, i int, r rune) bool {
	switch r {
	case '(', ')', ':':
		return true
	case '.', ',', ';':
		next := i + utf8.RuneLen(r)
		if next >= len(src) {
			return true
		}
		r2, _ := utf8.DecodeRuneInString(src[next:])
		return unicode.IsSpace(r2)
	default:
		return false
	}
}

// matchTextWords reports whether pattern matches the words beginning at index i.
// COBOL words compare without regard to case; a literal compares exactly, since
// its delimiters and content are data rather than a spelling.
func matchTextWords(words []textWord, i int, pattern []textWord) bool {
	if i+len(pattern) > len(words) {
		return false
	}
	for k, p := range pattern {
		if !textWordEqual(words[i+k].text, p.text) {
			return false
		}
	}
	return true
}

// textWordEqual reports whether two text words match for the purposes of
// REPLACING.
func textWordEqual(a, b string) bool {
	if isLiteralWord(a) || isLiteralWord(b) {
		return a == b
	}
	return strings.EqualFold(a, b)
}

// isLiteralWord reports whether a text word is an alphanumeric literal.
func isLiteralWord(s string) bool {
	return len(s) > 0 && (s[0] == '"' || s[0] == '\'')
}
