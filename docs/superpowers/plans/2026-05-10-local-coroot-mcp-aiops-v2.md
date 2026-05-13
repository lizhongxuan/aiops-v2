# Local Coroot MCP aiops-v2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run Coroot locally with Docker-backed dependencies, verify the Coroot stdio MCP server, and expose Coroot tools inside aiops-v2.

**Architecture:** Prometheus and ClickHouse run as Docker dependencies, while `coroot-local` runs directly on the host at `127.0.0.1:18180`. `coroot-mcp-server` is built and smoke-tested over stdio, and aiops-v2 registers its existing `coroot.*` dynamic MCP tools from `AIOPS_COROOT_BASE_URL`.

**Tech Stack:** Go 1.26, Bash, Docker, Prometheus, ClickHouse, Coroot, MCP JSON-RPC, aiops-v2 Go backend.

---

## File Structure

- Modify `/Users/lizhongxuan/Desktop/aiops/aiops-v2/cmd/ai-server/main.go`: add a small helper that resolves Coroot endpoint env vars consistently.
- Modify `/Users/lizhongxuan/Desktop/aiops/aiops-v2/cmd/ai-server/main_test.go`: add table tests for Coroot endpoint env precedence.
- Optionally modify `/Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp/run.sh`: make it resolve `coroot-mcp-server` relative to the script directory if the current file still points to the wrong absolute path.
- Runtime artifacts created by commands:
  - `/Users/lizhongxuan/Desktop/aiops/coroot/coroot-local`
  - `/Users/lizhongxuan/Desktop/aiops/coroot/coroot.pid`
  - `/Users/lizhongxuan/Desktop/aiops/coroot/coroot.log`
  - `/Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp/coroot-mcp-server`
  - `/Users/lizhongxuan/Desktop/aiops/aiops-v2/.data/bin/ai-server`

## Task 1: Make aiops-v2 Honor AIOPS_COROOT_BASE_URL

**Files:**
- Modify: `/Users/lizhongxuan/Desktop/aiops/aiops-v2/cmd/ai-server/main.go`
- Modify: `/Users/lizhongxuan/Desktop/aiops/aiops-v2/cmd/ai-server/main_test.go`

- [ ] **Step 1: Write the failing test**

Add this test to `/Users/lizhongxuan/Desktop/aiops/aiops-v2/cmd/ai-server/main_test.go` near the existing env helper tests:

```go
func TestCorootEndpointFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "prefers explicit endpoint",
			env: map[string]string{
				"AIOPS_COROOT_ENDPOINT": " http://coroot-endpoint.internal ",
				"AIOPS_COROOT_BASE_URL": "http://coroot-base.internal",
				"COROOT_BASE_URL":       "http://coroot-fallback.internal",
			},
			want: "http://coroot-endpoint.internal",
		},
		{
			name: "falls back to aiops base url",
			env: map[string]string{
				"AIOPS_COROOT_BASE_URL": " http://127.0.0.1:18180 ",
				"COROOT_BASE_URL":       "http://coroot-fallback.internal",
			},
			want: "http://127.0.0.1:18180",
		},
		{
			name: "falls back to coroot base url",
			env: map[string]string{
				"COROOT_BASE_URL": " http://coroot.local ",
			},
			want: "http://coroot.local",
		},
		{
			name: "returns empty when unset",
			env:  map[string]string{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := corootEndpointFromEnv(func(key string) string { return tt.env[key] })
			if got != tt.want {
				t.Fatalf("corootEndpointFromEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test ./cmd/ai-server -run TestCorootEndpointFromEnv -count=1
```

Expected: FAIL to compile with `undefined: corootEndpointFromEnv`.

- [ ] **Step 3: Implement minimal helper and wire it into startup**

In `/Users/lizhongxuan/Desktop/aiops/aiops-v2/cmd/ai-server/main.go`, replace:

```go
corootEndpoint := envOrDefault("AIOPS_COROOT_ENDPOINT", "")
```

with:

```go
corootEndpoint := corootEndpointFromEnv(os.Getenv)
```

Add this helper next to `runnerStudioUpstreamFromEnv`:

```go
func corootEndpointFromEnv(getenv func(string) string) string {
	for _, key := range []string{
		"AIOPS_COROOT_ENDPOINT",
		"AIOPS_COROOT_BASE_URL",
		"COROOT_BASE_URL",
	} {
		if value := strings.TrimSpace(getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
```

- [ ] **Step 4: Run focused tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test ./cmd/ai-server -run 'TestCorootEndpointFromEnv|TestRunnerStudioUpstreamFromEnv' -count=1
```

Expected: PASS.

- [ ] **Step 5: Run Coroot integration and MCP registry tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test ./internal/integrations/coroot ./internal/mcp ./cmd/ai-server -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the aiops-v2 code fix**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
git add cmd/ai-server/main.go cmd/ai-server/main_test.go
git commit -m "fix: honor coroot base url env"
```

Expected: commit contains only the env helper test and implementation.

## Task 2: Build and Smoke-Test Coroot MCP

**Files:**
- Inspect: `/Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp/main.go`
- Inspect or modify: `/Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp/run.sh`

- [ ] **Step 1: Run MCP unit tests**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 2: Build MCP binary**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp
go build -o coroot-mcp-server .
test -x /Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp/coroot-mcp-server
```

Expected: binary exists and is executable.

- [ ] **Step 3: Verify whether run.sh points at the wrong workspace**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp
sed -n '1,20p' run.sh
```

Expected before fix: the script may contain `/Users/lizhongxuan/Desktop/coroot/coroot-mcp/coroot-mcp-server`, which is not the current workspace path.

- [ ] **Step 4: Fix run.sh only if it still has a hard-coded wrong path**

Change `/Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp/run.sh` to:

```bash
#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
exec "$SCRIPT_DIR/coroot-mcp-server"
```

- [ ] **Step 5: Validate run.sh syntax**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp
bash -n run.sh
```

Expected: exit code 0.

- [ ] **Step 6: Run stdio MCP tools/list smoke test**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/coroot/coroot-mcp
printf '%s\n%s\n%s\n' \
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"aiops-smoke","version":"0.0.1"}}}' \
'{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}' \
'{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' \
| COROOT_URL=http://127.0.0.1:18180 ./run.sh
```

Expected: output includes `"name":"list_applications"` and no JSON-RPC error. If Coroot is not running yet, `tools/list` should still work because it lists registered tool definitions without calling Coroot HTTP APIs.

## Task 3: Start Docker Dependencies and Local Coroot

**Files:**
- Read-only reference: `/Users/lizhongxuan/Desktop/aiops/coroot/start-coroot.sh`
- Runtime output: `/Users/lizhongxuan/Desktop/aiops/coroot/coroot.log`
- Runtime output: `/Users/lizhongxuan/Desktop/aiops/coroot/coroot.pid`

- [ ] **Step 1: Check required commands and ports**

Run:

```bash
docker version
go version
lsof -nP -iTCP:18180 -sTCP:LISTEN || true
lsof -nP -iTCP:19190 -sTCP:LISTEN || true
lsof -nP -iTCP:19000 -sTCP:LISTEN || true
```

Expected: Docker and Go are available. If `18180`, `19190`, or `19000` is occupied by an unrelated process, stop and report the PID before changing anything.

- [ ] **Step 2: Start Prometheus**

Run:

```bash
docker start aiops-coroot-prometheus 2>/dev/null || docker run -d \
  --name aiops-coroot-prometheus \
  -p 127.0.0.1:19190:9090 \
  prom/prometheus:v2.53.5 \
  --config.file=/etc/prometheus/prometheus.yml \
  --storage.tsdb.path=/prometheus \
  --web.console.libraries=/usr/share/prometheus/console_libraries \
  --web.console.templates=/usr/share/prometheus/consoles \
  --web.enable-lifecycle \
  --web.enable-remote-write-receiver
```

Expected: container ID or name printed.

- [ ] **Step 3: Start ClickHouse**

Run:

```bash
docker start aiops-coroot-clickhouse 2>/dev/null || docker run -d \
  --name aiops-coroot-clickhouse \
  -p 127.0.0.1:19000:9000 \
  -p 127.0.0.1:18123:8123 \
  -e CLICKHOUSE_SKIP_USER_SETUP=1 \
  --ulimit nofile=262144:262144 \
  clickhouse/clickhouse-server:24.3
```

Expected: container ID or name printed.

- [ ] **Step 4: Wait for dependency health**

Run:

```bash
curl -fsS --noproxy 127.0.0.1 http://127.0.0.1:19190/-/healthy
curl -fsS --noproxy 127.0.0.1 http://127.0.0.1:18123/ping
```

Expected: Prometheus returns `Prometheus Server is Healthy.` and ClickHouse returns `Ok.`.

- [ ] **Step 5: Build Coroot if the local binary is missing**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/coroot
test -x coroot-local || go build -o coroot-local .
test -x /Users/lizhongxuan/Desktop/aiops/coroot/coroot-local
```

Expected: `coroot-local` exists and is executable.

- [ ] **Step 6: Start Coroot as a local process**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/coroot
if [ -f coroot.pid ] && kill -0 "$(cat coroot.pid)" 2>/dev/null; then
  echo "coroot already running with pid $(cat coroot.pid)"
else
  nohup ./coroot-local \
    --listen=:18180 \
    --bootstrap-prometheus-url=http://127.0.0.1:19190 \
    --bootstrap-prometheus-remote-write-url=http://127.0.0.1:19190/api/v1/write \
    --bootstrap-clickhouse-address=127.0.0.1:19000 \
    --auth-anonymous-role=Admin \
    > coroot.log 2>&1 &
  echo "$!" > coroot.pid
fi
```

Expected: either an existing Coroot PID is reported, or a new PID is written to `coroot.pid`.

- [ ] **Step 7: Verify Coroot health**

Run:

```bash
curl -fsS --noproxy 127.0.0.1 http://127.0.0.1:18180/health
```

Expected: HTTP 200 response. If it fails, run:

```bash
tail -50 /Users/lizhongxuan/Desktop/aiops/coroot/coroot.log
```

and diagnose from the logged error.

## Task 4: Start aiops-v2 with Coroot Enabled

**Files:**
- Runtime binary: `/Users/lizhongxuan/Desktop/aiops/aiops-v2/.data/bin/ai-server`
- Runtime data: `/Users/lizhongxuan/Desktop/aiops/aiops-v2/.data-coroot`

- [ ] **Step 1: Build aiops-v2 backend**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go build -o .data/bin/ai-server ./cmd/ai-server
test -x /Users/lizhongxuan/Desktop/aiops/aiops-v2/.data/bin/ai-server
```

Expected: server binary exists and is executable.

- [ ] **Step 2: Start aiops-v2 on port 18080**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
AIOPS_HTTP_ADDR=:18080 \
AIOPS_GRPC_ADDR=:18090 \
AIOPS_DATA_DIR=.data-coroot \
AIOPS_COROOT_BASE_URL=http://127.0.0.1:18180 \
AIOPS_COROOT_PROJECT=default \
AIOPS_SERVER_BIN=.data/bin/ai-server \
SKIP_WEB_BUILD=1 \
SKIP_GO_BUILD=1 \
./scripts/start.sh
```

Expected: foreground server logs include `http: http://127.0.0.1:18080` and the command keeps running. For this session, run it through the terminal tool as a long-running process and keep the session id.

- [ ] **Step 3: Verify aiops-v2 HTTP is reachable**

Run from another shell:

```bash
curl -fsS --noproxy 127.0.0.1 http://127.0.0.1:18080/api/v1/mcp/servers
```

Expected: JSON response with an `items` array.

- [ ] **Step 4: Verify Coroot MCP registry entry**

Run:

```bash
curl -fsS --noproxy 127.0.0.1 http://127.0.0.1:18080/api/v1/mcp/servers
```

Expected: response contains an item with `"name":"coroot"`, `"status":"connected"`, and `"toolCount":7`.

## Task 5: Final Verification

**Files:**
- Inspect: `/Users/lizhongxuan/Desktop/aiops/aiops-v2/.data-coroot/mcp-servers.json` if it exists.
- Inspect: `/Users/lizhongxuan/Desktop/aiops/coroot/coroot.log`

- [ ] **Step 1: Run final aiops-v2 test subset**

Run:

```bash
cd /Users/lizhongxuan/Desktop/aiops/aiops-v2
go test ./internal/integrations/coroot ./internal/mcp ./cmd/ai-server -count=1
```

Expected: PASS.

- [ ] **Step 2: Capture running service status**

Run:

```bash
docker ps --filter name=aiops-coroot --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Status}}'
lsof -nP -iTCP:18180 -sTCP:LISTEN
lsof -nP -iTCP:18080 -sTCP:LISTEN
```

Expected: Prometheus and ClickHouse containers are running; Coroot listens on 18180; aiops-v2 listens on 18080.

- [ ] **Step 3: Report endpoints and stop commands**

Report:

```text
Coroot: http://127.0.0.1:18180
aiops-v2: http://127.0.0.1:18080
Stop Coroot: kill $(cat /Users/lizhongxuan/Desktop/aiops/coroot/coroot.pid)
Stop dependencies: docker stop aiops-coroot-prometheus aiops-coroot-clickhouse
Stop aiops-v2: stop the long-running terminal session
```

Expected: user has exact URLs and stop commands.
