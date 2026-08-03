// Copyright (c) 2026 Z5Labs and Contributors
//
// This software is released under the MIT License.
// https://opensource.org/licenses/MIT

package sexpr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The tokenizer, parser, and printer land in later stories. This smoke test
// keeps the module with a runnable test target and testify wired in as a test
// dependency until real tests replace it.
func TestPackage(t *testing.T) {
	t.Run("will compile and link the test binary", func(t *testing.T) {
		require.NotEmpty(t, t.Name())
	})
}
