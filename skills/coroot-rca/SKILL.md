---
name: coroot-rca
description: 当用户要求分析服务、Coroot incident、SLO 异常、延迟、错误率、资源饱和、依赖故障或生产症状根因时使用。
tools:
  - coroot.collect_rca_context
  - coroot.service_metrics
  - coroot.service_topology
  - coroot.slo_status
  - coroot.incident_timeline
  - coroot.rca_report
---

# Coroot RCA Skill

Use this skill when the user asks for root cause analysis of a service, Coroot incident, SLO violation, latency spike, elevated error rate, resource saturation, dependency failure, or unexplained production symptom.

## Workflow

1. Identify the target: project, service, incidentId, time window, and symptoms.
2. Prefer `coroot.collect_rca_context` as the first Coroot read. It returns the model-safe RCA evidence pack and keeps raw Coroot payloads behind rawRefs.
   - The aggregate context can include SLOs, service metric summaries, dependency direction, recent incidents, error-like logs, error/slow traces, profiling availability, and deployment events.
   - Treat these as summarized evidence from Coroot MCP, not as raw Coroot API payloads.
3. Use lower-level tools only for progressive drill-down:
   - `coroot.service_metrics` only when charts or a specific metric detail is needed.
   - `coroot.service_topology` only when dependency direction or nearby services are still unclear.
   - `coroot.slo_status` only when SLO compliance or impact scope needs confirmation.
   - `coroot.incident_timeline` only when the user provides or asks about a specific incident id.
   - `coroot.rca_report` only as optional reference evidence, never as the primary RCA source. If it returns 404, Application not found, AI disabled, or unavailable while list/overview evidence shows the service exists, treat only the native RCA report as unavailable and keep investigating with aggregate context or drill-down tools.
4. Treat Coroot MCP output as evidence, not as final truth. Correlate symptom, affected target, dependency direction, time relation, and contradictory evidence before stating a root cause.
5. Do not treat an external dependency, IP, DNS name, or `ExternalService` failed-connection edge as the final root cause. First resolve what it is and why it is failing: service owner, namespace, Service/Endpoint or EndpointSlice, expected port/protocol, DNS result, and caller-to-dependency network path. If those checks are unavailable, state that RCA is not closed and list the exact missing evidence.
6. Output only the sections the user needs for the current context. If context is missing, ask for the minimum missing fields instead of forcing a Root Cause / Evidence / Impact / Next Step template.
7. Cite rawRefs, evidence ids, toolCallIds, or metric/event names for every non-obvious claim.
8. If evidence is insufficient, do not stop after one failed Coroot call. Try at least one independent read-only evidence source that matches the gap, then say the result is inconclusive and list the smallest missing evidence set.
9. Keep the final text short: strongest hypothesis, confidence reason, key evidence, contradictions, and the next read-only drill-down.

## Safety

- Do not invent metrics, events, deployment ids, query fingerprints, traces, logs, profile data, or Coroot RCA output.
- Do not ask for separate logs/traces/profiling Coroot tools unless `coroot.collect_rca_context` says the aggregate section is unavailable or insufficient.
- Do not execute remediation.
- Do not ask aiops-v2 backend to call Coroot APIs directly.
- Do not call ops manual search for analysis-only or troubleshooting-only requests. Search runbooks only when the user asks to fix/restore/execute remediation or after a clear root cause has a matching recovery intent.
- Do not return HTML, iframe, script, or arbitrary UI code.
