package logging

import (
	"testing"
)

func TestNewLogger_ValidLevels(t *testing.T) {
	levels := []string{"debug", "info", "warn", "error"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, err := NewLogger(level)
			if err != nil {
				t.Fatalf("NewLogger(%q) returned error: %v", level, err)
			}
			if logger == nil {
				t.Fatalf("NewLogger(%q) returned nil logger", level)
			}
		})
	}
}

func TestNewLogger_InvalidLevel(t *testing.T) {
	_, err := NewLogger("invalid")
	if err == nil {
		t.Fatal("NewLogger(\"invalid\") should return an error")
	}
}

func TestNewLogger_EmptyLevel(t *testing.T) {
	// zap treats empty string as info level (its zero value).
	logger, err := NewLogger("")
	if err != nil {
		t.Fatalf("NewLogger(\"\") returned error: %v", err)
	}
	if logger == nil {
		t.Fatal("NewLogger(\"\") returned nil logger")
	}
}

func TestNewLogger_CaseInsensitiveLevel(t *testing.T) {
	// zap's UnmarshalText is case-insensitive for level names.
	levels := []string{"DEBUG", "INFO", "WARN", "ERROR"}

	for _, level := range levels {
		t.Run(level, func(t *testing.T) {
			logger, err := NewLogger(level)
			if err != nil {
				t.Fatalf("NewLogger(%q) returned error: %v", level, err)
			}
			if logger == nil {
				t.Fatalf("NewLogger(%q) returned nil logger", level)
			}
		})
	}
}
