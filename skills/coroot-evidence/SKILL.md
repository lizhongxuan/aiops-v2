---
name: coroot-evidence
description: Use Coroot as a read-only evidence source for service health, SLO, incident timeline, topology, metrics, logs, traces, dependency propagation, and production symptoms. 中文触发: Coroot, SLO, 服务延迟, 错误率, 拓扑, 指标, 依赖故障.
tools:
  - coroot.collect_rca_context
  - coroot.service_metrics
  - coroot.service_topology
  - coroot.slo_status
  - coroot.incident_timeline
  - coroot.rca_report
when_to_use: Use when Coroot is mentioned, a Coroot webhook or incident is present, or service-level symptoms need observability evidence. Collect evidence, do not remediate.
preview: Prefer collect_rca_context, drill down only when aggregate evidence is missing, and cite rawRefs or evidence ids for non-obvious claims.
resource_types:
  - coroot
  - slo
  - incident
  - topology
  - metrics
task_intents:
  - coroot
  - slo
  - service-latency
  - error-rate
  - dependency-failure
  - topology
  - collect-evidence
  - 服务延迟
  - 错误率
  - 依赖故障
  - 服务指标
  - 拓扑
modes:
  - chat
  - inspect
  - plan
activation_mode: model
model_invocable: true
required_for_match: true
---

# Coroot Evidence Skill

Use this skill when Coroot can provide read-only observability evidence for an incident, service symptom, SLO issue, or dependency failure.

This skill gathers and interprets evidence. It does not execute remediation.

## Workflow

1. Identify the Coroot target.
   - Project, service/application, incident id, time window, environment, and symptom.
   - Prefer session-bound Coroot project when present.
   - If the service is ambiguous, ask for the smallest missing field or list candidate services when a read-only list tool is available.
2. Start with aggregate evidence.
   - Prefer `coroot.collect_rca_context` for named services, incident-driven RCA, latency, error rate, saturation, dependency failure, or SLO issues.
   - Treat the result as a model-safe evidence pack with rawRefs, not as final truth.
3. Drill down only when needed.
   - Use `coroot.service_metrics` for CPU, memory, network, latency, request rate, error rate, saturation, or chart evidence.
   - Use `coroot.service_topology` when dependency direction, upstream/downstream impact, or blast radius is unclear.
   - Use `coroot.slo_status` when user impact or SLO compliance matters.
   - Use `coroot.incident_timeline` for a specific incident id or timeline question.
   - Use `coroot.rca_report` only as optional native Coroot reference evidence.
4. Correlate evidence.
   - Compare symptom timing, deployment events, dependency direction, SLO impact, logs/traces, metric changes, and contradictory evidence.
   - Separate observed facts from inference.
5. Handle external dependencies carefully.
   - Do not finalize RCA on an external IP, DNS name, or `ExternalService` failed edge alone.
   - Resolve identity, owner, expected endpoint, protocol, DNS, and caller-to-dependency path when possible.
   - If not possible, mark RCA incomplete and list the missing evidence.
6. Report the smallest useful result.
   - Strongest hypothesis.
   - Confidence reason.
   - Key evidence refs.
   - Contradictions.
   - Next read-only drill-down or handoff to operation planning.

## Stop Conditions

Stop or mark inconclusive when:

- Coroot capability is unavailable.
- Target service or incident cannot be identified.
- Aggregate evidence is empty or contradictory and no independent read-only source is available.
- The user asks to fix before RCA evidence is sufficient.
- Evidence points to a high-risk change without workflow, approval, rollback, and postcheck.

## Output Contract

Return:

- Target and time window.
- Coroot evidence used.
- Hypothesis and confidence.
- Supporting evidence refs.
- Contradicting or missing evidence.
- Next read-only step or operation-plan handoff.

## Safety

- Do not execute remediation.
- Do not invent Coroot metrics, logs, traces, incidents, deployments, or RCA reports.
- Do not call aiops-v2 backend Coroot APIs directly; use Coroot tools.
- Do not search OpsManuals for analysis-only requests.
- Do not ask the user for screenshots when chart reports are already attached.
