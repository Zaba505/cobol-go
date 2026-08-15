// Copyright (c) 2026 Richard Carson Derr
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package cobol

import "iter"

// listingDirectiveWords are the listing-control statements that stand alone in a
// line of their own: EJECT begins a new page of the compiler's source listing,
// and SKIP1 / SKIP2 / SKIP3 leave one, two or three blank lines in it. TITLE is
// deliberately absent — it is the one form carrying an operand, so
// skipListingDirective recognizes it separately.
var listingDirectiveWords = []string{"EJECT", "SKIP1", "SKIP2", "SKIP3"}

// skipListingDirectives returns src with every listing-control statement — EJECT,
// SKIP1, SKIP2, SKIP3 and TITLE <literal> — removed, as a pass over the token
// stream between [Tokenize] and the parser.
//
// These are compiler-directing statements that affect only the compiler's source
// listing and have no effect on the compilation of the source text (SPEC
// "Listing-Control Statements"), so the package discards them rather than
// modelling them: this printer emits canonical source, not a listing, and
// already drops the page-eject sense of the one spelling the tokenizer did
// handle — the fixed-format column-7 `/` survives as its comment text alone.
// Removing them here rather than in the grammar is what keeps every division,
// section and entry parser free of them, exactly as expandCopy keeps them free
// of COPY.
//
// The pass wraps each tokenizer stream separately — the source unit's, and each
// copybook's inside [expandCopy] — because recognition is line-relative and
// copied tokens are positioned within their own copybook. One filter spanning
// the seam would read a copybook's line 1 as a continuation of the copying
// source's line 1.
func skipListingDirectives(src iter.Seq2[Token, error]) iter.Seq2[Token, error] {
	return func(yield func(Token, error) bool) {
		pull, stop := iter.Pull2(src)
		defer stop()

		d := &directiveFilter{pull: pull}
		for {
			tok, err, ok := d.next()
			if err != nil {
				yield(Token{}, err)
				return
			}
			if !ok {
				return // stream exhausted
			}
			if d.skipListingDirective(tok) {
				continue
			}
			d.lastLine = tok.Pos.Line
			if !yield(tok, nil) {
				return
			}
		}
	}
}

// directiveFilter is the state of one skipListingDirectives pass over one
// tokenizer stream.
type directiveFilter struct {
	// pull pulls the next (Token, error) pair from the stream being filtered; the
	// final bool reports whether a value was produced (false once exhausted).
	pull func() (Token, error, bool)
	// unread holds the one triple skipListingDirective read past the statement it
	// was scanning and must hand back before pulling again.
	unread *pulledToken
	// lastLine is the source line of the token most recently emitted, or 0 before
	// the first. It is what applies the "only statement on the line" rule: a
	// directive word is one only when it opens its line.
	lastLine int
}

// pulledToken is a single (Token, error, ok) triple held for pushback.
type pulledToken struct {
	tok Token
	err error
	ok  bool
}

// next returns the next triple of the stream, handing back the one triple
// skipListingDirective read past the statement it was scanning before pulling
// again. Pushing an unwanted triple back rather than acting on it is what keeps
// the lookahead free of its own error handling: a pull error or an exhausted
// stream met mid-scan is simply returned here on the next turn of the loop and
// handled by the one path that already handles them.
func (d *directiveFilter) next() (Token, error, bool) {
	if d.unread != nil {
		u := d.unread
		d.unread = nil
		return u.tok, u.err, u.ok
	}
	return d.pull()
}

// skipListingDirective reports whether word begins a listing-control statement
// and, when it does, consumes the rest of that statement so nothing of it is
// emitted.
//
// Recognition rests on the rule that makes these statements unambiguous: each
// must be the only statement on its line (*IBM Enterprise COBOL for z/OS*, SKIP
// statements). So word counts only when it opens a line — nothing was emitted
// from that line before it — and only tokens on word's own line can belong to
// it: TITLE's literal operand, and the optional separator period. A period on a
// later line belongs to a following construct and is left alone.
func (d *directiveFilter) skipListingDirective(word Token) bool {
	if word.Pos.Line == d.lastLine {
		return false
	}
	switch {
	case keywordIs(word, listingDirectiveWords...):
	case keywordIs(word, "TITLE"):
		// A TITLE statement always carries a literal operand, so a bare TITLE is
		// an ordinary word — a data name or a paragraph name — and is left to the
		// grammar.
		tok, err, ok := d.next()
		if err != nil || !ok || tok.Type != TokenString || tok.Pos.Line != word.Pos.Line {
			d.unread = &pulledToken{tok: tok, err: err, ok: ok}
			return false
		}
	default:
		return false
	}
	if tok, err, ok := d.next(); err != nil || !ok || !isPeriod(tok) || tok.Pos.Line != word.Pos.Line {
		d.unread = &pulledToken{tok: tok, err: err, ok: ok}
	}
	return true
}
