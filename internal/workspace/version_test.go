package workspace

import "testing"

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		project, running string
		want             Versions
	}{
		{"", "v0.8.0", SameVersion},
		{"v0.8.0", "v0.8.0", SameVersion},
		{"v0.7.0", "v0.8.0", NewerVersion},
		{"v0.9.0", "v0.8.0", OlderVersion},
		{"v0.8.0", "v0.8.1-0.20261118205728-330f22b642c4+dirty", DevelopmentVersion},
		{"v0.7.1-0.20261118205728-330f22b642c4", "v0.8.0", DevelopmentVersion},
		{"v0.8.0", "dev", DevelopmentVersion},
	} {
		if got := CompareVersions(c.project, c.running); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.project, c.running, got, c.want)
		}
	}
}
