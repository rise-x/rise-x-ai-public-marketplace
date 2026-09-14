package selfupdate

import "testing"

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v0.1.0-rc.1", "v0.1.0-rc.2", -1},
		{"v0.1.0-rc.2", "v0.1.0-rc.10", -1},
		{"v0.1.0-rc.10", "v0.1.0", -1},
		{"v0.1.0", "v0.1.1", -1},
		{"v0.1.1", "v0.2.0", -1},
		{"v0.2.0", "v0.1.1", 1},
		{"kit-v0.1.0", "v0.1.0", 0},
		{"v0.1.0", "0.1.0", 0},
		{"kit-v0.1.0", "0.1.0", 0},
		{"v0.1", "v0.1.0", 0},
		{"v1.0.0", "v0.9.9", 1},
		{"v0.1.0", "v0.1.0-rc.1", 1},
		{"v0.1.0-rc.1", "v0.1.0", -1},
		{"v0.1.0-rc", "v0.1.0-rc.1", -1},
		{"v0.1.0-alpha.1", "v0.1.0-beta.1", -1},
		{"v0.1.0+abc", "v0.1.0+def", 0},
		{"dev", "v0.0.0", 0},
		{"", "0.0.0", 0},
	}
	for _, tt := range tests {
		if got := Compare(tt.a, tt.b); got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
		if got := Compare(tt.b, tt.a); got != -tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.b, tt.a, got, -tt.want)
		}
	}
}

func TestIsPrerelease(t *testing.T) {
	tests := []struct {
		v    string
		want bool
	}{
		{"v0.1.0-rc.1", true},
		{"kit-v0.1.0-rc.1", true},
		{"v0.1.0", false},
		{"0.1.0", false},
		{"v0.1.0+build", false},
		{"dev", false},
	}
	for _, tt := range tests {
		if got := IsPrerelease(tt.v); got != tt.want {
			t.Errorf("IsPrerelease(%q) = %v, want %v", tt.v, got, tt.want)
		}
	}
}
