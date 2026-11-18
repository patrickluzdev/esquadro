package changes

import "testing"

func TestBetween(t *testing.T) {
	if got := Between("v0.7.0", "v0.8.0"); len(got) != 1 || got[0].Version != "v0.8.0" {
		t.Fatalf("v0.7.0 to v0.8.0: %v", got)
	}
	if got := Between("v0.8.0", "v0.8.0"); len(got) != 0 {
		t.Fatalf("nothing between a version and itself: %v", got)
	}
	if got := Between("v0.7.0", ""); len(got) == 0 {
		t.Fatal("a development build shows the notes after the project's version")
	}
}
