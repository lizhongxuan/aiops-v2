---
name: coroot-triage
description: Use Coroot as a read-only observability evidence source for service health, metrics, topology, incidents, SLOs, and RCA when Coroot MCP tools are available.
---

# Coroot Triage

Use Coroot tools only as read-only observability evidence. Prefer the session-bound Coroot project when present; otherwise omit `project` so the configured default is used.

When the user asks for monitoring, health, RCA, or incident investigation:

1. Start with `coroot.list_services` when the target service is ambiguous.
2. Prefer `coroot.collect_rca_context` for named services, warning/critical services, or any RCA request before drawing conclusions.
3. Use `coroot.service_metrics` whenever service health, status, RCA, CPU, memory, network, or resource evidence would benefit from metric details or a visual card. The user does not need to say "chart", "metrics", or "Agent-to-UI"; decide from the task whether metric charts would help.
4. Use `coroot.service_topology`, logs, traces, incidents, or SLO tools as follow-up drill-down when aggregate evidence is missing or insufficient.
5. Use `coroot.rca_report` only as optional native Coroot RCA reference. A 404, Application not found, AI disabled, or unavailable result from that native RCA endpoint does not prove the service is absent when service list or overview evidence found it.
6. If the strongest evidence is an external dependency, IP, DNS name, or `ExternalService` failed-connection edge, keep drilling down before finalizing: resolve the dependency identity, owner, Service/Endpoint or EndpointSlice, expected port/protocol, DNS result, and caller-to-dependency network path. If that cannot be inspected, mark the RCA as incomplete instead of calling the dependency edge the final cause.
7. If one Coroot probe fails, state that evidence source is unavailable and continue with aggregate or independent read-only evidence before finalizing.
8. Do not ask the user whether Coroot evidence exists unless the system lacks enough target information to inspect it.

`coroot.service_metrics` can return `chartReports`. Those reports are rendered by the Agent-to-UI renderer `coroot.chart.v1`. When chart reports are present, say the chart card is attached or visible; do not ask the user for screenshots.

For service/application CPU, memory, network, or resource usage, call `coroot.service_metrics`. Do not use terminal commands such as `top`, `ps`, or `exec_command` as substitutes for service metrics, because they inspect the selected host OS rather than the Coroot application.

Coroot evidence can support root-cause conclusions, but it is not the only source of truth. Separate observed facts from inference and combine Coroot evidence with logs, host data, workflow preflight, or other available read-only sources when needed.
