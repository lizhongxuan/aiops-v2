#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHECKER="${SCRIPT_DIR}/check-aiops-change-budget.sh"
FIXTURE_ROOT="$(mktemp -d)"
trap 'rm -rf "${FIXTURE_ROOT}"' EXIT

fail=0

new_repo() {
	local root="$1"
	mkdir -p "${root}/scripts"
	git -C "${root}" init -q
	git -C "${root}" config user.name "Harness Gate Selftest"
	git -C "${root}" config user.email "harness-gate@example.invalid"
	printf '%s\n' '# policy marker present before reviewed changes' >"${root}/scripts/check-aiops-change-budget.sh"
	git -C "${root}" add .
	git -C "${root}" commit -q -m "test: establish change policy"
}

base_of() {
	git -C "$1" rev-list --max-parents=0 HEAD
}

commit_plain() {
	local root="$1"
	local subject="$2"
	git -C "${root}" add .
	git -C "${root}" commit -q -m "${subject}"
}

commit_exception() {
	local root="$1"
	local kind="$2"
	local subject="$3"
	git -C "${root}" add .
	git -C "${root}" commit -q \
		-m "${subject}" \
		-m "AIOps-Change-Exception: ${kind}
AIOps-Change-Reason: fixture proves audited exception behavior
AIOps-Change-Review: docs/harness/change-review.md"
}

run_gate() {
	local root="$1"
	shift
	AIOPS_CHANGE_REPO_ROOT="${root}" \
	AIOPS_HARNESS_BASE_REF="$(base_of "${root}")" \
		"$@" bash "${CHECKER}" 2>&1
}

expect_allowed() {
	local name="$1"
	local root="$2"
	local output
	if ! output="$(run_gate "${root}")"; then
		echo "ERROR: legal change fixture unexpectedly failed: ${name}" >&2
		echo "${output}" >&2
		fail=1
	fi
}

expect_rejected() {
	local name="$1"
	local root="$2"
	local rule="$3"
	local output
	if output="$(run_gate "${root}")"; then
		echo "ERROR: violation fixture unexpectedly passed: ${name}" >&2
		fail=1
		return
	fi
	if [[ "${output}" != *"${rule}"* ]]; then
		echo "ERROR: violation fixture omitted expected rule: ${name}" >&2
		echo "expected: ${rule}" >&2
		echo "${output}" >&2
		fail=1
	fi
}

small_root="${FIXTURE_ROOT}/small"
budget_files_root="${FIXTURE_ROOT}/budget-files"
budget_lines_root="${FIXTURE_ROOT}/budget-lines"
mechanical_root="${FIXTURE_ROOT}/mechanical"
missing_reason_root="${FIXTURE_ROOT}/missing-reason"
semantic_root="${FIXTURE_ROOT}/business-string"
machine_boundary_root="${FIXTURE_ROOT}/machine-boundary"
metadata_root="${FIXTURE_ROOT}/metadata"
registered_metadata_root="${FIXTURE_ROOT}/registered-metadata"
runtime_root="${FIXTURE_ROOT}/runtime-no-story"
runtime_story_root="${FIXTURE_ROOT}/runtime-story"
ui_root="${FIXTURE_ROOT}/ui-no-screenshot"
ui_snapshot_root="${FIXTURE_ROOT}/ui-screenshot"
baseline_root="${FIXTURE_ROOT}/baseline-undeclared"
declared_baseline_root="${FIXTURE_ROOT}/baseline-declared"
playwright_screenshot_root="${FIXTURE_ROOT}/playwright-screenshot-undeclared"
declared_playwright_screenshot_root="${FIXTURE_ROOT}/playwright-screenshot-declared"
harness_golden_root="${FIXTURE_ROOT}/harness-golden-undeclared"
declared_harness_golden_root="${FIXTURE_ROOT}/harness-golden-declared"
transport_story_root="${FIXTURE_ROOT}/transport-story-undeclared"
declared_transport_story_root="${FIXTURE_ROOT}/transport-story-declared"
bootstrap_root="${FIXTURE_ROOT}/bootstrap"

for root in \
	"${small_root}" \
	"${budget_files_root}" \
	"${budget_lines_root}" \
	"${mechanical_root}" \
	"${missing_reason_root}" \
	"${semantic_root}" \
	"${machine_boundary_root}" \
	"${metadata_root}" \
	"${registered_metadata_root}" \
	"${runtime_root}" \
	"${runtime_story_root}" \
	"${ui_root}" \
	"${ui_snapshot_root}" \
	"${baseline_root}" \
	"${declared_baseline_root}" \
	"${playwright_screenshot_root}" \
	"${declared_playwright_screenshot_root}" \
	"${harness_golden_root}" \
	"${declared_harness_golden_root}" \
	"${transport_story_root}" \
	"${declared_transport_story_root}"; do
	new_repo "${root}"
done

mkdir -p "${small_root}/internal/feature"
printf '%s\n' 'package feature' 'const enabled = true' >"${small_root}/internal/feature/feature.go"
commit_plain "${small_root}" "feat: legal small production change"

for root in "${budget_files_root}" "${mechanical_root}" "${missing_reason_root}"; do
	mkdir -p "${root}/internal/feature"
	for i in 1 2 3 4 5 6; do
		printf '%s\n' 'package feature' "const Value${i} = ${i}" >"${root}/internal/feature/file${i}.go"
	done
done
commit_plain "${budget_files_root}" "feat: exceed production file budget"
commit_exception "${mechanical_root}" mechanical "refactor: audited mechanical split"
git -C "${missing_reason_root}" add .
git -C "${missing_reason_root}" commit -q \
	-m "refactor: unaudited exception" \
	-m "AIOps-Change-Exception: mechanical" \
	-m "AIOps-Change-Review: docs/harness/change-review.md"

mkdir -p "${budget_lines_root}/internal/feature"
{
	printf '%s\n' 'package feature' 'const ('
	for i in $(seq 1 501); do
		printf 'Value%s = %s\n' "${i}" "${i}"
	done
	printf '%s\n' ')'
} >"${budget_lines_root}/internal/feature/large.go"
commit_plain "${budget_lines_root}" "feat: exceed production churn budget"

mkdir -p "${semantic_root}/internal/runtimekernel" "${semantic_root}/internal/server"
printf '%s\n' \
	'package runtimekernel' \
	'func chooseRoute(userMessage string) bool { return strings.Contains(userMessage, "restart redis") }' \
	>"${semantic_root}/internal/runtimekernel/route.go"
printf '%s\n' \
	'package server' \
	'func TestHarnessStoryUsesRunTurn() { kernel.RunTurn() }' \
	>"${semantic_root}/internal/server/runtime_story_test.go"
commit_plain "${semantic_root}" "feat: add keyword route patch"

mkdir -p "${machine_boundary_root}/internal/runtimekernel" "${machine_boundary_root}/internal/server"
printf '%s\n' \
	'package runtimekernel' \
	'func validURL(rawURL string) bool { return strings.HasPrefix(rawURL, "https://") } // aiops-harness: machine-boundary URL scheme validation' \
	>"${machine_boundary_root}/internal/runtimekernel/url.go"
printf '%s\n' \
	'package server' \
	'func TestMachineBoundaryHarnessStory() { kernel.RunTurn() }' \
	>"${machine_boundary_root}/internal/server/machine_boundary_story_test.go"
commit_plain "${machine_boundary_root}" "feat: add audited machine boundary parser"

mkdir -p "${metadata_root}/internal/appui"
printf '%s\n' \
	'package appui' \
	'func write(metadata map[string]string) { metadata["aiops.control.unregistered"] = "true" }' \
	>"${metadata_root}/internal/appui/control.go"
commit_plain "${metadata_root}" "feat: add unregistered control metadata"

mkdir -p "${registered_metadata_root}/internal/appui" "${registered_metadata_root}/internal/runtimecontract" "${registered_metadata_root}/internal/server"
printf '%s\n' \
	'package runtimecontract' \
	'const MetadataControlRegistered = "aiops.control.registered"' \
	>"${registered_metadata_root}/internal/runtimecontract/metadata_keys.go"
printf '%s\n' \
	'package appui' \
	'func write(metadata map[string]string) { metadata["aiops.control.registered"] = "true" }' \
	>"${registered_metadata_root}/internal/appui/control.go"
printf '%s\n' \
	'package server' \
	'func TestMetadataHarnessStory() { transport := AssistantTransport{}; _ = transport }' \
	>"${registered_metadata_root}/internal/server/metadata_story_test.go"
commit_plain "${registered_metadata_root}" "feat: register typed control metadata"

mkdir -p "${runtime_root}/internal/runtimekernel"
printf '%s\n' 'package runtimekernel' 'const loopLimit = 8' >"${runtime_root}/internal/runtimekernel/loop.go"
commit_plain "${runtime_root}" "feat: change runtime without story"

mkdir -p "${runtime_story_root}/internal/runtimekernel" "${runtime_story_root}/internal/server"
printf '%s\n' 'package runtimekernel' 'const loopLimit = 8' >"${runtime_story_root}/internal/runtimekernel/loop.go"
printf '%s\n' \
	'package server' \
	'func TestRuntimeHarnessStory() { kernel.RunTurn() }' \
	>"${runtime_story_root}/internal/server/runtime_story_test.go"
commit_plain "${runtime_story_root}" "feat: change runtime with story"

mkdir -p "${ui_root}/web/src/pages"
printf '%s\n' 'export const Page = () => <main>changed</main>;' >"${ui_root}/web/src/pages/Page.tsx"
commit_plain "${ui_root}" "feat: change UI without screenshot"

mkdir -p "${ui_snapshot_root}/web/src/pages" "${ui_snapshot_root}/web/tests"
printf '%s\n' 'export const Page = () => <main>changed</main>;' >"${ui_snapshot_root}/web/src/pages/Page.tsx"
printf '%s\n' \
	'import { test, expect } from "@playwright/test";' \
	'test("page", async ({ page }) => { await expect(page).toHaveScreenshot("page.png"); });' \
	>"${ui_snapshot_root}/web/tests/page.spec.js"
git -C "${ui_snapshot_root}" add -f web/tests/page.spec.js
commit_plain "${ui_snapshot_root}" "feat: change UI with screenshot"

for root in "${baseline_root}" "${declared_baseline_root}"; do
	mkdir -p "${root}/web/tests"
	printf '%s\n' 'fixture baseline bytes' >"${root}/web/tests/page.snap"
done
git -C "${baseline_root}" add -f web/tests/page.snap
git -C "${declared_baseline_root}" add -f web/tests/page.snap
commit_plain "${baseline_root}" "test: drift snapshot without declaration"
commit_exception "${declared_baseline_root}" baseline "test: review expected snapshot change"

for root in "${playwright_screenshot_root}" "${declared_playwright_screenshot_root}"; do
	mkdir -p "${root}/web/tests/__screenshots__"
	printf '%s\n' 'fixture Playwright screenshot bytes' >"${root}/web/tests/__screenshots__/page-linux.txt"
done
git -C "${playwright_screenshot_root}" add -f web/tests/__screenshots__/page-linux.txt
git -C "${declared_playwright_screenshot_root}" add -f web/tests/__screenshots__/page-linux.txt
commit_plain "${playwright_screenshot_root}" "test: drift Playwright screenshot without declaration"
commit_exception "${declared_playwright_screenshot_root}" baseline "test: review expected Playwright screenshot change"

for root in "${harness_golden_root}" "${declared_harness_golden_root}"; do
	mkdir -p "${root}/internal/runtimekernel/testdata/aichat_harness_golden"
	printf '%s\n' '{"case":"single-readonly","expectedStatus":"completed"}' >"${root}/internal/runtimekernel/testdata/aichat_harness_golden/single_readonly.json"
done
commit_plain "${harness_golden_root}" "test: drift AI Chat harness golden without declaration"
commit_exception "${declared_harness_golden_root}" baseline "test: review expected AI Chat harness golden change"

for root in "${transport_story_root}" "${declared_transport_story_root}"; do
	mkdir -p "${root}/internal/server/testdata/assistant_transport_story"
	printf '%s\n' '{"name":"approval-resume","expectedStatus":"completed"}' >"${root}/internal/server/testdata/assistant_transport_story/approval_resume.json"
done
commit_plain "${transport_story_root}" "test: drift AssistantTransport story without declaration"
commit_exception "${declared_transport_story_root}" baseline "test: review expected AssistantTransport story change"

mkdir -p "${bootstrap_root}"
git -C "${bootstrap_root}" init -q
git -C "${bootstrap_root}" config user.name "Harness Gate Selftest"
git -C "${bootstrap_root}" config user.email "harness-gate@example.invalid"
printf '%s\n' 'baseline before policy' >"${bootstrap_root}/README.md"
git -C "${bootstrap_root}" add .
git -C "${bootstrap_root}" commit -q -m "chore: pre-policy baseline"
bootstrap_base="$(git -C "${bootstrap_root}" rev-parse HEAD)"
mkdir -p "${bootstrap_root}/internal/legacy"
printf '%s\n' 'package legacy' 'const BeforePolicy = true' >"${bootstrap_root}/internal/legacy/legacy.go"
git -C "${bootstrap_root}" add .
git -C "${bootstrap_root}" commit -q -m "feat: legacy change before enforceable policy"
mkdir -p "${bootstrap_root}/scripts"
printf '%s\n' '# policy introduced' >"${bootstrap_root}/scripts/check-aiops-change-budget.sh"
mkdir -p "${bootstrap_root}/internal/bootstrap"
for i in 1 2 3 4 5 6; do
	printf '%s\n' 'package bootstrap' "const Baseline${i} = ${i}" >"${bootstrap_root}/internal/bootstrap/file${i}.go"
done
git -C "${bootstrap_root}" add .
git -C "${bootstrap_root}" commit -q -m "ci: introduce policy"

expect_allowed "small production change" "${small_root}"
expect_rejected "production file budget" "${budget_files_root}" "change budget exceeded"
expect_rejected "production churn budget" "${budget_lines_root}" "change budget exceeded"
expect_allowed "audited mechanical exception" "${mechanical_root}"
expect_rejected "exception without reason" "${missing_reason_root}" "change budget exceeded"
expect_rejected "core business string semantics" "${semantic_root}" "core business string semantics added"
expect_allowed "annotated fixed machine boundary" "${machine_boundary_root}"
expect_rejected "unregistered control metadata" "${metadata_root}" "unregistered aiops.* control metadata added"
expect_allowed "registered control metadata" "${registered_metadata_root}"
expect_rejected "runtime change without story" "${runtime_root}" "runtime production change lacks story/integration proof"
expect_allowed "runtime change with story" "${runtime_story_root}"
expect_rejected "UI change without screenshot" "${ui_root}" "UI production change lacks toHaveScreenshot coverage"
expect_allowed "UI change with screenshot" "${ui_snapshot_root}"
expect_rejected "undeclared snapshot drift" "${baseline_root}" "undeclared golden/snapshot drift"
expect_allowed "declared baseline change" "${declared_baseline_root}"
expect_rejected "undeclared Playwright __screenshots__ drift" "${playwright_screenshot_root}" "undeclared golden/snapshot drift"
expect_allowed "declared Playwright __screenshots__ change" "${declared_playwright_screenshot_root}"
expect_rejected "undeclared AI Chat harness golden drift" "${harness_golden_root}" "undeclared golden/snapshot drift"
expect_allowed "declared AI Chat harness golden change" "${declared_harness_golden_root}"
expect_rejected "undeclared AssistantTransport story drift" "${transport_story_root}" "undeclared golden/snapshot drift"
expect_allowed "declared AssistantTransport story change" "${declared_transport_story_root}"

bootstrap_output=""
if bootstrap_output="$(
	AIOPS_CHANGE_REPO_ROOT="${bootstrap_root}" \
	AIOPS_HARNESS_BASE_REF="${bootstrap_base}" \
	AIOPS_HARNESS_REQUIRE_BASELINE_ACK=1 \
		bash "${CHECKER}" 2>&1
)"; then
	echo "ERROR: pre-policy history passed without explicit acknowledgement" >&2
	fail=1
elif [[ "${bootstrap_output}" != *"pre-policy history requires explicit baseline acknowledgement"* ]]; then
	echo "ERROR: pre-policy rejection omitted acknowledgement rule" >&2
	echo "${bootstrap_output}" >&2
	fail=1
fi
if ! bootstrap_output="$(
	AIOPS_CHANGE_REPO_ROOT="${bootstrap_root}" \
	AIOPS_HARNESS_BASE_REF="${bootstrap_base}" \
	AIOPS_HARNESS_REQUIRE_BASELINE_ACK=1 \
	AIOPS_CHANGE_BASELINE_ACK="2026-07-14-policy-bootstrap" \
	AIOPS_CHANGE_BASELINE_REASON="existing commits predate the enforceable gate" \
		bash "${CHECKER}" 2>&1
)"; then
	echo "ERROR: explicitly acknowledged pre-policy baseline failed" >&2
	echo "${bootstrap_output}" >&2
	fail=1
fi

if [[ "${fail}" -ne 0 ]]; then
	exit 1
fi

echo "aiops harness change budget self-test passed"
