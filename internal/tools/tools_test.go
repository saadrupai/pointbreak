package tools

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/saadrupai/pointbreak"
	"github.com/saadrupai/pointbreak/internal/mcp"
	"github.com/saadrupai/pointbreak/internal/workspace"
)

func server(t *testing.T) (*mcp.Server, string) {
	t.Helper()
	root := t.TempDir()
	for _, r := range []string{"orders-svc", "payments-svc"} {
		if err := os.MkdirAll(filepath.Join(root, r, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	tpl, err := fs.Sub(pointbreak.Templates, "templates/context")
	if err != nil {
		t.Fatal(err)
	}
	ws := workspace.New(root, tpl)
	ws.Now = func() time.Time { return time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC) }
	ws.Branch = func(string) string { return "main" }
	return mcp.NewServer("pointbreak", "test", "", All(ws)), root
}

func call(t *testing.T, s *mcp.Server, raw string) map[string]any {
	t.Helper()
	out, err := json.Marshal(s.Handle([]byte(raw)))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func toolText(t *testing.T, resp map[string]any) (string, bool) {
	t.Helper()
	res, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", resp)
	}
	content := res["content"].([]any)[0].(map[string]any)
	return content["text"].(string), res["isError"].(bool)
}

func TestEveryToolDeclaresAllFourAnnotations(t *testing.T) {
	s, _ := server(t)
	resp := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	list := resp["result"].(map[string]any)["tools"].([]any)
	if len(list) != 10 {
		t.Fatalf("got %d tools, want 10", len(list))
	}
	for _, x := range list {
		tool := x.(map[string]any)
		ann, ok := tool["annotations"].(map[string]any)
		if !ok {
			t.Fatalf("%v has no annotations", tool["name"])
		}
		for _, h := range []string{"readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
			if _, ok := ann[h].(bool); !ok {
				t.Errorf("%v: %s missing or not boolean", tool["name"], h)
			}
		}
		if ann["destructiveHint"] == true || ann["openWorldHint"] == true {
			t.Errorf("%v: no Pointbreak tool is destructive or open-world", tool["name"])
		}
	}
}

func TestReadToolsAreMarkedReadOnly(t *testing.T) {
	s, _ := server(t)
	readOnly := map[string]bool{"list_services": true, "load_context": true, "task_status": true, "search_logs": true, "impact": true}
	for _, tool := range s.Tools() {
		if tool.Annotations.ReadOnlyHint != readOnly[tool.Name] {
			t.Errorf("%s readOnlyHint = %v", tool.Name, tool.Annotations.ReadOnlyHint)
		}
	}
}

func TestEndToEndOverJSONRPC(t *testing.T) {
	s, root := server(t)

	init := call(t, s, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if init["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
		t.Errorf("initialize: %v", init)
	}
	if call(t, s, `{"jsonrpc":"2.0","method":"notifications/initialized"}`) != nil {
		t.Error("notification must not get a response")
	}

	text, isErr := toolText(t, call(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"init_workspace","arguments":{}}}`))
	if isErr || !strings.Contains(text, "Created 12 files") {
		t.Fatalf("init_workspace: %s", text)
	}

	text, isErr = toolText(t, call(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"start_task","arguments":{"service":"orders-svc","title":"Refunds","goal":"g"}}}`))
	if isErr || !strings.Contains(text, "### branch: main — Refunds") {
		t.Fatalf("start_task: %s", text)
	}
	text, isErr = toolText(t, call(t, s, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"save_progress","arguments":{"service":"orders-svc","next":"tests"}}}`))
	if isErr {
		t.Fatalf("save_progress: %s", text)
	}
	text, isErr = toolText(t, call(t, s, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"end_task","arguments":{"service":"orders-svc","summary":["done"]}}}`))
	if isErr || !strings.Contains(text, "reset to (none active)") {
		t.Fatalf("end_task: %s", text)
	}
	log, err := os.ReadFile(filepath.Join(root, "context", "orders-svc", "logs", "2026-11.md"))
	if err != nil || !strings.Contains(string(log), "### branch: main — Refunds\n- done") {
		t.Errorf("log after end_task:\n%s", log)
	}
}

func TestToolErrorsAreToolResultsNotCrashes(t *testing.T) {
	s, _ := server(t)
	text, isErr := toolText(t, call(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"load_context","arguments":{"services":["../../etc"]}}}`))
	if !isErr || !strings.Contains(text, "invalid service name") {
		t.Errorf("want tool error, got %v %s", isErr, text)
	}
	text, isErr = toolText(t, call(t, s, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"impact","arguments":{"term":"x","bogus":1}}}`))
	if !isErr || !strings.Contains(text, "invalid arguments") {
		t.Errorf("unknown argument should be rejected: %s", text)
	}
	resp := call(t, s, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"nope"}}`)
	if resp["error"] == nil {
		t.Error("unknown tool should be a JSON-RPC error")
	}
	resp = call(t, s, `{"jsonrpc":"2.0","id":4,"method":"does/not/exist"}`)
	if resp["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Error("unknown method should return -32601")
	}
}
