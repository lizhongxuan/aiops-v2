---
name: coroot-rca
description: 当用户要求分析服务、Coroot incident、SLO 异常、延迟、错误率、资源饱和、依赖故障或生产症状根因时使用。
tools:
  - coroot.collect_rca_context
  - coroot.service_metrics
  - coroot.service_topology
  - coroot.incident_timeline
  - coroot.query_range
  - aiops.ui_artifact_emit
---

# Coroot RCA Skill

Use this skill when the user asks for root cause analysis of a service, Coroot incident, SLO violation, latency spike, elevated error rate, resource saturation, dependency failure, or unexplained production symptom.

## Workflow

1. Identify the target: project, service, incidentId, time window, and symptoms.
2. Prefer `coroot.collect_rca_context` for evidence collection.
3. If `coroot.collect_rca_context` is unavailable, collect evidence with `coroot.service_metrics`, `coroot.service_topology`, `coroot.incident_timeline`, and `coroot.query_range`.
4. Treat Coroot MCP output as evidence, not as final truth.
5. Separate symptom, suspected cause, propagation effect, contradiction, and missing evidence.
6. Cite supportingEvidenceRefs, toolCallIds, rawRefs, or metric/event ids for every non-obvious claim.
7. If evidence is insufficient, set RCA status to `inconclusive` and list missing evidence.
8. To render UI, call `aiops.ui_artifact_emit` with `type=rca_report` and `inlineData.schemaVersion=aiops.rca_report/v1`.
9. Keep the final text short: strongest hypothesis, confidence, key evidence, contradictions, and next step.

## Safety

- Do not invent metrics, events, deployment ids, query fingerprints, traces, logs, or Coroot RCA output.
- Do not execute remediation.
- Do not ask aiops-v2 backend to call Coroot APIs directly.
- Do not return HTML, iframe, script, or arbitrary UI code.
