---
name: operation-plan
description: Use when the user asks to fix, restore, remediate, handle, execute, or turn an RCA into a governed operation plan. 中文触发: 修复, 恢复, 处理, 执行方案, 处置方案, 生成 workflow.
when_to_use: Use after a fault is understood enough to propose action, or when the user explicitly asks for a repair, recovery, workflow, runbook, preflight, or execution plan. Do not execute changes.
preview: Build an ActionProposal with target, evidence, steps, risk, prerequisites, rollback, verification, and approval requirements.
resource_types:
  - actionproposal
  - workflow
  - opsmanual
  - runbook
  - managed-resource
task_intents:
  - fix
  - remediate
  - restore
  - repair
  - operation-plan
  - preflight
  - workflow
  - runbook
  - 修复
  - 恢复
  - 处理
  - 执行方案
  - 处置方案
  - 生成workflow
modes:
  - chat
  - plan
  - execute
activation_mode: model
model_invocable: true
required_for_match: true
---

# Operation Plan Skill

Use this skill when the user wants to move from analysis to a governed repair, recovery, workflow, or runbook-backed operation.

This skill creates a plan. It does not execute the plan.

## Workflow

1. Confirm the operation target.
   - Identify service, host, cluster, managed resource, workflow, namespace, project, and time window when available.
   - If the target is ambiguous, ask for the smallest missing target field before planning execution.
2. Bind the plan to evidence.
   - Use existing RCA, Coroot evidence, logs, metrics, tool results, or user-provided evidence.
   - If no evidence supports the action, produce a read-only evidence request instead of a remediation plan.
3. Choose the operation type.
   - `mitigate`: reduce impact without claiming root cause closure.
   - `repair`: fix the likely cause.
   - `rollback`: revert a known recent change.
   - `scale`: capacity or saturation response.
   - `config_change`: controlled configuration update.
   - `manual_check`: evidence is insufficient, so no change should run yet.
4. Match existing assets.
   - Prefer existing OpsManual or Workflow candidates when the action maps cleanly.
   - If no matching workflow exists, produce a draft ActionProposal and state that execution is blocked until a workflow/manual exists or the user approves manual execution design.
5. Define prerequisites.
   - Required parameters.
   - Preconditions.
   - Permissions.
   - Resource locks.
   - Maintenance window or blast-radius constraints.
   - Backup or snapshot requirements when data risk exists.
6. Define rollback.
   - Every mutating plan must include rollback or a clear statement that rollback is not safe.
   - If rollback is unknown, execution is blocked.
7. Define verification.
   - Include postcheck signals such as Coroot status, service health, logs, command outputs, workflow result, SLO, endpoint check, or user confirmation.
   - Do not use model prose as success evidence.
8. Hand off to approval.
   - Any write, restart, deletion, failover, promote, scale, configuration change, data rewrite, or traffic shift requires explicit approval before execution.

## Stop Conditions

Stop and ask or block when:

- The target resource is missing or conflicts with session context.
- Evidence does not support the proposed action.
- The action has data-loss, availability, or security risk and lacks rollback.
- Required parameters are unresolved.
- Existing workflow/manual confidence is low.
- The user requested analysis only or prohibited execution.
- Approval is required and not yet granted.

## Output Contract

Return a concise ActionProposal:

- Target.
- Evidence basis.
- Proposed operation.
- Preconditions.
- Step list.
- Risk and impact.
- Rollback.
- Verification.
- Approval requirement.
- Blockers, if any.

## Safety

- Do not execute tools that mutate state.
- Do not claim the operation is complete before execution and postcheck evidence exist.
- Do not turn an uncertain RCA into a confident remediation.
- Do not bypass OpsManual, Workflow preflight, resource locks, approval, or postcheck.
