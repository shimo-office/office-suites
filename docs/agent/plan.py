#!/usr/bin/env python3
"""Turn a local CSV into a validated Shimo sheet-update payload, entirely offline.

No network, no credentials, no CLI required: this reads the CSV, checks it
against the requested range, and writes ``payload.json`` plus a ``run.sh`` that
performs the actual write once a link and an account exist. It never writes to
Shimo itself, so the result is always "prepared, not written".
"""
import argparse
import csv
import json
import os
from pathlib import Path
import re
import sys

RANGE = re.compile(r'^([A-Z]+)([1-9][0-9]*)(?::([A-Z]+)([1-9][0-9]*))?$')
MAX_CELL = 5000


class PlanError(Exception):
    """Invalid input: report it instead of guessing what the user meant."""


def column_number(letters):
    number = 0
    for letter in letters:
        number = number * 26 + (ord(letter) - 64)
    return number


def column_letters(number):
    letters = ''
    while number > 0:
        number, remainder = divmod(number - 1, 26)
        letters = chr(65 + remainder) + letters
    return letters


def parse_range(raw, sheet):
    """Return (sheet, start row, start column, rows, columns or None)."""
    text = raw.strip()
    if '!' in text:
        prefix, text = text.split('!', 1)
        if sheet and prefix.strip() and prefix.strip() != sheet:
            raise PlanError(f'range 里的工作表名 "{prefix}" 与 --sheet "{sheet}" 不一致；只保留一个')
        sheet = prefix.strip() or sheet
    if not sheet:
        raise PlanError('缺少工作表名；用 --sheet 工作表1 或在 --range 里写 工作表1!A1')
    if '!' in sheet:
        raise PlanError('工作表名里不能有 "!"')
    match = RANGE.fullmatch(text.upper())
    if not match:
        raise PlanError(f'range "{raw}" 不合法；只接受 A1、A1:C3、工作表1!A1:C3 这类写法')
    start_col, start_row, end_col, end_row = match.groups()
    start_column, start_row_number = column_number(start_col), int(start_row)
    if end_col is None:
        return sheet, start_row_number, start_column, None, None
    end_column, end_row_number = column_number(end_col), int(end_row)
    rows, columns = end_row_number - start_row_number + 1, end_column - start_column + 1
    if rows < 1 or columns < 1:
        raise PlanError('range 的结束单元格在开始单元格之前；请按 A1:C3 的顺序写')
    return sheet, start_row_number, start_column, rows, columns


def read_rows(path, delimiter):
    try:
        text = path.read_text(encoding='utf-8-sig')
    except UnicodeDecodeError:
        try:
            text = path.read_text(encoding='gb18030')
        except UnicodeDecodeError as error:
            raise PlanError(f'无法解码 {path}：{error}；请另存为 UTF-8') from error
    rows = [row for row in csv.reader(text.splitlines(), delimiter=delimiter)]
    if not rows:
        raise PlanError(f'{path} 是空的，没有可写的数据')
    return [[cell.strip() for cell in row] for row in rows]


def trim(rows):
    """Drop trailing blank rows and columns, reporting how much was dropped."""
    warnings = []
    dropped_rows = 0
    while rows and not any(cell for cell in rows[-1]):
        rows.pop()
        dropped_rows += 1
    if dropped_rows:
        warnings.append(f'末尾 {dropped_rows} 行全空，已裁掉')
    if not rows:
        raise PlanError('整张表都是空值，没有可写的数据')
    width = max(len(row) for row in rows)
    dropped = 0
    while width and all(len(row) < width or not row[width - 1] for row in rows):
        width -= 1
        dropped += 1
    if dropped:
        warnings.append(f'末尾 {dropped} 列全空，已裁掉')
    rows = [row + [''] * (width - len(row)) for row in rows]
    return rows, warnings


def shape(rows, wanted_rows, wanted_columns):
    actual_rows, actual_columns = len(rows), len(rows[0])
    if wanted_rows is None:
        return actual_rows, actual_columns
    if (wanted_rows, wanted_columns) != (actual_rows, actual_columns):
        raise PlanError(
            f'range 是 {wanted_rows} 行 × {wanted_columns} 列，数据是 '
            f'{actual_rows} 行 × {actual_columns} 列；请改 range，或先核对 CSV')
    return actual_rows, actual_columns


def check_cells(rows):
    warnings = []
    longest = max((len(cell) for row in rows for cell in row), default=0)
    if longest > MAX_CELL:
        warnings.append(f'有单元格超过 {MAX_CELL} 字符（最长 {longest}），服务端可能截断')
    return warnings


def write_outputs(out_dir, payload, rows, columns):
    out_dir.mkdir(parents=True, exist_ok=True)
    payload_file = out_dir / 'payload.json'
    payload_file.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + '\n',
                            encoding='utf-8')
    script = out_dir / 'run.sh'
    script.write_text(
        '#!/bin/sh\n'
        '# 由 scripts/plan.py 生成：本地只做了校验，没有写入石墨。\n'
        '# 用法：sh run.sh <石墨表格链接>\n'
        'set -eu\n'
        'target="${1:?用法: sh run.sh <石墨表格链接>}"\n'
        'here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)\n'
        'exec shimo-net editor "$target" --action sheet-update \\\n'
        '  --payload "$(cat "$here/payload.json")" --json\n',
        encoding='utf-8')
    os.chmod(script, 0o755)
    return payload_file, script


def build(csv_path, sheet, raw_range, out_dir, delimiter=','):
    sheet, start_row, start_column, wanted_rows, wanted_columns = parse_range(raw_range, sheet)
    rows, warnings = trim(read_rows(csv_path, delimiter))
    rows_count, columns_count = shape(rows, wanted_rows, wanted_columns)
    warnings += check_cells(rows)
    end = f'{column_letters(start_column + columns_count - 1)}{start_row + rows_count - 1}'
    normalized = f'{sheet}!{column_letters(start_column)}{start_row}:{end}'
    payload = {'sheetName': sheet, 'range': normalized,
               'resource': {'values': rows}}
    payload_file, script = write_outputs(out_dir, payload, rows_count, columns_count)
    return {'ok': True, 'written': False, 'sheetName': sheet, 'range': normalized,
            'rows': rows_count, 'columns': columns_count,
            'files': {'payload': str(payload_file.resolve()), 'script': str(script.resolve())},
            'warnings': warnings,
            'next': f'sh {script.resolve()} <石墨表格链接>'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--csv', required=True, type=Path, help='本地 CSV（UTF-8 或 GB18030）')
    parser.add_argument('--sheet', default='', help='工作表名，例如 工作表1')
    parser.add_argument('--range', default='A1', help='起始单元格或区域，例如 A1 或 A1:C3')
    parser.add_argument('--out', type=Path, default=Path('output'), help='输出目录')
    parser.add_argument('--delimiter', default=',', help='CSV 分隔符，默认逗号')
    args = parser.parse_args()
    try:
        result = build(args.csv, args.sheet, args.range, args.out, args.delimiter)
    except PlanError as error:
        print(json.dumps({'ok': False, 'written': False, 'error': str(error)},
                         ensure_ascii=False))
        return 2
    print(json.dumps(result, ensure_ascii=False, indent=2))
    return 0


if __name__ == '__main__':
    sys.exit(main())
