package acp_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/chaojimct/cli-agent-gateway/internal/acp"
)

// fakeACPStdinServer mimics cursor-api-proxy fake-acp-server for integration tests.
func fakeACPStdinServer(stdin io.Reader, stdout io.Writer) {
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		var msg struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) != nil {
			continue
		}
		if msg.ID == nil {
			continue
		}
		var result interface{}
		switch msg.Method {
		case "initialize":
			result = map[string]interface{}{"protocolVersion": 1}
		case "authenticate":
			result = map[string]interface{}{}
		case "session/new":
			result = map[string]interface{}{"sessionId": "sess-test-1"}
		case "session/set_config_option":
			result = map[string]interface{}{}
		case "session/prompt":
			// stream one chunk then return stopReason
			notify, _ := json.Marshal(map[string]interface{}{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]interface{}{
					"sessionId": "sess-test-1",
					"update": map[string]interface{}{
						"sessionUpdate": "agent_message_chunk",
						"content":       map[string]string{"type": "text", "text": "Hello from fake ACP"},
					},
				},
			})
			_, _ = stdout.Write(append(notify, '\n'))
			result = map[string]interface{}{"stopReason": "end_turn"}
		default:
			result = map[string]interface{}{}
		}
		resp, _ := json.Marshal(map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      *msg.ID,
			"result":  result,
		})
		_, _ = stdout.Write(append(resp, '\n'))
	}
}

func TestFakeACPClientIntegration(t *testing.T) {
	if os.Getenv("CG_RUN_ACP_INTEGRATION") == "" {
		t.Skip("set CG_RUN_ACP_INTEGRATION=1 to run fake ACP integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestFakeACPHelperProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	_ = cmd.Start()

	client, err := acp.NewClient(ctx, acp.Config{
		Command:          cmd.Path,
		Args:             []string{"-test.run=TestFakeACPHelperProcess"},
		SkipAuthenticate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	if err := client.Bootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := client.Request(ctx, "session/new", acp.SessionNewParams{CWD: os.TempDir(), McpServers: []interface{}{}})
	if err != nil {
		t.Fatal(err)
	}
	var sn acp.SessionNewResult
	_ = json.Unmarshal(raw, &sn)
	if sn.SessionID == "" {
		t.Fatal("empty session id")
	}
	_, err = client.Request(ctx, "session/prompt", acp.PromptParams{
		SessionID: sn.SessionID,
		Prompt:    []acp.ContentBlock{acp.TextBlock("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = stdin
	_ = stdout
}

func TestFakeACPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fakeACPStdinServer(os.Stdin, os.Stdout)
	os.Exit(0)
}
