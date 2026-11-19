#!/usr/bin/env python3
"""Build an evidence-backed phase-three report; never infer test success."""
from __future__ import annotations

import argparse
import copy
import hashlib
import json
import re
import shlex
import sys
from collections import Counter
from datetime import datetime
from pathlib import Path
from xml.sax.saxutils import escape
from zoneinfo import ZoneInfo

from reportlab.lib import colors
from reportlab.lib.enums import TA_LEFT
from reportlab.lib.pagesizes import A4
from reportlab.lib.styles import ParagraphStyle
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.cidfonts import UnicodeCIDFont
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.platypus import (
    CondPageBreak, Image, KeepTogether, PageBreak, Paragraph, SimpleDocTemplate, Spacer, Table, TableStyle,
)

ROOT = Path(__file__).resolve().parents[2]
LABELS = {"pending": "待测", "passed": "通过", "failed": "失败", "blocked": "阻塞", "deferred": "保留"}
INK = colors.HexColor("#19232D")
MUTED = colors.HexColor("#647381")
RULE = colors.HexColor("#D7DFE5")
STATE_COLORS = {
    "pending": "#687482", "passed": "#167252", "failed": "#B63A32", "blocked": "#A16419", "deferred": "#795692",
}
ALLOWED_OVERRIDES = {
    "status", "actualCommand", "observedResult", "executedAt", "exitCode", "reason",
    "targetGuid", "cliVersion", "evidence", "assertions", "logText", "deferral",
}
MAX_LOG_BYTES = 256 * 1024
# The source requirements enumerate these 28 rows, despite the title saying 29.
REQUIRED_SCENES = {
    number: [f"{number}.{index}" for index in range(1, count + 1)]
    for number, count in zip(range(29, 43), (3, 2, 2, 2, 2, 2, 3, 3, 2, 1, 2, 1, 2, 1))
}
REQUIRED_GLOBALS = [f"G{index}" for index in range(1, 6)]


def complete_scope(data):
    scenes = data.get("scenes", [])
    return (
        [scene.get("number") for scene in scenes] == list(REQUIRED_SCENES)
        and all([case.get("id") for case in scene.get("capabilities", [])]
                == REQUIRED_SCENES[scene["number"]] for scene in scenes)
        and Counter(case.get("id") for case in data.get("globalNegativeCases", []))
        == Counter(REQUIRED_GLOBALS)
    )


def read_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def register_font():
    for candidate in (Path("/System/Library/Fonts/STHeiti Light.ttc"),):
        if candidate.is_file():
            try:
                pdfmetrics.registerFont(TTFont("ReportCJK", str(candidate), subfontIndex=0))
                pdfmetrics.registerFontFamily("ReportCJK", normal="ReportCJK", bold="ReportCJK")
                return "ReportCJK"
            except Exception:
                pass
    pdfmetrics.registerFont(UnicodeCIDFont("STSong-Light"))
    pdfmetrics.registerFontFamily("STSong-Light", normal="STSong-Light", bold="STSong-Light")
    return "STSong-Light"


def redact(text: str) -> str:
    """Keep raw evidence on disk; remove credential values from report text."""
    text = re.sub(r'(?im)("?(?:cookie|authorization|sid|access_token|refresh_token)"?\s*[:=]\s*)("[^"\n]*"|[^\n,;}]+)', r'\1[已脱敏]', text)
    return re.sub(r'(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+', 'Bearer [已脱敏]', text)


def display(value) -> str:
    if isinstance(value, list):
        return shlex.join(str(part) for part in value)
    if isinstance(value, dict):
        return json.dumps(value, ensure_ascii=False, indent=2)
    return "" if value is None else str(value)


def evidence_records(case, base: Path):
    records = []
    for raw in case.get("evidence", []):
        if isinstance(raw, str):
            record = {"name": Path(raw).name, "path": raw, "kind": "artifact"}
        elif isinstance(raw, dict):
            record = dict(raw)
        else:
            raise ValueError(f"{case['id']}: evidence 必须是路径或对象")
        if not record.get("path") or not record.get("name"):
            raise ValueError(f"{case['id']}: evidence 缺少 name/path")
        target = Path(record["path"]).expanduser()
        if not target.is_absolute():
            target = base / target
        target = target.resolve()
        record["path"] = str(target)
        record["exists"] = target.is_file() and target.stat().st_size > 0
        record["kind"] = record.get("kind", "artifact")
        if record["exists"]:
            record["sha256"] = hashlib.sha256(target.read_bytes()).hexdigest()
            record["bytes"] = target.stat().st_size
        records.append(record)
    return records


def valid_deferral(case):
    decision = case.get("deferral", {})
    return (isinstance(decision, dict) and decision.get("requestedBy") == "user"
            and decision.get("releaseBlocking") is False
            and all(isinstance(decision.get(key), str) and decision[key].strip()
                    for key in ("requestedAt", "quote", "scope"))
            and bool(case.get("reason")))


def check_claim(case, *, negative=False):
    if case.get("status", "pending") not in LABELS:
        raise ValueError(f"{case['id']}: 未知状态 {case['status']}")
    if case.get("status") == "deferred":
        if negative or not valid_deferral(case):
            raise ValueError(f"{case['id']}: 保留项须有用户明确范围决定；全局安全门槛不能保留跳过")
        return
    if case.get("status") != "passed":
        return
    missing = []
    for key in ("actualCommand", "observedResult", "executedAt", "targetGuid", "cliVersion"):
        if not case.get(key):
            missing.append(key)
    code = case.get("exitCode")
    if negative:
        if type(code) is not int or code == 0:
            missing.append("exitCode须为真实非0整数")
        if not any(item["exists"] for item in case["evidence"]):
            missing.append("至少一份存在且非空的失败物证")
    elif code != 0 or isinstance(code, bool):
        missing.append("exitCode=0")
    present = {item["name"] for item in case["evidence"] if item["exists"]}
    missing.extend(name for name in case.get("evidenceRequired", []) if name not in present)
    assertions = case.get("assertions", [])
    expected = case.get("expected", [])
    if negative:
        expected = [expected] if isinstance(expected, str) and expected.strip() else []
        if not expected:
            missing.append("全局反例expected须为非空需求")
    if len(assertions) != len(expected):
        missing.append("assertions须逐条对应expected")
    elif any(item.get("passed") is not True or not item.get("description") for item in assertions):
        missing.append("全部assertions.passed=true且有description")
    if missing:
        raise ValueError(f"{case['id']}: 拒绝把证据不全项目标为通过：{'、'.join(missing)}")


def prepare(cases_path: Path, manifest_path: Path | None):
    data = copy.deepcopy(read_json(cases_path))
    if [scene["number"] for scene in data["scenes"]] != list(range(29, 43)):
        raise ValueError("用例必须保持场景29-42原顺序")
    updates = read_json(manifest_path).get("cases", {}) if manifest_path else {}
    functional_cases = [item for scene in data["scenes"] for item in scene["capabilities"]]
    global_cases = data.get("globalNegativeCases", [])
    known = {item["id"] for item in functional_cases + global_cases}
    if set(updates) - known:
        raise ValueError(f"证据清单含未知功能项：{sorted(set(updates) - known)}")
    for cases, negative in ((functional_cases, False), (global_cases, True)):
        for case in cases:
            override = updates.get(case["id"], {})
            invalid = set(override) - ALLOWED_OVERRIDES
            if invalid:
                raise ValueError(f"{case['id']}: 证据清单不能改写需求字段：{sorted(invalid)}")
            case.update(override)
            base = manifest_path.parent if "evidence" in override else cases_path.parent
            case["evidence"] = evidence_records(case, base)
            check_claim(case, negative=negative)
    return data


class Report:
    def __init__(self, data, output: Path, built_at: str, full_evidence=False):
        self.data, self.output, self.built_at = data, output, built_at
        self.full_evidence = full_evidence
        self.font = register_font()
        self.width = A4[0] - 72
        self.left_width = 88
        self.right_width = self.width - self.left_width
        self.styles = {}
        for name, size, leading, color in (
            ("body", 9.1, 14, INK), ("small", 7.6, 11.3, MUTED),
            ("title", 21, 29, INK), ("heading", 12.5, 19, INK),
            ("quote", 10.4, 16, INK), ("log", 8, 12.2, colors.HexColor("#E4ECEF")),
        ):
            self.styles[name] = ParagraphStyle(name, fontName=self.font, fontSize=size,
                leading=leading, textColor=color, wordWrap="CJK", splitLongWords=True,
                alignment=TA_LEFT, spaceAfter=0, allowWidows=0, allowOrphans=0)
        self.story = []
        self.counts = Counter(c.get("status", "pending") for s in data["scenes"] for c in s["capabilities"])
        global_cases = data.get("globalNegativeCases", [])
        self.complete = (
            complete_scope(data)
            and self.counts["passed"] == sum(self.counts.values())
            and bool(global_cases)
            and all(case.get("status") == "passed" for case in global_cases)
        )
        self.release_scope_complete = (
            complete_scope(data) and bool(global_cases)
            and all(case.get("status") == "passed" for case in global_cases)
            and all(case.get("status") == "passed" or
                    (case.get("status") == "deferred" and valid_deferral(case))
                    for scene in data["scenes"] for case in scene["capabilities"])
        )

    def p(self, text, style="body", markup=False):
        content = str(text) if markup else escape(redact(display(text))).replace("\n", "<br/>")
        return Paragraph(content, self.styles[style])

    def row(self, left, right, panel=False):
        if not isinstance(right, list):
            right = [right]
        table = Table([[left or "", right]], colWidths=[self.left_width, self.right_width], hAlign="LEFT")
        commands = [("VALIGN", (0, 0), (-1, -1), "TOP"),
                    ("LEFTPADDING", (0, 0), (0, 0), 0),
                    ("RIGHTPADDING", (0, 0), (0, 0), 10),
                    ("LEFTPADDING", (1, 0), (1, 0), 10),
                    ("RIGHTPADDING", (1, 0), (1, 0), 9),
                    ("TOPPADDING", (0, 0), (-1, -1), 2),
                    ("BOTTOMPADDING", (0, 0), (-1, -1), 3)]
        if panel:
            commands.extend([("BACKGROUND", (1, 0), (1, 0), colors.HexColor("#20272E")),
                             ("BOX", (1, 0), (1, 0), .4, colors.HexColor("#34434F"))])
        table.setStyle(TableStyle(commands))
        self.story.append(table)

    def log(self, text, label="命令输出（真实日志文本，非截图）"):
        self.row("", self.p(label, "small"))
        # Each original line remains visible. Small chunks allow natural page
        # breaks without clipping long logs inside a single unsplittable table.
        for line in redact(text).splitlines() or ["（空输出）"]:
            for start in range(0, max(1, len(line)), 320):
                self.row("", self.p(line[start:start + 320] or " ", "log"), panel=True)

    def evidence(self, record, show_picture=True):
        block_start = len(self.story)
        target = Path(record["path"])
        label = record.get("label", record["name"])
        if not record["exists"]:
            self.row("", self.p(f"证据缺失：{label}\n{target}", "small"))
            return
        try:
            shown_path = target.relative_to(ROOT)
        except ValueError:
            shown_path = target
        digest = record['sha256'] if self.full_evidence else record['sha256'][:12]
        digest_label = "SHA-256" if self.full_evidence else "SHA-256 前12位"
        self.row("", self.p(f"证据：{label}\n{shown_path}\n{digest_label} {digest}", "small"))
        if record["kind"] == "screenshot" and show_picture:
            picture = Image(str(target))
            available = self.right_width - 20
            scale = min(available / picture.imageWidth, 220 / picture.imageHeight, 1)
            picture.drawWidth, picture.drawHeight = picture.imageWidth * scale, picture.imageHeight * scale
            self.row("", picture)
            self.row("", self.p("真实页面截图；图片内容仍须人工核对目标与断言。", "small"))
            self.story[block_start:] = [KeepTogether(self.story[block_start:])]
        elif record["kind"] == "log" and self.full_evidence:
            if target.stat().st_size > MAX_LOG_BYTES:
                raise ValueError(f"日志超过256KiB，拒绝静默截断：{target}；请提供按测试项分割的日志。")
            self.log(target.read_text(encoding="utf-8"))

    def evidence_group(self, records, case_id):
        screenshots = [r for r in records if r.get("kind") == "screenshot" and r.get("exists")]
        current_images = [r for r in screenshots if not any(term in (r["name"] + " " + r["path"]).lower()
                          for term in ("crop", "historical", "before", "failure"))] or screenshots
        chosen = next((r for name in ("full-page-render", "reload-full", "slideshow-full", "reload.png", "after.png", "results.png", "slideshow.png")
                       for r in current_images if r["name"] == name), current_images[-1] if current_images else None)
        if self.full_evidence:
            for record in records:
                self.evidence(record)
            return
        if records:
            counts = Counter(r.get("kind", "artifact") for r in records)
            self.row("", self.p(f"物证清单：{len(records)} 项，含 {counts['screenshot']} 张截图、{counts['log']} 份日志。完整路径与 SHA-256：{self.output.with_suffix('.summary.json').name}，项目 {case_id}。", "small"))
        if chosen:
            self.evidence(chosen)
        for record in records:
            if not record.get("exists"):
                self.evidence(record, show_picture=False)

    def cover(self):
        status = "完成" if self.complete else ("本次范围完成（含保留项）" if self.release_scope_complete else "未完成")
        self.story.extend([self.p(f"石墨三期 · 阶段复测记录", "title"), Spacer(1, 9),
                           self.p(f"{status}  |  14 个场景 · {sum(self.counts.values())} 个列明功能项", "heading"), Spacer(1, 13)])
        stat = "   ".join(f"{LABELS[k]} {self.counts[k]}" for k in LABELS)
        self.story.extend([self.p(stat, "heading"), Spacer(1, 12),
                           self.p(f"生成时间：{self.built_at}（Asia/Shanghai）", "small"), Spacer(1, 14)])
        for scene in self.data["scenes"]:
            for case in scene["capabilities"]:
                if case.get("status") == "deferred":
                    decision = case["deferral"]
                    self.story.extend([self.p(f"用户范围决定（{decision['requestedAt']}）：{case['id']} {decision['scope']}保留，未实现/未验收部分不计通过，不阻塞本次发布。用户原话：{decision['quote']}"), Spacer(1, 11)])
        if self.counts["deferred"]:
            self.story.extend([self.p("原需求完整性与本次发布范围分开：本次范围验收" + ("已完成。" if self.release_scope_complete else "尚未完成，仍有其他门槛。")), Spacer(1, 11)])
        for text in (
            "本记录保持原需求编号 29-42。每个功能项右侧列出自然语言话术、实际命令、执行结果和证据；未提供的材料会明确标注。",
            self.data["source"]["countDiscrepancy"],
            "通过数只由用例状态及完整证据清单产生。生成器检查材料是否存在和断言是否逐条回填，不能替代测试执行或判定截图真实性。",
            "本地示例、Mock 测试、结构检查或演示视频不计入线上功能通过数。一个功能行中的全部子动作均须测试。",
            "正文采用场景卡与代表截图；实际命令按原记录展示。完整日志、其他截图、导出文件路径及完整哈希保存在同名 summary.json 的证据索引中。" if not self.full_evidence else "本报告展开全部截图与原始日志；完整证据索引另见同名 summary.json。",
        ):
            self.story.extend([self.p(text), Spacer(1, 11)])
        self.story.extend([Spacer(1, 12), self.p("状态说明", "heading"), Spacer(1, 5)])
        for key in LABELS:
            definition = self.data["statusDefinitions"].get(key, "用户明确保留的后续范围；不计通过，不作为本次发布阻塞。")
            self.story.extend([self.p(f"{LABELS[key]}：{definition}"), Spacer(1, 6)])
        self.story.extend([Spacer(1, 10), self.p("原需求与追溯", "heading"), Spacer(1, 5),
                           self.p(self.data["source"]["title"]),
                           self.p(self.data["source"]["url"], "small"),
                           self.p("需求文件 SHA-256：" + self.data["source"].get("sha256", "未记录"), "small")])

    def scene(self, scene):
        self.story.append(PageBreak())
        self.story.extend([self.p(f"{scene['number']}  {scene['title']}", "heading"),
                           self.p(f"批次 {scene['batch']} · {len(scene['capabilities'])} 个功能项 · 原编号不变", "small"), Spacer(1, 10)])
        for index, case in enumerate(scene["capabilities"]):
            if index:
                self.story.append(CondPageBreak(180))
                self.story.append(Spacer(1, 13))
            state = case.get("status", "pending")
            left = self.p(f"{case['id']}<br/><font color='{STATE_COLORS[state]}'>{LABELS[state]}</font>", "heading", markup=True)
            self.row(left, [self.p(case["requirement"], "small"), Spacer(1, 3), self.p("「" + case["naturalLanguage"] + "」", "quote")])
            command = display(case.get("actualCommand"))
            self.row("", self.p("实际命令（[NL] 表示原换行，原始 argv 见日志）" if "\n" in command else "实际命令", "small"))
            displayed_command = (command or "未提供可执行命令").replace("\n", " [NL] ")
            for start in range(0, len(displayed_command), 320):
                self.row("", self.p(displayed_command[start:start + 320], "log"), panel=True)
            for label, key in (("实际结果", "observedResult"), ("原因 / 备注", "reason")):
                text = display(case.get(key))
                if key == "observedResult" and not text:
                    text = "未提供本次实际执行结果。"
                if text:
                    self.row("", self.p(f"{label}：{text}"))
            metadata = [f"{label}：{case[key]}" for label, key in (
                ("执行时间", "executedAt"), ("退出码", "exitCode"), ("目标 GUID", "targetGuid"), ("CLI 构建", "cliVersion"))
                if case.get(key) is not None]
            if metadata:
                self.row("", self.p("\n".join(metadata), "small"))
            self.row("", self.p("必须满足的结果", "small"))
            for n, expected in enumerate(case["expected"], 1):
                self.row("", self.p(f"{n}. {expected}"))
            if case.get("logText"):
                self.log(display(case["logText"]))
            self.evidence_group(case["evidence"], case["id"])
            if not case["evidence"]:
                self.row("", self.p("未提供实测截图或文件证据。", "small"))

    def appendix(self):
        self.story.append(PageBreak())
        self.story.extend([self.p("共同验收门槛", "heading"), Spacer(1, 8),
                           self.p("以下反例与保护条件单独记录，不作为第 29 个功能项凑数。"), Spacer(1, 12)])
        for case in self.data.get("globalNegativeCases", []):
            self.row(self.p(case["id"], "heading"), [self.p(case["naturalLanguage"], "quote"), Spacer(1, 5),
                self.p(case["expected"]), Spacer(1, 5), self.p("状态：" + LABELS.get(case.get("status", "pending"), "待测"), "small")])
            if case.get("reason"):
                self.row("", self.p(case["reason"], "small"))
            if case.get("actualCommand"):
                self.row("", self.p("实际命令：" + display(case["actualCommand"]), "small"))
                self.row("", self.p(f"运行时间：{case.get('executedAt', '')}；退出码：{case.get('exitCode', '')}；目标：{case.get('targetGuid', '')}；构建：{case.get('cliVersion', '')}", "small"))
            if case.get("observedResult"):
                self.row("", self.p(case["observedResult"]))
            for assertion in case.get("assertions", []):
                self.row("", self.p(("通过：" if assertion.get("passed") is True else "未通过：") + assertion.get("description", ""), "small"))
            self.evidence_group(case.get("evidence", []), case["id"])
        self.story.extend([Spacer(1, 16), self.p("复核提醒", "heading"), Spacer(1, 6)])
        for n, text in enumerate(self.data.get("globalRequirements", []), 1):
            self.story.extend([self.p(f"{n}. {text}"), Spacer(1, 6)])

    def footer(self, canvas, doc):
        canvas.saveState()
        canvas.setStrokeColor(RULE)
        canvas.line(36, 39, A4[0] - 36, 39)
        canvas.setFont(self.font, 7.2)
        canvas.setFillColor(MUTED)
        status = "完成" if self.complete else ("本次范围完成 · 原需求含保留项" if self.release_scope_complete else "未完成")
        canvas.drawString(36, 27, "石墨三期复测记录 · " + status)
        canvas.drawRightString(A4[0] - 36, 27, f"{doc.page}")
        canvas.restoreState()

    def build(self):
        self.cover()
        for scene in self.data["scenes"]:
            self.scene(scene)
        self.appendix()
        self.output.parent.mkdir(parents=True, exist_ok=True)
        doc = SimpleDocTemplate(str(self.output), pagesize=A4,
            leftMargin=36, rightMargin=36, topMargin=33, bottomMargin=52,
            title="石墨三期阶段复测记录 - " + ("完成" if self.complete else ("本次范围完成（含保留项）" if self.release_scope_complete else "未完成")),
            author="shimo-net CLI 验收", pageCompression=1)
        doc.build(self.story, onFirstPage=self.footer, onLaterPages=self.footer)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cases", type=Path, default=ROOT / "docs/phase3-cases.json")
    parser.add_argument("--evidence-manifest", type=Path)
    parser.add_argument("--out", type=Path, default=ROOT / "output/pdf/石墨三期阶段复测记录.pdf")
    parser.add_argument("--built-at", help="可复现的生成时间标签；省略时使用上海当前时间")
    parser.add_argument("--full-evidence", action="store_true", help="展开全部原始日志和截图；默认正文展示代表截图并保留完整证据索引")
    args = parser.parse_args()
    cases_path = args.cases.resolve()
    manifest_path = args.evidence_manifest.resolve() if args.evidence_manifest else None
    built_at = args.built_at or datetime.now(ZoneInfo("Asia/Shanghai")).isoformat(timespec="seconds")
    data = prepare(cases_path, manifest_path)
    report = Report(data, args.out.resolve(), built_at, full_evidence=args.full_evidence)
    report.build()
    summary = {"output": str(report.output), "builtAt": built_at,
               "casesSource": str(cases_path), "manifestSource": str(manifest_path) if manifest_path else None,
               "sceneCount": len(data["scenes"]), "capabilityCount": sum(report.counts.values()),
               "counts": {key: report.counts[key] for key in LABELS}, "complete": report.complete,
               "releaseScopeComplete": report.release_scope_complete,
               "fullEvidenceRendered": report.full_evidence,
               "globalNegativeCases": data.get("globalNegativeCases", []),
               "evidence": {c["id"]: c["evidence"] for s in data["scenes"] for c in s["capabilities"]}}
    report.output.with_suffix(".summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({key: value for key, value in summary.items() if key != "evidence"}, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, KeyError, TypeError) as error:
        print(f"报告生成失败：{error}", file=sys.stderr)
        raise SystemExit(1)
