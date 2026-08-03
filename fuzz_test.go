// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fuzzSeeds are inputs worth starting from: one per construct, plus the shapes
// which have caused trouble before.
var fuzzSeeds = []string{
	"",
	" ",
	"a",
	"nil",
	"#t",
	"#false",
	"42",
	"-1",
	"1.5",
	"1e10",
	".5",
	`"s"`,
	`"a\nb"`,
	`"aéb"`,
	"()",
	"(a b c)",
	"(a (b c) d)",
	"(a . b)",
	"(a b . c)",
	"'x",
	"`(a ,b ,@c)",
	"; comment",
	"#| block |#",
	"(a ; note\n b)",
	"(λ (x) (* x x))",
	`(日本語 "こんにちは")`,

	// Malformed shapes the tokenizer and parser must reject rather than crash.
	"(",
	")",
	"(a",
	"(.)",
	"(a . )",
	"(a . b . c)",
	"'",
	",@",
	"#",
	"#x",
	"#tx",
	`"unterminated`,
	`"\q"`,
	"#| unterminated",
	"1.2.3",
	"--1",
	"123abc",
	"9223372036854775808",
	"[",
}

func addFuzzSeeds(f *testing.F) {
	f.Helper()

	for _, seed := range fuzzSeeds {
		f.Add(seed)
	}
}

// FuzzTokenize checks that no input makes the tokenizer panic, and that the
// positions it reports only ever move forwards.
func FuzzTokenize(f *testing.F) {
	addFuzzSeeds(f)

	f.Fuzz(func(t *testing.T, src string) {
		var previous Pos
		first := true

		for tok, err := range Tokenize(strings.NewReader(src)) {
			if err != nil {
				// An error ends the stream; the zero token carries no position.
				break
			}

			if !first {
				require.Falsef(
					t,
					lessPos(tok.Pos, previous),
					"position moved backwards: %v came after %v",
					tok.Pos, previous,
				)
			}

			require.Positive(t, tok.Pos.Line, "line must be one based")
			require.Positive(t, tok.Pos.Column, "column must be one based")

			previous = tok.Pos
			first = false
		}
	})
}

// FuzzParse checks that no input makes the parser panic, and that a successful
// parse yields a usable AST rather than something half built.
func FuzzParse(f *testing.F) {
	addFuzzSeeds(f)

	f.Fuzz(func(t *testing.T, src string) {
		file, err := Parse(strings.NewReader(src))
		if err != nil {
			require.Nil(t, file, "a failed parse must not also return a file")
			return
		}

		require.NotNil(t, file)
		requireWellFormed(t, file.Nodes)
	})
}

// requireWellFormed walks an AST checking the invariants the parser promises.
func requireWellFormed(t *testing.T, nodes []Node) {
	t.Helper()

	for _, n := range nodes {
		require.NotNil(t, n, "a parsed AST must not hold a nil node")

		switch node := n.(type) {
		case List:
			// A tail needs something before the dot, which is what makes the
			// list printable.
			if node.Tail != nil {
				require.NotEmpty(t, node.Elements, "a tail with no elements")
				requireWellFormed(t, []Node{node.Tail})
			}
			requireWellFormed(t, node.Elements)
			for _, c := range node.Comments {
				require.NotNil(t, c, "a parsed AST must not hold a nil comment")
			}
		case Quote:
			require.NotNil(t, node.Datum, "a quote must apply to something")
			requireWellFormed(t, []Node{node.Datum})
		case Symbol:
			// The printer writes symbols verbatim, so anything the parser
			// produces has to be writable.
			require.Truef(t, validSymbol(node.Value), "unprintable symbol %q", node.Value)
		}
	}
}

// FuzzRoundTrip checks the property the package really rests on: anything which
// parses can be printed, and printing then reparsing gives the same AST.
func FuzzRoundTrip(f *testing.F) {
	addFuzzSeeds(f)

	f.Fuzz(func(t *testing.T, src string) {
		first, err := Parse(strings.NewReader(src))
		if err != nil {
			return
		}

		for _, c := range first.Comments {
			require.NotNil(t, c, "a parsed AST must not hold a nil comment")
		}

		var out bytes.Buffer
		require.NoError(t, Print(&out, first), "an AST which parsed must print")

		second, err := Parse(strings.NewReader(out.String()))
		require.NoErrorf(t, err, "printed output does not reparse:\n%s", out.String())

		require.Equal(t, withoutPos(first), withoutPos(second), "the AST changed across a round trip")

		// And the output has to be a fixed point.
		var again bytes.Buffer
		require.NoError(t, Print(&again, second))
		require.Equal(t, out.String(), again.String(), "the printer is not stable")
	})
}
