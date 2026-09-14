package selfupdate

import (
	"strconv"
	"strings"
)

// normalize strips the tag prefix, an optional "v", and build metadata, so
// "kit-v0.1.0-rc.2+deadbeef" reads as "0.1.0-rc.2".
func normalize(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "kit-")
	v = strings.TrimPrefix(v, "v")
	if base, _, ok := strings.Cut(v, "+"); ok {
		v = base
	}
	return v
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parsable reports whether v looks like a kit version at all. "dev", "" and
// "unknown" do not, and a binary carrying one of those must never be offered
// an update.
func parsable(v string) bool {
	n := normalize(v)
	if n == "" {
		return false
	}
	core, _, _ := strings.Cut(n, "-")
	first, _, _ := strings.Cut(core, ".")
	return allDigits(first)
}

// IsPrerelease reports whether v carries a prerelease part ("-rc.1").
func IsPrerelease(v string) bool {
	_, pre, ok := strings.Cut(normalize(v), "-")
	return ok && pre != ""
}

// Compare orders two kit versions: -1, 0, +1. It accepts an optional "v"
// prefix and an optional "kit-" prefix, compares the X.Y.Z core numerically
// (missing parts count as 0), ranks a version without a prerelease above the
// same core with one, ignores build metadata, and reads an unparsable version
// as 0.0.0.
func Compare(a, b string) int {
	coreA, preA := split(a)
	coreB, preB := split(b)
	if c := compareCore(coreA, coreB); c != 0 {
		return c
	}
	return comparePrerelease(preA, preB)
}

func split(v string) (core []string, pre string) {
	n := normalize(v)
	c, p, _ := strings.Cut(n, "-")
	return strings.Split(c, "."), p
}

func compareCore(a, b []string) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		x, y := numAt(a, i), numAt(b, i)
		if x != y {
			return sign(x - y)
		}
	}
	return 0
}

func numAt(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, _ := strconv.Atoi(parts[i])
	return n
}

func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	ia, ib := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(ia) && i < len(ib); i++ {
		if c := compareIdent(ia[i], ib[i]); c != 0 {
			return c
		}
	}
	return sign(len(ia) - len(ib))
}

func compareIdent(a, b string) int {
	if allDigits(a) && allDigits(b) {
		x, _ := strconv.Atoi(a)
		y, _ := strconv.Atoi(b)
		return sign(x - y)
	}
	return strings.Compare(a, b)
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
