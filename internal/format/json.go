package format

import (
	"strings"

	"github.com/lmcanavals/waybar-updates-btw/internal/protocol"
)

// Update is a single package update in the generic JSON output.
type Update struct {
	Name      string `json:"name"`
	Source    string `json:"source"` // "pacman" or "aur"
	Old       string `json:"old"`
	Unchanged string `json:"unchanged"` // leading part of the new version shared with old
	Changed   string `json:"changed"`   // remaining part of the new version
	Level     Level  `json:"level"`
}

// JSONResult is the generic, application-agnostic output of updates-query.
// It mirrors protocol.ResponseData with the update lines parsed into structured entries.
type JSONResult struct {
	Changed   bool     `json:"changed"`
	Version   int64    `json:"version"`
	Updates   []Update `json:"updates"`
	Count     int      `json:"count"`
	Timestamp string   `json:"timestamp"`
}

// ParseUpdate parses a line like "pkg 1.0-1 -> 1.1-1" or "aur/pkg 1.0-1 -> 1.1-1".
// It returns false if the line does not match that shape.
func ParseUpdate(line string) (Update, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 || fields[2] != "->" {
		return Update{}, false
	}

	name, source := fields[0], "pacman"
	if after, ok := strings.CutPrefix(name, "aur/"); ok {
		name, source = after, "aur"
	}

	oldVer, newVer := fields[1], fields[3]
	unchanged, changed, level := Classify(oldVer, newVer)

	return Update{
		Name:      name,
		Source:    source,
		Old:       oldVer,
		Unchanged: unchanged,
		Changed:   changed,
		Level:     level,
	}, true
}

// BuildJSON converts a daemon response into the generic JSON result.
// Malformed update lines are skipped; Updates is never nil so it encodes as [].
func BuildJSON(resp protocol.ResponseData) JSONResult {
	updates := make([]Update, 0, len(resp.Updates))
	for _, line := range resp.Updates {
		if u, ok := ParseUpdate(line); ok {
			updates = append(updates, u)
		}
	}
	return JSONResult{
		Changed:   resp.Changed,
		Version:   resp.Version,
		Updates:   updates,
		Count:     resp.Count,
		Timestamp: resp.Timestamp,
	}
}
