package transformer

import (
	"regexp"
	"testing"

	"codeberg.org/lexicore/lexicore/pkg/source"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegexSanitizer_Apply(t *testing.T) {
	sanitizer := &RegexSanitizer{
		field:   "username",
		pattern: mustCompile(t, `^(.+)@.+$`),
		replace: "$1",
	}

	identity := source.Identity{
		Username: "user@example.com",
	}

	err := sanitizer.Apply(&identity)
	require.NoError(t, err)
	assert.Equal(t, "user", identity.Username)
}

func TestNormalizeSanitizer_Apply(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		input     string
		want      string
	}{
		{
			name:      "lowercase",
			operation: "lowercase",
			input:     "USER@EXAMPLE.COM",
			want:      "user@example.com",
		},
		{
			name:      "uppercase",
			operation: "uppercase",
			input:     "user@example.com",
			want:      "USER@EXAMPLE.COM",
		},
		{
			name:      "trim",
			operation: "trim",
			input:     "  user@example.com  ",
			want:      "user@example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sanitizer := &NormalizeSanitizer{
				field:     "email",
				operation: tt.operation,
			}

			identity := source.Identity{
				Email: tt.input,
			}

			err := sanitizer.Apply(&identity)
			require.NoError(t, err)
			assert.Equal(t, tt.want, identity.Email)
		})
	}
}

func TestComputeSanitizer_Apply(t *testing.T) {
	sanitizer := &ComputeSanitizer{
		target:     "displayEmail",
		expression: "{{username}}@example.com",
	}

	identity := source.Identity{
		Username:   "john",
		Attributes: make(map[string]any),
	}

	err := sanitizer.Apply(&identity)
	require.NoError(t, err)
	assert.Equal(t, "john@example.com", identity.Attributes["displayEmail"])
}

func mustCompile(t *testing.T, pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	require.NoError(t, err)
	return re
}
