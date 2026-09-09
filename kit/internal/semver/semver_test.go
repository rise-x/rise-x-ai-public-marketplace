package semver

import (
	"reflect"
	"testing"
)

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.10.0", "1.9.0", false}, // 10 > 9, not a string compare
		{"1.9.0", "1.10.0", true},
		{"1.5.0", "1.5", false}, // a missing segment is 0
		{"1.5", "1.5.0", false},
		{"9.0.0", "22.4.0", true},
		{"2.1.258 (Claude Code)", "2.1.259", true},
		{"v20.11.0", "20.11.0", false},
		{"1.5.0-rc1", "1.5.0", false}, // pre-release ordering is out of scope
	}
	for _, c := range cases {
		if got := VersionLess(c.a, c.b); got != c.want {
			t.Errorf("VersionLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestVersionParts(t *testing.T) {
	if got := VersionParts("v20.11.0"); !reflect.DeepEqual(got, []int{20, 11, 0}) {
		t.Errorf("VersionParts = %v", got)
	}
	if got := VersionParts("2.1.258 (Claude Code)"); !reflect.DeepEqual(got, []int{2, 1, 258}) {
		t.Errorf("VersionParts = %v", got)
	}
}
