// Package lsp is esquadro's language server: it shows the errors of .esq
// files in an editor while they are typed, the same esquadro check reports,
// and formats them as esquadro fmt does. It speaks the Language Server
// Protocol over stdin and stdout.
package lsp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
)

// message is a JSON-RPC 2.0 request, response or notification.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// isRequest reports whether m expects a response.
func (m *message) isRequest() bool { return len(m.ID) > 0 && m.Method != "" }

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC error codes the server answers with.
const (
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// conn reads and writes messages framed by a Content-Length header.
type conn struct {
	r  *bufio.Reader
	mu sync.Mutex
	w  io.Writer
}

func newConn(r io.Reader, w io.Writer) *conn {
	return &conn{r: bufio.NewReader(r), w: w}
}

// read returns the next message; io.EOF when the client is gone.
func (c *conn) read() (*message, error) {
	header, err := textproto.NewReader(c.r).ReadMIMEHeader()
	if err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(header.Get("Content-Length")))
	if err != nil || n < 0 {
		return nil, fmt.Errorf("lsp: bad Content-Length %q", header.Get("Content-Length"))
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, err
	}
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("lsp: %w", err)
	}
	return &m, nil
}

// write sends m; it is safe to call from several goroutines.
func (c *conn) write(m *message) error {
	m.JSONRPC = "2.0"
	body, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.w.Write(body)
	return err
}
