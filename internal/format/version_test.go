package format_test

import (
	"testing"

	"github.com/lmcanavals/waybar-updates-btw/internal/format"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		oldVer   string
		newVer   string
		expected int
	}{
		{"1.0.0", "2.0.0", 0},     // Major
		{"1.0.0", "1.1.0", 1},     // Minor
		{"1.0.0", "1.0.1", 2},     // Patch
		{"1.0.0-1", "1.0.0-2", 3}, // Revision/pre
	}

	for _, tc := range tests {
		got := format.ParseVersion(tc.oldVer, tc.newVer)
		if got != tc.expected {
			t.Errorf("ParseVersion(%q, %q) = %d; expected %d", tc.oldVer, tc.newVer, got, tc.expected)
		}
	}
}
