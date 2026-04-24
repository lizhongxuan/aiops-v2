# agent-dispatch

Dispatch steps to remote agent servers over HTTP.

## Start agents (token required, default `runner-token`)

```bash
# terminal 1
go run ./runner/examples/agent-server --addr :7072 --token runner-token

# terminal 2
go run ./runner/examples/agent-server --addr :7073 --token runner-token

# terminal 3
go run ./runner/examples/agent-server --addr :7074 --token runner-token
```

## Run dispatch client

```bash
go run ./runner/examples/agent-dispatch --token runner-token ./runner/examples/agent-dispatch/workflow.yaml
```

## Workflow behavior

- Step 1 runs on agent-a and exports variables by printing `KEY=VALUE`.
- Exported vars are injected into later steps as environment variables.
- Step 2 runs on agent-b and can read `$TOKEN` / `$PORT`.
- Step 3 runs on agent-c.

You can change host addresses in `workflow.yaml` to point at real agents.
