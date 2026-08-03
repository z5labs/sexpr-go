// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata")

// withoutPos returns a copy of f with every position cleared.
//
// Printing normalizes layout, so a reparse lands its nodes at different
// positions than the source did. Everything else must match exactly.
func withoutPos(f *File) *File {
	return &File{
		Nodes:    nodesWithoutPos(f.Nodes),
		Comments: commentsWithoutPos(f.Comments),
	}
}

func nodesWithoutPos(nodes []Node) []Node {
	if nodes == nil {
		return nil
	}

	out := make([]Node, len(nodes))
	for i, n := range nodes {
		out[i] = nodeWithoutPos(n)
	}
	return out
}

func commentsWithoutPos(comments []*Comment) []*Comment {
	if comments == nil {
		return nil
	}

	out := make([]*Comment, len(comments))
	for i, c := range comments {
		out[i] = &Comment{Text: c.Text}
	}
	return out
}

func nodeWithoutPos(n Node) Node {
	switch node := n.(type) {
	case Symbol:
		node.Pos = Pos{}
		return node
	case String:
		node.Pos = Pos{}
		return node
	case Int:
		node.Pos = Pos{}
		return node
	case Float:
		node.Pos = Pos{}
		return node
	case Bool:
		node.Pos = Pos{}
		return node
	case Nil:
		node.Pos = Pos{}
		return node
	case List:
		node.Pos = Pos{}
		node.Elements = nodesWithoutPos(node.Elements)
		if node.Tail != nil {
			node.Tail = nodeWithoutPos(node.Tail)
		}
		node.Comments = commentsWithoutPos(node.Comments)
		return node
	case Quote:
		node.Pos = Pos{}
		node.Datum = nodeWithoutPos(node.Datum)
		return node
	default:
		return n
	}
}

// requireRoundTrip parses src, prints it, and parses that back, requiring the
// two ASTs to agree and the printer to be stable. It returns the printed text.
func requireRoundTrip(t *testing.T, src string) string {
	t.Helper()

	first, err := Parse(strings.NewReader(src))
	require.NoError(t, err, "parsing the source")

	var firstOut bytes.Buffer
	require.NoError(t, Print(&firstOut, first), "printing the first AST")

	second, err := Parse(strings.NewReader(firstOut.String()))
	require.NoErrorf(t, err, "reparsing the printed output:\n%s", firstOut.String())

	require.Equal(t, withoutPos(first), withoutPos(second), "the AST changed across a round trip")

	// Printing again must land on the same bytes, or the output is not a fixed
	// point and reformatting a file repeatedly would keep changing it.
	var secondOut bytes.Buffer
	require.NoError(t, Print(&secondOut, second), "printing the second AST")
	require.Equal(t, firstOut.String(), secondOut.String(), "the printer is not stable")

	return firstOut.String()
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		src  string
	}{
		{name: "empty input", src: ""},
		{name: "only whitespace", src: "  \n\t "},

		{name: "a symbol", src: "add"},
		{name: "a punctuation symbol", src: "->list"},
		{name: "sign symbols", src: "(- + ... a.b)"},
		{name: "a string", src: `"hello"`},
		{name: "an empty string", src: `""`},
		{name: "integers", src: "0 42 -1 +42"},
		{name: "the integer bounds", src: "9223372036854775807 -9223372036854775808"},
		{name: "floats", src: "1.5 -0.5 .5 -.5 0.0"},
		{name: "floats with exponents", src: "1e10 1.5e-3 1E+2"},
		{name: "booleans", src: "#t #true #f #false"},
		{name: "nil", src: "nil"},
		{name: "every atom kind together", src: `(sym "str" 1 1.5 #t nil)`},

		{name: "an empty list", src: "()"},
		{name: "a list of atoms", src: "(a b c)"},
		{name: "nested lists", src: "(a (b c) d)"},
		{name: "lists nested only in lists", src: "((((()))))"},
		{name: "several top level lists", src: "(a) (b) (c)"},

		{name: "a dotted pair", src: "(a . b)"},
		{name: "an improper list", src: "(a b . c)"},
		{name: "nested dotted pairs", src: "((a . b) . (c . d))"},
		{name: "a dotted pair ending in nil", src: "(a . nil)"},

		{name: "quote", src: "'x"},
		{name: "quasiquote", src: "`x"},
		{name: "unquote", src: ",x"},
		{name: "unquote splicing", src: ",@x"},
		{name: "stacked quotes", src: "''x"},
		{name: "a quasiquoted template", src: "`(a ,b ,@c)"},
		{name: "a quoted empty list", src: "'()"},
		{name: "a quoted dotted pair", src: "'(a . b)"},

		{name: "a line comment alone", src: "; only"},
		{name: "a block comment alone", src: "#| only |#"},
		{name: "comments around a datum", src: "; lead\na ; trail"},
		{name: "comments inside a list", src: "(a ; note\n b)"},
		{name: "a comment before the first element", src: "(; first\n a b)"},
		{name: "a comment after the last element", src: "(a b ; last\n)"},
		{name: "comments on nested lists", src: "((a ; inner\n) ; outer\n)"},
		{name: "a comment before a tail", src: "(a . ; before\n b)"},
		{name: "both comment styles", src: "; line\n#| block |#\na"},
		{name: "a multi-line block comment", src: "#| one\n   two |#\na"},

		{name: "unicode symbols", src: "(λ (x) (* x x))"},
		{name: "unicode strings", src: `"こんにちは" "français" "🎉"`},
		{name: "unicode in both", src: `(日本語 "こんにちは")`},
		{name: "string escapes", src: `"a\tb" "c\"d" "e\\f" "g\nh"`},

		{name: "a wrapped list", src: "(a-very-long-function-name-that-forces-wrapping first-argument second-argument third)"},
		{name: "a wrapped nested list", src: "(outer (inner aaaaaaaaaaaaaaaaaaaaaa bbbbbbbbbbbbbbbbbbbbbb cccccccccccccccccccccc) tail)"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			requireRoundTrip(t, tc.src)
		})
	}
}

func TestRoundTripGoldenFiles(t *testing.T) {
	t.Parallel()

	sources, err := filepath.Glob(filepath.Join("testdata", "*.sexpr"))
	require.NoError(t, err)
	require.NotEmpty(t, sources, "no golden sources found")

	for _, source := range sources {
		t.Run(filepath.Base(source), func(t *testing.T) {
			t.Parallel()

			raw, err := os.ReadFile(source)
			require.NoError(t, err)

			printed := requireRoundTrip(t, string(raw))

			// The formatted output is pinned as well, so a change in layout has
			// to be a deliberate one. Regenerate with:
			//
			//	go test -run TestRoundTripGoldenFiles -update
			golden := strings.TrimSuffix(source, ".sexpr") + ".golden"

			if *update {
				require.NoError(t, os.WriteFile(golden, []byte(printed), 0o644))
				return
			}

			want, err := os.ReadFile(golden)
			require.NoErrorf(t, err, "missing golden file, regenerate with -update")
			require.Equal(t, string(want), printed, "formatted output changed")
		})
	}
}
