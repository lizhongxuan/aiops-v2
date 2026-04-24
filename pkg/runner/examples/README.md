# runner examples

Examples for the standalone runner library.

- `runner-simple`: run a workflow
- `runner-plan`: generate plan JSON
- `agent-server` + `agent-dispatch`: dispatch steps to a remote agent
- `shell.run`: inline shell script step (no script store required)
- `web-ui`: static page to run YAML and view per-step logs

## runner-simple

```bash
go run ./runner/examples/runner-simple ./examples/simple.yaml
```

## runner-plan

```bash
go run ./runner/examples/runner-plan ./examples/simple.yaml
```

## agent-server + agent-dispatch

Start the agent server:

```bash
go run ./runner/examples/agent-server --addr :7072 --token runner-token
```

Run the client dispatch example:

```bash
go run ./runner/examples/agent-dispatch --token runner-token ./runner/examples/agent-dispatch/workflow.yaml
```

## pg-restore example

```bash
go run ./runner/examples/agent-dispatch --token runner-token ./runner/examples/pg-restore.yaml
```

## shell.run example

```yaml
steps:
  - name: inline shell
    action: shell.run
    args:
      script: |
        echo "hello from shell"
        echo "BOPS_EXPORT:FOO=bar"
      export_vars: true
```

Run it:

```bash
go run ./runner/examples/agent-dispatch --token runner-token ./runner/examples/shell-run.yaml
```

## web-ui

```bash
go run ./runner/examples/web-ui
```

Then open http://localhost:8088 to run YAML and watch step logs.
