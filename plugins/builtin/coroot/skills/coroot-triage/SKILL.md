---
name: coroot-triage
description: Use Coroot as a read-only observability evidence source for service health, metrics, topology, incidents, SLOs, and RCA when Coroot MCP tools are available.
---

# Coroot Triage

Use Coroot tools only as read-only observability evidence. Prefer the session-bound Coroot project when present; otherwise omit `project` so the configured default is used.

When the user asks for monitoring, health, RCA, or incident investigation:

1. Start with `coroot.list_services` when the target service is ambiguous.
2. Use `coroot.service_metrics` for named services or warning/critical services before drawing RCA conclusions.
3. Use `coroot.rca_report` or `coroot.service_topology` when root-cause or dependency evidence is needed.
4. If a Coroot probe fails, state that Coroot evidence is unavailable and continue with other evidence.
5. Do not ask the user whether Coroot evidence exists unless the system lacks enough target information to inspect it.

`coroot.service_metrics` can return `chartReports`. Those reports are rendered by the Agent-to-UI renderer `coroot.chart.v1`. When chart reports are present, say the chart card is attached or visible; do not ask the user for screenshots.

Coroot evidence can support root-cause conclusions, but it is not the only source of truth. Separate observed facts from inference and combine Coroot evidence with logs, host data, workflow preflight, or other available read-only sources when needed.
