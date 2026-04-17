// Package lsp provides a thin LSP client that manages a gopls subprocess
// and exposes References, Definition, and Symbols methods for agent use.
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client manages a gopls subprocess and communicates via JSON-RPC 2.0 over stdio.
type Client struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	mu         sync.Mutex
	nextID     int64
	projectDir string // absolute path
}

// Location represents a source code location returned by LSP.
type Location struct {
	File string `json:"file"` // relative to project root
	Line int    `json:"line"` // 1-indexed
	Text string `json:"text"` // source line content
}

// SymbolInfo represents a workspace symbol.
type SymbolInfo struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "function", "struct", "method", etc.
	File string `json:"file"` // relative to project root
	Line int    `json:"line"` // 1-indexed
}

// NewClient starts a gopls subprocess, performs the LSP initialize handshake,
// and returns a ready-to-use Client. Returns an error if gopls is unavailable.
func NewClient(ctx context.Context, projectDir string) (*Client, error) {
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("resolve project dir: %w", err)
	}

	goplsPath, err := findGopls()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, goplsPath, "serve")
	cmd.Dir = absDir
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start gopls: %w", err)
	}

	c := &Client{
		cmd:        cmd,
		stdin:      stdin,
		stdout:     bufio.NewReaderSize(stdoutPipe, 1024*1024),
		projectDir: absDir,
	}

	if err := c.initialize(); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("lsp initialize: %w", err)
	}

	return c, nil
}

// Close performs an orderly LSP shutdown.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Best-effort shutdown
	c.sendLocked("shutdown", nil)
	c.notifyLocked("exit", nil)

	done := make(chan error, 1)
	go func() { done <- c.cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		c.cmd.Process.Kill()
		<-done
	}
	return nil
}

// ── Public LSP methods ───────────────────────────────────────

// References finds all references to the given symbol in the given file.
func (c *Client) References(ctx context.Context, file, symbol string) ([]Location, error) {
	line, col, err := c.resolveSymbolPosition(file, symbol)
	if err != nil {
		return nil, fmt.Errorf("resolve %s in %s: %w", symbol, file, err)
	}

	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": c.fileURI(file)},
		"position":     map[string]int{"line": line, "character": col},
		"context":      map[string]bool{"includeDeclaration": true},
	}

	result, err := c.call("textDocument/references", params)
	if err != nil {
		return nil, err
	}

	return c.parseLocations(result)
}

// Definition finds the definition of the given symbol in the given file.
func (c *Client) Definition(ctx context.Context, file, symbol string) ([]Location, error) {
	line, col, err := c.resolveSymbolPosition(file, symbol)
	if err != nil {
		return nil, fmt.Errorf("resolve %s in %s: %w", symbol, file, err)
	}

	params := map[string]interface{}{
		"textDocument": map[string]string{"uri": c.fileURI(file)},
		"position":     map[string]int{"line": line, "character": col},
	}

	result, err := c.call("textDocument/definition", params)
	if err != nil {
		return nil, err
	}

	return c.parseLocations(result)
}

// Symbols searches for symbols matching the query across the workspace.
func (c *Client) Symbols(ctx context.Context, query string) ([]SymbolInfo, error) {
	params := map[string]string{"query": query}

	result, err := c.call("workspace/symbol", params)
	if err != nil {
		return nil, err
	}

	var rawSymbols []struct {
		Name     string `json:"name"`
		Kind     int    `json:"kind"`
		Location struct {
			URI   string `json:"uri"`
			Range struct {
				Start struct {
					Line int `json:"line"`
				} `json:"start"`
			} `json:"range"`
		} `json:"location"`
	}

	if err := json.Unmarshal(result, &rawSymbols); err != nil {
		return nil, fmt.Errorf("parse symbols: %w", err)
	}

	var symbols []SymbolInfo
	for _, s := range rawSymbols {
		symbols = append(symbols, SymbolInfo{
			Name: s.Name,
			Kind: symbolKindName(s.Kind),
			File: c.uriToRelPath(s.Location.URI),
			Line: s.Location.Range.Start.Line + 1,
		})
	}
	return symbols, nil
}

// ── LSP initialization ──────────────────────────────────────

func (c *Client) initialize() error {
	params := map[string]interface{}{
		"processId": os.Getpid(),
		"rootUri":   "file://" + c.projectDir,
		"capabilities": map[string]interface{}{
			"textDocument": map[string]interface{}{
				"references":    map[string]bool{"dynamicRegistration": false},
				"definition":    map[string]bool{"dynamicRegistration": false},
				"synchronization": map[string]interface{}{
					"didOpen": true,
				},
			},
			"workspace": map[string]interface{}{
				"symbol": map[string]bool{"dynamicRegistration": false},
			},
		},
	}

	_, err := c.call("initialize", params)
	if err != nil {
		return err
	}

	return c.notify("initialized", struct{}{})
}

// ── JSON-RPC 2.0 transport ──────────────────────────────────

type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *Client) call(method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sendLocked(method, params)
}

func (c *Client) notify(method string, params interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.notifyLocked(method, params)
}

func (c *Client) sendLocked(method string, params interface{}) (json.RawMessage, error) {
	c.nextID++
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      c.nextID,
		Method:  method,
		Params:  params,
	}

	if err := c.writeMessage(req); err != nil {
		return nil, fmt.Errorf("write %s: %w", method, err)
	}

	// Read responses, skipping notifications (no ID / ID=0) from the server
	for {
		resp, err := c.readMessage()
		if err != nil {
			return nil, fmt.Errorf("read %s response: %w", method, err)
		}

		if resp.ID == req.ID {
			if resp.Error != nil {
				return nil, fmt.Errorf("lsp %s error %d: %s", method, resp.Error.Code, resp.Error.Message)
			}
			return resp.Result, nil
		}
		// Skip server-initiated notifications/requests (window/logMessage, etc.)
	}
}

func (c *Client) notifyLocked(method string, params interface{}) error {
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(req)
}

func (c *Client) writeMessage(msg interface{}) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(c.stdin, header); err != nil {
		return err
	}
	_, err = c.stdin.Write(body)
	return err
}

func (c *Client) readMessage() (*jsonrpcResponse, error) {
	// Read Content-Length header
	contentLen := 0
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break // end of headers
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLen, err = strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("parse Content-Length %q: %w", val, err)
			}
		}
	}

	if contentLen == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLen)
	if _, err := io.ReadFull(c.stdout, body); err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var resp jsonrpcResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}

// ── Helpers ──────────────────────────────────────────────────

// resolveSymbolPosition finds the 0-indexed line and column of a symbol in a file.
func (c *Client) resolveSymbolPosition(relFile, symbol string) (line, col int, err error) {
	absPath := filepath.Join(c.projectDir, relFile)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return 0, 0, fmt.Errorf("read file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	for i, l := range lines {
		idx := strings.Index(l, symbol)
		if idx >= 0 {
			return i, idx, nil // 0-indexed
		}
	}
	return 0, 0, fmt.Errorf("symbol %q not found in %s", symbol, relFile)
}

func (c *Client) fileURI(relPath string) string {
	absPath := filepath.Join(c.projectDir, relPath)
	return "file://" + filepath.ToSlash(absPath)
}

func (c *Client) uriToRelPath(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	path = strings.TrimPrefix(path, "file:")
	rel, err := filepath.Rel(c.projectDir, path)
	if err != nil {
		return path
	}
	return rel
}

func (c *Client) parseLocations(raw json.RawMessage) ([]Location, error) {
	var lspLocs []struct {
		URI   string `json:"uri"`
		Range struct {
			Start struct {
				Line      int `json:"line"`
				Character int `json:"character"`
			} `json:"start"`
		} `json:"range"`
	}

	if err := json.Unmarshal(raw, &lspLocs); err != nil {
		// Try single location (definition can return one instead of array)
		var single struct {
			URI   string `json:"uri"`
			Range struct {
				Start struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"start"`
			} `json:"range"`
		}
		if err2 := json.Unmarshal(raw, &single); err2 == nil && single.URI != "" {
			lspLocs = append(lspLocs, single)
		} else {
			return nil, fmt.Errorf("parse locations: %w", err)
		}
	}

	var locs []Location
	for _, l := range lspLocs {
		relPath := c.uriToRelPath(l.URI)
		lineNum := l.Range.Start.Line + 1 // convert to 1-indexed
		text := c.readLineFromFile(relPath, l.Range.Start.Line)
		locs = append(locs, Location{
			File: relPath,
			Line: lineNum,
			Text: strings.TrimSpace(text),
		})
	}
	return locs, nil
}

func (c *Client) readLineFromFile(relPath string, lineIdx int) string {
	absPath := filepath.Join(c.projectDir, relPath)
	f, err := os.Open(absPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for i := 0; scanner.Scan(); i++ {
		if i == lineIdx {
			return scanner.Text()
		}
	}
	return ""
}

// findGopls locates the gopls binary, installing it if necessary.
func findGopls() (string, error) {
	// Check PATH
	if p, err := exec.LookPath("gopls"); err == nil {
		return p, nil
	}
	// Check common locations
	for _, dir := range []string{
		filepath.Join(os.Getenv("HOME"), "gopath", "bin"),
		filepath.Join(os.Getenv("HOME"), "go", "bin"),
		filepath.Join(os.Getenv("HOME"), ".cache", "opencode", "bin"),
	} {
		p := filepath.Join(dir, "gopls")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Try auto-install
	log.Printf("gopls not found, installing...")
	cmd := exec.Command("go", "install", "golang.org/x/tools/gopls@latest")
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gopls not found and auto-install failed: %w", err)
	}
	// Check again
	if p, err := exec.LookPath("gopls"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("gopls installed but not on PATH")
}

func symbolKindName(kind int) string {
	names := map[int]string{
		1: "file", 2: "module", 3: "namespace", 4: "package",
		5: "class", 6: "method", 7: "property", 8: "field",
		9: "constructor", 10: "enum", 11: "interface", 12: "function",
		13: "variable", 14: "constant", 15: "string", 16: "number",
		17: "boolean", 18: "array", 19: "object", 20: "key",
		21: "null", 22: "enummember", 23: "struct", 24: "event",
		25: "operator", 26: "typeparameter",
	}
	if n, ok := names[kind]; ok {
		return n
	}
	return fmt.Sprintf("kind_%d", kind)
}
