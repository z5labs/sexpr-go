// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package sexpr provides tools for working with S-expressions.
//
// # Entry points
//
// There are three, layered on each other:
//
//   - [Tokenize] scans a reader into an [iter.Seq2] of [Token] values, keeping
//     the source text of each lexeme. Use it when you want the lexemes and
//     their positions without an AST.
//   - [Parse] builds a [File] from a reader, turning tokens into [Node] values
//     and decoding string escapes and number literals along the way.
//   - [Print] writes a [File] back out as S-expression text. Parsing what
//     [Print] produced yields the same values it was given.
//
// # Grammar
//
// A source is any number of top level datums. A datum is one of:
//
//	symbol      add, x, +, ->list, hello-world
//	number      42, -7, 0.5, -1.5e-3
//	string      "hello", "a\nb", "é"
//	boolean     #t, #true, #f, #false
//	nil         nil
//	list        (), (add 1 2), (a b . c)
//	quoted      'x, `x, ,x, ,@x
//
// A symbol is a run of letters, digits, and the punctuation
// "+-*/<>=!?:$%_&~^@." which is not a number and is not the single character
// ".". Letters include non-ASCII ones, so π is a symbol.
//
// A number is an optional sign, then digits with an optional fraction and an
// optional decimal exponent. Digits are ASCII only. A decimal point must have a
// digit after it, so "1." is not a number, and an exponent must have at least
// one digit. A lexeme which begins like a number but is malformed, such as
// "12abc" or "--1", is an error rather than a symbol. [Parse] yields an [Int]
// for a literal with neither fraction nor exponent and a [Float] otherwise.
//
// A string is delimited by double quotes and may contain the escapes \" \\ \n
// \r \t \b \f and \uXXXX, where X is a hex digit. Any other escape is an error.
// [Tokenize] keeps the escapes as written; [Parse] decodes them into
// [String.Value].
//
// A list is a parenthesized sequence of datums. Writing a "." before the last
// datum makes an improper list, whose tail is held in [List.Tail] rather than
// in [List.Elements]. The empty list "()" is distinct from [Nil].
//
// The reader macros "'", "`", ",", and ",@" each apply to the datum which
// follows, producing a [Quote]. The shorthand is kept rather than rewritten to
// (quote x), so printing reproduces what was written.
//
// Comments come in two forms: ";" runs to the end of the line, and "#| ... |#"
// spans lines and nests. Both are kept, attached to the [File] or [List] which
// encloses them.
//
// Nesting is bounded by [MaxDepth], which [Parse] enforces so a pathological
// input fails with a [MaxDepthExceededError] rather than exhausting the stack.
//
// # Out of scope
//
// This package covers the common core of S-expression syntax, not any one
// Lisp's full reader. None of the following is supported, and each is a syntax
// error rather than a silently accepted extension:
//
//   - "[...]" bracket lists
//   - "#;" datum comments
//   - "#\char" character literals
//   - "#(...)" vectors
//   - "|escaped symbols|"
//   - the full numeric tower: rationals, complex numbers, arbitrary precision
//     integers, and the "#x", "#o", "#b", "#e", and "#i" prefixes. Numbers are
//     decimal, and are an int64 or a float64.
//   - the canonical and base64 transport encodings of the S-expression
//     internet draft
//
// Marshalling between S-expressions and Go structs is also out of scope. This
// package gives you the syntax tree; mapping it onto your own types is yours to
// do.
//
// # Printing layout
//
// [Print] writes each top level datum on its own line.
//
// A list is written on one line, with its elements separated by a single
// space, whenever that line would end at or before [MaxLineWidth]:
//
//	(add 1 2)
//
// When it would run past that, the list breaks onto one element per line. The
// head stays beside the opening parenthesis and the remaining elements are
// indented two spaces past it, so they line up under the head:
//
//	(function-with-a-long-name
//	  argument-one
//	  argument-two)
//
// The indent is measured from the list's own opening parenthesis, so nesting
// compounds it: a list which starts at column 2 indents its elements to 4.
// Only the lists which do not fit are broken, so a short inner list stays on
// one line inside a broken outer one.
//
// The tail of a dotted pair follows the same rule, written on its own line
// after the dot:
//
//	(aaa
//	  bbb
//	  . ccc)
//
// Quote shorthands are reproduced as written, never expanded to (quote x), and
// a broken datum after one is indented past the macro.
//
// An atom is never broken, so a string or symbol longer than [MaxLineWidth]
// simply runs past it.
//
// # Comments
//
// Comments are written back in source order among the datums of whichever
// container holds them, each on its own line:
//
//	; leading
//	(add 1 2)
//	; trailing
//
// A line comment is always followed by a newline, so it can never swallow the
// code after it. A block comment is written exactly as it appeared, delimiters
// included.
//
// Because of that, a list holding any comment is always broken across lines
// even when it would otherwise fit, and a comment written before a list's first
// element pushes that element off the opening line:
//
//	(
//	  ; about a
//	  a
//	  b)
//
// A comment after the last element likewise moves the closing parenthesis onto
// its own line.
package sexpr
