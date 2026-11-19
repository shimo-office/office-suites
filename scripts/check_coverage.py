#!/usr/bin/env python3
"""Require 80% statement coverage from a single Go coverprofile.

Run against the complete profile emitted by the workflow's go test command.
This validates its format and weights blocks by NumStmt; it does not establish
that an independently supplied profile covers every source file.
"""
import argparse
import json
from pathlib import Path
import re
import sys


MINIMUM_PERCENT = 80
RECORD = re.compile(r"(.+):(\d+)\.(\d+),(\d+)\.(\d+) (\d+) (\d+)")


def check_profile(path):
    lines = path.read_text(encoding="utf-8").splitlines()
    if not lines or lines[0] not in {"mode: set", "mode: count", "mode: atomic"}:
        raise ValueError("missing or invalid coverprofile mode")
    mode = lines[0].split()[1]
    covered, total = 0, 0
    seen = set()
    for number, line in enumerate(lines[1:], start=2):
        match = RECORD.fullmatch(line)
        if match is None:
            raise ValueError(f"line {number}: malformed coverage record")
        filename = match[1]
        start_line, start_col, end_line, end_col, statements, count = map(int, match.groups()[1:])
        start, end = (start_line, start_col), (end_line, end_col)
        if filename != filename.strip() or min(*start, *end) < 1 or end < start:
            raise ValueError(f"line {number}: invalid file or block positions")
        if mode == "set" and count not in (0, 1):
            raise ValueError(f"line {number}: set mode count must be 0 or 1")
        key = (filename, start, end)
        if key in seen:
            raise ValueError(f"line {number}: duplicate block; use a single Go-produced profile")
        seen.add(key)
        total += statements
        if count > 0:
            covered += statements
    if total == 0:
        raise ValueError("coverprofile contains no measurable statements")
    return {"profile": str(path), "mode": mode, "coveredStatements": covered,
            "totalStatements": total, "coveragePercent": covered * 100 / total,
            "minimumPercent": MINIMUM_PERCENT,
            "passed": covered * 100 >= total * MINIMUM_PERCENT}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("profile", type=Path)
    args = parser.parse_args()
    try:
        result = check_profile(args.profile)
    except (OSError, UnicodeError, ValueError) as error:
        print(json.dumps({"passed": False, "error": str(error)}), file=sys.stderr)
        return 2
    print(json.dumps(result))
    return 0 if result["passed"] else 1


if __name__ == "__main__":
    sys.exit(main())
