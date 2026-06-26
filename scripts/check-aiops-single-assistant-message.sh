#!/usr/bin/env bash
set -euo pipefail

fail=0

check_absent() {
  local name="$1"
  local pattern="$2"
  shift 2

  if rg -n -P "$pattern" "$@" \
    --glob '!internal/agentstate/types_test.go' \
    --glob '!internal/agentstate/assistant_message_migration.go' \
    --glob '!internal/agentstate/assistant_message_migration_test.go' \
    --glob '!internal/runtimekernel/assistant_message_contract_test.go' \
    --glob '!scripts/check-aiops-single-assistant-message.sh'; then
    echo "ERROR: forbidden single-assistant-message residue found: ${name}" >&2
    fail=1
  fi
}

check_absent \
  "legacy assistant turn item constants" \
  'TurnItemTypeAssistantProgress|TurnItemTypeAssistantAnswer|TurnItemTypeFinalAnswer|TurnItemType\("assistant_progress"\)|TurnItemType\("assistant_answer"\)|TurnItemType\("final_answer"\)' \
  internal web/src web/tests testdata scripts

check_absent \
  "candidate or superseded answer fields" \
  'candidateForFinal|candidateState|answerState|supersededByIteration' \
  internal web/src web/tests testdata scripts

check_absent \
  "legacy assistant display kinds" \
  'assistant\.(answer|draft)|assistant\.final(?!\.delta)' \
  internal web/src web/tests testdata scripts

check_absent \
  "frontend candidate promotion helpers" \
  'finalCandidateForTurn|terminalAssistantAnswerIndex|hiddenFinalAssistantBlockIndexes|runningAssistantCandidateIndex|isRunningAssistantCandidateBlock|isSupersededAssistantCandidateBlock' \
  web/src web/tests

check_absent \
  "final evidence retry rewrite path" \
  'finalEvidenceRetryPrompt|finalEvidenceRetryDraftContext|finalEvidenceRetryProgressSummary' \
  internal scripts

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
