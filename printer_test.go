// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    *File
		expected string
	}{
		{
			name:     "an empty file",
			input:    &File{},
			expected: "",
		},
		{
			name:     "a symbol",
			input:    &File{Nodes: []Node{Symbol{Value: "add"}}},
			expected: "add\n",
		},
		{
			name:     "a symbol of punctuation",
			input:    &File{Nodes: []Node{Symbol{Value: "->list"}}},
			expected: "->list\n",
		},
		{
			name:     "nil",
			input:    &File{Nodes: []Node{Nil{}}},
			expected: "nil\n",
		},
		{
			name:     "booleans",
			input:    &File{Nodes: []Node{Bool{Value: true}, Bool{Value: false}}},
			expected: "#t\n#f\n",
		},
		{
			name: "integers",
			input: &File{Nodes: []Node{
				Int{Value: 0},
				Int{Value: 42},
				Int{Value: -1},
				Int{Value: math.MaxInt64},
				Int{Value: math.MinInt64},
			}},
			expected: "0\n42\n-1\n9223372036854775807\n-9223372036854775808\n",
		},
		{
			name: "floats",
			input: &File{Nodes: []Node{
				Float{Value: 1.5},
				Float{Value: -0.5},
				Float{Value: 0},
			}},
			expected: "1.5\n-0.5\n0.0\n",
		},
		{
			name:     "a float with an integral value keeps a fraction",
			input:    &File{Nodes: []Node{Float{Value: 100}}},
			expected: "100.0\n",
		},
		{
			name:     "a float needing an exponent",
			input:    &File{Nodes: []Node{Float{Value: 1e21}}},
			expected: "1e+21\n",
		},
		{
			name:     "a string",
			input:    &File{Nodes: []Node{String{Value: "hello"}}},
			expected: "\"hello\"\n",
		},
		{
			name:     "an empty string",
			input:    &File{Nodes: []Node{String{Value: ""}}},
			expected: "\"\"\n",
		},
		{
			name:     "a string needing escapes",
			input:    &File{Nodes: []Node{String{Value: "a\"b\\c\nd\te"}}},
			expected: `"a\"b\\c\nd\te"` + "\n",
		},
		{
			name:     "a string with the remaining short escapes",
			input:    &File{Nodes: []Node{String{Value: "\r\b\f"}}},
			expected: `"\r\b\f"` + "\n",
		},
		{
			name:     "a string with a control character",
			input:    &File{Nodes: []Node{String{Value: "a\x00b\x1fc"}}},
			expected: `"a\u0000b\u001Fc"` + "\n",
		},
		{
			name:     "a string with non-ASCII text",
			input:    &File{Nodes: []Node{String{Value: "héllo"}}},
			expected: "\"héllo\"\n",
		},
		{
			name: "several datums of mixed kinds",
			input: &File{Nodes: []Node{
				Symbol{Value: "a"},
				Int{Value: 1},
				Float{Value: 1.5},
				String{Value: "s"},
				Bool{Value: true},
				Nil{},
			}},
			expected: "a\n1\n1.5\n\"s\"\n#t\nnil\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := Print(&buf, tc.input)

			require.NoError(t, err)
			require.Equal(t, tc.expected, buf.String())
		})
	}
}

// printAndReparse writes nodes out and reads them straight back, which is the
// property that actually matters for the printer.
func printAndReparse(t *testing.T, nodes []Node) []Node {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, Print(&buf, &File{Nodes: nodes}))

	file, err := Parse(strings.NewReader(buf.String()))
	require.NoErrorf(t, err, "reparsing %q", buf.String())

	return file.Nodes
}

func TestPrintReparses(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		nodes []Node
	}{
		{name: "a symbol", nodes: []Node{Symbol{Value: "add"}}},
		{name: "a punctuation symbol", nodes: []Node{Symbol{Value: "->x2"}}},
		{name: "a sign symbol", nodes: []Node{Symbol{Value: "-"}}},
		{name: "an ellipsis symbol", nodes: []Node{Symbol{Value: "..."}}},
		{name: "a dotted symbol", nodes: []Node{Symbol{Value: "a.b"}}},
		{name: "nil", nodes: []Node{Nil{}}},
		{name: "booleans", nodes: []Node{Bool{Value: true}, Bool{Value: false}}},
		{name: "zero", nodes: []Node{Int{Value: 0}}},
		{name: "a negative integer", nodes: []Node{Int{Value: -42}}},
		{name: "the integer bounds", nodes: []Node{Int{Value: math.MaxInt64}, Int{Value: math.MinInt64}}},
		{name: "a simple float", nodes: []Node{Float{Value: 1.5}}},
		{name: "an integral float", nodes: []Node{Float{Value: 100}}},
		{name: "a negative zero float", nodes: []Node{Float{Value: math.Copysign(0, -1)}}},
		{name: "a tiny float", nodes: []Node{Float{Value: 5e-324}}},
		{name: "a huge float", nodes: []Node{Float{Value: math.MaxFloat64}}},
		{name: "a float needing many digits", nodes: []Node{Float{Value: 0.1 + 0.2}}},
		{name: "an empty string", nodes: []Node{String{Value: ""}}},
		{name: "a plain string", nodes: []Node{String{Value: "hello"}}},
		{name: "a string with a quote", nodes: []Node{String{Value: `a"b`}}},
		{name: "a string with a backslash", nodes: []Node{String{Value: `a\b`}}},
		{name: "a string with every short escape", nodes: []Node{String{Value: "\"\\\n\r\t\b\f"}}},
		{name: "a string with control characters", nodes: []Node{String{Value: "\x00\x01\x1f"}}},
		{name: "a string with non-ASCII text", nodes: []Node{String{Value: "héllo ꯍ"}}},
		{name: "a string which looks like a form", nodes: []Node{String{Value: "(a . b) ; c"}}},
		{name: "several datums", nodes: []Node{Symbol{Value: "a"}, Int{Value: 1}, String{Value: "s"}, Bool{Value: false}, Nil{}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := printAndReparse(t, tc.nodes)

			require.Len(t, got, len(tc.nodes))
			for i := range tc.nodes {
				// Positions come from wherever the text landed, so compare the
				// kind and the value rather than the whole node.
				require.IsTypef(t, tc.nodes[i], got[i], "node %d changed type", i)
				require.Equal(t, valueOf(tc.nodes[i]), valueOf(got[i]), "node %d changed value", i)
			}
		})
	}
}

// valueOf strips the position from an atom so two nodes can be compared by what
// they mean rather than where they were written.
func valueOf(n Node) any {
	switch node := n.(type) {
	case Symbol:
		return node.Value
	case String:
		return node.Value
	case Int:
		return node.Value
	case Float:
		return node.Value
	case Bool:
		return node.Value
	case Nil:
		return nil
	default:
		return n
	}
}

// errWriter fails every write.
type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) {
	return 0, w.err
}

// shortWriter accepts n writes and then fails.
type shortWriter struct {
	remaining int
	err       error
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, w.err
	}
	w.remaining--
	return len(p), nil
}

func TestPrintErrors(t *testing.T) {
	t.Parallel()

	t.Run("will return a writer failure", func(t *testing.T) {
		t.Parallel()

		writeErr := errors.New("boom")

		err := Print(errWriter{err: writeErr}, &File{Nodes: []Node{Symbol{Value: "a"}}})

		require.ErrorIs(t, err, writeErr)
	})

	t.Run("will keep the first writer failure", func(t *testing.T) {
		t.Parallel()

		writeErr := errors.New("boom")
		w := &shortWriter{remaining: 1, err: writeErr}

		err := Print(w, &File{Nodes: []Node{Symbol{Value: "a"}, Symbol{Value: "b"}}})

		require.ErrorIs(t, err, writeErr)
	})

	t.Run("will stop writing once a failure is recorded", func(t *testing.T) {
		t.Parallel()

		writeErr := errors.New("boom")
		pr := &printer{w: errWriter{err: writeErr}}

		pr.write("first")
		require.ErrorIs(t, pr.err, writeErr)

		// A later failure must not replace the first one.
		pr.fail(errors.New("second"))
		require.ErrorIs(t, pr.err, writeErr)

		// Writing again is a no-op rather than another attempt.
		pr.write("more")
		require.ErrorIs(t, pr.err, writeErr)
	})

	t.Run("will reject a nil file", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := Print(&buf, nil)

		require.Error(t, err)
		require.Empty(t, buf.String())
	})

	t.Run("will reject a nil node", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := Print(&buf, &File{Nodes: []Node{nil}})

		require.Equal(t, UnsupportedNodeError{Node: nil}, err)
	})

	t.Run("will reject a node it cannot yet write", func(t *testing.T) {
		t.Parallel()

		// Lists arrive in a later story.
		var buf bytes.Buffer
		err := Print(&buf, &File{Nodes: []Node{List{}}})

		require.Equal(t, UnsupportedNodeError{Node: List{}}, err)
	})

	t.Run("will reject a non-finite float", func(t *testing.T) {
		t.Parallel()

		for _, v := range []float64{math.Inf(1), math.Inf(-1), math.NaN()} {
			var buf bytes.Buffer
			err := Print(&buf, &File{Nodes: []Node{Float{Value: v}}})

			require.IsType(t, NonFiniteFloatError{}, err)
		}
	})
}

func TestPrinterErrorMessages(t *testing.T) {
	t.Parallel()

	t.Run("will describe a nil node", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "cannot print a nil node", UnsupportedNodeError{}.Error())
	})

	t.Run("will describe an unsupported node", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "cannot print a node of type sexpr.List", UnsupportedNodeError{Node: List{}}.Error())
	})

	t.Run("will describe a non-finite float", func(t *testing.T) {
		t.Parallel()

		require.Equal(t, "cannot print the non-finite float +Inf", NonFiniteFloatError{Value: math.Inf(1)}.Error())
	})
}

func TestPrinterWritef(t *testing.T) {
	t.Parallel()

	t.Run("will write formatted output", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		pr := &printer{w: &buf}

		pr.writef("%s=%d", "n", 42)

		require.NoError(t, pr.err)
		require.Equal(t, "n=42", buf.String())
	})

	t.Run("will do nothing once an error is recorded", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		pr := &printer{w: &buf, err: errors.New("already failed")}

		pr.writef("%s", "ignored")

		require.Empty(t, buf.String())
	})
}

func TestFormatFloat(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value    float64
		expected string
	}{
		{value: 0, expected: "0.0"},
		{value: 100, expected: "100.0"},
		{value: -3, expected: "-3.0"},
		{value: 1.5, expected: "1.5"},
		{value: -0.5, expected: "-0.5"},
		{value: 1e21, expected: "1e+21"},
		{value: 1e-7, expected: "1e-07"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tc.expected, formatFloat(tc.value))
		})
	}
}
