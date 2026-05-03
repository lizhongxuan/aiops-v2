#!/usr/bin/env bash
set -euo pipefail

tmp="$(mktemp -d)"
latest_file=".data/prompt_optimization/latest_run.txt"
had_latest=0
if [ -f "$latest_file" ]; then
  cp "$latest_file" "$tmp/old_latest_run.txt"
  had_latest=1
fi

cleanup() {
  if [ "$had_latest" -eq 1 ]; then
    mkdir -p "$(dirname "$latest_file")"
    cp "$tmp/old_latest_run.txt" "$latest_file"
  else
    rm -f "$latest_file"
  fi
  rm -rf "$tmp"
}
trap cleanup EXIT

bash -n ./scripts/prompt-regression.sh

./scripts/prompt-regression.sh \
  --agent mock \
  --priority P0 \
  --cases testdata/eval_cases \
  --trace-dir "$tmp/traces" \
  --out "$tmp/run" \
  --run-id prompt-regression-smoke

test -f "$tmp/run/diagnosis.json"
test -f "$tmp/run/diagnosis.zh.md"
test -f "$tmp/run/compare.zh.md"
test -f "$tmp/run/trace-links.md"
test -f "$tmp/run/suggestions.zh.md"

./scripts/prompt-regression.sh \
  --agent mock \
  --case-id simple-no-plan \
  --cases testdata/eval_cases \
  --trace-dir "$tmp/traces" \
  --out "$tmp/run-one" \
  --run-id prompt-regression-one

test "$(jq -r '.summary.total' "$tmp/run-one/eval/report.json")" = "1"
test "$(cat .data/prompt_optimization/latest_run.txt)" = "$tmp/run-one"

cat > "$tmp/failed-report.json" <<'JSON'
{
  "cases": [
    {"caseId": "tool-calling", "passed": false, "score": 0}
  ]
}
JSON

./scripts/prompt-regression.sh \
  --agent mock \
  --failed-from "$tmp/failed-report.json" \
  --cases testdata/eval_cases \
  --trace-dir "$tmp/traces" \
  --out "$tmp/run-failed-from" \
  --run-id prompt-regression-failed-from

test "$(jq -r '.summary.total' "$tmp/run-failed-from/eval/report.json")" = "1"
