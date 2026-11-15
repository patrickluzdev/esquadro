package app

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v1.4.0", "v1.4.0", 0},
		{"v1.3.9", "v1.4.0", -1},
		{"v1.10.0", "v1.9.9", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"v1.4.0-rc.1", "v1.4.0", -1},
		{"v1.4.0", "v1.4.0-rc.1", 1},
		{"v1.4.0-rc.1", "v1.4.0-rc.2", -1},
		{"v0.0.0-20240101000000-abcdef", "v0.1.0", -1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
