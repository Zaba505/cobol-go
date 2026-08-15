# cobol Package - Claude Memory

This file documents the coding style and patterns for the `cobol` package: a
text file library that reads, parses, and formats COBOL source. It follows a
**tokenizer / parser / printer** pipeline, mirroring `go/scanner` +
`go/parser` + `go/printer` in shape but specialized to one language.

```
source ── Tokenize ─► iter.Seq2[Token, error] ─┬───────┬─► Parse ─► *File (AST) ── Print ─► source
              │                                │       │      │                       │
         tokenizer.go            skipListingDirectives │  parser.go              printer.go
                                     directive.go      │
                                                  expandCopy
                                                    copy.go
```

The whole pipeline is a state machine expressed as **recursive action
functions**. Each component has a slightly different action signature, but they
all behave the same way: an action does some work, then returns the next action
to run (or `nil` to stop). A small driver loop calls actions until one returns
`nil`.

## State Machine Pattern

### Tokenizer Actions

```go
type tokenizerAction func(t *tokenizer, yield func(Token, error) bool) tokenizerAction
```

- Each action reads some runes, optionally calls `yield` to emit a `Token`, and
  returns the next action to execute.
- Return `nil` to end iteration.
- `yield` follows Go iterator conventions: it returns `false` to stop early.

### Parser Actions

```go
type parserAction[T any] func(p *parser, t T) (parserAction[T], error)
```

- Generic over the AST node being built (e.g. `*File`, and later `*Division`).
- Return `(nil, nil)` to complete successfully.
- Return `(nil, err)` to terminate with an error — every error path returns
  `nil` for the next action so the loop stays monotone.

### Printer Actions

```go
type printerAction func(pr *printer, f *File) printerAction
```

- Each action writes some output and returns the next action; return `nil` to
  end. There is **no** error return — errors accumulate in `pr.err`, and the
  driver loop stops on the first write failure.

## Tokenizer (`tokenizer.go`)

The tokenizer turns bytes into a lazy stream of `Token` values via
`Tokenize(r io.Reader) iter.Seq2[Token, error]`. The `tokenizer` struct wraps a
`*bufio.Reader` for one-rune lookahead and tracks `Pos{Line, Column}` so every
token knows where it came from. `next()` advances and updates position;
`backup(previousPos Pos)` rewinds the last rune and restores the captured
position.

`Token.Value` is a `[]byte` slice. `TokenType` is a typed int with a `String()`
method — named values pay for themselves the first time a test fails.

### Helpers

- `yieldTokenThen(tok, next)` — yield a token, then continue with `next`. The
  most common ending of an action.
- `yieldErrorOr(err, next)` — continue with `next` on a nil error, terminate
  cleanly at end of input (`io.EOF` or `io.ErrUnexpectedEOF`), otherwise yield
  the error then continue. Use it after any operation that may fail.
- `skipWhitespace(next)` — consume leading whitespace, then run `next`.

### Entry point pattern

`tokenizeCOBOL` captures the position **before** reading a rune, then dispatches
on that rune to a specific sub-tokenizer:

```go
func tokenizeCOBOL(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
    return skipWhitespace(func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
        pos := t.pos
        r, err := t.next()
        if err != nil {
            return yieldErrorOr(err, nil)
        }
        switch {
        // case r == '*': return tokenizeComment(pos)
        // ... dispatch
        }
    })
}
```

When a sub-tokenizer needs to capture state (the start position of a literal,
accumulated digits), return a **closure** that holds that state rather than
adding per-token fields to the struct.

### Errors

Use a typed error per failure mode (e.g. `UnexpectedCharacterError{Pos, R}`),
never a bare `fmt.Errorf` in the hot path, so the parser and tests can assert
with `errors.As`.

## Parser (`parser.go`)

`Parse(r io.Reader) (*File, error)` converts the push-based tokenizer to
pull-based with `iter.Pull2(Tokenize(r))` (`defer stop()`), then runs the
top-level action loop against a `*File`. The root AST node is `File`, a thin
container of `Programs []*Program` (a COBOL source file holds one or more
programs); each `Program` carries `Divisions []Division`. Every node below
`File` carries a `Pos`, mirroring `go/ast`, where every node is position-aware.

`File` has one other container: `Fragment *Fragment`, the data description
entries of a standalone copybook read with `Parse(r, WithFragment())`. A
copybook has no IDENTIFICATION DIVISION and no program for a DATA DIVISION to
hang off, so its entries get their own field rather than a synthetic
`DataDivision` — no node then claims a division header the source never had.
`Fragment` is non-nil only in that mode, and `Programs` is empty when it is; the
entry loop itself (`parseDataEntries`) is shared with the DATA DIVISION
sections, so a fragment is exactly the entry list a section would hold.

### `expect`

The `parser` exposes one helper:

```go
tok, err := p.expect(TokenIdentifier, TokenSymbol)
```

It pulls the next token and checks its type against the given types, returning
`UnexpectedEndOfTokensError` if the stream is exhausted or `UnexpectedTokenError`
otherwise. Use it everywhere the grammar requires a specific token; never inline
the type check. Its sibling `expectKeyword(kw ...string)` requires an identifier
whose value matches one of `kw` (case-insensitively, since COBOL reserved words
are case-insensitive), returning `UnexpectedKeywordError` on mismatch; use it
wherever the grammar requires a specific keyword rather than a token type.

### The inner action loop rule (the one that matters)

For any complex/nested construct — divisions, sections, paragraphs, statements,
data items — implementations **must** use an inner action loop, **not** an
inline `for` with a `switch`:

```go
func parseDivision(p *parser, prog *Program) (parserAction[*Program], error) {
    div := &SomeDivision{}
    var err error
    for action := parseDivisionHeader; action != nil && err == nil; {
        action, err = action(p, div)
    }
    if err != nil {
        return nil, err
    }
    prog.Divisions = append(prog.Divisions, div)
    return parseNextDivision, nil // dispatch the next division, or nil to end the program
}
```

Each state of the construct gets its own `parserAction[T]`. Complex parsers
accrete states; a flat switch becomes unreadable and untestable, while small
named action functions can be exercised directly. This is the single rule a fast
implementer is most likely to break.

## COPY expansion (`copy.go`)

`COPY` is COBOL's text-manipulation facility, not a construct of the grammar:
the library text replaces the statement — the word `COPY` through its
terminating period, inclusive — before compilation. So it lives in its own pass
between the tokenizer and the parser, `expandCopy`, and **no `COPY` node exists
in the AST**. Every division, section and entry parser therefore sees only
copied text and needs to know nothing about copybooks.

- Callers supply library text through the `CopyBookResolver` interface
  (`Parse(r, WithCopyBooks(…))`); `FSCopyBooks` covers `os.DirFS` and `embed.FS`
  alike, `MapCopyBooks` covers in-memory, and `CopyBookFunc` covers anything
  else.
- The statement itself is parsed with the house action-loop pattern — one named
  `copyAction` per state (`parseCopyName`, `parseCopyLibrary`,
  `parseCopyReplacing`, …) — over the pull function the expander already holds.
- `REPLACING` is applied to the copybook's **text**, over *text words*, before
  that text is tokenized. Matching whole text words is what stops a pattern
  matching part of a word; replacing the matched **byte span** is what keeps
  `:TAG:` welded to the `-CUSTOMER-ID` after it. Neither property survives a
  token-level substitution, which is why this pass is textual.
- Copied text is re-tokenized in the same reference format and run through
  `expandCopy` again, so copybooks nest; a stack of case-folded, library-
  qualified names turns a loop into a `CopyBookCycleError`.
- Positions on copied nodes are positions *within the copybook*. `Pos` has no
  field naming a file, so this is a documented trade rather than an oversight.

## Listing directives (`directive.go`)

The listing-control statements — `EJECT`, `SKIP1`/`SKIP2`/`SKIP3` and
`TITLE <literal>` — direct the compiler's source listing and not the compilation
of the source text, so `skipListingDirectives` **drops** them from the token
stream and **no node exists for them in the AST**. That is the settled choice,
argued in `SPEC.md` §"Listing-Control Statements": this printer emits canonical
source rather than a listing, and the package already loses the same page-eject
intent when it re-emits a fixed-format `/` comment line as `*>`.

- Recognition is the standard's own rule — the statement must be **the only
  statement on its line** — so a word counts only when it *opens* a line, and
  only tokens on that same line can belong to it (`TITLE`'s literal, the
  optional separator period). A period on a later line is left to whatever
  follows.
- `TITLE` requires its literal operand to be recognized, which is what keeps a
  bare `TITLE` opening a line an ordinary word (a paragraph name, say).
- The pass wraps **each tokenizer stream separately** — the source unit's in
  `Parse`, and each copybook's inside `expandOne`. Recognition is line-relative
  and copied tokens are positioned within their own copybook, so one filter
  spanning the seam would read a copybook's line 1 as a continuation of the line
  the `COPY` statement sat on. This is why the pass is not simply a diversion in
  `parser.next()` beside the comment one.
- It runs **before** `expandCopy`, so the `COPY` scan never meets a directive.

## Printer (`printer.go`)

`Print(w io.Writer, f *File) error` runs the action loop, checking `pr.err` each
iteration. The `printer` wraps an `io.Writer` and stores `err error`; every
write goes through `pr.write(s)` or `pr.writef(format, args...)`, which
short-circuit when `pr.err != nil`. Use `writeThen(s, next)` for the common
write-then-continue step.

When printing a slice (divisions, statements), use a **closure** that captures
the current index and returns either "print the current element then advance" or
`nil` when the index is past the end — same shape as the tokenizer's closure
pattern, no mutable iterator state on the printer struct.

## Testing Style

- **Table-driven**, with a `testCases` slice and `t.Run(tc.name, ...)`. Names
  are lowercase descriptive.
- `t.Parallel()` at **both** the test function and each subtest. Action
  functions are pure, so parallel tests catch hidden global state.
- Assertions via `github.com/stretchr/testify/require` (not `assert`) — a
  parser test that keeps running after the first failure produces noise.
- Run `go test -race ./...` after every change.

### Tokenizer tests

Source string in, `[]Token` out. A `collect` helper drains the
`iter.Seq2[Token, error]`. Specify **exact** positions for every token — getting
them right early saves debugging later.

### Parser tests

Source string in, `*File` out via the public `Parse()`. **Drive `Parse()` with
real source strings, then assert the result against a hand-built expected
`*File` with `require.Equal`** — positions included, in the avro-go/idl
parser-test style this package is modeled on. Specify exact `Pos` for every node
(copy them from the matching tokenizer test). Failure-path subtests use
`require.ErrorAs` for typed errors and `require.ErrorIs` for sentinels.

### Printer tests

Two shapes, both required for every printer method once real ones exist:

1. **Direct** — explicit `*File` in, expected string out. Pins down formatting
   (whitespace, punctuation, fixed-format columns) the round-trip can't see.
2. **Round-trip** — `Parse → Print → Parse`, comparing the two ASTs **ignoring
   `Pos`** (the printer reformats canonically, as `go/printer` does, so positions
   shift). The cheapest end-to-end correctness check; a mismatch is almost always
   a parser dropping a token or a printer omitting punctuation the parser made
   optional.

## Why this shape

One format, one package, a small set of production files, one action-loop
pattern repeated in each of them, round-trip tests on every printer method.
COBOL accretes constructs; this layout keeps the round-trip property auditable
at a glance.
