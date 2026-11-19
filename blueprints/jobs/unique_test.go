package jobs

import (
	"testing"
	"time"
)

func TestPeriod(t *testing.T) {
	for s, want := range map[string]time.Duration{"10m": 10 * time.Minute, "24h": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "90s": 90 * time.Second} {
		if got, err := Period(s); err != nil || got != want {
			t.Errorf("%s: %v %v", s, got, err)
		}
	}
	for _, s := range []string{"", "d", "0d", "-1h", "soon"} {
		if _, err := Period(s); err == nil {
			t.Errorf("%q is a period", s)
		}
	}
	if got := goDuration(7 * 24 * time.Hour); got != "168 * time.Hour" {
		t.Errorf("goDuration: %s", got)
	}
}
