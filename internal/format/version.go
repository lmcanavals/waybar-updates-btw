package format

// ParseVersion counts dots/hyphens before the first difference character between oldVersion and newVersion
// to classify the update category (0: Major, 1: Minor, 2: Patch, 3: Pre-release, 4: Other).
func ParseVersion(oldVersion, newVersion string) int {
	dotCounter := 0
	maxLen := max(len(oldVersion), len(newVersion))

	for i := range maxLen {
		var cOld, cNew byte
		if i < len(oldVersion) {
			cOld = oldVersion[i]
		}
		if i < len(newVersion) {
			cNew = newVersion[i]
		}

		if cNew == '.' || cNew == '-' {
			dotCounter++
		}

		if cNew != cOld {
			break
		}
	}

	if dotCounter > 4 {
		return 4 // colorOther
	}
	return dotCounter
}
