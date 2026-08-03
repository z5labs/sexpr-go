// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

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
		pr.fail(fmt.Errorf("sexpr: cannot print a nil File"))
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
		return printNode(f.Nodes[idx], writeThen("\n", printNodes(idx+1)))
	}
}

// printNode writes a single node and continues with the next action.
func printNode(n Node, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		switch node := n.(type) {
		case Symbol:
			pr.write(node.Value)
		case String:
			pr.write(quoteString(node.Value))
		case Int:
			pr.write(strconv.FormatInt(node.Value, 10))
		case Float:
			return printFloat(node, next)
		case Bool:
			if node.Value {
				pr.write("#t")
			} else {
				pr.write("#f")
			}
		case Nil:
			pr.write("nil")
		default:
			// Lists and quote forms are printed in a later story, and a nil node
			// lands here too.
			pr.fail(UnsupportedNodeError{Node: n})
			return nil
		}

		return next
	}
}

func printFloat(node Float, next printerAction) printerAction {
	return func(pr *printer, f *File) printerAction {
		if math.IsInf(node.Value, 0) || math.IsNaN(node.Value) {
			pr.fail(NonFiniteFloatError{Value: node.Value})
			return nil
		}

		pr.write(formatFloat(node.Value))
		return next
	}
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
			// Remaining control characters have no short escape, and an invalid
			// byte would not survive being written raw, so both go out as \uXXXX.
			if r < 0x20 || r == utf8.RuneError {
				fmt.Fprintf(&out, `\u%04X`, r)
				continue
			}
			out.WriteRune(r)
		}
	}
	out.WriteByte('"')

	return out.String()
}
