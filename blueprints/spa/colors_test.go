package spa

import "testing"

func TestColors(t *testing.T) {
	for _, c := range []struct{ color, on string }{
		{"#2a85ff", "#ffffff"},
		{"#3d3d3d", "#ffffff"},
		{"#ffd400", "#171717"},
		{"#fff", "#171717"},
	} {
		if got := OnColor(c.color); got != c.on {
			t.Errorf("OnColor(%s) = %s, want %s", c.color, got, c.on)
		}
	}
	if got := DarkPrimary("#2a85ff", ""); got != "#2a85ff" {
		t.Errorf("a bright primary keeps itself in the dark: %s", got)
	}
	if got := DarkPrimary("#3d3d3d", ""); got != "#e5e5e5" {
		t.Errorf("a dark primary turns light in the dark: %s", got)
	}
	if got := DarkPrimary("#3d3d3d", "#FFAA00"); got != "#ffaa00" {
		t.Errorf("primary_dark is kept: %s", got)
	}
	if ColorError("primary", "blue") == "" || ColorError("primary", "#12345") == "" || ColorError("primary", "#abc") != "" {
		t.Error("colors are #rgb or #rrggbb")
	}
}
