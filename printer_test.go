// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"bytes"
	"errors"
	"math"
	"strconv"
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
			name:     "an empty list",
			input:    &File{Nodes: []Node{List{}}},
			expected: "()\n",
		},
		{
			name: "a list of atoms",
			input: &File{Nodes: []Node{List{Elements: []Node{
				Symbol{Value: "a"}, Symbol{Value: "b"}, Symbol{Value: "c"},
			}}}},
			expected: "(a b c)\n",
		},
		{
			name: "a nested list",
			input: &File{Nodes: []Node{List{Elements: []Node{
				Symbol{Value: "a"},
				List{Elements: []Node{Symbol{Value: "b"}, Symbol{Value: "c"}}},
				Symbol{Value: "d"},
			}}}},
			expected: "(a (b c) d)\n",
		},
		{
			name: "a dotted pair",
			input: &File{Nodes: []Node{List{
				Elements: []Node{Symbol{Value: "a"}},
				Tail:     Symbol{Value: "b"},
			}}},
			expected: "(a . b)\n",
		},
		{
			name: "an improper list of several elements",
			input: &File{Nodes: []Node{List{
				Elements: []Node{Symbol{Value: "a"}, Symbol{Value: "b"}},
				Tail:     Symbol{Value: "c"},
			}}},
			expected: "(a b . c)\n",
		},
		{
			name: "every quote shorthand",
			input: &File{Nodes: []Node{
				Quote{Kind: QuoteKindQuote, Datum: Symbol{Value: "w"}},
				Quote{Kind: QuoteKindQuasiquote, Datum: Symbol{Value: "x"}},
				Quote{Kind: QuoteKindUnquote, Datum: Symbol{Value: "y"}},
				Quote{Kind: QuoteKindUnquoteSplicing, Datum: Symbol{Value: "z"}},
			}},
			expected: "'w\n`x\n,y\n,@z\n",
		},
		{
			name: "a quoted list is never expanded",
			input: &File{Nodes: []Node{Quote{Kind: QuoteKindQuote, Datum: List{Elements: []Node{
				Int{Value: 1}, Int{Value: 2},
			}}}}},
			expected: "'(1 2)\n",
		},
		{
			name: "stacked quotes",
			input: &File{Nodes: []Node{Quote{Kind: QuoteKindQuote, Datum: Quote{
				Kind: QuoteKindQuote, Datum: Symbol{Value: "x"},
			}}}},
			expected: "''x\n",
		},
		{
			name: "top level datums are newline separated",
			input: &File{Nodes: []Node{
				List{Elements: []Node{Symbol{Value: "a"}}},
				List{Elements: []Node{Symbol{Value: "b"}}},
			}},
			expected: "(a)\n(b)\n",
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

// symbols builds n symbols of the given width, so a list's single line width is
// easy to reason about.
func symbols(n, width int) []Node {
	out := make([]Node, 0, n)
	for i := range n {
		out = append(out, Symbol{Value: strings.Repeat(string(rune('a'+i)), width)})
	}
	return out
}

// listOfWidth builds a list whose single line form is exactly width columns.
func listOfWidth(width int) List {
	// Seven symbols of eight characters, then a last one padded to fit.
	elements := symbols(7, 8)
	// "(" + 7*8 + 7 separators + " " + ")" is the fixed part.
	fixed := 1 + 7*8 + 7 + 1
	elements = append(elements, Symbol{Value: strings.Repeat("h", width-fixed)})
	return List{Elements: elements}
}

func mustInline(n Node) string {
	s, err := renderInline(n, 0)
	if err != nil {
		panic(err)
	}
	return s
}

func TestListOfWidthIsExact(t *testing.T) {
	t.Parallel()

	// The wrapping cases below only mean what they say if these hold.
	require.Len(t, mustInline(listOfWidth(MaxLineWidth)), MaxLineWidth)
	require.Len(t, mustInline(listOfWidth(MaxLineWidth+1)), MaxLineWidth+1)
}

func TestPrintWrapping(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    *File
		expected string
	}{
		{
			// Widths are asserted below rather than hand counted, so the
			// boundary cases cannot drift into meaning something else.
			name:     "a list which exactly fills the width stays on one line",
			input:    &File{Nodes: []Node{listOfWidth(MaxLineWidth)}},
			expected: mustInline(listOfWidth(MaxLineWidth)) + "\n",
		},
		{
			name:  "a list one column too wide breaks",
			input: &File{Nodes: []Node{listOfWidth(MaxLineWidth + 1)}},
			expected: "(aaaaaaaa\n" +
				"  bbbbbbbb\n" +
				"  cccccccc\n" +
				"  dddddddd\n" +
				"  eeeeeeee\n" +
				"  ffffffff\n" +
				"  gggggggg\n" +
				"  " + strings.Repeat("h", 16) + ")\n",
		},
		{
			name: "a broken list keeps short inner lists on one line",
			input: &File{Nodes: []Node{List{Elements: []Node{
				Symbol{Value: "outer"},
				List{Elements: symbols(3, 22)},
				Symbol{Value: "tail"},
			}}}},
			expected: "(outer\n" +
				"  (" + strings.Repeat("a", 22) + " " + strings.Repeat("b", 22) + " " + strings.Repeat("c", 22) + ")\n" +
				"  tail)\n",
		},
		{
			name: "nested breaks indent cumulatively",
			input: &File{Nodes: []Node{List{Elements: []Node{
				Symbol{Value: "alpha"},
				List{Elements: []Node{
					Symbol{Value: "beta"},
					List{Elements: symbols(3, 30)},
				}},
				Symbol{Value: "omega"},
			}}}},
			expected: "(alpha\n" +
				"  (beta\n" +
				"    (aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
				"      bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n" +
				"      cccccccccccccccccccccccccccccc))\n" +
				"  omega)\n",
		},
		{
			name: "a broken dotted pair puts the tail after the dot",
			input: &File{Nodes: []Node{List{
				Elements: symbols(2, 30),
				Tail:     Symbol{Value: strings.Repeat("c", 30)},
			}}},
			expected: "(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
				"  bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n" +
				"  . cccccccccccccccccccccccccccccc)\n",
		},
		{
			name:  "a broken quoted list indents past the macro",
			input: &File{Nodes: []Node{Quote{Kind: QuoteKindQuote, Datum: List{Elements: symbols(3, 30)}}}},
			expected: "'(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n" +
				"   bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n" +
				"   cccccccccccccccccccccccccccccc)\n",
		},
		{
			name:     "an over-long atom is never broken",
			input:    &File{Nodes: []Node{Symbol{Value: strings.Repeat("x", 100)}}},
			expected: strings.Repeat("x", 100) + "\n",
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

func TestPrintWrappingRespectsTheWidth(t *testing.T) {
	t.Parallel()

	t.Run("will keep every line within the width where it can", func(t *testing.T) {
		t.Parallel()

		// A deep, wide tree whose lines must all fit once broken, except for
		// atoms which are too long to break.
		var elements []Node
		for i := range 12 {
			elements = append(elements, List{Elements: []Node{
				Symbol{Value: strings.Repeat(string(rune('a'+i)), 12)},
				Symbol{Value: strings.Repeat(string(rune('m'+i)), 12)},
			}})
		}

		var buf bytes.Buffer
		require.NoError(t, Print(&buf, &File{Nodes: []Node{List{Elements: elements}}}))

		for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
			require.LessOrEqualf(t, len(line), MaxLineWidth, "line over width: %q", line)
		}
	})
}

func TestPrintComments(t *testing.T) {
	t.Parallel()

	// Driven from source so that the positions which decide ordering are the
	// real ones rather than something hand assigned.
	testCases := []struct {
		name     string
		src      string
		expected string
	}{
		{
			name:     "a file of only line comments",
			src:      "; one\n; two",
			expected: "; one\n; two\n",
		},
		{
			name:     "a file of only a block comment",
			src:      "#| only |#",
			expected: "#| only |#\n",
		},
		{
			name:     "a comment before a datum",
			src:      "; lead\na",
			expected: "; lead\na\n",
		},
		{
			name:     "a comment after a datum",
			src:      "a ; trail",
			expected: "a\n; trail\n",
		},
		{
			name:     "comments on both sides of a datum",
			src:      "; lead\na ; trail",
			expected: "; lead\na\n; trail\n",
		},
		{
			name:     "comments interleaved with several datums",
			src:      "; f1\na ; f2\nb ; f3",
			expected: "; f1\na\n; f2\nb\n; f3\n",
		},
		{
			name:     "a block comment keeps its delimiters",
			src:      "#| b |# a",
			expected: "#| b |#\na\n",
		},
		{
			name:     "both comment styles in one file",
			src:      "; line\n#| block |#\na",
			expected: "; line\n#| block |#\na\n",
		},
		{
			name: "a comment inside a list",
			src:  "(a ; note\n b)",
			expected: "(a\n" +
				"  ; note\n" +
				"  b)\n",
		},
		{
			name: "a comment before a list's first element",
			src:  "(; first\n a b)",
			expected: "(\n" +
				"  ; first\n" +
				"  a\n" +
				"  b)\n",
		},
		{
			name: "a comment after a list's last element",
			src:  "(a b ; last\n)",
			expected: "(a\n" +
				"  b\n" +
				"  ; last\n" +
				")\n",
		},
		{
			name: "a block comment inside a list",
			src:  "(a #| mid |# b)",
			expected: "(a\n" +
				"  #| mid |#\n" +
				"  b)\n",
		},
		{
			name: "a comment before a dotted pair tail",
			src:  "(a . ; before\n b)",
			expected: "(a\n" +
				"  ; before\n" +
				"  . b)\n",
		},
		{
			name: "comments on nested lists stay with their own list",
			src:  "((a ; inner\n) ; outer\n)",
			expected: "((a\n" +
				"   ; inner\n" +
				" )\n" +
				"  ; outer\n" +
				")\n",
		},
		{
			name: "a short list with a comment is still broken",
			src:  "(a ; c\n b)",
			expected: "(a\n" +
				"  ; c\n" +
				"  b)\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := Parse(strings.NewReader(tc.src))
			require.NoError(t, err)

			var buf bytes.Buffer
			require.NoError(t, Print(&buf, file))
			require.Equal(t, tc.expected, buf.String())
		})
	}
}

// countComments totals the comments held anywhere in a file.
func countComments(f *File) int {
	total := len(f.Comments)

	var walk func([]Node)
	walk = func(nodes []Node) {
		for _, n := range nodes {
			switch node := n.(type) {
			case List:
				total += len(node.Comments)
				walk(node.Elements)
				if node.Tail != nil {
					walk([]Node{node.Tail})
				}
			case Quote:
				walk([]Node{node.Datum})
			}
		}
	}
	walk(f.Nodes)

	return total
}

func TestPrintCommentsSurviveAReparse(t *testing.T) {
	t.Parallel()

	sources := []string{
		"; only",
		"; lead\na ; trail",
		"#| b |# a",
		"(a ; note\n b)",
		"(; first\n a b ; last\n)",
		"((a ; inner\n) ; outer\n)",
		"(a . ; before\n b)",
		"; f1\na ; f2\nb ; f3",
		"'(a ; in quoted\n b)",
		"(a #| one |# b #| two |# c)",
	}

	for _, src := range sources {
		t.Run(strconv.Quote(src), func(t *testing.T) {
			t.Parallel()

			first, err := Parse(strings.NewReader(src))
			require.NoError(t, err)

			var buf bytes.Buffer
			require.NoError(t, Print(&buf, first))

			second, err := Parse(strings.NewReader(buf.String()))
			require.NoErrorf(t, err, "printed output does not reparse:\n%s", buf.String())

			require.Equal(t, countComments(first), countComments(second), "comment count changed")
			require.Equal(t, shapeOf(first.Nodes), shapeOf(second.Nodes), "datum tree changed")

			// Text is preserved verbatim, delimiters included.
			require.Equal(t, commentTexts(first), commentTexts(second))
		})
	}
}

// commentTexts collects every comment's text in the order it is stored.
func commentTexts(f *File) []string {
	var out []string
	for _, c := range f.Comments {
		out = append(out, c.Text)
	}

	var walk func([]Node)
	walk = func(nodes []Node) {
		for _, n := range nodes {
			switch node := n.(type) {
			case List:
				for _, c := range node.Comments {
					out = append(out, c.Text)
				}
				walk(node.Elements)
				if node.Tail != nil {
					walk([]Node{node.Tail})
				}
			case Quote:
				walk([]Node{node.Datum})
			}
		}
	}
	walk(f.Nodes)

	return out
}

func TestPrintUnquoteBeforeAtSymbol(t *testing.T) {
	t.Parallel()

	// Found by FuzzRoundTrip. "," written straight against a symbol starting
	// with "@" spells ",@", which reads back as unquote-splicing rather than as
	// an unquote of that symbol, so a space has to separate them.
	testCases := []struct {
		name     string
		src      string
		expected string
	}{
		{name: "the at symbol alone", src: "(, @)", expected: "(, @)\n"},
		{name: "a symbol starting with at", src: "(, @foo)", expected: "(, @foo)\n"},
		{name: "at the top level", src: ", @", expected: ", @\n"},
		{name: "genuine unquote splicing is untouched", src: ",@x", expected: ",@x\n"},
		{name: "another macro needs no space", src: "'@x", expected: "'@x\n"},
		{name: "quasiquote needs no space", src: "`@x", expected: "`@x\n"},
		{name: "an at symbol not after a macro", src: "(@ a)", expected: "(@ a)\n"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file, err := Parse(strings.NewReader(tc.src))
			require.NoError(t, err)

			var buf bytes.Buffer
			require.NoError(t, Print(&buf, file))
			require.Equal(t, tc.expected, buf.String())

			// The point of the space is that this reparses to the same thing.
			again, err := Parse(strings.NewReader(buf.String()))
			require.NoError(t, err)
			require.Equal(t, shapeOf(file.Nodes), shapeOf(again.Nodes))
		})
	}
}

func TestPrintRejectsUnwritableSymbols(t *testing.T) {
	t.Parallel()

	// A symbol is written verbatim, so any value which would read back as
	// something else has to be refused rather than silently changed.
	unwritable := []string{
		"",    // vanishes entirely
		"123", // reads back as an Int
		"1.5", // reads back as a Float
		"nil", // reads back as a Nil
		".",   // reads back as the dotted pair marker
		"a b", // reads back as two symbols
		"(",   // does not parse
		")",   // does not parse
		"a)b", // does not parse
		";x",  // reads back as a comment
		`"q`,  // does not parse
		"#t",  // reads back as a Bool
	}

	for _, value := range unwritable {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			err := Print(&buf, &File{Nodes: []Node{Symbol{Value: value}}})

			require.Equalf(t, InvalidSymbolError{Value: value}, err, "printed %q", buf.String())
		})
	}
}

func TestPrintAcceptsWritableSymbols(t *testing.T) {
	t.Parallel()

	writable := []string{"a", "add", "->list", "-", "+", "...", "a.b", "x2", "-e10", "λ", "日本語", "@", "@foo"}

	for _, value := range writable {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			require.NoError(t, Print(&buf, &File{Nodes: []Node{Symbol{Value: value}}}))

			file, err := Parse(strings.NewReader(buf.String()))
			require.NoError(t, err)
			require.Equal(t, []Node{Symbol{Pos: Pos{Line: 1, Column: 1}, Value: value}}, file.Nodes)
		})
	}
}

// TestValidSymbolAgreesWithTheTokenizer keeps validSymbol from drifting away
// from the rules it is meant to mirror.
func TestValidSymbolAgreesWithTheTokenizer(t *testing.T) {
	t.Parallel()

	candidates := []string{
		"a", "add", "->list", "-", "+", "...", "a.b", "x2", "-e10", "λ", "@", "@foo",
		"", "123", "1.5", "nil", ".", "a b", "(", ")", "a)b", ";x", "#t", "1abc", "--1",
		"'x", ",@", "a\tb", "a\nb", `"q`, "#|c|#",
	}

	for _, value := range candidates {
		t.Run(strconv.Quote(value), func(t *testing.T) {
			t.Parallel()

			// The authority: does tokenizing this text give back exactly one
			// symbol token carrying it, and does the parser keep it a symbol?
			var tokens []Token
			var tokenErr error
			for tok, err := range Tokenize(strings.NewReader(value)) {
				if err != nil {
					tokenErr = err
					break
				}
				tokens = append(tokens, tok)
			}

			roundTrips := tokenErr == nil &&
				len(tokens) == 1 &&
				tokens[0].Type == TokenSymbol &&
				string(tokens[0].Value) == value &&
				value != nilLiteral

			require.Equalf(t, roundTrips, validSymbol(value), "validSymbol disagrees with the tokenizer for %q", value)
		})
	}
}

func TestPrintCommentErrors(t *testing.T) {
	t.Parallel()

	t.Run("will reject a nil comment on a file", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := Print(&buf, &File{Comments: []*Comment{nil}})

		require.ErrorIs(t, err, ErrNilComment)
	})

	t.Run("will reject a nil comment on a list", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := Print(&buf, &File{Nodes: []Node{List{
			Elements: []Node{Symbol{Value: "a"}},
			Comments: []*Comment{nil},
		}}})

		require.ErrorIs(t, err, ErrNilComment)
	})
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
		{name: "a string with DEL and C1 controls", nodes: []Node{String{Value: "\u007f\u0080\u009f"}}},
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
		// Compared as bits so that a lost sign on zero is caught: -0.0 and 0.0
		// are equal under both == and reflect.DeepEqual.
		return math.Float64bits(node.Value)
	case Bool:
		return node.Value
	case Nil:
		return nil
	default:
		return n
	}
}

// unknownNode is a Node the printer has no case for. The interface is sealed,
// so only a test inside the package can produce one.
type unknownNode struct{}

func (unknownNode) sexpr() {}

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

	t.Run("will reject a node type it does not know", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := Print(&buf, &File{Nodes: []Node{unknownNode{}}})

		require.Equal(t, UnsupportedNodeError{Node: unknownNode{}}, err)
	})

	t.Run("will reject an unknown node nested in a list", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := Print(&buf, &File{Nodes: []Node{List{Elements: []Node{unknownNode{}}}}})

		require.Equal(t, UnsupportedNodeError{Node: unknownNode{}}, err)
	})

	t.Run("will reject a quote of an unknown kind", func(t *testing.T) {
		t.Parallel()

		bad := Quote{Kind: QuoteKind(99), Datum: Symbol{Value: "x"}}

		var buf bytes.Buffer
		err := Print(&buf, &File{Nodes: []Node{bad}})

		require.Equal(t, UnsupportedNodeError{Node: bad}, err)
	})

	t.Run("will reject a non-finite float nested in a list", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := Print(&buf, &File{Nodes: []Node{List{Elements: []Node{
			Symbol{Value: "a"},
			Float{Value: math.Inf(1)},
		}}}})

		require.IsType(t, NonFiniteFloatError{}, err)
	})

	t.Run("will reject a list with a tail but no elements", func(t *testing.T) {
		t.Parallel()

		// "(. b)" is not readable syntax, so refusing beats emitting it.
		bad := List{Pos: Pos{Line: 2, Column: 3}, Tail: Symbol{Value: "b"}}

		var buf bytes.Buffer
		err := Print(&buf, &File{Nodes: []Node{bad}})

		require.Equal(t, TailWithoutElementsError{Pos: Pos{Line: 2, Column: 3}}, err)
	})

	t.Run("will reject a dangling tail nested in a list", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer
		err := Print(&buf, &File{Nodes: []Node{List{Elements: []Node{
			List{Tail: Symbol{Value: "b"}},
		}}}})

		require.IsType(t, TailWithoutElementsError{}, err)
	})

	t.Run("will reject an AST nested past the depth limit", func(t *testing.T) {
		t.Parallel()

		// Parse cannot build this, but a caller assembling an AST by hand can,
		// and unbounded recursion here would take the process down.
		node := Node(Symbol{Value: "x"})
		for range MaxDepth + 2 {
			node = List{Elements: []Node{node}}
		}

		var buf bytes.Buffer
		err := Print(&buf, &File{Nodes: []Node{node}})

		require.IsType(t, MaxDepthExceededError{}, err)
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

		require.Equal(t, "cannot print a node of type sexpr.unknownNode", UnsupportedNodeError{Node: unknownNode{}}.Error())
	})

	t.Run("will describe a dangling tail", func(t *testing.T) {
		t.Parallel()

		err := TailWithoutElementsError{Pos: Pos{Line: 4, Column: 7}}

		require.Equal(t, "cannot print a list with a tail but no elements at line 4, column 7", err.Error())
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
