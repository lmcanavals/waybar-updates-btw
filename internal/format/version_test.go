package format_test

import (
	"testing"

	"github.com/lmcanavals/waybar-updates-btw/internal/format"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		old, new  string
		unchanged string
		changed   string
		level     string
	}{
		// Rows from the reference example (libheif corrected to "patch").
		{"2:2.4.19-1", "2:2.4.20-1", "2:2.4.", "20-1", "patch"},
		{"14.5.1-1", "14.6.0-1", "14.", "6.0-1", "minor"},
		{"0.411-1", "0.412-1", "0.", "412-1", "minor"},
		{"1.20.0-1", "1.21.0-1", "1.", "21.0-1", "minor"},
		{"4.3.0-1", "4.3.2-1", "4.3.", "2-1", "patch"},
		{"0.65.1-1", "0.66.0-1", "0.", "66.0-1", "minor"},
		{"2.2.1.r23.gdee3b387-1", "2.2.1.r28.gce65eb74-1", "2.2.1.", "r28.gce65eb74-1", "other"},
		{"1.23.5-1", "1.23.6-1", "1.23.", "6-1", "patch"},
		{"5.12.0-1", "5.13.1-1", "5.", "13.1-1", "minor"},
		{"1.8.13-1", "1.8.13-2", "1.8.13-", "2", "pre"},
		{"1.29.1-1", "1.29.2-1", "1.29.", "2-1", "patch"},
		{"2025.1-1", "2026.1-1", "", "2026.1-1", "major"},

		// Edge cases.
		{"1:2.0-1", "2:1.0-1", "", "2:1.0-1", "major"}, // epoch bump
		{"1.0-1", "1:1.0-1", "", "1:1.0-1", "major"},   // epoch added
		{"1.0-1", "1.0.1-1", "1.0", ".1-1", "patch"},   // component appended
		{"1.0", "1.1", "1.", "1", "minor"},             // no pkgrel
		{"9.0-1", "10.0-1", "", "10.0-1", "major"},     // different lengths
		{"v0.3.2.r0-1", "v0.3.3.r0-1", "v0.3.", "3.r0-1", "patch"},
	}

	for _, tc := range tests {
		unchanged, changed, level := format.Classify(tc.old, tc.new)
		if unchanged != tc.unchanged || changed != tc.changed || level.String() != tc.level {
			t.Errorf("Classify(%q, %q) = (%q, %q, %s); want (%q, %q, %s)",
				tc.old, tc.new, unchanged, changed, level, tc.unchanged, tc.changed, tc.level)
		}
		if unchanged+changed != tc.new {
			t.Errorf("Classify(%q, %q): unchanged+changed = %q, want %q", tc.old, tc.new, unchanged+changed, tc.new)
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		oldVer, newVer string
		expected       int
	}{
		{"1.0.0", "2.0.0", 0},     // Major
		{"1.0.0", "1.1.0", 1},     // Minor
		{"1.0.0", "1.0.1", 2},     // Patch
		{"1.0.0-1", "1.0.0-2", 3}, // Pre (pkgrel only)
		{"1.0.0.1", "1.0.0.2", 4}, // Other
	}

	for _, tc := range tests {
		got := format.ParseVersion(tc.oldVer, tc.newVer)
		if got != tc.expected {
			t.Errorf("ParseVersion(%q, %q) = %d; expected %d", tc.oldVer, tc.newVer, got, tc.expected)
		}
	}
}
