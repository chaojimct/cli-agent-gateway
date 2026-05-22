package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// RawDebug from CG_ACP_RAW_DEBUG=1
func RawDebugEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CG_ACP_RAW_DEBUG")))
	return v == "1" || v == "true"
}

// Handler handles inbound JSON-RPC from the agent (notifications + server requests).
type Handler func(ctx context.Context, method string, id *int, params json.RawMessage) (result interface{}, respond bool)

// MessageRecorder captures raw ACP JSON-RPC lines (direction: "in" | "out").
type MessageRecorder func(direction, method, payload string)

// Client speaks ACP over stdio with a long-lived cursor-agent acp child.
type Client struct {
	mu               sync.Mutex
	cmd              *exec.Cmd
	stdin            io.WriteCloser
	nextID           atomic.Int32
	pending          map[int]chan responseWait
	handlers         []Handler
	recorder         MessageRecorder
	logger           *slog.Logger
	rawDebug         bool
	skipAuthenticate bool
	authMethod       string
	initResult       *InitializeResult
	closed           bool
	wg               sync.WaitGroup
	stderrRing       []string
	stderrMu         sync.Mutex
}

type responseWait struct {
	result json.RawMessage
	err    error
}

// Config for spawning the ACP child.
type Config struct {
	Command          string
	Args             []string
	Env              []string
	Dir              string
	Logger           *slog.Logger
	SkipAuthenticate bool
	AuthMethod       string
	NoAppendACP      bool // true for npx bridge etc.; do not auto-append "acp" subcommand
	RawDebug         bool
}

// NewClient spawns agent acp and starts the read loop.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	if cfg.Command == "" {
		cfg.Command = "cursor-agent"
	}
	args := cfg.Args
	if len(args) == 0 && !cfg.NoAppendACP {
		args = []string{"acp"}
	} else if len(args) > 0 && !cfg.NoAppendACP && !containsACP(args) {
		args = append(append([]string{}, args...), "acp")
	}

	exe, fullArgs := ResolveSpawn(cfg.Command, args, cfg.NoAppendACP)
	cmd := exec.CommandContext(ctx, exe, fullArgs...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}
	env := os.Environ()
	env = append(env,
		"LANG=en_US.UTF-8",
		"LC_ALL=en_US.UTF-8",
		"PYTHONIOENCODING=utf-8",
		"CHCP=65001",
	)
	if len(cfg.Env) > 0 {
		env = append(env, cfg.Env...)
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start acp: %w", err)
	}

	c := &Client{
		cmd:              cmd,
		stdin:            stdin,
		pending:          make(map[int]chan responseWait),
		logger:           cfg.Logger,
		rawDebug:         cfg.RawDebug || RawDebugEnabled(),
		skipAuthenticate: cfg.SkipAuthenticate,
		authMethod:       cfg.AuthMethod,
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.readLoop(stdout)
	}()
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.drainStderr(stderr)
	}()
	return c, nil
}

func containsACP(args []string) bool {
	for _, a := range args {
		if a == "acp" {
			return true
		}
	}
	return false
}

func resolveCommand(binPath string) (exe string, argsPrefix []string) {
	if binPath == "" {
		binPath = "cursor-agent"
	}
	if filepath.Separator != '\\' {
		return binPath, nil
	}
	lower := strings.ToLower(binPath)
	if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") {
		comspec := os.Getenv("COMSPEC")
		if comspec == "" {
			comspec = `C:\Windows\System32\cmd.exe`
		}
		return comspec, []string{"/c", binPath}
	}
	return binPath, nil
}

func (c *Client) drainStderr(r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		c.appendStderr(line)
		if c.logger != nil && strings.TrimSpace(line) != "" {
			c.logger.Debug("acp stderr", "line", line)
		}
	}
}

func (c *Client) appendStderr(line string) {
	const maxLines = 32
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	c.stderrRing = append(c.stderrRing, line)
	if len(c.stderrRing) > maxLines {
		c.stderrRing = c.stderrRing[len(c.stderrRing)-maxLines:]
	}
}

// RecentStderr returns the last stderr lines captured from the child process.
func (c *Client) RecentStderr() []string {
	c.stderrMu.Lock()
	defer c.stderrMu.Unlock()
	out := make([]string, len(c.stderrRing))
	copy(out, c.stderrRing)
	return out
}

func (c *Client) readLoop(stdout io.Reader) {
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if c.rawDebug && c.logger != nil {
			c.logger.Debug("acp raw", "line", string(line))
		}
		c.record("in", line)
		c.dispatchLine(line)
	}
	if err := sc.Err(); err != nil && c.logger != nil {
		c.logger.Warn("acp read loop ended", "error", err)
	}
	c.failPending(fmt.Errorf("acp process exited"))
}

func (c *Client) dispatchLine(line []byte) {
	var probe struct {
		JSONRPC string `json:"jsonrpc"`
		ID      *int   `json:"id"`
		Method  string `json:"method"`
		Result  json.RawMessage `json:"result"`
		Error   *RPCError `json:"error"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return
	}

	// Response to our request
	if probe.ID != nil && (probe.Result != nil || probe.Error != nil) {
		c.mu.Lock()
		ch, ok := c.pending[*probe.ID]
		if ok {
			delete(c.pending, *probe.ID)
		}
		c.mu.Unlock()
		if ok {
			if probe.Error != nil {
				ch <- responseWait{err: fmt.Errorf("acp rpc: %s", probe.Error.Message)}
			} else {
				ch <- responseWait{result: probe.Result}
			}
		}
		return
	}

	// Server → client request (needs response)
	if probe.Method != "" && probe.ID != nil {
		ctx := context.Background()
		var params json.RawMessage
		_ = json.Unmarshal(line, &struct {
			Params *json.RawMessage `json:"params"`
		}{Params: &params})
		for _, h := range c.handlers {
			result, respond := h(ctx, probe.Method, probe.ID, params)
			if respond {
				_ = c.writeResponse(*probe.ID, result, nil)
				return
			}
		}
		return
	}

	// Notification from agent
	if probe.Method != "" {
		var n Notification
		_ = json.Unmarshal(line, &n)
		ctx := context.Background()
		for _, h := range c.handlers {
			_, _ = h(ctx, probe.Method, nil, n.Params)
		}
	}
}

func (c *Client) writeResponse(id int, result interface{}, rpcErr *RPCError) error {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
	}
	if rpcErr != nil {
		msg["error"] = rpcErr
	} else {
		msg["result"] = result
	}
	return c.writeJSON(msg)
}

// SetRecorder installs a callback for inbound/outbound JSON-RPC lines.
func (c *Client) SetRecorder(r MessageRecorder) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.recorder = r
}

func (c *Client) record(direction string, line []byte) {
	c.mu.Lock()
	rec := c.recorder
	c.mu.Unlock()
	if rec == nil || len(line) == 0 {
		return
	}
	method := rpcMethod(line)
	rec(direction, method, string(line))
}

func rpcMethod(line []byte) string {
	var probe struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return ""
	}
	return probe.Method
}

// SetHandler replaces inbound method handlers (single handler slot).
func (c *Client) SetHandler(h Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if h == nil {
		c.handlers = nil
		return
	}
	c.handlers = []Handler{h}
}

// OnHandler registers an inbound method handler (appends; prefer SetHandler).
func (c *Client) OnHandler(h Handler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers = append(c.handlers, h)
}

// Request sends a JSON-RPC request and waits for the matching response.
func (c *Client) Request(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := int(c.nextID.Add(1))
	ch := make(chan responseWait, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("acp client closed")
	}
	c.pending[id] = ch
	c.mu.Unlock()

	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		body["params"] = params
	}
	if err := c.writeJSON(body); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	case w := <-ch:
		return w.result, w.err
	}
}

func (c *Client) writeJSON(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return fmt.Errorf("stdin closed")
	}
	payload := append(b, '\n')
	if _, err = c.stdin.Write(payload); err != nil {
		return err
	}
	if c.recorder != nil {
		c.recorder("out", rpcMethod(b), string(b))
	}
	return nil
}

// Notify sends a JSON-RPC notification (no id).
func (c *Client) Notify(method string, params interface{}) error {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		body["params"] = params
	}
	return c.writeJSON(body)
}

// Close shuts down the child process.
func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.failPending(fmt.Errorf("acp client closed"))
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	var waitErr error
	if c.cmd != nil {
		waitErr = c.cmd.Wait()
	}
	c.wg.Wait()
	return waitErr
}

func (c *Client) failPending(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, ch := range c.pending {
		select {
		case ch <- responseWait{err: err}:
		default:
		}
		delete(c.pending, id)
	}
}

// Wait returns process exit error.
func (c *Client) Wait() error {
	if c.cmd == nil {
		return nil
	}
	return c.cmd.Wait()
}

// IsBenignExit reports Windows/normal cancel exit codes.
func IsBenignExit(err error) bool {
	if err == nil {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "exit status 1") ||
		strings.Contains(s, "exit status -1") ||
		strings.Contains(s, "signal: killed") ||
		strings.Contains(s, "terminated")
}
