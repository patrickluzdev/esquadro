package project

import (
	"testing"
)

func TestJobs(t *testing.T) {
	src := `package accounts

app shop {
  module "example.com/shop"
}

enum Locale {
  EN
  PT
}

/// Sends the welcome email.
job SendWelcomeEmail {
  user_id uuid   required
  locale  Locale default(EN)
  id      string
}

job CloseDailyReport
`
	p, diags := Load(Source{Name: "accounts.esq", Src: []byte(src)})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if len(p.Jobs) != 2 || len(p.Modules) != 1 || len(p.Modules[0].Jobs) != 2 || len(p.Entities) != 0 {
		t.Fatalf("jobs %d, modules %d", len(p.Jobs), len(p.Modules))
	}
	j := p.Jobs[0]
	if j.Pascal != "SendWelcomeEmail" || j.Snake != "send_welcome_email" || j.Module.Raw != "accounts" || j.Doc[0] != "Sends the welcome email." {
		t.Fatalf("job: %+v", j)
	}
	if len(j.Fields) != 3 || !j.Field("user_id").Required || j.Field("locale").Enum == nil || j.Field("id").Implicit {
		t.Fatalf("fields: %+v", j.Fields)
	}
	if len(p.Jobs[1].Fields) != 0 {
		t.Fatal("a bare job has no arguments")
	}
	d := p.Describe()
	if len(d.Jobs) != 2 || d.Jobs[0].Pascal != "SendWelcomeEmail" || d.Jobs[0].Module != "accounts" || len(d.Jobs[0].Fields) != 3 {
		t.Fatalf("described: %+v", d.Jobs)
	}
}
