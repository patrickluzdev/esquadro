package app

import "testing"

func TestCommandNames(t *testing.T) {
	cases := "\tcase \"\", \"serve\":\n\t\treturn serve(ctx)\n\tcase \"migrate\":\n\t\treturn migrate(ctx)\n"
	if got := commandNames(cases); got != "check, migrate, serve" {
		t.Fatalf("got %q", got)
	}
	if got := commandNames(""); got != "check" {
		t.Fatalf("no cases: %q", got)
	}
}
