# COBOL Specification Reference

## Overview

This document is the single source of truth for the `cobol` package — a
tokenizer / parser / printer pipeline for COBOL source. It distills the lexical
and syntactic rules of the ISO/IEC 1989 COBOL standard, as implemented by
GnuCOBOL, into a form each pipeline stage can be built against. It covers **both
reference formats** COBOL source may be written in: **free format** (free-form)
and **fixed format** (the historical, column-oriented "reference format").

The lexical token classes and the grammar are **identical across the two
formats**; they differ only in *source layout* — whether columns carry meaning,
how comments are marked, and how a line is continued. Those differences are
called out in per-format subsections (`#### Free format` / `#### Fixed format`)
where they arise; everything else applies to both.

The COBOL language is large; this reference is deliberately scoped to the
*core* of a source program: the lexical surface (character set, COBOL words,
literals, separators, PICTURE character-strings) and the skeleton of all four
divisions (IDENTIFICATION, ENVIRONMENT, DATA, PROCEDURE). It covers enough to
tokenize, parse, and round-trip realistic programs; the detailed grammar of
individual PROCEDURE statements and rarely-used clauses is left to the story
that implements each one.

> **Implementation status:** Free format is implemented first; the fixed-format
> batch follows. The fixed format is **fully specified here**; its *tokenization*
> (#21) is **implemented** — opt in with `WithFixedFormat()` — while
> *source-format detection / configuration* (#22) and *round-trip fixtures* (#23)
> are deferred to later stories. Also out of scope:
> the COPY/REPLACE text-manipulation facility (copybooks), the REPORT and SCREEN
> sections beyond their headers, object-oriented (class/method) and
> user-defined-function compilation units, and the full ~1130-word reserved word
> list.

**Governing sources**

- **ISO/IEC 1989:2014**, *Information technology — Programming languages —
  Programming language COBOL* (and its successor **ISO/IEC 1989:2023**). This is
  the normative standard; free-form reference format was introduced in
  ISO/IEC 1989:2002. <https://www.iso.org/standard/51416.html>
- **GnuCOBOL Programmer's Guide**, Chapter 2 "COBOL Fundamentals" — the
  practical, freely-available reference this document is grounded in. Section
  numbers below (e.g. *GnuCOBOL §2.1.16*) refer to it.
  <https://superbol.eu/gnucobol/gnucobpg/chapter2.html>

> **Ambiguity:** The ISO standard is paywalled; where the standard and GnuCOBOL
> differ, this document follows **GnuCOBOL** behavior and flags the divergence,
> since GnuCOBOL is the reference implementation the package is validated
> against. GnuCOBOL-specific extensions (e.g. the `&` literal-concatenation
> operator, `Z"…"` null-terminated literals) are marked as such.

## Lexical Elements (Tokens)

The tokenizer turns a byte stream into a lazy sequence of `Token` values. The
token classes below are **identical in both reference formats**; what differs is
the *source layout* — column significance, comment markers, and line
continuation — covered in
[Whitespace and Delimiters](#whitespace-and-delimiters), [Comments](#comments),
and [Line Continuation](#line-continuation). Every byte of a well-formed program
belongs to exactly one of the token classes in this section (in fixed format,
once the column areas have been accounted for).

The package's existing `TokenType` enum seeds these classes
(`Comment`, `Identifier`, `Symbol`, `String`, `Number`); the names below are the
authoritative lexical classes and note the seed value each maps to.

| Token class | Maps to seed `TokenType` | Role |
|---|---|---|
| `Comment` | `TokenComment` | `*>` inline/full-line comment |
| `Word` | `TokenIdentifier` | COBOL word: reserved word or user-defined word |
| `Symbol` | `TokenSymbol` | separators and operators (`.`, `(`, `)`, `+`, `=`, …) |
| `AlphanumericLiteral` | `TokenString` | quoted string literal (and hex/national variants) |
| `NumericLiteral` | `TokenNumber` | integer, fixed-point, or floating-point number |
| `PictureString` | *(new)* | a PICTURE character-string, scanned as one token |
| `CompilerDirective` | *(new)* | a `>>` compiler-directing line |

> **Ambiguity:** Whether reserved words get a distinct token type or are
> `Word` tokens disambiguated by a keyword table is an implementation choice.
> This reference treats all COBOL words as the lexical class `Word`; the parser
> (or a keyword-lookup step) separates reserved words from user-defined words.
> Likewise, a level-number is lexically a `NumericLiteral` (1–2 digits); the
> *parser* recognizes it as a level-number by position (start of a data
> description entry). See [Symbols and Operators](#symbols-and-operators) and the
> data division grammar.

### Whitespace and Delimiters

- **Significant whitespace:** A space (`U+0020`) is *the* COBOL separator and is
  significant as a token boundary. One or more spaces separate adjacent COBOL
  words and literals; `MOVE A TO B` is three words, `MOVEATOB` is one.
- **Ignorable whitespace:** Beyond their separating role, runs of spaces, tabs
  (`U+0009`), and newlines (`\n`, `\r\n`) carry no meaning between tokens and may
  appear freely. Blank lines are permitted anywhere (*GnuCOBOL §2.1.16*).

Whether *columns* constrain where tokens may appear is the one lexical rule that
differs between the two reference formats.

#### Free format

A statement "may be up to 255 characters long, with no specific requirements as
to what should appear in which columns" (*GnuCOBOL §2.1.16*). There is **no
column significance**; the Area A / Area B distinctions below do **not** apply,
and tokens are recognized purely by content.

#### Fixed format

Each physical source line is divided into fixed **column areas**, a legacy of
the 80-column punched card (*GnuCOBOL §2.1.16*):

| Columns | Area | Contents |
|---|---|---|
| 1–6 | Sequence (number) area | A historical 6-digit card sequence number. Ignored by the compiler; any characters may appear and carry no meaning. |
| 7 | Indicator area | A single control character (see table below). |
| 8–11 | Area A | Where DIVISION / SECTION / paragraph headers, the level numbers `01` and `77`, and the `FD` / `SD` description indicators must **begin**. |
| 12–72 | Area B | Everything else: clauses, statements, and continued text. |
| 73–80 | Identification (program-name) area | A historical program-id / commentary field. Ignored — GnuCOBOL discards anything past column 72. |

The **indicator area** (column 7) holds one of:

| Char | Meaning |
|---|---|
| space | Normal source line. |
| `*` | The whole line (columns 8–72) is a comment. |
| `/` | Comment line that also forces a page eject in the compiler listing. |
| `-` | Continuation line — see [Line Continuation](#line-continuation). |
| `D` / `d` | Debugging line: a valid statement normally treated as a comment, but compiled as ordinary Area B source when `WITH DEBUGGING MODE` is in effect. |

- A line may be **shorter** than 72 columns; the missing columns are treated as
  spaces. Characters at or past column 73 are **ignored**.
- Which constructs must begin in Area A vs Area B is a *fixed-format-only*
  constraint on the grammar — see
  [Ordering and Optionality](#ordering-and-optionality). The token classes
  themselves are unchanged.

  > **Ambiguity (tabs, Area A/B strictness):** Tab handling in the column areas
  > is implementation-defined; GnuCOBOL expands tabs to a configurable tab width
  > before applying the column rules. GnuCOBOL is also lenient by default about
  > *enforcing* Area A vs Area B placement (a relaxation, not a hard error). The
  > tokenizer (#21) **records** column positions in each token's `Pos` but does
  > **not** enforce Area A vs Area B placement, matching GnuCOBOL's lenient
  > default; any such enforcement is left to the parser.

The **separators** of COBOL (a string of one or more of these characters) are
the same in both reference formats:

| Separator | Characters | Notes |
|---|---|---|
| Space | `U+0020` | the primary separator; required between words |
| Separator comma | `,` followed by a space | optional; readability only |
| Separator semicolon | `;` followed by a space | optional; readability only |
| Separator period | `.` followed by a space, newline, or EOF | terminates a sentence or entry |
| Left/right parenthesis | `(` `)` | subscripts, reference modification, expressions |
| Quotation mark | `"` or `'` | delimits an alphanumeric literal (must match) |
| Colon | `:` | reference-modification range; must be bounded by spaces or used inside `( … )` |

> **Ambiguity (separator period vs decimal point):** A period is a **separator
> period** only when immediately followed by a space, newline, or end of input;
> a period embedded in a numeric literal (`3.14`) or a PICTURE string
> (`ZZ9.99`) is **not** a separator. The tokenizer must use the trailing-space
> rule to decide. Symmetrically, a comma or semicolon is a **separator** only
> when followed by a space, and is then consumed like whitespace (not emitted as
> a token) — so even in list contexts such as subscripts the operands are
> effectively space-separated: `(2, 3)` and `(2 3)` tokenize identically, while
> `(2,3)` (no space) is *not* a separator comma. Under `DECIMAL-POINT IS COMMA`
> the separator comma is unavailable — a `,` is then a decimal point inside
> numeric literals and PICTURE strings — so a semicolon or space must separate
> list items (see Semantics).

### Comments

The floating `*>` comment works in **both** reference formats; fixed format adds
two column-7 indicators.

- **Inline / floating comment (`*>`, both formats):** `*>` begins a comment that
  runs to the end of the physical line, and may appear in any column — as the
  first non-blank characters of a line (full-line comment) or after code on the
  same line (inline comment) (*GnuCOBOL §2.1.16*).

  ```cobol
  *> a full-line comment
  DISPLAY "hi".  *> an inline comment
  ```

- Comments do **not** nest and have no meaning inside an alphanumeric literal —
  `"a*>b"` is the five-character literal `a*>b`, not a comment.
- **Comment-entry** (a different thing): the text following an IDENTIFICATION
  paragraph such as `AUTHOR.` is a free-text *comment-entry*, not a `*>` comment.
  See the ambiguity note in [Grammar Productions](#grammar-productions).

#### Free format

`*>` is the **only** comment form; there are no column-7 indicators.

#### Fixed format

In addition to `*>`, the [indicator area](#whitespace-and-delimiters) (column 7)
marks whole-line comments (*GnuCOBOL §2.1.16*):

- `*` in column 7 — the entire line (columns 8–72) is commentary.
- `/` in column 7 — likewise a comment, and also forces a page eject in the
  compiler listing.
- `D` / `d` in column 7 — a **debugging line**: treated as a comment unless the
  program is compiled `WITH DEBUGGING MODE` (the `SOURCE-COMPUTER … WITH
  DEBUGGING MODE` clause), in which case columns 8–72 are ordinary Area B source.

### Line Continuation

How a long construct is spread across multiple physical lines is the other
lexical rule that differs between the formats. The `&` literal-concatenation
operator works in **both**; fixed format additionally has the column-7 hyphen.

#### Free format

There is **no** column-7 continuation. A statement may simply span multiple
physical lines, but a single token (word, number, picture-string, or a complete
quoted literal) may **not** be split across a newline. A long alphanumeric
literal is continued by splitting it into fragments joined with the
concatenation operator `&` (*GnuCOBOL §2.1.19.2*):

```cobol
01 MSG PIC X(40) VALUE "This is a long literal that "
                     & "spans two source lines.".
```

> **Ambiguity:** `&` literal concatenation is standard in COBOL 2014
> (concatenation expression) and supported by GnuCOBOL; treat it as the
> free-format continuation mechanism.

#### Fixed format

A `-` (hyphen) in the [indicator area](#whitespace-and-delimiters) (column 7)
marks a **continuation line**: the first non-blank character of its Area B
continues the last non-blank character of the preceding non-comment line **with
no intervening space**. Area A of a continuation line must be blank, and any
intervening comment or blank lines are skipped when finding the line being
continued (*GnuCOBOL §2.1.19.2*).

- **Words and numeric literals.** Run the word or numeric literal up to (at most)
  column 72, put `-` in column 7 of the next line, and resume at the first
  non-blank character of Area B. No quotation mark is involved. (Rarely needed —
  a word is at most 31 characters — but legal.)
- **Alphanumeric (nonnumeric) literals.** The continued line carries the literal
  up to and **including column 72 with no closing quotation mark**; the
  continuation line has `-` in column 7 and a **matching quotation mark** (the
  same `"` or `'` that opened the literal) somewhere in Area B, and the literal
  resumes at the character **immediately after** that quotation mark. Because the
  continued line has no closing quote, **every character position through column
  72 — including trailing spaces — is part of the literal**.

Worked example (the ruler shows column numbers and is not part of the source):

```cobol
----+----1----+----2----+----3----+----4----+----5----+----6----+----7--
           DISPLAY "This long literal runs out to column 72, then resume
      -    "d here.".
```

The first line's literal text fills exactly to column 72 (no closing quote); the
continuation line, with `-` in column 7, resumes the literal at the character
after its `"`, giving the value `This long literal runs out to column 72, then
resumed here.`

> **Ambiguity (trailing spaces):** The "everything through column 72 belongs to
> the literal" rule is the subtle part — to avoid accidental trailing spaces, the
> portable practice is to fill the literal exactly to column 72 on each continued
> line. The tokenizer must honor the column-72 boundary precisely; getting it
> wrong silently changes a literal's length. This is a central reason the
> fixed-format tokenizer (#21) must be column-aware.

The `&` concatenation operator (above) also works in fixed format and is the
format-independent alternative.

### Literals

**Alphanumeric literal** (`AlphanumericLiteral`)

- A run of characters delimited by a matching pair of `"` or `'`
  (*GnuCOBOL §2.1.19.2*). The opening and closing delimiter must be the same
  character.
- **Embedding the delimiter:** double it inside the literal. `"He said ""hi"""`
  is the literal `He said "hi"`; `'it''s'` is `it's`.
- How a long literal spans physical lines depends on the source format; see
  [Line Continuation](#line-continuation). In brief: free format joins fragments
  with `&`, while fixed format also allows the column-7 hyphen continuation.
- The empty literal (`""` / `''`) is valid.
- **Hexadecimal alphanumeric literal:** `X"4142"` (or `X'…'`, `H"…"`) — an even
  number of hex digits, each pair one byte; `X"4142"` is `AB`
  (*GnuCOBOL §2.1.19.2*).
- **National literal:** `N"…"` — national (typically UTF-16/multi-byte)
  characters.
- **Null-terminated literal (GnuCOBOL extension):** `Z"…"` — like an
  alphanumeric literal but with a trailing `0x00` byte.
- **Boolean literal (extension):** `B"1010"` — a string of `0`/`1` bits.

  > The `X`/`N`/`Z`/`B`/`H` prefix is part of the token; the tokenizer reads the
  > prefix, then the delimited body. The prefix is case-insensitive.

**Numeric literal** (`NumericLiteral`)

- Optional leading sign (`+` or `-`), one or more decimal digits, an optional
  decimal point (`.`), and an optional exponent introduced by `E`/`e`
  (*GnuCOBOL §2.1.19.1*). Examples: `0`, `42`, `-7`, `3.14`, `-2.95`,
  `9.92E25`, `5.7E-14`.
- At most one sign and at most one decimal point in the mantissa. A decimal
  point may not be the last character of the literal token (a trailing `.` is a
  separator period).
- Precision/range per GnuCOBOL: roughly `−1.7×10^308 … +1.7×10^308`, up to ~15
  significant decimal digits in floating form; fixed-point integers up to 18
  (standard) or 31 (extended) digits.
- Hex/binary/octal numeric variants (`H"…"`, `B"…"`, `O"…"`) exist as
  extensions; treat as numeric literals.

**Figurative constants** (reserved words that stand for a literal value;
*GnuCOBOL §2.1.19.3*)

| Constant | Aliases | Value |
|---|---|---|
| `ZERO` | `ZEROS`, `ZEROES` | the value 0 / one-or-more `0` characters |
| `SPACE` | `SPACES` | one or more space characters |
| `HIGH-VALUE` | `HIGH-VALUES` | highest character in the collating sequence |
| `LOW-VALUE` | `LOW-VALUES` | lowest character (ASCII NUL by default) |
| `QUOTE` | `QUOTES` | the quotation-mark character(s) |
| `NULL` | `NULLS` | a zero-valued (`0x00`) byte / null pointer |
| `ALL literal` | — | the literal repeated to fill the receiving field |

Figurative constants are reserved words, not user-defined names; their *length*
is determined by the context they are used in (the size of the receiving item),
so they may not be used where a fixed length is required, e.g. as a subprogram
argument (*GnuCOBOL §2.1.19.3*). See Semantics.

### Keywords and Reserved Words

- COBOL defines a large fixed set of **reserved words** (GnuCOBOL: "well over
  1130", *GnuCOBOL §2.1.1*) — division/section/paragraph names, verbs, clause
  keywords, figurative constants, and special registers. They cannot be used as
  user-defined names.
- **Case-insensitive:** reserved words and user-defined words alike are matched
  without regard to case — `IDENTIFICATION`, `identification`, and `Identification`
  are the same word (*GnuCOBOL §2.1.3*).
- A programmer may use a reserved word as *part* of a longer user-defined word,
  but may not define a word identical (ignoring case) to a reserved word
  (*GnuCOBOL §2.1.1*).
- This reference does **not** enumerate the full reserved-word list; the
  implementer maintains a keyword table. The reserved words actually exercised
  by the [grammar](#structure-grammar) below (division/section headers, common
  clauses, common verbs) are the minimum that table must contain.

**User-defined word** (`Word` that is not reserved; *GnuCOBOL §2.1.2*)

- Composed of letters `A–Z`/`a–z`, digits `0–9`, hyphen (`-`), and underscore
  (`_`).
- May **not** begin or end with a hyphen or underscore.
- May begin with a digit (unusual among languages), but must contain at least
  one letter — **except** procedure (paragraph/section) names, which may consist
  entirely of digits.
- Maximum length **31** characters (COBOL 2014); extendable to 63 in GnuCOBOL
  via `-std` compatibility flags.

### Symbols and Operators

`Symbol` tokens cover structural punctuation and operators. Most are single
characters; a few are multi-character and must be recognized greedily.

| Symbol(s) | Role |
|---|---|
| `.` | separator period — sentence / entry terminator (when followed by space/EOL) |
| `(` `)` | subscripts, reference modification, grouping in expressions |
| `:` | reference-modification separator, `start:length` |
| `,` `;` | optional separators (followed by a space) |
| `+` `-` `*` `/` `**` | arithmetic operators (`**` = exponentiation); `+`/`-` also unary sign |
| `=` | relational equality; assignment target marker in `COMPUTE` |
| `>` `<` `>=` `<=` `<>` | relational operators (greater, less, ≥, ≤, not-equal) |
| `&` | alphanumeric concatenation (literal continuation) |
| `>>` | introduces a [compiler-directing](#compiler-directives) line |

Word-form operators are reserved **words**, not symbols: `AND`, `OR`, `NOT`,
`GREATER`, `LESS`, `EQUAL`, `THAN`, `TO`. The tokenizer emits these as `Word`
tokens; the parser treats them as operators in conditions.

> **Ambiguity (greedy lexing):** `**`, `>=`, `<=`, and `<>` must be matched
> before their single-character prefixes (`*`, `>`, `<`), and `>>` before `>`.
> A bare `*` is multiplication; a `*>` is a comment start — the tokenizer must
> peek the following character to choose.

### PICTURE Character-Strings

A `PictureString` is a single token: a contiguous run of PICTURE symbols
describing a data item's size and category. It appears only after the reserved
word `PICTURE` / `PIC` (optionally followed by `IS`) in a data description
entry.

> **Ambiguity (context-sensitive lexing):** A PICTURE string such as
> `-ZZ,ZZ9.99` or `S9(5)V99` contains characters that are otherwise separators
> and operators (`-`, `,`, `.`, `(`, `)`, `+`, `*`, `/`). A general scan would
> shred it into many tokens. The tokenizer therefore must recognize that the
> word `PICTURE`/`PIC` (with optional `IS`) switches it into a PICTURE-scanning
> mode: it consumes the following run of PICTURE characters as **one**
> `PictureString` token, stopping at a separator (a space, or a separator period
> — `.` followed by space/EOL). This is the central reason PICTURE gets its own
> token class and its own implementing story.

PICTURE symbols (case-insensitive; `9 A X Z V S P B 0 CR DB` etc.), grounded in
the standard's PICTURE-clause table:

| Symbol | Category | Meaning |
|---|---|---|
| `9` | digit | a numeric digit position |
| `A` | alphabetic | a letter or space |
| `X` | alphanumeric | any character |
| `Z` | zero suppression | leading-zero suppressed to space |
| `*` | zero suppression | leading-zero replaced by `*` (check protection) |
| `V` | assumed decimal | implied decimal point; occupies no storage |
| `S` | sign | operational sign; leftmost; no storage unless `SIGN … SEPARATE` |
| `P` | scaling | assumed scaling digit; leftmost or rightmost run only; no storage |
| `B` | simple insertion | inserts a space |
| `0` | simple insertion | inserts a `0` |
| `/` | simple insertion | inserts a `/` |
| `,` | simple insertion | inserts a thousands separator |
| `.` | special insertion | actual (printed) decimal point |
| `+` | fixed/floating sign | inserts `+`/`-` per value sign |
| `-` | fixed/floating sign | inserts space/`-` per value sign |
| `CR` | fixed insertion | trailing `CR` when value is negative |
| `DB` | fixed insertion | trailing `DB` when value is negative |
| `$` (cs) | floating insertion | currency symbol (default `$`; set by `CURRENCY SIGN`) |

- **Repeat count:** a symbol may be followed by a parenthesized integer to
  repeat it: `9(5)` ≡ `99999`, `X(10)` ≡ ten `X`s, `Z(4)` ≡ `ZZZZ`. The repeat
  count is part of the PICTURE string token.
- **Position rules:** `S` (if present) is leftmost; `V` marks the assumed
  decimal point and may appear once; `P` scaling positions form a single run at
  the left or right end.
- The category of the item (numeric, alphabetic, alphanumeric, numeric-edited,
  alphanumeric-edited) is derived from which symbols appear — see Semantics.

### Compiler Directives

A line whose first non-blank characters are `>>` is a **compiler-directing**
line (CDF). It is a `CompilerDirective` token spanning to end of line. In fixed
format a CDF line is written in Area A / Area B (columns 8–72) with a blank
indicator area; in free format it may begin in any column. Relevant directives:

- **Source-format selection** — `>>SOURCE FORMAT IS FREE` | `FIXED`,
  `>>SET SOURCEFORMAT "FREE"` | `"FIXED"`, or `>>FORMAT IS FREE` | `FIXED`
  selects the reference format for the lines that follow (*GnuCOBOL §2.1.16*).
  GnuCOBOL defaults to **fixed** when neither such a directive nor a command-line
  `-free` / `-fixed` option is given. Detecting and honoring the format (vs.
  defaulting) is the source-format story (#22).
- `>>PAGE` — page eject; the free-format counterpart of the fixed-format `/`
  column-7 indicator.
- `>>SET`, `>>DEFINE`, `>>IF` / `>>ELSE` / `>>END-IF` — conditional compilation.

> **Ambiguity:** Full CDF semantics (conditional compilation, `DEFINE`
> substitution) are out of scope for the core parser; recognize the directive
> line as a token and, at minimum, honor `SOURCE FORMAT`. The AST may keep
> directive lines verbatim for round-tripping.

## Structure (Grammar)

The grammar below is the parser's contract. Each production becomes a parser
action; production names follow COBOL standard nomenclature so AST node names
can match.

**Meta-notation (EBNF):**

| Notation | Meaning |
|---|---|
| `name = …` | production definition |
| `"WORD"` | a terminal reserved word (`Word` token, case-insensitive) |
| `TokenClass` | a terminal of that lexical class (`PictureString`, `NumericLiteral`, …) |
| `a b` | sequence (a then b) |
| `a \| b` | alternation |
| `[ a ]` | optional (0 or 1) |
| `{ a }` | repetition (0 or more) |
| `( a )` | grouping |
| `« … »` | informal prose constraint |

Terminals written as bare UPPERCASE words are reserved words. `.` in a
production means a **separator period** token.

`Comment` and `CompilerDirective` tokens are **trivia**: they may appear between
any two tokens of any production and are not written into the productions below.
The tokenizer emits them (so they can be preserved for round-tripping) but they
do not affect phrase structure — with one exception, a leading
`>>SOURCE FORMAT IS FREE`, which selects the source format.

### Names and Terminals

The productions reference these leaf terminals, all resolving to lexical token
classes from [Lexical Elements](#lexical-elements-tokens):

```ebnf
user-defined-word = Word            « a Word that is not a reserved word »
data-name         = user-defined-word
file-name         = user-defined-word
computer-name     = user-defined-word
mnemonic-name     = user-defined-word
condition-name    = user-defined-word
index-name        = user-defined-word
alphabet-name     = user-defined-word
assignment-name   = user-defined-word
device-name       = Word            « an implementor-defined name, e.g. CONSOLE »
paragraph-name    = user-defined-word | NumericLiteral   « may be all digits »
section-name      = user-defined-word | NumericLiteral
procedure-name    = paragraph-name [ ( "IN" | "OF" ) section-name ]
                  | section-name

literal             = AlphanumericLiteral | NumericLiteral | figurative-constant
figurative-constant = "ZERO"  | "ZEROS"  | "ZEROES"
                    | "SPACE" | "SPACES"
                    | "HIGH-VALUE" | "HIGH-VALUES"
                    | "LOW-VALUE"  | "LOW-VALUES"
                    | "QUOTE" | "QUOTES" | "NULL" | "NULLS"
                    | "ALL" literal
comment-entry       = « free-form text up to the next header; not tokenized as
                        COBOL words — see the ambiguity note under Identification »
```

Structural placeholders left in prose (e.g. `« object-computer-clause »`,
`« file-clause »`, `« i-o-control-clause »`, `« use-spec »`, `« alphabet-spec »`)
are clause sets elaborated by the story that implements them; only the clauses
needed for the core slice are spelled out in the productions below.

### Top-Level Structure

A free-format source file is a sequence of one or more **programs**. A program
begins with an IDENTIFICATION DIVISION and contains the four divisions in fixed
order; only IDENTIFICATION (with its `PROGRAM-ID`) is mandatory.

```ebnf
source-file       = program { program }

program           = identification-division
                    [ environment-division ]
                    [ data-division ]
                    [ procedure-division ]
                    [ « nested-program » ]
                    [ "END" "PROGRAM" program-name "." ]

program-name      = user-defined-word | AlphanumericLiteral
```

- The divisions must appear in the order ID → ENVIRONMENT → DATA → PROCEDURE.
- `END PROGRAM` is required only when programs are nested or concatenated; a
  lone program may omit it.
- `>>` compiler-directing lines (`CompilerDirective` tokens) are trivia: they
  may appear between any tokens — before, between, or inside programs (e.g.
  `>>PAGE`, conditional-compilation directives), not only in a leading preamble.
  By convention `>>SOURCE FORMAT IS FREE`, when present, appears first. Document
  end: end of input after the last program.

> **Ambiguity:** Nested programs, and non-program compilation units (functions,
> classes, interfaces), are recognized structurally but their full grammar is
> out of scope here; the core target is a single program.

### Grammar Productions

#### Identification Division

```ebnf
identification-division =
      ( "IDENTIFICATION" | "ID" ) "DIVISION" "."
      program-id-paragraph
      { identification-comment-paragraph }

program-id-paragraph =
      "PROGRAM-ID" "." program-name
      [ [ "IS" ] ( "INITIAL" | "RECURSIVE" | "COMMON" ) [ "PROGRAM" ] ] "."

identification-comment-paragraph =
      ( "AUTHOR" | "INSTALLATION" | "DATE-WRITTEN"
      | "DATE-COMPILED" | "SECURITY" | "REMARKS" ) "." comment-entry
```

- `PROGRAM-ID` is **required**; it names the program.
- The comment paragraphs (`AUTHOR`, etc.) are **obsolete** in COBOL 2014 but
  still accepted; their content is a free-text *comment-entry*.

> **Ambiguity (comment-entry lexing):** A `comment-entry` is arbitrary text
> running to the start of the next paragraph/section/division header — it is
> *not* tokenized as COBOL words. In free format the practical rule is "to end
> of line(s) until the next recognizable header." Because these paragraphs are
> obsolete, an implementation may instead skip the entry text. Flag this and
> decide per story; treating them as opaque text up to the next header is the
> recommended default.

#### Environment Division

```ebnf
environment-division =
      "ENVIRONMENT" "DIVISION" "."
      [ configuration-section ]
      [ input-output-section ]

configuration-section =
      "CONFIGURATION" "SECTION" "."
      [ source-computer-paragraph ]
      [ object-computer-paragraph ]
      [ special-names-paragraph ]

source-computer-paragraph =
      "SOURCE-COMPUTER" "." [ computer-name [ "WITH" "DEBUGGING" "MODE" ] "." ]

object-computer-paragraph =
      "OBJECT-COMPUTER" "." [ computer-name { « object-computer-clause » } "." ]

special-names-paragraph =
      "SPECIAL-NAMES" "." { special-names-clause } [ "." ]

special-names-clause =
        "DECIMAL-POINT" [ "IS" ] "COMMA"
      | "CURRENCY" "SIGN" [ "IS" ] AlphanumericLiteral
      | device-name "IS" mnemonic-name
      | "ALPHABET" alphabet-name "IS" « alphabet-spec »
      | « other implementor associations »

input-output-section =
      "INPUT-OUTPUT" "SECTION" "."
      [ file-control-paragraph ]
      [ i-o-control-paragraph ]

file-control-paragraph =
      "FILE-CONTROL" "." { file-control-entry }

file-control-entry =
      "SELECT" [ "OPTIONAL" ] file-name
      "ASSIGN" [ "TO" ] ( assignment-name | AlphanumericLiteral )
      { select-clause } "."

select-clause =
        "ORGANIZATION" [ "IS" ] ( "SEQUENTIAL" | "LINE" "SEQUENTIAL"
                                | "RELATIVE" | "INDEXED" )
      | "ACCESS" [ "MODE" ] [ "IS" ] ( "SEQUENTIAL" | "RANDOM" | "DYNAMIC" )
      | "RECORD" [ "KEY" ] [ "IS" ] data-name
      | "FILE" "STATUS" [ "IS" ] data-name

i-o-control-paragraph = "I-O-CONTROL" "." { « i-o-control-clause » } [ "." ]
```

- The ENVIRONMENT DIVISION as a whole is optional; both sections are optional;
  every paragraph is optional. `DECIMAL-POINT IS COMMA` and `CURRENCY SIGN`
  affect lexing of numeric literals and PICTURE strings (see Semantics).

#### Data Division

```ebnf
data-division =
      "DATA" "DIVISION" "."
      [ file-section ]
      [ working-storage-section ]
      [ local-storage-section ]
      [ linkage-section ]

file-section =
      "FILE" "SECTION" "."
      { file-description-entry { data-description-entry } }

file-description-entry =
      ( "FD" | "SD" ) file-name { « file-clause » } "."

working-storage-section =
      "WORKING-STORAGE" "SECTION" "." { data-description-entry }

local-storage-section =
      "LOCAL-STORAGE" "SECTION" "." { data-description-entry }

linkage-section =
      "LINKAGE" "SECTION" "." { data-description-entry }

data-description-entry =
        level-number [ entry-name ] { data-clause } "."
      | renames-entry
      | condition-name-entry

level-number = NumericLiteral   « an integer 01–49 or 77 »
entry-name   = data-name | "FILLER"

renames-entry =
        NumericLiteral « 66 » data-name "RENAMES" data-name
        [ ( "THROUGH" | "THRU" ) data-name ] "."

condition-name-entry =
        NumericLiteral « 88 » condition-name "VALUE" [ "IS" ]
        value-spec { value-spec } "."

value-spec = literal [ ( "THROUGH" | "THRU" ) literal ]

data-clause =
        "REDEFINES" data-name
      | ( "PICTURE" | "PIC" ) [ "IS" ] PictureString
      | [ "USAGE" [ "IS" ] ] usage-type
      | ( "VALUE" | "VALUES" ) [ "IS" ] literal
      | occurs-clause
      | "SIGN" [ "IS" ] ( "LEADING" | "TRAILING" ) [ "SEPARATE" [ "CHARACTER" ] ]
      | ( "JUSTIFIED" | "JUST" ) [ "RIGHT" ]
      | ( "SYNCHRONIZED" | "SYNC" ) [ "LEFT" | "RIGHT" ]
      | "BLANK" [ "WHEN" ] "ZERO"
      | "GLOBAL" | "EXTERNAL"

usage-type =
        "DISPLAY" | "BINARY" | "PACKED-DECIMAL"
      | "COMP" | "COMP-1" | "COMP-2" | "COMP-3" | "COMP-4" | "COMP-5"
      | "INDEX" | "POINTER"

occurs-clause =
      "OCCURS" NumericLiteral [ "TO" NumericLiteral ] [ "TIMES" ]
      [ "DEPENDING" [ "ON" ] data-name ]
      { ( "ASCENDING" | "DESCENDING" ) [ "KEY" ] [ "IS" ] data-name }
      [ "INDEXED" [ "BY" ] index-name ]
```

- A data description entry starts with a **level-number** and ends with a
  separator period; clauses may appear in any order (the standard fixes some
  orderings, but the parser should accept clauses order-independently and
  validate separately).
- **Level-numbers:** `01`–`49` define the record/group hierarchy (lower number =
  more inclusive); `77` is a standalone elementary item; `66` `RENAMES` regroups
  fields; `88` defines a condition-name on the preceding item.

#### Procedure Division

```ebnf
procedure-division =
      "PROCEDURE" "DIVISION" [ using-phrase ] [ returning-phrase ] "."
      [ declaratives ]
      procedure-body

using-phrase     = "USING" { [ "BY" "REFERENCE" | "BY" "VALUE" ] data-name }
returning-phrase = "RETURNING" data-name

declaratives =
      "DECLARATIVES" "."
      { section-name "SECTION" "." "USE" « use-spec » "." { paragraph } }
      "END" "DECLARATIVES" "."

procedure-body =
        { paragraph }            « a program with no explicit sections »
      | { section }              « a program organized into sections »

section =
      section-name "SECTION" [ NumericLiteral ] "." { paragraph }

paragraph =
      [ paragraph-name "." ] { sentence }

sentence  = { statement } "."

statement = « a verb-led imperative or conditional statement; see catalog »
```

- A `paragraph-name` / `section-name` is a user-defined word (may be all
  digits). A **sentence** is one or more statements ended by a separator period.
- A program with statements directly under PROCEDURE DIVISION uses the
  paragraph form; once any `SECTION` appears, the body is section-organized.

**Statement catalog (core verbs).** The full statement grammar is large; each
statement's detailed syntax is elaborated by its implementing story. The core
verbs the parser recognizes, with representative productions for the most
common, are:

```ebnf
display-statement =
      "DISPLAY" { operand } [ "UPON" mnemonic-name ]
      [ [ "WITH" ] "NO" "ADVANCING" ]

move-statement =
      "MOVE" [ "CORRESPONDING" | "CORR" ] operand "TO" identifier { identifier }

accept-statement =
      "ACCEPT" identifier [ "FROM" ( mnemonic-name | "DATE" | "TIME" | … ) ]

compute-statement =
      "COMPUTE" receiver { receiver }
      ( "=" | "EQUAL" ) arithmetic-expression
      [ on-size-error ] [ "END-COMPUTE" ]

arithmetic-statement =                  « ADD / SUBTRACT / MULTIPLY / DIVIDE »
      ( "ADD" | "SUBTRACT" | "MULTIPLY" | "DIVIDE" ) operand { operand }
      [ ( "TO" | "FROM" | "BY" | "INTO" ) receiver { receiver } ]
      [ "GIVING" receiver { receiver } [ "REMAINDER" identifier ] ]
                                          « REMAINDER: DIVIDE … GIVING only »
      [ on-size-error ]
      [ "END-ADD" | "END-SUBTRACT" | "END-MULTIPLY" | "END-DIVIDE" ]

receiver = identifier [ "ROUNDED" ]

if-statement =
      "IF" condition [ "THEN" ]
          ( statement { statement } | "NEXT" "SENTENCE" )
      [ "ELSE" ( statement { statement } | "NEXT" "SENTENCE" ) ]
      [ "END-IF" ]

perform-statement =
      "PERFORM"
      [ procedure-name [ ( "THROUGH" | "THRU" ) procedure-name ] ]
      [ NumericLiteral "TIMES"
      | [ "WITH" "TEST" ( "BEFORE" | "AFTER" ) ] "UNTIL" condition
      | "VARYING" identifier "FROM" operand "BY" operand "UNTIL" condition ]
      [ { statement } "END-PERFORM" ]

evaluate-statement =
      "EVALUATE" subject { "ALSO" subject }
      { "WHEN" object { "ALSO" object } { statement } }
      [ "WHEN" "OTHER" { statement } ]
      "END-EVALUATE"
subject = operand | condition | "TRUE" | "FALSE"
object  = "ANY" | "TRUE" | "FALSE"
        | [ "NOT" ] ( operand [ ( "THROUGH" | "THRU" ) operand ] | condition )

call-statement =
      "CALL" ( AlphanumericLiteral | identifier )
      [ "USING" { [ "BY" "REFERENCE" | "BY" "CONTENT" | "BY" "VALUE" ] operand } ]
      [ "RETURNING" identifier ] [ "END-CALL" ]

stop-statement   = "STOP" "RUN" | "STOP" literal
goback-statement = "GOBACK"
exit-statement   = "EXIT" [ "PROGRAM" | "PARAGRAPH" | "SECTION" | "PERFORM" ]
go-to-statement  = "GO" [ "TO" ] procedure-name
                       [ "DEPENDING" [ "ON" ] identifier ]
continue-statement = "CONTINUE"

on-size-error = [ "ON" ] "SIZE" "ERROR" { statement }
              [ "NOT" [ "ON" ] "SIZE" "ERROR" { statement } ]

open-statement   = "OPEN" { ( "INPUT" | "OUTPUT" | "I-O" | "EXTEND" )
                            { file-name [ "REVERSED" | [ "WITH" ] "NO" "REWIND" ] } }
close-statement  = "CLOSE" { file-name [ [ "WITH" ] ( "LOCK" | "NO" "REWIND" )
                                       | "FOR" "REMOVAL" ] }
read-statement   = "READ" file-name [ "NEXT" | "PREVIOUS" ] [ "RECORD" ]
                       [ "INTO" identifier ] [ "KEY" [ "IS" ] identifier ]
                       [ at-end | invalid-key ] [ "END-READ" ]
write-statement  = "WRITE" record-name [ "FROM" identifier ]
                       [ ( "BEFORE" | "AFTER" ) [ "ADVANCING" ]
                         ( ( identifier | integer ) [ "LINE" | "LINES" ] | "PAGE" ) ]
                       [ end-of-page | invalid-key ] [ "END-WRITE" ]
rewrite-statement = "REWRITE" record-name [ "FROM" identifier ]
                       [ invalid-key ] [ "END-REWRITE" ]
delete-statement = "DELETE" file-name [ "RECORD" ]
                       [ invalid-key ] [ "END-DELETE" ]
start-statement  = "START" file-name
                       [ "KEY" [ "IS" ] relational-operator identifier ]
                       [ invalid-key ] [ "END-START" ]

initialize-statement = "INITIALIZE" identifier { identifier }
                     [ "WITH" "FILLER" ]
                     [ ( "ALL" | category { category } ) "TO" "VALUE" ]
                     [ "REPLACING" { category [ "DATA" ] "BY" operand } ]
                     [ "DEFAULT" ]
category = "ALPHABETIC" | "ALPHANUMERIC" | "NUMERIC" | "ALPHANUMERIC-EDITED"
         | "NUMERIC-EDITED" | "NATIONAL" | "NATIONAL-EDITED"

set-statement  = "SET"
                     ( "ADDRESS" "OF" identifier { "ADDRESS" "OF" identifier } "TO" set-source
                     | identifier { identifier }
                       ( "TO" set-source | ( "UP" | "DOWN" ) "BY" operand ) )
set-source = operand | "TRUE" | "FALSE" | "ON" | "OFF" | "ADDRESS" "OF" identifier

string-statement = "STRING"
                     { operand { operand } "DELIMITED" [ "BY" ] ( operand | "SIZE" ) }
                     "INTO" identifier [ [ "WITH" ] "POINTER" identifier ]
                     [ overflow ] [ "END-STRING" ]

unstring-statement = "UNSTRING" identifier
                     [ "DELIMITED" [ "BY" ] [ "ALL" ] operand { "OR" [ "ALL" ] operand } ]
                     "INTO" { identifier [ "DELIMITER" [ "IN" ] identifier ]
                                         [ "COUNT" [ "IN" ] identifier ] }
                     [ [ "WITH" ] "POINTER" identifier ] [ "TALLYING" [ "IN" ] identifier ]
                     [ overflow ] [ "END-UNSTRING" ]

inspect-statement = "INSPECT" identifier
                     [ "TALLYING" { identifier "FOR"
                         { ( "CHARACTERS" | ( "ALL" | "LEADING" ) operand ) [ region ] } } ]
                     [ "REPLACING"
                         { ( "CHARACTERS" "BY" operand [ region ]
                           | ( "ALL" | "LEADING" | "FIRST" ) operand "BY" operand [ region ] ) } ]
                     [ "CONVERTING" operand "TO" operand [ region ] ]
region = ( "BEFORE" | "AFTER" ) [ "INITIAL" ] operand

search-statement = "SEARCH" [ "ALL" ] identifier
                     [ "VARYING" identifier ] [ [ "AT" ] "END" { statement } ]
                     { "WHEN" condition ( { statement } | "NEXT" "SENTENCE" ) }
                     [ "END-SEARCH" ]
        « SEARCH ALL admits exactly one WHEN, and its condition must be an
          AND-conjunction of equality ("=") or condition-name tests »

                                          « shared file I/O exception handlers »
at-end       = [ "AT" ] "END" { statement }
                 [ "NOT" [ "AT" ] "END" { statement } ]
invalid-key  = "INVALID" [ "KEY" ] { statement }
                 [ "NOT" "INVALID" [ "KEY" ] { statement } ]
end-of-page  = [ "AT" ] ( "END-OF-PAGE" | "EOP" ) { statement }
                 [ "NOT" [ "AT" ] ( "END-OF-PAGE" | "EOP" ) { statement } ]
overflow     = [ "ON" ] "OVERFLOW" { statement }
                 [ "NOT" [ "ON" ] "OVERFLOW" { statement } ]
```

The data-manipulation verbs above now include the previously deferred sub-phrases:
`INSPECT … CONVERTING`; the SET pointer / `ADDRESS OF` and `ON`/`OFF` switch forms;
`INITIALIZE … REPLACING` / `DEFAULT` / `WITH FILLER` / `… TO VALUE`; and the
`SEARCH ALL` semantic constraint (a single WHEN of AND-joined equality or
condition-name tests).

```ebnf
operand    = identifier | literal           « literal includes figurative-constant »
identifier = qualified-name [ subscript ] [ reference-modifier ]
qualified-name   = data-name { ( "IN" | "OF" ) data-name }
subscript        = "(" operand { operand } ")"
        « operands are space-separated; an optional separator comma or semicolon
          — each requiring a following space — may appear between them and is
          consumed by the tokenizer as a separator »
reference-modifier = "(" arithmetic-expression ":" [ arithmetic-expression ] ")"

condition =                                 « precedence: NOT > AND > OR »
        and-condition { "OR" and-condition }
and-condition =
        combinable-condition { "AND" combinable-condition }
combinable-condition =
        [ "NOT" ] simple-condition | "(" condition ")"
simple-condition =
        relation-condition | class-condition | condition-name-reference
        | sign-condition
condition-name-reference =
        condition-name { ( "IN" | "OF" ) data-name } [ subscript ]
relation-condition =
        operand relational-operator operand
relational-operator =
        ( [ "IS" ] [ "NOT" ]
          ( ">" | "<" | "=" | ">=" | "<=" | "<>"
          | "GREATER" [ "THAN" ] | "LESS" [ "THAN" ] | "EQUAL" [ "TO" ] ) )
class-condition =
        operand [ "IS" ] [ "NOT" ] ( "NUMERIC" | "ALPHABETIC"
                                   | "ALPHABETIC-LOWER" | "ALPHABETIC-UPPER" )
sign-condition =
        operand [ "IS" ] [ "NOT" ] ( "POSITIVE" | "NEGATIVE" | "ZERO" )

arithmetic-expression =
        term { ( "+" | "-" ) term }
term       = factor { ( "*" | "/" ) factor }
factor     = [ "+" | "-" ] primary { "**" primary }
primary    = operand | "(" arithmetic-expression ")"
```

### Ordering and Optionality

- **Division order is fixed:** ID, then ENVIRONMENT, then DATA, then PROCEDURE.
  Only IDENTIFICATION (with `PROGRAM-ID`) is required; the rest are optional.
- **Section order within a division is fixed** (e.g. CONFIGURATION before
  INPUT-OUTPUT; FILE before WORKING-STORAGE before LOCAL-STORAGE before LINKAGE).
- **Paragraph order within IDENTIFICATION:** `PROGRAM-ID` first; comment
  paragraphs follow in any order.
- **Sentence/period rule:** every paragraph is a list of sentences; every
  sentence and every data/entry/paragraph header ends with a **separator
  period**. Inside the PROCEDURE DIVISION, scope terminators (`END-IF`,
  `END-PERFORM`, …) bound statements *without* a period; a period closes the
  whole sentence. See Semantics for the interaction.
- **Clause order within a data description entry** is largely free; some
  combinations are mutually exclusive (e.g. an item with subordinate items has
  no `PICTURE`).
- **Area A / Area B placement (fixed format only).** The grammar productions are
  unchanged between formats, but in fixed format *where* a construct begins on
  the line is constrained: DIVISION / SECTION / paragraph headers, the `FD` /
  `SD` file and sort descriptions, `DECLARATIVES.` / `END DECLARATIVES.`,
  `END PROGRAM`, and the level numbers `01` and `77` must **begin in Area A**
  (columns 8–11); other level numbers (`02`–`49`, `66`, `88`), all clauses, and
  all statements go in **Area B** (columns 12–72). Free format imposes no such
  constraint. Either way the parsed AST is identical. See
  [Whitespace and Delimiters](#whitespace-and-delimiters).

## Semantics

Meaning rules that shape the AST and must survive a round trip (parse → print →
parse).

- **Case-insensitivity.** Reserved words and user-defined words are matched
  case-insensitively (*GnuCOBOL §2.1.3*). `Move`, `MOVE`, and `move` are the
  same verb; `Cust-Name` and `CUST-NAME` are the same name. The printer chooses
  a canonical case (conventionally upper-case for reserved words); the AST should
  preserve the *identity* of a name, and may preserve its original spelling for
  faithful printing.

- **Figurative-constant length is contextual.** `SPACES` moved into a `PIC X(10)`
  field fills 10 spaces; the same constant in a 3-byte field fills 3. The value
  is fixed but the length is the receiving item's. `ZERO` is the numeric value 0
  in arithmetic and a string of `0` characters in a display context. Figurative
  constants may not be used where a definite length is required
  (*GnuCOBOL §2.1.19.3*).

- **Numeric equivalence.** `1`, `1.0`, `01`, and `+1` denote the same numeric
  value; leading/trailing zeros and an explicit `+` do not change the value.
  Whether the printer normalizes them is a printer policy; the parser should
  capture the literal faithfully enough to round-trip.

- **`DECIMAL-POINT IS COMMA`.** When this clause appears in SPECIAL-NAMES, the
  roles of `.` and `,` swap **inside numeric literals and PICTURE strings**:
  `3,14` becomes the numeric value three-and-fourteen-hundredths and `.` becomes
  the thousands separator. The separator period that ends a sentence is
  unaffected. The tokenizer/parser must honor this mode globally for the source
  unit (it is set in the ENVIRONMENT DIVISION, applied everywhere after).

  > **Ambiguity:** This couples lexing to a clause parsed later in the file.
  > Practical approaches: a two-pass scan, or a tokenizer flag toggled when
  > `DECIMAL-POINT IS COMMA` is seen (it occurs early, in the ENVIRONMENT
  > DIVISION, before any data/procedure literal). Flag and decide per story;
  > default to the single-pass flag toggled on the SPECIAL-NAMES clause.

- **PICTURE determines category.** The set of symbols in a PICTURE string fixes
  the item's category, which governs what may be `MOVE`d into it and how it
  prints:
  - only `9`/`S`/`V`/`P` → **numeric**
  - only `A` → **alphabetic**
  - only `X` (or a mix incl. `X`) → **alphanumeric**
  - `9` plus editing symbols (`Z * , . + - CR DB $ B 0 /`) → **numeric-edited**
  - `X`/`A` plus `B`/`0`/`/` → **alphanumeric-edited**

- **`USAGE` default is `DISPLAY`.** An item with no `USAGE` clause is `DISPLAY`
  (character) representation; `COMP`/`BINARY`/`PACKED-DECIMAL` change the stored
  encoding but not the logical value. `USAGE` is inherited by subordinate items
  from a group unless overridden.

- **`VALUE` initialization.** A `VALUE` clause sets the item's initial content;
  its literal must be compatible with the item's category. On a level-88 entry,
  `VALUE` lists the value(s) (or range via `THROUGH`) that make the
  condition-name true.

- **Period vs scope terminator (procedure division).** A separator period ends a
  *sentence* and closes every open inline scope. Explicit scope terminators
  (`END-IF`, `END-PERFORM`, `END-EVALUATE`, …) close exactly one statement
  without ending the sentence, allowing several statements to share one sentence.
  A conditional statement (e.g. `IF` without `END-IF`) extends to the next
  period. The AST should record scope explicitly so the printer can reproduce
  either style.

- **Reference format independence.** The parsed AST carries **no column
  information** and is identical whichever reference format the source used — the
  source format is a lexical property, not a syntactic one. In free format, line
  breaks and indentation are insignificant beyond separating tokens, so the
  printer is free to choose layout (this is what makes free-format round-trips
  robust). Fixed format adds column significance (Area A / Area B, the indicator
  area, column-7 continuation) on *input*; on *output* the printer must place
  tokens in their required column areas. The source format is therefore detected
  by the tokenizer (#22) and reproduced by the printer, so the round-trip
  property holds **per format**: a fixed-format source prints back as fixed
  format, a free-format source as free format.

## Examples

All three parse under the grammar above and are intended as round-trip fixtures
(parse → print → parse → AST equality).

### Minimal Valid File

The smallest valid program: an IDENTIFICATION DIVISION with a `PROGRAM-ID`. No
executable statements are required.

```cobol
IDENTIFICATION DIVISION.
PROGRAM-ID. minimal.
```

### Typical File

A "hello world": identification plus a procedure division with two statements in
one sentence each. (This mirrors the hello-world round-trip fixture.)

```cobol
IDENTIFICATION DIVISION.
PROGRAM-ID. hello.
PROCEDURE DIVISION.
    DISPLAY "Hello, world!".
    STOP RUN.
```

### Complex File

Exercises all four divisions, a `>>SOURCE FORMAT IS FREE` directive, comments,
the ENVIRONMENT `SPECIAL-NAMES` paragraph, DATA DIVISION entries with
PICTURE strings, level-numbers including a level-88 condition-name, a figurative
constant, a literal continued with `&`, a `PERFORM … VARYING` loop, an `IF` with
an explicit `END-IF`, and a `COMPUTE`.

```cobol
>>SOURCE FORMAT IS FREE
*> Free-format demonstration program.
IDENTIFICATION DIVISION.
PROGRAM-ID. demo.
AUTHOR. cobol-go.

ENVIRONMENT DIVISION.
CONFIGURATION SECTION.
SPECIAL-NAMES.
    CURRENCY SIGN IS "$".

DATA DIVISION.
WORKING-STORAGE SECTION.
01  COUNTER        PIC 9(2) VALUE ZERO.
01  TOTAL          PIC S9(5)V99 VALUE 0.
01  REPORT-LINE    PIC X(40) VALUE "Counting up "
                            & "to the limit.".
01  STATUS-FLAG    PIC X VALUE "N".
    88 DONE        VALUE "Y".

PROCEDURE DIVISION.
MAIN-PARAGRAPH.
    DISPLAY REPORT-LINE.
    PERFORM VARYING COUNTER FROM 1 BY 1 UNTIL COUNTER > 5
        COMPUTE TOTAL = TOTAL + COUNTER
        DISPLAY "COUNTER = " COUNTER
    END-PERFORM.
    IF TOTAL > 10 THEN
        DISPLAY "Total exceeds ten."
    ELSE
        DISPLAY "Total is ten or less."
    END-IF.
    STOP RUN.
```

### Fixed-Format Example

The hello-world program in **fixed format**: the sequence area and indicator
(columns 1–7) are blank, the division / paragraph headers and any `01` level
begin in Area A (column 8), and statements sit in Area B (column 12 onward). The
ruler shows column numbers and is not part of the source. (Fixed-format
round-trip fixtures are added in #23; this snippet is illustrative.)

```cobol
----+----1----+----2----+----3----+----4----+----5----+----6----+----7--
       IDENTIFICATION DIVISION.
       PROGRAM-ID. hello.
       PROCEDURE DIVISION.
           DISPLAY "Hello, world!".
           STOP RUN.
```

## Appendix

### Character Encoding

- The COBOL character set comprises the letters `A–Z`/`a–z`, digits `0–9`, the
  space, and the special characters `+ - * / = $ , ; . " ' ( ) > < : & _` (and,
  for PICTURE, the symbols listed above). Within alphanumeric literals **any**
  byte/character is permitted.
- Source is treated as a stream of single-byte characters by default
  (ASCII/UTF-8 compatible for the COBOL character set); `national` (`N"…"`)
  literals introduce multi-byte (e.g. UTF-16) data. The package reads runes via
  a buffered reader, so UTF-8 source is handled; a BOM, if present, is not part
  of any token.
- In **fixed format**, only columns 1–72 of each line are scanned; characters in
  columns 73–80 (and any beyond) are discarded before tokenizing. In **free
  format** the whole line is significant (up to 255 characters).

### Out-of-Scope / Deferred

- **Fixed-format reference format** — the column-oriented layout is
  **specified above** (see
  [Whitespace and Delimiters](#whitespace-and-delimiters) and
  [Line Continuation](#line-continuation)) and its *tokenization* (#21) is
  **implemented** (opt in with `WithFixedFormat()`); only the *round-trip
  fixtures* (#23) remain deferred.
- **Source-format detection / configuration** (honoring `>>SOURCE FORMAT`,
  defaulting) — beyond recognizing the directive token, deferred (#22).
- **COPY / REPLACE** text manipulation (copybooks), **REPLACE** statement, and
  pseudo-text (`== … ==`).
- Full **statement grammar** for every verb; only the core verbs above are
  specified, with the rest named for the keyword table.
- **REPORT SECTION** and **SCREEN SECTION** bodies (headers only).
- **Object-oriented** units (class/method) and **user-defined functions**.
- The complete **reserved-word list** (~1130 words).

### Implementation Notes

- The tokenizer is context-sensitive in two places: **PICTURE strings** (after
  `PIC`/`PICTURE [IS]`) and **comment-entries** (after obsolete ID paragraphs).
  Both are flagged with `> **Ambiguity:**` callouts above.
- `DECIMAL-POINT IS COMMA` couples numeric/PICTURE lexing to an ENVIRONMENT
  clause; resolve with a tokenizer flag set when the clause is seen.
- Greedy multi-character symbols (`**`, `>=`, `<=`, `<>`, `>>`, `*>`) must be
  matched before their single-character prefixes.
- **Fixed format** makes the tokenizer column-aware: it must split each line into
  the sequence / indicator / Area A / Area B / identification areas, act on the
  column-7 indicator (`*` and `/` comment, `D` debug, `-` continuation), ignore
  columns 73+, and apply the column-7 continuation rules. The column-72 boundary
  and the "trailing spaces through column 72 belong to a continued nonnumeric
  literal" rule are the easy things to get wrong. Free format skips all of this.

### Related Standards

- ISO/IEC 1989:2014 / ISO/IEC 1989:2023 — *Programming language COBOL*.
  <https://www.iso.org/standard/51416.html>
- GnuCOBOL Programmer's Guide, Chapter 2 — *COBOL Fundamentals*.
  <https://superbol.eu/gnucobol/gnucobpg/chapter2.html>
- GnuCOBOL project documentation. <https://gnucobol.sourceforge.io/>
