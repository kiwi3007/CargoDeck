package indexer

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	reVersionSemver = regexp.MustCompile(`(?i)v(\d+\.\d+(?:\.\d+)*)`)
	reVersionBuild  = regexp.MustCompile(`(?i)build[. _]?(\d{4,})`)
	reVersionUpdate = regexp.MustCompile(`(?i)update[. _]?(\d+)`)
)

// ParseVersionFromTitle extracts a version string from a release title.
// Returns "" if nothing meaningful is found.
// Examples:
//   "GameName.v1.2.3.REPACK-FitGirl"  → "1.2.3"
//   "Game.Build.20240101-GROUP"        → "Build.20240101"
//   "Game.v2.Update.5-GROUP"           → "2" (first match wins)
func ParseVersionFromTitle(title string) string {
	if m := reVersionSemver.FindStringSubmatch(title); len(m) > 1 {
		return m[1]
	}
	if m := reVersionBuild.FindStringSubmatch(title); len(m) > 1 {
		return "Build." + m[1]
	}
	if m := reVersionUpdate.FindStringSubmatch(title); len(m) > 1 {
		return "Update." + m[1]
	}
	return ""
}

// IsNewerVersion returns true if candidate appears strictly newer than installed.
// Falls back to "different" detection if version format is unrecognised.
func IsNewerVersion(installed, candidate string) bool {
	if installed == "" || candidate == "" {
		return false
	}
	// Normalise: strip leading "v" or "V"
	installed = strings.TrimLeft(installed, "vV")
	candidate = strings.TrimLeft(candidate, "vV")

	// Strip "Build." / "Update." prefixes before numeric compare
	stripLabel := func(s string) string {
		s = strings.TrimPrefix(s, "Build.")
		s = strings.TrimPrefix(s, "Update.")
		return s
	}
	iNorm := stripLabel(installed)
	cNorm := stripLabel(candidate)

	iParts := splitVersion(iNorm)
	cParts := splitVersion(cNorm)

	if len(iParts) == 0 || len(cParts) == 0 {
		// Can't parse — treat any difference as "newer"
		return candidate != installed
	}

	maxLen := len(iParts)
	if len(cParts) > maxLen {
		maxLen = len(cParts)
	}
	for i := 0; i < maxLen; i++ {
		iv := 0
		cv := 0
		if i < len(iParts) {
			iv = iParts[i]
		}
		if i < len(cParts) {
			cv = cParts[i]
		}
		if cv > iv {
			return true
		}
		if cv < iv {
			return false
		}
	}
	return false // equal
}

func splitVersion(s string) []int {
	parts := strings.Split(s, ".")
	var result []int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil // non-numeric part — not a parseable semver
		}
		result = append(result, n)
	}
	return result
}
