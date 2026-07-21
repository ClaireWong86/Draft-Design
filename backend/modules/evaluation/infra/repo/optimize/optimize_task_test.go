// Copyright (c) 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

package optimize

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

func TestTruncateOptimizeErrorMessage(t *testing.T) {
	t.Run("keeps short message", func(t *testing.T) {
		assert.Equal(t, "model timeout", truncateOptimizeErrorMessage("model timeout"))
	})

	t.Run("truncates by rune without breaking utf8", func(t *testing.T) {
		message := strings.Repeat("错误", optimizeErrorMessageMaxRunes)
		got := truncateOptimizeErrorMessage(message)
		assert.True(t, utf8.ValidString(got))
		assert.Equal(t, optimizeErrorMessageMaxRunes, utf8.RuneCountInString(got))
	})
}
