package format

import (
	"strings"
)

// Level classifies how significant a package version change is.
// Its numeric value doubles as the index into the Waybar color palette.
type Level int

const (
	LevelMajor Level = iota // epoch changed, or the first pkgver component changed
	LevelMinor              // second pkgver component changed
	LevelPatch              // third pkgver component changed
	LevelPre                // pkgver identical, only pkgrel changed
	LevelOther              // fourth or later pkgver component changed
)

var levelNames = [...]string{"major", "minor", "patch", "pre", "other"}

// String returns the lowercase name of the level (e.g. "patch").
func (l Level) String() string {
	if l < 0 || int(l) >= len(levelNames) {
		return "other"
	}
	return levelNames[l]
}

// MarshalText makes Level serialize as its string name in JSON.
func (l Level) MarshalText() ([]byte, error) {
	return []byte(l.String()), nil
}

// Classify splits newVersion into the leading part shared with oldVersion and
// the part that changed, and determines the update level.
//
// The version is interpreted as [epoch:]pkgver[-pkgrel]:
//   - epoch changed                          -> major
//   - pkgver changed, 0 dots before change   -> major
//   - pkgver changed, 1 dot before change    -> minor
//   - pkgver changed, 2 dots before change   -> patch
//   - pkgver changed, 3+ dots before change  -> other
//   - pkgver identical, pkgrel changed       -> pre
//
// The split point is the last separator (':', '.', '-') inside the common
// prefix, so "unchanged" always ends on a component boundary.
func Classify(oldVersion, newVersion string) (unchanged, changed string, level Level) {
	if oldVersion == newVersion {
		return newVersion, "", LevelOther
	}

	oldEpoch, oldVer, oldRel := splitEVR(oldVersion)
	newEpoch, newVer, newRel := splitEVR(newVersion)

	switch {
	case oldEpoch != newEpoch:
		// Epoch bump: nothing meaningful is shared.
		return "", newVersion, LevelMajor
	case oldVer == newVer:
		if oldRel != newRel {
			level = LevelPre
		} else {
			level = LevelOther
		}
	default:
		level = levelFromDots(oldVer, newVer)
	}

	cut := splitPoint(oldVersion, newVersion)
	return newVersion[:cut], newVersion[cut:], level
}

// ParseVersion returns the color index for an update from oldVersion to newVersion.
// Kept for the Waybar formatter; see Classify for the classification rules.
func ParseVersion(oldVersion, newVersion string) int {
	_, _, level := Classify(oldVersion, newVersion)
	return int(level)
}

// splitEVR splits [epoch:]pkgver[-pkgrel]. A missing epoch is reported as "0".
func splitEVR(v string) (epoch, ver, rel string) {
	epoch = "0"
	if i := strings.IndexByte(v, ':'); i > 0 && isAllDigits(v[:i]) {
		epoch = strings.TrimLeft(v[:i], "0")
		if epoch == "" {
			epoch = "0"
		}
		v = v[i+1:]
	}
	if i := strings.LastIndexByte(v, '-'); i != -1 {
		return epoch, v[:i], v[i+1:]
	}
	return epoch, v, ""
}

// levelFromDots counts the dots in pkgver that precede the first difference.
func levelFromDots(oldVer, newVer string) Level {
	p := commonPrefixLen(oldVer, newVer)
	dots := strings.Count(newVer[:p], ".")
	// A component appended or removed right at the divergence point
	// (e.g. "1.0" -> "1.0.1") counts as a change at the next level.
	if (p < len(newVer) && newVer[p] == '.') || (p < len(oldVer) && oldVer[p] == '.') {
		dots++
	}
	switch dots {
	case 0:
		return LevelMajor
	case 1:
		return LevelMinor
	case 2:
		return LevelPatch
	default:
		return LevelOther
	}
}

// splitPoint returns the index in newVersion where the changed part starts.
func splitPoint(oldVersion, newVersion string) int {
	p := commonPrefixLen(oldVersion, newVersion)
	// Divergence exactly on a boundary in both strings (e.g. "1.0-1" vs "1.0.1-1"):
	// the whole common prefix is a complete component.
	if p > 0 && atBoundary(oldVersion, p) && atBoundary(newVersion, p) {
		return p
	}
	if i := strings.LastIndexAny(newVersion[:p], ":.-"); i != -1 {
		return i + 1
	}
	return 0
}

func atBoundary(s string, i int) bool {
	return i >= len(s) || isSeparator(s[i])
}

func isSeparator(b byte) bool {
	return b == ':' || b == '.' || b == '-'
}

func commonPrefixLen(a, b string) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}
