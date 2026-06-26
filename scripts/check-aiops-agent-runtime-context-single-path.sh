#!/usr/bin/env bash
set -euo pipefail

fail=0

check_absent() {
  local pattern="$1"
  local path="$2"
  local message="$3"

  if rg -n "$pattern" "$path"; then
    echo "ERROR: $message" >&2
    fail=1
  fi
}

check_absent "handleHostOpsRoute|writeHostOpsMissionTurn" internal/appui "HostOps route must not write synthetic completed turns"
check_absent "startApprovalRecoveryFallback|buildApprovalFallbackTurnRequest" internal/appui "Approval denial must stay inside ResumeTurn, not start fallback RunTurn"
check_absent "dynamicPromptMessages\\(" internal/promptinput "Prompt input must not have a second dynamic prompt injection path"
check_absent "developerInstructionSections\\(" internal/promptcompiler "Old monolithic developer core must be removed after section migration"
check_absent "assistant_.*fallback_legacy|fallback_legacy_assistant" internal "Legacy assistant fallback projection must be removed"
check_absent "assistant_.*fallback_legacy|fallback_legacy_assistant" web/src "Legacy assistant fallback projection must be removed"
check_absent "IsAlwaysModelCallableTool" internal "Hidden-but-callable tool semantics must not exist"

if [[ "$fail" -ne 0 ]]; then
  exit 1
fi
