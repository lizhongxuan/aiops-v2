# Workflow Canvas MCP test corpus

This directory is the machine-readable inventory for the 169 cases defined in
`docs/2026-07-21-aiops-v2-workflow-canvas-mcp-control-plane-design.zh.md`.

- `test-manifest.json` is the protected case/activation inventory.
- `schemas/` documents manifest v1 and fixture v1.
- `fixtures/` contains deterministic shared fixtures. W4 is a seeded recipe so
  consumers can materialize the same 500-node/700-edge graph without committing
  a large generated payload.
- `contract-results/` is validator self-test input only. It is not evidence
  that a production regression story ran.

`activationState=planned` cases are excluded from pass rates. A Phase 0
`known_failure` case passes the manifest gate only when its exact stable failure
signature is observed. At `greenActivationPhase`, it must be converted to a
normal passing case.

The repository phase is protected. `AIOPS_WORKFLOW_CASE_PHASE` may preview a
higher phase locally, but cannot lower the checked-in phase. CI can pass a review
base manifest to `workflow-test-manifest validate --base ...` to reject case
deletion, priority downgrade, activation delay, or `active -> planned`.
