#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="${AIOPS_CHANGE_REPO_ROOT:-$(cd "${SCRIPT_DIR}/.." && pwd)}"
REPO_ROOT="$(cd "${REPO_ROOT}" && pwd)"
POLICY_PATH="scripts/check-aiops-change-budget.sh"
cd "${REPO_ROOT}"

# The gate audits every commit in AIOPS_HARNESS_BASE_REF..HEAD (or HEAD^..HEAD
# locally) and treats an uncommitted worktree as one additional change. Budget
# exceptions are intentionally commit-only and require these trailers:
#   AIOps-Change-Exception: mechanical|baseline
#   AIOps-Change-Reason: <why the exception is safe>
#   AIOps-Change-Review: <review record, issue, or design document>

fail=0
tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT
enforced_files="${tmp_dir}/enforced-files"
: >"${enforced_files}"

error() {
	local rule="$1"
	local owner="$2"
	shift 2
	echo "ERROR: ${rule}" >&2
	echo "owner: ${owner}" >&2
	if [[ "$#" -gt 0 ]]; then
		echo "$*" >&2
	fi
	fail=1
}

is_production_file() {
	local path="$1"
	case "${path}" in
		cmd/*|internal/*|pkg/*|web/src/*) ;;
		*) return 1 ;;
	esac
	case "${path}" in
		*_test.go|*.test.ts|*.test.tsx|*.test.js|*.test.jsx|*.spec.ts|*.spec.tsx|*.spec.js|*.spec.jsx|*/testdata/*|web/src/dist/*|web/dist/*)
			return 1
			;;
	esac
	return 0
}

is_runtime_production_file() {
	local path="$1"
	case "${path}" in
		internal/runtimekernel/*|internal/runtimecontract/*|internal/agentassembly/*|internal/promptcompiler/*|internal/promptinput/*|internal/modelrouter/*|internal/modeltrace/*)
			is_production_file "${path}"
			;;
		*) return 1 ;;
	esac
}

is_ui_production_file() {
	local path="$1"
	case "${path}" in
		web/src/*.ts|web/src/*.tsx|web/src/*.js|web/src/*.jsx|web/src/*.css|web/src/*.scss|web/src/*.sass|web/src/*.less|web/src/*.html|web/src/*.svg)
			is_production_file "${path}"
			;;
		*) return 1 ;;
	esac
}

is_baseline_file() {
	local path="$1"
	case "${path}" in
		*.snap|*.golden|*_golden.json|*_golden.yaml|*_golden.yml|*_golden.txt|*_snapshot.json|*_snapshot.yaml|*_snapshot.yml|*_snapshot.txt|*__snapshots__/*|*__screenshots__/*|*-snapshots/*|*/snapshots/*|*/golden/*|web/tests/*-snapshots/*|internal/eval/testdata/baseline/*|internal/runtimekernel/testdata/aichat_harness_golden/*|internal/server/testdata/assistant_transport_story/*.json)
			return 0
			;;
		*) return 1 ;;
	esac
}

commit_trailer() {
	local commit="$1"
	local key="$2"
	git log -1 --format='%(trailers:key='"${key}"',valueonly)' "${commit}" | sed -n '1p'
}

has_audited_exception() {
	local commit="$1"
	local expected_kind="$2"
	local kind
	local reason
	local review
	kind="$(commit_trailer "${commit}" "AIOps-Change-Exception")"
	reason="$(commit_trailer "${commit}" "AIOps-Change-Reason")"
	review="$(commit_trailer "${commit}" "AIOps-Change-Review")"
	if [[ "${expected_kind}" == "budget" ]]; then
		[[ "${kind}" == "mechanical" || "${kind}" == "baseline" ]] || return 1
	else
		[[ "${kind}" == "${expected_kind}" ]] || return 1
	fi
	[[ -n "${reason//[[:space:]]/}" && -n "${review//[[:space:]]/}" ]]
}

parent_for_commit() {
	local commit="$1"
	if git rev-parse "${commit}^" >/dev/null 2>&1; then
		git rev-parse "${commit}^"
	else
		git hash-object -t tree /dev/null
	fi
}

changed_files_between() {
	if [[ "$2" == "WORKTREE" ]]; then
		{
			git diff --no-renames --name-only "$1" --
			git ls-files --others --exclude-standard
		} | sort -u
		return
	fi
	git diff --no-renames --name-only "$1" "$2"
}

changed_numstat_between() {
	if [[ "$2" == "WORKTREE" ]]; then
		git diff --no-renames --numstat "$1" --
		return
	fi
	git diff --no-renames --numstat "$1" "$2"
}

added_lines_between() {
	if [[ "$2" == "WORKTREE" ]]; then
		if git ls-files --error-unmatch -- "$3" >/dev/null 2>&1; then
			git diff --no-ext-diff --no-renames --unified=0 "$1" -- "$3" \
				| sed -n '/^+++ /d; /^+/s/^+//p'
		elif [[ -f "$3" ]]; then
			sed -n 'p' "$3"
		fi
		return
	fi
	git diff --no-ext-diff --no-renames --unified=0 "$1" "$2" -- "$3" \
		| sed -n '/^+++ /d; /^+/s/^+//p'
}

file_at_revision_contains() {
	local revision="$1"
	local path="$2"
	local pattern="$3"
	if [[ "${revision}" == "WORKTREE" ]]; then
		[[ -f "${path}" ]] && rg -q "${pattern}" "${path}"
	else
		git show "${revision}:${path}" 2>/dev/null | rg -q "${pattern}"
	fi
}

registry_contains_key() {
	local revision="$1"
	local key="$2"
	local path
	for path in internal/runtimecontract/metadata_keys.go internal/runtimecontract/intent_frame.go; do
		if [[ "${revision}" == "WORKTREE" ]]; then
			if [[ -f "${path}" ]] && rg -Fq "\"${key}\"" "${path}"; then
				return 0
			fi
		elif git show "${revision}:${path}" 2>/dev/null | rg -Fq "\"${key}\""; then
			return 0
		fi
	done
	return 1
}

check_added_control_semantics() {
	local before="$1"
	local after="$2"
	local label="$3"
	local files_file="$4"
	local path
	local lines
	local key
	local semantic_lines
	while IFS= read -r path; do
		[[ -n "${path}" ]] || continue
		if ! is_production_file "${path}"; then
			continue
		fi
		lines="$(added_lines_between "${before}" "${after}" "${path}" || true)"
		case "${path}" in
			internal/runtimekernel/*|internal/runtimecontract/*|internal/agentassembly/*|internal/promptcompiler/*|internal/promptinput/*|internal/modelrouter/*|internal/modeltrace/*|internal/appui/*)
				semantic_lines="$(printf '%s\n' "${lines}" | rg -v 'aiops-harness:\s*machine-boundary\s+.+' || true)"
				if printf '%s\n' "${semantic_lines}" | rg -q -i 'strings\.(Contains|HasPrefix|HasSuffix|EqualFold)\([^\n]*((user|message|query|intent|operation|task|route|tool|evidence|final|answer)[A-Za-z0-9_]*\s*,|,\s*["'"'][^"'"']+["'"'])'; then
				error \
					"core business string semantics added (${label})" \
					"typed runtime contract or plugin/capability adapter" \
					"${path}: use typed intent/operation/evidence/tool metadata; fixed machine-boundary parsing requires an inline aiops-harness: machine-boundary reason"
				fi
				;;
		esac

		while IFS= read -r key; do
			[[ -n "${key}" ]] || continue
			if ! registry_contains_key "${after}" "${key}"; then
				error \
					"unregistered aiops.* control metadata added (${label})" \
					"internal/runtimecontract metadata registry" \
					"${path}: ${key}"
			fi
		done < <(
			printf '%s\n' "${lines}" \
				| rg -i '(metadata|Metadata)' \
				| rg -o 'aiops\.[A-Za-z0-9_.-]+' \
				| sort -u || true
		)
	done <"${files_file}"
}

check_change() {
	local before="$1"
	local after="$2"
	local label="$3"
	local commit="${4:-}"
	local bootstrap_exception="${5:-0}"
	local files_file="${tmp_dir}/files-$RANDOM"
	local prod_file_count=0
	local prod_churn=0
	local path
	local additions
	local deletions
	local baseline_changed=0

	changed_files_between "${before}" "${after}" | sort -u >"${files_file}"
	cat "${files_file}" >>"${enforced_files}"

	while IFS= read -r path; do
		[[ -n "${path}" ]] || continue
		if is_production_file "${path}"; then
			prod_file_count=$((prod_file_count + 1))
		fi
		if is_baseline_file "${path}"; then
			baseline_changed=1
		fi
	done <"${files_file}"

	while IFS=$'\t' read -r additions deletions path; do
		[[ -n "${path:-}" ]] || continue
		if is_production_file "${path}"; then
			if [[ "${additions}" == "-" || "${deletions}" == "-" ]]; then
				prod_churn=501
			else
				prod_churn=$((prod_churn + additions + deletions))
			fi
		fi
	done < <(changed_numstat_between "${before}" "${after}")
	if [[ "${after}" == "WORKTREE" ]]; then
		while IFS= read -r path; do
			[[ -n "${path}" && -f "${path}" ]] || continue
			if is_production_file "${path}"; then
				prod_churn=$((prod_churn + $(wc -l <"${path}")))
			fi
		done < <(git ls-files --others --exclude-standard)
	fi

	if [[ "${prod_file_count}" -gt 5 || "${prod_churn}" -gt 500 ]]; then
		if [[ "${bootstrap_exception}" != "1" ]] && { [[ -z "${commit}" ]] || ! has_audited_exception "${commit}" budget; }; then
			error \
				"change budget exceeded (${label})" \
				"commit author and harness reviewer" \
				"production_files=${prod_file_count} production_churn=${prod_churn}; limit=5 files/500 lines; use audited AIOps-Change-Exception, AIOps-Change-Reason, and AIOps-Change-Review trailers only for mechanical/baseline work"
		fi
	fi

	if [[ "${baseline_changed}" -eq 1 ]]; then
		if [[ "${bootstrap_exception}" != "1" ]] && { [[ -z "${commit}" ]] || ! has_audited_exception "${commit}" baseline; }; then
			error \
				"undeclared golden/snapshot drift (${label})" \
				"baseline reviewer" \
				"baseline files require AIOps-Change-Exception: baseline plus non-empty reason and review trailers"
		fi
	fi

	check_added_control_semantics "${before}" "${after}" "${label}" "${files_file}"
	rm -f "${files_file}"
}

resolve_base() {
	if [[ -n "${AIOPS_HARNESS_BASE_REF:-}" ]]; then
		if [[ "${AIOPS_HARNESS_BASE_REF}" =~ ^0+$ ]]; then
			AIOPS_HARNESS_BASE_REF=""
		else
			git rev-parse --verify "${AIOPS_HARNESS_BASE_REF}^{commit}"
			return
		fi
	fi
	if [[ -n "${GITHUB_BASE_REF:-}" ]] && git rev-parse --verify "origin/${GITHUB_BASE_REF}^{commit}" >/dev/null 2>&1; then
		git merge-base HEAD "origin/${GITHUB_BASE_REF}"
		return
	fi
	if git rev-parse HEAD^ >/dev/null 2>&1; then
		git rev-parse HEAD^
	else
		git hash-object -t tree /dev/null
	fi
}

base="$(resolve_base)"
if ! git rev-parse --verify "${base}^{commit}" >/dev/null 2>&1 && ! git cat-file -e "${base}^{tree}" >/dev/null 2>&1; then
	echo "ERROR: unable to resolve AIOps harness change base: ${base}" >&2
	exit 2
fi

pre_policy_count=0
last_pre_policy=""
while IFS= read -r commit; do
	[[ -n "${commit}" ]] || continue
	parent="$(parent_for_commit "${commit}")"
	bootstrap_exception=0
	if ! git cat-file -e "${parent}:${POLICY_PATH}" 2>/dev/null; then
		if ! git cat-file -e "${commit}:${POLICY_PATH}" 2>/dev/null; then
			pre_policy_count=$((pre_policy_count + 1))
			last_pre_policy="${commit}"
			continue
		fi
		if [[ "${AIOPS_HARNESS_REQUIRE_BASELINE_ACK:-0}" == "1" && -n "${AIOPS_CHANGE_BASELINE_ACK:-}" && -n "${AIOPS_CHANGE_BASELINE_REASON:-}" ]]; then
			bootstrap_exception=1
			echo "NOTICE: policy-introduction commit uses audited baseline acknowledgement: commit=${commit} ack=${AIOPS_CHANGE_BASELINE_ACK}" >&2
		fi
	fi
	check_change "${parent}" "${commit}" "commit ${commit}" "${commit}" "${bootstrap_exception}"
done < <(git rev-list --reverse "${base}..HEAD")

if [[ "${pre_policy_count}" -gt 0 ]]; then
	if [[ "${AIOPS_HARNESS_REQUIRE_BASELINE_ACK:-0}" == "1" ]]; then
		if [[ -z "${AIOPS_CHANGE_BASELINE_ACK:-}" || -z "${AIOPS_CHANGE_BASELINE_REASON:-}" ]]; then
			error \
				"pre-policy history requires explicit baseline acknowledgement" \
				"hardening CI workflow" \
				"count=${pre_policy_count} cutoff=${last_pre_policy}; set AIOPS_CHANGE_BASELINE_ACK and AIOPS_CHANGE_BASELINE_REASON"
		fi
	else
		echo "NOTICE: pre-policy commits treated as baseline: count=${pre_policy_count} cutoff=${last_pre_policy}" >&2
	fi
fi

if ! git diff --quiet HEAD -- || [[ -n "$(git ls-files --others --exclude-standard)" ]]; then
	check_change HEAD WORKTREE "uncommitted worktree" ""
fi

sort -u "${enforced_files}" -o "${enforced_files}"
runtime_changed=0
ui_changed=0
story_proof=0
screenshot_proof=0
while IFS= read -r path; do
	[[ -n "${path}" ]] || continue
	if is_runtime_production_file "${path}"; then
		runtime_changed=1
	fi
	if is_ui_production_file "${path}"; then
		ui_changed=1
	fi
	case "${path}" in
		internal/server/*story*_test.go|internal/runtimekernel/*_test.go|internal/runtimekernel/*/*_test.go|internal/eval/*_test.go)
			if file_at_revision_contains WORKTREE "${path}" '(RunTurn\(|AssistantTransport)'; then
				story_proof=1
			fi
			;;
		web/tests/*.spec.js|web/tests/*.spec.ts|web/tests/*/*.spec.js|web/tests/*/*.spec.ts)
			if file_at_revision_contains WORKTREE "${path}" 'toHaveScreenshot\('; then
				screenshot_proof=1
			fi
			;;
	esac
done <"${enforced_files}"

if [[ "${runtime_changed}" -eq 1 && "${story_proof}" -ne 1 ]]; then
	error \
		"runtime production change lacks story/integration proof" \
		"runtime owner" \
		"change a RunTurn/AssistantTransport harness or story test in the same review range"
fi
if [[ "${ui_changed}" -eq 1 && "${screenshot_proof}" -ne 1 ]]; then
	error \
		"UI production change lacks toHaveScreenshot coverage" \
		"web UI owner" \
		"change a Playwright web/tests spec containing toHaveScreenshot(...) in the same review range"
fi

if [[ "${fail}" -ne 0 ]]; then
	exit 1
fi

echo "aiops harness change budget gate passed"
