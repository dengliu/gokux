package logging

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLogger_ValidLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, err := NewLogger(level)
			require.NoError(t, err)
			assert.NotNil(t, logger)
		})
	}
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	_, err := NewLogger("invalid")
	assert.Error(t, err)
}

func TestNewLogger_EmptyLevel(t *testing.T) {
	// zap treats empty string as info level (its zero value).
	logger, err := NewLogger("")
	require.NoError(t, err)
	assert.NotNil(t, logger)
}

func TestNewLogger_CaseInsensitiveLevel(t *testing.T) {
	// zap's UnmarshalText is case-insensitive for level names.
	levels := []string{"DEBUG", "INFO", "WARN", "ERROR"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, err := NewLogger(level)
			require.NoError(t, err)
			assert.NotNil(t, logger)
		})
	}
}
