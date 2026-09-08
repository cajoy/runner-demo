package scenarios

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"
)

func (s *suite) testMCPProvenance(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.binary, "mcp", "--project", s.root)
	cmd.Dir = s.root
	cmd.Env = s.env
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		input.Close()
		if err := cmd.Wait(); err != nil {
			t.Errorf("MCP process: %v %s", err, &stderr)
		}
	}()
	encode := json.NewEncoder(input)
	if err := encode.Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "receipt-e2e", "version": "1"}}}); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	if !scanner.Scan() {
		t.Fatalf("MCP initialization failed: %v", scanner.Err())
	}
	if err := encode.Encode(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); err != nil {
		t.Fatal(err)
	}
	if err := encode.Encode(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": "runner_run", "arguments": map[string]any{"project_id": s.first.ProjectID, "selector": "api:preflight"}}}); err != nil {
		t.Fatal(err)
	}
	for scanner.Scan() {
		var message struct {
			ID     int             `json:"id"`
			Error  json.RawMessage `json:"error"`
			Result struct {
				IsError    bool `json:"isError"`
				Structured struct {
					State    string `json:"state"`
					Terminal struct {
						Actor struct{ Kind, Source string } `json:"actor"`
						Proof json.RawMessage               `json:"proof_status"`
					} `json:"terminal_receipt"`
				} `json:"structuredContent"`
			} `json:"result"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			t.Fatal(err)
		}
		if message.ID != 2 {
			continue
		}
		got := message.Result.Structured
		if message.Result.IsError || len(message.Error) != 0 || got.State != "passed" || got.Terminal.Actor.Kind != "agent" || got.Terminal.Actor.Source != "mcp" || len(got.Terminal.Proof) == 0 {
			t.Fatalf("MCP result: %s", scanner.Bytes())
		}
		return
	}
	t.Fatalf("MCP ended without terminal result: %v", scanner.Err())
}
