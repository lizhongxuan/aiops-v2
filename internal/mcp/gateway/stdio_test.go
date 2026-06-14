package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestGatewayStdioStartsListsAndClosesProcess(t *testing.T) {
	gateway := NewGateway(GatewayOptions{})
	connected, err := gateway.Connect(context.Background(), ServerConfigV2{
		ID: "stdio-fake",
		Stdio: &StdioConfig{
			Command: os.Args[0],
			Args:    []string{"-test.run=TestGatewayStdioHelperProcess", "--"},
			Env:     []string{"AIOPS_FAKE_MCP_STDIO=1"},
		},
	})
	if err != nil {
		t.Fatalf("Connect error = %v", err)
	}
	if len(connected.Tools) != 1 || connected.Tools[0].Name != "stdio_metrics" {
		t.Fatalf("connected tools = %#v, want stdio_metrics", connected.Tools)
	}

	result, err := gateway.CallTool(context.Background(), MCPToolCallRequest{
		ServerID:  "stdio-fake",
		ToolName:  "stdio_metrics",
		Arguments: json.RawMessage(`{"service":"api"}`),
	})
	if err != nil {
		t.Fatalf("CallTool error = %v", err)
	}
	if result.Content != "stdio metrics ok" {
		t.Fatalf("tool result content = %q, want stdio metrics ok", result.Content)
	}

	if err := gateway.Close(context.Background()); err != nil {
		t.Fatalf("Close error = %v", err)
	}
}

func TestGatewayStdioHelperProcess(t *testing.T) {
	if os.Getenv("AIOPS_FAKE_MCP_STDIO") != "1" {
		return
	}
	runFakeMCPStdioServer()
	os.Exit(0)
}

func runFakeMCPStdioServer() {
	reader := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if line == "" {
			continue
		}
		var req struct {
			ID     any             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeStdioRPCError(writer, nil, -32700, err.Error())
			continue
		}
		switch req.Method {
		case "initialize":
			writeStdioRPCResult(writer, req.ID, map[string]any{"protocolVersion": "2025-03-26"})
		case "tools/list":
			writeStdioRPCResult(writer, req.ID, map[string]any{"tools": []map[string]any{{
				"name":        "stdio_metrics",
				"description": "Read stdio metrics",
				"inputSchema": map[string]any{"type": "object"},
			}}})
		case "resources/list":
			writeStdioRPCResult(writer, req.ID, map[string]any{"resources": []any{}})
		case "tools/call":
			writeStdioRPCResult(writer, req.ID, map[string]any{"content": "stdio metrics ok"})
		default:
			writeStdioRPCError(writer, req.ID, -32601, fmt.Sprintf("unknown method %s", req.Method))
		}
	}
}

func writeStdioRPCResult(writer *bufio.Writer, id any, result any) {
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	_ = writer.Flush()
}

func writeStdioRPCError(writer *bufio.Writer, id any, code int, message string) {
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
	_ = writer.Flush()
}
