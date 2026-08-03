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

// ErrNilComment is returned by [Print] when a container holds a nil [Comment].
var ErrNilComment = errors.New("cannot print a nil comment")

// errNotInlineable reports that a node cannot be written on one line, which is
// a layout decision rather than a failure.
var errNotInlineable = errors.New("node cannot be written inline")

// InvalidSymbolError is the error returned by the printer when a [Symbol]
// holds text which is not a symbol.
//
// Symbols are written verbatim because the format has no way to quote them, so
// a value such as "123" or "a b" would read back as something other than the
// symbol it came from.
type InvalidSymbolError struct {
	// Value comes first because the message leads with it, and because being
	// field-identical to [Symbol] is a coincidence worth not relying on.
	Value string
	Pos   Pos
}

// Error implements the [error] interface.
func (e InvalidSymbolError) Error() string {
	return fmt.Sprintf(
		"cannot print %q as a symbol at line %d, column %d",
		e.Value,
		e.Pos.Line,
		e.Pos.Column,
	)
}

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
	return printNodes(0, 0)
}

// printNodes writes each top level datum on its own line, with the file's
// comments emitted in position order among them.
func printNodes(nodeIdx, commentIdx int) printerAction {
	return func(pr *printer, f *File) printerAction {
		// A comment which sits before the next datum, or after the last one,
		// goes out first.
		if commentIdx < len(f.Comments) {
			comment := f.Comments[commentIdx]
			if comment == nil {
				pr.fail(ErrNilComment)
				return nil
			}
			if nodeIdx >= len(f.Nodes) || lessPos(comment.Pos, posOf(f.Nodes[nodeIdx])) {
				pr.writeComment(comment)
				return printNodes(nodeIdx, commentIdx+1)
			}
		}

		if nodeIdx >= len(f.Nodes) {
			return nil
		}

		return printNode(f.Nodes[nodeIdx], 0, writeThen("\n", printNodes(nodeIdx+1, commentIdx)))
	}
}

// writeComment writes a comment's source text followed by a newline.
//
// The newline is what stops a line comment from swallowing whatever comes
// after it; a block comment gets one too so that every comment occupies its
// own line.
func (pr *printer) writeComment(comment *Comment) {
	pr.write(comment.Text)
	pr.write("\n")
}

// lessPos reports whether a comes before b in the source.
func lessPos(a, b Pos) bool {
	if a.Line != b.Line {
		return a.Line < b.Line
	}
	return a.Column < b.Column
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
	switch {
	case errors.Is(err, errNotInlineable):
		// Fall through to the breaking switch below.
	case err != nil:
		pr.fail(err)
		return
	case indent+len(line) <= MaxLineWidth:
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
		if node.Kind == QuoteKindUnquote && datumStartsWithAt(node.Datum) {
			macro += " "
			pr.write(" ")
		}
		pr.writeNode(node.Datum, indent+len(macro), depth+1)
	default:
		// An atom cannot be broken, so an over-long one simply runs on. Only a
		// list ever refuses to inline, so line is always valid here.
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

	comments := list.Comments
	next := 0

	// writeCommentsBefore emits, each on its own line, the comments which were
	// written earlier in the source than pos.
	writeCommentsBefore := func(pos Pos) bool {
		wrote := false
		for next < len(comments) {
			comment := comments[next]
			if comment == nil {
				pr.fail(ErrNilComment)
				return wrote
			}
			if !lessPos(comment.Pos, pos) {
				break
			}
			pr.write(pad)
			pr.write(comment.Text)
			next++
			wrote = true
		}
		return wrote
	}

	for i, element := range list.Elements {
		precededByComment := writeCommentsBefore(posOf(element))

		if i == 0 && !precededByComment {
			// The head stays on the opening line, just after the parenthesis.
			pr.writeNode(element, indent+1, depth+1)
			continue
		}

		pr.write(pad)
		pr.writeNode(element, inner, depth+1)
	}

	if list.Tail != nil {
		writeCommentsBefore(posOf(list.Tail))
		pr.write(pad)
		pr.write(". ")
		pr.writeNode(list.Tail, inner+2, depth+1)
	}

	// Whatever is left sat after the last datum.
	trailing := false
	for next < len(comments) {
		comment := comments[next]
		if comment == nil {
			pr.fail(ErrNilComment)
			return
		}
		pr.write(pad)
		pr.write(comment.Text)
		next++
		trailing = true
	}

	if trailing {
		// The closing parenthesis cannot share a line with a line comment, so
		// it drops to its own line under the opening one.
		pr.write("\n")
		pr.write(strings.Repeat(" ", indent))
	}

	pr.write(")")
}

// datumStartsWithAt reports whether n is written starting with '@'.
//
// Only a symbol can be, since '@' is not the leading character of any other
// node's spelling. It matters because "," followed directly by such a datum
// spells ",@", which reads back as unquote-splicing rather than as an unquote
// of a symbol.
func datumStartsWithAt(n Node) bool {
	symbol, ok := n.(Symbol)
	return ok && strings.HasPrefix(symbol.Value, "@")
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
		if !validSymbol(node.Value) {
			return InvalidSymbolError{Value: node.Value, Pos: node.Pos}
		}
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
		out.WriteString(nilLiteral)
	case List:
		if node.Tail != nil && len(node.Elements) == 0 {
			return TailWithoutElementsError{Pos: node.Pos}
		}
		if len(node.Comments) > 0 {
			// A line comment would swallow the rest of the line, so a list with
			// comments has to be broken across lines whatever its width.
			return errNotInlineable
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
		if node.Kind == QuoteKindUnquote && datumStartsWithAt(node.Datum) {
			// Keep "," and "@" apart so they are not read as one macro.
			out.WriteByte(' ')
		}
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

// validSymbol reports whether s would read back as the same symbol.
//
// The rules are the tokenizer's own: every character must be one a symbol may
// contain, and the lexeme as a whole must classify as a symbol rather than as a
// number or the dotted-pair dot. TestValidSymbolAgreesWithTheTokenizer keeps
// this honest.
func validSymbol(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if !isAtomRune(r) {
			return false
		}
	}

	if classifyAtom([]byte(s)) != TokenSymbol {
		return false
	}

	// The tokenizer calls this a symbol, but the parser reads it as the empty
	// value, so writing it would not give the symbol back.
	return s != nilLiteral
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
