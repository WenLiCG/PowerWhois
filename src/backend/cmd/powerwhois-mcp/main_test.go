package main

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/http/httptest"
	"testing"
	"typonamer/client"
)

func TestMCPToolContract(t *testing.T) {
	fixture := `{"id":"shared-task","state":"completed","items":[{"domain":"example.com","status":"unknown","source":"fixture"}]}`
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer query-credential" {
			t.Error("missing query credential")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fixture))
	}))
	defer api.Close()
	server := newServer(&client.Client{URL: api.URL, Token: "query-credential", HTTP: api.Client()})
	a, b := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, e := server.Connect(ctx, a, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer ss.Close()
	cs, e := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, b, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer cs.Close()
	list, e := cs.ListTools(ctx, nil)
	if e != nil || len(list.Tools) != 11 {
		t.Fatalf("tools: %v %v", list, e)
	}
	for _, tool := range list.Tools {
		if tool.Name == "register" || tool.Name == "delete_task" || tool.Name == "update_settings" {
			t.Fatal("privileged tool exposed")
		}
	}
	result, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_task", Arguments: map[string]interface{}{"id": "shared-task"}})
	if e != nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("structured result: %+v %v", result, e)
	}
	bad, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "get_task", Arguments: map[string]interface{}{"id": 123}})
	if e == nil && (bad == nil || !bad.IsError) {
		t.Fatal("invalid parameter type accepted")
	}
}
