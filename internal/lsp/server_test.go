package lsp

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// client talks to a server the way an editor does.
type client struct {
	t     *testing.T
	conn  *conn
	in    *io.PipeWriter
	next  int
	diags chan published
	resps chan *message
}

type published struct {
	URI         string          `json:"uri"`
	Diagnostics []lspDiagnostic `json:"diagnostics"`
}

func start(t *testing.T) *client {
	t.Helper()
	toServer, fromClient := io.Pipe()
	toClient, fromServer := io.Pipe()
	s := &Server{Version: "test", CacheDir: t.TempDir(), Delay: 10 * time.Millisecond}
	done := make(chan error, 1)
	go func() { done <- s.Serve(toServer, fromServer); fromServer.Close() }()
	c := &client{t: t, conn: newConn(toClient, fromClient), in: fromClient,
		diags: make(chan published, 64), resps: make(chan *message, 8)}
	go func() {
		for {
			m, err := c.conn.read()
			if err != nil {
				return
			}
			if m.Method == "textDocument/publishDiagnostics" {
				var p published
				json.Unmarshal(m.Params, &p)
				c.diags <- p
			} else {
				c.resps <- m
			}
		}
	}()
	t.Cleanup(func() {
		c.notify("exit", nil)
		fromClient.Close()
		<-done
	})
	if res := c.request("initialize", map[string]any{}); !strings.Contains(string(res.Result), `"documentFormattingProvider":true`) {
		t.Fatalf("initialize: %s", res.Result)
	}
	c.notify("initialized", map[string]any{})
	return c
}

func (c *client) notify(method string, params any) {
	c.t.Helper()
	data, _ := json.Marshal(params)
	if err := c.conn.write(&message{Method: method, Params: data}); err != nil {
		c.t.Fatal(err)
	}
}

func (c *client) request(method string, params any) *message {
	c.t.Helper()
	c.next++
	data, _ := json.Marshal(params)
	id, _ := json.Marshal(c.next)
	if err := c.conn.write(&message{ID: id, Method: method, Params: data}); err != nil {
		c.t.Fatal(err)
	}
	select {
	case m := <-c.resps:
		return m
	case <-time.After(5 * time.Second):
		c.t.Fatalf("no answer to %s", method)
	}
	return nil
}

func (c *client) open(path, src string) {
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": uriOf(path), "languageId": "esquadro", "version": 1, "text": src}})
}

// waitFor returns the diagnostics published for path once they satisfy ok.
func (c *client) waitFor(path string, ok func([]lspDiagnostic) bool) []lspDiagnostic {
	c.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case p := <-c.diags:
			if p.URI == uriOf(path) && ok(p.Diagnostics) {
				return p.Diagnostics
			}
		case <-deadline:
			c.t.Fatalf("no matching diagnostics for %s", path)
		}
	}
}

func projectDir(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, src := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const appEsq = "app shop {\n  module \"example.com/shop\"\n  blueprint app\n  blueprint domain\n}\n"

func TestErrorsAcrossFilesAppearAndClearWhileTyping(t *testing.T) {
	catalog := "package catalog\n\nentity Product {\n  name     string\n  category > Categry\n}\n"
	root := projectDir(t, map[string]string{
		"esquadro/app.esq":      appEsq,
		"esquadro/category.esq": "package catalog\n\nentity Category {\n  name string\n}\n",
	})
	path := filepath.Join(root, "esquadro", "catalog.esq")
	c := start(t)
	c.open(path, catalog) // never saved: only the editor has it

	got := c.waitFor(path, func(d []lspDiagnostic) bool { return len(d) > 0 })
	d := got[0]
	if !strings.HasPrefix(d.Message, `unknown entity "Categry"`) || !strings.Contains(d.Message, "\nhelp: ") ||
		d.Range != (Range{Start: Position{4, 13}, End: Position{4, 20}}) || d.Severity != 1 || d.Source != "esquadro" {
		t.Fatalf("diagnostic %+v", d)
	}

	// Fixing the typo, one character at a time as typed, clears it.
	c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uriOf(path), "version": 2},
		"contentChanges": []any{map[string]any{"range": Range{Start: Position{4, 18}, End: Position{4, 18}}, "text": "o"}},
	})
	c.waitFor(path, func(d []lspDiagnostic) bool { return len(d) == 0 })
}

func TestBlueprintOptionsAreChecked(t *testing.T) {
	root := projectDir(t, map[string]string{"esquadro/catalog.esq": "package catalog\n\nentity Product {\n  name string\n}\n"})
	path := filepath.Join(root, "esquadro", "app.esq")
	c := start(t)
	c.open(path, "app shop {\n  module \"example.com/shop\"\n  blueprint app\n  blueprint domain\n  blueprint http {\n    prot 9000\n  }\n}\n")
	got := c.waitFor(path, func(d []lspDiagnostic) bool { return len(d) > 0 })
	if !strings.HasPrefix(got[0].Message, `blueprint http has no option "prot"`) || got[0].Range.Start != (Position{5, 4}) {
		t.Fatalf("%+v", got)
	}
}

func TestLooseFilesGetTheirSyntaxChecked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.esq")
	c := start(t)
	c.open(path, "entity Product {\n  name\n}\n")
	got := c.waitFor(path, func(d []lspDiagnostic) bool { return len(d) > 0 })
	if got[0].Severity != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestFormattingIsEsquadroFmt(t *testing.T) {
	path := filepath.Join(projectDir(t, map[string]string{"esquadro/app.esq": appEsq}), "esquadro", "catalog.esq")
	c := start(t)
	c.open(path, "package catalog\nentity Product{\nname string required\n  price decimal\n}\n")
	res := c.request("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": uriOf(path)}})
	var edits []textEdit
	if err := json.Unmarshal(res.Result, &edits); err != nil || len(edits) != 1 {
		t.Fatalf("edits %s, %v", res.Result, err)
	}
	want := "package catalog\n\nentity Product {\n  name  string required\n  price decimal\n}\n"
	if edits[0].NewText != want || edits[0].Range.Start != (Position{}) || edits[0].Range.End != (Position{5, 0}) {
		t.Fatalf("edit %+v", edits[0])
	}

	// A file with a syntax error is left alone.
	c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uriOf(path), "version": 2},
		"contentChanges": []any{map[string]any{"text": "entity Product {\n  name\n"}},
	})
	res = c.request("textDocument/formatting", map[string]any{"textDocument": map[string]any{"uri": uriOf(path)}})
	if string(res.Result) != "[]" {
		t.Fatalf("formatted a broken file: %s", res.Result)
	}
}

func TestUnknownRequestsAreRefused(t *testing.T) {
	c := start(t)
	if res := c.request("textDocument/hover", map[string]any{}); res.Error == nil || res.Error.Code != codeMethodNotFound {
		t.Fatalf("%+v", res)
	}
}

func TestPositionsCountUTF16(t *testing.T) {
	tx := newText("ab\n/// café 😀 x\n")
	// "x" is rune column 12 of line 2, unit 12: the emoji takes 2 units.
	if p := tx.fromRunes(2, 12); p != (Position{1, 12}) {
		t.Fatalf("fromRunes = %+v", p)
	}
	if off := tx.offset(Position{1, 12}); tx.src[off:off+1] != "x" {
		t.Fatalf("offset %d", off)
	}
	if got := apply(tx.src, &Range{Start: Position{1, 4}, End: Position{1, 8}}, "chá"); got != "ab\n/// chá 😀 x\n" {
		t.Fatalf("apply = %q", got)
	}
}

func TestURIsOfWindowsPaths(t *testing.T) {
	// Editors on Windows send the drive in lower case, its colon escaped.
	if p, ok := pathOf("file:///c%3A/Users/me/esquadro/app.esq"); !ok || !strings.HasPrefix(filepath.ToSlash(p), "C:/Users/me/esquadro/app.esq") {
		t.Fatalf("pathOf = %q, %v", p, ok)
	}
	if u := uriOf("/home/me/app.esq"); u != "file:///home/me/app.esq" {
		t.Fatalf("uriOf = %q", u)
	}
	if p, _ := pathOf(uriOf("/home/me/app.esq")); p != filepath.FromSlash("/home/me/app.esq") {
		t.Fatalf("round trip = %q", p)
	}
}
