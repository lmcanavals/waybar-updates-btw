// Package format provides functions to format package updates into styled Pango markup for status bars.
package format

import (
	"fmt"
	"strings"
)

// UIResult matches the JSON object schema expected by Waybar/Polybar custom modules.
type UIResult struct {
	Text    string `json:"text"`
	Tooltip string `json:"tooltip"`
	Class   string `json:"class"`
	Alt     string `json:"alt"`
}

// AddFormat applies column alignment and Pango markup color coding to a list of update strings in-place.
func AddFormat(updates, colors []string, rawOutput, noColor bool) {
	allParts := make([][]string, len(updates))
	maxNameLen := 0
	maxVersionLen := 0
	var formatStr strings.Builder

	// 1. Parse and find max lengths
	for i, line := range updates {
		allParts[i] = strings.Fields(line)
		// Expected format: "package 1.0 -> 2.0" OR "aur/package 1.0 -> 2.0"
		// Fields should be >= 4 to contain name, v1, ->, v2
		if len(allParts[i]) < 4 {
			continue
		}
		maxNameLen = max(maxNameLen, len(allParts[i][0]))
		maxVersionLen = max(maxVersionLen, len(allParts[i][1]))
	}

	// 2. Format strings
	for i, part := range allParts {
		formatStr.Reset()
		if len(part) < 4 {
			continue
		}

		// The old version is at index 1, new version is at index 3
		oldVer := part[1]
		newVer := part[3]

		fmt.Fprint(&formatStr, "<span font-family='monospace'")
		if !noColor {
			category := ParseVersion(oldVer, newVer)
			if category >= 0 && category < len(colors) {
				fmt.Fprintf(&formatStr, " color='#%s'", colors[category])
			}
		}

		if rawOutput {
			fmt.Fprintf(&formatStr, ">%%s %%s -> %%s</span>")
		} else {
			// Pad name and old version for alignment
			fmt.Fprintf(&formatStr, ">%%-%ds %%-%ds -> %%s</span>", maxNameLen, maxVersionLen)
		}

		updates[i] = fmt.Sprintf(formatStr.String(), part[0], oldVer, newVer)
	}
}
