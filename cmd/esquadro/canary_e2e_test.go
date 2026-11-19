//go:build e2e

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// TestCanaryUpgrades generates a project of the model of a real one with the
// last release of esquadro, changes it as its team would, and upgrades it
// with this esquadro: it builds and passes its tests, nothing of the team's
// is lost, and the upgrade leaves no conflict.
func TestCanaryUpgrades(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatal("Docker is not available")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	tag := lastRelease(t, repo)
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(cache, "esquadro-canary-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })

	// The last release, built from its tag.
	src := filepath.Join(root, "release")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	archive := exec.Command("sh", "-c", "git archive "+tag+" | tar -x -C "+src)
	archive.Dir = repo
	if out, err := archive.CombinedOutput(); err != nil {
		t.Fatalf("git archive %s: %v\n%s", tag, err, out)
	}
	old := filepath.Join(root, "esquadro-"+tag)
	build := exec.Command("go", "build", "-ldflags", "-X github.com/patrickluzdev/esquadro/internal/version.Version="+tag, "-o", old, "./cmd/esquadro")
	build.Dir = src
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building esquadro %s: %v\n%s", tag, err, out)
	}

	// The project, as that release generates it.
	dir := filepath.Join(root, "promo")
	e := &lifecycle{t: t, dir: root}
	e.run(nil, old, "new", "-starter", "api", "-module", "example.com/promo", "-no-git", "-no-gen", "promo")
	e.dir = dir
	read := func(path string) string { return e.read(filepath.Join(dir, path)) }
	write := func(path, content string) { e.write(filepath.Join(dir, path), content) }
	canary := filepath.Join(repo, "cmd", "esquadro", "testdata", "canary", "promoduro")
	for _, name := range []string{"catalog.esq", "accounts.esq"} {
		os.Remove(filepath.Join(dir, "esquadro", name))
	}
	models, _ := filepath.Glob(filepath.Join(canary, "*.esq"))
	for _, m := range models {
		data, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		write(filepath.Join("esquadro", filepath.Base(m)), string(data))
	}
	e.run(nil, old, "gen", ".")
	e.run(nil, "go", "build", "./...")

	// What the team changes: a use case of its own, a job's work, a task,
	// the README.
	e.run(nil, "cp", "-R", filepath.Join(canary, "yours")+"/.", ".")
	work := read("internal/deals/fetch_offers_work.go")
	write("internal/deals/fetch_offers_work.go", strings.Replace(work, "\t_, _ = ctx, args", "\t// Ask the affiliate programs for offers.\n\t_, _ = ctx, args", 1))
	write("Taskfile.yml", read("Taskfile.yml")+"\ntasks:\n  fetch:\n    desc: Fetch offers now\n    cmds:\n      - go run ./cmd/promo run\n")
	write("README.md", read("README.md")+"\n## Promo do Duro\n\nThe groups, the site and the links.\n")
	e.run(nil, "go", "build", "./...")

	// Upgraded by this esquadro.
	out := e.esquadro(0, "upgrade", dir)
	if marked := e.run(nil, "sh", "-c", "grep -rl --exclude-dir=esquadro '^<<<<<<< yours' . || true"); marked != "" {
		t.Fatalf("the upgrade left conflicts in %s\n%s", marked, out)
	}
	e.esquadro(0, "gen", "-check", dir)
	if !strings.Contains(read("internal/deals/approve.go"), "ApproveOffer") ||
		!strings.Contains(read("internal/deals/fetch_offers_work.go"), "Ask the affiliate programs") ||
		!strings.Contains(read("Taskfile.yml"), "fetch:") ||
		!strings.Contains(read("README.md"), "## Promo do Duro") {
		t.Fatal("the upgrade lost a change of the team's")
	}
	e.run(nil, "go", "vet", "./...")

	// A feature of this esquadro adopted: money, in the app's currency.
	deals := read("esquadro/deals.esq")
	write("esquadro/deals.esq", regexp.MustCompile(`\bdecimal\b`).ReplaceAllString(deals, "money"))
	write("esquadro/app.esq", strings.Replace(read("esquadro/app.esq"), `  module "example.com/promo"`, "  module \"example.com/promo\"\n  currency \"BRL\"", 1))
	// And its public site: the offers and the coupons, for visitors.
	write("esquadro/app.esq", strings.Replace(read("esquadro/app.esq"), "  blueprint api_docs\n",
		"  blueprint api_docs\n  blueprint workspace\n  blueprint ui\n  blueprint ssr site {\n    auth \"visitor\"\n    locale pt_br\n    name \"Promo do Duro\"\n  }\n", 1))
	// The APIs it talks to: Shopee's offers and the Telegram bot.
	app := strings.Replace(read("esquadro/app.esq"), "  blueprint api_docs\n",
		"  blueprint api_docs\n  blueprint integration shopee {\n    url \"https://open-api.affiliate.shopee.com.br\"\n    rate_limit \"2/s\"\n    key true\n    secret true\n  }\n  blueprint integration telegram {\n    url \"https://api.telegram.org\"\n    rate_limit \"25/s\"\n    key true\n  }\n", 1)
	if !strings.Contains(app, "blueprint integration shopee") {
		t.Fatal("the canary's app.esq has no blueprint api_docs to add the integrations after")
	}
	write("esquadro/app.esq", app)
	deals = read("esquadro/deals.esq")
	deals = strings.Replace(deals, "  @security(read public, write [user.admin, user.operator])\n}\n\n/// A short link",
		"  @security(read public, write [user.admin, user.operator])\n  @ui(label \"Ofertas\", singular \"Oferta\")\n  @site(page, path \"ofertas\", schema product, menu)\n}\n\n/// A short link", 1)
	deals = strings.Replace(deals, "  expires_on date\n  @security(read public, write [user.admin, user.operator])",
		"  expires_on date\n  @security(read public, write [user.admin, user.operator])\n  @ui(label \"Cupons\", singular \"Cupom\")\n  @site(page, path \"cupons\", menu)", 1)
	// What the team tunes without a deploy: one record only.
	deals += "\n/// What the team tunes without a deploy.\nentity SiteSettings {\n  daily_limit   int    required default(20) min(1) max(100)\n  sending_hours string required default(\"08:00-22:00\")\n  @store(single)\n  @security(read public, write user.admin)\n}\n"
	// Commissions entered by hand, a batch at a time.
	deals = strings.Replace(deals, "  niche       > Niche\n  @security(read user.admin, write user.admin)\n}", "  niche       > Niche\n  @security(read user.admin, write user.admin)\n  @store(import)\n}", 1)
	if !strings.Contains(deals, "@store(import)") {
		t.Fatal("the canary's Commission was not marked @store(import)")
	}
	// A job posted once per offer and group, ever.
	deals += "\n/// Posts an offer to a group, once.\njob PostOffer {\n  offer_id       uuid required\n  destination_id uuid required\n  @jobs(unique_by [offer_id, destination_id])\n}\n"
	// Its short links: the site redirects by the code, which a rule makes up.
	deals = strings.Replace(deals, "  code        string required unique min(4) max(12)", "  code        string unique min(4) max(12)", 1)
	deals = strings.Replace(deals, "  destination > Destination\n  @security(read user, write [user.admin, user.operator])\n}",
		"  destination > Destination\n  @security(read user, write [user.admin, user.operator])\n  @site(redirect, by code, to target_url)\n}", 1)
	if !strings.Contains(deals, "@site(redirect, by code, to target_url)") || strings.Contains(deals, "code        string required") {
		t.Fatal("the canary's Link was not made a redirect")
	}
	// The visitors' comments on an offer, and the replies to them: each
	// visitor at most 30 an hour.
	deals += "\n/// A comment on an offer, or a reply to another.\nentity Comment {\n  body    text required max(2000)\n  offer   > Offer required on_delete(cascade) @store(filter)\n  parent  > Comment on_delete(cascade) @store(filter)\n  replies < Comment\n  author  > Visitor required\n  @security(read public, write owner)\n  @store(soft_delete)\n  @rate_limit(create \"30/h\")\n  @site(page, path \"comentarios\", relations [replies])\n}\n"
	withComments := strings.Replace(deals, `@site(page, path "ofertas", schema product, menu)`, `@site(page, path "ofertas", schema product, relations [comments], menu)`, 1)
	if withComments == deals {
		t.Fatal("the canary's Offer has no site page to list its comments on")
	}
	deals = withComments
	// The visitors' votes: an offer's temperature is their sum, and the hot
	// list orders by it, weighed by time.
	hot := strings.Replace(deals, "  niche                > Niche @store(filter)\n",
		"  niche                > Niche @store(filter)\n  temperature          int @store(sum votes.value, sort)\n  votes                < Vote\n"+
			"  @store(order [hot, \"temperature / power(extract(epoch from now() - created_at) / 3600 + 2, 1.5)\"])\n", 1)
	if hot == deals {
		t.Fatal("the canary's Offer has no niche to add its temperature after")
	}
	deals = hot + "\n/// A visitor's vote on an offer: +1 or -1.\nentity Vote {\n  value int required min(-1) max(1)\n  offer > Offer required on_delete(cascade)\n  @security(read public, write [user.admin, user.operator])\n}\n"
	write("esquadro/deals.esq", deals)
	write("internal/deals/link_rules.go", `package deals

import (
	"context"
	"crypto/rand"
	"strings"
)

type LinkRules struct {
	linkHooks
}

// A link without a code gets one made up.
func (LinkRules) BeforeCreate(ctx context.Context, m *Deals, n *NewLink) error {
	if n.Code == "" {
		n.Code = strings.ToLower(rand.Text()[:7])
	}
	return nil
}

// A link of an offer that expired sends to its page on the site.
func (LinkRules) BeforeRedirect(ctx context.Context, m *Deals, l Link, v Visit) (string, error) {
	offer, err := m.ReadOffer(ctx, l.OfferID)
	if err != nil {
		return "", err
	}
	if offer.Status == OfferStatusExpired {
		return "/ofertas/" + offer.ID.String(), nil
	}
	return l.TargetURL, nil
}

// Every visit is a click, a bot's told apart.
func (LinkRules) Visited(ctx context.Context, m *Deals, l Link, v Visit) error {
	_, err := m.CreateClick(ctx, NewClick{LinkID: l.ID, Bot: v.Bot, UserAgent: v.UserAgent, IPHash: v.Visitor})
	return err
}
`)
	// And a business rule: an offer that expired is not approved.
	write("internal/deals/offer_rules.go", `package deals

import "context"

type OfferRules struct {
	offerHooks
}

func (OfferRules) BeforeUpdate(ctx context.Context, m *Deals, stored Offer, n *NewOffer) error {
	if stored.Status == OfferStatusExpired && n.Status == OfferStatusApproved {
		return Broken("offer_expired", "Esta oferta expirou e não pode ser aprovada.", "status")
	}
	return nil
}
`)
	e.esquadro(0, "gen", dir)
	// The short links, end to end: the code made up, a person and a preview
	// sent on and counted apart, and an expired offer's link to its page.
	write("web/site/link_redirect_test.go", `package site

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bojanz/currency"

	"example.com/promo/internal/deals"
	"example.com/promo/internal/platform/postgres/postgrestest"
)

func TestShortLinks(t *testing.T) {
	ctx := context.Background()
	pool := postgrestest.Fresh(t)
	m := deals.New(deals.Deps{Pool: pool})
	price, _ := currency.NewAmount("89.90", "BRL")
	offer, err := m.CreateOffer(ctx, deals.NewOffer{Source: deals.SourceShopee, Store: "Shopee", Title: "Panela", ProductURL: "https://shopee.com.br/panela", Price: price})
	if err != nil {
		t.Fatal(err)
	}
	link, err := m.CreateLink(ctx, deals.NewLink{TargetURL: "https://shopee.com.br/panela", OfferID: offer.ID})
	if err != nil || len(link.Code) != 7 {
		t.Fatalf("a link without a code: %+v, %v", link, err)
	}
	srv := httptest.NewServer(New(Deps{Deals: m, VisitorSecret: []byte("test"), Logger: slog.New(slog.DiscardHandler)}))
	defer srv.Close()
	visit := func(agent string) *http.Response {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/"+link.Code, nil)
		req.Header.Set("User-Agent", agent)
		res, err := (&http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}
	if res := visit("Mozilla/5.0"); res.StatusCode != http.StatusFound || res.Header.Get("Location") != "https://shopee.com.br/panela" {
		t.Fatalf("a person: %d to %s", res.StatusCode, res.Header.Get("Location"))
	}
	if res := visit("WhatsApp/2.24"); res.StatusCode != http.StatusFound {
		t.Fatalf("a preview: %d", res.StatusCode)
	}
	var people, bots int
	if err := pool.QueryRow(ctx, "SELECT count(*) FILTER (WHERE NOT bot), count(*) FILTER (WHERE bot) FROM clicks").Scan(&people, &bots); err != nil || people != 1 || bots != 1 {
		t.Fatalf("clicks: %d people, %d bots (%v)", people, bots, err)
	}
	expired := deals.OfferStatusExpired
	if _, err := m.PatchOffer(ctx, offer.ID, 0, deals.OfferChanges{Status: &expired}, nil); err != nil {
		t.Fatal(err)
	}
	if res := visit("Mozilla/5.0"); res.Header.Get("Location") != Path+"/ofertas/"+offer.ID.String() {
		t.Fatalf("an expired offer's link: %d to %s", res.StatusCode, res.Header.Get("Location"))
	}
}
`)
	// The comments, end to end: the top ones apart, and the replies on
	// their comment's page.
	write("web/site/comment_replies_test.go", `package site

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"github.com/bojanz/currency"

	"example.com/promo/internal/accounts"
	"example.com/promo/internal/deals"
	"example.com/promo/internal/platform/postgres/postgrestest"
)

func TestCommentReplies(t *testing.T) {
	ctx := context.Background()
	pool := postgrestest.Fresh(t)
	m := deals.New(deals.Deps{Pool: pool})
	price, _ := currency.NewAmount("89.90", "BRL")
	offer, err := m.CreateOffer(ctx, deals.NewOffer{Source: deals.SourceShopee, Store: "Shopee", Title: "Panela", ProductURL: "https://shopee.com.br/panela", Price: price})
	if err != nil {
		t.Fatal(err)
	}
	top, err := m.CreateComment(ctx, deals.NewComment{Body: "Vale a pena?", OfferID: offer.ID, AuthorID: uuid.NewV7()})
	if err != nil {
		t.Fatal(err)
	}
	reply, err := m.CreateComment(ctx, deals.NewComment{Body: "Comprei, recomendo", OfferID: offer.ID, ParentID: top.ID, AuthorID: uuid.NewV7()})
	if err != nil {
		t.Fatal(err)
	}
	page, err := m.ListComments(ctx, deals.CommentQuery{Filter: deals.CommentFilter{OfferID: offer.ID, NoParent: true}, Limit: 10})
	if err != nil || len(page.Comments) != 1 || page.Comments[0].ID != top.ID {
		t.Fatalf("the top comments: %+v, %v", page.Comments, err)
	}
	srv := httptest.NewServer(New(Deps{Deals: m, Accounts: accounts.New(accounts.Deps{Pool: pool}), VisitorSecret: []byte("test"), Logger: slog.New(slog.DiscardHandler)}))
	defer srv.Close()
	res, err := http.Get(srv.URL + CommentPath(top))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), reply.Body) {
		t.Fatalf("a comment's page: %d, without its reply", res.StatusCode)
	}
	// A comment in the trash takes its replies with it.
	if err := m.DeleteComment(ctx, top.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ReadComment(ctx, reply.ID); err == nil {
		t.Fatal("a reply outlived its comment")
	}
}
`)
	if !strings.Contains(read("web/site/offer_gen.go"), "func (s *Site) listOffers(") || !strings.Contains(read("web/site/coupon_gen.go"), "func (s *Site) readCoupon(") {
		t.Fatal("the site has no pages of offers and coupons")
	}
	if !strings.Contains(read("internal/deals/create_comment.go"), "func (d *Deals) commentLimit(") || !strings.Contains(read("cmd/promo/api_test.go"), "func TestAPICommentCreateLimit(") {
		t.Fatal("the comments have no limit of creates, or no test of it")
	}
	// The temperature, kept as votes come and go, and the hot list.
	write("internal/deals/temperature_test.go", `package deals_test

import (
	"context"
	"testing"

	"github.com/bojanz/currency"

	"example.com/promo/internal/deals"
	"example.com/promo/internal/platform/postgres/postgrestest"
)

func TestTemperature(t *testing.T) {
	ctx := context.Background()
	m := deals.New(deals.Deps{Pool: postgrestest.Fresh(t)})
	price, _ := currency.NewAmount("10", "BRL")
	offer := func(title string) deals.Offer {
		o, err := m.CreateOffer(ctx, deals.NewOffer{Source: deals.SourceManual, Store: "Loja", Title: title, ProductURL: "https://loja.com.br", Price: price})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	cold, warm := offer("Fria"), offer("Quente")
	var last deals.Vote
	for _, v := range []int{1, 1, -1} {
		vote, err := m.CreateVote(ctx, deals.NewVote{Value: v, OfferID: warm.ID})
		if err != nil {
			t.Fatal(err)
		}
		last = vote
	}
	if err := m.DeleteVote(ctx, last.ID, 0); err != nil {
		t.Fatal(err)
	}
	read, err := m.ReadOffer(ctx, warm.ID)
	if err != nil || read.Temperature != 2 {
		t.Fatalf("temperature %d, %v", read.Temperature, err)
	}
	page, err := m.ListOffers(ctx, deals.OfferQuery{Sort: "hot", Page: 1, PerPage: 10})
	if err != nil || len(page.Offers) != 2 || page.Offers[0].ID != warm.ID || page.Offers[1].ID != cold.ID {
		t.Fatalf("hot: %+v, %v", page.Offers, err)
	}
}
`)
	e.run(nil, "go", "vet", "./...")
	e.run(nil, "go", "test", "./...")
	e.esquadro(0, "gen", "-check", dir)
}

// lastRelease is the newest release tag before this commit: not one on it,
// as when CI runs for the release being made.
func lastRelease(t *testing.T, repo string) string {
	t.Helper()
	here := strings.Fields(gitOut(t, repo, "tag", "--points-at", "HEAD", "--list", "v*"))
	for _, tag := range strings.Fields(gitOut(t, repo, "tag", "--merged", "HEAD", "--list", "v*", "--sort=-v:refname")) {
		if !slices.Contains(here, tag) && !strings.Contains(tag, "-") {
			return tag
		}
	}
	t.Fatal("no release before this commit")
	return ""
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}
