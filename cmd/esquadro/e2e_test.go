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
	"os"
	"os/exec"
	"path/filepath"
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

	// 2. Running against Postgres, with records, and its telemetry sent to
	// an OTLP endpoint of the test's.
	db := e.postgres()
	e.run(db, "go", "run", "./cmd/shop", "migrate")
	otlp := newOTLPReceiver(t)
	stop := e.serve(append(db, "OTEL_EXPORTER_OTLP_ENDPOINT="+otlp.URL))
	category := e.call("POST", "/categories", `{"name":"Books"}`, 201)
	product := e.call("POST", "/products", fmt.Sprintf(`{"name":"Dune","price":"39.90","categoryId":%q}`, category["id"]), 201)
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
	e.write(filepath.Join(dir, "esquadro", "catalog.esq"), catalog)
	e.esquadro(0, "gen", dir)
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
	kept := e.call("GET", "/products/"+product["id"].(string), "", 200)
	if kept["name"] != "Dune" || kept["price"] != "39.9" { // decimals come back without trailing zeros
		t.Fatalf("the stored product changed: %v", kept)
	}
	brand := e.call("POST", "/brands", `{"name":"Ace"}`, 201)
	added := e.call("POST", "/products", fmt.Sprintf(`{"name":"Emma","price":"12.00","categoryId":%q,"brandId":%q,"sku":"EM-1"}`,
		category["id"], brand["id"]), 201)
	if added["status"] != "INACTIVE" || added["sku"] != "EM-1" {
		t.Fatalf("the new product does not follow the new model: %v", added)
	}
	refused := e.call("POST", "/products", fmt.Sprintf(`{"name":"Copy","price":"1","categoryId":%q,"sku":"EM-1"}`, category["id"]), 422)
	if !strings.Contains(fmt.Sprint(refused["errors"]), "taken") {
		t.Fatalf("a taken sku was not refused: %v", refused)
	}

	// 6. Its own tests pass on the new model.
	e.run(nil, "go", "test", "./...")
}

type lifecycle struct {
	t    *testing.T
	dir  string
	base string // the API's URL while serving
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

// call makes a request to the API and wants its status; it returns the JSON
// object answered, if any.
func (e *lifecycle) call(method, path, body string, status int) map[string]any {
	e.t.Helper()
	req, err := http.NewRequest(method, e.base+"/api/v1"+path, strings.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
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
	var doc map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &doc); err != nil {
			e.t.Fatalf("%s %s: %v: %s", method, path, err, raw)
		}
	}
	return doc
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
