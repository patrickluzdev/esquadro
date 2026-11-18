//go:build e2e

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestLifecycle follows a project through its life as a user lives it:
// esquadro new, run it against Postgres and store records, write code of
// one's own, change the model, regenerate, migrate the database that has
// the records, and run again. It needs Docker and the network; run it with
// task e2e.
func TestLifecycle(t *testing.T) {
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatal("Docker is not available")
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(cache, "esquadro-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	dir := filepath.Join(root, "shop")
	e := &lifecycle{t: t, dir: dir}

	// 1. A new project, generated.
	e.esquadro(0, "new", "-starter", "api", "-module", "example.com/shop", dir)
	e.run(nil, "go", "vet", "./...")

	// Its environments: a file each, for the user only and out of git;
	// production refuses to start without its database, saying so.
	for _, env := range []string{"development", "staging", "production"} {
		info, err := os.Stat(filepath.Join(dir, ".env."+env))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf(".env.%s: %v %v", env, err, info)
		}
	}
	if ignore := e.read(filepath.Join(dir, ".gitignore")); !strings.Contains(ignore, ".env.production") {
		t.Fatalf(".gitignore:\n%s", ignore)
	}
	cmd := exec.Command("go", "run", "./cmd/shop", "check")
	cmd.Dir, cmd.Env = dir, append(os.Environ(), "APP_ENV=production")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "config (production): DATABASE_URL is not set") {
		t.Fatalf("production without DATABASE_URL: %v\n%s", err, out)
	}
	if out := e.esquadro(1, "env", "check", "production", dir); !strings.Contains(out, "DATABASE_URL is required in production") {
		t.Fatalf("esquadro env check production:\n%s", out)
	}
	if out := e.run(nil, "go", "run", "./cmd/shop", "env"); !strings.Contains(out, "postgres://postgres:xxxxx@localhost") || !strings.Contains(out, "APP_NAME") {
		t.Fatalf("shop env:\n%s", out)
	}

	// 2. Running against Postgres, with records, and its telemetry sent to
	// an OTLP endpoint of the test's.
	db := e.postgres()
	e.run(db, "go", "run", "./cmd/shop", "migrate")
	e.createAdmin(db)
	otlp := newOTLPReceiver(t)
	mailURL, mailbox := e.mailpit()
	stop := e.serve(append(db, "OTEL_EXPORTER_OTLP_ENDPOINT="+otlp.URL, "MAIL_URL="+mailURL))
	stopWork := e.work(append(db, "MAIL_URL="+mailURL))
	e.signIn()
	e.securityScenario()
	// The customer's verification email went by the queue and Mailpit; its
	// link verifies the email.
	token := e.verificationToken(mailbox, "bia@example.com")
	e.request("POST", "/identity/customer/verify-email", `{"token":"`+token+`"}`, nil, 204)
	stopWork()
	category := e.call("POST", "/categories", `{"name":"Books"}`, 201)
	product := e.call("POST", "/products", fmt.Sprintf(`{"name":"Dune","price":"39.90","category_id":%q}`, category["id"]), 201)
	stop()
	// The spans of the request, of the API's operation and of its query;
	// logs and metrics too.
	for _, want := range []string{"POST /api/v1/", "CreateCategory", "INSERT"} {
		if !strings.Contains(otlp.got("/v1/traces"), want) {
			t.Errorf("no span %q reached the OTLP endpoint", want)
		}
	}
	if otlp.got("/v1/logs") == "" || otlp.got("/v1/metrics") == "" {
		t.Error("no logs or no metrics reached the OTLP endpoint")
	}

	// 3. Code of one's own, and a generated file changed by hand.
	mine := filepath.Join(dir, "internal", "catalog", "discount.go")
	e.write(mine, "package catalog\n\n// Discounted is written by hand: esquadro must leave it alone.\nfunc Discounted(p Product) bool { return false }\n")
	generated := filepath.Join(dir, "internal", "catalog", "category.go")
	original := e.read(generated)
	e.write(generated, original+"\n// changed by hand\n")
	if out := e.esquadro(1, "gen", dir); !strings.Contains(out, "internal/catalog/category.go") {
		t.Fatalf("gen did not refuse to overwrite the edited file:\n%s", out)
	}
	e.write(generated, original)

	// 4. The model changes: a new entity, a new column, a new default.
	catalog := e.read(filepath.Join(dir, "esquadro", "catalog.esq"))
	before := catalog
	catalog = strings.Replace(catalog, "  status      Status  default(ACTIVE)\n",
		"  status      Status default(INACTIVE)\n  sku         string unique\n  brand       > Brand\n", 1)
	if catalog == before {
		t.Fatal("the example module changed: update the line this test replaces")
	}
	catalog += "\n/// Who makes a product.\nentity Brand {\n  name string required\n}\n"
	// A record with an owner: each customer's own reviews.
	// A report, whose SQL esquadro writes first and the test then writes.
	catalog += "\n/// Products per brand.\nreport ProductsByBrand {\n  brand    string required\n  products int    required\n  @security(read [user, service.reader])\n}\n"
	catalog += "\n/// What a customer thinks of a product.\nentity Review {\n  text     string required\n  customer > Customer required\n  product  > Product required\n  @security(read [owner, user], create owner, update owner, delete [owner, user.admin])\n}\n"
	e.write(filepath.Join(dir, "esquadro", "catalog.esq"), catalog)
	// The TypeScript client too: go test runs its tests against the API.
	appEsq := filepath.Join(dir, "esquadro", "app.esq")
	e.write(appEsq, strings.Replace(e.read(appEsq), "  blueprint api_docs\n", "  blueprint api_docs\n  blueprint client_ts {\n    scope \"acme\"\n  }\n", 1))
	// Service keys: a reader, which the report lets in.
	e.write(appEsq, strings.Replace(e.read(appEsq), "  blueprint security\n", "  blueprint security {\n    services [reader]\n  }\n", 1))
	e.esquadro(0, "gen", dir)
	if !strings.Contains(e.read(filepath.Join(dir, "clients", "ts", "package.json")), `"@acme/shop-client"`) {
		t.Fatal("no TypeScript client")
	}
	reportSQL := filepath.Join(dir, "internal", "catalog", "reports", "products_by_brand.sql")
	if !strings.Contains(e.read(reportSQL), "-- name: ProductsByBrand :many") {
		t.Fatal("esquadro did not write the report's first SQL")
	}
	e.write(reportSQL, `-- name: ProductsByBrand :many
SELECT b.name::text AS brand, count(p.id)::bigint AS products
FROM brands b LEFT JOIN products p ON p.brand_id = b.id
GROUP BY b.name ORDER BY b.name
LIMIT sqlc.arg(row_limit);
`)
	e.esquadro(0, "gen", dir)
	if !strings.Contains(e.read(reportSQL), "LEFT JOIN") {
		t.Fatal("gen wrote over the report's SQL")
	}
	migrations, _ := filepath.Glob(filepath.Join(dir, "migrations", "*.sql"))
	if len(migrations) != 2 {
		t.Fatalf("want the first migration and one for the change, got %v", migrations)
	}
	if got := e.read(mine); !strings.Contains(got, "written by hand") {
		t.Fatal("gen changed a file of the user's")
	}

	// 5. The database with records follows the model, and the service runs.
	e.run(db, "go", "run", "./cmd/shop", "migrate")
	e.run(nil, "go", "vet", "./...")
	stop = e.serve(db)
	defer stop()
	e.signIn()
	kept := e.call("GET", "/products/"+product["id"].(string), "", 200)
	if kept["name"] != "Dune" || kept["price"] != "39.9" { // decimals come back without trailing zeros
		t.Fatalf("the stored product changed: %v", kept)
	}
	brand := e.call("POST", "/brands", `{"name":"Ace"}`, 201)
	added := e.call("POST", "/products", fmt.Sprintf(`{"name":"Emma","price":"12.00","category_id":%q,"brand_id":%q,"sku":"EM-1"}`,
		category["id"], brand["id"]), 201)
	if added["status"] != "INACTIVE" || added["sku"] != "EM-1" {
		t.Fatalf("the new product does not follow the new model: %v", added)
	}
	refused := e.call("POST", "/products", fmt.Sprintf(`{"name":"Copy","price":"1","category_id":%q,"sku":"EM-1"}`, category["id"]), 422)
	// Lists as the model declares: ordered, filtered, searched, by page.
	names := func(page map[string]any) []string {
		var out []string
		for _, item := range page["items"].([]any) {
			out = append(out, item.(map[string]any)["name"].(string))
		}
		return out
	}
	if got := names(e.call("GET", "/products?sort=-price", "", 200)); !slices.Equal(got, []string{"Dune", "Emma"}) {
		t.Fatalf("by price: %v", got)
	}
	if got := names(e.call("GET", "/products?price_max=20", "", 200)); !slices.Equal(got, []string{"Emma"}) {
		t.Fatalf("up to 20: %v", got)
	}
	if got := names(e.call("GET", "/products?q=dune", "", 200)); !slices.Equal(got, []string{"Dune"}) {
		t.Fatalf("search: %v", got)
	}
	if page := e.call("GET", "/products?page=1&per_page=1", "", 200); page["total"] != 2.0 || page["total_pages"] != 2.0 {
		t.Fatalf("by page: %v", page)
	}
	if !strings.Contains(fmt.Sprint(refused["errors"]), "taken") {
		t.Fatalf("a taken sku was not refused: %v", refused)
	}

	e.ownerScenario(added["id"].(string))
	e.safeWrites(added, category["id"].(string), brand["id"].(string))
	e.report()
	e.serviceKeys(db)

	// 6. Its own tests pass on the new model.
	e.run(nil, "go", "test", "./...")
}

type lifecycle struct {
	t    *testing.T
	dir  string
	base string // the API's URL while serving
	auth string // the Authorization header of the calls: the admin's session
}

const adminPassword = "a long admin passphrase"

// createAdmin makes the first admin with the app's command: the password
// from stdin, never in the arguments.
func (e *lifecycle) createAdmin(db []string) {
	e.t.Helper()
	create := exec.Command("go", "run", "./cmd/shop", "security", "create", "user", "admin@example.com", "name=Admin", "-role", "admin", "-password-stdin")
	create.Dir, create.Env, create.Stdin = e.dir, append(os.Environ(), db...), strings.NewReader(adminPassword+"\n")
	if out, err := create.CombinedOutput(); err != nil || !strings.Contains(string(out), "created user admin@example.com") {
		e.t.Fatalf("security create: %v\n%s", err, out)
	}
}

// signIn signs the admin in, for the calls that follow.
func (e *lifecycle) signIn() {
	e.t.Helper()
	res := e.request("POST", "/identity/user/sign-in", `{"email":"admin@example.com","password":"`+adminPassword+`","cookie":false}`, nil, 200)
	token, _ := res.body["session_token"].(string)
	if token == "" {
		e.t.Fatalf("sign-in: %v", res.body)
	}
	e.auth = "Bearer " + token
}

// securityScenario checks who may do what, through the running service.
func (e *lifecycle) securityScenario() {
	e.t.Helper()
	product := `{"name":"Dune","price":"1","category_id":"01a1e4af-0000-7000-8000-000000000000"}`
	// Anyone reads the catalog; no one unsigned changes it.
	e.request("GET", "/api/v1/products", "", nil, 200)
	e.request("POST", "/api/v1/categories", `{"name":"X"}`, nil, 401)

	// A customer signs up and in, and may not change the catalog.
	e.request("POST", "/identity/customer/sign-up", `{"email":"bia@example.com","password":"correct horse battery staple","name":"Bia"}`, nil, 202)
	customer := e.request("POST", "/identity/customer/sign-in", `{"email":"bia@example.com","password":"correct horse battery staple","cookie":false}`, nil, 200)
	asCustomer := map[string]string{"Authorization": "Bearer " + customer.body["session_token"].(string)}
	e.request("POST", "/api/v1/categories", `{"name":"X"}`, asCustomer, 403)

	// A customer's JWT is refused where a user's is needed.
	jwt := e.request("POST", "/identity/customer/token", "", asCustomer, 200)
	e.request("GET", "/identity/user/session", "", map[string]string{"Authorization": "Bearer " + jwt.body["token"].(string)}, 401)

	// With the session cookie, a change needs the CSRF token.
	admin := e.request("POST", "/identity/user/sign-in", `{"email":"admin@example.com","password":"`+adminPassword+`"}`, nil, 200)
	cookie := map[string]string{"Cookie": strings.Split(admin.header.Get("Set-Cookie"), ";")[0]}
	e.request("POST", "/api/v1/products", product, cookie, 403)
	cookie["X-CSRF-Token"] = admin.body["csrf_token"].(string)
	e.request("POST", "/api/v1/products", product, cookie, 422) // let in: the category does not exist
}

// ownerScenario checks the owner rule through the running service: each
// customer sees and changes only its own reviews.
func (e *lifecycle) ownerScenario(product string) {
	e.t.Helper()
	customer := func(email string) map[string]string {
		e.request("POST", "/identity/customer/sign-up", `{"email":"`+email+`","password":"correct horse battery staple","name":"Someone"}`, nil, 202)
		signedIn := e.request("POST", "/identity/customer/sign-in", `{"email":"`+email+`","password":"correct horse battery staple","cookie":false}`, nil, 200)
		return map[string]string{"Authorization": "Bearer " + signedIn.body["session_token"].(string)}
	}
	carla, davi := customer("carla@example.com"), customer("davi@example.com")
	carlaID := e.request("GET", "/identity/customer/session", "", carla, 200).body["id"].(string)

	// Carla's review is hers, whatever the body says.
	review := e.request("POST", "/api/v1/reviews", `{"text":"Great","product_id":"`+product+`"}`, carla, 201).body
	if review["customer_id"] != carlaID {
		e.t.Fatalf("the review's owner: %v, want %s", review["customer_id"], carlaID)
	}
	path := "/api/v1/reviews/" + review["id"].(string)

	// Davi does not see it, nor change it, and lists none.
	e.request("GET", path, "", davi, 404)
	e.request("PUT", path, `{"text":"Bad","product_id":"`+product+`"}`, davi, 404)
	e.request("DELETE", path, "", davi, 404)
	if items := e.request("GET", "/api/v1/reviews", "", davi, 200).body["items"].([]any); len(items) != 0 {
		e.t.Fatalf("davi lists %v", items)
	}
	// No one signed in learns nothing; an admin sees it.
	e.request("GET", path, "", nil, 401)
	e.request("GET", path, "", map[string]string{"Authorization": e.auth}, 200)

	// Carla lists it, and cannot hand it over to Davi.
	if items := e.request("GET", "/api/v1/reviews", "", carla, 200).body["items"].([]any); len(items) != 1 {
		e.t.Fatalf("carla lists %v", items)
	}
	daviID := e.request("GET", "/identity/customer/session", "", davi, 200).body["id"].(string)
	kept := e.request("PUT", path, `{"text":"Still great","product_id":"`+product+`","customer_id":"`+daviID+`"}`, carla, 200).body
	if kept["customer_id"] != carlaID || kept["text"] != "Still great" {
		e.t.Fatalf("the review after carla's update: %v", kept)
	}

	// What each may do, as the API tells the screens: Carla changes and
	// deletes her review; Davi may write reviews of his own, but not hers;
	// no one signed in may not write any.
	if can := fmt.Sprint(e.request("GET", path, "", carla, 200).body["can"]); can != "[update delete]" {
		e.t.Fatalf("carla's review: can %s", can)
	}
	if can := fmt.Sprint(e.request("GET", "/api/v1/reviews", "", davi, 200).body["can"]); can != "[create]" {
		e.t.Fatalf("davi's reviews: can %s", can)
	}
	if can := fmt.Sprint(e.request("GET", "/api/v1/products", "", nil, 200).body["can"]); can != "[]" {
		e.t.Fatalf("the products, to no one: can %s", can)
	}

	// A PATCH changes only the text. The review comes with its product and
	// only the fields asked for; its customer only to whom may read
	// customers (staff), not to Carla.
	e.request("PATCH", path, `{"text":"Even better"}`, carla, 200)
	query := path + "?include=customer,product&fields=text,customer.name,product.name"
	read := e.request("GET", query, "", carla, 200).body
	reviewed, _ := read["product"].(map[string]any)
	if read["text"] != "Even better" || read["product_id"] != nil || read["customer"] != nil || reviewed["id"] != product || reviewed["name"] == nil {
		e.t.Fatalf("the review with include and fields, as carla: %v", read)
	}
	owner, _ := e.request("GET", query, "", map[string]string{"Authorization": e.auth}, 200).body["customer"].(map[string]any)
	if owner["id"] != carlaID || owner["name"] != "Someone" || owner["created_at"] != nil {
		e.t.Fatalf("the review's customer, as an admin: %v", owner)
	}
}

// report reads the report through the running service: whoever signed in
// as a user sees the numbers; no one signed in, not.
func (e *lifecycle) report() {
	e.t.Helper()
	e.request("GET", "/api/v1/reports/products-by-brand", "", nil, 401)
	rows := e.request("GET", "/api/v1/reports/products-by-brand", "", map[string]string{"Authorization": e.auth}, 200).body["items"].([]any)
	found := false
	for _, r := range rows {
		row := r.(map[string]any)
		if row["brand"] == "Ace" && row["products"] == float64(1) {
			found = true
		}
	}
	if !found {
		e.t.Fatalf("the report: %v", rows)
	}
}

// serviceKeys creates a service key with the app's command, and uses it:
// the report lets service.reader in, the catalog's changes do not, and a
// revoked key is refused.
func (e *lifecycle) serviceKeys(db []string) {
	e.t.Helper()
	create := exec.Command("go", "run", "./cmd/shop", "security", "key", "create", "studio", "-role", "reader")
	create.Dir, create.Env = e.dir, append(os.Environ(), db...)
	out, err := create.Output()
	key := strings.TrimSpace(string(out))
	if err != nil || !strings.HasPrefix(key, "sk_") {
		e.t.Fatalf("security key create: %v\n%s", err, out)
	}
	asKey := map[string]string{"Authorization": "Bearer " + key}
	e.request("GET", "/api/v1/reports/products-by-brand", "", asKey, 200)
	e.request("POST", "/api/v1/categories", `{"name":"Y"}`, asKey, 403)
	if list := e.run(db, "go", "run", "./cmd/shop", "security", "key", "list"); !strings.Contains(list, "studio") || strings.Contains(list, key) {
		e.t.Fatalf("security key list:\n%s", list)
	}
	e.run(db, "go", "run", "./cmd/shop", "security", "key", "revoke", "studio")
	e.request("GET", "/api/v1/reports/products-by-brand", "", asKey, 401)
}

// safeWrites checks If-Match and Idempotency-Key through the running
// service: an old version is refused, and a POST sent again makes one
// record.
func (e *lifecycle) safeWrites(product map[string]any, category, brand string) {
	e.t.Helper()
	admin := map[string]string{"Authorization": e.auth}
	path := "/api/v1/products/" + product["id"].(string)
	read := e.request("GET", path, "", admin, 200)
	etag := read.header.Get("ETag")
	if etag != fmt.Sprintf(`"%v"`, read.body["version"]) {
		e.t.Fatalf("ETag %q, version %v", etag, read.body["version"])
	}
	change := fmt.Sprintf(`{"name":"Emma","price":"13.00","category_id":%q,"brand_id":%q,"sku":"EM-1"}`, category, brand)
	withETag := map[string]string{"Authorization": e.auth, "If-Match": etag}
	e.request("PUT", path, change, withETag, 200)
	// Someone changed it since that ETag: refused, nothing lost.
	if refused := e.request("PUT", path, change, withETag, 412); refused.body["code"] != "precondition_failed" {
		e.t.Fatalf("an old ETag: %v", refused.body)
	}

	// The same POST twice, as a retry: one brand.
	withKey := map[string]string{"Authorization": e.auth, "Idempotency-Key": "e2e-brand-1"}
	first := e.request("POST", "/api/v1/brands", `{"name":"Zeta"}`, withKey, 201)
	again := e.request("POST", "/api/v1/brands", `{"name":"Zeta"}`, withKey, 201)
	if first.body["id"] != again.body["id"] || again.header.Get("Idempotent-Replayed") != "true" {
		e.t.Fatalf("a retry made another brand: %v, %v", first.body, again.body)
	}
}

// answer is a response: its status, headers and JSON object.
type answer struct {
	header http.Header
	body   map[string]any
}

// request makes a request to the service, with headers, and wants its
// status.
func (e *lifecycle) request(method, path, body string, headers map[string]string, status int) answer {
	e.t.Helper()
	req, err := http.NewRequest(method, e.base+path, strings.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != status {
		e.t.Fatalf("%s %s: %d, want %d: %s", method, path, res.StatusCode, status, raw)
	}
	a := answer{header: res.Header}
	if len(raw) > 0 && raw[0] == '{' {
		if err := json.Unmarshal(raw, &a.body); err != nil {
			e.t.Fatalf("%s %s: %v: %s", method, path, err, raw)
		}
	}
	return a
}

// esquadro runs an esquadro command in-process and wants its exit code.
func (e *lifecycle) esquadro(want int, args ...string) string {
	e.t.Helper()
	var out bytes.Buffer
	if code := run(args, &out, &out); code != want {
		e.t.Fatalf("esquadro %s: exit %d, want %d\n%s", strings.Join(args, " "), code, want, out.String())
	}
	return out.String()
}

func (e *lifecycle) run(env []string, name string, args ...string) string {
	e.t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = e.dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		e.t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (e *lifecycle) read(path string) string {
	e.t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatal(err)
	}
	return string(b)
}

func (e *lifecycle) write(path, content string) {
	e.t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// postgres starts a Postgres for the test and returns its DATABASE_URL.
func (e *lifecycle) postgres() []string {
	e.t.Helper()
	out, err := exec.Command("docker", "run", "-d", "--rm", "-e", "POSTGRES_PASSWORD=postgres", "-e", "POSTGRES_DB=shop",
		"-p", "127.0.0.1::5432", "postgres:17-alpine").Output()
	if err != nil {
		e.t.Fatalf("docker run: %v", err)
	}
	container := strings.TrimSpace(string(out))
	e.t.Cleanup(func() { exec.Command("docker", "rm", "-f", container).Run() })
	port, err := exec.Command("docker", "port", container, "5432/tcp").Output()
	if err != nil {
		e.t.Fatal(err)
	}
	_, hostPort, _ := strings.Cut(strings.TrimSpace(strings.Split(string(port), "\n")[0]), ":")
	deadline := time.Now().Add(60 * time.Second)
	// Over TCP: the image's first, temporary server listens on the Unix
	// socket only, and is restarted once the database is initialized.
	for exec.Command("docker", "exec", container, "psql", "-h", "127.0.0.1", "-U", "postgres", "-d", "shop", "-c", "select 1").Run() != nil {
		if time.Now().After(deadline) {
			e.t.Fatal("Postgres did not start")
		}
		time.Sleep(300 * time.Millisecond)
	}
	return []string{"DATABASE_URL=postgres://postgres:postgres@127.0.0.1:" + hostPort + "/shop?sslmode=disable"}
}

// mailpit starts Mailpit: the MAIL_URL of its SMTP, and the address of its
// API.
func (e *lifecycle) mailpit() (string, string) {
	e.t.Helper()
	out, err := exec.Command("docker", "run", "-d", "--rm", "-p", "127.0.0.1::1025", "-p", "127.0.0.1::8025", "axllent/mailpit:v1.33.0").Output()
	if err != nil {
		e.t.Fatalf("docker run mailpit: %v", err)
	}
	container := strings.TrimSpace(string(out))
	e.t.Cleanup(func() { exec.Command("docker", "rm", "-f", container).Run() })
	port := func(p string) string {
		out, err := exec.Command("docker", "port", container, p+"/tcp").Output()
		if err != nil {
			e.t.Fatal(err)
		}
		_, hostPort, _ := strings.Cut(strings.TrimSpace(strings.Split(string(out), "\n")[0]), ":")
		return hostPort
	}
	return "smtp://127.0.0.1:" + port("1025"), "http://127.0.0.1:" + port("8025")
}

// verificationToken waits for the verification email to address in
// Mailpit, and answers the token of its link.
func (e *lifecycle) verificationToken(mailbox, address string) string {
	e.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		res, err := http.Get(mailbox + "/api/v1/search?query=" + url.QueryEscape("to:"+address+" subject:\"Verify your email\""))
		if err == nil {
			var found struct{ Messages []struct{ ID string } }
			json.NewDecoder(res.Body).Decode(&found)
			res.Body.Close()
			if len(found.Messages) > 0 {
				res, err := http.Get(mailbox + "/api/v1/message/" + found.Messages[0].ID)
				if err != nil {
					e.t.Fatal(err)
				}
				var msg struct{ Text string }
				json.NewDecoder(res.Body).Decode(&msg)
				res.Body.Close()
				_, token, ok := strings.Cut(msg.Text, "token=")
				if !ok {
					e.t.Fatalf("no link in the email:\n%s", msg.Text)
				}
				return strings.Fields(token)[0]
			}
		}
		if time.Now().After(deadline) {
			e.t.Fatal("the verification email did not reach Mailpit")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// work starts the app's background work (jobs), and returns how to stop
// it.
func (e *lifecycle) work(env []string) func() {
	e.t.Helper()
	cmd := exec.Command(filepath.Join(e.dir, "bin", "shop"), "run")
	cmd.Dir = e.dir
	cmd.Env = append(os.Environ(), env...)
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		e.t.Fatal(err)
	}
	return func() {
		cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			cmd.Process.Kill()
			<-done
		}
	}
}

// serve builds and starts the service, and returns how to stop it.
func (e *lifecycle) serve(db []string) func() {
	e.t.Helper()
	e.run(nil, "go", "build", "-o", "bin/shop", "./cmd/shop")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		e.t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	cmd := exec.Command(filepath.Join(e.dir, "bin", "shop"))
	// Where the app runs: it reads its .env.development there.
	cmd.Dir = e.dir
	cmd.Env = append(append(os.Environ(), db...), fmt.Sprintf("PORT=%d", port))
	var logs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &logs, &logs
	if err := cmd.Start(); err != nil {
		e.t.Fatal(err)
	}
	// Stopped as a platform stops it: SIGTERM, then a kill if it hangs.
	stop := func() {
		cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			cmd.Process.Kill()
			<-done
		}
	}
	e.base = fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(30 * time.Second)
	for {
		res, err := http.Get(e.base + "/healthz")
		if err == nil && res.StatusCode == 204 {
			res.Body.Close()
			return stop
		}
		if time.Now().After(deadline) {
			stop()
			e.t.Fatalf("the service did not start:\n%s", logs.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// call makes a request to the API, signed in as the admin, and wants its
// status; it returns the JSON object answered, if any.
func (e *lifecycle) call(method, path, body string, status int) map[string]any {
	e.t.Helper()
	return e.request(method, "/api/v1"+path, body, map[string]string{"Authorization": e.auth}, status).body
}

// otlpReceiver is an OTLP/HTTP endpoint that keeps what it is sent, by
// path: protobuf, in which names are plain strings.
type otlpReceiver struct {
	*httptest.Server
	mu     sync.Mutex
	bodies map[string][]byte
}

func newOTLPReceiver(t *testing.T) *otlpReceiver {
	o := &otlpReceiver{bodies: map[string][]byte{}}
	o.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		o.mu.Lock()
		o.bodies[r.URL.Path] = append(o.bodies[r.URL.Path], body...)
		o.mu.Unlock()
	}))
	t.Cleanup(o.Close)
	return o
}

func (o *otlpReceiver) got(path string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return string(o.bodies[path])
}
