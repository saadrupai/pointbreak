package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func testServer() *Server {
	return NewServer("t", "1", "hi", []Tool{
		{Name: "echo", Handler: func(a json.RawMessage) (string, error) { return string(a), nil }},
		{Name: "boom", Handler: func(json.RawMessage) (string, error) { return "", errors.New("it broke") }},
	})
}

func run(t *testing.T, input string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := testServer().Serve(strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	var res []map[string]any
	for _, ln := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if ln == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatalf("bad output line %q: %v", ln, err)
		}
		res = append(res, m)
	}
	return res
}

func TestServeEdgeCases(t *testing.T) {
	in := strings.Join([]string{
		``,
		`not json at all`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/whatever"}`,
		`{"jsonrpc":"2.0","id":null,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":"s","method":"ping"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":null}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"boom","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":"bad"}`,
		`{"jsonrpc":"2.0","id":6,"method":"nope"}`,
	}, "\n")
	res := run(t, in)
	if len(res) != 8 {
		t.Fatalf("want 8 responses (parse error + 7 requests; notifications and null id get none), got %d: %v", len(res), res)
	}
	if res[0]["error"].(map[string]any)["code"].(float64) != -32700 {
		t.Error("garbage line should give a parse error")
	}
	if v := res[1]["result"].(map[string]any)["protocolVersion"]; v != "2025-06-18" {
		t.Errorf("default protocol version: %v", v)
	}
	if res[2]["id"] != "s" {
		t.Error("string ids must be echoed")
	}
	for _, i := range []int{3, 4} {
		text := res[i]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"]
		if text != "{}" {
			t.Errorf("missing/null arguments should become {}: %v", text)
		}
	}
	boom := res[5]["result"].(map[string]any)
	if boom["isError"] != true || !strings.Contains(boom["content"].([]any)[0].(map[string]any)["text"].(string), "it broke") {
		t.Errorf("handler error should be a tool error result: %v", boom)
	}
	if res[6]["error"].(map[string]any)["code"].(float64) != -32602 {
		t.Error("bad params should be -32602")
	}
	if res[7]["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Error("unknown method should be -32601")
	}
}

func TestServeLargeLine(t *testing.T) {
	big := strings.Repeat("x", 2<<20)
	res := run(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"s":"`+big+`"}}}`)
	if len(res) != 1 || !strings.Contains(res[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string), big[:100]) {
		t.Error("a 2 MB request should be handled")
	}
}
