# sexpr Package - Claude Memory

This file documents the coding style and patterns for the S-expression tokenizer,
parser, and printer. The conventions are adapted from
[`z5labs/avro-go/idl`](https://github.com/z5labs/avro-go/tree/main/idl) — mirror
those patterns rather than inventing new ones.

The package lives at the repository root as `package sexpr`.

## License Header

Every `.go` file starts with this header, followed by a blank line:

```go
// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT
```

## File Layout

| File           | Contents                                              |
|----------------|-------------------------------------------------------|
| `doc.go`       | Package doc comment                                   |
| `tokenizer.go` | `Pos`, `Token`, `TokenType`, `Tokenize`, tokenizer actions |
| `parser.go`    | `Node` and its implementations, `File`, `Parse`, parser actions |
| `printer.go`   | `Print`, printer actions                              |

Tests live beside their implementation as `*_test.go`.

## State Machine Pattern

The tokenizer, parser, and printer all use a recursive action function pattern
for clean sequential processing.

### Tokenizer Actions

```go
type tokenizerAction func(t *tokenizer, yield func(Token, error) bool) tokenizerAction
```

- Each action function returns the next action to execute
- Return `nil` to end iteration
- The `yield` function follows Go iterator conventions: return `false` to stop early

### Parser Actions

```go
type parserAction[T any] func(p *parser, t T) (parserAction[T], error)
```

- Generic over the type being built (e.g. `*File`, `*List`)
- Returns both the next action and any error
- Return `(nil, nil)` to complete successfully
- Return `(nil, err)` to terminate with error

### Printer Actions

```go
type printerAction func(pr *printer, f *File) printerAction
```

- Each action takes a `*printer` and `*File`, returns the next action
- Return `nil` to end printing
- The printer accumulates errors in `pr.err`; actions short-circuit when an error is set

## Tokenizer

### Helper Functions

#### `yieldErrorOr(err error, next tokenizerAction) tokenizerAction`

Handles error propagation in the tokenizer chain:

- If `err == nil`, returns `next` action
- If `err == io.ErrUnexpectedEOF`, returns `nil` (clean termination)
- Otherwise, yields the error and returns `next`

Use this after any operation that may fail:

```go
err := t.backup(pos)
return yieldErrorOr(err, tokenizeSexpr)
```

#### `yieldTokenThen(tok Token, next tokenizerAction) tokenizerAction`

Yields a token and continues with the next action:

```go
return yieldTokenThen(
    Token{Pos: pos, Type: TokenLParen, Value: []byte("(")},
    tokenizeSexpr,
)
```

#### `skipWhitespace(next tokenizerAction) tokenizerAction`

Wraps an action to skip leading whitespace before executing:

```go
return skipWhitespace(tokenizeSexpr)
```

### Entry Point Pattern

The main `tokenizeSexpr` function follows this structure:

```go
func tokenizeSexpr(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
    return skipWhitespace(
        func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
            pos := t.pos
            r, err := t.next()

            return yieldErrorOr(
                err,
                func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
                    switch {
                    case r == '(':
                        return tokenizeLParen(pos)
                    case r == ';':
                        return tokenizeLineComment(pos)
                    case r == '"':
                        return tokenizeString(pos)
                    case isAtomRune(r):
                        // Rewind so the scanner sees the whole lexeme.
                        err = t.backup(pos)
                        return yieldErrorOr(err, tokenizeAtom)
                    // ... more cases
                    default:
                        return yieldErrorOr(UnexpectedCharacterError{Pos: pos, R: r}, nil)
                    }
                },
            )
        },
    )
}
```

Key pattern: capture position before reading, then dispatch to a specific tokenizer.

Two dispatch shapes appear here, and the difference matters:

- When the tokenizer only needs the character it already read, pass the captured
  position to a closure — `tokenizeLParen(pos)`.
- When the scanner needs to re-read the lexeme from its first rune, call
  `t.backup(pos)` first and dispatch to a plain `tokenizerAction` which
  re-captures `pos` itself — `tokenizeSymbol`.

### Closure Pattern for Capturing State

When a tokenizer needs to capture state (like the start position), return a closure:

```go
func tokenizeLineComment(pos Pos) tokenizerAction {
    return func(t *tokenizer, yield func(Token, error) bool) tokenizerAction {
        var comment bytes.Buffer
        err := t.copyIf(&comment, func(r rune) bool { return r != '\n' })
        return yieldErrorOr(
            err,
            yieldTokenThen(
                Token{Pos: pos, Type: TokenComment, Value: comment.Bytes()},
                skipWhitespace(tokenizeSexpr),
            ),
        )
    }
}
```

### Reader Helpers

`copyIf(dst *bytes.Buffer, cond func(rune) bool) error` copies runes while
`cond` holds and unreads the first one that fails it — use it for run-based
lexemes such as symbols, line comments, and numbers.

`copyUntil(dst *bytes.Buffer, delim []rune) error` copies runes until the
delimiter sequence is consumed, writing everything before it — use it for
terminated constructs which cannot contain themselves, where the first
delimiter always ends the construct. If the input ends before the delimiter
arrives it flushes what it consumed and returns `io.ErrUnexpectedEOF`, so `dst`
always holds the full lexeme either way.

`copyNested(dst *bytes.Buffer, open, closing []rune) error` copies a construct
whose delimiters nest, counting depth instead of stopping at the first close.
It assumes one opening delimiter has already been consumed and stops once that
one's matching close is. `copyUntil` cannot express this, which is why both
exist — use `copyNested` for `#| ... |#` and anything else that may contain
itself.

All three return `io.ErrUnexpectedEOF` when the input ends, and all three keep
`pos` in step with the reader for every rune they consume, including the runes
which complete a delimiter — token positions after a block comment depend on it.

`yieldErrorOr` treats `io.ErrUnexpectedEOF` as clean termination, so a construct
where running out of input is genuinely an error must intercept it *before*
reaching `yieldErrorOr` and substitute a real error:

```go
err := t.copyNested(&comment, blockCommentOpen, blockCommentClose)
if errors.Is(err, io.ErrUnexpectedEOF) {
    return yieldErrorOr(UnterminatedCommentError{Pos: pos}, nil)
}
```

Conversely, a construct that may legitimately end at EOF — such as a `;`
comment with no trailing newline — yields its token on that branch instead:

```go
if errors.Is(err, io.ErrUnexpectedEOF) {
    return yieldTokenThen(tok, nil)
}
```

### String Literals

The tokenizer validates escape sequences but does not decode them. A
`TokenString` value holds the source text between the quotes, with the quotes
excluded and every escape left as written — `"a\nb"` yields the five characters
`a\nb`, not a string containing a newline. Decoding into `String.Value` is the
parser's job, which keeps the token faithful to the source for the printer.

### Atoms: Scan First, Classify After

Symbols, numbers, and the dotted-pair dot share one character set, so they are
not distinguished at dispatch time. `tokenizeAtom` scans the maximal run of
`isAtomRune` characters and only then decides what the lexeme is:

1. exactly `.` — `TokenDot`
2. `looksNumeric` — `TokenNumber`, or `InvalidNumberError` if it is malformed
3. otherwise — `TokenSymbol`

Two properties fall out of scanning first rather than dispatching on the leading
character:

- `123abc` is one malformed number rather than a number followed by a symbol,
  which is what makes "a number must be followed by a delimiter" hold without a
  separate lookahead check.
- A lone `.` is recognised as the dotted-pair marker simply because the run
  stopped after it, so no lookahead is needed to tell `.` from `.5`, `...`, or
  `a.b`.

`looksNumeric` and `validNumber` are deliberately different tests. `validNumber`
is the grammar. `looksNumeric` is the looser question "was this meant to be a
number", and it is what decides whether a bad lexeme is an error or a symbol:
`--1` is a malformed number, while `->x2` and `-e10` are ordinary symbols. Keep
that distinction in mind before loosening either one.

### Hash Dispatch

`#` never stands alone. `tokenizeHash` reads the character after it and routes:
`|` opens a block comment, `t` and `f` begin a boolean, and anything else is
rejected. Adding a new `#` form means adding a case there.

Two failure modes are deliberately distinguished:

- **Unknown dispatch** (`#x`, `#1`, a trailing `#`) — nothing is known about
  what was intended, so the error is about the whole form and anchors at the
  `#`.
- **Known dispatch with trailing junk** (`#tx`, `#truex`) — the construct is
  identified and the offending characters are, too, so the error anchors at
  where the junk starts.

Booleans scan the whole atom run rather than stopping after `t`, which is what
makes `#tx` an error instead of `#t` followed by the symbol `x`.

### Reader Macros

`'`, `` ` ``, `,`, and `,@` all produce a single `TokenQuote` whose value is the
macro's literal text; the parser tells them apart by that value.

`,@` is the only construct needing lookahead. `tokenizeUnquote` reads one
character past the comma and calls `t.backup` when it is not `@`, so the next
token still sees it. Note `@` is an atom character, which is why the lookahead
has to consume it explicitly rather than letting the atom scanner take it.

The tokenizer stays permissive: a macro with nothing after it is still a
complete token, so a trailing `'` at end of input is not an error. Deciding
whether a datum actually follows belongs to the parser, which is the only layer
that knows what a datum is.

### Naming Note

In `avro-go/idl`, `TokenSymbol` means punctuation. Here "symbol" means the Lisp
atom, so punctuation gets explicit token types instead — `TokenLParen`,
`TokenRParen`, `TokenDot`, `TokenQuote`, and so on.

## Parser

### Iterator Usage

The parser uses `iter.Pull2` to convert the push-based tokenizer into pull-based:

```go
next, stop := iter.Pull2(Tokenize(r))
defer stop()

p := &parser{next: next}
```

### Expect Pattern

Use `p.expect()` to require specific token types:

```go
tok, err := p.expect(TokenSymbol, TokenString)
if err != nil {
    return nil, err
}
```

### Nested Parsing

When parsing nested structures, create a sub-loop:

```go
func parseList(p *parser, f *File) (_ parserAction[*File], err error) {
    list := &List{}
    for action := parseListElement; action != nil && err == nil; {
        action, err = action(p, list)
    }
    return nil, err
}
```

### Atom Decoding Belongs to the Parser

The tokenizer keeps source text; the parser turns it into values. Three
conversions live in `parseDatum` and nowhere else:

- **Strings** — `decodeString` resolves the escapes the tokenizer left raw.
- **Numbers** — the lexeme becomes an `Int` or a `Float` depending on whether
  it carries a fraction or exponent. A value which does not fit reports
  `NumberRangeError`; `InvalidNumberError` is kept for a lexeme which is not a
  number at all, which the tokenizer already rejects.
- **`nil`** — spelled like a symbol, so the tokenizer emits `TokenSymbol` and
  the parser is what turns it into a `Nil` node.

Because the tokenizer validates first, the malformed-input branches in these
conversions are unreachable through `Parse`. They are still implemented and
tested directly rather than left to panic.

### Recursion Is Bounded

Lists parse by recursive descent, so nesting costs stack. `parseDatum` and
`parseList` thread a `depth` through, and `parseList` refuses to go past
`MaxDepth`, returning `MaxDepthExceededError` instead. Any future construct
which can contain a datum — dotted pairs, quote forms — must pass its `depth`
along rather than restarting the count, otherwise the bound stops holding.

The limit is exported so callers can see it, and the tests parse at exactly
`MaxDepth` to show the bound is reachable rather than merely theoretical.

A dotted pair's `Tail` is parsed at the *same* depth as the list's elements,
since it sits inside that list. There is a test which nests exclusively through
tails and expects the bound to still apply, so getting this wrong fails loudly
rather than silently opening a way around the limit.

Reader macros **do** increment the depth, even though `'''x` looks flat rather
than nested. Each macro recurses into `parseDatum` for the datum it applies to,
so a long run of them recurses exactly as a long run of `(` does. Passing the
depth through unchanged there is not a wrong answer, it is a fatal stack
overflow which takes the process down — verified by removing the increment and
watching the runtime abort. Treat "does it recurse" rather than "does it look
nested" as the test for whether a construct owes the budget a level.

### Comments Are Drained, Not Dropped

`parser.advance` returns every token, comments included. Each site which expects
a datum calls `parser.collectComments` first, which drains the comment tokens
sitting there into the enclosing container and leaves the reader on the next
real token.

That container is a `*File` at the top level and a `*List` inside one, so the
comment slice is threaded through `parseDatum`, `parseTail`, and `parseQuote`
rather than being a field on the parser. A comment therefore belongs to the list
that encloses it, not to the node it happens to precede — `((a ; inner\n) ; outer\n)`
puts each comment on a different list.

Appending as they are met is what keeps them in position order; nothing sorts.

Add a `collectComments` call to any new site which reads a token expecting a
datum, or comments there will surface as an `UnexpectedTokenError` instead of
being recorded.

### Sealed Node Interface

`Node` is sealed by an unexported method so that only this package can implement it:

```go
type Node interface {
    sexpr()
}
```

## Printer

### Helper Methods

#### `pr.write(s string)` / `pr.writef(format string, args ...any)`

Write output to the underlying writer. Both no-op if `pr.err` is already set:

```go
pr.write("(")
pr.writef("%d", n.Value)
```

#### `writeThen(s string, next printerAction) printerAction`

Writes a string and returns the next action — the printer equivalent of
`yieldTokenThen`:

```go
return writeThen(")", nil)
```

### Closure Pattern for Capturing State

Same closure pattern as the tokenizer — return a closure when state (like an
index) needs to be captured:

```go
func printNodes(idx int) printerAction {
    return func(pr *printer, f *File) printerAction {
        if idx >= len(f.Nodes) {
            return nil
        }
        return printNode(f.Nodes[idx], writeThen("\n", printNodes(idx+1)))
    }
}
```

### Type Dispatch

Use a type switch on the `Node` interface to determine how to print a value:

```go
func printNode(n Node, next printerAction) printerAction {
    return func(pr *printer, f *File) printerAction {
        switch node := n.(type) {
        case Symbol:
            pr.write(node.Value)
        case Bool:
            // ...
        }
        return next
    }
}
```

### One Renderer, Two Layers

`writeInline` renders any node on a single line and is the only place a node's
text is decided. `writeNode` sits above it: it renders inline, and if the result
fits within `MaxLineWidth` from the current column it writes that, otherwise it
breaks the node across lines and recurses. Adding a node kind means adding one
case to `writeInline`; adding a *breakable* kind means a case in `writeNode` too.

Indent is passed down, never accumulated in the printer, so nesting compounds
naturally: a list breaking at column 2 indents its elements to 4. Only lists
which do not fit are broken, so a short inner list stays inline inside a broken
outer one.

Both `writeInline` and `writeNode` carry a depth and stop at `MaxDepth`. `Parse`
cannot produce anything deeper, but a hand built AST can, and the recursion here
would otherwise overflow the stack exactly as the parser's would.

### Comments Force a Break

A container's comments are merged with its datums by `Pos`, so ordering comes
from the source rather than from a separate list appended at the end.

The constraint which shapes the layout: a line comment runs to end of line, so
writing one inside a single line list would comment out the rest of that list.
`writeInline` therefore refuses a list carrying comments by returning
`errNotInlineable`, a layout signal rather than a failure, and `writeNode`
treats that as "must break". Because the refusal comes from `writeInline`, it
propagates outward — an *outer* list cannot be inlined either if anything nested
inside it carries a comment, which would otherwise drop those comments silently.

### Whatever Is Printed Must Reparse

The printer's real contract is that `Print` then `Parse` gives back the same
values. Two cases need care, and both have mutation-tested coverage:

- **Floats.** `strconv.FormatFloat(v, 'g', -1, 64)` gives the shortest text
  which round trips *as a float64*, but for an integral value it produces
  `100`, which reparses as an `Int`. `formatFloat` appends `.0` when the result
  carries no `.`, `e`, or `E`, so the node type survives too.
- **Strings.** `quoteString` re-escapes `"` and `\` because the tokenizer would
  otherwise read a different string, and the whitespace escapes plus any
  remaining control character as `\uXXXX` so the output stays printable.

Infinities and NaN have no S-expression syntax at all; writing them would
produce text which reparses as a *symbol*, so the printer refuses with
`NonFiniteFloatError` rather than emitting something which silently changes
meaning.

When adding a node kind, check the round trip rather than only the expected
text — a printer test asserting an exact string can pass while the output no
longer parses back.

### Entry Point

The public `Print` function drives the action loop, checking `pr.err` each iteration:

```go
func Print(w io.Writer, f *File) error {
    pr := &printer{w: w}
    for action := printFile; action != nil && pr.err == nil; {
        action = action(pr, f)
    }
    return pr.err
}
```

## Error Types

### Tokenizer Errors

```go
type UnexpectedCharacterError struct {
    Pos Pos
    R   rune
}
```

### Parser Errors

```go
type UnexpectedEndOfTokensError struct {
    Expected []TokenType
    Pos      Pos
}

type UnexpectedTokenError struct {
    Expected []TokenType
    Actual   Token
}
```

## Testing Style

All tests are table-driven, with subtests named as behavioural phrases.

### Tokenizer Tests

Use an `iter.Seq2` collection helper inside tests:

```go
collect := func(seq iter.Seq2[Token, error]) ([]Token, error) {
    tokens := make([]Token, 0, len(tc.expected))
    for item, err := range seq {
        if err != nil {
            return tokens, err
        }
        t.Log(item)
        tokens = append(tokens, item)
    }
    return tokens, nil
}

tokens, err := collect(Tokenize(strings.NewReader(tc.src)))

require.NoError(t, err)
require.Equal(t, tc.expected, tokens)
```

#### Token Test Case Format

Specify exact positions for all tokens:

```go
{
    name: "a simple call form",
    src:  `(add a b)`,
    expected: []Token{
        {Pos: Pos{Line: 1, Column: 1}, Type: TokenLParen, Value: []byte("(")},
        {Pos: Pos{Line: 1, Column: 2}, Type: TokenSymbol, Value: []byte("add")},
        {Pos: Pos{Line: 1, Column: 6}, Type: TokenSymbol, Value: []byte("a")},
        {Pos: Pos{Line: 1, Column: 8}, Type: TokenSymbol, Value: []byte("b")},
        {Pos: Pos{Line: 1, Column: 9}, Type: TokenRParen, Value: []byte(")")},
    },
},
```

### Printer Tests

Test with an explicit `*File` input and an expected string output:

```go
{
    name: "a list of symbols",
    input: &File{
        Nodes: []Node{
            &List{
                Pos: Pos{Line: 1, Column: 1},
                Elements: []Node{
                    Symbol{Pos: Pos{Line: 1, Column: 2}, Value: "add"},
                },
            },
        },
    },
    expected: `(add)`,
},
```

### Round-Trip Tests

Validate Parse → Print → Parse produces equivalent ASTs. Compare semantic
fields rather than positions:

```go
// Parse the original source
file1, err := Parse(strings.NewReader(tc.src))
require.NoError(t, err)

// Print the parsed AST
var buf bytes.Buffer
err = Print(&buf, file1)
require.NoError(t, err)

// Parse the printed output
file2, err := Parse(strings.NewReader(buf.String()))
require.NoError(t, err)

// Compare semantic fields, ignoring positions
require.Equal(t, len(file1.Nodes), len(file2.Nodes))
```

## Verification

Before opening a pull request, all of these must pass:

```sh
go build ./...
go vet ./...
go test -race ./...
gofmt -l .
```
