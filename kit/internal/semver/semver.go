// Package semver compares the dot-separated numeric versions Claude Code, the
// plugin catalog and node report. It is deliberately not a full semver
// implementation: no pre-release or build-metadata ordering.
package semver

import (
	"strconv"
	"strings"
)

// VersionParts splits v into its numeric segments, ignoring a leading "v" and
// anything after the first space ("2.1.258 (Claude Code)"). A non-numeric
// segment counts as 0.
func VersionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if first, _, ok := strings.Cut(v, " "); ok {
		v = first
	}
	segments := strings.Split(v, ".")
	out := make([]int, len(segments))
	for i, s := range segments {
		out[i], _ = strconv.Atoi(s)
	}
	return out
}

// VersionLess reports whether a orders below b, comparing segment by segment
// so 9.0.0 sorts below 22.4.0 and 1.5 equals 1.5.0.
func VersionLess(a, b string) bool {
	pa, pb := VersionParts(a), VersionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		x, y := partAt(pa, i), partAt(pb, i)
		if x != y {
			return x < y
		}
	}
	return false
}

func partAt(parts []int, i int) int {
	if i >= len(parts) {
		return 0
	}
	return parts[i]
}
