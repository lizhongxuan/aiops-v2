---
name: ops-triage
description: Use as the first operational intake protocol for alerts, incidents, production symptoms, ambiguous ops requests, or user reports that need routing into RCA, planning, rollback, verification, or advisory mode. 中文触发: 告警入口, 故障分诊, 运维问题, 帮我看看, 线上异常.
when_to_use: Use to classify an operational turn into advisory, read-only RCA, Coroot evidence collection, operation planning, rollback, verification, or closure. Keep the user experience simple and route to the next protocol.
preview: Determine intent, target, evidence, risk, missing fields, and the next protocol without exposing skill names to the user.
resource_types:
  - incident
  - case
  - alert
  - managed-resource
task_intents:
  - triage
  - intake
  - classify
  - route
  - incident
  - alert
  - ops
  - 告警
  - 故障分诊
  - 运维问题
  - 线上异常
  - 帮我看看
modes:
  - chat
  - inspect
  - plan
activation_mode: model
model_invocable: true
required_for_match: true
---

# Ops Triage Skill

Use this skill at the start of an operational case or when the user's request is ambiguous.

The goal is to route the user into the smallest safe workflow, not to expose internal skill choices.

## Workflow

1. Identify the user goal.
   - Advisory: explanation or general guidance.
   - Read-only RCA: diagnose symptoms or root cause.
   - Evidence collection: gather Coroot, logs, metrics, topology, or host state.
   - Operation plan: propose repair, recovery, workflow, or runbook-backed action.
   - Rollback: revert a known or suspected change.
   - Verification: confirm recovery or validate a previous action.
   - Closure: summarize evidence, decision, action, verification, and learning candidates.
2. Identify the target.
   - Service, host, managed resource, incident id, project, environment, namespace, cluster, time window, and business owner when available.
   - Never invent a default host or resource.
3. Identify available evidence.
   - User-provided logs, command output, metrics, screenshots, Coroot alert, incident timeline, workflow result, or prior run record.
   - Treat user-provided evidence as first-class evidence.
4. Identify constraints.
   - Analysis only.
   - No host execution.
   - No public web.
   - Read-only first.
   - Maintenance window.
   - Approval required.
5. Choose the next protocol.
   - Use `read-only-rca` for diagnosis or root-cause requests.
   - Use `coroot-evidence` when Coroot, SLO, service metrics, topology, or incident evidence is relevant.
   - Use `operation-plan` when the user asks to fix, restore, execute, or generate a workflow.
   - If the target is missing, ask one minimal question instead of starting tools.

## Stop Conditions

Stop and ask a minimal question when:

- The request implies execution but the target is missing.
- The user says to fix something without enough evidence to choose an action.
- Multiple resources could match the request.
- User constraints conflict with the selected route.
- A Coroot or host-backed path is relevant but the configured capability is unavailable.

## User Experience Rules

- Do not mention internal skill names unless the user asks for implementation details.
- Show user-facing states such as "reading evidence", "building plan", "waiting for approval", or "verifying recovery".
- Keep the initial response short.
- Prefer one next step over a menu of internal options.

## Output Contract

Return:

- Route.
- Target summary.
- Evidence summary.
- Constraints.
- Missing fields.
- Next protocol or next user question.

## Safety

- Do not execute remediation.
- Do not bind to a default host.
- Do not treat tool failure as target health.
- Do not close a case from model text alone.
