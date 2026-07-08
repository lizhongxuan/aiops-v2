---
name: read-only-rca
description: Use for root-cause analysis, diagnosis, troubleshooting, incident investigation, or production symptom analysis before any remediation. 中文触发: 根因, 排查, 诊断, 为什么故障, 只分析, 不执行.
when_to_use: Use when the user asks why something failed, wants diagnosis, provides evidence, or asks for analysis without execution. Gather and reason from read-only evidence before suggesting operations.
preview: Facts first, hypotheses second, contradictions third, next read-only evidence fourth. No remediation execution.
resource_types:
  - incident
  - evidence
  - logs
  - metrics
  - managed-resource
task_intents:
  - rca
  - root-cause
  - diagnose
  - troubleshoot
  - investigate
  - read-only
  - 根因
  - 排查
  - 诊断
  - 为什么
  - 只分析
  - 不执行
modes:
  - chat
  - inspect
  - plan
activation_mode: model
model_invocable: true
required_for_match: true
---

# Read-Only RCA Skill

Use this skill for diagnosis before remediation. The workflow is read-only and evidence-led.

## Workflow

1. Restate the problem as observable symptoms.
   - Affected resource.
   - Time window.
   - User impact.
   - What changed, if known.
2. Inventory evidence.
   - User-provided logs, command output, metrics, stack traces, config snippets, incident timeline, workflow results, Coroot evidence, host checks, or prior run records.
   - Treat user-provided evidence as first-class evidence.
3. Separate facts from inference.
   - Facts are directly supported by evidence or tool results.
   - Inference must be labeled as hypothesis.
4. Generate ranked hypotheses.
   - Provide up to three candidates.
   - For each candidate, explain why it fits, what contradicts it, and what evidence would confirm or falsify it.
5. Gather the next smallest read-only evidence.
   - Prefer Coroot evidence for service/SLO/topology/metrics.
   - Prefer host inspection only when a specific host target is bound.
   - Prefer OpsGraph/OpsManual search only when the user asks for known procedures or after RCA indicates a recovery intent.
6. Decide readiness.
   - `inconclusive`: evidence is insufficient or contradictory.
   - `likely_root_cause`: one hypothesis dominates but needs confirmation.
   - `confirmed_root_cause`: direct evidence supports cause, timing, impact, and mechanism.
   - `ready_for_operation_plan`: evidence supports a bounded action proposal.

## Stop Conditions

Stop and ask or report inconclusive when:

- No target or time window can be inferred.
- The request requires host evidence but no host is explicitly bound.
- The user prohibited execution and the next evidence would require host command execution.
- Evidence is contradictory.
- Only one failed tool call exists and no independent evidence supports the conclusion.
- A proposed fix would be speculative.

## Output Contract

Return:

- Known facts.
- Ranked hypotheses.
- Evidence for each hypothesis.
- Contradictions.
- Missing evidence.
- Confidence.
- Next read-only step or operation-plan handoff.

## Safety

- Do not execute remediation.
- Do not claim root cause from a single weak signal.
- Do not invent metrics, logs, command output, topology, or user impact.
- Do not ignore user-provided evidence because tool evidence is missing.
- Do not close an incident from model prose alone.
