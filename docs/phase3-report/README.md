# 三期复测 PDF 生成器

从 `docs/phase3-cases.json` 和可选的真实执行清单生成 A4 报告。保持原场景编号 29-42，左侧列功能编号和状态，右侧列话术、实际命令、结果、断言和证据。功能项与全局负例均通过后才标记“完成”；只要双账号等全局项仍待验收，标题与每页页脚就保留“未完成”。

默认正文按场景卡展示实际命令、结果、物证数量及每项一张代表截图（优先重新打开后的截图），避免把大量原始 JSON 逐行展开。全部原图与日志保持完整，完整证据索引（路径、SHA-256、字节数）写入同名 summary JSON；PDF 中代表截图的哈希明确标注仅展示前 12 位。加 `--full-evidence` 可生成展开全部证据索引、原始日志及截图的详版。

不传执行清单时，模板输出为 28 项待测，仅代表报告样式与用例基线。真实验收必须传入 `output/evidence/phase3/run-manifest.json`，从该清单生成动态状态；不得把之前的模板 PDF 当作最新验收结果。生成器不改写用例文件。

## 运行

在仓库根目录运行：

```bash
python3 docs/phase3-report/build_report.py
```

其他环境可使用已安装 `reportlab` 的 Python 3.9+。默认优先使用 macOS `STHeiti Light` 字体；没有时回退到 ReportLab 的 `STSong-Light`。不自动安装依赖。

输出为：

- `output/pdf/石墨三期阶段复测记录.pdf`
- 同名 `.summary.json`，记录源路径、动态统计和证据文件 SHA-256

附真实执行清单：

```bash
python3 docs/phase3-report/build_report.py \
  --cases docs/phase3-cases.json \
  --evidence-manifest /absolute/path/run-manifest.json \
  --out output/pdf/石墨三期阶段复测记录.pdf
```

`--built-at '2026-11-18 18:00'` 可提供固定生成时间标签；省略时使用上海当前时间。

## 执行清单格式

`cases` 的键必须对应现有功能项 ID，例如 `29.1`，或全局反例 `G1`–`G5`。只覆盖执行记录字段，不允许改写需求、话术、预期断言或证据要求。以下是**格式示例，未执行，不是验收结果**：

```json
{
  "cases": {
    "29.1": {
      "status": "blocked",
      "actualCommand": "替换为本次实际执行命令",
      "observedResult": "替换为实际结果；不能编造输出",
      "reason": "填写具体阻塞条件和恢复办法",
      "executedAt": "2026-11-18T18:00:00+08:00",
      "exitCode": 1,
      "targetGuid": "本次测试目标GUID",
      "cliVersion": "版本及构建标识",
      "evidence": [
        {
          "name": "command-log",
          "path": "logs/29.1.stderr.json",
          "kind": "log",
          "label": "本次命令标准错误"
        }
      ]
    }
  }
}
```

路径相对执行清单所在目录解析，也接受绝对路径。用例 JSON 内已有证据路径则相对用例文件所在目录解析。

`evidence` 每项必须有 `name` 和 `path`；`name` 用于匹配用例的 `evidenceRequired`。支持：

| kind | 报告中的显示 |
|---|---|
| `log` | 默认在 summary JSON 列原文件及哈希索引。详版读取实际 UTF-8 文件，以“真实日志文本，非截图”标注，逐行完整排版；单文件超过 256 KiB 时拒绝展开，应先按测试项分割日志。 |
| `screenshot` | 默认每项嵌入一张代表原图，详版嵌入全部原图；两种模式均在 summary JSON 保留所有源文件路径和哈希索引，不生成模拟终端截图。 |
| `artifact` | 列出原文件路径、大小信息摘要中的字节数及 SHA-256，供打开 DOCX/PPTX/CSV 等复核。 |

没有证据时显示“未提供实测截图或文件证据”；没有命令时显示“未提供可执行命令”。缺失文件明确显示“证据缺失”。`evidenceRequired` 中以 `/` 结尾的成组证据可提供一个实际归档文件，并使用相同 `name`，例如 `name: "intermediate-exports/"` 对应 ZIP 路径。

另可填写 `logText`，但只有真实执行文本才可使用。建议提供日志文件以保留哈希和可追溯来源。日志中明显的 cookie、sid、Authorization 和 token 值会在报告文本里脱敏；原文件不会改动。提供清单前仍应检查证据中是否含凭据或无关个人资料。

## “通过”的硬检查

状态为 `passed` 时必须同时具备：

1. 非空 `actualCommand`、`observedResult`、`executedAt`、`targetGuid`、`cliVersion`。
2. 数值 `exitCode: 0`。
3. `evidenceRequired` 中每个名字都有对应的真实、非空本地文件。
4. `assertions` 数量与 `expected` 一致，每项有 `description` 且 `passed: true`，顺序逐条对应预期断言。

全局反例的“通过”表示命令正确拒绝了测试请求，因此要求真实的非 0 整数退出码、至少一份存在且非空的失败物证，以及对应其 `expected` 文本的一条通过断言；其余执行元数据同样必须齐全。全局记录及证据也进入 PDF 附录和 summary JSON。只有 28 个功能编号及其场景归属完整、唯一，并且 G1–G5 均有通过记录时，报告才允许标记完成；删除或重复待测条目不能绕过门槛。

证据不全会报错并以非 0 退出，拒绝输出虚假通过数。此检查验证材料齐备，不证明执行真实性或截图内容正确；测试执行者仍需核对全部子动作、同一目标 GUID 和刷新后的持久化结果。不得把本地示例或 Mock 测试标成线上通过。

```json
"assertions": [
  {"description": "对应 expected 第一项的实际验证", "passed": true},
  {"description": "对应 expected 第二项的实际验证", "passed": true}
]
```

所有状态统计来自合并后的功能项，不手写统计数字。需求标题的“29 项”与正文“28 项”差异仍保留在首页，不通过添加条目凑数。

## 渲染与视觉复核

```bash
mkdir -p output/pdf/rendered
pdftoppm -png -scale-to 1200 \
  output/pdf/石墨三期阶段复测记录.pdf \
  output/pdf/rendered/phase3
```

每次真实清单更新后重新渲染，检查所有页面中的文字、命令、截图、续页和页脚。功能项过长可以跨页，不能为了压页删掉失败内容或截断日志。视觉检查记录位于 `.omx/state/phase3-report/ralph-progress.json`，仅适用于记录中指定的 PDF 版本，不能当成后续新证据版的视觉通过结果。

## 用户确认的保留项

`deferred`显示为“保留”，不计入通过数。必须提供`deferral`中的用户原话、日期、范围、`requestedBy: user`和`releaseBlocking: false`；缺失会拒绝生成。G1–G5全局门槛不允许保留跳过。原需求`complete`仍要求全部通过；`releaseScopeComplete`单独反映用户确认的本次发布范围，仍要求所有全局门槛通过。当前37.2仅关联子功能由用户保留，字段增删改和类型转换的原证据保留，不改写原需求。
