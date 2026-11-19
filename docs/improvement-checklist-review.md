# 改进建议清单：修复与逐项复核

> 公开摘要：本页记录逐项结论、代码和测试入口。完整自然语言复测报告、原始命令、截图、导出文件及哈希清单作为本地交付档案保留，不随源码公开；文中“本地验收档案”不是远端下载链接。只有在完成隐私审核并实际上传后，才补充可下载的 Release 附件链接。

> 后续状态（2026-11-19）：G1/G2真实双账号样本及单账号对照已完成，最终本地覆盖率达到81.35%，原生Keychain合成凭据往返通过；37.2关联字段经用户确认保留、不阻塞发布，G1–G5列明样本已完成，干净检出覆盖率81.14%已通过，发布仍需实际CI通过。下文测试数及未测描述是该次历史快照，最新结果见[发布前验收补充](release-acceptance-2026-11-19.md)。

这是前一轮 30 项清单的检查快照；后续 Issue #1 改变的运行边界与新增检查以 [Issue #1 修复记录](issue-1-review.md)为准，旧日志不冒充当前构建结果。

2026-11-19。依据桌面《office-suites改进建议清单.md》的 30 项建议，对本地工作区（HEAD `f2293db`，包含未提交修改）修复并复核。

**29 项已完成本地修复与检查；第 24 项的 CHANGELOG 和本地来源标签已补齐，远端标签/发布尚未执行。** 不将“本地已修复”写成“远端已发布”。修前为 4 完成、18 部分、8 未完成，原报告保留在修前记录（本地验收档案）。原建议文件未改写。

这是文档、能力分类、命令兼容和分发一致性检查，不是新增线上功能验收。以下为本轮补证前的历史快照（已由页首最新结果取代）：三期当时结果为 **27 项功能通过、1 项关联字段环境阻塞；G1–G5 待完整线上验收，双账号项缺第二账号**。全仓覆盖率未达 80%，不能宣称整体验收或发布就绪。

## 逐项结果

| 编号 | 状态 | 已落实的调整与核查证据 |
|---:|---|---|
| 1 | 已完成 | [场景 23、27](scenarios.md#编辑与协作)明确表格写入、插删行、格式和合并的 API；不再把整项笼统标成页面操作。 |
| 2 | 已完成 | [帮助/schema](../cli/schema.go)区分未暴露 SDK 的页面专用动作和已有面板动作；[命令参考](cli-reference.md#编辑器接口与页面入口)明确打开保存版本/评论面板不等于保存或提交成功；拒绝路径有[回归](../cli/capability_catalog_test.go)。 |
| 3 | 已完成 | [bootstrap](../bootstrap.sh)的 WorkBuddy 提示指向 SkillHub，且明确不是内置技能市场；清除旧英文注释。 |
| 4 | 已完成 | [登录说明](install.md#登录复用会话与切账号)解释普通 login 会复用旧账号，切账号必须 login --new；[四宿主教程](agent/README.md)同步并引用权威流程。 |
| 5 | 已完成 | [SECURITY](../SECURITY.md)删除“CLI 批量改权限”暗示；权限巡检是只读，权限写入交用户网页处理。 |
| 6 | 已完成 | [目录源](internal/capabilities.json)实际为 28 场景、51 条能力；[生成矩阵](internal/capability-matrix.md)、schema/help 同源，删除重复数据验证计数。 |
| 7 | 已完成 | [场景 6](scenarios.md#文件与目录)同时列 new 与 mkdir，区分 doc 轻文档和 docx 文稿；教程不再保留漏项的场景副本。 |
| 8 | 已完成 | [schema](../cli/schema.go)保留 sheet-append/insert/delete 已验证 API 列表，并从标准构建入口核验。 |
| 9 | 已完成 | [技能](agent/SKILL.md#按执行方式选入口)与[场景 24、27](scenarios.md#编辑与协作)统一底色使用 sheet-format 的 attrs.background，移除要求改走菜单的相反规程。 |
| 10 | 已完成 | [矩阵](internal/capability-matrix.md)拆分插删行 API 与插删列/工作表页面管理；不是整行一起标成页面操作。 |
| 11 | 已完成 | [浏览器优先级](scenarios.md#浏览器优先级)是唯一规则：宿主内置 → 经同意的 agent-browser → open → 最后 --default。教程、技能与 CLI 提示引用它；不再将 agent-browser 称为宿主内置能力。 |
| 12 | 已完成 | [场景 23](scenarios.md#编辑与协作)列 update/append/insert/delete；根 README 与四教程改为引用，不再各自遗漏。 |
| 13 | 已完成 | [WorkBuddy 教程](agent/workbuddy.md)、[历史连通记录](agent/validation.md)与 bootstrap 统一 SkillHub 的名称，区分 shimo-net-cli 技能与 agent-browser。 |
| 14 | 已完成 | [新建类型](cli-reference.md#新建类型)、schema/help 和实现对齐八种类型；已有 CHANGELOG 记录，创建能力不外推为全部编辑能力。 |
| 15 | 已完成 | 安装、场景、命令、技能与教程迁到 docs；[旧 cli 路径](../cli/README.md)及旧 tutorials 仅留短跳转，链接和分发检查纳入 CI。 |
| 16 | 已完成 | [根 README](../README.md)缩为短安装入口和导航；只保留一份[场景正文](scenarios.md)和一份[技能正文](agent/SKILL.md)，不再重复完整表。 |
| 17 | 已完成 | 原 cli README 与安装长文按职责合入[安装说明](install.md)和[命令参考](cli-reference.md)。 |
| 18 | 已完成 | [唯一场景](scenarios.md)覆盖 1–28：1–15 给出命令、话术与验收点，16–28 按子动作区分 API/页面/人工禁用；修正相对链接。 |
| 19 | 已完成 | [SKILL](agent/SKILL.md)缩为 54 行，按三类入口速查，引用唯一场景和浏览器规则；单文件分发的链接使用仓库绝对 URL。 |
| 20 | 已完成 | [WorkBuddy](agent/workbuddy.md)、[DSH](agent/dsh.md)、[豆包](agent/doubao.md)、[Codex](agent/codex.md)第 7 节均引用唯一场景，不再复制场景表。 |
| 21 | 已完成 | [内部矩阵](internal/capability-matrix.md)、[回归索引](internal/test-results.md)与图片迁入 docs/internal；记录按 0.9.4 → 0.9.6 → 0.9.7 正序保留，原日期冲突单列，不改写历史事实。 |
| 22 | 已完成 | [生成器](../scripts/generate_capabilities.py)从 JSON 生成 Go 计数/help 场景与 Markdown 矩阵；[回归测试](../cli/capability_catalog_test.go)和 CI 检查漂移。另以 LF 属性及真实 Git checkout 测试保证 Windows 换行不改变源摘要。 |
| 23 | 已完成 | 教程图片位于 docs/images/tutorials，内部截图位于 docs/internal/images；旧 docs/images/0.9.6 已移除。迁移图片核对字节一致，旧教程图仅为外链兼容保留。 |
| 24 | 本地完成，待发布 | [CHANGELOG](../CHANGELOG.md)补 0.9.2，统一中文并正序整理；带说明的本地 v0.9.6 来源标签指向源码确为 0.9.6 的提交。未 push/创建 Release，详见[来源与发布边界](internal/release-provenance.md)。 |
| 25 | 已完成 | 代码与技能为 0.9.15，原“停在 0.9.7”的前提已过时；[CHANGELOG 版本记录](../CHANGELOG.md)和命令参考明确当前源码可能领先发行包，未假称本轮已发新版本。 |
| 26 | 已完成 | [环境要求](install.md#环境要求)补 Brave 会话复用，并如实说明新登录/open 的自动发现仍只支持 Chrome/Chromium/Edge；用跨平台 doctor 取代固定 macOS 路径检查。 |
| 27 | 已完成 | [schema](../cli/schema.go)补 purge 拒绝命令与 version；[通用参考](cli-reference.md#配置与诊断)含 config/session/version，mkdir 在文件命令中；有 schema 契约测试。 |
| 28 | 已完成 | 新正式命令 editor 与 phase2 共用处理器，保留兼容；公开主体文档只在[命令参考](cli-reference.md)出现一次兼容别名说明。别名 API 请求、拒绝行为一致，phase3 禁止自动重放逻辑未改。 |
| 29 | 已完成 | 根 [LICENSE](../LICENSE)已存在，[README](../README.md#安全与许可)明确保留所有权利；没有冒称开源许可。 |
| 30 | 已完成 | [401 FAQ](install.md#faq)与四教程说明会自动尝试复用浏览器会话；换账号使用 login --new，并以 whoami 核验。 |

## 复核中补修的问题

- `.gitignore` 原 `internal/` 会意外忽略 docs/internal 全部新文档与机器源，现改成 `/internal/`；仍保留根私有资料忽略规则。文档检查新增 Git 可追踪性检查。
- macOS 安装器的 DSH 目标路径误替换，现修回 `~/.dsh/skills/shimo-net-cli/SKILL.md`；回归测试实际执行复制段并逐字比较安装结果。
- editor/兼容命令未做页面核验时，原文字输出声称“已完成”并打印未生成的截图。现明确“API 请求已完成，结果待页面核验”，只报告实际执行截图路径；JSON 仍明确未验证。读 API 不会因为传了 --verify 就报告未生成的截图。
- 能力 JSON 的原始字节参与摘要；新增 `.gitattributes` 固定 LF，并用开启 core.autocrlf 的临时 Git 仓库实际签出验证摘要不变。
- 检查脚本统一使用 POSIX 相对路径比较，并显式用 UTF-8 与 Git/gofmt 通信；非 UTF-8 子进程环境的生成和文档检查均有回归，避免 Windows 路径和区域设置造成误报。

前两项和 editor 输出问题先记录失败回归，再修复。独立只读复核覆盖清单、能力拆分、命令边界及安装分发，发现的问题均已处理。安装脚本、PowerShell 脚本和发布工作流均分发 `docs/agent/SKILL.md` 权威正文。

## 验证与尚未满足的条件

当前完整命令、退出码、日志、源码与二进制摘要、30 项机器可读结果见本轮验证清单（本地验收档案）。验证包含 Go 全量离线测试与覆盖率、vet/gofmt、Python 生成器/文档回归、报告测试、链接/分发/Git 忽略检查、shell 语法与 diff 检查，以及重建后的标准 `./shimo` 入口冒烟。

结果：Go 测试 741 个通过事件、0 失败、70 跳过；全仓覆盖率 56.0%；Python 回归 15 项、报告测试 17 项、46 份 Markdown 的检查及标准入口冒烟均通过。Go 全量后仅补修 Python 跨平台检查并更新报告，对这些变更单独重跑相关检查；产品 Go 源码与全量测试时一致。跳过和离线 mock 不计入线上通过数。

51 条目录是 **基础 19 + 编辑协作 32**；比原 50 多一条来自拆分“插删行 API”和“列/工作表页面操作”。分类为 API 26、页面操作 24、人工禁用 1，**不代表 51 个动作全部可自动执行**。

本轮未运行新的线上写入，也未重写三期历史 PDF。全仓覆盖率仍低于 80% 门槛；Windows CI 未在本机实际运行，仅本地执行了跨平台换行回归。第 24 项远端标签/Release 与整体发布不在此次本地修复结果内，`releaseReady` 保持 false。新文档和技能引用的 main 路径需随正式提交发布后才成为远端可访问内容。
