package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/patrickluzdev/esquadro/internal/diag"
	"github.com/patrickluzdev/esquadro/internal/dsl"
)

// Server is the language server of one editor session.
type Server struct {
	// Version is reported to the editor.
	Version string
	// CacheDir is where blueprints from git are cached.
	CacheDir string
	// Delay is how long the server waits after a change before checking,
	// so that typing does not check on every key.
	Delay time.Duration
	// Log receives the server's own problems.
	Log io.Writer

	conn *conn
	// mu guards docs, published and timer.
	mu        sync.Mutex
	docs      map[string]string // open documents by absolute path
	published map[string]bool   // files whose last diagnostics were not empty
	timer     *time.Timer
	// checking serializes checks.
	checking sync.Mutex
}

// Serve answers the editor on r and w until it exits.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	s.conn = newConn(r, w)
	s.docs = map[string]string{}
	s.published = map[string]bool{}
	if s.Log == nil {
		s.Log = io.Discard
	}
	for {
		m, err := s.conn.read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if m.Method == "exit" {
			s.stopTimer()
			return nil
		}
		result, rerr := s.handle(m)
		if !m.isRequest() {
			if rerr != nil {
				fmt.Fprintf(s.Log, "esquadro lsp: %s: %s\n", m.Method, rerr.Message)
			}
			continue
		}
		reply := &message{ID: m.ID, Error: rerr}
		if rerr == nil {
			if reply.Result, err = json.Marshal(result); err != nil {
				return err
			}
		}
		if err := s.conn.write(reply); err != nil {
			return err
		}
	}
}

// handle answers one message: the result of a request, or nil.
func (s *Server) handle(m *message) (any, *rpcError) {
	switch m.Method {
	case "initialize":
		return map[string]any{
			"capabilities": map[string]any{
				"positionEncoding":           "utf-16",
				"textDocumentSync":           map[string]any{"openClose": true, "change": 2, "save": map[string]any{}},
				"documentFormattingProvider": true,
			},
			"serverInfo": map[string]any{"name": "esquadro", "version": s.Version},
		}, nil
	case "initialized", "$/cancelRequest", "$/setTrace":
		return nil, nil
	case "shutdown":
		s.stopTimer()
		return nil, nil
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil {
			return nil, invalid(err)
		}
		s.edit(p.TextDocument.URI, func(string) string { return p.TextDocument.Text })
	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Range *Range `json:"range"`
				Text  string `json:"text"`
			} `json:"contentChanges"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil {
			return nil, invalid(err)
		}
		s.edit(p.TextDocument.URI, func(src string) string {
			for _, c := range p.ContentChanges {
				src = apply(src, c.Range, c.Text)
			}
			return src
		})
	case "textDocument/didClose":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil {
			return nil, invalid(err)
		}
		if path, ok := pathOf(p.TextDocument.URI); ok {
			s.mu.Lock()
			delete(s.docs, path)
			s.mu.Unlock()
			s.schedule()
		}
	case "textDocument/didSave", "workspace/didChangeWatchedFiles":
		s.schedule()
	case "textDocument/formatting":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if err := json.Unmarshal(m.Params, &p); err != nil {
			return nil, invalid(err)
		}
		return s.format(p.TextDocument.URI), nil
	default:
		if m.isRequest() {
			return nil, &rpcError{Code: codeMethodNotFound, Message: "method not supported: " + m.Method}
		}
	}
	return nil, nil
}

func invalid(err error) *rpcError { return &rpcError{Code: codeInvalidParams, Message: err.Error()} }

// edit changes an open document and schedules a check.
func (s *Server) edit(uri string, change func(string) string) {
	path, ok := pathOf(uri)
	if !ok {
		return
	}
	s.mu.Lock()
	s.docs[path] = change(s.docs[path])
	s.mu.Unlock()
	s.schedule()
}

// schedule checks after Delay without further changes.
func (s *Server) schedule() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.Delay, s.check)
}

func (s *Server) stopTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
}

// check checks what the open documents belong to and publishes the
// diagnostics of every file, an empty list for those that became clean.
func (s *Server) check() {
	s.checking.Lock()
	defer s.checking.Unlock()
	s.mu.Lock()
	docs := maps.Clone(s.docs)
	s.mu.Unlock()

	results := checkAll(docs, s.CacheDir)
	s.mu.Lock()
	for path := range s.published {
		if _, ok := results[path]; !ok {
			results[path] = nil
		}
	}
	s.mu.Unlock()
	for path, list := range results {
		src, ok := docs[path]
		if !ok {
			data, err := os.ReadFile(path)
			if err != nil && len(list) > 0 {
				continue
			}
			src = string(data)
		}
		if err := s.publish(path, newText(src), list); err != nil {
			fmt.Fprintf(s.Log, "esquadro lsp: %v\n", err)
		}
		s.mu.Lock()
		if len(list) > 0 {
			s.published[path] = true
		} else {
			delete(s.published, path)
		}
		s.mu.Unlock()
	}
}

// lspDiagnostic is a diagnostic as the protocol sends it.
type lspDiagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

func (s *Server) publish(path string, t text, list []diag.Diagnostic) error {
	out := make([]lspDiagnostic, 0, len(list))
	for _, d := range list {
		out = append(out, toLSP(t, d))
	}
	params, err := json.Marshal(map[string]any{"uri": uriOf(path), "diagnostics": out})
	if err != nil {
		return err
	}
	return s.conn.write(&message{Method: "textDocument/publishDiagnostics", Params: params})
}

// toLSP converts an esquadro diagnostic to the protocol's: its range in
// UTF-16, and its suggestion on a line of its own, as in the terminal.
func toLSP(t text, d diag.Diagnostic) lspDiagnostic {
	endLine, endCol := d.End.Line, d.End.Col
	if !d.End.IsValid() || endLine != d.Pos.Line || endCol <= d.Pos.Col {
		endLine, endCol = d.Pos.Line, t.wordEnd(d.Pos.Line, d.Pos.Col)
	}
	msg := d.Message
	if d.Suggestion != "" {
		msg += "\nhelp: " + d.Suggestion
	}
	severity := 1
	if d.Severity == diag.Warning {
		severity = 2
	}
	return lspDiagnostic{
		Range:    Range{Start: t.fromRunes(d.Pos.Line, d.Pos.Col), End: t.fromRunes(endLine, endCol)},
		Severity: severity,
		Source:   "esquadro",
		Message:  msg,
	}
}

// textEdit is a replacement of a span of a document.
type textEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// format formats an open document as esquadro fmt does: one edit for the
// whole document, or none when it is formatted or has a syntax error.
func (s *Server) format(uri string) []textEdit {
	path, ok := pathOf(uri)
	if !ok {
		return []textEdit{}
	}
	s.mu.Lock()
	src, open := s.docs[path]
	s.mu.Unlock()
	if !open {
		return []textEdit{}
	}
	f, diags := dsl.Parse(path, []byte(src))
	if diags.HasErrors() {
		return []textEdit{}
	}
	out := string(dsl.Format(f))
	if out == src {
		return []textEdit{}
	}
	return []textEdit{{Range: Range{End: newText(src).end()}, NewText: out}}
}

// pathOf is the absolute path of a file:// URI.
func pathOf(uri string) (string, bool) {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return "", false
	}
	return filepath.Clean(filepath.FromSlash(u.Path)), true
}

// uriOf is the file:// URI of an absolute path.
func uriOf(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
