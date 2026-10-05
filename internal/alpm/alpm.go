// Package alpm provides Arch Linux package version comparison adhering to libalpm/vercmp rules.
package alpm

import (
	"strings"
)

// Compare compares two Arch Linux package versions adhering to libalpm/vercmp rules.
// Returns:
//
//	 1 if a > b
//	 0 if a == b
//	-1 if a < b
func Compare(a, b string) int {
	if a == b {
		return 0
	}
	epoch1, ver1, rel1 := parseEVR(a)
	epoch2, ver2, rel2 := parseEVR(b)

	ret := rpmvercmp(epoch1, epoch2)
	if ret != 0 {
		return ret
	}
	ret = rpmvercmp(ver1, ver2)
	if ret != 0 {
		return ret
	}
	if rel1 != "" && rel2 != "" {
		return rpmvercmp(rel1, rel2)
	}
	return 0
}

func parseEVR(evr string) (epoch, ver, rel string) {
	epoch = "0"
	s := evr
	colonIdx := strings.IndexByte(s, ':')
	if colonIdx != -1 {
		allDigits := true
		for i := range colonIdx {
			if s[i] < '0' || s[i] > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			epoch = s[:colonIdx]
			if epoch == "" {
				epoch = "0"
			}
			s = s[colonIdx+1:]
		}
	}

	dashIdx := strings.LastIndexByte(s, '-')
	if dashIdx != -1 {
		ver = s[:dashIdx]
		rel = s[dashIdx+1:]
	} else {
		ver = s
		rel = ""
	}
	return
}

func isAlnum(b byte) bool {
	return isDigit(b) || isAlpha(b)
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func isAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func rpmvercmp(a, b string) int {
	if a == b {
		return 0
	}

	one := []byte(a)
	two := []byte(b)
	i := 0
	j := 0

	for i < len(one) && j < len(two) {
		prevI := i
		prevJ := j

		for i < len(one) && !isAlnum(one[i]) {
			i++
		}
		for j < len(two) && !isAlnum(two[j]) {
			j++
		}

		if i >= len(one) || j >= len(two) {
			break
		}

		if (i - prevI) != (j - prevJ) {
			if (i - prevI) < (j - prevJ) {
				return -1
			}
			return 1
		}

		segStartI := i
		segStartJ := j

		isNum := isDigit(one[i])
		if isNum {
			for i < len(one) && isDigit(one[i]) {
				i++
			}
			for j < len(two) && isDigit(two[j]) {
				j++
			}
		} else {
			for i < len(one) && isAlpha(one[i]) {
				i++
			}
			for j < len(two) && isAlpha(two[j]) {
				j++
			}
		}

		if segStartI == i {
			return -1
		}

		if segStartJ == j {
			if isNum {
				return 1
			}
			return -1
		}

		segI := string(one[segStartI:i])
		segJ := string(two[segStartJ:j])

		if isNum {
			trimmedI := strings.TrimLeft(segI, "0")
			trimmedJ := strings.TrimLeft(segJ, "0")

			if len(trimmedI) > len(trimmedJ) {
				return 1
			}
			if len(trimmedJ) > len(trimmedI) {
				return -1
			}
			segI = trimmedI
			segJ = trimmedJ
		}

		if segI < segJ {
			return -1
		} else if segI > segJ {
			return 1
		}
	}

	if i >= len(one) && j >= len(two) {
		return 0
	}

	if (i >= len(one) && !isAlpha(two[j])) || (i < len(one) && isAlpha(one[i])) {
		return -1
	}
	return 1
}
