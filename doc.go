// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

// Package sexpr provides tools for working with S-expressions.
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
