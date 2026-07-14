#!/usr/bin/env python3
"""Reject runtime control decisions derived from final assistant display text.

This is intentionally a small, dependency-free, bounded data-flow checker for
the Go and TypeScript/JavaScript production surfaces owned by the AIOps agent
harness. It is not a full parser. It strips comments, keeps analysis inside the
nearest function-like lexical scope, resolves the nearest preceding assignment,
and follows aliases/transforms up to MAX_TAINT_DEPTH hops.
"""

from __future__ import annotations

import argparse
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, Iterator


MAX_TAINT_DEPTH = 16
SCAN_PATHS = ("internal/runtimekernel", "internal/appui", "web/src")
SOURCE_RE = re.compile(
    r"\b(?:finalText|assistantText|markdown|FinalOutput|[A-Za-z_$][A-Za-z0-9_$]*FinalOutput)\b"
)
IDENT_RE = re.compile(r"\b[A-Za-z_$][A-Za-z0-9_$]*\b")
STATE_RE = re.compile(
    r"(?<![A-Za-z0-9])(?:approved|approval|blocked|completed|failed|verified|pending|denied|success|error|running)(?![A-Za-z0-9])",
    re.IGNORECASE,
)
ASSIGNMENT_START_RE = re.compile(
    r"(?m)^[ \t]*(?:(?:const|let|var)\s+)?"
    r"(?P<lhs>[A-Za-z_$][A-Za-z0-9_$]*)"
    r"(?:\s*\??:\s*[^=\n]+)?\s*"
    r"(?::=|=(?!=|>))\s*"
)
IF_RE = re.compile(r"\bif\b")
SWITCH_RE = re.compile(r"\bswitch\b")
CASE_RE = re.compile(r"\b(?:case|default)\b")
RETURN_RE = re.compile(r"\breturn\b")
SAFE_DISPLAY_RETURN_RE = re.compile(
    r"^(?:(?:renderMarkdown|escapeForDisplay|sanitizeForDisplay|normalizeForDisplay)\s*\("
    r"|(?:finalText|assistantText|markdown|FinalOutput|[A-Za-z_$][A-Za-z0-9_$]*FinalOutput)\b)"
)
FUNCTION_HEADER_RES = (
    re.compile(r"(?:^|[}\n])\s*func\b[^{}]*$", re.DOTALL),
    re.compile(r"(?:^|[;}\n])[^{}\n]*\bfunc\s*\([^{}]*$", re.DOTALL),
    re.compile(r"(?:^|[;}\n])\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\b[^{}]*$", re.DOTALL),
    re.compile(
        r"(?:^|[;}\n])\s*(?:export\s+)?(?:const|let|var)\s+"
        r"[A-Za-z_$][A-Za-z0-9_$]*\b[^{}]*=>\s*$",
        re.DOTALL,
    ),
    re.compile(r"(?:^|[;}\n])[^{}\n]*=>\s*$", re.DOTALL),
    re.compile(
        r"(?:^|[;}\n])\s*(?:(?:public|private|protected|static|async|readonly)\s+)*"
        r"(?!(?:if|for|while|switch|catch|with)\b)"
        r"[A-Za-z_$][A-Za-z0-9_$]*\s*\([^{};]*\)\s*(?::[^{}=]+)?$",
        re.DOTALL,
    ),
)
SKIP_DIRS = {
    ".git",
    "node_modules",
    "dist",
    "build",
    "coverage",
    "vendor",
    "testdata",
    "__tests__",
    "tests",
}
SUPPORTED_SUFFIXES = {".go", ".ts", ".tsx", ".js", ".jsx"}


@dataclass(frozen=True)
class Assignment:
    offset: int
    rhs: str


@dataclass(frozen=True)
class Scope:
    start: int
    end: int


@dataclass(frozen=True)
class Condition:
    offset: int
    text: str


@dataclass(frozen=True)
class SwitchDecision:
    offset: int
    expression: str
    case_label: str
    branch: str


@dataclass(frozen=True)
class Violation:
    path: Path
    line: int
    source_path: str
    condition: str


def strip_comments(source: str, template_interpolation: bool) -> str:
    """Replace // and /* */ comments with spaces while preserving line offsets."""
    out = list(source)
    index = 0
    stack: list[tuple[str, str | int]] = [("code", 0)]
    while index < len(source):
        char = source[index]
        next_char = source[index + 1] if index + 1 < len(source) else ""
        mode, value = stack[-1]
        if mode == "string":
            quote = str(value)
            if char == "\\" and quote != "`":
                index += 2
                continue
            if char == quote:
                stack.pop()
            index += 1
            continue
        if mode == "template":
            if char == "\\":
                index += 2
                continue
            if char == "`":
                stack.pop()
                index += 1
                continue
            if template_interpolation and char == "$" and next_char == "{":
                stack.append(("interpolation", 1))
                index += 2
                continue
            index += 1
            continue

        if char in {'"', "'"}:
            stack.append(("string", char))
            index += 1
            continue
        if char == "`":
            if template_interpolation:
                stack.append(("template", "`"))
            else:
                stack.append(("string", "`"))
            index += 1
            continue
        if char == "/" and next_char == "/":
            out[index] = out[index + 1] = " "
            index += 2
            while index < len(source) and source[index] != "\n":
                out[index] = " "
                index += 1
            continue
        if char == "/" and next_char == "*":
            out[index] = out[index + 1] = " "
            index += 2
            while index < len(source):
                if index + 1 < len(source) and source[index] == "*" and source[index + 1] == "/":
                    out[index] = out[index + 1] = " "
                    index += 2
                    break
                if source[index] != "\n":
                    out[index] = " "
                index += 1
            continue
        if mode == "interpolation":
            depth = int(value)
            if char == "{":
                stack[-1] = (mode, depth + 1)
            elif char == "}":
                if depth == 1:
                    stack.pop()
                else:
                    stack[-1] = (mode, depth - 1)
        index += 1
    return "".join(out)


def mask_strings(source: str, template_interpolation: bool) -> str:
    """Mask literals but preserve JavaScript template interpolation expressions."""
    out = list(source)
    index = 0
    stack: list[tuple[str, str | int]] = [("code", 0)]
    while index < len(source):
        char = source[index]
        next_char = source[index + 1] if index + 1 < len(source) else ""
        mode, value = stack[-1]
        if mode == "string":
            quote = str(value)
            if char != "\n":
                out[index] = " "
            if char == "\\" and quote != "`":
                if index + 1 < len(source) and source[index + 1] != "\n":
                    out[index + 1] = " "
                index += 2
                continue
            if char == quote:
                stack.pop()
            index += 1
            continue
        if mode == "template":
            if char != "\n":
                out[index] = " "
            if char == "\\":
                if index + 1 < len(source) and source[index + 1] != "\n":
                    out[index + 1] = " "
                index += 2
                continue
            if char == "`":
                stack.pop()
                index += 1
                continue
            if char == "$" and next_char == "{":
                out[index] = out[index + 1] = " "
                stack.append(("interpolation", 1))
                index += 2
                continue
            index += 1
            continue

        if char in {'"', "'"}:
            stack.append(("string", char))
            out[index] = " "
            index += 1
            continue
        if char == "`":
            if template_interpolation:
                stack.append(("template", "`"))
            else:
                stack.append(("string", "`"))
            out[index] = " "
            index += 1
            continue
        if mode == "interpolation":
            depth = int(value)
            if char == "{":
                stack[-1] = (mode, depth + 1)
            elif char == "}":
                out[index] = " "
                if depth == 1:
                    stack.pop()
                else:
                    stack[-1] = (mode, depth - 1)
        index += 1
    return "".join(out)


def string_literals(source: str) -> Iterator[str]:
    index = 0
    while index < len(source):
        quote = source[index]
        if quote not in {'"', "'", "`"}:
            index += 1
            continue
        index += 1
        body: list[str] = []
        escaped = False
        while index < len(source):
            char = source[index]
            if escaped:
                body.append(char)
                escaped = False
            elif char == "\\" and quote != "`":
                escaped = True
            elif char == quote:
                index += 1
                break
            else:
                body.append(char)
            index += 1
        yield "".join(body)


def has_state_literal(condition: str) -> bool:
    return any(STATE_RE.search(literal) for literal in string_literals(condition))


def matching_delimiter(masked: str, start: int, opening: str, closing: str) -> int | None:
    depth = 0
    for index in range(start, len(masked)):
        char = masked[index]
        if char == opening:
            depth += 1
        elif char == closing:
            depth -= 1
            if depth == 0:
                return index
    return None


def matching_opening_delimiter(
    masked: str, closing_index: int, opening: str, closing: str
) -> int | None:
    depth = 0
    for index in range(closing_index, -1, -1):
        char = masked[index]
        if char == closing:
            depth += 1
        elif char == opening:
            depth -= 1
            if depth == 0:
                return index
    return None


def function_scopes(masked: str) -> list[Scope]:
    stack: list[int] = []
    pairs: list[tuple[int, int]] = []
    for index, char in enumerate(masked):
        if char == "{":
            stack.append(index)
        elif char == "}" and stack:
            pairs.append((stack.pop(), index))

    scopes: list[Scope] = []
    for opening, closing in pairs:
        header = masked[max(0, opening - 800) : opening]
        if any(pattern.search(header) for pattern in FUNCTION_HEADER_RES):
            scopes.append(Scope(opening + 1, closing))
    scopes.sort(key=lambda scope: (scope.start, -scope.end))
    return scopes


def scope_for_offset(scopes: list[Scope], offset: int, source_len: int) -> Scope:
    containing = [scope for scope in scopes if scope.start <= offset <= scope.end]
    if not containing:
        return Scope(0, source_len)
    return min(containing, key=lambda scope: scope.end - scope.start)


def scope_chain_for_offset(scopes: list[Scope], offset: int, source_len: int) -> list[Scope]:
    containing = [scope for scope in scopes if scope.start <= offset <= scope.end]
    containing.sort(key=lambda scope: scope.end - scope.start)
    file_scope = Scope(0, source_len)
    if file_scope not in containing:
        containing.append(file_scope)
    return containing


def function_parameter_span(masked: str, scope: Scope) -> tuple[int, int] | None:
    """Return the source span inside the parameter list for a function scope."""
    opening_brace = scope.start - 1
    header_start = max(0, opening_brace - 2000)
    header = masked[header_start:opening_brace]

    arrow = header.rfind("=>")
    if arrow != -1 and not header[arrow + 2 :].strip():
        arrow_global = header_start + arrow
        closing = masked.rfind(")", header_start, arrow_global)
        if closing != -1:
            opening = matching_opening_delimiter(masked, closing, "(", ")")
            if opening is not None:
                return opening + 1, closing
        single = re.search(r"([A-Za-z_$][A-Za-z0-9_$]*)\s*$", header[:arrow])
        if single:
            return header_start + single.start(1), header_start + single.end(1)

    keywords = list(re.finditer(r"\b(?:func|function)\b", header))
    if keywords:
        keyword = keywords[-1]
        cursor = header_start + keyword.end()
        while cursor < opening_brace and masked[cursor].isspace():
            cursor += 1
        if cursor < opening_brace and masked[cursor] == "(":
            first_close = matching_delimiter(masked, cursor, "(", ")")
            if first_close is None or first_close >= opening_brace:
                return None
            after = first_close + 1
            while after < opening_brace and masked[after].isspace():
                after += 1
            receiver_name = re.match(r"[A-Za-z_$][A-Za-z0-9_$]*", masked[after:opening_brace])
            if receiver_name:
                after += receiver_name.end()
                while after < opening_brace and masked[after].isspace():
                    after += 1
                if after < opening_brace and masked[after] == "(":
                    params_close = matching_delimiter(masked, after, "(", ")")
                    if params_close is not None and params_close < opening_brace:
                        return after + 1, params_close
            return cursor + 1, first_close

        params_open = masked.find("(", cursor, opening_brace)
        if params_open != -1:
            params_close = matching_delimiter(masked, params_open, "(", ")")
            if params_close is not None and params_close < opening_brace:
                return params_open + 1, params_close

    closing = masked.rfind(")", header_start, opening_brace)
    if closing != -1:
        opening = matching_opening_delimiter(masked, closing, "(", ")")
        if opening is not None:
            return opening + 1, closing
    return None


def split_top_level(text: str, masked: str, separator: str) -> list[tuple[str, str]]:
    parts: list[tuple[str, str]] = []
    start = 0
    paren_depth = bracket_depth = brace_depth = 0
    for index, char in enumerate(masked):
        if char == "(":
            paren_depth += 1
        elif char == ")":
            paren_depth -= 1
        elif char == "[":
            bracket_depth += 1
        elif char == "]":
            bracket_depth -= 1
        elif char == "{":
            brace_depth += 1
        elif char == "}":
            brace_depth -= 1
        elif char == separator and paren_depth == bracket_depth == brace_depth == 0:
            parts.append((text[start:index], masked[start:index]))
            start = index + 1
    parts.append((text[start:], masked[start:]))
    return parts


def parameter_definitions(
    source: str,
    masked: str,
    scope: Scope,
) -> list[tuple[str, str]]:
    span = function_parameter_span(masked, scope)
    if span is None:
        return []
    start, end = span
    definitions: list[tuple[str, str]] = []
    for raw_param, masked_param in split_top_level(
        source[start:end], masked[start:end], ","
    ):
        raw_param = raw_param.strip()
        masked_param = masked_param.strip()
        if not raw_param or raw_param.startswith(("{", "[")):
            continue
        default_rhs = ""
        for index, char in enumerate(masked_param):
            if char == "=" and not masked_param[index : index + 2] in {"=>", "=="}:
                default_rhs = raw_param[index + 1 :].strip()
                raw_param = raw_param[:index].strip()
                break
        name_match = re.match(
            r"(?:(?:public|private|protected|readonly)\s+)*(?:\.\.\.)?"
            r"([A-Za-z_$][A-Za-z0-9_$]*)",
            raw_param,
        )
        if name_match:
            definitions.append((name_match.group(1), default_rhs))
    return definitions


def extract_if_conditions(source: str, masked: str) -> Iterator[Condition]:
    for match in IF_RE.finditer(masked):
        cursor = match.end()
        while cursor < len(masked) and masked[cursor].isspace():
            cursor += 1
        if cursor >= len(masked):
            continue
        if masked[cursor] == "(":
            end = matching_delimiter(masked, cursor, "(", ")")
            if end is not None:
                yield Condition(match.start(), source[cursor + 1 : end])
            continue

        # Go permits an unparenthesized if condition. Bound it by the opening
        # block brace and reject absurdly large candidates caused by bad syntax.
        end = masked.find("{", cursor, min(len(masked), cursor + 2000))
        if end != -1:
            yield Condition(match.start(), source[cursor:end])


def extract_ternary_conditions(source: str, masked: str) -> Iterator[Condition]:
    line_start = 0
    for line in masked.splitlines(keepends=True):
        for question in (match.start() for match in re.finditer(r"(?<![?.])\?(?![?.:])", line)):
            if ":" not in line[question + 1 :]:
                continue
            prefix = source[line_start : line_start + question]
            yield Condition(line_start + question, prefix)
        line_start += len(line)


def switch_branch_has_control_return(branch: str, template_interpolation: bool) -> bool:
    """Return whether a case branch returns non-display control data.

    A final-text switch that only forwards text into the approved display sinks
    is harmless. Any other return selects structured data from prose and is a
    control decision, including booleans, status strings, objects, and helper
    results.
    """
    masked = mask_strings(branch, template_interpolation)
    for match in RETURN_RE.finditer(masked):
        expression_start = match.end()
        expression_end = len(branch)
        for delimiter in (";", "\n", "}"):
            candidate = masked.find(delimiter, expression_start)
            if candidate != -1:
                expression_end = min(expression_end, candidate)
        expression = branch[expression_start:expression_end].strip()
        if not expression:
            continue
        if SAFE_DISPLAY_RETURN_RE.match(expression):
            continue
        return True
    return False


def extract_switch_decisions(
    source: str, masked: str, template_interpolation: bool
) -> Iterator[SwitchDecision]:
    """Yield state-literal switch cases controlled by final display text.

    Both ``switch (value)`` in TypeScript and Go's unparenthesized
    ``switch value`` form are supported. Case positions are found in masked
    source so strings/comments cannot forge structure, while labels and return
    expressions are read from the original stripped source.
    """
    for switch in SWITCH_RE.finditer(masked):
        cursor = switch.end()
        while cursor < len(masked) and masked[cursor].isspace():
            cursor += 1
        if cursor >= len(masked):
            continue

        if masked[cursor] == "(":
            expression_end = matching_delimiter(masked, cursor, "(", ")")
            if expression_end is None:
                continue
            expression_start = cursor + 1
            block_open = masked.find("{", expression_end + 1)
        else:
            block_open = masked.find("{", cursor, min(len(masked), cursor + 2000))
            expression_start = cursor
            expression_end = block_open
        if block_open == -1 or expression_end is None or expression_end < expression_start:
            continue
        block_close = matching_delimiter(masked, block_open, "{", "}")
        if block_close is None:
            continue

        expression = source[expression_start:expression_end].strip()
        block_masked = masked[block_open + 1 : block_close]
        case_matches = list(CASE_RE.finditer(block_masked))
        for index, case_match in enumerate(case_matches):
            if case_match.group(0) == "default":
                continue
            case_global = block_open + 1 + case_match.start()
            label_start = block_open + 1 + case_match.end()
            label_end = masked.find(":", label_start, block_close)
            if label_end == -1:
                continue
            case_label = source[label_start:label_end]
            if not has_state_literal(case_label):
                continue
            if index + 1 < len(case_matches):
                branch_end = block_open + 1 + case_matches[index + 1].start()
            else:
                branch_end = block_close
            branch = source[label_end + 1 : branch_end]
            if not switch_branch_has_control_return(branch, template_interpolation):
                continue
            yield SwitchDecision(case_global, expression, case_label, branch)


def condition_atoms(condition: Condition, template_interpolation: bool) -> Iterator[Condition]:
    """Split boolean clauses so typed status facts do not taint display guards.

    A condition such as ``typedStatus == "failed" || hasFinalText`` is legal:
    the state comparison is typed and the final text is only a display-presence
    guard. The forbidden form has the tainted value and state literal in the
    same boolean atom, for example ``candidate.includes("failed")``.
    """
    masked = mask_strings(condition.text, template_interpolation)
    cursor = 0
    for operator in re.finditer(r"&&|\|\|", masked):
        yield Condition(condition.offset + cursor, condition.text[cursor : operator.start()])
        cursor = operator.end()
    yield Condition(condition.offset + cursor, condition.text[cursor:])


def assignment_records(
    source: str, masked: str, scope: Scope
) -> Iterator[tuple[str, Assignment]]:
    """Yield assignments with a single bounded scan over multiline call RHS."""
    for match in ASSIGNMENT_START_RE.finditer(source, scope.start, scope.end):
        rhs_start = match.end()
        rhs_end = rhs_start
        paren_depth = 0
        bracket_depth = 0
        while rhs_end < scope.end:
            char = masked[rhs_end]
            if char == "(":
                paren_depth += 1
            elif char == ")":
                paren_depth -= 1
            elif char == "[":
                bracket_depth += 1
            elif char == "]":
                bracket_depth -= 1
            elif char == ";" and paren_depth <= 0 and bracket_depth <= 0:
                break
            elif char == "\n" and paren_depth <= 0 and bracket_depth <= 0:
                break
            rhs_end += 1
        yield match.group("lhs"), Assignment(
            match.start(), source[rhs_start:rhs_end].strip()
        )


def assignment_index(
    source: str,
    masked: str,
    scopes: list[Scope],
    source_len: int,
) -> dict[Scope, dict[str, list[Assignment]]]:
    """Index every assignment once under its innermost lexical function scope."""
    indexed: dict[Scope, dict[str, list[Assignment]]] = {}
    file_scope = Scope(0, source_len)
    for lhs, assignment in assignment_records(source, masked, file_scope):
        owner = scope_for_offset(scopes, assignment.offset, source_len)
        indexed.setdefault(owner, {}).setdefault(lhs, []).append(assignment)
    for scope in scopes:
        for name, default_rhs in parameter_definitions(source, masked, scope):
            indexed.setdefault(scope, {}).setdefault(name, []).append(
                Assignment(scope.start - 1, default_rhs)
            )
    for assignments in indexed.values():
        for definitions in assignments.values():
            definitions.sort(key=lambda definition: definition.offset)
    return indexed


def nearest_assignment(
    scopes: list[Scope],
    source_len: int,
    assignments_by_scope: dict[Scope, dict[str, list[Assignment]]],
    name: str,
    before: int,
) -> Assignment | None:
    for scope in scope_chain_for_offset(scopes, before, source_len):
        assignments = assignments_by_scope.get(scope, {})
        candidates = assignments.get(name, ())
        for assignment in reversed(candidates):
            if assignment.offset < before:
                return assignment
    return None


def taint_path(
    expression: str,
    before: int,
    scopes: list[Scope],
    source_len: int,
    assignments_by_scope: dict[Scope, dict[str, list[Assignment]]],
    template_interpolation: bool,
    seen: frozenset[tuple[str, int]] = frozenset(),
    depth: int = 0,
) -> list[str] | None:
    masked_expression = mask_strings(expression, template_interpolation)
    direct = SOURCE_RE.search(masked_expression)
    if direct:
        return [direct.group(0)]
    if depth >= MAX_TAINT_DEPTH:
        return None

    for name in dict.fromkeys(IDENT_RE.findall(masked_expression)):
        assignment = nearest_assignment(
            scopes,
            source_len,
            assignments_by_scope,
            name,
            before,
        )
        if assignment is None:
            continue
        key = (name, assignment.offset)
        if key in seen:
            continue
        nested = taint_path(
            assignment.rhs,
            assignment.offset,
            scopes,
            source_len,
            assignments_by_scope,
            template_interpolation,
            seen | {key},
            depth + 1,
        )
        if nested:
            return [name, *nested]
    return None


def analyze_file(path: Path) -> list[Violation]:
    raw = path.read_text(encoding="utf-8", errors="replace")
    template_interpolation = path.suffix.lower() != ".go"
    source = strip_comments(raw, template_interpolation)
    masked = mask_strings(source, template_interpolation)
    scopes = function_scopes(masked)
    assignments_by_scope = assignment_index(
        source, masked, scopes, len(source)
    )
    violations: list[Violation] = []

    conditions = list(extract_if_conditions(source, masked))
    conditions.extend(extract_ternary_conditions(source, masked))
    for condition in conditions:
        for atom in condition_atoms(condition, template_interpolation):
            if not has_state_literal(atom.text):
                continue
            path_to_source = taint_path(
                atom.text,
                atom.offset,
                scopes,
                len(source),
                assignments_by_scope,
                template_interpolation,
            )
            if not path_to_source:
                continue
            line = source.count("\n", 0, atom.offset) + 1
            compact_condition = " ".join(atom.text.split())[:240]
            violations.append(
                Violation(path, line, " <- ".join(path_to_source), compact_condition)
            )

    for decision in extract_switch_decisions(source, masked, template_interpolation):
        path_to_source = taint_path(
            decision.expression,
            decision.offset,
            scopes,
            len(source),
            assignments_by_scope,
            template_interpolation,
        )
        if not path_to_source:
            continue
        line = source.count("\n", 0, decision.offset) + 1
        compact_case = " ".join(decision.case_label.split())[:160]
        violations.append(
            Violation(
                path,
                line,
                " <- ".join(path_to_source),
                f"switch {decision.expression} case {compact_case}",
            )
        )
    return violations


def is_production_source(path: Path) -> bool:
    if path.suffix.lower() not in SUPPORTED_SUFFIXES:
        return False
    lowered_parts = {part.lower() for part in path.parts}
    if lowered_parts & SKIP_DIRS:
        return False
    name = path.name.lower()
    if name.endswith("_test.go") or ".test." in name or ".spec." in name:
        return False
    return True


def source_files(roots: Iterable[Path]) -> Iterator[Path]:
    seen: set[Path] = set()
    for root in roots:
        for relative in SCAN_PATHS:
            scan_root = root / relative
            if not scan_root.is_dir():
                continue
            for path in scan_root.rglob("*"):
                if not path.is_file() or not is_production_source(path):
                    continue
                resolved = path.resolve()
                if resolved in seen:
                    continue
                seen.add(resolved)
                yield path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("roots", nargs="+", type=Path, help="repository roots to scan")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    violations: list[Violation] = []
    try:
        for path in source_files(args.roots):
            violations.extend(analyze_file(path))
    except OSError as error:
        print(f"final-text control scan failed: {error}", file=sys.stderr)
        return 2

    for violation in violations:
        print(
            f"{violation.path}:{violation.line}: tainted final display text controls runtime state "
            f"({violation.source_path}): {violation.condition}",
            file=sys.stderr,
        )
    return 1 if violations else 0


if __name__ == "__main__":
    raise SystemExit(main())
