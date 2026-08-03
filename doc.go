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
package sexpr
