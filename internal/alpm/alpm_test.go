package alpm_test

import (
	"testing"

	"github.com/lmcanavals/waybar-updates-btw/internal/alpm"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int // -1, 0, 1
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.10.0", "1.9.0", 1},
		{"1.9.0", "1.10.0", -1},
		{"1.0a", "1.0", -1},
		{"1.0alpha", "1.0beta", -1},
		{"2:1.0", "1:2.0", 1},
		{"1:2.0", "2:1.0", -1},
		{"1.0-2", "1.0-1", 1},
		{"1.0-1", "1.0-1", 0},
		{"1.0-1", "1.0", 0},
		{"1.0.1", "1.0", 1},
		{"1.0", "1.0.1", -1},
		{"2.0.r10", "2.0.r9", 1},
		{"v0.3.2.r0", "v0.3.1.r0", 1},
	}

	for _, tc := range tests {
		got := alpm.Compare(tc.v1, tc.v2)
		normGot := 0
		if got > 0 {
			normGot = 1
		} else if got < 0 {
			normGot = -1
		}

		if normGot != tc.expected {
			t.Errorf("Compare(%q, %q) = %d; expected %d", tc.v1, tc.v2, normGot, tc.expected)
		}
	}
}
