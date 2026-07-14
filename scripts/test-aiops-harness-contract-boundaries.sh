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
		"${root}/cmd/agent-eval" \
		"${root}/internal/appui" \
		"${root}/internal/eval" \
		"${root}/internal/promptinput" \
		"${root}/internal/runtimekernel" \
		"${root}/web/src/chat" \
		"${root}/web/src/dist" \
		"${root}/web/src/transport"
	printf '%s\n' \
		'package appui' \
		'func resumeApproval() {' \
		'  ResumeTurn()' \
		'}' >"${root}/internal/appui/approval_service.go"
	printf '%s\n' \
		'package runtimekernel' \
		'func runTurn() {' \
		'  k.observeRuntimeStage(ctx, session.ID, turnID, iteration, "turn_assembly_built")' \
		'  k.observeRuntimeStage(ctx, session.ID, turnID, iteration, "prompt_compiled")' \
		'  stepCtx, promptBuild, modelErr := k.buildRuntimeStepContext(req, session, agentKind, iteration, contextState, contextMessages, compiled, runtimeToolSurface, RuntimeStepControlFacts{})' \
		'  dispatcher = dispatcher.' \
		'    WithStepToolRouter(runtimeToolSurface).' \
		'    Done()' \
		'}' >"${root}/internal/runtimekernel/runtime_kernel.go"
	printf '%s\n' \
		'package runtimekernel' \
		'func buildRuntimeStepContext() {' \
		'  providerReq := ProviderRequestSnapshot{' \
		'    Tools: providerToolSpecsFromRuntimeToolSurface(toolSurface),' \
		'  }' \
		'}' \
		'func providerToolSpecsFromStepToolRouter(surface StepToolRouter) {}' \
		'func providerToolSpecsFromRuntimeToolSurface(surface RuntimeToolRouterSnapshot) {' \
		'  return providerToolSpecsFromStepToolRouter(surface)' \
		'}' \
		>"${root}/internal/runtimekernel/step_builder.go"
	printf '%s\n' \
		'package promptinput' \
		'func validateModelInput() {' \
		'  fail("model input must begin with L0 then L1")' \
		'  fail("model input L6 must be last")' \
		'}' >"${root}/internal/promptinput/model_input_validation.go"
	printf '%s\n' \
		'// if (markdown.includes("completed")) return { status: "completed" };' \
		'/*' \
		' * const candidate = markdown;' \
		' * if (candidate.includes("approved")) return { status: "approved" };' \
		' * const normalized = markdown.trim();' \
		' * if (normalized.includes("completed")) return { status: "completed" };' \
		' * const localeCandidate = markdown.trim().toLocaleLowerCase();' \
		' * if (localeCandidate.includes("failed")) return { status: "failed" };' \
		' * const first = markdown;' \
		' * const second = normalizeForControl(first);' \
		' * if (second.includes("blocked")) return { status: "blocked" };' \
		' */' >"${root}/web/src/chat/control_comments.ts"
	printf '%s\n' \
		'package appui' \
		'// candidate := strings.ToLower(finalText)' \
		'// if strings.Contains(candidate, "approved") { return }' \
		'/*' \
		'candidate := strings.ToLower(strings.TrimSpace(finalText))' \
		'first := finalText' \
		'second := normalizeForControl(first)' \
		'if strings.Contains(second, "blocked") { return }' \
		'*/' \
		>"${root}/internal/appui/control_comments.go"
	printf '%s\n' \
		'package appui' \
		'func displayOnly(finalText string) string {' \
		'  sanitized := strings.ToLower(strings.TrimSpace(finalText))' \
		'  return renderMarkdown(sanitized)' \
		'}' \
		>"${root}/internal/appui/final_display.go"
	printf '%s\n' \
		'export function displayOnly(markdown: string) {' \
		'  const sanitized = markdown.trim().toLocaleLowerCase();' \
		'  const displayValue = escapeForDisplay(sanitized);' \
		'  return renderMarkdown(displayValue);' \
		'}' \
		>"${root}/web/src/chat/finalDisplay.ts"
	printf '%s\n' \
		'export function ignoredGeneratedControl(markdown: string) {' \
		'  if (markdown.includes("completed")) return "completed";' \
		'}' \
		>"${root}/web/src/dist/generated.ts"
	printf '%s\n' \
		'package appui' \
		'func ignoredTestControl(finalText string) {' \
		'  if strings.Contains(finalText, "failed") { return }' \
		'}' \
		>"${root}/internal/appui/control_test.go"
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
eval_state_root="${FIXTURE_ROOT}/eval-state-endpoint"
runtime_final_root="${FIXTURE_ROOT}/runtime-final-control"
appui_final_root="${FIXTURE_ROOT}/appui-final-control"
web_final_root="${FIXTURE_ROOT}/web-final-control"
assembly_marker_root="${FIXTURE_ROOT}/assembly-marker-missing"
step_router_root="${FIXTURE_ROOT}/step-router-marker-missing"
prompt_first_root="${FIXTURE_ROOT}/prompt-first-validator-missing"
prompt_last_root="${FIXTURE_ROOT}/prompt-last-validator-missing"
provider_call_root="${FIXTURE_ROOT}/provider-router-call-missing"
dispatcher_binding_root="${FIXTURE_ROOT}/dispatcher-router-binding-missing"
alias_final_root="${FIXTURE_ROOT}/aliased-final-control"
go_transform_alias_root="${FIXTURE_ROOT}/go-transformed-aliased-final-control"
ts_transform_alias_root="${FIXTURE_ROOT}/ts-transformed-aliased-final-control"
provider_adapter_root="${FIXTURE_ROOT}/provider-router-adapter-missing"
step_call_surface_root="${FIXTURE_ROOT}/step-call-wrong-tool-surface"
nested_go_alias_root="${FIXTURE_ROOT}/nested-go-final-control"
locale_ts_alias_root="${FIXTURE_ROOT}/locale-ts-final-control"
two_hop_go_alias_root="${FIXTURE_ROOT}/two-hop-go-final-control"
two_hop_ts_alias_root="${FIXTURE_ROOT}/two-hop-ts-final-control"
template_three_hop_root="${FIXTURE_ROOT}/template-three-hop-final-control"
go_closure_capture_root="${FIXTURE_ROOT}/go-closure-capture-final-control"
ts_closure_capture_root="${FIXTURE_ROOT}/ts-closure-capture-final-control"
ts_default_parameter_root="${FIXTURE_ROOT}/ts-default-parameter-final-control"

create_fixture "${legal_root}"
create_fixture "${markdown_root}"
create_fixture "${dispatcher_root}"
create_fixture "${approval_root}"
create_fixture "${multi_bad_root}"
create_fixture "${eval_state_root}"
create_fixture "${runtime_final_root}"
create_fixture "${appui_final_root}"
create_fixture "${web_final_root}"
create_fixture "${assembly_marker_root}"
create_fixture "${step_router_root}"
create_fixture "${prompt_first_root}"
create_fixture "${prompt_last_root}"
create_fixture "${provider_call_root}"
create_fixture "${dispatcher_binding_root}"
create_fixture "${alias_final_root}"
create_fixture "${go_transform_alias_root}"
create_fixture "${ts_transform_alias_root}"
create_fixture "${provider_adapter_root}"
create_fixture "${step_call_surface_root}"
create_fixture "${nested_go_alias_root}"
create_fixture "${locale_ts_alias_root}"
create_fixture "${two_hop_go_alias_root}"
create_fixture "${two_hop_ts_alias_root}"
create_fixture "${template_three_hop_root}"
create_fixture "${go_closure_capture_root}"
create_fixture "${ts_closure_capture_root}"
create_fixture "${ts_default_parameter_root}"
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

printf '%s\n' \
	'package eval' \
	'const legacyStateEndpoint = "/api/v1/state"' \
	>"${eval_state_root}/internal/eval/legacy_state.go"

printf '%s\n' \
	'package runtimekernel' \
	'func finalControl(finalText string) string {' \
	'  if strings.Contains(finalText, "approved") { return "approval_granted" }' \
	'  return "pending"' \
	'}' >"${runtime_final_root}/internal/runtimekernel/final_control.go"

printf '%s\n' \
	'package appui' \
	'func finalControl(finalText string) string {' \
	'  if strings.Contains(finalText, "blocked") { return "blocked" }' \
	'  return "running"' \
	'}' >"${appui_final_root}/internal/appui/final_control.go"

printf '%s\n' \
	'export function finalControl(markdown: string) {' \
	'  if (markdown.includes("completed")) return { status: "completed" };' \
	'  return { status: "running" };' \
	'}' >"${web_final_root}/web/src/chat/finalControl.ts"

printf '%s\n' \
	'package runtimekernel' \
	'func runTurn() {' \
	'  k.observeRuntimeStage(ctx, session.ID, turnID, iteration, "prompt_compiled")' \
	'  stepCtx, promptBuild, modelErr := k.buildRuntimeStepContext(req, session, agentKind, iteration, contextState, contextMessages, compiled, runtimeToolSurface, RuntimeStepControlFacts{})' \
	'  dispatcher = dispatcher.' \
	'    WithStepToolRouter(runtimeToolSurface).' \
	'    Done()' \
	'}' >"${assembly_marker_root}/internal/runtimekernel/runtime_kernel.go"

printf '%s\n' \
	'package runtimekernel' \
	'func buildProviderToolsWithoutSharedRouter() {}' \
	>"${step_router_root}/internal/runtimekernel/step_builder.go"

printf '%s\n' \
	'package promptinput' \
	'func validateModelInput() {' \
	'  fail("model input L6 must be last")' \
	'}' >"${prompt_first_root}/internal/promptinput/model_input_validation.go"

printf '%s\n' \
	'package promptinput' \
	'func validateModelInput() {' \
	'  fail("model input must begin with L0 then L1")' \
	'}' >"${prompt_last_root}/internal/promptinput/model_input_validation.go"

printf '%s\n' \
	'package runtimekernel' \
	'func providerToolSpecsFromStepToolRouter(surface StepToolRouter) {}' \
	'func buildRuntimeStepContextWithoutProviderRouterCall() {}' \
	>"${provider_call_root}/internal/runtimekernel/step_builder.go"

printf '%s\n' \
	'package runtimekernel' \
	'func runTurn() {' \
	'  k.observeRuntimeStage(ctx, session.ID, turnID, iteration, "turn_assembly_built")' \
	'  k.observeRuntimeStage(ctx, session.ID, turnID, iteration, "prompt_compiled")' \
	'  stepCtx, promptBuild, modelErr := k.buildRuntimeStepContext(req, session, agentKind, iteration, contextState, contextMessages, compiled, runtimeToolSurface, RuntimeStepControlFacts{})' \
	'  // WithStepToolRouter(runtimeToolSurface)' \
	'}' >"${dispatcher_binding_root}/internal/runtimekernel/runtime_kernel.go"

printf '%s\n' \
	'package appui' \
	'func aliasControl(finalText string) string {' \
	'  candidate := finalText' \
	'  if strings.Contains(candidate, "approved") { return "approval_granted" }' \
	'  return "pending"' \
	'}' >"${alias_final_root}/internal/appui/alias_control.go"

printf '%s\n' \
	'export function aliasControl(markdown: string) {' \
	'  const candidate = markdown;' \
	'  if (candidate.includes("completed")) return { status: "completed" };' \
	'  return { status: "running" };' \
	'}' >"${alias_final_root}/web/src/chat/aliasControl.ts"

printf '%s\n' \
	'package appui' \
	'func transformedAliasControl(finalText string) string {' \
	'  candidate := strings.ToLower(finalText)' \
	'  if strings.Contains(candidate, "approved") { return "approval_granted" }' \
	'  return "pending"' \
	'}' >"${go_transform_alias_root}/internal/appui/alias_control.go"

printf '%s\n' \
	'export function transformedAliasControl(markdown: string) {' \
	'  const candidate = markdown.trim();' \
	'  if (candidate.includes("completed")) return { status: "completed" };' \
	'  return { status: "running" };' \
	'}' >"${ts_transform_alias_root}/web/src/chat/aliasControl.ts"

printf '%s\n' \
	'package runtimekernel' \
	'func buildRuntimeStepContext() {' \
	'  providerReq := ProviderRequestSnapshot{' \
	'    Tools: providerToolSpecsFromRuntimeToolSurface(toolSurface),' \
	'  }' \
	'}' \
	'func providerToolSpecsFromStepToolRouter(surface StepToolRouter) {}' \
	'func providerToolSpecsFromRuntimeToolSurface(surface RuntimeToolRouterSnapshot) {' \
	'  return providerToolSpecsFromDifferentRouter(surface)' \
	'}' \
	>"${provider_adapter_root}/internal/runtimekernel/step_builder.go"

printf '%s\n' \
	'package runtimekernel' \
	'func runTurn() {' \
	'  k.observeRuntimeStage(ctx, session.ID, turnID, iteration, "turn_assembly_built")' \
	'  k.observeRuntimeStage(ctx, session.ID, turnID, iteration, "prompt_compiled")' \
	'  stepCtx, promptBuild, modelErr := k.buildRuntimeStepContext(req, session, agentKind, iteration, contextState, contextMessages, compiled, otherToolSurface, RuntimeStepControlFacts{})' \
	'  dispatcher = dispatcher.' \
	'    WithStepToolRouter(runtimeToolSurface).' \
	'    Done()' \
	'}' >"${step_call_surface_root}/internal/runtimekernel/runtime_kernel.go"

printf '%s\n' \
	'package appui' \
	'func nestedTransformControl(finalText string) string {' \
	'  candidate := strings.ToLower(strings.TrimSpace(finalText))' \
	'  if strings.Contains(candidate, "approved") { return "approval_granted" }' \
	'  return "pending"' \
	'}' >"${nested_go_alias_root}/internal/appui/nested_control.go"

printf '%s\n' \
	'export function localeTransformControl(markdown: string) {' \
	'  const candidate = markdown.trim().toLocaleLowerCase();' \
	'  if (candidate.includes("completed")) return { status: "completed" };' \
	'  return { status: "running" };' \
	'}' >"${locale_ts_alias_root}/web/src/chat/localeControl.ts"

printf '%s\n' \
	'package runtimekernel' \
	'func twoHopControl(finalText string) string {' \
	'  first := finalText' \
	'  second := normalizeForControl(first)' \
	'  if strings.Contains(second, "blocked") { return "blocked" }' \
	'  return "running"' \
	'}' >"${two_hop_go_alias_root}/internal/runtimekernel/two_hop_control.go"

printf '%s\n' \
	'export function twoHopControl(markdown: string) {' \
	'  const first = markdown;' \
	'  const second = normalizeForControl(first);' \
	'  if (second.includes("failed")) return { status: "failed" };' \
	'  return { status: "running" };' \
	'}' >"${two_hop_ts_alias_root}/web/src/transport/twoHopControl.ts"

printf '%s\n' \
	'export function templateThreeHopControl(markdown: string) {' \
	'  const first = `${markdown}`;' \
	'  const second = normalizeForControl(first);' \
	'  const third = second;' \
	'  if (third.includes("failed")) return { status: "failed" };' \
	'  return { status: "running" };' \
	'}' >"${template_three_hop_root}/web/src/chat/templateControl.ts"

printf '%s\n' \
	'package appui' \
	'func closureCaptureControl(finalText string) string {' \
	'  candidate := normalizeForControl(finalText)' \
	'  result := "running"' \
	'  func() {' \
	'    captured := candidate' \
	'    if strings.Contains(captured, "blocked") { result = "blocked" }' \
	'  }()' \
	'  return result' \
	'}' >"${go_closure_capture_root}/internal/appui/closure_control.go"

printf '%s\n' \
	'export function closureCaptureControl(markdown: string) {' \
	'  const candidate = normalizeForControl(markdown);' \
	'  return () => {' \
	'    const captured = candidate;' \
	'    if (captured.includes("completed")) return { status: "completed" };' \
	'    return { status: "running" };' \
	'  };' \
	'}' >"${ts_closure_capture_root}/web/src/transport/closureControl.ts"

printf '%s\n' \
	'export function defaultParameterControl(markdown: string) {' \
	'  const outerCandidate = normalizeForControl(markdown);' \
	'  const predicate = (candidate = outerCandidate) => {' \
	'    if (candidate.includes("failed")) return false;' \
	'    return true;' \
	'  };' \
	'  return predicate();' \
	'}' >"${ts_default_parameter_root}/web/src/transport/defaultParameterControl.ts"

printf '%s\n' \
	'package appui' \
	'func closureDisplayOnly(finalText string) string {' \
	'  candidate := normalizeForDisplay(finalText)' \
	'  rendered := ""' \
	'  func() {' \
	'    captured := candidate' \
	'    rendered = renderMarkdown(captured)' \
	'  }()' \
	'  return rendered' \
	'}' \
	'func closureSiblingShadow(finalText string) bool {' \
	'  candidate := normalizeForDisplay(finalText)' \
	'  _ = func() string { return renderMarkdown(candidate) }' \
	'  return func() bool {' \
	'    candidate := "typed safe value"' \
	'    return strings.Contains(candidate, "failed")' \
	'  }()' \
	'}' >"${legal_root}/internal/appui/closure_display.go"

printf '%s\n' \
	'export function closureDisplayOnly(markdown: string) {' \
	'  const candidate = normalizeForDisplay(markdown);' \
	'  const displayClosure = () => {' \
	'    const captured = candidate;' \
	'    return renderMarkdown(captured);' \
	'  };' \
	'  const typedSibling = () => {' \
	'    const candidate = "typed safe value";' \
	'    if (candidate.includes("failed")) return false;' \
	'    return true;' \
	'  };' \
	'  const ordinaryTemplate = `markdown`;' \
	'  if (ordinaryTemplate.includes("failed")) return null;' \
	'  return typedSibling() ? displayClosure() : null;' \
	'}' >"${legal_root}/web/src/chat/closureDisplay.ts"

printf '%s\n' \
	'package appui' \
	'func anonymousParameterShadow(finalText string) bool {' \
	'  candidate := normalizeForDisplay(finalText)' \
	'  return func(candidate string) bool {' \
	'    if strings.Contains(candidate, "failed") { return false }' \
	'    return true' \
	'  }("typed safe value")' \
	'}' >"${legal_root}/internal/appui/parameter_shadow.go"

printf '%s\n' \
	'export function arrowParameterShadow(markdown: string) {' \
	'  const candidate = normalizeForDisplay(markdown);' \
	'  const typedPredicate = (candidate: string) => {' \
	'    if (candidate.includes("failed")) return false;' \
	'    return true;' \
	'  };' \
	'  const safeDefaultPredicate = (candidate: string = "typed safe value") => {' \
	'    if (candidate.includes("failed")) return false;' \
	'    return true;' \
	'  };' \
	'  return typedPredicate("typed safe value") && safeDefaultPredicate();' \
	'}' >"${legal_root}/web/src/transport/parameterShadow.ts"

expect_allowed "typed runtime state with comment and display-only final text" "${legal_root}"
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
expect_rejected \
	"eval legacy state endpoint" \
	"${eval_state_root}" \
	"agent eval legacy state endpoint" \
	"eval AssistantTransport adapter"
expect_rejected \
	"runtime control derived from final text" \
	"${runtime_final_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"appui control derived from final text" \
	"${appui_final_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"web control derived from markdown" \
	"${web_final_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"TurnAssembly before prompt marker missing" \
	"${assembly_marker_root}" \
	"TurnAssembly before prompt production marker" \
	"runtimekernel turn admission"
expect_rejected \
	"shared StepToolRouter provider marker missing" \
	"${step_router_root}" \
	"StepToolRouter provider request wiring" \
	"runtimekernel step builder"
expect_rejected \
	"L0 L1 first validator missing" \
	"${prompt_first_root}" \
	"model input L0/L1 first validator" \
	"promptinput model input validator"
expect_rejected \
	"L6 last validator missing" \
	"${prompt_last_root}" \
	"model input L6 last validator" \
	"promptinput model input validator"
expect_rejected \
	"provider StepToolRouter call missing" \
	"${provider_call_root}" \
	"StepToolRouter provider request wiring" \
	"runtimekernel step builder"
expect_rejected \
	"dispatcher StepToolRouter binding missing" \
	"${dispatcher_binding_root}" \
	"StepToolRouter dispatcher binding marker" \
	"runtimekernel dispatcher"
expect_rejected \
	"aliased final text controls state" \
	"${alias_final_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"Go transformed aliased final text controls state" \
	"${go_transform_alias_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"TypeScript transformed aliased markdown controls state" \
	"${ts_transform_alias_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"provider StepToolRouter adapter missing" \
	"${provider_adapter_root}" \
	"StepToolRouter provider surface adapter" \
	"runtimekernel step builder"
expect_rejected \
	"runtime step context receives a different tool surface" \
	"${step_call_surface_root}" \
	"runtime step context StepToolRouter binding" \
	"runtimekernel step admission"
expect_rejected \
	"nested Go transform controls state" \
	"${nested_go_alias_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"TypeScript locale transform controls state" \
	"${locale_ts_alias_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"two-hop Go alias controls state" \
	"${two_hop_go_alias_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"two-hop TypeScript alias controls state" \
	"${two_hop_ts_alias_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"template interpolation three-hop alias controls state" \
	"${template_three_hop_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"Go anonymous closure captures tainted final text" \
	"${go_closure_capture_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"TypeScript arrow closure captures tainted markdown" \
	"${ts_closure_capture_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"
expect_rejected \
	"TypeScript default parameter captures tainted outer alias" \
	"${ts_default_parameter_root}" \
	"control state derived from final text or markdown" \
	"runtime/appui/web typed control facts"

if [[ "${fail}" -ne 0 ]]; then
	exit 1
fi

echo "aiops harness contract boundary self-test passed"
