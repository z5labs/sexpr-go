# sexpr

A library for working with [S-expressions](https://en.wikipedia.org/wiki/S-expression) in Go.

## Install

```sh
go get github.com/z5labs/sexpr-go
```

## Quickstart

Parse some source into an AST, walk it, and print it back out:

```go
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/z5labs/sexpr-go"
)

func main() {
	f, err := sexpr.Parse(strings.NewReader(`(add 1 2.5 "hi" #t nil)`))
	if err != nil {
		panic(err)
	}

	list := f.Nodes[0].(sexpr.List)
	for _, elem := range list.Elements {
		switch n := elem.(type) {
		case sexpr.Symbol:
			fmt.Printf("symbol %s\n", n.Value)
		case sexpr.Int:
			fmt.Printf("int %d\n", n.Value)
		case sexpr.Float:
			fmt.Printf("float %v\n", n.Value)
		case sexpr.String:
			fmt.Printf("string %q\n", n.Value)
		case sexpr.Bool:
			fmt.Printf("bool %t\n", n.Value)
		case sexpr.Nil:
			fmt.Println("nil")
		}
	}

	if err := sexpr.Print(os.Stdout, f); err != nil {
		panic(err)
	}
}
```

There are three entry points, layered on each other:

| Function                                                    | Purpose                                                                    |
|-------------------------------------------------------------|----------------------------------------------------------------------------|
| `sexpr.Tokenize(r io.Reader) iter.Seq2[sexpr.Token, error]` | Scan a reader into tokens, keeping each lexeme's source text and position. |
| `sexpr.Parse(r io.Reader) (*sexpr.File, error)`             | Build an AST, decoding string escapes and number literals.                 |
| `sexpr.Print(w io.Writer, f *sexpr.File) error`             | Write an AST back out as S-expression text.                                |

`Print` followed by `Parse` gives back the same values, so the two round trip.

Runnable versions of all of these live in [`example_test.go`](./example_test.go).

## Supported grammar

A source is any number of top level datums. A datum is one of:

| Kind    | Examples                       | Node type                        |
|---------|--------------------------------|----------------------------------|
| symbol  | `add`, `x`, `+`, `->list`      | `sexpr.Symbol`                   |
| number  | `42`, `-7`, `0.5`, `-1.5e-3`   | `sexpr.Int` or `sexpr.Float`     |
| string  | `"hello"`, `"a\nb"`            | `sexpr.String`                   |
| boolean | `#t`, `#true`, `#f`, `#false`  | `sexpr.Bool`                     |
| nil     | `nil`                          | `sexpr.Nil`                      |
| list    | `()`, `(add 1 2)`, `(a b . c)` | `sexpr.List`                     |
| quoted  | `'x`, `` `x ``, `,x`, `,@x`    | `sexpr.Quote`                    |

- **Symbols** are runs of letters, digits, and the punctuation `+-*/<>=!?:$%_&~^@.`
  which are neither a number nor the single character `.`. Letters include
  non-ASCII ones, so `π` is a symbol.
- **Numbers** are an optional sign, then digits with an optional fraction and an
  optional decimal exponent. Digits are ASCII only. A decimal point must have a
  digit after it, so `1.` is not a number, and an exponent needs at least one
  digit. A lexeme that starts like a number but is malformed — `12abc`, `--1` —
  is an error rather than a symbol. A literal with neither fraction nor exponent
  parses as an `Int`, everything else as a `Float`.
- **Strings** are double quoted and support the escapes `\"` `\\` `\n` `\r` `\t`
  `\b` `\f` and `\uXXXX`. Any other escape is an error.
- **Lists** may be improper: a `.` before the last datum puts it in `List.Tail`
  instead of `List.Elements`. The empty list `()` is distinct from `nil`.
- **Reader macros** apply to the datum that follows. The shorthand is kept
  rather than rewritten to `(quote x)`, so printing reproduces what was written.
- **Comments** come in two forms — `;` to end of line, and `#| ... |#`, which
  spans lines and nests. Both are kept, attached to the `File` or `List` that
  encloses them.

Nesting is bounded by `MaxDepth`, so a pathological input fails with a
`MaxDepthExceededError` instead of exhausting the stack.

## Out of scope

This package covers the common core of S-expression syntax, not any one Lisp's
full reader. None of the following is supported, and each is a syntax error
rather than a silently accepted extension:

- `[...]` bracket lists
- `#;` datum comments
- `#\char` character literals
- `#(...)` vectors
- `|escaped symbols|`
- the full numeric tower — rationals, complex numbers, arbitrary precision
  integers, and the `#x`, `#o`, `#b`, `#e`, and `#i` prefixes. Numbers are
  decimal, and are an `int64` or a `float64`.
- the canonical and base64 transport encodings of the S-expression internet
  draft

Marshalling between S-expressions and Go structs is also out of scope. This
package gives you the syntax tree; mapping it onto your own types is yours to do.

## License

[MIT](./LICENSE)
