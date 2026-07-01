#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

scope=(
  internal/appui
  internal/runtimekernel
  internal/tooling
  internal/taskdepth
)

patterns=(
  'func userProhibitsHostExec\b'
  '\buserProhibitsHostExec\('
  '\bcontainsAnyRuntimeString\b'
  '\bgeneralOpsLooksLike[A-Za-z0-9_]*\b'
  '\bpostRunLooksLike[A-Za-z0-9_]*\b'
  '\bmissingPostgresTimeline[A-Za-z0-9_]*\b'
  '\bdomainPostgresTimeline[A-Za-z0-9_]*\b'
  '\bunsupportedTimelineAuthority[A-Za-z0-9_]*\b'
  'func looksLikeLogEvidence\b'
  '\blooksLikeLogEvidence\('
  '\blooksLike[A-Za-z0-9_]*Evidence\b'
  'func looksLikeRecoveryConfigEvidence\b'
  '\blooksLikeRecoveryConfigEvidence\('
  'func looksLikeDatabaseControlTimelineEvidence\b'
  '\blooksLikeDatabaseControlTimelineEvidence\('
  'func userRequestsPublicResearch\b'
  '\buserRequestsPublicResearch\('
  'func looksLikePostgresTimelineRCAAnswer\b'
  '\blooksLikePostgresTimelineRCAAnswer\('
  '\bcontainsAny\('
  'strings\.Contains\(lower,\s*"(pg_|postgres|postgresql|pg_auto_failover|pgbackrest|timeline|kubernetes|nginx|redis|mysql)'
)

allowlist='(_test\.go:|internal/appui/intent_frame_builder\.go:|internal/appui/user_evidence_extractor\.go:|internal/runtimekernel/risky_advice_guard\.go:|internal/runtimekernel/genericity_guard\.go:|internal/opsmanual/)'
tmp_file="$(mktemp)"
trap 'rm -f "$tmp_file"' EXIT

for pattern in "${patterns[@]}"; do
  rg -n --pcre2 "$pattern" "${scope[@]}" >>"$tmp_file" || true
done

if [[ -s "$tmp_file" ]]; then
  blocked="$(grep -Ev "$allowlist" "$tmp_file" || true)"
  if [[ -n "$blocked" ]]; then
    printf 'core scenario-patch string semantics found:\n%s\n' "$blocked" >&2
    exit 1
  fi
fi

printf 'no core scenario-patch string semantics found\n'
