// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxLineWidth is the column past which [Print] breaks a list across lines.
//
// Width is counted in bytes, matching how [Pos] counts columns.
const MaxLineWidth = 80

// TailWithoutElementsError is the error returned by the printer when a [List]
// carries a tail but has no elements.
//
// A dot separates two halves, so there is no S-expression for a tail with
// nothing before it; writing "(. b)" would produce text the parser rejects.
type TailWithoutElementsError struct {
	Pos Pos
}

// Error implements the [error] interface.
func (e TailWithoutElementsError) Error() string {
	return fmt.Sprintf(
		"cannot print a list with a tail but no elements at line %d, column %d",
		e.Pos.Line,
		e.Pos.Column,
	)
}

// ErrNilFile is returned by [Print] when given a nil [File].
var ErrNilFile = errors.New("cannot print a nil file")

// UnsupportedNodeError is the error returned by the printer when it meets a node it cannot write.
type UnsupportedNodeError struct {
	Node Node
}

// Error implements the [error] interface.
func (e UnsupportedNodeError) Error() string {
	if e.Node == nil {
		return "cannot print a nil node"
	}
	return fmt.Sprintf("cannot print a node of type %T", e.Node)
}

// NonFiniteFloatError is the error returned by the printer when a [Float] holds
// an infinity or a NaN.
//
// S-expressions have no syntax for those, so writing one would produce output
// which reads back as a symbol rather than a number.
type NonFiniteFloatError struct {
	Value float64
}

// Error implements the [error] interface.
func (e NonFiniteFloatError) Error() string {
	return fmt.Sprintf("cannot print the non-finite float %v", e.Value)
}

// Print the given [File] to the given writer as S-expression text.
func Print(w io.Writer, f *File) error {
	pr := &printer{w: w}

	for action := printFile; action != nil && pr.err == nil; {
		action = action(pr, f)
	}

	return pr.err
}

type printer struct {
	w   io.Writer
	err error
}

// write appends s to the output, doing nothing once an error has been recorded.
func (pr *printer) write(s string) {
	if pr.err != nil {
		return
	}
	_, pr.err = io.WriteString(pr.w, s)
}

// writef appends formatted output, doing nothing once an error has been recorded.
func (pr *printer) writef(format string, args ...any) {
	if pr.err != nil {
		return
	}
	_, pr.err = fmt.Fprintf(pr.w, format, args...)
}

// fail records err unless one is already recorded, so the first failure is the
// one returned.
func (pr *printer) fail(err error) {
	if pr.err == nil {
		pr.err = err
	}
}

type printerAction func(pr *printer, f *File) printerAction

// writeThen writes a string and continues with the next action.
func writeThen(s string, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		pr.write(s)
		return next
	}
}

func printFile(pr *printer, f *File) printerAction {
	if f == nil {
		pr.fail(ErrNilFile)
		return nil
	}
	return printNodes(0)
}

// printNodes writes each top level datum on its own line.
func printNodes(idx int) printerAction {
	return func(pr *printer, f *File) printerAction {
		if idx >= len(f.Nodes) {
			return nil
		}
		return printNode(f.Nodes[idx], 0, writeThen("\n", printNodes(idx+1)))
	}
}

// printNode writes a single node at the given indent and continues with the
// next action.
func printNode(n Node, indent int, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		pr.writeNode(n, indent, 0)
		if pr.err != nil {
			return nil
		}
		return next
	}
}

// writeNode writes n starting at column indent, breaking it across lines when
// its single line form would run past [MaxLineWidth].
//
// depth bounds the recursion the same way the parser does, since a hand built
// AST can nest arbitrarily deep even though a parsed one cannot.
func (pr *printer) writeNode(n Node, indent, depth int) {
	if pr.err != nil {
		return
	}
	if depth > MaxDepth {
		pr.fail(MaxDepthExceededError{Pos: posOf(n), Depth: MaxDepth})
		return
	}

	line, err := renderInline(n, depth)
	if err != nil {
		pr.fail(err)
		return
	}

	if indent+len(line) <= MaxLineWidth {
		pr.write(line)
		return
	}

	// Only a list has anywhere to break; a quote breaks whatever it applies to.
	switch node := n.(type) {
	case List:
		pr.writeWrappedList(node, indent, depth)
	case Quote:
		macro := quoteMacros[node.Kind]
		pr.write(macro)
		pr.writeNode(node.Datum, indent+len(macro), depth+1)
	default:
		// An atom cannot be broken, so an over-long one simply runs on.
		pr.write(line)
	}
}

// writeWrappedList writes a list one element per line, each indented two spaces
// past the opening parenthesis so that the elements sit under the head.
func (pr *printer) writeWrappedList(list List, indent, depth int) {
	pr.write("(")

	// Two past the '(' column, which nesting then compounds.
	inner := indent + 2
	pad := "\n" + strings.Repeat(" ", inner)

	for i, element := range list.Elements {
		if i == 0 {
			// The head stays on the opening line, just after the parenthesis.
			pr.writeNode(element, indent+1, depth+1)
			continue
		}
		pr.write(pad)
		pr.writeNode(element, inner, depth+1)
	}

	if list.Tail != nil {
		pr.write(pad)
		pr.write(". ")
		pr.writeNode(list.Tail, inner+2, depth+1)
	}

	pr.write(")")
}

// quoteMacros maps a quote kind back to the shorthand it was written with, so
// that printing reproduces the sugar rather than expanding it.
var quoteMacros = map[QuoteKind]string{
	QuoteKindQuote:           "'",
	QuoteKindQuasiquote:      "`",
	QuoteKindUnquote:         ",",
	QuoteKindUnquoteSplicing: ",@",
}

// posOf reports where a node came from, for errors which need a position.
func posOf(n Node) Pos {
	switch node := n.(type) {
	case Symbol:
		return node.Pos
	case String:
		return node.Pos
	case Int:
		return node.Pos
	case Float:
		return node.Pos
	case Bool:
		return node.Pos
	case Nil:
		return node.Pos
	case List:
		return node.Pos
	case Quote:
		return node.Pos
	default:
		return Pos{}
	}
}

// renderInline renders n on a single line.
func renderInline(n Node, depth int) (string, error) {
	var out strings.Builder
	if err := writeInline(&out, n, depth); err != nil {
		return "", err
	}
	return out.String(), nil
}

func writeInline(out *strings.Builder, n Node, depth int) error {
	if depth > MaxDepth {
		return MaxDepthExceededError{Pos: posOf(n), Depth: MaxDepth}
	}

	switch node := n.(type) {
	case Symbol:
		out.WriteString(node.Value)
	case String:
		out.WriteString(quoteString(node.Value))
	case Int:
		out.WriteString(strconv.FormatInt(node.Value, 10))
	case Float:
		if math.IsInf(node.Value, 0) || math.IsNaN(node.Value) {
			return NonFiniteFloatError{Value: node.Value}
		}
		out.WriteString(formatFloat(node.Value))
	case Bool:
		if node.Value {
			out.WriteString("#t")
		} else {
			out.WriteString("#f")
		}
	case Nil:
		out.WriteString("nil")
	case List:
		if node.Tail != nil && len(node.Elements) == 0 {
			return TailWithoutElementsError{Pos: node.Pos}
		}

		out.WriteByte('(')
		for i, element := range node.Elements {
			if i > 0 {
				out.WriteByte(' ')
			}
			if err := writeInline(out, element, depth+1); err != nil {
				return err
			}
		}
		if node.Tail != nil {
			if len(node.Elements) > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(". ")
			if err := writeInline(out, node.Tail, depth+1); err != nil {
				return err
			}
		}
		out.WriteByte(')')
	case Quote:
		macro, ok := quoteMacros[node.Kind]
		if !ok {
			return UnsupportedNodeError{Node: n}
		}
		out.WriteString(macro)
		return writeInline(out, node.Datum, depth+1)
	default:
		// A nil node lands here too.
		return UnsupportedNodeError{Node: n}
	}

	return nil
}

// formatFloat renders v so that reading it back yields the same [Float].
//
// The shortest representation which round trips is used, then a fractional part
// is added when the result would otherwise read back as an integer: 100 must be
// written 100.0, or reparsing gives an [Int].
func formatFloat(v float64) string {
	s := strconv.FormatFloat(v, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

// quoteString renders s as a string literal, escaping whatever must be escaped
// for the tokenizer to read the same text back.
func quoteString(s string) string {
	var out strings.Builder
	out.Grow(len(s) + 2)

	out.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		default:
			// Every other control character has no short escape, and an invalid
			// byte would not survive being written raw, so both go out as \uXXXX.
			// unicode.IsControl covers DEL and the C1 range as well as C0, so
			// none of them reach the output as unprintable bytes.
			if unicode.IsControl(r) || r == utf8.RuneError {
				fmt.Fprintf(&out, `\u%04X`, r)
				continue
			}
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')

	return out.String()
}
