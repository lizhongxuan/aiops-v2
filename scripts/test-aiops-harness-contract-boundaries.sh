#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BOUNDARY_SCRIPT="${SCRIPT_DIR}/check-aiops-harness-contract-boundaries.sh"
FIXTURE_ROOT="$(mktemp -d)"
trap 'rm -rf "${FIXTURE_ROOT}"' EXIT

fail=0

create_fixture() {
	local root="$1"
	mkdir -p \
		"${root}/internal/appui" \
		"${root}/internal/runtimekernel" \
		"${root}/web/src/chat" \
		"${root}/web/src/transport"
	printf '%s\n' \
		'package appui' \
		'func resumeApproval() {' \
		'  ResumeTurn()' \
		'}' >"${root}/internal/appui/approval_service.go"
}

run_gate() {
	local root="$1"
	AIOPS_HARNESS_SCAN_ROOTS="${root}" bash "${BOUNDARY_SCRIPT}" 2>&1
}

expect_allowed() {
	local name="$1"
	local root="$2"
	local output

	if ! output="$(run_gate "${root}")"; then
		echo "ERROR: legal fixture unexpectedly failed: ${name}" >&2
		echo "${output}" >&2
		fail=1
	fi
}

expect_rejected() {
	local name="$1"
	local root="$2"
	local rule="$3"
	local owner="$4"
	local output

	if output="$(run_gate "${root}")"; then
		echo "ERROR: bad fixture unexpectedly passed: ${name}" >&2
		fail=1
		return
	fi

	if [[ "${output}" != *"${rule}"* ]]; then
		echo "ERROR: rejected fixture omitted rule name: ${name}" >&2
		echo "${output}" >&2
		fail=1
	fi
	if [[ "${output}" != *"owner: ${owner}"* ]]; then
		echo "ERROR: rejected fixture omitted owner: ${name}" >&2
		echo "${output}" >&2
		fail=1
	fi
}

legal_root="${FIXTURE_ROOT}/legal"
markdown_root="${FIXTURE_ROOT}/markdown-verified"
dispatcher_root="${FIXTURE_ROOT}/dispatcher-bypass"
approval_root="${FIXTURE_ROOT}/approval-rerun"
multi_bad_root="${FIXTURE_ROOT}/multi-root-bypass"
multi_missing_root="${FIXTURE_ROOT}/multi-root-missing-scan-surface"

create_fixture "${legal_root}"
create_fixture "${markdown_root}"
create_fixture "${dispatcher_root}"
create_fixture "${approval_root}"
create_fixture "${multi_bad_root}"
mkdir -p "${multi_missing_root}"

printf '%s\n' \
	'export function projectFinal(finalText: string) {' \
	'  return { verified: finalText.includes("verified") };' \
	'}' >"${markdown_root}/web/src/chat/final_projection.ts"

printf '%s\n' \
	'package runtimekernel' \
	'func bypassDispatcher() {' \
	'  toolRegistry.Execute()' \
	'}' >"${dispatcher_root}/internal/runtimekernel/direct_tool.go"

printf '%s\n' \
	'package runtimekernel' \
	'func bypassDispatcherAcrossRoots() {' \
	'  toolRegistry.Execute()' \
	'}' >"${multi_bad_root}/internal/runtimekernel/direct_tool.go"

printf '%s\n' \
	'package appui' \
	'func applyApproval() {' \
	'  ResumeTurn()' \
	'  service.RunTurn()' \
	'}' >"${approval_root}/internal/appui/approval_service.go"

expect_allowed "typed runtime state" "${legal_root}"
expect_rejected \
	"final markdown inferred verified state" \
	"${markdown_root}" \
	"UI verified state inferred from final markdown" \
	"frontend final projection"
expect_rejected \
	"direct tool execution bypass" \
	"${dispatcher_root}" \
	"direct tool execution bypassing ToolDispatcher" \
	"runtimekernel dispatcher"
expect_rejected \
	"scan error cannot mask direct tool bypass" \
	"${multi_bad_root}:${multi_missing_root}" \
	"direct tool execution bypassing ToolDispatcher" \
	"runtimekernel dispatcher"
expect_rejected \
	"approval fallback to RunTurn" \
	"${approval_root}" \
	"approval decision fallback RunTurn" \
	"appui approval service"

if [[ "${fail}" -ne 0 ]]; then
	exit 1
fi

echo "aiops harness contract boundary self-test passed"
