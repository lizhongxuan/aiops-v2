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
FUNCTION_HEADER_RES = (
    re.compile(r"(?:^|[}\n])\s*func\b[^{}]*$", re.DOTALL),
    re.compile(r"(?:^|[;}\n])\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\b[^{}]*$", re.DOTALL),
    re.compile(
        r"(?:^|[;}\n])\s*(?:export\s+)?(?:const|let|var)\s+"
        r"[A-Za-z_$][A-Za-z0-9_$]*\b[^{}]*=>\s*$",
        re.DOTALL,
    ),
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
class Violation:
    path: Path
    line: int
    source_path: str
    condition: str


def strip_comments(source: str) -> str:
    """Replace // and /* */ comments with spaces while preserving line offsets."""
    out = list(source)
    index = 0
    quote = ""
    escaped = False
    while index < len(source):
        char = source[index]
        next_char = source[index + 1] if index + 1 < len(source) else ""
        if quote:
            if escaped:
                escaped = False
            elif char == "\\" and quote != "`":
                escaped = True
            elif char == quote:
                quote = ""
            index += 1
            continue
        if char in {'"', "'", "`"}:
            quote = char
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
        index += 1
    return "".join(out)


def mask_strings(source: str) -> str:
    """Replace string contents with spaces while retaining shape and newlines."""
    out = list(source)
    index = 0
    quote = ""
    escaped = False
    while index < len(source):
        char = source[index]
        if quote:
            if char != "\n":
                out[index] = " "
            if escaped:
                escaped = False
            elif char == "\\" and quote != "`":
                escaped = True
            elif char == quote:
                quote = ""
            index += 1
            continue
        if char in {'"', "'", "`"}:
            quote = char
            out[index] = " "
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


def condition_atoms(condition: Condition) -> Iterator[Condition]:
    """Split boolean clauses so typed status facts do not taint display guards.

    A condition such as ``typedStatus == "failed" || hasFinalText`` is legal:
    the state comparison is typed and the final text is only a display-presence
    guard. The forbidden form has the tainted value and state literal in the
    same boolean atom, for example ``candidate.includes("failed")``.
    """
    masked = mask_strings(condition.text)
    cursor = 0
    for operator in re.finditer(r"&&|\|\|", masked):
        yield Condition(condition.offset + cursor, condition.text[cursor : operator.start()])
        cursor = operator.end()
    yield Condition(condition.offset + cursor, condition.text[cursor:])


def delimiter_balance(expression: str) -> int:
    masked = mask_strings(expression)
    return sum(masked.count(opening) - masked.count(closing) for opening, closing in (("(", ")"), ("[", "]"), ("{", "}")))


def assignment_records(source: str, scope: Scope) -> Iterator[tuple[str, Assignment]]:
    """Yield simple assignments, extending RHS over balanced continuation lines."""
    for match in ASSIGNMENT_START_RE.finditer(source, scope.start, scope.end):
        rhs_start = match.end()
        line_end = source.find("\n", rhs_start, scope.end)
        if line_end == -1:
            line_end = scope.end
        semicolon = source.find(";", rhs_start, line_end)
        rhs_end = semicolon if semicolon != -1 else line_end
        rhs = source[rhs_start:rhs_end]
        while delimiter_balance(rhs) > 0 and rhs_end < scope.end:
            next_end = source.find("\n", rhs_end + 1, scope.end)
            if next_end == -1:
                next_end = scope.end
            rhs_end = next_end
            rhs = source[rhs_start:rhs_end]
        yield match.group("lhs"), Assignment(match.start(), rhs.strip())


def assignments_for_scope(source: str, scope: Scope) -> dict[str, list[Assignment]]:
    assignments: dict[str, list[Assignment]] = {}
    for lhs, assignment in assignment_records(source, scope):
        assignments.setdefault(lhs, []).append(assignment)
    return assignments


def nearest_assignment(
    assignments: dict[str, list[Assignment]], name: str, before: int
) -> Assignment | None:
    candidates = assignments.get(name, ())
    for assignment in reversed(candidates):
        if assignment.offset < before:
            return assignment
    return None


def taint_path(
    expression: str,
    before: int,
    assignments: dict[str, list[Assignment]],
    seen: frozenset[tuple[str, int]] = frozenset(),
    depth: int = 0,
) -> list[str] | None:
    masked_expression = mask_strings(expression)
    direct = SOURCE_RE.search(masked_expression)
    if direct:
        return [direct.group(0)]
    if depth >= MAX_TAINT_DEPTH:
        return None

    for name in dict.fromkeys(IDENT_RE.findall(masked_expression)):
        assignment = nearest_assignment(assignments, name, before)
        if assignment is None:
            continue
        key = (name, assignment.offset)
        if key in seen:
            continue
        nested = taint_path(
            assignment.rhs,
            assignment.offset,
            assignments,
            seen | {key},
            depth + 1,
        )
        if nested:
            return [name, *nested]
    return None


def analyze_file(path: Path) -> list[Violation]:
    raw = path.read_text(encoding="utf-8", errors="replace")
    source = strip_comments(raw)
    masked = mask_strings(source)
    scopes = function_scopes(masked)
    assignment_cache: dict[Scope, dict[str, list[Assignment]]] = {}
    violations: list[Violation] = []

    conditions = list(extract_if_conditions(source, masked))
    conditions.extend(extract_ternary_conditions(source, masked))
    for condition in conditions:
        for atom in condition_atoms(condition):
            if not has_state_literal(atom.text):
                continue
            scope = scope_for_offset(scopes, atom.offset, len(source))
            assignments = assignment_cache.setdefault(
                scope, assignments_for_scope(source, scope)
            )
            path_to_source = taint_path(atom.text, atom.offset, assignments)
            if not path_to_source:
                continue
            line = source.count("\n", 0, atom.offset) + 1
            compact_condition = " ".join(atom.text.split())[:240]
            violations.append(
                Violation(path, line, " <- ".join(path_to_source), compact_condition)
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
