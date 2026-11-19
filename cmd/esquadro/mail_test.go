package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// layoutTest is a test of the generated mail package, written into the
// project: how its layout writes an email.
const layoutTest = `package mail

import (
	"strings"
	"testing"
)

func TestTheLayout(t *testing.T) {
	msg, err := Write("welcome", map[string]any{"Name": "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	msg = withLogo(msg)
	for _, want := range strings.Split(os.Getenv("WANT_HTML"), "|") {
		if !strings.Contains(msg.HTML, want) {
			t.Errorf("the HTML lacks %q:\n%s", want, msg.HTML)
		}
	}
	for _, want := range strings.Split(os.Getenv("WANT_TEXT"), "|") {
		if !strings.Contains(msg.Text, want) {
			t.Errorf("the text lacks %q:\n%s", want, msg.Text)
		}
	}
	for _, not := range strings.Split(os.Getenv("NOT"), "|") {
		if not != "" && strings.Contains(msg.HTML+msg.Text, not) {
			t.Errorf("the email has %q:\n%s\n%s", not, msg.HTML, msg.Text)
		}
	}
	if got := len(msg.Inline); got != len(os.Getenv("INLINE")) {
		t.Errorf("%d files inline", got)
	}
}
`

// mailProject is a project with mail, whose app.esq gives mail the
// options opts.
func mailProject(t *testing.T, opts string) (dir, appEsq string) {
	t.Helper()
	dir, appEsq = smallProject(t, "app,env,mail")
	src := read(t, appEsq)
	if !strings.Contains(src, "  blueprint mail\n") {
		t.Fatalf("no blueprint mail in:\n%s", src)
	}
	os.WriteFile(appEsq, []byte(strings.Replace(src, "  blueprint mail\n", "  blueprint mail {\n"+opts+"  }\n", 1)), 0o644)
	return dir, appEsq
}

// runLayout runs layoutTest in the project, wanting want of the HTML and
// the text, not none of them, and inline files.
func runLayout(t *testing.T, dir string, html, text, none []string, inline int) {
	t.Helper()
	test := strings.Replace(layoutTest, "import (\n", "import (\n\t\"os\"\n", 1)
	os.WriteFile(filepath.Join(dir, "internal", "platform", "mail", "layout_check_test.go"), []byte(test), 0o644)
	cmd := exec.Command("go", "test", "-count=1", "-run", "TestTheLayout|TestButton|TestLogo|TestTemplates|TestLayoutsAreNoEmails", "./internal/platform/mail/")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "WANT_HTML="+strings.Join(html, "|"), "WANT_TEXT="+strings.Join(text, "|"),
		"NOT="+strings.Join(none, "|"), "INLINE="+strings.Repeat("x", inline))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test: %v\n%s", err, out)
	}
}

// Without a brand, the emails keep their neutral look: no header, no
// footer; with one, the layout takes its colors, the names of its fonts
// and its logo, sent inline, with a footer in the locale's words.
func TestMailLayoutWithABrand(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a generated project")
	}
	dir, appEsq := mailProject(t, "")
	if code, _, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	layout := read(t, filepath.Join(dir, "internal", "platform", "mail", "templates", "_layout.html"))
	if !strings.Contains(layout, "{{- if branded}}") || !strings.Contains(layout, `{{block "why" .}}`) {
		t.Fatalf("_layout.html:\n%s", layout)
	}
	if _, err := os.Stat(filepath.Join(dir, "brand", "embed.go")); err == nil {
		t.Fatal("a brand package without a brand")
	}
	runLayout(t, dir, []string{"background:#f5f5f4", "Hi Ana,"}, []string{"Hi Ana,"},
		[]string{"You are receiving this email", "cid:", "<table"}, 0)

	src := strings.Replace(read(t, appEsq), "  blueprint mail {\n", "  blueprint mail {\n    brand \"brand\"\n    locale pt_br\n    address \"Shop Ltd, 1 Main Street\"\n", 1)
	os.WriteFile(appEsq, []byte(src), 0o644)
	if code, _, errOut := esq(t, "check", dir); code == 0 || !strings.Contains(errOut, "brand brand:") {
		t.Fatalf("a brand without its folder checks: exit %d\n%s", code, errOut)
	}
	write := func(name, content string) {
		p := filepath.Join(dir, "brand", filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}
	theme := `{"colors": {"accent": "#2F6FEB", "on_accent": "#FFFFFF", "page": "#F3F1EC"},
		"fonts": {"display": {"family": "Display Serif", "fallback": "Georgia, serif"}, "body": {"family": "Body Sans"}},
		"identity": {"logo": "logo.svg"}}`
	write("theme.json", theme)
	write("logo.svg", "<svg/>")
	code, _, errOut := esq(t, "gen", dir)
	if code != 0 || !strings.Contains(errOut, "the emails write the app's name in place of the logo, logo.svg") {
		t.Fatalf("an SVG logo: exit %d\n%s", code, errOut)
	}
	gen := read(t, filepath.Join(dir, "internal", "platform", "mail", "layout.go"))
	if strings.Contains(gen, "example.com/shop/brand") {
		t.Errorf("layout.go imports the brand without a logo for emails:\n%s", gen)
	}
	runLayout(t, dir, []string{"background:#F3F1EC", "&#39;Display Serif&#39;, Georgia, serif", "&#39;Body Sans&#39;, -apple-system",
		">Shop</td>", "Você está recebendo este e-mail de Shop sobre a sua conta.", "Shop Ltd, 1 Main Street"},
		[]string{"Olá, Ana,", "\n--\nShop\nVocê está recebendo este e-mail de Shop sobre a sua conta.\nShop Ltd, 1 Main Street"}, []string{"cid:", "Hi Ana"}, 0)

	write("logo.png", "\x89PNG\r\n\x1a\n")
	write("theme.json", strings.Replace(theme, `"logo": "logo.svg"`, `"logo": "logo.svg", "logo_email": "logo.png"`, 1))
	code, _, errOut = esq(t, "gen", dir)
	if code != 0 || strings.Contains(errOut, "in place of the logo") {
		t.Fatalf("a PNG logo for emails: exit %d\n%s", code, errOut)
	}
	for path, want := range map[string][]string{
		"brand/embed.go":                   {"package brand", "//go:embed all:*", "the emails take their logo"},
		"internal/platform/mail/layout.go": {`brand "example.com/shop/brand"`, `brand.Files.ReadFile("logo.png")`, `"image/png"`, `const lang = "pt-BR"`},
	} {
		got := read(t, filepath.Join(dir, filepath.FromSlash(path)))
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s lacks %q:\n%s", path, w, got)
			}
		}
	}
	runLayout(t, dir, []string{`<img src="cid:brand-logo" alt="Shop"`}, []string{"Olá, Ana,"}, nil, 1)

	for from, tc := range map[string]struct{ to, want string }{
		"locale pt_br":                      {"locale fr", "locale fr is not en, pt_br, or frontend"},
		`address "Shop Ltd, 1 Main Street"`: {`address "Shop Ltd\n1 Main Street"`, "address is a single line"},
	} {
		os.WriteFile(appEsq, []byte(strings.Replace(src, from, tc.to, 1)), 0o644)
		if code, _, errOut := esq(t, "check", dir); code == 0 || !strings.Contains(errOut, tc.want) {
			t.Errorf("%s: exit %d, want %q in:\n%s", tc.to, code, tc.want, errOut)
		}
	}
	write("theme.json", strings.Replace(theme, `"logo": "logo.svg"`, `"logo": "logo.svg", "logo_email": "logo.svg"`, 1))
	os.WriteFile(appEsq, []byte(src), 0o644)
	if code, _, errOut := esq(t, "check", dir); code == 0 || !strings.Contains(errOut, "identity.logo_email logo.svg is not a PNG, JPEG or GIF") {
		t.Errorf("an SVG logo_email: exit %d\n%s", code, errOut)
	}
}

// The built-in emails come in mail's locale: an untouched copy follows
// it, one the project changed has the change merged in, with conflicts
// where both changed a line.
func TestMailEmailsInTheLocale(t *testing.T) {
	dir, appEsq := mailProject(t, "    locale pt_br\n")
	if code, _, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	templates := filepath.Join(dir, "internal", "platform", "mail", "templates")
	for name, want := range map[string]string{
		"welcome.txt":  "Subject: Boas-vindas, {{.Name}}\nOlá, {{.Name}},",
		"welcome.html": "<p>Olá, {{.Name}},</p>",
		"../layout.go": `return fmt.Sprintf("%d de %s, %s UTC", t.Day(), months[t.Month()-1], t.Format("15:04"))`,
	} {
		if got := read(t, filepath.Join(templates, name)); !strings.Contains(got, want) {
			t.Errorf("%s lacks %q:\n%s", name, want, got)
		}
	}
	if got := read(t, filepath.Join(dir, "cmd", "shop", "mail_command.go")); !strings.Contains(got, `mail.Wrap(mail.Word("testSubject")`) {
		t.Errorf("mail test is not in the layouts:\n%s", got)
	}
	runLayout(t, dir, []string{"Olá, Ana,"}, []string{"Olá, Ana,"}, []string{"Hi Ana"}, 0)

	// English again: the untouched welcome.html follows, the welcome.txt
	// the project signed is merged.
	txt := filepath.Join(templates, "welcome.txt")
	os.WriteFile(txt, []byte(read(t, txt)+"\nAtenciosamente, a equipe.\n"), 0o644)
	os.WriteFile(appEsq, []byte(strings.Replace(read(t, appEsq), "    locale pt_br\n", "    locale en\n", 1)), 0o644)
	_, out, errOut := esq(t, "gen", dir)
	if got := read(t, filepath.Join(templates, "welcome.html")); !strings.Contains(got, "<p>Hi {{.Name}},</p>") {
		t.Errorf("the untouched welcome.html is not in English:\n%s", got)
	}
	got := read(t, txt)
	if !strings.Contains(got, "Subject: Welcome, {{.Name}}") || !strings.Contains(got, "Atenciosamente, a equipe.") {
		t.Errorf("the changed welcome.txt lost the template's change or the project's:\n%s\n%s%s", got, out, errOut)
	}
}

// A site and the emails naming the same brand share its package, which
// the site writes.
func TestMailAndASiteShareABrand(t *testing.T) {
	dir, code, errOut := ssrProject(t, func(s string) string {
		s = strings.Replace(s, "    name \"Loja\"\n", "    name \"Loja\"\n    brand \"brand\"\n", 1)
		return strings.Replace(s, "  blueprint mail\n", "  blueprint mail {\n    brand \"brand/\"\n  }\n", 1)
	}, nil)
	if code == 0 || !strings.Contains(errOut, "brand brand:") {
		t.Fatalf("a brand without its folder: exit %d\n%s", code, errOut)
	}
	if strings.Count(errOut, "theme.json") != 1 {
		t.Errorf("the brand's problems are told once, by the site:\n%s", errOut)
	}
	os.MkdirAll(filepath.Join(dir, "brand"), 0o755)
	os.WriteFile(filepath.Join(dir, "brand", "theme.json"), []byte(`{"colors": {"accent": "#2F6FEB"}}`), 0o644)
	if code, _, errOut := esq(t, "gen", dir); code != 0 {
		t.Fatal(errOut)
	}
	embed := read(t, filepath.Join(dir, "brand", "embed.go"))
	if !strings.Contains(embed, "site serves them") {
		t.Errorf("brand/embed.go is not the site's:\n%s", embed)
	}
	// The emails take the language of the site, pt_br.
	if got := read(t, filepath.Join(dir, "internal", "platform", "mail", "templates", "verify_email.txt")); !strings.HasPrefix(got, "Subject: Confirme seu e-mail") {
		t.Errorf("verify_email.txt is not in the site's language:\n%s", got)
	}
	if _, out, _ := esq(t, "explain", "-C", dir, "brand/embed.go"); !strings.Contains(out, "generated by  shop") || strings.Contains(out, "replaces") {
		t.Errorf("two blueprints write brand/embed.go:\n%s", out)
	}
	vet := exec.Command("go", "vet", "./brand/...", "./internal/platform/mail/...")
	vet.Dir = dir
	if out, err := vet.CombinedOutput(); err != nil {
		t.Fatalf("go vet: %v\n%s", err, out)
	}
}
